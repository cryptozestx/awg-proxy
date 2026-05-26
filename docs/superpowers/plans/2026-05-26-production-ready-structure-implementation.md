# Production-Ready Structure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the Go CLI from one root `main` package into a production-ready `cmd/` + `internal/` structure while preserving CLI behavior, proxy behavior, tunnel behavior, dry-run behavior, and tests.

**Architecture:** The executable becomes a thin `cmd/awg-proxy` entrypoint that delegates to `internal/app`. Runtime responsibilities are split into `config`, `platform`, `awgnet`, `proxy`, `routing`, `dns`, and `tunnel`; factories stay in the package whose concepts they construct. Transparent tunnel order and dry-run substitution remain explicit tested invariants.

**Tech Stack:** Go 1.24, `amneziawg-go`, gVisor `netstack`, `miekg/dns`, standard Go testing, build tags for Darwin/Linux.

---

## Current Baseline

Current branch: `refactor/production-ready-structure`.

Current package layout: all production Go files live in repository root under package `main`.

Baseline test note: `go test ./...` may fail in this sandbox on DNS tests that bind UDP on `127.0.0.1:0` with `operation not permitted`. In a normal local environment, rerun the same command outside the sandbox before claiming the full suite passes.

## Target File Map

- Create: `cmd/awg-proxy/main.go`  
  Thin entrypoint. Calls `app.Run(os.Args)`, prints fatal errors, exits once.

- Create: `internal/app/cli.go`  
  Contains `Options`, `TunnelOptions`, `ParseCLI(args []string) (Options, error)`, and default config resolution.

- Create: `internal/app/usage.go`  
  Contains usage output currently in `printUsage`.

- Create: `internal/app/app.go`  
  Contains `Run(args []string) error`, config loading, command dispatch.

- Create: `internal/app/proxy_mode.go`  
  Orchestrates proxy modes with `config`, `awgnet`, and `proxy`.

- Create: `internal/app/tunnel_mode.go`  
  Orchestrates tunnel mode with `tunnel.Run`.

- Create: `internal/config/awg.go`  
  Contains `AWGConfig`, `InterfaceConfig`, `PeerConfig`, `Parse(path string) (*AWGConfig, error)`, and key conversion helpers.

- Create: `internal/config/uapi.go`  
  Contains `func (c *AWGConfig) ToUAPI() (string, error)`.

- Create: `internal/platform/command_runner.go`  
  Contains `CommandRunner`, `ExecRunner`, `DryRunRunner`, and `CommandString`.

- Create: `internal/awgnet/userspace.go`  
  Contains proxy-mode userspace netstack and AmneziaWG device lifecycle.

- Create: `internal/proxy/socks.go`, `internal/proxy/http.go`, `internal/proxy/runner.go`, `internal/proxy/app_darwin.go`  
  Owns SOCKS5, HTTP, command/shell runner, and macOS app launcher.

- Create: `internal/tunnel/service.go`, `device.go`, `config.go`, `rules.go`, `dryrun.go`, `cleanup.go`  
  Owns transparent tunnel orchestration, native TUN device, tunnel config validation, rules, dry-run substitutes, cleanup stack.

- Create: `internal/routing/manager.go`, `policy.go`, `builders.go`, `platform_darwin.go`, `platform_linux.go`  
  Owns route plans, platform route managers, static/dynamic bypass routes, default route parsing, route command builders.

- Create: `internal/dns/manager.go`, `manager_darwin.go`, `manager_linux.go`, `manager_darwin_parse.go`, `forwarder.go`, `rules.go`  
  Owns DNS managers, DNS forwarder, and DNS-domain rule matching used by the forwarder.

- Modify: `README.md`, `README_RU.md`  
  Replace root build command with `go build -o awg-proxy ./cmd/awg-proxy`.

- Delete from root after moves: all root production `*.go` files.

---

### Task 1: Add Structural Verification Tests

**Files:**
- Create: `internal/structure/structure_test.go`

- [ ] **Step 1: Write failing structure tests**

Create `internal/structure/structure_test.go`:

```go
package structure_test

import (
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

func TestNoRootProductionGoFiles(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("os.ReadDir returned error: %v", err)
	}
	var got []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		got = append(got, name)
	}
	if len(got) != 0 {
		t.Fatalf("root production Go files = %v, want none", got)
	}
}

func TestInternalPlatformHasNoHighLevelImports(t *testing.T) {
	root := repoRoot(t)
	pkg, err := build.ImportDir(filepath.Join(root, "internal", "platform"), 0)
	if err != nil {
		t.Fatalf("build.ImportDir returned error: %v", err)
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "/internal/app") ||
			strings.Contains(imp, "/internal/tunnel") ||
			strings.Contains(imp, "/internal/routing") ||
			strings.Contains(imp, "/internal/dns") {
			t.Fatalf("internal/platform imports high-level package %s", imp)
		}
	}
}

func TestLowerLevelPackagesDoNotImportApp(t *testing.T) {
	root := repoRoot(t)
	packages := []string{"awgnet", "config", "dns", "platform", "proxy", "routing", "tunnel"}
	for _, name := range packages {
		pkg, err := build.ImportDir(filepath.Join(root, "internal", name), 0)
		if err != nil {
			if strings.Contains(err.Error(), "no Go files") {
				continue
			}
			t.Fatalf("build.ImportDir(%s) returned error: %v", name, err)
		}
		if slices.Contains(pkg.Imports, "awg-proxy/internal/app") {
			t.Fatalf("internal/%s imports internal/app", name)
		}
	}
}
```

