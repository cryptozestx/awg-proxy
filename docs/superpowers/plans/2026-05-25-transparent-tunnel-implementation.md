# Transparent Tunnel Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a privileged `tunnel` command that creates a native macOS/Linux TUN interface and routes IPv4 system traffic through AmneziaWG.

**Architecture:** Keep current rootless proxy modes on gVisor netstack. Add a separate `tunnel` path that dispatches before proxy infrastructure is created, validates tunnel-specific config, resolves and rewrites the peer endpoint to an IPv4 `ip:port`, creates a native TUN-backed `amneziawg-go` device, applies full-tunnel `/1` routes, applies DNS unless `--no-dns`, and rolls back every successful setup step in reverse order.

**Tech Stack:** Go 1.24, `github.com/amnezia-vpn/amneziawg-go` native `tun` and `device` packages, command-backed macOS `ifconfig`/`route`/`networksetup`, command-backed Linux `ip` and `/etc/resolv.conf`, standard Go unit tests with fake runners.

---

## File Structure

- Modify `main.go`: add `tunnel` command recognition and dispatch it before current netstack/proxy setup.
- Create `cli.go`: parse shared CLI flags and tunnel-only flags into typed options so `main.go` stays small.
- Create `cli_test.go`: verify command parsing, `tunnel` flags, and default config resolution.
- Create `tunnel_config.go`: tunnel validation, IPv4 CIDR parsing, endpoint parsing/resolution, config cloning with resolved endpoint.
- Create `tunnel_config_test.go`: validation, CIDR preservation, IPv6 rejection, endpoint rewrite tests.
- Create `cleanup.go`: idempotent rollback stack.
- Create `cleanup_test.go`: reverse-order cleanup and double-cleanup behavior.
- Create `route_policy.go`: full-tunnel route plan with endpoint bypass.
- Create `route_policy_test.go`: route plan tests.
- Create `command_runner.go`: command runner abstraction plus dry-run recording runner.
- Create `command_runner_test.go`: dry-run recording tests.
- Create `route_manager.go`: shared route manager interfaces and route record types.
- Create `route_manager_builders.go`: platform command builder helpers that are unit-testable on every OS.
- Create `route_manager_darwin.go`: macOS command-backed route and TUN address operations.
- Create `route_manager_linux.go`: Linux command-backed route and TUN address operations.
- Create `route_manager_test.go`: platform-independent tests using the command builder functions.
- Create `dns_manager.go`: shared DNS manager interface.
- Create `dns_manager_darwin_parse.go`: untagged macOS DNS output parsers for cross-platform tests.
- Create `dns_manager_darwin.go`: macOS DNS apply/restore via `networksetup`.
- Create `dns_manager_linux.go`: Linux DNS apply/restore for regular `resolv.conf`.
- Create `dns_manager_darwin_test.go`: macOS DNS parser tests.
- Create `dns_manager_linux_test.go`: Linux DNS file tests under `//go:build linux`.
- Create `tunnel_device.go`: interfaces for native TUN device lifecycle and real `amneziawg-go` device creation.
- Create `tunnel.go`: `RunTunnel` lifecycle orchestration.
- Create `tunnel_test.go`: lifecycle tests with fake device, route manager, DNS manager, and cleanup failure paths.
- Modify `README.md` and `README_RU.md`: document `tunnel`, privileges, DNS behavior, and `--dry-run`.

---

### Task 1: Extract CLI Parsing And Add Tunnel Command Recognition

**Files:**
- Create: `cli.go`
- Create: `cli_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing CLI parser tests**

Add `cli_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./... -run 'TestParseCLI' -count=1
```

Expected: FAIL because `parseCLI` and option types do not exist.

- [ ] **Step 3: Implement CLI parser**

Create `cli.go`:

```go
package main

import (
	"flag"
	"fmt"
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
	Tunnel      TunnelOptions
}

