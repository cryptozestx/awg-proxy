//go:build linux

package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

type LinuxRouteManager struct {
	Runner CommandRunner
}

func (m LinuxRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	for _, cmd := range linuxConfigureAddressCommands(ifName, addr, mtu) {
		if err := m.Runner.Run(ctx, cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (m LinuxRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	out, err := m.Runner.Output(ctx, "ip", "route", "show", "default")
	if err != nil {
		return DefaultRoute{}, err
	}
	return parseLinuxDefaultRoute(string(out))
}

func parseLinuxDefaultRoute(out string) (DefaultRoute, error) {
	fields := strings.Fields(out)
	var route DefaultRoute
	for i := 0; i+1 < len(fields); i++ {
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
	if !route.Gateway.IsValid() {
		return DefaultRoute{}, fmt.Errorf("default route gateway missing")
	}
	if route.Device == "" {
		return DefaultRoute{}, fmt.Errorf("default route device missing")
	}
	return route, nil
}

func (m LinuxRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	endpointIP := plan.EndpointBypass.Addr().String()
	if err := m.Runner.Run(ctx, "ip", "route", "add", endpointIP, "via", defaultRoute.Gateway.String(), "dev", defaultRoute.Device); err != nil {
		return err
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		return m.Runner.Run(context.Background(), "ip", "route", "del", endpointIP)
	})

	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if err := m.Runner.Run(ctx, "ip", "route", "add", cidrText, "dev", ifName); err != nil {
			return err
		}
		cleanup.Add("delete tunnel route "+cidrText, func() error {
			return m.Runner.Run(context.Background(), "ip", "route", "del", cidrText, "dev", ifName)
		})
	}

	return nil
}
