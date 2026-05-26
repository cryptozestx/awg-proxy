//go:build darwin

package main

import "testing"

func TestDefaultTunnelNameDarwin(t *testing.T) {
	if got := defaultTunnelName(); got != "utun" {
		t.Fatalf("defaultTunnelName() = %q, want utun", got)
	}
}
