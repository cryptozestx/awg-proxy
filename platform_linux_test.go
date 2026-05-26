//go:build linux

package main

import "testing"

func TestDefaultTunnelNameLinux(t *testing.T) {
	if got := defaultTunnelName(); got != "awgproxy0" {
		t.Fatalf("defaultTunnelName() = %q, want awgproxy0", got)
	}
}
