package dns

import (
	"context"
	"net/netip"
	"strings"
	"time"
)

type DomainRule struct {
	Pattern string
}

type DynamicRouteAdder interface {
	AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error
}

type DynamicRoutes interface {
	DynamicRouteAdder
	Close() error
}

func (r DomainRule) Matches(host string) bool {
	pattern := normalizeDomainPattern(r.Pattern)
	host = normalizeDomainPattern(host)
	if pattern == "" || host == "" {
		return false
	}
	return matchDomainGlob(pattern, host)
}

func domainRulesMatch(rules []DomainRule, host string) bool {
	for _, rule := range rules {
		if rule.Matches(host) {
			return true
		}
	}
	return false
}

func normalizeDomainPattern(pattern string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(pattern)), ".")
}

func matchDomainGlob(pattern, host string) bool {
	pParts := strings.Split(pattern, ".")
	hParts := strings.Split(host, ".")
	if !validDomainParts(pParts) || !validDomainParts(hParts) {
		return false
	}
	return matchDomainParts(pParts, hParts)
}

func validDomainParts(parts []string) bool {
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func matchDomainParts(pattern, host []string) bool {
	if len(pattern) == 0 {
		return len(host) == 0
	}
	if pattern[0] != "*" {
		return len(host) > 0 && pattern[0] == host[0] && matchDomainParts(pattern[1:], host[1:])
	}
	if len(pattern) == 1 {
		return len(host) == 1
	}
	for consumed := 1; consumed <= len(host); consumed++ {
		if matchDomainParts(pattern[1:], host[consumed:]) {
			return true
		}
	}
	return false
}
