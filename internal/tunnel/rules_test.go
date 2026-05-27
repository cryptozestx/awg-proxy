package tunnel

import (
	dnsruntime "awg-proxy/internal/dns"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadRulesStaticBypass(t *testing.T) {
	path := writeTempRules(t, `
exclude_ip = 203.0.113.10
exclude_cidr = 198.51.100.0/24
`)

	rules, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules returned error: %v", err)
	}

	want := []netip.Prefix{
		netip.MustParsePrefix("203.0.113.10/32"),
		netip.MustParsePrefix("198.51.100.0/24"),
	}
	if !reflect.DeepEqual(rules.StaticBypass, want) {
		t.Fatalf("StaticBypass = %v, want %v", rules.StaticBypass, want)
	}
	if !reflect.DeepEqual(rules.StaticBypassCIDRs(), want) {
		t.Fatalf("StaticBypassCIDRs() = %v, want %v", rules.StaticBypassCIDRs(), want)
	}
}

func TestLoadRulesEmptyPathReturnsEmptyRules(t *testing.T) {
	rules, err := LoadRules("")
	if err != nil {
		t.Fatalf("LoadRules returned error: %v", err)
	}
	if len(rules.StaticBypass) != 0 {
		t.Fatalf("StaticBypass = %v, want empty", rules.StaticBypass)
	}
	if len(rules.DomainRules) != 0 {
		t.Fatalf("DomainRules = %v, want empty", rules.DomainRules)
	}
}

func TestLoadRulesRejectsInvalidStaticRule(t *testing.T) {
	path := writeTempRules(t, `exclude_cidr = not-a-cidr`)

	_, err := LoadRules(path)
	if err == nil {
		t.Fatalf("LoadRules succeeded, want error")
	}
}

func TestLoadRulesRejectsUnknownKey(t *testing.T) {
	path := writeTempRules(t, `include_domain = example.com`)

	_, err := LoadRules(path)
	if err == nil {
		t.Fatalf("LoadRules succeeded, want error")
	}
}

func TestRulesHasDomainRules(t *testing.T) {
	path := writeTempRules(t, `exclude_domain = *.delimobil.*`)

	rules, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules returned error: %v", err)
	}
	if len(rules.DomainRules) != 1 {
		t.Fatalf("DomainRules len = %d, want 1", len(rules.DomainRules))
	}
	if !rules.HasDomainRules() {
		t.Fatalf("HasDomainRules = false, want true")
	}
}

func TestRulesDNSDomainRulesReturnsDefensiveCopy(t *testing.T) {
	rules := Rules{
		DomainRules: []dnsruntime.DomainRule{{Pattern: "*.delimobil.*"}},
	}

	copied := rules.DNSDomainRules()
	copied[0].Pattern = "*.changed.test"

	if rules.DomainRules[0].Pattern != "*.delimobil.*" {
		t.Fatalf("DomainRules mutated to %q", rules.DomainRules[0].Pattern)
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
