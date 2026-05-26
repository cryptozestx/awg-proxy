package awgnet

import (
	"fmt"
	"log"
	"net/netip"
	"strings"

	"awg-proxy/internal/config"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun/netstack"
)

const defaultMTU = 1420

var defaultDNS = netip.MustParseAddr("1.1.1.1")

type Session struct {
	Dialer *netstack.Net
	dev    *device.Device
}

func Start(cfg *config.AWGConfig, debug bool) (*Session, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	localAddrs, err := parseAddresses(cfg.Interface.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse interface addresses: %w", err)
	}
	if len(localAddrs) == 0 {
		return nil, fmt.Errorf("no interface IP addresses defined in [Interface]")
	}

	dnsAddrs, err := parseAddresses(cfg.Interface.DNS)
	if err != nil {
		log.Printf("[Warning] DNS parse issue: %v. Defaulting to 1.1.1.1.", err)
		dnsAddrs = []netip.Addr{defaultDNS}
	}
	if len(dnsAddrs) == 0 {
		dnsAddrs = []netip.Addr{defaultDNS}
	}

	mtu := cfg.Interface.MTU
	if mtu <= 0 {
		mtu = defaultMTU
	}

	fmt.Println("[awg-proxy] Initializing userspace network stack...")
	tunDev, tnet, err := netstack.CreateNetTUN(localAddrs, dnsAddrs, mtu)
	if err != nil {
		return nil, fmt.Errorf("failed to create userspace network stack: %w", err)
	}

	logLevel := device.LogLevelSilent
	if debug {
		logLevel = device.LogLevelVerbose
	}
	logger := device.NewLogger(logLevel, "[AWG] ")
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	fmt.Println("[awg-proxy] Setting up secure AmneziaWG connection tunnel...")
	uapiConf, err := cfg.ToUAPI()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("failed to construct UAPI config: %w", err)
	}

	if err := dev.IpcSet(uapiConf); err != nil {
		dev.Close()
		return nil, fmt.Errorf("failed to configure AmneziaWG interface keys & obfuscation: %w", err)
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("failed to establish tunnel connection: %w", err)
	}

	return &Session{
		Dialer: tnet,
		dev:    dev,
	}, nil
}

func (s *Session) Close() {
	if s == nil || s.dev == nil {
		return
	}
	s.dev.Close()
}

func parseAddresses(addrs []string) ([]netip.Addr, error) {
	var result []netip.Addr
	for _, a := range addrs {
		ipStr := a
		if idx := strings.Index(a, "/"); idx >= 0 {
			ipStr = a[:idx]
		}
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			return nil, fmt.Errorf("invalid IP address: %s", a)
		}
		result = append(result, ip)
	}
	return result, nil
}