- [ ] **Step 2: Run the new tests and verify they fail**

Run:

```bash
go test ./internal/structure
```

Expected: FAIL because root production `.go` files still exist and target internal packages do not exist yet.

- [ ] **Step 3: Commit the failing structural tests**

Run:

```bash
git add internal/structure/structure_test.go
git commit -m "test: add structure guardrails"
```

---

### Task 2: Extract `internal/platform`

**Files:**
- Move: `command_runner.go` -> `internal/platform/command_runner.go`
- Move: `command_runner_test.go` -> `internal/platform/command_runner_test.go`
- Modify: `internal/platform/command_runner.go`
- Modify references in route/DNS files that use command runners.

- [ ] **Step 1: Move files**

Run:

```bash
mkdir -p internal/platform
git mv command_runner.go internal/platform/command_runner.go
git mv command_runner_test.go internal/platform/command_runner_test.go
```

- [ ] **Step 2: Update package and exported helper names**

In `internal/platform/command_runner.go`, use this package declaration and exported command formatter:

```go
package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

func (ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

var ErrDryRunOutputUnavailable = errors.New("dry-run output unavailable")

type DryRunRunner struct {
	commands     []string
	outputRunner CommandRunner
}

func NewDryRunRunner() *DryRunRunner {
	return &DryRunRunner{}
}

func NewDryRunRunnerWithOutput(outputRunner CommandRunner) *DryRunRunner {
	return &DryRunRunner{outputRunner: outputRunner}
}

func (r *DryRunRunner) Run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, CommandString(name, args...))
	return nil
}

func (r *DryRunRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, CommandString(name, args...))
	if r.outputRunner != nil {
		return r.outputRunner.Output(ctx, name, args...)
	}
	return nil, ErrDryRunOutputUnavailable
}

func (r *DryRunRunner) RecordDryRun(change string) {
	r.commands = append(r.commands, change)
}

func (r *DryRunRunner) Commands() []string {
	return append([]string(nil), r.commands...)
}

func CommandString(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}
```

- [ ] **Step 3: Update platform tests**

In `internal/platform/command_runner_test.go`, change `package main` to:

```go
package platform
```

Replace any `commandString` references with `CommandString`.

- [ ] **Step 4: Update current root package references**

In root route/DNS/platform files that still live in package `main`, import platform:

```go
import "awg-proxy/internal/platform"
```

Use these replacements:

```go
CommandRunner -> platform.CommandRunner
ExecRunner{} -> platform.ExecRunner{}
NewDryRunRunner() -> platform.NewDryRunRunner()
NewDryRunRunnerWithOutput(...) -> platform.NewDryRunRunnerWithOutput(...)
commandString(...) -> platform.CommandString(...)
DryRunRunner -> platform.DryRunRunner
```

- [ ] **Step 5: Run platform tests**

Run:

```bash
go test ./internal/platform
```

Expected: PASS.

- [ ] **Step 6: Run full compile check**

Run:

```bash
go test ./...
```

Expected in normal environment: existing behavior tests pass except possible sandbox UDP bind failures described in the baseline note.

- [ ] **Step 7: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract platform command runners"
```

---

### Task 3: Extract `internal/config`

**Files:**
- Move: `config.go` -> split into `internal/config/awg.go` and `internal/config/uapi.go`
- Move related config tests when present.
- Modify all references from `AWGConfig` to `config.AWGConfig`.

- [ ] **Step 1: Create config package files**

Run:

```bash
mkdir -p internal/config
git mv config.go internal/config/awg.go
```

In `internal/config/awg.go`, change package:

```go
package config
```

Keep these exported names:

```go
type AWGConfig struct
type InterfaceConfig struct
type PeerConfig struct
func Parse(path string) (*AWGConfig, error)
func (c *AWGConfig) ToUAPI() (string, error)
```

Rename `ParseConfig` to `Parse`.

- [ ] **Step 2: Update config call sites**

In current callers, add:

```go
import "awg-proxy/internal/config"
```

Use these replacements:

```go
AWGConfig -> config.AWGConfig
ParseConfig(path) -> config.Parse(path)
```

Do not move tunnel validation yet; it will continue to compile against `config.AWGConfig`.

- [ ] **Step 3: Run config-focused tests**

Run:

```bash
go test ./...
```

Expected in normal environment: all behavior tests pass except possible sandbox UDP bind failures.

- [ ] **Step 4: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract config package"
```

---

### Task 4: Extract `internal/proxy`

**Files:**
- Move: `proxy_socks.go` -> `internal/proxy/socks.go`
- Move: `proxy_http.go` -> `internal/proxy/http.go`
- Move: `runner.go` -> `internal/proxy/runner.go`
- Move: `app_mac.go` -> `internal/proxy/app_darwin.go`
- No existing proxy-specific test files move in this task.

- [ ] **Step 1: Move proxy files**

Run:

```bash
mkdir -p internal/proxy
git mv proxy_socks.go internal/proxy/socks.go
git mv proxy_http.go internal/proxy/http.go
git mv runner.go internal/proxy/runner.go
git mv app_mac.go internal/proxy/app_darwin.go
```

