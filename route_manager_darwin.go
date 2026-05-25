//go:build darwin

package main

import (
	"context"
	"fmt"
	"net/netip"
)

type DarwinRouteManager struct {
	Runner CommandRunner
}

func (m DarwinRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	cmd := darwinConfigureAddressCommand(ifName, addr, mtu)
	if err := m.Runner.Run(ctx, cmd[0], cmd[1:]...); err != nil {
		return fmt.Errorf("configure interface %s address %s mtu %d: %w", ifName, addr, mtu, err)
	}
	return nil
}

func (m DarwinRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	out, err := m.Runner.Output(ctx, "route", "-n", "get", "default")
	if err != nil {
		return DefaultRoute{}, fmt.Errorf("get default route: %w", err)
	}
	return parseDarwinDefaultRoute(string(out))
}

func (m DarwinRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	return darwinApplyRoutes(ctx, m.Runner, ifName, plan, defaultRoute, cleanup)
}
