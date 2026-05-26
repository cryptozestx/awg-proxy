//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"
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

func NewPlatformDynamicBypassRoutes(defaultRoute DefaultRoute) DynamicBypassRoutes {
	return &LinuxDynamicBypassRoutes{Runner: ExecRunner{}, DefaultRoute: defaultRoute}
}

type LinuxDynamicBypassRoutes struct {
	Runner       CommandRunner
	DefaultRoute DefaultRoute
	set          dynamicRouteSet
}

func (m *LinuxDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	target := prefix.String()
	if !m.set.reserve(prefix, ttl, m.deleteBypassRoute) {
		return nil
	}
	if err := m.Runner.Run(ctx, "ip", "route", "add", target, "via", m.DefaultRoute.Gateway.String(), "dev", m.DefaultRoute.Device); err != nil {
		m.set.forget(prefix)
		return fmt.Errorf("add dynamic bypass route %s via %s dev %s: %w", target, m.DefaultRoute.Gateway, m.DefaultRoute.Device, err)
	}
	m.set.markAdded(prefix)
	return nil
}

func (m *LinuxDynamicBypassRoutes) Close() error {
	var errs []error
	for _, prefix := range m.set.takeAdded() {
		if err := m.deleteBypassRoute(prefix); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *LinuxDynamicBypassRoutes) deleteBypassRoute(prefix netip.Prefix) error {
	target := prefix.String()
	if err := m.Runner.Run(context.Background(), "ip", "route", "del", target); err != nil {
		return fmt.Errorf("delete dynamic bypass route %s: %w", target, err)
	}
	return nil
}
