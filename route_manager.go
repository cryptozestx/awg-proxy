package main

import (
	"context"
	"net/netip"
	"time"
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

type DynamicBypassRoutes interface {
	AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error
	Close() error
}

type DynamicBypassRouteFactory func(DefaultRoute) DynamicBypassRoutes
