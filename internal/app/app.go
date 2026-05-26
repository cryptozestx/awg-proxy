package app

import (
	"awg-proxy/internal/config"
	"fmt"
	"io"
	"os"
)

type Runtime struct {
	Stdout io.Writer
	Stderr io.Writer
}

func Run(args []string) error {
	return Runtime{}.Run(args)
}

func (r Runtime) Run(args []string) error {
	stdout := r.stdout()
	stderr := r.stderr()

	opts, err := ParseCLI(args)
	if err != nil {
		PrintUsage(stderr)
		return err
	}

	if opts.ConfigPath == "" {
		PrintUsage(stderr)
		return fmt.Errorf("configuration file path is required")
	}

	fmt.Fprintf(stdout, "[awg-proxy] Parsing configuration: %s...\n", opts.ConfigPath)
	cfg, err := config.Parse(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("configuration parse error: %w", err)
	}
	fmt.Fprintln(stdout, "[awg-proxy] Configuration parsed successfully.")

	if opts.Command == "tunnel" {
		return r.runTunnelMode(cfg, opts)
	}

	return r.runProxyMode(cfg, opts)
}

func (r Runtime) stdout() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r Runtime) stderr() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}
