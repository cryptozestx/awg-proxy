//go:build linux

package main

func NewPlatformRouteManager(runner CommandRunner) RouteManager {
	return LinuxRouteManager{Runner: runner}
}

func NewPlatformDNSManager(runner CommandRunner) DNSManager {
	return LinuxDNSManager{}
}
