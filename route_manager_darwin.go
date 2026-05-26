//go:build darwin

package main

import (
	"awg-proxy/internal/platform"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"
)

type DarwinRouteManager struct {
	Runner platform.CommandRunner
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

func NewPlatformDynamicBypassRoutes(defaultRoute DefaultRoute) DynamicBypassRoutes {
	return &DarwinDynamicBypassRoutes{Runner: platform.ExecRunner{}, DefaultRoute: defaultRoute}
}

type DarwinDynamicBypassRoutes struct {
	Runner       platform.CommandRunner
	DefaultRoute DefaultRoute
	set          dynamicRouteSet
}

func (m *DarwinDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	target := routeTarget(prefix)
	if !m.set.reserve(prefix, ttl, m.deleteBypassRoute) {
		return nil
	}
	if err := m.Runner.Run(ctx, "route", "add", target, m.DefaultRoute.Gateway.String()); err != nil {
		m.set.forget(prefix)
		return fmt.Errorf("add dynamic bypass route %s via %s: %w", target, m.DefaultRoute.Gateway, err)
	}
	m.set.markAdded(prefix)
	return nil
}

func (m *DarwinDynamicBypassRoutes) Close() error {
	var errs []error
	for _, prefix := range m.set.takeAdded() {
		if err := m.deleteBypassRoute(prefix); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *DarwinDynamicBypassRoutes) deleteBypassRoute(prefix netip.Prefix) error {
	target := routeTarget(prefix)
	if err := m.Runner.Run(context.Background(), "route", "delete", target); err != nil {
		return fmt.Errorf("delete dynamic bypass route %s: %w", target, err)
	}
	return nil
}
