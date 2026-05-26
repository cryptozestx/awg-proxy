//go:build linux

package main

import (
	"context"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

type recordingRunner struct {
	commands []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, commandString(name, args...))
	return nil
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
