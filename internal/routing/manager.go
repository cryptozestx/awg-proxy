package routing

import (
	"context"
	"net/netip"
	"time"
)

type DefaultRoute struct {
	Gateway netip.Addr
	Device  string
}

type Manager interface {
	ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error
	DefaultRoute(ctx context.Context) (DefaultRoute, error)
	Apply(ctx context.Context, ifName string, plan Plan, defaultRoute DefaultRoute, cleanup Cleanup) error
}

type Cleanup interface {
	Add(name string, fn func() error)
}

type DynamicBypassRoutes interface {
	AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error
	Close() error
}

type DynamicBypassRouteFactory func(DefaultRoute) DynamicBypassRoutes
