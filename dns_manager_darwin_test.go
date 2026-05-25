package main

import (
	"reflect"
	"testing"
)

func TestParseDarwinNetworkServicesSkipsDisabledServices(t *testing.T) {
	out := "An asterisk (*) denotes that a network service is disabled.\nWi-Fi\n*Thunderbolt Bridge\nUSB LAN\n"

	got := parseDarwinNetworkServices(out)
	want := []string{"Wi-Fi", "USB LAN"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDarwinNetworkServices() = %v, want %v", got, want)
	}
}

func TestParseDarwinDNSStateEmpty(t *testing.T) {
	got := parseDarwinDNSState("Wi-Fi", "There aren't any DNS Servers set on Wi-Fi.\n")

	if got.Service != "Wi-Fi" {
		t.Fatalf("Service = %q, want Wi-Fi", got.Service)
	}
	if !got.Empty {
		t.Fatal("Empty = false, want true")
	}
	if len(got.Servers) != 0 {
		t.Fatalf("Servers = %v, want empty", got.Servers)
	}
}

func TestParseDarwinDNSStateServers(t *testing.T) {
	got := parseDarwinDNSState("Wi-Fi", "1.1.1.1\n8.8.8.8\n")
	want := darwinDNSState{Service: "Wi-Fi", Servers: []string{"1.1.1.1", "8.8.8.8"}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDarwinDNSState() = %#v, want %#v", got, want)
	}
}
