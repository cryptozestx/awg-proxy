package routing

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestBuildFullTunnelPlan(t *testing.T) {
	endpoint := netip.MustParseAddrPort("203.0.113.10:51820")

	plan := BuildFullTunnelPlan(endpoint)

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

func TestBuildTunnelPlanWithStaticBypass(t *testing.T) {
	endpoint := netip.MustParseAddrPort("203.0.113.10:51820")
	rules := staticBypassRules{
		cidrs: []netip.Prefix{
			netip.MustParsePrefix("198.51.100.0/24"),
			netip.MustParsePrefix("203.0.113.20/32"),
		},
	}

	plan := BuildTunnelPlan(endpoint, rules)

	if plan.EndpointBypass != endpoint {
		t.Fatalf("EndpointBypass = %v, want %v", plan.EndpointBypass, endpoint)
	}
	if !reflect.DeepEqual(plan.StaticBypassCIDRs, rules.cidrs) {
		t.Fatalf("StaticBypassCIDRs = %v, want %v", plan.StaticBypassCIDRs, rules.cidrs)
	}
}

type staticBypassRules struct {
	cidrs []netip.Prefix
}

func (r staticBypassRules) StaticBypassCIDRs() []netip.Prefix {
	return append([]netip.Prefix(nil), r.cidrs...)
}
