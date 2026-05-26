package main

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

type TunnelRules struct {
	StaticBypass []netip.Prefix
	DomainRules  []DomainRule
}

type DomainRule struct {
	Pattern string
}

func (r TunnelRules) HasDomainRules() bool {
	return len(r.DomainRules) > 0
}

func (r TunnelRules) StaticBypassCIDRs() []netip.Prefix {
	return append([]netip.Prefix(nil), r.StaticBypass...)
}

func (r DomainRule) Matches(host string) bool {
	pattern := normalizeDomainPattern(r.Pattern)
	host = normalizeDomainPattern(host)
	if pattern == "" || host == "" {
		return false
	}
	return matchDomainGlob(pattern, host)
}

func LoadTunnelRules(path string) (TunnelRules, error) {
	var rules TunnelRules
	if path == "" {
		return rules, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return rules, fmt.Errorf("open tunnel rules %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if idx := strings.IndexAny(line, "#;"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return rules, fmt.Errorf("parse tunnel rules %s:%d: expected key = value", path, lineNo)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			return rules, fmt.Errorf("parse tunnel rules %s:%d: empty value for %s", path, lineNo, key)
		}

		switch key {
		case "exclude_ip":
			addr, err := netip.ParseAddr(value)
			if err != nil {
				return rules, fmt.Errorf("parse tunnel rules %s:%d: invalid exclude_ip %q: %w", path, lineNo, value, err)
			}
			if !addr.Is4() {
				return rules, fmt.Errorf("parse tunnel rules %s:%d: exclude_ip must be IPv4: %s", path, lineNo, value)
			}
			rules.StaticBypass = append(rules.StaticBypass, netip.PrefixFrom(addr, 32))
		case "exclude_cidr":
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return rules, fmt.Errorf("parse tunnel rules %s:%d: invalid exclude_cidr %q: %w", path, lineNo, value, err)
			}
			if !prefix.Addr().Is4() {
				return rules, fmt.Errorf("parse tunnel rules %s:%d: exclude_cidr must be IPv4: %s", path, lineNo, value)
			}
			rules.StaticBypass = append(rules.StaticBypass, prefix.Masked())
		case "exclude_domain":
			rules.DomainRules = append(rules.DomainRules, DomainRule{Pattern: normalizeDomainPattern(value)})
		default:
			return rules, fmt.Errorf("parse tunnel rules %s:%d: unknown key %q", path, lineNo, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return rules, fmt.Errorf("read tunnel rules %s: %w", path, err)
	}

	return rules, nil
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
