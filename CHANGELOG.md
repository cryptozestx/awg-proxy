# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] - 2026-05-24

### Added
- Initial release of `awg-proxy`
- Userspace AmneziaWG tunnel via `amneziawg-go` + gVisor `netstack`
- `shell` command: interactive subshell with proxy env vars injected
- `run` command: single command executed under the proxy
- `server` command: persistent SOCKS5 / HTTP proxy for external apps
- SOCKS5 proxy server (no external dependencies, stdlib only)
- HTTP / HTTPS CONNECT proxy server (supports all standard tools)
- Full AmneziaWG config parser supporting:
  - AWG 1.x: `Jc`, `Jmin`, `Jmax`, `S1`, `S2`, `H1`–`H4` (single values)
  - AWG 2.0: `S3`, `S4`, `H1`–`H4` (ranges), `I1`–`I5` obfuscation chains
- Dynamic port allocation (bind port `0` → OS chooses a free port)
- Signal forwarding (`SIGINT`, `SIGTERM`) to child processes
- Coloured terminal session banners (open/close notifications)
- `example.conf` annotated configuration template
- MIT License
