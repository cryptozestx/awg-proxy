package main

import (
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadTunnelRulesStaticBypass(t *testing.T) {
	path := writeTempRules(t, `
exclude_ip = 203.0.113.10
exclude_cidr = 198.51.100.0/24
`)

	rules, err := LoadTunnelRules(path)
	if err != nil {
		t.Fatalf("LoadTunnelRules returned error: %v", err)
	}

	want := []netip.Prefix{
		netip.MustParsePrefix("203.0.113.10/32"),
		netip.MustParsePrefix("198.51.100.0/24"),
	}
	if !reflect.DeepEqual(rules.StaticBypassCIDRs, want) {
		t.Fatalf("StaticBypassCIDRs = %v, want %v", rules.StaticBypassCIDRs, want)
	}
}

func TestLoadTunnelRulesEmptyPathReturnsEmptyRules(t *testing.T) {
	rules, err := LoadTunnelRules("")
	if err != nil {
		t.Fatalf("LoadTunnelRules returned error: %v", err)
	}
	if len(rules.StaticBypassCIDRs) != 0 {
		t.Fatalf("StaticBypassCIDRs = %v, want empty", rules.StaticBypassCIDRs)
	}
	if len(rules.DomainRules) != 0 {
		t.Fatalf("DomainRules = %v, want empty", rules.DomainRules)
	}
}

func TestLoadTunnelRulesRejectsInvalidStaticRule(t *testing.T) {
	path := writeTempRules(t, `exclude_cidr = not-a-cidr`)

	_, err := LoadTunnelRules(path)
	if err == nil {
		t.Fatalf("LoadTunnelRules succeeded, want error")
	}
}

func TestLoadTunnelRulesRejectsUnknownKey(t *testing.T) {
	path := writeTempRules(t, `include_domain = example.com`)

	_, err := LoadTunnelRules(path)
	if err == nil {
		t.Fatalf("LoadTunnelRules succeeded, want error")
	}
}

func TestDomainRuleMatchesDelimobilWildcard(t *testing.T) {
	rule := DomainRule{Pattern: "*.delimobil.*"}

	matches := []string{
		"git.delimobil.ru",
		"ride-frontend-mobile.st.delimobil.ru",
		"GIT.DELIMOBIL.RU.",
	}
	for _, host := range matches {
		if !rule.Matches(host) {
			t.Fatalf("Matches(%q) = false, want true", host)
		}
	}

	nonMatches := []string{
		"ya.ru",
		"openai.com",
		"delimobil.ru",
		"git.delimobil",
		"git.delimobil.ru.evil.com",
		"x.delimobil.attacker.com",
		".delimobil.ru",
		"git.delimobil.ru..",
	}
	for _, host := range nonMatches {
		if rule.Matches(host) {
			t.Fatalf("Matches(%q) = true, want false", host)
		}
	}
}

func TestTunnelRulesHasDomainRules(t *testing.T) {
	path := writeTempRules(t, `exclude_domain = *.delimobil.*`)

	rules, err := LoadTunnelRules(path)
	if err != nil {
		t.Fatalf("LoadTunnelRules returned error: %v", err)
	}
	if len(rules.DomainRules) != 1 {
		t.Fatalf("DomainRules len = %d, want 1", len(rules.DomainRules))
	}
	if !rules.HasDomainRules() {
		t.Fatalf("HasDomainRules = false, want true")
	}
}

func writeTempRules(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tunnel.rules")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	return path
}
