package main

import "net/netip"

type RoutePlan struct {
	TunnelCIDRs    []netip.Prefix
	EndpointBypass netip.AddrPort
}

func BuildFullTunnelRoutePlan(endpoint netip.AddrPort) RoutePlan {
	return RoutePlan{
		TunnelCIDRs: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
		EndpointBypass: endpoint,
	}
}