- [ ] **Step 2: Update package declarations**

At the top of each moved file:

```go
package proxy
```

In `internal/proxy/socks.go`, keep:

```go
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

func NewSOCKS5Server(port int, dialer ContextDialer) (*SOCKS5Server, int, error)
```

In `internal/proxy/http.go`, keep:

```go
func NewHTTPProxyServer(port int, dialer ContextDialer) (*HTTPProxyServer, int, error)
```

In `internal/proxy/runner.go`, keep:

```go
func RunCommand(command []string, socksPort, httpPort int) error
func RunShell(socksPort, httpPort int) error
```

In `internal/proxy/app_darwin.go`, keep:

```go
func ResolveAppPath(appName string) (string, error)
func GetBundleExecutable(appPath string) (string, error)
func RunApp(appTarget string, extraArgs []string, socksPort, httpPort int) error
```

- [ ] **Step 3: Update root `main.go` proxy references while app is still in root**

Import:

```go
import "awg-proxy/internal/proxy"
```

Use:

```go
proxy.NewSOCKS5Server(...)
proxy.NewHTTPProxyServer(...)
proxy.RunCommand(...)
proxy.RunApp(...)
proxy.RunShell(...)
```

- [ ] **Step 4: Run proxy compile check**

Run:

```bash
go test ./internal/proxy ./...
```

Expected in normal environment: proxy package compiles and existing behavior tests pass except possible sandbox UDP bind failures.

- [ ] **Step 5: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract proxy package"
```

---

### Task 5: Extract `internal/awgnet`

**Files:**
- Create: `internal/awgnet/userspace.go`
- Create: `internal/awgnet/userspace_test.go`
- Modify: root `main.go` in this task; Task 9 moves the resulting proxy-mode orchestration into `internal/app/proxy_mode.go`.
- Modify: remove `parseAddresses` from `internal/proxy/socks.go` after moving it.

- [ ] **Step 1: Write tests for proxy-mode userspace setup helpers**

Create `internal/awgnet/userspace_test.go`:

```go
package awgnet

import (
	"net/netip"
	"testing"
)

func TestParseAddressesRejectsMalformedAddress(t *testing.T) {
	_, err := parseAddresses([]string{"not-an-ip"})
	if err == nil {
		t.Fatalf("parseAddresses succeeded, want error")
	}
}

