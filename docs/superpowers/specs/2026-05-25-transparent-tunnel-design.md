# Transparent Tunnel Mode Design

## Goal

Add a privileged `tunnel` command that routes system traffic through AmneziaWG on macOS and Linux. Existing `shell`, `run`, `server`, and `app` modes remain rootless proxy modes built on the current userspace netstack.

The first release is a full-tunnel IPv4 MVP. The design keeps route policy separate so split-tunnel rules can be added without rewriting the tunnel engine.

## Non-Goals For MVP

- Process-based split tunnel.
- Domain-based split tunnel.
- IPv6 full tunnel.
- Kill switch.
- Daemon or service mode.
- GUI or macOS Network Extension.
- Automatic privilege escalation.

## User Interface

Primary command:

```bash
sudo ./awg-proxy tunnel -c amnezia.conf
```

Diagnostic and safety flags:

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --dry-run
sudo ./awg-proxy tunnel -c amnezia.conf --no-dns
sudo ./awg-proxy tunnel -c amnezia.conf --verbose
```

Future split-tunnel shape:

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --exclude-cidr 192.168.0.0/16
sudo ./awg-proxy tunnel -c amnezia.conf --include-cidr 8.8.8.8/32
```

The MVP does not expose these future route-policy flags. Route-policy internals are still structured so these flags can be added later.

## Architecture

The `tunnel` command is a separate execution path from the current proxy modes.

```text
main.go
  parses command and shared config
  dispatches to tunnel command

tunnel.go
  RunTunnel(opts)
  owns lifecycle: setup, wait, cleanup

tunnel_device.go
  creates native TUN
  creates amneziawg-go device
  applies UAPI config
  brings device up and down

route_policy.go
  builds desired routing intent
  full-tunnel MVP now
  keeps include/exclude CIDR extension points

route_manager_darwin.go
route_manager_linux.go
  platform-specific route apply and restore

dns_manager_darwin.go
dns_manager_linux.go
  platform-specific DNS apply and restore

cleanup.go
  signal handling and ordered rollback
```

`RoutePolicy` must not run shell commands. It returns a normalized plan:

```go
type RoutePlan struct {
    TunnelCIDRs    []netip.Prefix
    ExcludedCIDRs  []netip.Prefix
    EndpointBypass netip.AddrPort
}
```

Platform-specific route managers decide how to apply that plan on macOS and Linux.

## Data Flow

Runtime traffic path:

```text
System apps
  -> OS routing table
  -> native TUN interface
  -> amneziawg-go device
  -> encrypted UDP to AWG endpoint
  -> VPN server
```

Startup sequence:

1. Parse the AmneziaWG config.
2. Validate that tunnel mode has exactly one peer with an `Endpoint`.
3. Resolve the endpoint hostname to an IP before route changes.
4. Create the native TUN interface.
5. Configure TUN IP and MTU from `[Interface]`.
6. Start the `amneziawg-go` device on the native TUN.
7. Apply the UAPI config and bring the device up.
8. Add an endpoint bypass route through the original default gateway.
9. Add full-tunnel IPv4 split default routes through the TUN:
   - `0.0.0.0/1`
   - `128.0.0.0/1`
10. Apply DNS from `[Interface] DNS` unless `--no-dns` is set.
11. Wait until `SIGINT` or `SIGTERM`.

Shutdown runs in reverse:

1. Restore DNS.
2. Remove full-tunnel routes.
3. Remove endpoint bypass route.
4. Bring the AWG device down.
5. Close the TUN interface.

Endpoint resolution before route changes is required. If the endpoint is a hostname and resolution happens after full-tunnel routes are installed, the tunnel transport can route into itself.

## Route Strategy

The MVP uses two IPv4 split default routes instead of deleting or replacing the existing default route:

```text
0.0.0.0/1
128.0.0.0/1
```

These routes cover the IPv4 internet and are more specific than `0.0.0.0/0`, so they win while preserving the original default route. Cleanup only needs to remove these two routes and the endpoint bypass route.

The AWG endpoint IP must always be excluded from the tunnel and routed via the original default gateway.

## macOS Implementation

The macOS implementation creates a native `utun` interface, configures it with `ifconfig`, and manages routes with `route`.

