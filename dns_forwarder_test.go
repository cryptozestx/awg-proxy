package main

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

type fakeDynamicBypassRoutes struct {
	prefixes []netip.Prefix
	reasons  []string
	ttls     []time.Duration
}

func (r *fakeDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.prefixes = append(r.prefixes, prefix)
	r.reasons = append(r.reasons, reason)
	r.ttls = append(r.ttls, ttl)
	return nil
}

func (r *fakeDynamicBypassRoutes) Close() error {
	return nil
}

func TestDomainBypassRuntimeAddsRoutesForMatchingAnswers(t *testing.T) {
	runtime := NewDomainBypassRuntime()
	routes := &fakeDynamicBypassRoutes{}
	rules := TunnelRules{DomainRules: []DomainRule{{Pattern: "*.delimobil.*"}}}
	answer := DNSAnswer{
		Name: "git.delimobil.ru",
		A:    []netip.Addr{netip.MustParseAddr("198.51.100.44")},
		TTL:  30 * time.Second,
	}

	if err := runtime.HandleAnswer(context.Background(), rules, answer, routes); err != nil {
		t.Fatalf("HandleAnswer returned error: %v", err)
	}

	want := netip.MustParsePrefix("198.51.100.44/32")
	if len(routes.prefixes) != 1 || routes.prefixes[0] != want {
		t.Fatalf("prefixes = %v, want [%v]", routes.prefixes, want)
	}
	if routes.reasons[0] != "dns:git.delimobil.ru" {
		t.Fatalf("reason = %q, want dns:git.delimobil.ru", routes.reasons[0])
	}
	if routes.ttls[0] != 10*time.Minute {
		t.Fatalf("ttl = %v, want 10m", routes.ttls[0])
	}
}

func TestDomainBypassRuntimeIgnoresNonMatchingAnswers(t *testing.T) {
	runtime := NewDomainBypassRuntime()
	routes := &fakeDynamicBypassRoutes{}
	rules := TunnelRules{DomainRules: []DomainRule{{Pattern: "*.delimobil.*"}}}
	answer := DNSAnswer{
		Name: "openai.com",
		A:    []netip.Addr{netip.MustParseAddr("198.51.100.44")},
		TTL:  time.Hour,
	}

	if err := runtime.HandleAnswer(context.Background(), rules, answer, routes); err != nil {
		t.Fatalf("HandleAnswer returned error: %v", err)
	}
	if len(routes.prefixes) != 0 {
		t.Fatalf("prefixes = %v, want empty", routes.prefixes)
	}
}
