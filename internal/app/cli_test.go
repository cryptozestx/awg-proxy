package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCLIRecognizesTunnel(t *testing.T) {
	opts, err := ParseCLI([]string{"awg-proxy", "tunnel", "-c", "amnezia.conf", "--dry-run", "--no-dns", "--verbose"})
	if err != nil {
		t.Fatalf("ParseCLI returned error: %v", err)
	}
	if opts.Command != "tunnel" {
		t.Fatalf("Command = %q, want tunnel", opts.Command)
	}
	if opts.ConfigPath != "amnezia.conf" {
		t.Fatalf("ConfigPath = %q", opts.ConfigPath)
	}
	if !opts.Tunnel.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if !opts.Tunnel.NoDNS {
		t.Fatalf("NoDNS = false, want true")
	}
	if !opts.Tunnel.Verbose {
		t.Fatalf("Verbose = false, want true")
	}
}

func TestParseCLITunnelRulesPath(t *testing.T) {
	opts, err := ParseCLI([]string{"awg-proxy", "tunnel", "-c", "amnezia.conf", "--rules", "tunnel.rules"})
	if err != nil {
		t.Fatalf("ParseCLI returned error: %v", err)
	}
	if opts.Command != "tunnel" {
		t.Fatalf("Command = %q, want tunnel", opts.Command)
	}
	if opts.Tunnel.RulesPath != "tunnel.rules" {
		t.Fatalf("RulesPath = %q, want tunnel.rules", opts.Tunnel.RulesPath)
	}
}

func TestParseCLIDefaultsFlagOnlyInvocationToShell(t *testing.T) {
	opts, err := ParseCLI([]string{"awg-proxy", "-c", "amnezia.conf"})
	if err != nil {
		t.Fatalf("ParseCLI returned error: %v", err)
	}
	if opts.Command != "shell" {
		t.Fatalf("Command = %q, want shell", opts.Command)
	}
}

func TestParseCLIRunRequiresSeparatorAndCommand(t *testing.T) {
	_, err := ParseCLI([]string{"awg-proxy", "run", "-c", "amnezia.conf"})
	if err == nil {
		t.Fatalf("ParseCLI succeeded, want error")
	}
}

func TestParseCLIRejectsMissingCommand(t *testing.T) {
	_, err := ParseCLI([]string{"awg-proxy"})
	if err == nil {
		t.Fatalf("ParseCLI succeeded, want error")
	}
}

func TestParseCLIRejectsUnknownCommand(t *testing.T) {
	_, err := ParseCLI([]string{"awg-proxy", "bogus"})
	if err == nil {
		t.Fatalf("ParseCLI succeeded, want error")
	}
}

func TestParseCLIResolvesDefaultConfigForTunnel(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("os.Chdir restore returned error: %v", err)
		}
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "amnezia.conf"), []byte("[Interface]\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir returned error: %v", err)
	}

	opts, err := ParseCLI([]string{"awg-proxy", "tunnel"})
	if err != nil {
		t.Fatalf("ParseCLI returned error: %v", err)
	}
	if opts.ConfigPath != "amnezia.conf" {
		t.Fatalf("ConfigPath = %q, want amnezia.conf", opts.ConfigPath)
	}
	if opts.Tunnel.ConfigPath != "amnezia.conf" {
		t.Fatalf("Tunnel.ConfigPath = %q, want amnezia.conf", opts.Tunnel.ConfigPath)
	}
}
