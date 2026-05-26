package tunnel

import (
	"awg-proxy/internal/config"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func validTunnelConfig() *config.AWGConfig {
	return &config.AWGConfig{
		Interface: config.InterfaceConfig{
			PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			Address:    []string{"10.8.0.2/32", "fd00::2/128"},
			DNS:        []string{"1.1.1.1"},
		},
		Peers: []config.PeerConfig{{
			PublicKey:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			Endpoint:   "vpn.example.test:51820",
			AllowedIPs: []string{"0.0.0.0/0"},
		}},
	}
}

func TestValidateConfigPreservesIPv4CIDR(t *testing.T) {
	cfg := validTunnelConfig()

	tunnel, err := ValidateConfig(cfg)
	if err != nil {
		t.Fatalf("ValidateConfig returned error: %v", err)
	}

	if tunnel.InterfaceIPv4.String() != "10.8.0.2/32" {
		t.Fatalf("InterfaceIPv4 = %q, want 10.8.0.2/32", tunnel.InterfaceIPv4)
	}
}

func TestValidateConfigRejectsMissingPeerEndpoint(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Peers[0].Endpoint = ""

	_, err := ValidateConfig(cfg)
	if err == nil {
		t.Fatalf("ValidateConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "exactly one peer with Endpoint") {
		t.Fatalf("error = %q, want exactly one peer with Endpoint", err)
	}
}

func TestValidateConfigRejectsMissingIPv4Address(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Interface.Address = []string{"fd00::2/128"}

	_, err := ValidateConfig(cfg)
	if err == nil {
		t.Fatalf("ValidateConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "IPv4 CIDR") {
		t.Fatalf("error = %q, want IPv4 CIDR", err)
	}
}

func TestValidateConfigRejectsMalformedAddressWithValidIPv4Present(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Interface.Address = []string{"bad", "10.8.0.2/32"}

	_, err := ValidateConfig(cfg)
	if err == nil {
		t.Fatalf("ValidateConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "invalid Interface Address CIDR") {
		t.Fatalf("error = %q, want invalid Interface Address CIDR", err)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error = %q, want offending value", err)
	}
}

func TestValidateConfigRejectsMalformedAddressAfterValidIPv4(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Interface.Address = []string{"10.8.0.2/32", "bad"}

	_, err := ValidateConfig(cfg)
	if err == nil {
		t.Fatalf("ValidateConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "invalid Interface Address CIDR") {
		t.Fatalf("error = %q, want invalid Interface Address CIDR", err)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error = %q, want offending value", err)
	}
}

func TestValidateConfigRejectsMalformedAddressWithoutValidIPv4(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Interface.Address = []string{"bad", "fd00::2/128"}

	_, err := ValidateConfig(cfg)
	if err == nil {
		t.Fatalf("ValidateConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "invalid Interface Address CIDR") {
		t.Fatalf("error = %q, want invalid Interface Address CIDR", err)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error = %q, want offending value", err)
	}
}

func TestCloneConfigWithResolvedEndpointRewritesCloneAndDoesNotMutateOriginal(t *testing.T) {
	cfg := validTunnelConfig()
	endpoint := netip.MustParseAddrPort("203.0.113.10:51820")

	clone := CloneConfigWithResolvedEndpoint(cfg, endpoint)

	if clone.Peers[0].Endpoint != "203.0.113.10:51820" {
		t.Fatalf("clone Endpoint = %q, want 203.0.113.10:51820", clone.Peers[0].Endpoint)
	}
	if cfg.Peers[0].Endpoint != "vpn.example.test:51820" {
		t.Fatalf("original Endpoint = %q, want vpn.example.test:51820", cfg.Peers[0].Endpoint)
	}

	clone.Interface.Address[0] = "10.8.0.99/32"
	clone.Interface.DNS[0] = "9.9.9.9"
	clone.Peers[0].AllowedIPs[0] = "10.0.0.0/8"

	if cfg.Interface.Address[0] != "10.8.0.2/32" {
		t.Fatalf("original Address mutated to %q", cfg.Interface.Address[0])
	}
	if cfg.Interface.DNS[0] != "1.1.1.1" {
		t.Fatalf("original DNS mutated to %q", cfg.Interface.DNS[0])
	}
	if cfg.Peers[0].AllowedIPs[0] != "0.0.0.0/0" {
		t.Fatalf("original AllowedIPs mutated to %q", cfg.Peers[0].AllowedIPs[0])
	}
}

func TestResolveEndpointIPv4RejectsIPv6OnlyLookupResult(t *testing.T) {
	_, err := ResolveEndpointIPv4("vpn.example.test", 51820, func(string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("2001:db8::1")}, nil
	})
	if err == nil {
		t.Fatalf("ResolveEndpointIPv4 succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not IPv4") {
		t.Fatalf("error = %q, want not IPv4", err)
	}
}

func TestResolveEndpointIPv4SelectsFirstIPv4AfterIPv6(t *testing.T) {
	endpoint, err := ResolveEndpointIPv4("vpn.example.test", 51820, func(string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("2001:db8::1"),
			netip.MustParseAddr("203.0.113.10"),
			netip.MustParseAddr("203.0.113.11"),
		}, nil
	})
	if err != nil {
		t.Fatalf("ResolveEndpointIPv4 returned error: %v", err)
	}
	if endpoint.String() != "203.0.113.10:51820" {
		t.Fatalf("endpoint = %q, want 203.0.113.10:51820", endpoint)
	}
}

func TestResolveEndpointIPv4WrapsLookupErrorWithHost(t *testing.T) {
	_, err := ResolveEndpointIPv4("vpn.example.test", 51820, func(string) ([]netip.Addr, error) {
		return nil, errors.New("lookup failed")
	})
	if err == nil {
		t.Fatalf("ResolveEndpointIPv4 succeeded, want error")
	}
	if !strings.Contains(err.Error(), "vpn.example.test") {
		t.Fatalf("error = %q, want host context", err)
	}
}
