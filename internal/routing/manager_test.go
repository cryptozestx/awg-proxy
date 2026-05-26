package routing

import (
	"awg-proxy/internal/platform"
	"context"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

type fakeRouteRunner struct {
	outputs    map[string][]byte
	runErrAt   int
	runCount   int
	runRecords []string
}

type testCleanupStack struct {
	actions []func() error
}

func newTestCleanupStack() *testCleanupStack {
	return &testCleanupStack{}
}

func (s *testCleanupStack) Add(_ string, fn func() error) {
	s.actions = append(s.actions, fn)
}

func (s *testCleanupStack) Run() error {
	for i := len(s.actions) - 1; i >= 0; i-- {
		if err := s.actions[i](); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeRouteRunner) Run(_ context.Context, name string, args ...string) error {
	r.runCount++
	record := platform.CommandString(name, args...)
	r.runRecords = append(r.runRecords, record)
	if r.runErrAt == r.runCount {
		return errors.New("runner failed")
	}
	return nil
}

func (r *fakeRouteRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if out, ok := r.outputs[platform.CommandString(name, args...)]; ok {
		return out, nil
	}
	return nil, errors.New("missing output")
}

func TestDarwinTunAddressCommands32(t *testing.T) {
	addr := netip.MustParsePrefix("10.8.0.2/32")

	got := darwinConfigureAddressCommand("utun7", addr, 1420)

	want := []string{"ifconfig", "utun7", "inet", "10.8.0.2", "10.8.0.2", "mtu", "1420", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %v, want %v", got, want)
	}
}

func TestDarwinTunAddressCommands24(t *testing.T) {
	addr := netip.MustParsePrefix("10.8.0.2/24")

	got := darwinConfigureAddressCommand("utun7", addr, 1420)

	want := []string{"ifconfig", "utun7", "inet", "10.8.0.2", "netmask", "255.255.255.0", "mtu", "1420", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %v, want %v", got, want)
	}
}

func TestLinuxTunAddressCommands(t *testing.T) {
	addr := netip.MustParsePrefix("10.8.0.2/32")

	got := linuxConfigureAddressCommands("tun0", addr, 1420)

	want := [][]string{
		{"ip", "addr", "add", "10.8.0.2/32", "dev", "tun0"},
		{"ip", "link", "set", "dev", "tun0", "mtu", "1420", "up"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
}

func TestParseLinuxDefaultRouteChoosesFirstValidDefault(t *testing.T) {
	out := strings.Join([]string{
		"default dev eth-missing-gateway proto dhcp",
		"default via 192.0.2.1 dev eth0 proto dhcp metric 100",
		"default via 198.51.100.1 dev eth1 proto dhcp metric 200",
		"203.0.113.0/24 dev eth2 proto kernel",
	}, "\n")

	got, err := parseLinuxDefaultRoute(out)
	if err != nil {
		t.Fatalf("parseLinuxDefaultRoute returned error: %v", err)
	}

	want := DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"}
	if got != want {
		t.Fatalf("route = %+v, want %+v", got, want)
	}
}

func TestParseDarwinDefaultRoute(t *testing.T) {
	out := strings.Join([]string{
		"   route to: default",
		"destination: default",
		"       mask: default",
		"    gateway: 192.0.2.1",
		"  interface: en0",
	}, "\n")

	got, err := parseDarwinDefaultRoute(out)
	if err != nil {
		t.Fatalf("parseDarwinDefaultRoute returned error: %v", err)
	}

	want := DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"}
	if got != want {
		t.Fatalf("route = %+v, want %+v", got, want)
	}
}

func TestDarwinApplyCommandSequenceAndCleanup(t *testing.T) {
	runner := &fakeRouteRunner{}
	cleanup := newTestCleanupStack()
	plan := routeManagerTestPlan()

	if err := darwinApplyRoutes(context.Background(), runner, "utun7", plan, routeManagerDefaultRoute(), cleanup); err != nil {
		t.Fatalf("darwinApplyRoutes returned error: %v", err)
	}

	wantAdds := []string{
		"route add 203.0.113.10 192.0.2.1",
		"route add 0.0.0.0/1 -interface utun7",
		"route add 128.0.0.0/1 -interface utun7",
	}
	if !reflect.DeepEqual(runner.runRecords, wantAdds) {
		t.Fatalf("run records = %v, want %v", runner.runRecords, wantAdds)
	}

	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	wantAll := append(wantAdds,
		"route delete 128.0.0.0/1",
		"route delete 0.0.0.0/1",
		"route delete 203.0.113.10",
	)
	if !reflect.DeepEqual(runner.runRecords, wantAll) {
		t.Fatalf("run records after cleanup = %v, want %v", runner.runRecords, wantAll)
	}
}

func TestLinuxApplyCommandSequenceAndCleanup(t *testing.T) {
	runner := &fakeRouteRunner{}
	cleanup := newTestCleanupStack()
	plan := routeManagerTestPlan()

	if err := linuxApplyRoutes(context.Background(), runner, "tun0", plan, routeManagerDefaultRoute(), cleanup); err != nil {
		t.Fatalf("linuxApplyRoutes returned error: %v", err)
	}

	wantAdds := []string{
		"ip route add 203.0.113.10 via 192.0.2.1 dev en0",
		"ip route add 0.0.0.0/1 dev tun0",
		"ip route add 128.0.0.0/1 dev tun0",
	}
	if !reflect.DeepEqual(runner.runRecords, wantAdds) {
		t.Fatalf("run records = %v, want %v", runner.runRecords, wantAdds)
	}

	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	wantAll := append(wantAdds,
		"ip route del 128.0.0.0/1 dev tun0",
		"ip route del 0.0.0.0/1 dev tun0",
		"ip route del 203.0.113.10",
	)
	if !reflect.DeepEqual(runner.runRecords, wantAll) {
		t.Fatalf("run records after cleanup = %v, want %v", runner.runRecords, wantAll)
	}
}

func TestDarwinApplyRoutesIncludesStaticBypass(t *testing.T) {
	runner := &fakeRouteRunner{}
	cleanup := newTestCleanupStack()
	plan := Plan{
		TunnelCIDRs: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/1")},
		StaticBypassCIDRs: []netip.Prefix{
			netip.MustParsePrefix("198.51.100.0/24"),
			netip.MustParsePrefix("203.0.113.20/32"),
		},
		EndpointBypass: netip.MustParseAddrPort("203.0.113.10:51820"),
	}
	defaultRoute := DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"}

	if err := darwinApplyRoutes(context.Background(), runner, "utun9", plan, defaultRoute, cleanup); err != nil {
		t.Fatalf("darwinApplyRoutes returned error: %v", err)
	}

	want := []string{
		"route add 203.0.113.10 192.0.2.1",
		"route add 198.51.100.0/24 192.0.2.1",
		"route add 203.0.113.20 192.0.2.1",
		"route add 0.0.0.0/1 -interface utun9",
	}
	if !reflect.DeepEqual(runner.runRecords, want) {
		t.Fatalf("commands = %#v, want %#v", runner.runRecords, want)
	}
}

func TestLinuxApplyRoutesIncludesStaticBypass(t *testing.T) {
	runner := &fakeRouteRunner{}
	cleanup := newTestCleanupStack()
	plan := Plan{
		TunnelCIDRs: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/1")},
		StaticBypassCIDRs: []netip.Prefix{
			netip.MustParsePrefix("198.51.100.0/24"),
			netip.MustParsePrefix("203.0.113.20/32"),
		},
		EndpointBypass: netip.MustParseAddrPort("203.0.113.10:51820"),
	}
	defaultRoute := DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"}

	if err := linuxApplyRoutes(context.Background(), runner, "tun0", plan, defaultRoute, cleanup); err != nil {
		t.Fatalf("linuxApplyRoutes returned error: %v", err)
	}

	want := []string{
		"ip route add 203.0.113.10 via 192.0.2.1 dev eth0",
		"ip route add 198.51.100.0/24 via 192.0.2.1 dev eth0",
		"ip route add 203.0.113.20/32 via 192.0.2.1 dev eth0",
		"ip route add 0.0.0.0/1 dev tun0",
	}
	if !reflect.DeepEqual(runner.runRecords, want) {
		t.Fatalf("commands = %#v, want %#v", runner.runRecords, want)
	}
}

func TestDarwinApplyFailureCleansOnlySuccessfulAdds(t *testing.T) {
	runner := &fakeRouteRunner{runErrAt: 2}
	cleanup := newTestCleanupStack()

	err := darwinApplyRoutes(context.Background(), runner, "utun7", routeManagerTestPlan(), routeManagerDefaultRoute(), cleanup)
	if err == nil {
		t.Fatal("darwinApplyRoutes returned nil, want error")
	}
	if !strings.Contains(err.Error(), "add tunnel route 0.0.0.0/1") {
		t.Fatalf("error = %v, want tunnel route context", err)
	}

	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	want := []string{
		"route add 203.0.113.10 192.0.2.1",
		"route add 0.0.0.0/1 -interface utun7",
		"route delete 203.0.113.10",
	}
	if !reflect.DeepEqual(runner.runRecords, want) {
		t.Fatalf("run records = %v, want %v", runner.runRecords, want)
	}
}

func routeManagerTestPlan() Plan {
	return BuildFullTunnelPlan(netip.MustParseAddrPort("203.0.113.10:51820"))
}

func routeManagerDefaultRoute() DefaultRoute {
	return DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"}
}
