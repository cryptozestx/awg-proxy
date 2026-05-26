package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
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

func routeTarget(prefix netip.Prefix) string {
	if prefix.Bits() == 32 {
		return prefix.Addr().String()
	}
	return prefix.String()
}

func parseDarwinDefaultRoute(out string) (DefaultRoute, error) {
	var route DefaultRoute
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "gateway":
			gateway, err := netip.ParseAddr(value)
			if err != nil {
				return DefaultRoute{}, fmt.Errorf("parse default route gateway %q: %w", value, err)
			}
			route.Gateway = gateway
		case "interface":
			route.Device = value
		}
	}
	if !route.Gateway.IsValid() {
		return DefaultRoute{}, fmt.Errorf("default route gateway missing")
	}
	if route.Device == "" {
		return DefaultRoute{}, fmt.Errorf("default route interface missing")
	}
	return route, nil
}

func parseLinuxDefaultRoute(out string) (DefaultRoute, error) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "default" {
			continue
		}

		var route DefaultRoute
		for i := 1; i+1 < len(fields); i++ {
			switch fields[i] {
			case "via":
				gateway, err := netip.ParseAddr(fields[i+1])
				if err != nil {
					return DefaultRoute{}, fmt.Errorf("parse default route gateway %q: %w", fields[i+1], err)
				}
				route.Gateway = gateway
			case "dev":
				route.Device = fields[i+1]
			}
		}
		if route.Gateway.IsValid() && route.Device != "" {
			return route, nil
		}
	}

	return DefaultRoute{}, fmt.Errorf("default route with gateway and device missing")
}

func darwinApplyRoutes(ctx context.Context, runner CommandRunner, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	endpointIP := plan.EndpointBypass.Addr().String()
	if err := runner.Run(ctx, "route", "add", endpointIP, defaultRoute.Gateway.String()); err != nil {
		return fmt.Errorf("add endpoint bypass route %s via %s: %w", endpointIP, defaultRoute.Gateway, err)
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		if err := runner.Run(context.Background(), "route", "delete", endpointIP); err != nil {
			return fmt.Errorf("delete endpoint bypass route %s: %w", endpointIP, err)
		}
		return nil
	})

	for _, cidr := range plan.StaticBypassCIDRs {
		target := routeTarget(cidr)
		if err := runner.Run(ctx, "route", "add", target, defaultRoute.Gateway.String()); err != nil {
			return fmt.Errorf("add static bypass route %s via %s: %w", target, defaultRoute.Gateway, err)
		}
		cleanup.Add("delete static bypass route "+target, func() error {
			if err := runner.Run(context.Background(), "route", "delete", target); err != nil {
				return fmt.Errorf("delete static bypass route %s: %w", target, err)
			}
			return nil
		})
	}

	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if err := runner.Run(ctx, "route", "add", cidrText, "-interface", ifName); err != nil {
			return fmt.Errorf("add tunnel route %s via interface %s: %w", cidrText, ifName, err)
		}
		cleanup.Add("delete tunnel route "+cidrText, func() error {
			if err := runner.Run(context.Background(), "route", "delete", cidrText); err != nil {
				return fmt.Errorf("delete tunnel route %s: %w", cidrText, err)
			}
			return nil
		})
	}

	return nil
}

func linuxApplyRoutes(ctx context.Context, runner CommandRunner, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	endpointIP := plan.EndpointBypass.Addr().String()
	if err := runner.Run(ctx, "ip", "route", "add", endpointIP, "via", defaultRoute.Gateway.String(), "dev", defaultRoute.Device); err != nil {
		return fmt.Errorf("add endpoint bypass route %s via %s dev %s: %w", endpointIP, defaultRoute.Gateway, defaultRoute.Device, err)
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		if err := runner.Run(context.Background(), "ip", "route", "del", endpointIP); err != nil {
			return fmt.Errorf("delete endpoint bypass route %s: %w", endpointIP, err)
		}
		return nil
	})

	for _, cidr := range plan.StaticBypassCIDRs {
		target := cidr.String()
		if err := runner.Run(ctx, "ip", "route", "add", target, "via", defaultRoute.Gateway.String(), "dev", defaultRoute.Device); err != nil {
			return fmt.Errorf("add static bypass route %s via %s dev %s: %w", target, defaultRoute.Gateway, defaultRoute.Device, err)
		}
		cleanup.Add("delete static bypass route "+target, func() error {
			if err := runner.Run(context.Background(), "ip", "route", "del", target); err != nil {
				return fmt.Errorf("delete static bypass route %s: %w", target, err)
			}
			return nil
		})
	}

	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if err := runner.Run(ctx, "ip", "route", "add", cidrText, "dev", ifName); err != nil {
			return fmt.Errorf("add tunnel route %s dev %s: %w", cidrText, ifName, err)
		}
		cleanup.Add("delete tunnel route "+cidrText, func() error {
			if err := runner.Run(context.Background(), "ip", "route", "del", cidrText, "dev", ifName); err != nil {
				return fmt.Errorf("delete tunnel route %s dev %s: %w", cidrText, ifName, err)
			}
			return nil
		})
	}

	return nil
}