func TestParseAddressesAcceptsCIDRAndPlainIP(t *testing.T) {
	got, err := parseAddresses([]string{"10.8.0.2/32", "1.1.1.1"})
	if err != nil {
		t.Fatalf("parseAddresses returned error: %v", err)
	}
	want := []netip.Addr{netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("1.1.1.1")}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("parseAddresses = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Create userspace package**

Create `internal/awgnet/userspace.go` with:

```go
package awgnet

import (
	"fmt"
	"net/netip"

	"awg-proxy/internal/config"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun/netstack"
)

type Session struct {
	Dialer *netstack.Net
	close  func()
}

func (s *Session) Close() {
	if s != nil && s.close != nil {
		s.close()
	}
}

func Start(cfg *config.AWGConfig, debug bool) (*Session, error) {
	localAddrs, err := parseAddresses(cfg.Interface.Address)
	if err != nil {
		return nil, fmt.Errorf("parse interface addresses: %w", err)
	}
	if len(localAddrs) == 0 {
		return nil, fmt.Errorf("no interface IP addresses defined in [Interface]")
	}

	dnsAddrs, err := parseAddresses(cfg.Interface.DNS)
	if err != nil {
		dnsAddrs = []netip.Addr{netip.MustParseAddr("1.1.1.1")}
	}
	if len(dnsAddrs) == 0 {
		dnsAddrs = []netip.Addr{netip.MustParseAddr("1.1.1.1")}
	}

	mtu := cfg.Interface.MTU
	if mtu <= 0 {
		mtu = 1420
	}

	tunDev, tnet, err := netstack.CreateNetTUN(localAddrs, dnsAddrs, mtu)
	if err != nil {
		return nil, fmt.Errorf("create userspace network stack: %w", err)
	}

	logLevel := device.LogLevelSilent
	if debug {
		logLevel = device.LogLevelVerbose
	}
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(logLevel, "[AWG] "))

	uapiConf, err := cfg.ToUAPI()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("construct UAPI config: %w", err)
	}
	if err := dev.IpcSet(uapiConf); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure AmneziaWG interface keys and obfuscation: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("establish tunnel connection: %w", err)
	}

	return &Session{Dialer: tnet, close: dev.Close}, nil
}

func parseAddresses(addrs []string) ([]netip.Addr, error) {
	result := make([]netip.Addr, 0, len(addrs))
	for _, value := range addrs {
		if prefix, err := netip.ParsePrefix(value); err == nil {
			result = append(result, prefix.Addr())
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, err
		}
		result = append(result, addr)
	}
	return result, nil
}
```

- [ ] **Step 3: Remove old parse helper from proxy package**

Delete `parseAddresses` from `internal/proxy/socks.go`; address parsing now belongs to `internal/awgnet`.

- [ ] **Step 4: Temporarily update current proxy mode orchestration**

In current `main.go`, replace netstack/device setup with:

```go
session, err := awgnet.Start(cfg, opts.Debug)
if err != nil {
	return fmt.Errorf("start userspace AmneziaWG session: %w", err)
}
defer session.Close()

socksServer, socksActualPort, err := proxy.NewSOCKS5Server(opts.SocksPort, session.Dialer)
httpServer, httpActualPort, err := proxy.NewHTTPProxyServer(opts.HTTPPort, session.Dialer)
```

If `runProxyMode` still returns no error, change it to:

```go
func runProxyMode(cfg *config.AWGConfig, opts CLIOptions) error
```

and return errors instead of calling `log.Fatalf`.

- [ ] **Step 5: Run awgnet tests**

Run:

```bash
go test ./internal/awgnet
```

Expected: PASS.

- [ ] **Step 6: Run full compile check**

Run:

```bash
go test ./...
```

Expected in normal environment: existing behavior tests pass except possible sandbox UDP bind failures.

- [ ] **Step 7: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract userspace awg netstack"
```

---

### Task 6: Extract `internal/routing`

**Files:**
- Move: `route_manager.go` -> `internal/routing/manager.go`
- Move: `route_policy.go` -> `internal/routing/policy.go`
- Move: `route_manager_builders.go` -> `internal/routing/builders.go`
- Move: `route_manager_darwin.go` -> `internal/routing/platform_darwin.go`
- Move: `route_manager_linux.go` -> `internal/routing/platform_linux.go`
- Move route tests to `internal/routing/`.
- Modify tunnel/root references to use `routing`.

- [ ] **Step 1: Move routing files and tests**

Run:

```bash
mkdir -p internal/routing
git mv route_manager.go internal/routing/manager.go
git mv route_policy.go internal/routing/policy.go
git mv route_manager_builders.go internal/routing/builders.go
git mv route_manager_darwin.go internal/routing/platform_darwin.go
git mv route_manager_linux.go internal/routing/platform_linux.go
git mv route_manager_test.go internal/routing/manager_test.go
git mv route_policy_test.go internal/routing/policy_test.go
git mv route_manager_dynamic_darwin_test.go internal/routing/dynamic_darwin_test.go
git mv route_manager_dynamic_linux_test.go internal/routing/dynamic_linux_test.go
```

- [ ] **Step 2: Update package and platform imports**

At the top of moved routing files:

```go
package routing
```

In files that use command runners, import:

```go
import "awg-proxy/internal/platform"
```

Use:

```go
Runner platform.CommandRunner
platform.ExecRunner{}
platform.CommandString(...)
```

Keep platform factories in routing:

```go
func NewPlatformManager(runner platform.CommandRunner) Manager
func NewPlatformDynamicBypassRoutes(defaultRoute DefaultRoute) DynamicBypassRoutes
```

Rename interface:

```go
type Manager interface {
	ConfigureInterface(ctx context.Context, ifName string, addr netip.Prefix, mtu int) error
	DefaultRoute(ctx context.Context) (DefaultRoute, error)
	Apply(ctx context.Context, ifName string, plan Plan, defaultRoute DefaultRoute, cleanup Cleanup) error
}
```

To avoid importing `tunnel.CleanupStack`, define the cleanup contract in routing:

```go
type Cleanup interface {
	Add(name string, fn func() error)
}
```

Rename:

```go
RoutePlan -> Plan
BuildFullTunnelRoutePlan -> BuildFullTunnelPlan
BuildTunnelRoutePlan -> BuildTunnelPlan
```

- [ ] **Step 3: Keep tunnel rules dependency out of routing**

Change routing plan builder signature to avoid importing `tunnel`:

```go
type StaticBypassSource interface {
	StaticBypassCIDRs() []netip.Prefix
}

func BuildTunnelPlan(endpoint netip.AddrPort, source StaticBypassSource) Plan
```

Later `tunnel.Rules` will implement:

```go
func (r Rules) StaticBypassCIDRs() []netip.Prefix {
	return append([]netip.Prefix(nil), r.StaticBypass...)
}
```

- [ ] **Step 4: Update tests**

Change routing tests to `package routing`. Replace old names:

```go
RoutePlan -> Plan
BuildTunnelRoutePlan -> BuildTunnelPlan
BuildFullTunnelRoutePlan -> BuildFullTunnelPlan
```

Where tests need static bypasses, use:

```go
type staticBypassRules struct {
	cidrs []netip.Prefix
}

func (r staticBypassRules) StaticBypassCIDRs() []netip.Prefix {
	return append([]netip.Prefix(nil), r.cidrs...)
}
```

- [ ] **Step 5: Update current root/tunnel references**

In files still in root package, import:

```go
import "awg-proxy/internal/routing"
```

Use:

```go
routing.Manager
routing.DefaultRoute
routing.DynamicBypassRoutes
routing.NewPlatformManager(...)
routing.NewPlatformDynamicBypassRoutes(...)
routing.Plan
routing.BuildTunnelPlan(...)
```

- [ ] **Step 6: Run routing tests**

Run:

```bash
go test ./internal/routing
```

Expected: PASS on the current OS for applicable build-tagged files.

- [ ] **Step 7: Run full compile check**

Run:

```bash
go test ./...
```

Expected in normal environment: existing behavior tests pass except possible sandbox UDP bind failures.

- [ ] **Step 8: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract routing package"
```

---

### Task 7: Extract `internal/dns`

**Files:**
- Move: `dns_manager.go` -> `internal/dns/manager.go`
- Move: `dns_manager_darwin.go` -> `internal/dns/manager_darwin.go`
- Move: `dns_manager_darwin_parse.go` -> `internal/dns/manager_darwin_parse.go`
- Move: `dns_manager_linux.go` -> `internal/dns/manager_linux.go`
- Move: `dns_forwarder.go` -> `internal/dns/forwarder.go`
- Move DNS tests to `internal/dns/`.
- Move domain rule matching from `tunnel_rules.go` into `internal/dns/rules.go`.

- [ ] **Step 1: Move DNS files and tests**

Run:

```bash
mkdir -p internal/dns
git mv dns_manager.go internal/dns/manager.go
git mv dns_manager_darwin.go internal/dns/manager_darwin.go
git mv dns_manager_darwin_parse.go internal/dns/manager_darwin_parse.go
git mv dns_manager_linux.go internal/dns/manager_linux.go
git mv dns_forwarder.go internal/dns/forwarder.go
git mv dns_manager_darwin_test.go internal/dns/manager_darwin_test.go
git mv dns_manager_darwin_behavior_test.go internal/dns/manager_darwin_behavior_test.go
git mv dns_manager_linux_test.go internal/dns/manager_linux_test.go
git mv dns_forwarder_test.go internal/dns/forwarder_test.go
```

- [ ] **Step 2: Update package declarations and imports**

Use:

```go
package dns
```

In DNS manager files that use command runner, import:

```go
import "awg-proxy/internal/platform"
```

Use:

```go
type Manager interface {
	Apply(ctx context.Context, servers []string, cleanup Cleanup) error
}

type Cleanup interface {
	Add(name string, fn func() error)
}

type DarwinManager struct {
	Runner platform.CommandRunner
}
```

Add platform DNS factory behind build tags:

```go
func NewPlatformManager(runner platform.CommandRunner) Manager {
	return DarwinManager{Runner: runner}
}
```

and Linux equivalent:

```go
func NewPlatformManager(platform.CommandRunner) Manager {
	return LinuxManager{}
}
```

- [ ] **Step 3: Define DNS-local rule and dynamic-route contracts**

Create `internal/dns/rules.go`:

```go
package dns

import (
	"context"
	"net/netip"
	"strings"
	"time"
)

type DomainRule struct {
	Pattern string
}

func (r DomainRule) Matches(host string) bool {
	return matchDomainGlob(normalizeDomainPattern(r.Pattern), normalizeDomainPattern(host))
}

type DynamicRouteAdder interface {
	AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error
}

type DynamicRoutes interface {
	DynamicRouteAdder
	Close() error
}

func normalizeDomainPattern(pattern string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(pattern)), ".")
}
```

Cut `matchDomainGlob`, `validDomainParts`, and `matchDomainParts` from `tunnel_rules.go` and paste those three functions into `internal/dns/rules.go` with the same signatures and bodies they have before this task.

- [ ] **Step 4: Update forwarder config to avoid tunnel/routing imports**

In `internal/dns/forwarder.go`, use:

```go
type DomainBypassConfig struct {
	ListenAddr string
	Upstream   string
	Rules      []DomainRule
	Routes     DynamicRouteAdder
}
```

Replace `TunnelRules` parameters with `[]DomainRule`.

Use:

```go
func domainRulesMatch(rules []DomainRule, host string) bool
```

- [ ] **Step 5: Update DNS tests**

Change moved tests to `package dns`. Replace fake route type so it implements:

```go
func (r *fakeDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error
```

Replace `TunnelRules{DomainRules: ...}` with:

```go
[]DomainRule{{Pattern: "*.delimobil.*"}}
```

- [ ] **Step 6: Update current root/tunnel references**

In current tunnel code still in root, import:

```go
import dnsruntime "awg-proxy/internal/dns"
```

Use:

```go
dnsruntime.Manager
dnsruntime.DomainBypassRuntime
dnsruntime.DomainBypassConfig
dnsruntime.DNSAnswer
dnsruntime.NewDomainBypassRuntime
dnsruntime.NewPlatformManager(...)
```

Convert tunnel rules for DNS runtime:

```go
func (r TunnelRules) DNSDomainRules() []dnsruntime.DomainRule {
	out := make([]dnsruntime.DomainRule, 0, len(r.DomainRules))
	for _, rule := range r.DomainRules {
		out = append(out, dnsruntime.DomainRule{Pattern: rule.Pattern})
	}
	return out
}
```

- [ ] **Step 7: Run DNS tests**

Run:

```bash
go test ./internal/dns
```

Expected in normal environment: PASS. In sandbox, tests that bind UDP may fail with `operation not permitted`.

- [ ] **Step 8: Run full compile check**

Run:

```bash
go test ./...
```

Expected in normal environment: existing behavior tests pass.

- [ ] **Step 9: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract dns package"
```

---

### Task 8: Extract `internal/tunnel`

**Files:**
- Move: `cleanup.go` -> `internal/tunnel/cleanup.go`
- Move: `tunnel.go` -> split into `internal/tunnel/service.go` and `internal/tunnel/dryrun.go`
- Move: `tunnel_device.go` -> `internal/tunnel/device.go`
- Move: `tunnel_config.go` -> `internal/tunnel/config.go`
- Move: `tunnel_rules.go` -> `internal/tunnel/rules.go`
- Move tunnel tests to `internal/tunnel/`.
- Modify package imports and type names.

- [ ] **Step 1: Move tunnel files and tests**

Run:

```bash
mkdir -p internal/tunnel
git mv cleanup.go internal/tunnel/cleanup.go
git mv cleanup_test.go internal/tunnel/cleanup_test.go
git mv tunnel.go internal/tunnel/service.go
git mv tunnel_test.go internal/tunnel/service_test.go
git mv tunnel_device.go internal/tunnel/device.go
git mv tunnel_device_test.go internal/tunnel/device_test.go
git mv tunnel_config.go internal/tunnel/config.go
git mv tunnel_config_test.go internal/tunnel/config_test.go
git mv tunnel_rules.go internal/tunnel/rules.go
git mv tunnel_rules_test.go internal/tunnel/rules_test.go
```

- [ ] **Step 2: Update package declarations and imports**

Use:

```go
package tunnel
```

Import:

```go
import (
	"awg-proxy/internal/config"
	dnsruntime "awg-proxy/internal/dns"
	"awg-proxy/internal/platform"
	"awg-proxy/internal/routing"
)
```

Use these public names:

```go
type Options struct {
	ConfigPath string
	RulesPath  string
	DryRun     bool
	NoDNS      bool
	Verbose    bool
}

type Deps struct {
	DeviceFactory        DeviceFactory
	RouteManager         routing.Manager
	DNSManager           dnsruntime.Manager
	DomainRuntimeFactory func() dnsruntime.DomainBypassRuntime
	DynamicRoutesFactory routing.DynamicBypassRouteFactory
	Lookup               func(string) ([]netip.Addr, error)
	Wait                 func(context.Context) error
}

func Run(cfg *config.AWGConfig, opts Options) error
func RunWithDeps(ctx context.Context, cfg *config.AWGConfig, opts Options, deps Deps) error
```

- [ ] **Step 3: Preserve tunnel execution order in code**

In `internal/tunnel/service.go`, keep this order:

```go
tcfg, err := ValidateConfig(cfg)
endpoint, err := ResolveEndpointIPv4(tcfg.EndpointHost, tcfg.EndpointPort, deps.Lookup)
dnsServers, err := tunnelDNSServers(cfg.Interface.DNS)
rules, err := LoadRules(opts.RulesPath)
if opts.NoDNS && rules.HasDomainRules() { return error }
cleanup := NewCleanupStack()
dev, err := deps.DeviceFactory.Create(defaultName(), mtu, opts.Verbose)
err = deps.RouteManager.ConfigureInterface(...)
uapi, err := BuildResolvedUAPI(cfg, endpoint)
err = dev.Up(uapi)
defaultRoute, err := deps.RouteManager.DefaultRoute(ctx)
plan := routing.BuildTunnelPlan(endpoint, rules)
err = deps.RouteManager.Apply(ctx, dev.Name(), plan, defaultRoute, cleanup)
start domain runtime before deps.DNSManager.Apply(...)
return deps.Wait(ctx)
```

Use exact existing behavior for cleanup:

```go
defer func() {
	if err := cleanup.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup failed: %v\n", err)
		retErr = errors.Join(retErr, err)
	}
}()
```

- [ ] **Step 4: Implement rules adapters**

In `internal/tunnel/rules.go`, use:

```go
type Rules struct {
	StaticBypass []netip.Prefix
	DomainRules  []dnsruntime.DomainRule
}

func (r Rules) HasDomainRules() bool {
	return len(r.DomainRules) > 0
}

func (r Rules) StaticBypassCIDRs() []netip.Prefix {
	return append([]netip.Prefix(nil), r.StaticBypass...)
}

func (r Rules) DNSDomainRules() []dnsruntime.DomainRule {
	return append([]dnsruntime.DomainRule(nil), r.DomainRules...)
}
```

Keep parser function:

```go
func LoadRules(path string) (Rules, error)
```

- [ ] **Step 5: Update tunnel tests**

Change moved tests to `package tunnel`. Replace old names:

```go
TunnelOptions -> Options
TunnelDeps -> Deps
RunTunnel -> Run
RunTunnelWithDeps -> RunWithDeps
AWGConfig -> config.AWGConfig
DefaultRoute -> routing.DefaultRoute
RoutePlan -> routing.Plan
DomainBypassConfig -> dnsruntime.DomainBypassConfig
DNSAnswer -> dnsruntime.DNSAnswer
```

Keep assertions for:

```go
TestRunTunnelRejectsDomainRulesWithNoDNSBeforeDeviceSetup
TestRunTunnelStartsDomainRuntimeBeforeApplyingDNS
TestRunTunnelDryRunSkipsInjectedDeviceFactory
TestRunTunnelDryRunExitsWithoutWaiting
TestRunTunnelRouteApplyFailureRunsCleanup
TestRunTunnelRejectsEmptyDNSBeforeDeviceCreation
```

- [ ] **Step 6: Run tunnel tests**

Run:

```bash
go test ./internal/tunnel
```

Expected: PASS.

- [ ] **Step 7: Run full compile check**

Run:

```bash
go test ./...
```

Expected in normal environment: existing behavior tests pass except possible sandbox UDP bind failures.

- [ ] **Step 8: Commit**

Run:

```bash
git add .
git commit -m "refactor: extract tunnel package"
```

---

### Task 9: Create `internal/app` and Thin Entrypoint

**Files:**
- Move: `cli.go` -> `internal/app/cli.go`
- Move: `cli_test.go` -> `internal/app/cli_test.go`
- Create: `internal/app/usage.go`
- Create: `internal/app/app.go`
- Create: `internal/app/proxy_mode.go`
- Create: `internal/app/tunnel_mode.go`
- Move: `main.go` -> `cmd/awg-proxy/main.go`

- [ ] **Step 1: Move CLI files and entrypoint**

Run:

```bash
mkdir -p internal/app cmd/awg-proxy
git mv cli.go internal/app/cli.go
git mv cli_test.go internal/app/cli_test.go
git mv main.go cmd/awg-proxy/main.go
```

- [ ] **Step 2: Update `internal/app/cli.go` API**

Use:

```go
package app

import "awg-proxy/internal/tunnel"

type Options struct {
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

func ParseCLI(args []string) (Options, error)
```

Keep default config resolution search order exactly as: explicit path, `amnezia.conf` in the current working directory, `amnezia.conf` in the executable directory, then empty string.

- [ ] **Step 3: Create usage API**

Create `internal/app/usage.go`:

```go
package app

import (
	"fmt"
	"io"
)

const Version = "1.0.0"

func PrintUsage(w io.Writer) {
	fmt.Fprintln(w, "\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m")
	fmt.Fprintf(w, "\x1b[1;36m│          🛠️   AWG-PROXY CLI UTILITY v%-10s         │\x1b[0m\n", Version)
	fmt.Fprintln(w, "\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m  \x1b[1;33mUsage:\x1b[0m                                                \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy <command> [options]                       \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m  \x1b[1;33mCommands:\x1b[0m                                             \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    shell   Start proxies & launch interactive subshell \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m            (default mode if no command specified)      \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    run     Start proxies, run a single command, exit   \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    app     Start proxies, launch specific macOS app,   \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m            keep alive until app is closed              \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    server  Start persistent proxies in foreground      \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    tunnel  Route system traffic via native TUN          \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m  \x1b[1;33mOptions:\x1b[0m                                              \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    -c, --config      Path to AmneziaWG .conf file      \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                      (required)                        \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    -a, --app         macOS App name or path to proxy   \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                      (only for 'app' command)          \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    -s, --socks-port  SOCKS5 port to bind (default: 0,   \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                      which auto-selects a free port)   \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    -h, --http-port   HTTP proxy port to bind (default: \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                      0, which auto-selects a free port)\x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    -d, --debug       Enable verbose connection logging \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    tunnel options:  --rules PATH, --dry-run, --no-dns, \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                      --verbose                         \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m  \x1b[1;33mExamples:\x1b[0m                                             \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy shell -c vpn.conf                         \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy run -c vpn.conf -- curl ipinfo.io/json    \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy app -c vpn.conf -a \"Google Chrome\"        \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy app -c vpn.conf -- Telegram               \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy server -c vpn.conf -s 1080 -h 8080        \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m    awg-proxy tunnel -c vpn.conf --dry-run              \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")
}
```

When moving the existing lines, preserve text for commands, options, and examples.

- [ ] **Step 4: Create application runner**

Create `internal/app/app.go`:

```go
package app

import (
	"fmt"
	"io"

	"awg-proxy/internal/config"
)

type Runtime struct {
	Stdout io.Writer
	Stderr io.Writer
}

func Run(args []string) error {
	return Runtime{Stdout: defaultStdout{}, Stderr: defaultStderr{}}.Run(args)
}

func (r Runtime) Run(args []string) error {
	opts, err := ParseCLI(args)
	if err != nil {
		PrintUsage(r.Stdout)
		return err
	}
	if opts.ConfigPath == "" {
		PrintUsage(r.Stdout)
		return fmt.Errorf("configuration file path is required")
	}
	fmt.Fprintf(r.Stdout, "[awg-proxy] Parsing configuration: %s...\n", opts.ConfigPath)
	cfg, err := config.Parse(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("configuration parse error: %w", err)
	}
	fmt.Fprintln(r.Stdout, "[awg-proxy] Configuration parsed successfully.")
	if opts.Command == "tunnel" {
		return r.runTunnelMode(cfg, opts)
	}
	return r.runProxyMode(cfg, opts)
}
```

Define stdout/stderr adapters:

```go
type defaultStdout struct{}
func (defaultStdout) Write(p []byte) (int, error) { return os.Stdout.Write(p) }

type defaultStderr struct{}
func (defaultStderr) Write(p []byte) (int, error) { return os.Stderr.Write(p) }
```

- [ ] **Step 5: Create proxy mode runner**

Create `internal/app/proxy_mode.go`:

```go
package app

import (
	"fmt"

	"awg-proxy/internal/awgnet"
	"awg-proxy/internal/config"
	"awg-proxy/internal/proxy"
)

func (r Runtime) runProxyMode(cfg *config.AWGConfig, opts Options) error {
	fmt.Fprintln(r.Stdout, "[awg-proxy] Initializing userspace network stack...")
	session, err := awgnet.Start(cfg, opts.Debug)
	if err != nil {
		return err
	}
	defer session.Close()

	socksServer, socksActualPort, err := proxy.NewSOCKS5Server(opts.SocksPort, session.Dialer)
	if err != nil {
		return fmt.Errorf("start SOCKS5 proxy server: %w", err)
	}
	defer socksServer.Close()
	go socksServer.Start()

	httpServer, httpActualPort, err := proxy.NewHTTPProxyServer(opts.HTTPPort, session.Dialer)
	if err != nil {
		return fmt.Errorf("start HTTP proxy server: %w", err)
	}
	defer httpServer.Close()

	switch opts.Command {
	case "server":
		waitForProxyInterrupt(r.Stdout, socksActualPort, httpActualPort)
	case "run":
		if err := proxy.RunCommand(opts.CommandArgs, socksActualPort, httpActualPort); err != nil {
			return fmt.Errorf("command returned exit error: %w", err)
		}
	case "app":
		if err := proxy.RunApp(opts.AppTarget, opts.AppArgs, socksActualPort, httpActualPort); err != nil {
			return fmt.Errorf("app returned exit error: %w", err)
		}
	case "shell":
		if err := proxy.RunShell(socksActualPort, httpActualPort); err != nil {
			return fmt.Errorf("shell session error: %w", err)
		}
	}
	return nil
}
```

Move `waitForProxyInterrupt` into this file and write to `io.Writer`.

- [ ] **Step 6: Create tunnel mode runner**

Create `internal/app/tunnel_mode.go`:

```go
package app

import (
	"awg-proxy/internal/config"
	"awg-proxy/internal/tunnel"
)

func (r Runtime) runTunnelMode(cfg *config.AWGConfig, opts Options) error {
	return tunnel.Run(cfg, opts.Tunnel)
}
```

- [ ] **Step 7: Create thin command entrypoint**

Replace `cmd/awg-proxy/main.go` with:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"awg-proxy/internal/app"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "\x1b[1;31mError: %v\x1b[0m\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 8: Update app tests**

Change moved CLI tests to `package app`. Replace:

```go
parseCLI -> ParseCLI
CLIOptions -> Options
TunnelOptions -> tunnel.Options
```

Add error handling tests in `internal/app/cli_test.go`:

```go
func TestParseCLIRejectsMissingCommand(t *testing.T) {
	_, err := ParseCLI([]string{"awg-proxy"})
	if err == nil || !strings.Contains(err.Error(), "missing command") {
		t.Fatalf("ParseCLI error = %v, want missing command", err)
	}
}

func TestParseCLIRejectsUnknownCommand(t *testing.T) {
	_, err := ParseCLI([]string{"awg-proxy", "bad"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("ParseCLI error = %v, want unknown command", err)
	}
}
```

- [ ] **Step 9: Run app tests and build**

Run:

```bash
go test ./internal/app
go build ./cmd/awg-proxy
```

Expected: PASS.

- [ ] **Step 10: Run full compile check**

Run:

```bash
go test ./...
```

Expected in normal environment: all tests pass except possible sandbox UDP bind failures.

- [ ] **Step 11: Commit**

Run:

```bash
git add .
git commit -m "refactor: add app package and thin entrypoint"
```

---

### Task 10: Update Documentation And Final Guardrails

**Files:**
- Modify: `README.md`
- Modify: `README_RU.md`
- Modify: `internal/structure/structure_test.go` if package import paths need adjustment.

- [ ] **Step 1: Update build instructions**

In `README.md` and `README_RU.md`, replace root build command:

```bash
go build -o awg-proxy .
```

with:

```bash
go build -o awg-proxy ./cmd/awg-proxy
```

- [ ] **Step 2: Run root production file check**

Run:

```bash
find . -maxdepth 1 -name '*.go' -print
```

Expected: no output.

- [ ] **Step 3: Run package list check**

Run:

```bash
go list ./...
```

Expected output includes packages under:

```text
awg-proxy/cmd/awg-proxy
awg-proxy/internal/app
awg-proxy/internal/awgnet
awg-proxy/internal/config
awg-proxy/internal/dns
awg-proxy/internal/platform
awg-proxy/internal/proxy
awg-proxy/internal/routing
awg-proxy/internal/tunnel
```

- [ ] **Step 4: Run final tests**

Run:

```bash
go test ./...
go vet ./...
go build -o awg-proxy ./cmd/awg-proxy
```

Expected in normal environment: all commands pass. If sandbox blocks UDP DNS tests, rerun `go test ./...` with normal permissions and document the sandbox-specific failure before final status.

- [ ] **Step 5: Commit**

Run:

```bash
git add README.md README_RU.md internal/structure/structure_test.go
git commit -m "docs: update build path for cmd layout"
```

---

## Self-Review Checklist

- Spec coverage: package split, exit ownership, `internal/awgnet`, tunnel ordering, dry-run invariants, platform factory ownership, DNS/routing/tunnel boundary, docs, and quality gates each have explicit tasks.
- Placeholder scan: this plan contains no red-flag markers from the writing-plans checklist.
- Type consistency: planned public names are `app.Options`, `tunnel.Options`, `config.AWGConfig`, `routing.Manager`, `routing.Plan`, `dns.Manager`, `platform.CommandRunner`.
- Regression focus: the plan preserves existing tests and adds structure guardrails plus CLI error tests.
- Verification caveat: current sandbox can block UDP bind tests; final completion requires a normal-environment `go test ./...` or a clear note that only sandbox UDP bind failed.
