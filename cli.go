package main

import (
	"awg-proxy/internal/tunnel"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type CLIOptions struct {
	Command     string
	ConfigPath  string
	SocksPort   int
	HTTPPort    int
	Debug       bool
	AppTarget   string
	CommandArgs []string
	AppArgs     []string
	Tunnel      tunnel.Options
}

func parseCLI(args []string) (CLIOptions, error) {
	var opts CLIOptions
	if len(args) < 2 {
		return opts, fmt.Errorf("missing command")
	}

	argsStart := 1
	switch args[1] {
	case "shell", "run", "server", "app", "tunnel":
		opts.Command = args[1]
		argsStart = 2
	default:
		if len(args[1]) > 0 && args[1][0] == '-' {
			opts.Command = "shell"
		} else {
			return opts, fmt.Errorf("unknown command: %s", args[1])
		}
	}

	fs := flag.NewFlagSet("awg-proxy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to AmneziaWG configuration file")
	fs.StringVar(&opts.ConfigPath, "c", "", "Path to AmneziaWG configuration file")
	fs.IntVar(&opts.SocksPort, "socks-port", 0, "SOCKS5 port to bind")
	fs.IntVar(&opts.SocksPort, "s", 0, "SOCKS5 port to bind")
	fs.IntVar(&opts.HTTPPort, "http-port", 0, "HTTP port to bind")
	fs.IntVar(&opts.HTTPPort, "h", 0, "HTTP port to bind")
	fs.BoolVar(&opts.Debug, "debug", false, "Enable verbose debug logs")
	fs.BoolVar(&opts.Debug, "d", false, "Enable verbose debug logs")
	fs.StringVar(&opts.AppTarget, "app", "", "macOS application name or path to proxy")
	fs.StringVar(&opts.AppTarget, "a", "", "macOS application name or path to proxy")
	fs.BoolVar(&opts.Tunnel.DryRun, "dry-run", false, "Print tunnel changes without applying them")
	fs.StringVar(&opts.Tunnel.RulesPath, "rules", "", "Path to tunnel bypass rules file")
	fs.BoolVar(&opts.Tunnel.NoDNS, "no-dns", false, "Do not change system DNS")
	fs.BoolVar(&opts.Tunnel.Verbose, "verbose", false, "Enable verbose tunnel logging")

	if opts.Command == "run" || opts.Command == "app" {
		sepIdx := -1
		for i := argsStart; i < len(args); i++ {
			if args[i] == "--" {
				sepIdx = i
				break
			}
		}
		if sepIdx != -1 {
			if err := fs.Parse(args[argsStart:sepIdx]); err != nil {
				return opts, err
			}
			opts.CommandArgs = args[sepIdx+1:]
		} else if err := fs.Parse(args[argsStart:]); err != nil {
			return opts, err
		}
	} else if err := fs.Parse(args[argsStart:]); err != nil {
		return opts, err
	}

	if opts.Command == "run" && len(opts.CommandArgs) == 0 {
		return opts, fmt.Errorf("command 'run' requires '--' followed by the CLI command to run")
	}

	if opts.Command == "app" {
		if len(opts.CommandArgs) > 0 {
			if opts.AppTarget == "" {
				opts.AppTarget = opts.CommandArgs[0]
				opts.AppArgs = opts.CommandArgs[1:]
			} else {
				opts.AppArgs = opts.CommandArgs
			}
		} else {
			leftovers := fs.Args()
			if opts.AppTarget == "" {
				if len(leftovers) > 0 {
					opts.AppTarget = leftovers[0]
					opts.AppArgs = leftovers[1:]
				}
			} else {
				opts.AppArgs = leftovers
			}
		}

		if opts.AppTarget == "" {
			return opts, fmt.Errorf("command 'app' requires specifying the application to run")
		}
	}

	opts.ConfigPath = resolveDefaultConfigPath(opts.ConfigPath)
	opts.Tunnel.ConfigPath = opts.ConfigPath
	return opts, nil
}

func resolveDefaultConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	if _, err := os.Stat("amnezia.conf"); err == nil {
		return "amnezia.conf"
	}
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	fallbackPath := filepath.Join(filepath.Dir(execPath), "amnezia.conf")
	if _, err := os.Stat(fallbackPath); err == nil {
		return fallbackPath
	}
	return ""
}
