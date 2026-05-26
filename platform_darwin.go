//go:build darwin

package main

import "awg-proxy/internal/platform"

func NewPlatformRouteManager(runner platform.CommandRunner) RouteManager {
	return DarwinRouteManager{Runner: runner}
}

func NewPlatformDNSManager(runner platform.CommandRunner) DNSManager {
	return DarwinDNSManager{Runner: runner}
}

func defaultTunnelName() string {
	return "utun"
}
