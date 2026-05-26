package main

import (
	"awg-proxy/internal/platform"
	"context"
	"errors"
	"io"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeTunnelDevice struct {
	name     string
	closed   bool
	upUAPI   string
	closeErr error
}

func (d *fakeTunnelDevice) Name() string {
	return d.name
}

func (d *fakeTunnelDevice) Up(uapi string) error {
	d.upUAPI = uapi
	return nil
}

func (d *fakeTunnelDevice) Close() error {
	d.closed = true
	return d.closeErr
}

type fakeTunnelDeviceFactory struct {
	dev     *fakeTunnelDevice
	name    string
	mtu     int
	verbose bool
	called  bool
}

func (f *fakeTunnelDeviceFactory) Create(name string, mtu int, verbose bool) (TunnelDevice, error) {
	f.called = true
	f.name = name
	f.mtu = mtu
	f.verbose = verbose
	return f.dev, nil
}

type fakeRouteManager struct {
	calls      []string
	defaults   int
	defaultErr error
	applyErr   error
	lastPlan   RoutePlan
}

func (m *fakeRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	m.calls = append(m.calls, "configure:"+ifName+":"+addr.String())
	return ctx.Err()
}

func (m *fakeRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	m.defaults++
	if m.defaultErr != nil {
		return DefaultRoute{}, m.defaultErr
	}
	return DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"}, ctx.Err()
}

func (m *fakeRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	m.calls = append(m.calls, "routes:"+ifName)
	m.lastPlan = plan
	cleanup.Add("routes", func() error {
		m.calls = append(m.calls, "cleanup-routes")
		return nil
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.applyErr
}

type fakeDNSManager struct {
	calls        []string
	orderedCalls *[]string
	servers      []string
}

func (m *fakeDNSManager) Apply(ctx context.Context, servers []string, cleanup *CleanupStack) error {
	m.calls = append(m.calls, "dns")
	if m.orderedCalls != nil {
		*m.orderedCalls = append(*m.orderedCalls, "dns")
	}
	m.servers = append([]string(nil), servers...)
	cleanup.Add("dns", func() error {
		m.calls = append(m.calls, "cleanup-dns")
		if m.orderedCalls != nil {
			*m.orderedCalls = append(*m.orderedCalls, "cleanup-dns")
		}
		return nil
	})
	return ctx.Err()
}

type fakeDomainRuntime struct {
	calls   *[]string
	config  DomainBypassConfig
	onStart func(context.Context, DomainBypassConfig) error
}

func (r *fakeDomainRuntime) Start(ctx context.Context, config DomainBypassConfig) error {
	*r.calls = append(*r.calls, "domain-runtime-start")
	r.config = config
	if r.onStart != nil {
		if err := r.onStart(ctx, config); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (r *fakeDomainRuntime) Addr() string {
	return "127.0.0.1:5353"
}

func (r *fakeDomainRuntime) Close() error {
	*r.calls = append(*r.calls, "domain-runtime-close")
	return nil
}

func (r *fakeDomainRuntime) HandleAnswer(ctx context.Context, rules TunnelRules, answer DNSAnswer, routes DynamicBypassRoutes) error {
	return ctx.Err()
}

type fakeDynamicRoutes struct {
	calls *[]string
}

func (r *fakeDynamicRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	*r.calls = append(*r.calls, "dynamic-route-add:"+prefix.String())
	return ctx.Err()
}

func (r *fakeDynamicRoutes) Close() error {
	*r.calls = append(*r.calls, "dynamic-routes-close")
	return nil
}

func fakeTunnelDeps(dev *fakeTunnelDevice, routes *fakeRouteManager, dns *fakeDNSManager) TunnelDeps {
	return TunnelDeps{
		DeviceFactory: &fakeTunnelDeviceFactory{dev: dev},
		RouteManager:  routes,
		DNSManager:    dns,
		Lookup: func(host string) ([]netip.Addr, error) {
			if host != "vpn.example.test" {
				return nil, errors.New("unexpected host")
			}
			return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
		},
		Wait: func(context.Context) error {
			return nil
		},
	}
}

func TestRunTunnelSetupThenCleanup(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	factory := &fakeTunnelDeviceFactory{dev: dev}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	deps.DeviceFactory = factory
	cfg := validTunnelConfig()
	cfg.Interface.MTU = -1

	err := RunTunnelWithDeps(context.Background(), cfg, TunnelOptions{ConfigPath: "amnezia.conf"}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	if factory.name != defaultTunnelName() {
		t.Fatalf("device name = %q, want %s", factory.name, defaultTunnelName())
	}
	if factory.mtu != 1420 {
		t.Fatalf("device MTU = %d, want 1420", factory.mtu)
	}
	if !dev.closed {
		t.Fatalf("device was not closed")
	}

	wantRouteCalls := []string{"configure:utun99:10.8.0.2/32", "routes:utun99", "cleanup-routes"}
	if !reflect.DeepEqual(routes.calls, wantRouteCalls) {
		t.Fatalf("route calls = %#v, want %#v", routes.calls, wantRouteCalls)
	}

	wantDNSCalls := []string{"dns", "cleanup-dns"}
	if !reflect.DeepEqual(dns.calls, wantDNSCalls) {
		t.Fatalf("DNS calls = %#v, want %#v", dns.calls, wantDNSCalls)
	}
}

func TestRunTunnelNoDNSSkipsDNSManager(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{NoDNS: true}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	if len(dns.calls) != 0 {
		t.Fatalf("DNS calls = %#v, want none", dns.calls)
	}
}

func TestRunTunnelRejectsDomainRulesWithNoDNSBeforeDeviceSetup(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	factory := &fakeTunnelDeviceFactory{dev: dev}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	deps.DeviceFactory = factory
	path := writeTempRules(t, `exclude_domain = *.delimobil.*`)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{RulesPath: path, NoDNS: true}, deps)
	if err == nil {
		t.Fatalf("RunTunnelWithDeps succeeded, want error")
	}
	if factory.called {
		t.Fatalf("device factory was called before domain/no-dns validation failed")
	}
}

func TestRunTunnelStartsDomainRuntimeBeforeApplyingDNS(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	var calls []string
	dns.orderedCalls = &calls
	runtime := &fakeDomainRuntime{calls: &calls}
	dynamicRoutes := &fakeDynamicRoutes{calls: &calls}
	deps.DomainRuntimeFactory = func() DomainBypassRuntime {
		return runtime
	}
	deps.DynamicRoutesFactory = func(DefaultRoute) DynamicBypassRoutes {
		return dynamicRoutes
	}
	path := writeTempRules(t, `exclude_domain = *.delimobil.*`)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{RulesPath: path}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	if !reflect.DeepEqual(dns.servers, []string{"127.0.0.1"}) {
		t.Fatalf("DNS servers = %v, want [127.0.0.1]", dns.servers)
	}
	wantCalls := []string{"domain-runtime-start", "dns", "cleanup-dns", "domain-runtime-close", "dynamic-routes-close"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("ordered calls = %v, want %v", calls, wantCalls)
	}
	if runtime.config.ListenAddr != "127.0.0.1:53" {
		t.Fatalf("ListenAddr = %q, want 127.0.0.1:53", runtime.config.ListenAddr)
	}
	if runtime.config.Upstream != "1.1.1.1:53" {
		t.Fatalf("Upstream = %q, want 1.1.1.1:53", runtime.config.Upstream)
	}
	if len(runtime.config.Rules.DomainRules) != 1 || runtime.config.Rules.DomainRules[0].Pattern != "*.delimobil.*" {
		t.Fatalf("DomainRules = %#v, want *.delimobil.*", runtime.config.Rules.DomainRules)
	}
	if runtime.config.Routes == nil {
		t.Fatalf("Routes is nil")
	}
	wrappedRoutes, ok := runtime.config.Routes.(staticAwareDynamicBypassRoutes)
	if !ok {
		t.Fatalf("Routes = %#v, want static-aware dynamic routes", runtime.config.Routes)
	}
	if wrappedRoutes.DynamicBypassRoutes != dynamicRoutes {
		t.Fatalf("wrapped Routes = %#v, want injected dynamic routes", wrappedRoutes.DynamicBypassRoutes)
	}
}

func TestRunTunnelDomainRoutesSkipStaticCoveredPrefix(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	var calls []string
	runtime := &fakeDomainRuntime{calls: &calls}
	runtime.onStart = func(ctx context.Context, config DomainBypassConfig) error {
		return config.Routes.AddBypassRoute(ctx, netip.MustParsePrefix("198.51.100.44/32"), "dns:api.delimobil.test", time.Minute)
	}
	dynamicRoutes := &fakeDynamicRoutes{calls: &calls}
	deps.DomainRuntimeFactory = func() DomainBypassRuntime {
		return runtime
	}
	deps.DynamicRoutesFactory = func(DefaultRoute) DynamicBypassRoutes {
		return dynamicRoutes
	}
	path := writeTempRules(t, `
exclude_ip = 198.51.100.44
exclude_domain = *.delimobil.*
`)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{RulesPath: path}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	if slices.Contains(calls, "dynamic-route-add:198.51.100.44/32") {
		t.Fatalf("dynamic route add was called for statically covered prefix: %v", calls)
	}
}

func TestRunTunnelLoadsRulesAndPassesStaticBypassToRoutes(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	path := writeTempRules(t, `exclude_ip = 198.51.100.44`)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{RulesPath: path}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	want := []netip.Prefix{netip.MustParsePrefix("198.51.100.44/32")}
	if !reflect.DeepEqual(routes.lastPlan.StaticBypassCIDRs, want) {
		t.Fatalf("StaticBypassCIDRs = %v, want %v", routes.lastPlan.StaticBypassCIDRs, want)
	}
}

func TestRunTunnelDryRunSkipsInjectedDeviceFactory(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	factory := &fakeTunnelDeviceFactory{dev: dev}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	deps.DeviceFactory = factory

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{DryRun: true}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	if factory.called {
		t.Fatalf("dry-run called the injected device factory")
	}
	if dev.upUAPI != "" {
		t.Fatalf("dry-run called Up on injected device")
	}
	if len(routes.calls) != 0 {
		t.Fatalf("dry-run called mutating route methods: %#v", routes.calls)
	}
	if routes.defaults != 1 {
		t.Fatalf("dry-run default route discovery calls = %d, want 1", routes.defaults)
	}
	if len(dns.calls) != 0 {
		t.Fatalf("dry-run called injected DNS manager: %#v", dns.calls)
	}
}

func TestRunTunnelDryRunExitsWithoutWaiting(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	waitErr := errors.New("wait called")
	deps.Wait = func(context.Context) error {
		return waitErr
	}

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{DryRun: true}, deps)
	if errors.Is(err, waitErr) {
		t.Fatalf("dry-run waited for signal: %v", err)
	}
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}
}

func TestRunTunnelDryRunDomainRulesDoNotUseInjectedRuntimeFactories(t *testing.T) {
	recorder := platform.NewDryRunRunner()
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	deps.DeviceFactory = dryRunTunnelDeviceFactory{Recorder: recorder}
	deps.RouteManager = dryRunRouteManager{
		RouteManager: routes,
		Recorder:     recorder,
		Fallback:     dryRunDefaultRouteFallback(),
	}
	deps.DNSManager = dryRunDNSManager{Recorder: recorder}
	deps.DomainRuntimeFactory = func() DomainBypassRuntime {
		panic("dry-run called injected domain runtime factory")
	}
	deps.DynamicRoutesFactory = func(DefaultRoute) DynamicBypassRoutes {
		panic("dry-run called injected dynamic route factory")
	}
	path := writeTempRules(t, `exclude_domain = *.delimobil.*`)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{DryRun: true, RulesPath: path}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	commands := recorder.Commands()
	if !slices.Contains(commands, "start domain bypass DNS runtime 127.0.0.1:53 upstream 1.1.1.1:53") {
		t.Fatalf("commands = %#v, want domain runtime dry-run intent", commands)
	}
	if !slices.Contains(commands, "set DNS servers 127.0.0.1") {
		t.Fatalf("commands = %#v, want DNS redirected to local runtime", commands)
	}
}

func TestRunTunnelDomainRuntimeUsesBracketedIPv6DNSUpstream(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	var calls []string
	runtime := &fakeDomainRuntime{calls: &calls}
	dynamicRoutes := &fakeDynamicRoutes{calls: &calls}
	deps.DomainRuntimeFactory = func() DomainBypassRuntime {
		return runtime
	}
	deps.DynamicRoutesFactory = func(DefaultRoute) DynamicBypassRoutes {
		return dynamicRoutes
	}
	cfg := validTunnelConfig()
	cfg.Interface.DNS = []string{"2001:4860:4860::8888"}
	path := writeTempRules(t, `exclude_domain = *.delimobil.*`)

	err := RunTunnelWithDeps(context.Background(), cfg, TunnelOptions{RulesPath: path}, deps)
	if err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}

	if runtime.config.Upstream != "[2001:4860:4860::8888]:53" {
		t.Fatalf("Upstream = %q, want [2001:4860:4860::8888]:53", runtime.config.Upstream)
	}
}

func TestDryRunRouteManagerFallsBackWhenDefaultRouteDiscoveryFails(t *testing.T) {
	discoveryErr := errors.New("route discovery failed")
	routes := &fakeRouteManager{defaultErr: discoveryErr}
	recorder := platform.NewDryRunRunner()
	fallback := DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.254"), Device: "default0"}
	manager := dryRunRouteManager{
		RouteManager: routes,
		Recorder:     recorder,
		Fallback:     fallback,
	}

	got, err := manager.DefaultRoute(context.Background())
	if err != nil {
		t.Fatalf("DefaultRoute returned error: %v", err)
	}
	if got != fallback {
		t.Fatalf("DefaultRoute = %#v, want %#v", got, fallback)
	}

	want := []string{"default route discovery failed: route discovery failed; using dry-run placeholder gateway 192.0.2.254 dev default0"}
	if !reflect.DeepEqual(recorder.Commands(), want) {
		t.Fatalf("recorded commands = %#v, want %#v", recorder.Commands(), want)
	}
}

func TestDryRunRouteManagerRecordsStaticBypass(t *testing.T) {
	recorder := platform.NewDryRunRunner()
	manager := dryRunRouteManager{
		Recorder: recorder,
		Fallback: dryRunDefaultRouteFallback(),
	}
	cleanup := NewCleanupStack()
	plan := RoutePlan{
		TunnelCIDRs: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/1")},
		StaticBypassCIDRs: []netip.Prefix{
			netip.MustParsePrefix("198.51.100.0/24"),
		},
		EndpointBypass: netip.MustParseAddrPort("203.0.113.10:51820"),
	}
	defaultRoute := dryRunDefaultRouteFallback()

	if err := manager.Apply(context.Background(), "utun9", plan, defaultRoute, cleanup); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	wantContains := "add static bypass route 198.51.100.0/24 via 192.0.2.254 dev default0"
	if !slices.Contains(recorder.Commands(), wantContains) {
		t.Fatalf("commands = %#v, want contains %q", recorder.Commands(), wantContains)
	}
}

func TestRunTunnelRouteApplyFailureRunsCleanup(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routeErr := errors.New("route apply failed")
	routes := &fakeRouteManager{applyErr: routeErr}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{}, deps)
	if !errors.Is(err, routeErr) {
		t.Fatalf("RunTunnelWithDeps error = %v, want %v", err, routeErr)
	}

	if !dev.closed {
		t.Fatalf("device was not closed")
	}
	wantRouteCalls := []string{"configure:utun99:10.8.0.2/32", "routes:utun99", "cleanup-routes"}
	if !reflect.DeepEqual(routes.calls, wantRouteCalls) {
		t.Fatalf("route calls = %#v, want %#v", routes.calls, wantRouteCalls)
	}
	if len(dns.calls) != 0 {
		t.Fatalf("DNS calls = %#v, want none", dns.calls)
	}
}

func TestRunTunnelReturnsCleanupError(t *testing.T) {
	closeErr := errors.New("close failed")
	dev := &fakeTunnelDevice{name: "utun99", closeErr: closeErr}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)

	err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), TunnelOptions{}, deps)
	if !errors.Is(err, closeErr) {
		t.Fatalf("RunTunnelWithDeps error = %v, want cleanup error %v", err, closeErr)
	}
}

