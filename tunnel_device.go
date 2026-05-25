package main

import (
	"fmt"
	"net/netip"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun"
)

type TunnelDevice interface {
	Name() string
	Up(uapi string) error
	Close() error
}

type TunnelDeviceFactory interface {
	Create(name string, mtu int, verbose bool) (TunnelDevice, error)
}

type AWGTunnelDeviceFactory struct{}

type AWGTunnelDevice struct {
	name string
	tun  tun.Device
	dev  *device.Device
}

func (AWGTunnelDeviceFactory) Create(name string, mtu int, verbose bool) (TunnelDevice, error) {
	tunDev, err := tun.CreateTUN(name, mtu)
	if err != nil {
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

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(level, "[AWG] "))
	return &AWGTunnelDevice{name: actualName, tun: tunDev, dev: dev}, nil
}

func (d *AWGTunnelDevice) Name() string {
	return d.name
}

func (d *AWGTunnelDevice) Up(uapi string) error {
	if err := d.dev.IpcSet(uapi); err != nil {
		return fmt.Errorf("apply tunnel UAPI: %w", err)
	}
	if err := d.dev.Up(); err != nil {
		return fmt.Errorf("start tunnel device: %w", err)
	}
	return nil
}

func (d *AWGTunnelDevice) Close() error {
	d.dev.Close()
	return d.tun.Close()
}

func BuildResolvedTunnelUAPI(cfg *AWGConfig, endpoint netip.AddrPort) (string, error) {
	return CloneConfigWithResolvedEndpoint(cfg, endpoint).ToUAPI()
}