type TunnelOptions struct {
	ConfigPath string
	DryRun     bool
	NoDNS      bool
	Verbose    bool
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
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to AmneziaWG configuration file")
	fs.StringVar(&opts.ConfigPath, "c", "", "Path to AmneziaWG configuration file")
	fs.IntVar(&opts.SocksPort, "socks-port", 0, "SOCKS5 port to bind")
	fs.IntVar(&opts.SocksPort, "s", 0, "SOCKS5 port to bind")
	fs.IntVar(&opts.HTTPPort, "http-port", 0, "HTTP port to bind")
	fs.IntVar(&opts.HTTPPort, "h", 0, "HTTP port to bind")
	fs.BoolVar(&opts.Debug, "debug", false, "Enable verbose debug logs")
	fs.BoolVar(&opts.Debug, "d", false, "Enable verbose debug logs")
	fs.StringVar(&opts.AppTarget, "app", "", "macOS application name or path")
	fs.StringVar(&opts.AppTarget, "a", "", "macOS application name or path")
	fs.BoolVar(&opts.Tunnel.DryRun, "dry-run", false, "Print tunnel changes without applying them")
	fs.BoolVar(&opts.Tunnel.NoDNS, "no-dns", false, "Do not change system DNS")
	fs.BoolVar(&opts.Tunnel.Verbose, "verbose", false, "Verbose tunnel lifecycle logging")

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
		leftovers := fs.Args()
		if len(opts.CommandArgs) > 0 {
			if opts.AppTarget == "" {
				opts.AppTarget = opts.CommandArgs[0]
				opts.AppArgs = opts.CommandArgs[1:]
			} else {
				opts.AppArgs = opts.CommandArgs
			}
		} else if opts.AppTarget == "" && len(leftovers) > 0 {
			opts.AppTarget = leftovers[0]
			opts.AppArgs = leftovers[1:]
		} else {
			opts.AppArgs = leftovers
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
	if execPath, err := os.Executable(); err == nil {
		fallbackPath := filepath.Join(filepath.Dir(execPath), "amnezia.conf")
		if _, err := os.Stat(fallbackPath); err == nil {
			return fallbackPath
		}
	}
	return ""
}
```

- [ ] **Step 4: Refactor `main.go` to use parser and early tunnel dispatch**

Replace manual command parsing in `main()` with:

```go
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	opts, err := parseCLI(os.Args)
	if err != nil {
		fmt.Printf("\x1b[1;31mError: %v\x1b[0m\n\n", err)
		printUsage()
		os.Exit(1)
	}

	if opts.ConfigPath == "" {
		fmt.Println("\x1b[1;31mError: Configuration file path is required.\x1b[0m")
		printUsage()
		os.Exit(1)
	}

	fmt.Printf("[awg-proxy] Parsing configuration: %s...\n", opts.ConfigPath)
	cfg, err := ParseConfig(opts.ConfigPath)
	if err != nil {
		log.Fatalf("Configuration parse error: %v", err)
	}

	if opts.Command == "tunnel" {
		if err := RunTunnel(cfg, opts.Tunnel); err != nil {
			log.Fatalf("Tunnel error: %v", err)
		}
		return
	}

	runProxyMode(cfg, opts)
}
```

Move the existing netstack/proxy body into:

```go
func runProxyMode(cfg *AWGConfig, opts CLIOptions) {
	localAddrs, err := parseAddresses(cfg.Interface.Address)
	if err != nil {
		log.Fatalf("Failed to parse interface addresses: %v", err)
	}
	if len(localAddrs) == 0 {
		log.Fatalf("No interface IP addresses defined in [Interface]")
	}

	dnsAddrs, err := parseAddresses(cfg.Interface.DNS)
	if err != nil {
		log.Printf("[Warning] DNS parse issue: %v. Defaulting to 1.1.1.1.", err)
		dnsAddrs = []netip.Addr{netip.MustParseAddr("1.1.1.1")}
	}
	if len(dnsAddrs) == 0 {
		dnsAddrs = []netip.Addr{netip.MustParseAddr("1.1.1.1")}
	}

	mtu := cfg.Interface.MTU
	if mtu <= 0 {
		mtu = 1420
	}

	fmt.Println("[awg-proxy] Initializing userspace network stack...")
	tunDev, tnet, err := netstack.CreateNetTUN(localAddrs, dnsAddrs, mtu)
	if err != nil {
		log.Fatalf("Failed to create userspace network stack: %v", err)
	}

	logLevel := device.LogLevelSilent
	if opts.Debug {
		logLevel = device.LogLevelVerbose
	}
	logger := device.NewLogger(logLevel, "[AWG] ")
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	fmt.Println("[awg-proxy] Setting up secure AmneziaWG connection tunnel...")
	uapiConf, err := cfg.ToUAPI()
	if err != nil {
		log.Fatalf("Failed to construct UAPI config: %v", err)
	}
	if err := dev.IpcSet(uapiConf); err != nil {
		log.Fatalf("Failed to configure AmneziaWG interface keys & obfuscation: %v", err)
	}
	if err := dev.Up(); err != nil {
		log.Fatalf("Failed to establish tunnel connection: %v", err)
	}
	defer dev.Close()

	socksServer, socksActualPort, err := NewSOCKS5Server(opts.SocksPort, tnet)
	if err != nil {
		log.Fatalf("Failed to start SOCKS5 proxy server: %v", err)
	}
	defer socksServer.Close()
	go socksServer.Start()

	httpServer, httpActualPort, err := NewHTTPProxyServer(opts.HTTPPort, tnet)
	if err != nil {
		log.Fatalf("Failed to start HTTP proxy server: %v", err)
	}
	defer httpServer.Close()

	switch opts.Command {
	case "server":
		waitForProxyInterrupt(socksActualPort, httpActualPort)
	case "run":
		if err := RunCommand(opts.CommandArgs, socksActualPort, httpActualPort); err != nil {
			log.Fatalf("Command returned exit error: %v", err)
		}
	case "app":
		if err := RunApp(opts.AppTarget, opts.AppArgs, socksActualPort, httpActualPort); err != nil {
			log.Fatalf("App returned exit error: %v", err)
		}
	case "shell":
		if err := RunShell(socksActualPort, httpActualPort); err != nil {
			log.Fatalf("Shell session error: %v", err)
		}
	}
}
```

Add `waitForProxyInterrupt` beside `runProxyMode`:

```go
func waitForProxyInterrupt(socksActualPort, httpActualPort int) {
	fmt.Println("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m")
	fmt.Printf("\x1b[1;36m│          🚀  AWG-PROXY SERVER RUNNING IN FG            │\x1b[0m\n")
	fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5 proxy:\x1b[0m     socks5://127.0.0.1:%-19d \x1b[1;36m│\x1b[0m\n", socksActualPort)
	fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mHTTP/HTTPS proxy:\x1b[0m  http://127.0.0.1:%-21d \x1b[1;36m│\x1b[0m\n", httpActualPort)
	fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[33mPress Ctrl+C to terminate proxy servers.             \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	fmt.Println("\n[awg-proxy] Shutting down proxy servers...")
}
```

- [ ] **Step 5: Run CLI parser tests**

Run:

```bash
go test ./... -run 'TestParseCLI' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cli.go cli_test.go main.go
git commit -m "refactor: add cli parser for tunnel command"
```

---

### Task 2: Tunnel Config Validation, CIDR Preservation, And Endpoint Rewrite

**Files:**
- Create: `tunnel_config.go`
- Create: `tunnel_config_test.go`

- [ ] **Step 1: Write failing tunnel config tests**

Create `tunnel_config_test.go`:

```go
package main

import (
	"net/netip"
	"strings"
	"testing"
)

func validTunnelConfig() *AWGConfig {
	return &AWGConfig{
		Interface: InterfaceConfig{
			PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			Address:    []string{"10.8.0.2/32", "fd00::2/128"},
			DNS:        []string{"1.1.1.1"},
		},
		Peers: []PeerConfig{{
			PublicKey:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			Endpoint:   "vpn.example.test:51820",
			AllowedIPs: []string{"0.0.0.0/0"},
		}},
	}
}

func TestValidateTunnelConfigPreservesIPv4CIDR(t *testing.T) {
	cfg := validTunnelConfig()
	result, err := ValidateTunnelConfig(cfg)
	if err != nil {
		t.Fatalf("ValidateTunnelConfig returned error: %v", err)
	}
	want := netip.MustParsePrefix("10.8.0.2/32")
	if result.InterfaceIPv4 != want {
		t.Fatalf("InterfaceIPv4 = %v, want %v", result.InterfaceIPv4, want)
	}
}

func TestValidateTunnelConfigRejectsMissingEndpoint(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Peers[0].Endpoint = ""
	_, err := ValidateTunnelConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "exactly one peer with Endpoint") {
		t.Fatalf("error = %v, want exactly one peer with Endpoint", err)
	}
}

func TestValidateTunnelConfigRejectsMissingIPv4Address(t *testing.T) {
	cfg := validTunnelConfig()
	cfg.Interface.Address = []string{"fd00::2/128"}
	_, err := ValidateTunnelConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "IPv4 CIDR") {
		t.Fatalf("error = %v, want IPv4 CIDR error", err)
	}
}

func TestCloneConfigWithResolvedEndpointDoesNotMutateOriginal(t *testing.T) {
	cfg := validTunnelConfig()
	resolved := netip.MustParseAddrPort("203.0.113.10:51820")
	clone := CloneConfigWithResolvedEndpoint(cfg, resolved)
	if clone.Peers[0].Endpoint != "203.0.113.10:51820" {
		t.Fatalf("clone endpoint = %q", clone.Peers[0].Endpoint)
	}
	if cfg.Peers[0].Endpoint != "vpn.example.test:51820" {
		t.Fatalf("original endpoint mutated to %q", cfg.Peers[0].Endpoint)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./... -run 'TestValidateTunnelConfig|TestCloneConfigWithResolvedEndpoint' -count=1
```

Expected: FAIL because tunnel config helpers do not exist.

- [ ] **Step 3: Implement tunnel config helpers**

Create `tunnel_config.go`:

```go
package main

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

type TunnelConfig struct {
	InterfaceIPv4 netip.Prefix
	PeerIndex     int
	EndpointHost  string
	EndpointPort  uint16
}

func ValidateTunnelConfig(cfg *AWGConfig) (TunnelConfig, error) {
	var out TunnelConfig
	endpointPeers := 0
	for i, peer := range cfg.Peers {
		if peer.Endpoint != "" {
			endpointPeers++
			out.PeerIndex = i
			host, port, err := net.SplitHostPort(peer.Endpoint)
			if err != nil {
				return out, fmt.Errorf("invalid peer Endpoint %q: %w", peer.Endpoint, err)
			}
			portNum, err := strconv.Atoi(port)
			if err != nil || portNum < 1 || portNum > 65535 {
				return out, fmt.Errorf("invalid peer Endpoint port %q", port)
			}
			out.EndpointHost = host
			out.EndpointPort = uint16(portNum)
		}
	}
	if endpointPeers != 1 {
		return out, fmt.Errorf("tunnel mode requires exactly one peer with Endpoint, found %d", endpointPeers)
	}

	for _, raw := range cfg.Interface.Address {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return out, fmt.Errorf("invalid Interface Address CIDR %q: %w", raw, err)
		}
		if prefix.Addr().Is4() {
			out.InterfaceIPv4 = prefix
			return out, nil
		}
	}
	return out, fmt.Errorf("tunnel mode requires at least one IPv4 CIDR in Interface Address")
}

func ResolveEndpointIPv4(host string, port uint16, lookup func(string) ([]netip.Addr, error)) (netip.AddrPort, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !addr.Is4() {
			return netip.AddrPort{}, fmt.Errorf("resolved endpoint is not IPv4: %s", addr)
		}
		return netip.AddrPortFrom(addr, port), nil
	}
	addrs, err := lookup(host)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("failed to resolve endpoint host %q: %w", host, err)
	}
	for _, addr := range addrs {
		if addr.Is4() {
			return netip.AddrPortFrom(addr, port), nil
		}
	}
	return netip.AddrPort{}, fmt.Errorf("resolved endpoint is not IPv4: %s", host)
}

