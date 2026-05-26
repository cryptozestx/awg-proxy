//go:build darwin

package main

import "awg-proxy/internal/platform"

func NewPlatformDNSManager(runner platform.CommandRunner) DNSManager {
	return DarwinDNSManager{Runner: runner}
}

func defaultTunnelName() string {
	return "utun"
}
