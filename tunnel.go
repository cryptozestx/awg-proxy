package main

import (
	"awg-proxy/internal/config"
	dnsruntime "awg-proxy/internal/dns"
	"awg-proxy/internal/platform"
	"awg-proxy/internal/routing"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const endpointLookupTimeout = 10 * time.Second

type TunnelDeps struct {
	DeviceFactory        TunnelDeviceFactory
	RouteManager         routing.Manager
	DNSManager           dnsruntime.Manager
	DomainRuntimeFactory func() dnsruntime.DomainBypassRuntime
	DynamicRoutesFactory routing.DynamicBypassRouteFactory
	Lookup               func(string) ([]netip.Addr, error)
	Wait                 func(context.Context) error
}

type DryRunRecorder interface {
	RecordDryRun(string)
}

func RunTunnel(cfg *config.AWGConfig, opts TunnelOptions) error {
	ctx := context.Background()
	if opts.DryRun {
		runner := platform.NewDryRunRunnerWithOutput(platform.ExecRunner{})
		deps := TunnelDeps{
			DeviceFactory: dryRunTunnelDeviceFactory{Recorder: runner},
			RouteManager: dryRunRouteManager{
				RouteManager: routing.NewPlatformManager(runner),
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
		RouteManager:  routing.NewPlatformManager(platform.ExecRunner{}),
		DNSManager:    dnsruntime.NewPlatformManager(platform.ExecRunner{}),
		Lookup:        netipLookup,
		Wait:          waitForSignal,
	}
	return RunTunnelWithDeps(ctx, cfg, opts, deps)
}

func RunTunnelWithDeps(ctx context.Context, cfg *config.AWGConfig, opts TunnelOptions, deps TunnelDeps) (retErr error) {
	if ctx == nil {
		return fmt.Errorf("tunnel context is nil")
	}
	if opts.DryRun {
		recorder := dryRunRecorderFromDeps(deps)
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
		deps.DomainRuntimeFactory = func() dnsruntime.DomainBypassRuntime {
			return dryRunDomainBypassRuntime{Recorder: recorder}
		}
		deps.DynamicRoutesFactory = func(routing.DefaultRoute) routing.DynamicBypassRoutes {
			return dryRunDynamicBypassRoutes{Recorder: recorder}
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
	if deps.DomainRuntimeFactory == nil {
		deps.DomainRuntimeFactory = dnsruntime.NewDomainBypassRuntime
	}
	if deps.DynamicRoutesFactory == nil {
		deps.DynamicRoutesFactory = routing.NewPlatformDynamicBypassRoutes
	}

	tcfg, err := ValidateTunnelConfig(cfg)
	if err != nil {
		return err
	}
	logTunnelProgress(opts, "Tunnel config validated: interface %s, endpoint %s:%d.", tcfg.InterfaceIPv4, tcfg.EndpointHost, tcfg.EndpointPort)

	logTunnelProgress(opts, "Resolving endpoint %s...", tcfg.EndpointHost)
	endpoint, err := ResolveEndpointIPv4(tcfg.EndpointHost, tcfg.EndpointPort, deps.Lookup)
	if err != nil {
		return err
	}
	logTunnelProgress(opts, "Endpoint resolved: %s.", endpoint)

	dnsServers := cfg.Interface.DNS
	if !opts.NoDNS {
		logTunnelProgress(opts, "Validating DNS servers...")
		dnsServers, err = tunnelDNSServers(cfg.Interface.DNS)
		if err != nil {
			return err
		}
		logTunnelProgress(opts, "DNS servers validated: %s.", strings.Join(dnsServers, ", "))
	} else {
		logTunnelProgress(opts, "DNS changes disabled.")
	}

	rules, err := LoadTunnelRules(opts.RulesPath)
	if err != nil {
		return err
	}
	if opts.NoDNS && rules.HasDomainRules() {
		return fmt.Errorf("domain bypass rules require tunnel DNS control; remove --no-dns or remove exclude_domain rules")
	}

	mtu := cfg.Interface.MTU
	if mtu <= 0 {
		mtu = 1420
	}
	logTunnelProgress(opts, "Using MTU %d.", mtu)

	cleanup := NewCleanupStack()
	defer func() {
		if err := cleanup.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup failed: %v\n", err)
			retErr = errors.Join(retErr, err)
		}
	}()

	logTunnelProgress(opts, "Creating native TUN device %s...", defaultTunnelName())
	dev, err := deps.DeviceFactory.Create(defaultTunnelName(), mtu, opts.Verbose)
	if err != nil {
		return fmt.Errorf("create tunnel device: %w", err)
	}
	cleanup.Add("close tunnel device", dev.Close)
	logTunnelProgress(opts, "Created native TUN device %s.", dev.Name())

	logTunnelProgress(opts, "Configuring interface %s...", dev.Name())
	if err := deps.RouteManager.ConfigureInterface(ctx, dev.Name(), tcfg.InterfaceIPv4, mtu); err != nil {
		return err
	}
	logTunnelProgress(opts, "Interface %s configured.", dev.Name())

	logTunnelProgress(opts, "Building resolved AmneziaWG config...")
	uapi, err := BuildResolvedTunnelUAPI(cfg, endpoint)
	if err != nil {
		return err
	}
	logTunnelProgress(opts, "Starting AmneziaWG device...")
	if err := dev.Up(uapi); err != nil {
		return err
	}
	logTunnelProgress(opts, "AmneziaWG device started.")

	logTunnelProgress(opts, "Discovering default route...")
	defaultRoute, err := deps.RouteManager.DefaultRoute(ctx)
	if err != nil {
		return err
	}
	logTunnelProgress(opts, "Default route: gateway %s dev %s.", defaultRoute.Gateway, defaultRoute.Device)

	plan := routing.BuildTunnelPlan(endpoint, rules)
	logTunnelProgress(opts, "Applying tunnel routes...")
	if err := deps.RouteManager.Apply(ctx, dev.Name(), plan, defaultRoute, cleanup); err != nil {
		return err
	}
	logTunnelProgress(opts, "Tunnel routes applied.")

	if rules.HasDomainRules() {
		dynamicRoutes := staticAwareDynamicBypassRoutes{
			DynamicBypassRoutes: deps.DynamicRoutesFactory(defaultRoute),
			staticBypassCIDRs:   plan.StaticBypassCIDRs,
		}
		cleanup.Add("delete dynamic domain bypass routes", dynamicRoutes.Close)
		runtime := deps.DomainRuntimeFactory()
		if err := runtime.Start(ctx, dnsruntime.DomainBypassConfig{
			ListenAddr: "127.0.0.1:53",
			Upstream:   net.JoinHostPort(dnsServers[0], "53"),
			Rules:      rules.DNSDomainRules(),
			Routes:     dynamicRoutes,
		}); err != nil {
			return err
		}
		cleanup.Add("stop domain bypass DNS runtime", runtime.Close)
		dnsServers = []string{"127.0.0.1"}
	}

	if !opts.NoDNS {
		logTunnelProgress(opts, "Applying DNS settings...")
		if err := deps.DNSManager.Apply(ctx, dnsServers, cleanup); err != nil {
			return err
		}
		logTunnelProgress(opts, "DNS settings applied.")
	}

	logTunnelProgress(opts, "Tunnel is up. Press Ctrl+C to stop.")
	return deps.Wait(ctx)
}

type staticAwareDynamicBypassRoutes struct {
	routing.DynamicBypassRoutes
	staticBypassCIDRs []netip.Prefix
}

func (r staticAwareDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	for _, staticPrefix := range r.staticBypassCIDRs {
		if staticPrefix.Contains(prefix.Addr()) {
			return nil
		}
	}
	return r.DynamicBypassRoutes.AddBypassRoute(ctx, prefix, reason, ttl)
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
	ctx, cancel := context.WithTimeout(context.Background(), endpointLookupTimeout)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func logTunnelProgress(opts TunnelOptions, format string, args ...any) {
	if opts.DryRun {
		return
	}
	fmt.Printf("[awg-proxy] "+format+"\n", args...)
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
	RouteManager routing.Manager
	Recorder     DryRunRecorder
	Fallback     routing.DefaultRoute
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

func (m dryRunRouteManager) DefaultRoute(ctx context.Context) (routing.DefaultRoute, error) {
	if m.RouteManager == nil {
		return m.defaultRouteFallback(nil), nil
	}

	route, err := m.RouteManager.DefaultRoute(ctx)
	if err == nil {
		return route, nil
	}

	return m.defaultRouteFallback(err), nil
}

func (m dryRunRouteManager) defaultRouteFallback(discoveryErr error) routing.DefaultRoute {
	fallback := m.Fallback
	if !fallback.Gateway.IsValid() || fallback.Device == "" {
		fallback = dryRunDefaultRouteFallback()
	}
	if m.Recorder != nil && discoveryErr != nil {
		m.Recorder.RecordDryRun(fmt.Sprintf("default route discovery failed: %v; using dry-run placeholder gateway %s dev %s", discoveryErr, fallback.Gateway, fallback.Device))
	}
	return fallback
}

func (m dryRunRouteManager) Apply(ctx context.Context, ifName string, plan routing.Plan, defaultRoute routing.DefaultRoute, cleanup routing.Cleanup) error {
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

	for _, cidr := range plan.StaticBypassCIDRs {
		cidrText := cidr.String()
		if m.Recorder != nil {
			m.Recorder.RecordDryRun(fmt.Sprintf("add static bypass route %s via %s dev %s", cidrText, defaultRoute.Gateway, defaultRoute.Device))
		}
		cleanup.Add("delete static bypass route "+cidrText, func() error {
			if m.Recorder != nil {
				m.Recorder.RecordDryRun("delete static bypass route " + cidrText)
			}
			return nil
		})
	}

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

func dryRunDefaultRouteFallback() routing.DefaultRoute {
	return routing.DefaultRoute{
		Gateway: netip.MustParseAddr("192.0.2.254"),
		Device:  "default0",
	}
}

func dryRunRecorderFromDeps(deps TunnelDeps) DryRunRecorder {
	switch factory := deps.DeviceFactory.(type) {
	case dryRunTunnelDeviceFactory:
		if factory.Recorder != nil {
			return factory.Recorder
		}
	case *dryRunTunnelDeviceFactory:
		if factory != nil && factory.Recorder != nil {
			return factory.Recorder
		}
	}
	switch manager := deps.RouteManager.(type) {
	case dryRunRouteManager:
		if manager.Recorder != nil {
			return manager.Recorder
		}
	case *dryRunRouteManager:
		if manager != nil && manager.Recorder != nil {
			return manager.Recorder
		}
	}
	switch manager := deps.DNSManager.(type) {
	case dryRunDNSManager:
		return manager.Recorder
	case *dryRunDNSManager:
		if manager != nil {
			return manager.Recorder
		}
	}
	return nil
}

type dryRunDomainBypassRuntime struct {
	Recorder DryRunRecorder
}

func (r dryRunDomainBypassRuntime) Start(ctx context.Context, config dnsruntime.DomainBypassConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.Recorder != nil {
		r.Recorder.RecordDryRun(fmt.Sprintf("start domain bypass DNS runtime %s upstream %s", config.ListenAddr, config.Upstream))
	}
	return nil
}

func (r dryRunDomainBypassRuntime) Addr() string {
	return ""
}

func (r dryRunDomainBypassRuntime) Close() error {
	if r.Recorder != nil {
		r.Recorder.RecordDryRun("stop domain bypass DNS runtime")
	}
	return nil
}

type dryRunDynamicBypassRoutes struct {
	Recorder DryRunRecorder
}

func (r dryRunDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.Recorder != nil {
		r.Recorder.RecordDryRun(fmt.Sprintf("add dynamic domain bypass route %s reason %s ttl %s", prefix, reason, ttl))
	}
	return nil
}

func (r dryRunDynamicBypassRoutes) Close() error {
	if r.Recorder != nil {
		r.Recorder.RecordDryRun("delete dynamic domain bypass routes")
	}
	return nil
}

type dryRunDNSManager struct {
	Recorder DryRunRecorder
}

func (m dryRunDNSManager) Apply(ctx context.Context, servers []string, cleanup dnsruntime.Cleanup) error {
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