func CloneConfigWithResolvedEndpoint(cfg *AWGConfig, endpoint netip.AddrPort) *AWGConfig {
	clone := *cfg
	clone.Interface = cfg.Interface
	clone.Interface.Address = append([]string(nil), cfg.Interface.Address...)
	clone.Interface.DNS = append([]string(nil), cfg.Interface.DNS...)
	clone.Peers = append([]PeerConfig(nil), cfg.Peers...)
	for i := range clone.Peers {
		clone.Peers[i].AllowedIPs = append([]string(nil), cfg.Peers[i].AllowedIPs...)
		if clone.Peers[i].Endpoint != "" {
			clone.Peers[i].Endpoint = endpoint.String()
		}
	}
	return &clone
}
```

- [ ] **Step 4: Add endpoint resolution tests**

Append to `tunnel_config_test.go`:

```go
func TestResolveEndpointIPv4RejectsIPv6OnlyResult(t *testing.T) {
	_, err := ResolveEndpointIPv4("vpn.example.test", 51820, func(string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("2001:db8::1")}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not IPv4") {
		t.Fatalf("error = %v, want not IPv4", err)
	}
}

func TestResolveEndpointIPv4SelectsFirstIPv4(t *testing.T) {
	got, err := ResolveEndpointIPv4("vpn.example.test", 51820, func(string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("2001:db8::1"),
			netip.MustParseAddr("203.0.113.44"),
			netip.MustParseAddr("203.0.113.45"),
		}, nil
	})
	if err != nil {
		t.Fatalf("ResolveEndpointIPv4 returned error: %v", err)
	}
	want := netip.MustParseAddrPort("203.0.113.44:51820")
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 5: Run tunnel config tests**

Run:

```bash
go test ./... -run 'TestValidateTunnelConfig|TestCloneConfigWithResolvedEndpoint|TestResolveEndpointIPv4' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add tunnel_config.go tunnel_config_test.go
git commit -m "feat: validate tunnel config"
```

---

### Task 3: Cleanup Stack

**Files:**
- Create: `cleanup.go`
- Create: `cleanup_test.go`

- [ ] **Step 1: Write failing cleanup tests**

Create `cleanup_test.go`:

```go
package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestCleanupStackRunsInReverseOrder(t *testing.T) {
	var calls []string
	c := NewCleanupStack()
	c.Add("first", func() error { calls = append(calls, "first"); return nil })
	c.Add("second", func() error { calls = append(calls, "second"); return nil })
	if err := c.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := []string{"second", "first"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestCleanupStackIsIdempotent(t *testing.T) {
	calls := 0
	c := NewCleanupStack()
	c.Add("once", func() error { calls++; return nil })
	_ = c.Run()
	_ = c.Run()
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestCleanupStackAggregatesErrors(t *testing.T) {
	c := NewCleanupStack()
	c.Add("bad", func() error { return errors.New("route delete failed") })
	err := c.Run()
	if err == nil {
		t.Fatalf("Run returned nil error")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./... -run 'TestCleanupStack' -count=1
```

Expected: FAIL because cleanup stack does not exist.

- [ ] **Step 3: Implement cleanup stack**

Create `cleanup.go`:

