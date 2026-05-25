package main

import "testing"

func TestParseCLIRecognizesTunnel(t *testing.T) {
	opts, err := parseCLI([]string{"awg-proxy", "tunnel", "-c", "amnezia.conf", "--dry-run", "--no-dns", "--verbose"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
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

func TestParseCLIDefaultsFlagOnlyInvocationToShell(t *testing.T) {
	opts, err := parseCLI([]string{"awg-proxy", "-c", "amnezia.conf"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if opts.Command != "shell" {
		t.Fatalf("Command = %q, want shell", opts.Command)
	}
}

func TestParseCLIRunRequiresSeparatorAndCommand(t *testing.T) {
	_, err := parseCLI([]string{"awg-proxy", "run", "-c", "amnezia.conf"})
	if err == nil {
		t.Fatalf("parseCLI succeeded, want error")
	}
}
