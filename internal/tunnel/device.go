package tunnel

import (
	"awg-proxy/internal/config"
	"fmt"
	"net/netip"
	"os"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun"
)

type Device interface {
	Name() string
	Up(uapi string) error
	Close() error
}

type DeviceFactory interface {
	Create(name string, mtu int, verbose bool) (Device, error)
}

type AWGDeviceFactory struct{}

type AWGDevice struct {
	name string
	tun  tun.Device
	dev  awgDevice
}

func (AWGDeviceFactory) Create(name string, mtu int, verbose bool) (Device, error) {
	tunDev, err := createTUN(name, mtu)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("create TUN device: permission denied; tunnel mode requires elevated privileges on macOS/Linux, run with sudo: %w", err)
		}
		return nil, fmt.Errorf("create TUN device: %w", err)
	}

	actualName, err := tunDev.Name()
	if err != nil {
		_ = tunDev.Close()
		return nil, fmt.Errorf("get TUN device name: %w", err)
	}

	level := device.LogLevelSilent
	if verbose {
		level = device.LogLevelVerbose
	}

	dev := newAWGDevice(tunDev, level)
	return &AWGDevice{name: actualName, tun: tunDev, dev: dev}, nil
}

func (d *AWGDevice) Name() string {
	return d.name
}

func (d *AWGDevice) Up(uapi string) error {
	if err := d.dev.IpcSet(uapi); err != nil {
		return fmt.Errorf("apply tunnel UAPI: %w", err)
	}
	if err := d.dev.Up(); err != nil {
		return fmt.Errorf("start tunnel device: %w", err)
	}
	return nil
}

func (d *AWGDevice) Close() error {
	// amneziawg-go device owns closing the underlying TUN.
	d.dev.Close()
	return nil
}

func BuildResolvedUAPI(cfg *config.AWGConfig, endpoint netip.AddrPort) (string, error) {
	return CloneConfigWithResolvedEndpoint(cfg, endpoint).ToUAPI()
}

type awgDevice interface {
	IpcSet(string) error
	Up() error
	Close()
}

var createTUN = tun.CreateTUN

var newAWGDevice = func(tunDev tun.Device, level int) awgDevice {
	return device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(level, "[AWG] "))
}
