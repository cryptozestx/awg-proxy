//go:build linux

package main

import "awg-proxy/internal/platform"

func NewPlatformDNSManager(runner platform.CommandRunner) DNSManager {
	return LinuxDNSManager{}
}

func defaultTunnelName() string {
	return "awgproxy0"
}
