# Production-Ready Project Structure Design

Date: 2026-05-26
Branch: refactor/production-ready-structure

## Goal

Reorganize the project from a single root-level Go package into a production-ready package structure with clear layers and responsibility boundaries.

The refactor must preserve the existing CLI contract and runtime behavior for:

- `shell`
- `run`
- `app`
- `server`
- `tunnel`
- tunnel dry-run
- tunnel bypass rules
- DNS behavior
- SOCKS5 and HTTP proxy behavior

The chosen refactor depth is moderate: internal APIs and type names may change when it improves package boundaries, but user-facing CLI behavior must remain compatible.

## Target Structure

```text
cmd/awg-proxy/
  main.go

internal/
  app/
    cli.go
    usage.go
    proxy_mode.go
    tunnel_mode.go

  config/
    awg.go
    awg_uapi.go

  proxy/
    socks.go
    http.go
    runner.go
    app_darwin.go

  awgnet/
    userspace.go

  tunnel/
    service.go
    device.go
    config.go
    rules.go
    dryrun.go
    cleanup.go

  routing/
    manager.go
    policy.go
    dynamic.go
    platform_darwin.go
    platform_linux.go
    builders.go

  dns/
    manager.go
    manager_darwin.go
    manager_linux.go
    forwarder.go

  platform/
    command_runner.go
```

## Package Responsibilities

### `cmd/awg-proxy`

Contains only the executable entrypoint.

`main.go` should delegate to `internal/app` and translate the returned error into process output and exit code. It must not contain CLI parsing, tunnel setup, proxy setup, route management, DNS management, or config parsing logic.

The entrypoint is the only package that may call `os.Exit`. Runtime packages should return errors. The migration must preserve the current user-facing error/output shape through `app.Run` where behavior is covered by existing flows or new tests. Lower-level packages must not introduce new `log.Fatalf` calls. Existing `log.Fatalf` calls should be eliminated while moving code into packages.

### `internal/app`

Owns application-level orchestration:

- CLI parsing.
- Usage/help output.
- Command dispatch for `shell`, `run`, `app`, `server`, and `tunnel`.
- Proxy mode startup.
- Tunnel mode startup.

This package may compose lower-level packages, but it must not implement platform route commands, DNS mutation, SOCKS protocol handling, HTTP proxy handling, TUN setup, or AmneziaWG config parsing.

`app.Run(args []string) error` is the top-level application API. It owns user-facing command dispatch and maps lower-level errors to the same user-facing messages currently emitted by the CLI. It may print progress and usage text, but it should avoid owning protocol or platform details.

### `internal/config`

Owns AmneziaWG configuration:

- Parsing `.conf` files.
- Config data structures.
- Key conversion and validation directly tied to config parsing.
- UAPI generation from parsed config.

This package must not import `app`, `proxy`, `tunnel`, `routing`, or `dns`.

### `internal/proxy`

Owns userspace proxy behavior:

- SOCKS5 server.
- HTTP/HTTPS proxy server.
- Running commands and shells with proxy environment variables.
- macOS application launch helpers.

This package is for proxy modes only and must not apply system routes or system DNS changes.

This package should not create the AmneziaWG userspace tunnel itself. Proxy modes consume a dialer/network stack created by `internal/awgnet`, then expose SOCKS5/HTTP endpoints and run the requested shell, command, or application.

### `internal/awgnet`

Owns userspace AmneziaWG network setup for proxy modes:

- Parse interface addresses into netstack inputs.
- Create the userspace `netstack` TUN.
- Create and start the AmneziaWG device.
- Apply UAPI config for proxy modes.
- Return a dialer plus cleanup function to `internal/app` or a proxy-mode service.

This package is separate from `internal/tunnel`. Proxy modes use userspace netstack and require no root or system route changes. Transparent tunnel mode uses a native TUN device and system route/DNS management.

### `internal/tunnel`

Owns transparent system tunnel orchestration:

- Tunnel config validation.
- Endpoint resolution.
- TUN device lifecycle.
- AmneziaWG device startup.
- Route application through routing interfaces.
- DNS application through DNS interfaces.
- Tunnel bypass rules.
- Domain bypass runtime composition.
- Dry-run behavior.
- Cleanup ordering and error aggregation.

This package may depend on `config`, `routing`, `dns`, and `platform` abstractions. It must not import `app`.

Required tunnel execution order:

1. Validate tunnel config.
2. Resolve endpoint before any system mutation.
3. Validate DNS servers before device creation unless `--no-dns` is set.
4. Load bypass rules and reject domain rules with `--no-dns` before device creation.
5. Create native TUN device and register cleanup.
6. Configure the interface.
7. Build resolved UAPI and start the AmneziaWG device.
8. Discover the original default route.
9. Apply endpoint, static bypass, and tunnel routes.
10. If domain rules exist, create static-aware dynamic bypass routes and start the local DNS runtime before changing system DNS.
11. Apply system DNS unless `--no-dns` is set.
12. Wait for cancellation or signal.
13. Run cleanup in reverse registration order and join cleanup errors with the primary error.