```go
package main

import (
	"errors"
	"fmt"
	"sync"
)

type cleanupAction struct {
	name string
	fn   func() error
}

type CleanupStack struct {
	mu      sync.Mutex
	actions []cleanupAction
	ran     bool
}

func NewCleanupStack() *CleanupStack {
	return &CleanupStack{}
}

func (c *CleanupStack) Add(name string, fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ran {
		return
	}
	c.actions = append(c.actions, cleanupAction{name: name, fn: fn})
}

func (c *CleanupStack) Run() error {
	c.mu.Lock()
	if c.ran {
		c.mu.Unlock()
		return nil
	}
	c.ran = true
	actions := append([]cleanupAction(nil), c.actions...)
	c.mu.Unlock()

	var errs []error
	for i := len(actions) - 1; i >= 0; i-- {
		if err := actions[i].fn(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", actions[i].name, err))
		}
	}
	return errors.Join(errs...)
}
```

- [ ] **Step 4: Run cleanup tests**

Run:

```bash
go test ./... -run 'TestCleanupStack' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cleanup.go cleanup_test.go
git commit -m "feat: add tunnel cleanup stack"
```

---

### Task 4: Route Policy And Command Runner

**Files:**
- Create: `route_policy.go`
- Create: `route_policy_test.go`
- Create: `command_runner.go`
- Create: `command_runner_test.go`

- [ ] **Step 1: Write failing route policy tests**

Create `route_policy_test.go`:

```go
package main

import (
	"net/netip"
	"testing"
)

func TestBuildFullTunnelRoutePlan(t *testing.T) {
	endpoint := netip.MustParseAddrPort("203.0.113.10:51820")
	plan := BuildFullTunnelRoutePlan(endpoint)
	want := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/1"),
		netip.MustParsePrefix("128.0.0.0/1"),
	}
	if len(plan.TunnelCIDRs) != len(want) {
		t.Fatalf("TunnelCIDRs len = %d", len(plan.TunnelCIDRs))
	}
	for i := range want {
		if plan.TunnelCIDRs[i] != want[i] {
			t.Fatalf("TunnelCIDRs[%d] = %v, want %v", i, plan.TunnelCIDRs[i], want[i])
		}
	}
	if plan.EndpointBypass != endpoint {
		t.Fatalf("EndpointBypass = %v, want %v", plan.EndpointBypass, endpoint)
	}
}
```

- [ ] **Step 2: Write failing command runner tests**

Create `command_runner_test.go`:

```go
package main

import (
	"reflect"
	"testing"
)

func TestDryRunRunnerRecordsCommands(t *testing.T) {
	r := NewDryRunRunner()
	err := r.Run("route", "add", "0.0.0.0/1", "-interface", "utun7")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := []string{"route add 0.0.0.0/1 -interface utun7"}
	if !reflect.DeepEqual(r.Commands(), want) {
		t.Fatalf("Commands = %v, want %v", r.Commands(), want)
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
go test ./... -run 'TestBuildFullTunnelRoutePlan|TestDryRunRunner' -count=1
```

Expected: FAIL because route policy and command runner do not exist.

- [ ] **Step 4: Implement route policy**

Create `route_policy.go`:

```go
package main

import "net/netip"

type RoutePlan struct {
	TunnelCIDRs    []netip.Prefix
	EndpointBypass netip.AddrPort
}

func BuildFullTunnelRoutePlan(endpoint netip.AddrPort) RoutePlan {
	return RoutePlan{
		TunnelCIDRs: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
		EndpointBypass: endpoint,
	}
}
```

- [ ] **Step 5: Implement command runner**

Create `command_runner.go`:

```go
package main

import (
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (ExecRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type DryRunRunner struct {
	commands []string
}

func NewDryRunRunner() *DryRunRunner {
	return &DryRunRunner{}
}

func (r *DryRunRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, strings.Join(append([]string{name}, args...), " "))
	return nil
}

func (r *DryRunRunner) Output(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, strings.Join(append([]string{name}, args...), " "))
	return nil, nil
}

func (r *DryRunRunner) Commands() []string {
	return append([]string(nil), r.commands...)
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./... -run 'TestBuildFullTunnelRoutePlan|TestDryRunRunner' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add route_policy.go route_policy_test.go command_runner.go command_runner_test.go
git commit -m "feat: add tunnel route policy"
```

---

### Task 5: Route Managers

**Files:**
- Create: `route_manager.go`
- Create: `route_manager_builders.go`
- Create: `route_manager_darwin.go`
- Create: `route_manager_linux.go`
- Create: `route_manager_test.go`

- [ ] **Step 1: Write route command builder tests**

Create `route_manager_test.go`:

```go
package main

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestDarwinTunAddressCommandsFor32(t *testing.T) {
	got := darwinConfigureAddressCommand("utun7", netip.MustParsePrefix("10.8.0.2/32"), 1420)
	want := []string{"ifconfig", "utun7", "inet", "10.8.0.2", "10.8.0.2", "mtu", "1420", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDarwinTunAddressCommandsFor24(t *testing.T) {
	got := darwinConfigureAddressCommand("utun7", netip.MustParsePrefix("10.8.0.2/24"), 1420)
	want := []string{"ifconfig", "utun7", "inet", "10.8.0.2", "netmask", "255.255.255.0", "mtu", "1420", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLinuxTunAddressCommands(t *testing.T) {
	got := linuxConfigureAddressCommands("awg0", netip.MustParsePrefix("10.8.0.2/32"), 1420)
	want := [][]string{
		{"ip", "addr", "add", "10.8.0.2/32", "dev", "awg0"},
		{"ip", "link", "set", "dev", "awg0", "mtu", "1420", "up"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./... -run 'TestDarwinTunAddressCommands|TestLinuxTunAddressCommands' -count=1
```

Expected: FAIL because command builders do not exist.

- [ ] **Step 3: Add shared route manager types**

Create `route_manager.go`:

```go
package main

import (
	"context"
	"net/netip"
)

type DefaultRoute struct {
	Gateway netip.Addr
	Device  string
}

type RouteManager interface {
	ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error
	DefaultRoute(ctx context.Context) (DefaultRoute, error)
	Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error
}
```

- [ ] **Step 4: Add untagged route command builders**

Create `route_manager_builders.go`:

