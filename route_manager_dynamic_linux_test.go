//go:build linux

package main

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"sync"
	"testing"
	"time"
)

type recordingRunner struct {
	mu       sync.Mutex
	commands []string
	err      error
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, commandString(name, args...))
	return r.err
}

func (r *recordingRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func TestLinuxDynamicBypassRouteAddDelete(t *testing.T) {
	runner := &recordingRunner{}
	manager := LinuxDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"},
	}

	if err := manager.AddBypassRoute(context.Background(), netip.MustParsePrefix("198.51.100.44/32"), "dns:git.delimobil.ru", time.Minute); err != nil {
		t.Fatalf("AddBypassRoute returned error: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{
		"ip route add 198.51.100.44/32 via 192.0.2.1 dev eth0",
		"ip route del 198.51.100.44/32",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestLinuxDynamicBypassRouteFailedAddCloseDoesNotDelete(t *testing.T) {
	runner := &recordingRunner{err: errors.New("ip route add failed")}
	manager := LinuxDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"},
	}

	err := manager.AddBypassRoute(context.Background(), netip.MustParsePrefix("198.51.100.44/32"), "dns:git.delimobil.ru", time.Minute)
	if err == nil {
		t.Fatal("AddBypassRoute returned nil, want error")
	}
	runner.err = nil
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{"ip route add 198.51.100.44/32 via 192.0.2.1 dev eth0"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestLinuxDynamicBypassRouteFailedAddCanRetry(t *testing.T) {
	runner := &recordingRunner{err: errors.New("ip route add failed")}
	manager := LinuxDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"},
	}

	prefix := netip.MustParsePrefix("198.51.100.44/32")
	if err := manager.AddBypassRoute(context.Background(), prefix, "dns:git.delimobil.ru", time.Minute); err == nil {
		t.Fatal("AddBypassRoute returned nil, want error")
	}
	runner.err = nil
	if err := manager.AddBypassRoute(context.Background(), prefix, "dns:git.delimobil.ru", time.Minute); err != nil {
		t.Fatalf("AddBypassRoute retry returned error: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{
		"ip route add 198.51.100.44/32 via 192.0.2.1 dev eth0",
		"ip route add 198.51.100.44/32 via 192.0.2.1 dev eth0",
		"ip route del 198.51.100.44/32",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestLinuxDynamicBypassRouteSuppressesDuplicateSuccessfulAdds(t *testing.T) {
	runner := &recordingRunner{}
	manager := LinuxDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"},
	}

	prefix := netip.MustParsePrefix("198.51.100.44/32")
	if err := manager.AddBypassRoute(context.Background(), prefix, "dns:git.delimobil.ru", time.Minute); err != nil {
		t.Fatalf("AddBypassRoute returned error: %v", err)
	}
	if err := manager.AddBypassRoute(context.Background(), prefix, "dns:git.delimobil.ru", time.Minute); err != nil {
		t.Fatalf("duplicate AddBypassRoute returned error: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{
		"ip route add 198.51.100.44/32 via 192.0.2.1 dev eth0",
		"ip route del 198.51.100.44/32",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestLinuxDynamicBypassRouteSuppressesConcurrentDuplicateAdds(t *testing.T) {
	runner := &recordingRunner{}
	manager := LinuxDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "eth0"},
	}

	prefix := netip.MustParsePrefix("198.51.100.44/32")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := manager.AddBypassRoute(context.Background(), prefix, "dns:git.delimobil.ru", time.Minute); err != nil {
				t.Errorf("AddBypassRoute returned error: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{
		"ip route add 198.51.100.44/32 via 192.0.2.1 dev eth0",
		"ip route del 198.51.100.44/32",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}
