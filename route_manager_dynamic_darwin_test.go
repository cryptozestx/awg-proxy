//go:build darwin

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

func TestDarwinDynamicBypassRouteAddDelete(t *testing.T) {
	runner := &recordingRunner{}
	manager := DarwinDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"},
	}

	if err := manager.AddBypassRoute(context.Background(), netip.MustParsePrefix("198.51.100.44/32"), "dns:git.delimobil.ru", time.Minute); err != nil {
		t.Fatalf("AddBypassRoute returned error: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{
		"route add 198.51.100.44 192.0.2.1",
		"route delete 198.51.100.44",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestDarwinDynamicBypassRouteFailedAddCloseDoesNotDelete(t *testing.T) {
	runner := &recordingRunner{err: errors.New("route add failed")}
	manager := DarwinDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"},
	}

	err := manager.AddBypassRoute(context.Background(), netip.MustParsePrefix("198.51.100.44/32"), "dns:git.delimobil.ru", time.Minute)
	if err == nil {
		t.Fatal("AddBypassRoute returned nil, want error")
	}
	runner.err = nil
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{"route add 198.51.100.44 192.0.2.1"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestDarwinDynamicBypassRouteFailedAddCanRetry(t *testing.T) {
	runner := &recordingRunner{err: errors.New("route add failed")}
	manager := DarwinDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"},
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
		"route add 198.51.100.44 192.0.2.1",
		"route add 198.51.100.44 192.0.2.1",
		"route delete 198.51.100.44",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestDarwinDynamicBypassRouteSuppressesDuplicateSuccessfulAdds(t *testing.T) {
	runner := &recordingRunner{}
	manager := DarwinDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"},
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
		"route add 198.51.100.44 192.0.2.1",
		"route delete 198.51.100.44",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestDarwinDynamicBypassRouteSuppressesConcurrentDuplicateAdds(t *testing.T) {
	runner := &recordingRunner{}
	manager := DarwinDynamicBypassRoutes{
		Runner:       runner,
		DefaultRoute: DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"},
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
		"route add 198.51.100.44 192.0.2.1",
		"route delete 198.51.100.44",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}