```go
package main

import (
	"fmt"
	"net/netip"
	"strconv"
)

func darwinConfigureAddressCommand(ifName string, addr netip.Prefix, mtu int) []string {
	if addr.Bits() == 32 {
		return []string{"ifconfig", ifName, "inet", addr.Addr().String(), addr.Addr().String(), "mtu", strconv.Itoa(mtu), "up"}
	}
	return []string{"ifconfig", ifName, "inet", addr.Addr().String(), "netmask", ipv4Netmask(addr.Bits()), "mtu", strconv.Itoa(mtu), "up"}
}

func linuxConfigureAddressCommands(ifName string, addr netip.Prefix, mtu int) [][]string {
	return [][]string{
		{"ip", "addr", "add", addr.String(), "dev", ifName},
		{"ip", "link", "set", "dev", ifName, "mtu", strconv.Itoa(mtu), "up"},
	}
}

func ipv4Netmask(bits int) string {
	if bits < 0 || bits > 32 {
		return "0.0.0.0"
	}
	mask := uint32(0)
	if bits > 0 {
		mask = ^uint32(0) << (32 - bits)
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask))
}
```

- [ ] **Step 5: Add macOS route manager skeleton**

Create `route_manager_darwin.go`:

```go
//go:build darwin

package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

type DarwinRouteManager struct {
	Runner CommandRunner
}

func (m DarwinRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	cmd := darwinConfigureAddressCommand(ifName, addr, mtu)
	return m.Runner.Run(cmd[0], cmd[1:]...)
}

func (m DarwinRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	out, err := m.Runner.Output("route", "-n", "get", "default")
	if err != nil {
		return DefaultRoute{}, err
	}
	return parseDarwinDefaultRoute(string(out))
}

func (m DarwinRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	endpointIP := plan.EndpointBypass.Addr().String()
	gateway := defaultRoute.Gateway.String()
	if err := m.Runner.Run("route", "add", endpointIP, gateway); err != nil {
		return fmt.Errorf("add endpoint bypass route: %w", err)
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		return m.Runner.Run("route", "delete", endpointIP)
	})
	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if err := m.Runner.Run("route", "add", cidrText, "-interface", ifName); err != nil {
			return fmt.Errorf("add full tunnel route %s: %w", cidrText, err)
		}
		cleanup.Add("delete full tunnel route "+cidrText, func() error {
			return m.Runner.Run("route", "delete", cidrText)
		})
	}
	return nil
}
```

Also add `parseDarwinDefaultRoute` in the same file:

```go
func parseDarwinDefaultRoute(out string) (DefaultRoute, error) {
	var route DefaultRoute
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "gateway:" {
			addr, err := netip.ParseAddr(fields[1])
			if err != nil {
				return route, err
			}
			route.Gateway = addr
		}
		if len(fields) == 2 && fields[0] == "interface:" {
			route.Device = fields[1]
		}
	}
	if !route.Gateway.IsValid() || route.Device == "" {
		return route, fmt.Errorf("default route missing gateway or interface")
	}
	return route, nil
}
```

- [ ] **Step 6: Add Linux route manager skeleton**

Create `route_manager_linux.go`:

```go
//go:build linux

package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

type LinuxRouteManager struct {
	Runner CommandRunner
}

func (m LinuxRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	for _, cmd := range linuxConfigureAddressCommands(ifName, addr, mtu) {
		if err := m.Runner.Run(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (m LinuxRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	out, err := m.Runner.Output("ip", "route", "show", "default")
	if err != nil {
		return DefaultRoute{}, err
	}
	return parseLinuxDefaultRoute(string(out))
}

func parseLinuxDefaultRoute(out string) (DefaultRoute, error) {
	fields := strings.Fields(out)
	var route DefaultRoute
	for i := 0; i < len(fields)-1; i++ {
		switch fields[i] {
		case "via":
			addr, err := netip.ParseAddr(fields[i+1])
			if err != nil {
				return route, err
			}
			route.Gateway = addr
		case "dev":
			route.Device = fields[i+1]
		}
	}
	if !route.Gateway.IsValid() || route.Device == "" {
		return route, fmt.Errorf("default route missing gateway or interface")
	}
	return route, nil
}

func (m LinuxRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	endpointIP := plan.EndpointBypass.Addr().String()
	gateway := defaultRoute.Gateway.String()
	if err := m.Runner.Run("ip", "route", "add", endpointIP, "via", gateway, "dev", defaultRoute.Device); err != nil {
		return fmt.Errorf("add endpoint bypass route: %w", err)
	}
	cleanup.Add("delete endpoint bypass route", func() error {
		return m.Runner.Run("ip", "route", "del", endpointIP)
	})
	for _, cidr := range plan.TunnelCIDRs {
		cidrText := cidr.String()
		if err := m.Runner.Run("ip", "route", "add", cidrText, "dev", ifName); err != nil {
			return fmt.Errorf("add full tunnel route %s: %w", cidrText, err)
		}
		cleanup.Add("delete full tunnel route "+cidrText, func() error {
			return m.Runner.Run("ip", "route", "del", cidrText, "dev", ifName)
		})
	}
	return nil
}
```

- [ ] **Step 7: Run route manager tests**

Run:

```bash
go test ./... -run 'TestDarwinTunAddressCommands|TestLinuxTunAddressCommands' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add route_manager.go route_manager_builders.go route_manager_darwin.go route_manager_linux.go route_manager_test.go
git commit -m "feat: add tunnel route managers"
```

---

### Task 6: DNS Managers

**Files:**
- Create: `dns_manager.go`
- Create: `dns_manager_darwin_parse.go`
- Create: `dns_manager_darwin.go`
- Create: `dns_manager_linux.go`
- Create: `dns_manager_darwin_test.go`
- Create: `dns_manager_linux_test.go`

- [ ] **Step 1: Write DNS tests**

Create `dns_manager_linux_test.go`:

```go
//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxDNSManagerWritesAndRestoresRegularResolvConf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 9.9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LinuxDNSManager{ResolvConfPath: path}
	cleanup := NewCleanupStack()
	if err := m.Apply([]string{"1.1.1.1", "8.8.8.8"}, cleanup); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "nameserver 1.1.1.1") || !strings.Contains(string(got), "nameserver 8.8.8.8") {
		t.Fatalf("resolv.conf = %q", string(got))
	}
	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != "nameserver 9.9.9.9\n" {
		t.Fatalf("restored = %q", string(restored))
	}
}

func TestLinuxDNSManagerRejectsSymlinkUnlessNoDNS(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(target, []byte("nameserver 9.9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	m := LinuxDNSManager{ResolvConfPath: link}
	err := m.Apply([]string{"1.1.1.1"}, NewCleanupStack())
	if err == nil || !strings.Contains(err.Error(), "managed or symlink") {
		t.Fatalf("error = %v, want managed or symlink", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./... -run 'TestLinuxDNSManager' -count=1
```

