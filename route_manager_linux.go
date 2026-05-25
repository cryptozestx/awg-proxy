//go:build linux

package main

import (
	"context"
	"fmt"
	"net/netip"
)

type LinuxRouteManager struct {
	Runner CommandRunner
}

func (m LinuxRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	for _, cmd := range linuxConfigureAddressCommands(ifName, addr, mtu) {
		if err := m.Runner.Run(ctx, cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("configure interface %s with %s: %w", ifName, commandString(cmd[0], cmd[1:]...), err)
		}
	}
	return nil
}

func (m LinuxRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	out, err := m.Runner.Output(ctx, "ip", "route", "show", "default")
	if err != nil {
		return DefaultRoute{}, fmt.Errorf("get default route: %w", err)
	}
	return parseLinuxDefaultRoute(string(out))
}

func (m LinuxRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	return linuxApplyRoutes(ctx, m.Runner, ifName, plan, defaultRoute, cleanup)
}
