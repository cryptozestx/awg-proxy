package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type TunnelDeps struct {
	DeviceFactory TunnelDeviceFactory
	RouteManager  RouteManager
	DNSManager    DNSManager
	Lookup        func(string) ([]netip.Addr, error)
	Wait          func(context.Context) error
}

func RunTunnel(cfg *AWGConfig, opts TunnelOptions) error {
	ctx := context.Background()
	var runner CommandRunner = ExecRunner{}
	var deviceFactory TunnelDeviceFactory = AWGTunnelDeviceFactory{}
	if opts.DryRun {
		runner = NewDryRunRunner()
		deviceFactory = dryRunTunnelDeviceFactory{}
	}

	deps := TunnelDeps{
		DeviceFactory: deviceFactory,
		RouteManager:  NewPlatformRouteManager(runner),
		DNSManager:    NewPlatformDNSManager(runner),
		Lookup:        netipLookup,
		Wait:          waitForSignal,
	}
	return RunTunnelWithDeps(ctx, cfg, opts, deps)
}

func RunTunnelWithDeps(ctx context.Context, cfg *AWGConfig, opts TunnelOptions, deps TunnelDeps) (retErr error) {
	if ctx == nil {
		return fmt.Errorf("tunnel context is nil")
	}
	if opts.DryRun {
		deps.DeviceFactory = dryRunTunnelDeviceFactory{}
	} else if deps.DeviceFactory == nil {
		return fmt.Errorf("tunnel dependency DeviceFactory is nil")
	}
	if deps.RouteManager == nil {
		return fmt.Errorf("tunnel dependency RouteManager is nil")
	}
	if !opts.NoDNS && deps.DNSManager == nil {
		return fmt.Errorf("tunnel dependency DNSManager is nil")
	}
	if deps.Lookup == nil {
		return fmt.Errorf("tunnel dependency Lookup is nil")
	}
	if deps.Wait == nil {
		return fmt.Errorf("tunnel dependency Wait is nil")
	}

	tcfg, err := ValidateTunnelConfig(cfg)
	if err != nil {
		return err
	}

	endpoint, err := ResolveEndpointIPv4(tcfg.EndpointHost, tcfg.EndpointPort, deps.Lookup)
	if err != nil {
		return err
	}

	dnsServers := cfg.Interface.DNS
	if !opts.NoDNS {
		dnsServers, err = tunnelDNSServers(cfg.Interface.DNS)
		if err != nil {
			return err
		}
	}

	mtu := cfg.Interface.MTU
	if mtu <= 0 {
		mtu = 1420
	}

	cleanup := NewCleanupStack()
	defer func() {
		if err := cleanup.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup failed: %v\n", err)
			retErr = errors.Join(retErr, err)
		}
	}()

	dev, err := deps.DeviceFactory.Create("awgproxy0", mtu, opts.Verbose)
	if err != nil {
		return fmt.Errorf("create tunnel device: %w", err)
	}
	cleanup.Add("close tunnel device", dev.Close)

	if err := deps.RouteManager.ConfigureInterface(ctx, dev.Name(), tcfg.InterfaceIPv4, mtu); err != nil {
		return err
	}

	uapi, err := BuildResolvedTunnelUAPI(cfg, endpoint)
	if err != nil {
		return err
	}
	if err := dev.Up(uapi); err != nil {
		return err
	}

	defaultRoute, err := deps.RouteManager.DefaultRoute(ctx)
	if err != nil {
		return err
	}

	plan := BuildFullTunnelRoutePlan(endpoint)
	if err := deps.RouteManager.Apply(ctx, dev.Name(), plan, defaultRoute, cleanup); err != nil {
		return err
	}

	if !opts.NoDNS {
		if err := deps.DNSManager.Apply(ctx, dnsServers, cleanup); err != nil {
			return err
		}
	}

	return deps.Wait(ctx)
}

func waitForSignal(ctx context.Context) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-signals:
		return nil
	}
}

func netipLookup(host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(context.Background(), "ip", host)
}

func tunnelDNSServers(servers []string) ([]string, error) {
	if len(servers) == 0 {
		return nil, fmt.Errorf("tunnel DNS is empty; set [Interface] DNS or use --no-dns")
	}

	result := make([]string, 0, len(servers))
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server == "" {
			return nil, fmt.Errorf("tunnel DNS contains an empty server; set [Interface] DNS or use --no-dns")
		}
		if _, err := netip.ParseAddr(server); err != nil {
			return nil, fmt.Errorf("invalid tunnel DNS server %q: %w", server, err)
		}
		result = append(result, server)
	}

	return result, nil
}

type dryRunTunnelDeviceFactory struct{}

func (dryRunTunnelDeviceFactory) Create(name string, _ int, _ bool) (TunnelDevice, error) {
	return dryRunTunnelDevice{name: name}, nil
}

type dryRunTunnelDevice struct {
	name string
}

func (d dryRunTunnelDevice) Name() string {
	return d.name
}

func (dryRunTunnelDevice) Up(string) error {
	return nil
}

func (dryRunTunnelDevice) Close() error {
	return nil
}