Dry-run mode must not call mutating injected dependencies. It may use read-only default route discovery, must replace device/DNS/dynamic-route mutation with recorders, must not wait for a signal, and must print the dry-run plan.

### `internal/routing`

Owns route-related behavior:

- Route plans.
- Static bypass routes.
- Dynamic bypass routes.
- Default route discovery.
- Platform-specific route application for Darwin and Linux.

This package may use `platform.CommandRunner`. It must not know about CLI parsing or proxy server modes.

Platform-specific route manager constructors belong in this package behind build tags, not in `internal/platform`. For example, `routing.NewPlatformManager(runner platform.CommandRunner)` may return the Darwin or Linux route manager. `routing.NewPlatformDynamicBypassRoutes(defaultRoute DefaultRoute)` may remain in this package because it returns routing concepts.

### `internal/dns`

Owns DNS behavior:

- System DNS manager interface.
- Darwin DNS manager.
- Linux DNS manager.
- Domain bypass DNS forwarder.

This package may use `platform.CommandRunner` and routing-facing interfaces for dynamic bypass routes. It must not import `app`.

The domain bypass forwarder must avoid importing `internal/tunnel`. Domain matching types that are needed by the forwarder should live in `internal/dns` if they are DNS-specific, or in a small neutral package if they are shared. The minimal dynamic route contract should be defined by the consumer of the DNS runtime:

```go
type DynamicRouteAdder interface {
  AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error
}
```

The concrete routing package may satisfy this interface without the DNS package importing routing internals.

### `internal/platform`

Owns platform and process infrastructure:

- Command runner interface.
- Real command runner.
- Dry-run command runner.

This package should be low-level and must not import `app`, `tunnel`, `routing`, or `dns`. It should not own factories that return higher-level package concepts.

## Dependency Rules

Allowed high-level flow:

```text
cmd/awg-proxy -> internal/app
internal/app -> config, awgnet, proxy, tunnel, platform
internal/awgnet -> config
internal/tunnel -> config, routing, dns, platform
internal/routing -> platform
internal/dns -> platform
internal/proxy -> platform when command execution is needed
```

Lower-level packages must not import `internal/app`.

If a dependency cycle appears, prefer defining a small interface in the consuming package and passing an implementation from the composing layer.

## Migration Strategy

1. Move the executable entrypoint to `cmd/awg-proxy/main.go`.
2. Move CLI parsing, usage text, and command dispatch into `internal/app`.
3. Extract `internal/platform` with command runners only.
4. Extract `internal/config`.
5. Extract `internal/awgnet` for userspace proxy-mode AmneziaWG/netstack setup.
6. Extract `internal/proxy` for SOCKS5, HTTP, command, shell, and macOS app launching.
7. Extract `internal/routing` and keep platform route factories there.
8. Extract `internal/dns`, defining only minimal interfaces needed by the DNS runtime.
9. Extract `internal/tunnel` last, because it coordinates config, routing, DNS, native TUN, dry-run, and cleanup behavior.
10. Move tests next to the package they validate.
11. Update README build instructions from root package builds to `go build -o awg-proxy ./cmd/awg-proxy`.
12. Keep behavior-compatible CLI parsing and runtime behavior throughout the migration.

## Testing And Quality Gates

The implementation is complete only when:

- `go test ./...` passes.
- `go vet ./...` passes, unless a concrete platform-specific limitation is documented.
- `go build ./cmd/awg-proxy` passes.
- CLI parsing tests still cover compatible behavior for `shell`, `run`, `app`, `server`, and `tunnel`.
- CLI error handling tests cover missing command, unknown command, missing config, invalid config, and `run` without `--`.
- Tunnel tests preserve the required execution order, dry-run dependency substitution, domain runtime before system DNS, cleanup reverse order, and early validation before device creation.
- Platform-specific tests remain under the correct build constraints.
- Production `.go` files no longer live in the repository root.
- A root-file check confirms there are no production `.go` files outside `cmd/` or `internal/`.
- A package check confirms lower-level packages do not import `internal/app`, and `internal/platform` does not import higher-level project packages.
- No new runtime dependencies are introduced unless required by the refactor.

## Non-Goals

- Changing user-facing CLI commands or flags.
- Changing tunnel routing semantics.
- Changing tunnel bypass rules syntax.
- Changing proxy protocol behavior.
- Adding new features.
- Rewriting the project into a generic clean architecture template.

## Open Decisions Resolved

- Refactor depth: moderate.
- Package strategy: layered packages by responsibility.
- CLI compatibility: required.
- Test preservation: required.
- Exit ownership: only `cmd/awg-proxy` may call `os.Exit`; moved runtime code returns errors.
- Proxy-mode userspace network lifecycle: `internal/awgnet`.
- Platform factories: owned by the package whose concepts they construct, not by `internal/platform`.
- Tunnel ordering and dry-run behavior: required invariants.