Required steps:

- Create a `utun` interface through a compatible Go TUN implementation.
- Assign the interface IPv4 address from `[Interface] Address`.
- Set MTU from config, defaulting to the existing project default when absent.
- Discover the current default gateway using `route -n get default`.
- Add a host route for the resolved AWG endpoint through the original gateway.
- Add `0.0.0.0/1` and `128.0.0.0/1` through the TUN interface.
- Restore all route changes on exit.

DNS on macOS:

- Capture DNS state for active network services using `networksetup -getdnsservers`.
- Set DNS servers from `[Interface] DNS` using `networksetup -setdnsservers`.
- Restore captured DNS state on exit.
- If DNS capture or restore fails, print concrete manual recovery commands.

## Linux Implementation

The Linux implementation creates a TUN interface via `/dev/net/tun`. The MVP uses command-backed route management through `ip` behind the `RouteManager` interface. A later implementation can replace that with netlink without changing tunnel lifecycle or route policy code.

Required steps:

- Create the TUN interface.
- Assign the interface IPv4 address from `[Interface] Address`.
- Set MTU from config, defaulting to the existing project default when absent.
- Discover the current default route and gateway.
- Add a host route for the resolved AWG endpoint through the original gateway.
- Add `0.0.0.0/1` and `128.0.0.0/1` through the TUN interface.
- Restore all route changes on exit.

DNS on Linux:

- Do not aggressively rewrite distro-managed DNS state.
- If `/etc/resolv.conf` is a regular file, back it up and write DNS servers from config.
- If `/etc/resolv.conf` is a symlink or clearly managed by another service, print a warning and continue without changing DNS.
- `--no-dns` skips DNS changes explicitly.

## Cleanup And Failure Handling

Every successful setup step immediately registers a rollback action.

Examples:

```text
create TUN
  rollback: close TUN

configure TUN address
  rollback: remove address or bring interface down

add endpoint bypass route
  rollback: delete endpoint bypass route

add 0.0.0.0/1 route
  rollback: delete 0.0.0.0/1 route

add 128.0.0.0/1 route
  rollback: delete 128.0.0.0/1 route

set DNS
  rollback: restore previous DNS
```

Cleanup runs on:

- normal exit;
- `Ctrl+C`;
- `SIGTERM`;
- startup failure after partial setup;
- panic, best effort through `defer`.

If cleanup partially fails, the CLI prints manual recovery commands for the affected platform.

## Validation Rules

Tunnel mode fails early when:

- no config file can be resolved;
- there is not exactly one peer with `Endpoint`;
- no IPv4 address exists in `[Interface] Address`;
- endpoint hostname cannot be resolved before route changes;
- the process lacks privileges to create TUN or change routes.

The error messages state what failed and what the user can do next.

## Testing

Automated tests must not modify real system routes or DNS.

Unit tests:

- `RoutePolicy` builds full-tunnel route plans.
- endpoint bypass is always present.
- future include/exclude CIDR parsing is deterministic.
- tunnel config validation rejects missing endpoint, multiple endpoints, and missing IPv4 address.
- lifecycle rollback runs successful steps in reverse order when a later step fails.

Manual smoke tests on macOS:

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --dry-run
sudo ./awg-proxy tunnel -c amnezia.conf
curl https://ipinfo.io/ip
agy --print-timeout 30s -p "Reply with exactly: tunnel-test"
# Press Ctrl+C.
netstat -rn
scutil --dns
```

Manual smoke tests on Linux:

```bash
sudo ./awg-proxy tunnel -c amnezia.conf --dry-run
sudo ./awg-proxy tunnel -c amnezia.conf
curl https://ipinfo.io/ip
agy --print-timeout 30s -p "Reply with exactly: tunnel-test"
# Press Ctrl+C.
ip route
cat /etc/resolv.conf
```

Success criteria:

- `curl` reports the VPN exit IP.
- `agy` works without relying on `HTTP_PROXY`, `HTTPS_PROXY`, or `ALL_PROXY`.
- AWG endpoint traffic is not routed into the tunnel.
- Routes and DNS are restored after exit.
- Failed startup restores all completed setup steps.
