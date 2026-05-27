package routing

import "net/netip"

type Plan struct {
	TunnelCIDRs       []netip.Prefix
	StaticBypassCIDRs []netip.Prefix
	EndpointBypass    netip.AddrPort
}

type StaticBypassSource interface {
	StaticBypassCIDRs() []netip.Prefix
}

func BuildFullTunnelPlan(endpoint netip.AddrPort) Plan {
	return BuildTunnelPlan(endpoint, nil)
}

func BuildTunnelPlan(endpoint netip.AddrPort, source StaticBypassSource) Plan {
	var staticBypassCIDRs []netip.Prefix
	if source != nil {
		staticBypassCIDRs = source.StaticBypassCIDRs()
	}
	return Plan{
		TunnelCIDRs: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
		StaticBypassCIDRs: append([]netip.Prefix(nil), staticBypassCIDRs...),
		EndpointBypass:    endpoint,
	}
}
