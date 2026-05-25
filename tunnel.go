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

type DryRunRecorder interface {
	RecordDryRun(string)
}

func RunTunnel(cfg *AWGConfig, opts TunnelOptions) error {
	ctx := context.Background()
	if opts.DryRun {
		runner := NewDryRunRunnerWithOutput(ExecRunner{})
		deps := TunnelDeps{
			DeviceFactory: dryRunTunnelDeviceFactory{Recorder: runner},
			RouteManager: dryRunRouteManager{
				RouteManager: NewPlatformRouteManager(runner),
				Recorder:     runner,
				Fallback:     dryRunDefaultRouteFallback(),
			},
			DNSManager: dryRunDNSManager{Recorder: runner},
			Lookup:     netipLookup,
			Wait: func(context.Context) error {
				return nil
			},
		}
		err := RunTunnelWithDeps(ctx, cfg, opts, deps)
		printDryRunPlan(runner.Commands())
		return err
	}

	deps := TunnelDeps{
		DeviceFactory: AWGTunnelDeviceFactory{},
		RouteManager:  NewPlatformRouteManager(ExecRunner{}),
		DNSManager:    NewPlatformDNSManager(ExecRunner{}),
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
		switch deps.DeviceFactory.(type) {
		case dryRunTunnelDeviceFactory, *dryRunTunnelDeviceFactory:
		default:
			deps.DeviceFactory = dryRunTunnelDeviceFactory{}
		}
		if deps.RouteManager != nil {
			switch deps.RouteManager.(type) {
			case dryRunRouteManager, *dryRunRouteManager:
			default:
				deps.RouteManager = dryRunRouteManager{
					RouteManager: deps.RouteManager,
					Fallback:     dryRunDefaultRouteFallback(),
				}
			}
		}
		if !opts.NoDNS {
			switch deps.DNSManager.(type) {
			case dryRunDNSManager, *dryRunDNSManager:
			default:
				deps.DNSManager = dryRunDNSManager{}
			}
		}
		deps.Wait = func(context.Context) error {
			return nil
		}
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

	dev, err := deps.DeviceFactory.Create(defaultTunnelName(), mtu, opts.Verbose)
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

func printDryRunPlan(commands []string) {
	if len(commands) == 0 {
		return
	}
	fmt.Println("[awg-proxy] Dry-run plan (no system changes applied):")
	for _, command := range commands {
		fmt.Printf("  - %s\n", command)
	}
}

type dryRunTunnelDeviceFactory struct {
	Recorder DryRunRecorder
}

func (f dryRunTunnelDeviceFactory) Create(name string, mtu int, _ bool) (TunnelDevice, error) {
	if f.Recorder != nil {
		f.Recorder.RecordDryRun(fmt.Sprintf("create native TUN device %s mtu %d", name, mtu))
	}
	return dryRunTunnelDevice{name: name, recorder: f.Recorder}, nil
}

type dryRunTunnelDevice struct {
	name     string
	recorder DryRunRecorder
}

func (d dryRunTunnelDevice) Name() string {
	return d.name
}

func (d dryRunTunnelDevice) Up(string) error {
	if d.recorder != nil {
		d.recorder.RecordDryRun("bring up AmneziaWG device with resolved endpoint config")
	}
	return nil
}

func (d dryRunTunnelDevice) Close() error {
	if d.recorder != nil {
		d.recorder.RecordDryRun("close native TUN device")
	}
	return nil
}

type dryRunRouteManager struct {
	RouteManager RouteManager
	Recorder     DryRunRecorder
	Fallback     DefaultRoute
}

func (m dryRunRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Recorder != nil {
		m.Recorder.RecordDryRun(fmt.Sprintf("configure interface %s address %s mtu %d up", ifName, addr, mtu))
	}
	return nil
}

func (m dryRunRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	if m.RouteManager == nil {
		return m.defaultRouteFallback(nil), nil
	}

	route, err := m.RouteManager.DefaultRoute(ctx)
	if err == nil {
		return route, nil
	}

	return m.defaultRouteFallback(err), nil
}

func (m dryRunRouteManager) defaultRouteFallback(discoveryErr error) DefaultRoute {
	fallback := m.Fallback
	if !fallback.Gateway.IsValid() || fallback.Device == "" {
		fallback = dryRunDefaultRouteFallback()
	}
	if m.Recorder != nil && discoveryErr != nil {
		m.Recorder.RecordDryRun(fmt.Sprintf("default route discovery failed: %v; using dry-run placeholder gateway %s dev %s", discoveryErr, fallback.Gateway, fallback.Device))
	}
	return fallback
}

func (m dryRunRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	endpointIP := plan.EndpointBypass.Addr().String()
	if m.Recorder != nil {
		m.Recorder.RecordDryRun(fmt.Sprintf("add endpoint bypass route %s via %s dev %s", endpointIP, defaultRoute.Gateway, defaultRoute.Device))
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		if m.Recorder != nil {
			m.Recorder.RecordDryRun("delete endpoint bypass route " + endpointIP)
		}
		return nil
	})

	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if m.Recorder != nil {
			m.Recorder.RecordDryRun(fmt.Sprintf("add tunnel route %s via interface %s", cidrText, ifName))
		}
		cleanup.Add("delete tunnel route "+cidrText, func() error {
			if m.Recorder != nil {
				m.Recorder.RecordDryRun("delete tunnel route " + cidrText)
			}
			return nil
		})
	}

	return nil
}

func dryRunDefaultRouteFallback() DefaultRoute {
	return DefaultRoute{
		Gateway: netip.MustParseAddr("192.0.2.254"),
		Device:  "default0",
	}
}

type dryRunDNSManager struct {
	Recorder DryRunRecorder
}

func (m dryRunDNSManager) Apply(ctx context.Context, servers []string, cleanup *CleanupStack) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Recorder != nil {
		m.Recorder.RecordDryRun("set DNS servers " + strings.Join(servers, ", "))
	}
	cleanup.Add("restore DNS servers", func() error {
		if m.Recorder != nil {
			m.Recorder.RecordDryRun("restore previous DNS settings")
		}
		return nil
	})
	return nil
}