Expected on Linux: FAIL because DNS manager does not exist. Expected on macOS: no matching tests run.

- [ ] **Step 3: Add DNS manager interface**

Create `dns_manager.go`:

```go
package main

type DNSManager interface {
	Apply(servers []string, cleanup *CleanupStack) error
}
```

- [ ] **Step 4: Add Linux DNS manager**

Create `dns_manager_linux.go`:

```go
//go:build linux

package main

import (
	"fmt"
	"os"
	"strings"
)

type LinuxDNSManager struct {
	ResolvConfPath string
}

func (m LinuxDNSManager) Apply(servers []string, cleanup *CleanupStack) error {
	path := m.ResolvConfPath
	if path == "" {
		path = "/etc/resolv.conf"
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is managed or symlink; rerun with --no-dns to accept existing DNS", path)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := make([]string, 0, len(servers))
	for _, server := range servers {
		lines = append(lines, "nameserver "+server)
	}
	next := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(path, next, info.Mode().Perm()); err != nil {
		return err
	}
	cleanup.Add("restore resolv.conf", func() error {
		return os.WriteFile(path, original, info.Mode().Perm())
	})
	return nil
}
```

- [ ] **Step 5: Add macOS DNS manager**

Create `dns_manager_darwin_parse.go`:

```go
package main

import "strings"

type darwinDNSState struct {
	Service string
	Servers []string
	Empty   bool
}

func parseDarwinNetworkServices(out string) []string {
	var services []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services
}

func parseDarwinDNSState(service string, out string) darwinDNSState {
	out = strings.TrimSpace(out)
	if out == "" || strings.Contains(out, "There aren't any DNS Servers") {
		return darwinDNSState{Service: service, Empty: true}
	}
	return darwinDNSState{Service: service, Servers: strings.Fields(out)}
}
```

Create `dns_manager_darwin.go`:

```go
//go:build darwin

package main

import (
	"fmt"
)

type DarwinDNSManager struct {
	Runner CommandRunner
}

func (m DarwinDNSManager) Apply(servers []string, cleanup *CleanupStack) error {
	out, err := m.Runner.Output("networksetup", "-listallnetworkservices")
	if err != nil {
		return err
	}
	services := parseDarwinNetworkServices(string(out))
	states := make([]darwinDNSState, 0, len(services))
	for _, service := range services {
		dnsOut, err := m.Runner.Output("networksetup", "-getdnsservers", service)
		if err != nil {
			return err
		}
		states = append(states, parseDarwinDNSState(service, string(dnsOut)))
		args := append([]string{"-setdnsservers", service}, servers...)
		if err := m.Runner.Run("networksetup", args...); err != nil {
			return fmt.Errorf("set DNS for %s: %w", service, err)
		}
	}
	cleanup.Add("restore macOS DNS", func() error {
		for _, state := range states {
			args := []string{"-setdnsservers", state.Service}
			if state.Empty {
				args = append(args, "Empty")
			} else {
				args = append(args, state.Servers...)
			}
			if err := m.Runner.Run("networksetup", args...); err != nil {
				return err
			}
		}
		return nil
	})
	return nil
}
```

- [ ] **Step 6: Add macOS DNS parser tests**

Create `dns_manager_darwin_test.go`:

```go
package main

import "testing"

func TestParseDarwinNetworkServicesSkipsDisabledServices(t *testing.T) {
	got := parseDarwinNetworkServices("An asterisk (*) denotes disabled services.\nWi-Fi\n*Thunderbolt Bridge\nUSB 10/100/1000 LAN\n")
	want := []string{"Wi-Fi", "USB 10/100/1000 LAN"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseDarwinDNSStateEmpty(t *testing.T) {
	state := parseDarwinDNSState("Wi-Fi", "There aren't any DNS Servers set on Wi-Fi.\n")
	if !state.Empty {
		t.Fatalf("Empty = false, want true")
	}
}
```

- [ ] **Step 7: Run DNS tests**

Run:

```bash
go test ./... -run 'TestLinuxDNSManager|TestDarwin' -count=1
```

Expected: PASS. On macOS, Darwin parser tests run. On Linux, Linux file tests and Darwin parser tests run because parser helpers are pure Go in `dns_manager_darwin_parse.go`.

- [ ] **Step 8: Commit**

```bash
git add dns_manager.go dns_manager_darwin_parse.go dns_manager_darwin.go dns_manager_linux.go dns_manager_darwin_test.go dns_manager_linux_test.go
git commit -m "feat: add tunnel dns managers"
```

---

### Task 7: Native Tunnel Device Factory

**Files:**
- Create: `tunnel_device.go`
- Create: `tunnel_device_test.go`

- [ ] **Step 1: Write UAPI endpoint rewrite test**

Create `tunnel_device_test.go`:

```go
package main

import (
	"net/netip"
	"strings"
	"testing"
)

func TestBuildResolvedTunnelUAPI(t *testing.T) {
	cfg := validTunnelConfig()
	resolved := netip.MustParseAddrPort("203.0.113.10:51820")
	uapi, err := BuildResolvedTunnelUAPI(cfg, resolved)
	if err != nil {
		t.Fatalf("BuildResolvedTunnelUAPI returned error: %v", err)
	}
	if !strings.Contains(uapi, "endpoint=203.0.113.10:51820") {
		t.Fatalf("uapi missing resolved endpoint:\n%s", uapi)
	}
	if strings.Contains(uapi, "endpoint=vpn.example.test:51820") {
		t.Fatalf("uapi contains original hostname:\n%s", uapi)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./... -run 'TestBuildResolvedTunnelUAPI' -count=1
```

Expected: FAIL because `BuildResolvedTunnelUAPI` does not exist.

- [ ] **Step 3: Implement device interfaces and UAPI helper**

Create `tunnel_device.go`:

