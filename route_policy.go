package main

import "net/netip"

type RoutePlan struct {
	TunnelCIDRs       []netip.Prefix
	StaticBypassCIDRs []netip.Prefix
	EndpointBypass    netip.AddrPort
}

func BuildFullTunnelRoutePlan(endpoint netip.AddrPort) RoutePlan {
	return BuildTunnelRoutePlan(endpoint, TunnelRules{})
}

func BuildTunnelRoutePlan(endpoint netip.AddrPort, rules TunnelRules) RoutePlan {
	return RoutePlan{
		TunnelCIDRs: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
		StaticBypassCIDRs: append([]netip.Prefix(nil), rules.StaticBypassCIDRs...),
		EndpointBypass:    endpoint,
	}
}
