package main

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestDarwinTunAddressCommands32(t *testing.T) {
	addr := netip.MustParsePrefix("10.8.0.2/32")

	got := darwinConfigureAddressCommand("utun7", addr, 1420)

	want := []string{"ifconfig", "utun7", "inet", "10.8.0.2", "10.8.0.2", "mtu", "1420", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %v, want %v", got, want)
	}
}

func TestDarwinTunAddressCommands24(t *testing.T) {
	addr := netip.MustParsePrefix("10.8.0.2/24")

	got := darwinConfigureAddressCommand("utun7", addr, 1420)

	want := []string{"ifconfig", "utun7", "inet", "10.8.0.2", "netmask", "255.255.255.0", "mtu", "1420", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %v, want %v", got, want)
	}
}

func TestLinuxTunAddressCommands(t *testing.T) {
	addr := netip.MustParsePrefix("10.8.0.2/32")

	got := linuxConfigureAddressCommands("tun0", addr, 1420)

	want := [][]string{
		{"ip", "addr", "add", "10.8.0.2/32", "dev", "tun0"},
		{"ip", "link", "set", "dev", "tun0", "mtu", "1420", "up"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
}
