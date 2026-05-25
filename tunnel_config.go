package main

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

type TunnelConfig struct {
	InterfaceIPv4 netip.Prefix
	PeerIndex     int
	EndpointHost  string
	EndpointPort  uint16
}

func ValidateTunnelConfig(cfg *AWGConfig) (TunnelConfig, error) {
	if cfg == nil {
		return TunnelConfig{}, fmt.Errorf("tunnel config is nil")
	}

	peerIndex := -1
	endpoint := ""
	for i, peer := range cfg.Peers {
		if peer.Endpoint == "" {
			continue
		}
		if peerIndex != -1 {
			return TunnelConfig{}, fmt.Errorf("expected exactly one peer with Endpoint")
		}
		peerIndex = i
		endpoint = peer.Endpoint
	}
	if peerIndex == -1 {
		return TunnelConfig{}, fmt.Errorf("expected exactly one peer with Endpoint")
	}

	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return TunnelConfig{}, fmt.Errorf("invalid peer Endpoint: %w", err)
	}
	if host == "" {
		return TunnelConfig{}, fmt.Errorf("invalid peer Endpoint: host is empty")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return TunnelConfig{}, fmt.Errorf("invalid peer Endpoint port %q", portText)
	}

	for _, address := range cfg.Interface.Address {
		prefix, err := netip.ParsePrefix(address)
		if err != nil {
			continue
		}
		if prefix.Addr().Is4() {
			return TunnelConfig{
				InterfaceIPv4: prefix,
				PeerIndex:     peerIndex,
				EndpointHost:  host,
				EndpointPort:  uint16(port),
			}, nil
		}
	}

	return TunnelConfig{}, fmt.Errorf("interface Address must include an IPv4 CIDR")
}

func ResolveEndpointIPv4(host string, port uint16, lookup func(string) ([]netip.Addr, error)) (netip.AddrPort, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !addr.Is4() {
			return netip.AddrPort{}, fmt.Errorf("endpoint host %q is not IPv4", host)
		}
		return netip.AddrPortFrom(addr, port), nil
	}

	addrs, err := lookup(host)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("lookup endpoint host %q: %w", host, err)
	}
	for _, addr := range addrs {
		if addr.Is4() {
			return netip.AddrPortFrom(addr, port), nil
		}
	}

	return netip.AddrPort{}, fmt.Errorf("endpoint host %q is not IPv4", host)
}

func CloneConfigWithResolvedEndpoint(cfg *AWGConfig, endpoint netip.AddrPort) *AWGConfig {
	if cfg == nil {
		return nil
	}

	clone := *cfg
	clone.Interface = cfg.Interface
	clone.Interface.Address = append([]string(nil), cfg.Interface.Address...)
	clone.Interface.DNS = append([]string(nil), cfg.Interface.DNS...)
	clone.Peers = append([]PeerConfig(nil), cfg.Peers...)

	for i := range clone.Peers {
		clone.Peers[i].AllowedIPs = append([]string(nil), cfg.Peers[i].AllowedIPs...)
		if clone.Peers[i].Endpoint != "" {
			clone.Peers[i].Endpoint = endpoint.String()
		}
	}

	return &clone
}
