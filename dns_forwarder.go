package main

import (
	"context"
	"net/netip"
	"time"
)

const minDomainBypassTTL = 10 * time.Minute

type DNSAnswer struct {
	Name string
	A    []netip.Addr
	TTL  time.Duration
}

type DomainBypassRuntime struct{}

func NewDomainBypassRuntime() *DomainBypassRuntime {
	return &DomainBypassRuntime{}
}

func (r *DomainBypassRuntime) HandleAnswer(ctx context.Context, rules TunnelRules, answer DNSAnswer, routes DynamicBypassRoutes) error {
	if routes == nil {
		return nil
	}
	if !domainRulesMatch(rules.DomainRules, answer.Name) {
		return nil
	}
	ttl := answer.TTL
	if ttl < minDomainBypassTTL {
		ttl = minDomainBypassTTL
	}
	for _, addr := range answer.A {
		if !addr.Is4() {
			continue
		}
		if err := routes.AddBypassRoute(ctx, netip.PrefixFrom(addr, 32), "dns:"+normalizeDomainPattern(answer.Name), ttl); err != nil {
			return err
		}
	}
	return nil
}

func domainRulesMatch(rules []DomainRule, host string) bool {
	for _, rule := range rules {
		if rule.Matches(host) {
			return true
		}
	}
	return false
}