```go
package main

import (
	"fmt"
	"net/netip"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun"
)

type TunnelDevice interface {
	Name() string
	Up(uapi string) error
	Close() error
}

type TunnelDeviceFactory interface {
	Create(name string, mtu int, verbose bool) (TunnelDevice, error)
}

type AWGTunnelDeviceFactory struct{}

type AWGTunnelDevice struct {
	name string
	tun  tun.Device
	dev  *device.Device
}

func (AWGTunnelDeviceFactory) Create(name string, mtu int, verbose bool) (TunnelDevice, error) {
	tunDev, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return nil, err
	}
	actualName, err := tunDev.Name()
	if err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	level := device.LogLevelSilent
	if verbose {
		level = device.LogLevelVerbose
	}
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(level, "[AWG] "))
	return &AWGTunnelDevice{name: actualName, tun: tunDev, dev: dev}, nil
}

func (d *AWGTunnelDevice) Name() string {
	return d.name
}

func (d *AWGTunnelDevice) Up(uapi string) error {
	if err := d.dev.IpcSet(uapi); err != nil {
		return fmt.Errorf("apply UAPI: %w", err)
	}
	if err := d.dev.Up(); err != nil {
		return fmt.Errorf("bring device up: %w", err)
	}
	return nil
}

func (d *AWGTunnelDevice) Close() error {
	d.dev.Close()
	return d.tun.Close()
}

func BuildResolvedTunnelUAPI(cfg *AWGConfig, endpoint netip.AddrPort) (string, error) {
	return CloneConfigWithResolvedEndpoint(cfg, endpoint).ToUAPI()
}
```

- [ ] **Step 4: Run device tests**

Run:

```bash
go test ./... -run 'TestBuildResolvedTunnelUAPI' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tunnel_device.go tunnel_device_test.go
git commit -m "feat: add native tunnel device factory"
```

---

### Task 8: Tunnel Lifecycle Orchestration

**Files:**
- Create: `tunnel.go`
- Create: `tunnel_test.go`

- [ ] **Step 1: Write lifecycle test with fakes**

Create `tunnel_test.go`:

```go
package main

import (
	"context"
	"net/netip"
	"reflect"
	"testing"
)

type fakeTunnelDevice struct {
	name   string
	closed bool
	uapi   string
}

func (d *fakeTunnelDevice) Name() string { return d.name }
func (d *fakeTunnelDevice) Up(uapi string) error { d.uapi = uapi; return nil }
func (d *fakeTunnelDevice) Close() error { d.closed = true; return nil }

type fakeDeviceFactory struct{ dev *fakeTunnelDevice }

func (f fakeDeviceFactory) Create(name string, mtu int, verbose bool) (TunnelDevice, error) {
	return f.dev, nil
}

type fakeRouteManager struct{ calls []string }

func (m *fakeRouteManager) ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error {
	m.calls = append(m.calls, "configure:"+ifName+":"+addr.String())
	return nil
}
func (m *fakeRouteManager) DefaultRoute(ctx context.Context) (DefaultRoute, error) {
	return DefaultRoute{Gateway: netip.MustParseAddr("192.0.2.1"), Device: "en0"}, nil
}
func (m *fakeRouteManager) Apply(ctx context.Context, ifName string, plan RoutePlan, defaultRoute DefaultRoute, cleanup *CleanupStack) error {
	m.calls = append(m.calls, "routes:"+ifName)
	cleanup.Add("routes", func() error {
		m.calls = append(m.calls, "cleanup-routes")
		return nil
	})
	return nil
}

type fakeDNSManager struct{ calls []string }

func (m *fakeDNSManager) Apply(servers []string, cleanup *CleanupStack) error {
	m.calls = append(m.calls, "dns")
	cleanup.Add("dns", func() error {
		m.calls = append(m.calls, "cleanup-dns")
		return nil
	})
	return nil
}

func TestRunTunnelSetupThenCleanup(t *testing.T) {
	dev := &fakeTunnelDevice{name: "utun99"}
	routes := &fakeRouteManager{}
	dns := &fakeDNSManager{}
	opts := TunnelOptions{ConfigPath: "amnezia.conf"}
	deps := TunnelDeps{
		DeviceFactory: fakeDeviceFactory{dev: dev},
		RouteManager:  routes,
		DNSManager:    dns,
		Lookup: func(string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
		},
		Wait: func(context.Context) error {
			return nil
		},
	}
	if err := RunTunnelWithDeps(context.Background(), validTunnelConfig(), opts, deps); err != nil {
		t.Fatalf("RunTunnelWithDeps returned error: %v", err)
	}
	if !dev.closed {
		t.Fatalf("device was not closed")
	}
	wantRouteCalls := []string{"configure:utun99:10.8.0.2/32", "routes:utun99", "cleanup-routes"}
	if !reflect.DeepEqual(routes.calls, wantRouteCalls) {
		t.Fatalf("route calls = %v, want %v", routes.calls, wantRouteCalls)
	}
	wantDNSCalls := []string{"dns", "cleanup-dns"}
	if !reflect.DeepEqual(dns.calls, wantDNSCalls) {
		t.Fatalf("dns calls = %v, want %v", dns.calls, wantDNSCalls)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./... -run 'TestRunTunnelSetupThenCleanup' -count=1
```

Expected: FAIL because `RunTunnelWithDeps` and `TunnelDeps` do not exist.

- [ ] **Step 3: Implement tunnel lifecycle**

Create `tunnel.go`:

```go
package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
)

type TunnelDeps struct {
	DeviceFactory TunnelDeviceFactory
	RouteManager  RouteManager
	DNSManager    DNSManager
	Lookup        func(string) ([]netip.Addr, error)
	Wait          func(context.Context) error
}

func RunTunnel(cfg *AWGConfig, opts TunnelOptions) error {
	ctx := context.Background()
	runner := CommandRunner(ExecRunner{})
	if opts.DryRun {
		runner = NewDryRunRunner()
	}
	deps := TunnelDeps{
		DeviceFactory: AWGTunnelDeviceFactory{},
		RouteManager:  NewPlatformRouteManager(runner),
		DNSManager:    NewPlatformDNSManager(runner),
		Lookup:        netipLookup,
		Wait:          waitForSignal,
	}
	return RunTunnelWithDeps(ctx, cfg, opts, deps)
}

func RunTunnelWithDeps(ctx context.Context, cfg *AWGConfig, opts TunnelOptions, deps TunnelDeps) error {
	tcfg, err := ValidateTunnelConfig(cfg)
	if err != nil {
		return err
	}
	endpoint, err := ResolveEndpointIPv4(tcfg.EndpointHost, tcfg.EndpointPort, deps.Lookup)
	if err != nil {
		return err
	}
	mtu := cfg.Interface.MTU
	if mtu <= 0 {
		mtu = 1420
	}
	cleanup := NewCleanupStack()
	defer func() {
		if err := cleanup.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[awg-proxy] cleanup errors: %v\n", err)
		}
	}()

	dev, err := deps.DeviceFactory.Create("awgproxy0", mtu, opts.Verbose)
	if err != nil {
		return err
	}
	cleanup.Add("close tunnel device", dev.Close)

	if err := deps.RouteManager.ConfigureInterface(ctx, dev.Name(), tcfg.InterfaceIPv4, mtu); err != nil {
		return err
	}

	uapi, err := BuildResolvedTunnelUAPI(cfg, endpoint)
	if err != nil {
		return err
	}
	if err := dev.Up(uapi); err != nil {
		return err
	}

	defaultRoute, err := deps.RouteManager.DefaultRoute(ctx)
	if err != nil {
		return err
	}
	plan := BuildFullTunnelRoutePlan(endpoint)
	if err := deps.RouteManager.Apply(ctx, dev.Name(), plan, defaultRoute, cleanup); err != nil {
		return err
	}

	if !opts.NoDNS {
		if err := deps.DNSManager.Apply(cfg.Interface.DNS, cleanup); err != nil {
			return err
		}
	}

	return deps.Wait(ctx)
}

func waitForSignal(ctx context.Context) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-sigs:
		return nil
	}
}

func netipLookup(host string) ([]netip.Addr, error) {
	addrs, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip", host)
	if err != nil {
		return nil, err
	}
	return addrs, nil
}
```

