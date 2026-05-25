package main

import (
	"net/netip"
	"strings"
	"testing"
)

func TestBuildResolvedTunnelUAPI(t *testing.T) {
	cfg := validTunnelConfig()
	resolved := netip.MustParseAddrPort("203.0.113.10:51820")

	uapi, err := BuildResolvedTunnelUAPI(cfg, resolved)
	if err != nil {
		t.Fatalf("BuildResolvedTunnelUAPI returned error: %v", err)
	}

	if !strings.Contains(uapi, "endpoint=203.0.113.10:51820") {
		t.Fatalf("UAPI = %q, want resolved endpoint", uapi)
	}
	if strings.Contains(uapi, "endpoint=vpn.example.test:51820") {
		t.Fatalf("UAPI = %q, contains unresolved endpoint", uapi)
	}
}