func TestRunTunnelRejectsEmptyDNSBeforeDeviceCreation(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	factory := &fakeTunnelDeviceFactory{dev: dev}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	deps := fakeTunnelDeps(dev, routes, dns)
	deps.DeviceFactory = factory
	cfg := validTunnelConfig()
	cfg.Interface.DNS = nil

	err := RunTunnelWithDeps(context.Background(), cfg, TunnelOptions{}, deps)
	if err == nil {
		t.Fatalf("RunTunnelWithDeps succeeded, want error")
	}
	if !strings.Contains(err.Error(), "tunnel DNS is empty") {
		t.Fatalf("RunTunnelWithDeps error = %v, want empty DNS error", err)
	}
	if factory.called {
		t.Fatalf("device factory called before DNS validation completed")
	}
	if len(routes.calls) != 0 {
		t.Fatalf("route calls = %#v, want none", routes.calls)
	}
	if len(dns.calls) != 0 {
		t.Fatalf("DNS calls = %#v, want none", dns.calls)
	}
}

func TestPrintUsageListsTunnelOptions(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	printUsage()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}

	text := string(out)
	for _, flag := range []string{"--rules", "--dry-run", "--no-dns", "--verbose"} {
		if !strings.Contains(text, flag) {
			t.Fatalf("usage output does not mention %s:\n%s", flag, text)
		}
	}
}
