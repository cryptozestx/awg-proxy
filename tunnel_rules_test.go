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

func writeTempRules(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tunnel.rules")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	return path
}
