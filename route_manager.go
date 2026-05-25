package main

import (
	"context"
	"net/netip"
)

type DefaultRoute struct {
	Gateway netip.Addr
	Device  string
}

type RouteManager interface {
	ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error
	DefaultRoute(ctx context.Context) (DefaultRoute, error)
	Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error
}