Add missing import `net` to `tunnel.go`.

- [ ] **Step 4: Add platform factory functions**

Add `platform_darwin.go`:

```go
//go:build darwin

package main

func NewPlatformRouteManager(runner CommandRunner) RouteManager {
	return DarwinRouteManager{Runner: runner}
}

func NewPlatformDNSManager(runner CommandRunner) DNSManager {
	return DarwinDNSManager{Runner: runner}
}
```

Add `platform_linux.go`:

```go
//go:build linux

package main

func NewPlatformRouteManager(runner CommandRunner) RouteManager {
	return LinuxRouteManager{Runner: runner}
}

func NewPlatformDNSManager(runner CommandRunner) DNSManager {
	return LinuxDNSManager{ResolvConfPath: "/etc/resolv.conf"}
}
```

- [ ] **Step 5: Run lifecycle tests**

Run:

```bash
go test ./... -run 'TestRunTunnelSetupThenCleanup' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add tunnel.go tunnel_test.go platform_darwin.go platform_linux.go
git commit -m "feat: orchestrate tunnel lifecycle"
```

---

### Task 9: Build, Docs, And Dry-Run Verification

**Files:**
- Modify: `README.md`
- Modify: `README_RU.md`

- [ ] **Step 1: Update usage text in code**

Modify `printUsage()` in `main.go` to include:

```go
fmt.Println("\x1b[1;36m│\x1b[0m    tunnel  Route system traffic via native TUN          \x1b[1;36m│\x1b[0m")
fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy tunnel -c vpn.conf --dry-run              \x1b[1;36m│\x1b[0m")
```

- [ ] **Step 2: Document English tunnel usage**

Add to `README.md` after persistent proxy server:

```markdown
### Transparent system tunnel

`tunnel` creates a native TUN interface and routes IPv4 system traffic through AmneziaWG. It requires elevated privileges because it changes system routes and DNS.

```bash
sudo ./awg-proxy tunnel -c my_vpn.conf --dry-run
sudo ./awg-proxy tunnel -c my_vpn.conf
sudo ./awg-proxy tunnel -c my_vpn.conf --no-dns
```

The tunnel mode resolves the peer endpoint before changing routes, rewrites the device endpoint to the resolved IPv4 address, adds a host bypass route for that endpoint, then adds `0.0.0.0/1` and `128.0.0.0/1` through the TUN interface. DNS from `[Interface] DNS` is applied by default. Use `--no-dns` only when you accept existing DNS behavior.
```

- [ ] **Step 3: Document Russian tunnel usage**

Add equivalent text to `README_RU.md`:

```markdown
### Прозрачный системный туннель

`tunnel` создаёт настоящий TUN-интерфейс и маршрутизирует IPv4-трафик системы через AmneziaWG. Команда требует повышенных прав, потому что меняет системные маршруты и DNS.

```bash
sudo ./awg-proxy tunnel -c my_vpn.conf --dry-run
sudo ./awg-proxy tunnel -c my_vpn.conf
sudo ./awg-proxy tunnel -c my_vpn.conf --no-dns
```

Режим `tunnel` резолвит endpoint пира до изменения маршрутов, передаёт в устройство уже resolved IPv4 endpoint, добавляет отдельный маршрут в обход туннеля до VPN-сервера, затем добавляет маршруты `0.0.0.0/1` и `128.0.0.0/1` через TUN-интерфейс. DNS из `[Interface] DNS` применяется по умолчанию. Используйте `--no-dns` только если вы сознательно принимаете текущее DNS-поведение системы.
```

- [ ] **Step 4: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Build binary**

Run:

```bash
go build -o awg-proxy .
```

Expected: command exits successfully and updates `./awg-proxy`.

- [ ] **Step 6: Run dry-run smoke test**

Run:

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --dry-run
```

Expected: command prints planned TUN, route, and DNS changes and exits without modifying routes or DNS.

- [ ] **Step 7: Commit**

```bash
git add main.go README.md README_RU.md awg-proxy
git commit -m "docs: document tunnel mode"
```

---

## Manual Privileged Smoke Tests

Run these only after all unit tests pass and the binary builds.

### macOS

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --dry-run
netstat -rn | grep -E '0.0.0.0/1|128.0.0.0/1' || true
sudo ./awg-proxy tunnel -c amnezia.conf
curl https://ipinfo.io/ip
agy --print-timeout 30s -p "Reply with exactly: tunnel-test"
# Press Ctrl+C.
netstat -rn
scutil --dns
```

Expected:

- During tunnel, `curl` reports the VPN exit IP.
- `agy` works without proxy environment variables.
- After `Ctrl+C`, the `/1` routes and endpoint bypass route are gone.
- DNS is restored to the pre-run state.

### Linux

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --dry-run
ip route | grep -E '0.0.0.0/1|128.0.0.0/1' || true
sudo ./awg-proxy tunnel -c amnezia.conf
curl https://ipinfo.io/ip
agy --print-timeout 30s -p "Reply with exactly: tunnel-test"
# Press Ctrl+C.
ip route
cat /etc/resolv.conf
```

Expected:

- During tunnel, `curl` reports the VPN exit IP.
- `agy` works without proxy environment variables.
- After `Ctrl+C`, the `/1` routes and endpoint bypass route are gone.
- DNS is restored when `/etc/resolv.conf` was a regular file.
