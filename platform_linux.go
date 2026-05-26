//go:build linux

package main

import "awg-proxy/internal/platform"

func NewPlatformRouteManager(runner platform.CommandRunner) RouteManager {
	return LinuxRouteManager{Runner: runner}
}

func NewPlatformDNSManager(runner platform.CommandRunner) DNSManager {
	return LinuxDNSManager{}
}

func defaultTunnelName() string {
	return "awgproxy0"
}
