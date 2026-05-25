//go:build darwin

package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

type DarwinRouteManager struct {
	Runner CommandRunner
}

func (m DarwinRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	cmd := darwinConfigureAddressCommand(ifName, addr, mtu)
	return m.Runner.Run(ctx, cmd[0], cmd[1:]...)
}

func (m DarwinRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	out, err := m.Runner.Output(ctx, "route", "-n", "get", "default")
	if err != nil {
		return DefaultRoute{}, err
	}
	return parseDarwinDefaultRoute(string(out))
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

func (m DarwinRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	endpointIP := plan.EndpointBypass.Addr().String()
	if err := m.Runner.Run(ctx, "route", "add", endpointIP, defaultRoute.Gateway.String()); err != nil {
		return err
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		return m.Runner.Run(context.Background(), "route", "delete", endpointIP)
	})

	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if err := m.Runner.Run(ctx, "route", "add", cidrText, "-interface", ifName); err != nil {
			return err
		}
		cleanup.Add("delete tunnel route "+cidrText, func() error {
			return m.Runner.Run(context.Background(), "route", "delete", cidrText)
		})
	}

	return nil
}
