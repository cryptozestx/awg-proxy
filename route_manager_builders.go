package main

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

func darwinConfigureAddressCommand(ifName string, addr netip.Prefix, mtu int) []string {
	ip := addr.Addr().String()
	args := []string{"ifconfig", ifName, "inet", ip}
	if addr.Bits() == 32 {
		args = append(args, ip)
	} else {
		args = append(args, "netmask", ipv4Netmask(addr.Bits()))
	}
	args = append(args, "mtu", strconv.Itoa(mtu), "up")
	return args
}

func linuxConfigureAddressCommands(ifName string, addr netip.Prefix, mtu int) [][]string {
	return [][]string{
		{"ip", "addr", "add", addr.String(), "dev", ifName},
		{"ip", "link", "set", "dev", ifName, "mtu", strconv.Itoa(mtu), "up"},
	}
}

func ipv4Netmask(bits int) string {
	mask := net.CIDRMask(bits, 32)
	if mask == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}
