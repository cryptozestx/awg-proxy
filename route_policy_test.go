package main

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestBuildFullTunnelRoutePlan(t *testing.T) {
	endpoint := netip.MustParseAddrPort("203.0.113.10:51820")

	plan := BuildFullTunnelRoutePlan(endpoint)

	wantCIDRs := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/1"),
		netip.MustParsePrefix("128.0.0.0/1"),
	}
	if !reflect.DeepEqual(plan.TunnelCIDRs, wantCIDRs) {
		t.Fatalf("TunnelCIDRs = %v, want %v", plan.TunnelCIDRs, wantCIDRs)
	}
	if plan.EndpointBypass != endpoint {
		t.Fatalf("EndpointBypass = %v, want %v", plan.EndpointBypass, endpoint)
	}
}
