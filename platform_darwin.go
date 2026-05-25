//go:build darwin

package main

func NewPlatformRouteManager(runner CommandRunner) RouteManager {
	return DarwinRouteManager{Runner: runner}
}

func NewPlatformDNSManager(runner CommandRunner) DNSManager {
	return DarwinDNSManager{Runner: runner}
}
