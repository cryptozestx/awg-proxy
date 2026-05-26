package main

import (
	"fmt"
	"net/netip"
	"strings"
)

// Helpers to parse netip addresses
func parseAddresses(addrs []string) ([]netip.Addr, error) {
	var result []netip.Addr
	for _, a := range addrs {
		// Strip CIDR mask if present
		ipStr := a
		if idx := strings.Index(a, "/"); idx >= 0 {
			ipStr = a[:idx]
		}
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			return nil, fmt.Errorf("invalid IP address: %s", a)
		}
		result = append(result, ip)
	}
	return result, nil
}
