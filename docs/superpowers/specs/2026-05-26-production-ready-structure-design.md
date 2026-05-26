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
    platform_darwin.go
    platform_linux.go
```

## Package Responsibilities

### `cmd/awg-proxy`

Contains only the executable entrypoint.

`main.go` should delegate to `internal/app` and translate the returned error into process output and exit code. It must not contain CLI parsing, tunnel setup, proxy setup, route management, DNS management, or config parsing logic.

### `internal/app`

Owns application-level orchestration:

- CLI parsing.
- Usage/help output.
- Command dispatch for `shell`, `run`, `app`, `server`, and `tunnel`.
- Proxy mode startup.
- Tunnel mode startup.

This package may compose lower-level packages, but it must not implement platform route commands, DNS mutation, SOCKS protocol handling, HTTP proxy handling, TUN setup, or AmneziaWG config parsing.

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

### `internal/routing`

Owns route-related behavior:

- Route plans.
- Static bypass routes.
- Dynamic bypass routes.
- Default route discovery.
- Platform-specific route application for Darwin and Linux.

This package may use `platform.CommandRunner`. It must not know about CLI parsing or proxy server modes.

### `internal/dns`

Owns DNS behavior:

- System DNS manager interface.
- Darwin DNS manager.
- Linux DNS manager.
- Domain bypass DNS forwarder.

This package may use `platform.CommandRunner` and routing-facing interfaces for dynamic bypass routes. It must not import `app`.

### `internal/platform`

Owns platform and process infrastructure:

- Command runner interface.
- Real command runner.
- Dry-run command runner.
- Platform factories such as default tunnel name and platform manager construction when useful.

This package should be low-level and avoid importing application packages.

## Dependency Rules

Allowed high-level flow:

```text
cmd/awg-proxy -> internal/app
internal/app -> config, proxy, tunnel, platform
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
3. Extract independent low-level packages first: `config`, `platform`, `proxy`, `routing`, and `dns`.
4. Extract `internal/tunnel` last, because it currently coordinates most other areas.
5. Move tests next to the package they validate.
6. Update README build instructions from root package builds to `go build -o awg-proxy ./cmd/awg-proxy`.
7. Keep behavior-compatible CLI parsing and runtime behavior throughout the migration.

## Testing And Quality Gates

The implementation is complete only when:

- `go test ./...` passes.
- `go vet ./...` passes, unless a concrete platform-specific limitation is documented.
- `go build ./cmd/awg-proxy` passes.
- CLI parsing tests still cover compatible behavior for `shell`, `run`, `app`, `server`, and `tunnel`.
- Platform-specific tests remain under the correct build constraints.
- Production `.go` files no longer live in the repository root.
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
