# awg-proxy 🔒

**Userspace AmneziaWG → SOCKS5 / HTTP proxy CLI**

Route specific terminal commands or spawn a proxied subshell — no root, no system-wide changes, no kernel drivers required.

[![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](https://github.com)

---

## How it works

`awg-proxy` embeds the [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) userspace implementation and a gVisor TCP/IP stack (`netstack`) directly into a single binary. It parses your existing AmneziaWG `.conf` file, establishes an encrypted tunnel entirely in userspace, and exposes **SOCKS5** and **HTTP/HTTPS** proxy servers on localhost.

```
Your App → local SOCKS5 / HTTP proxy → netstack (gVisor) → AmneziaWG tunnel → VPN server
```

No `sudo`. No `utun` interface. No changes to system routing.

---

## Compatibility

| AmneziaWG version | Config parameters supported |
|---|---|
| AWG 1.x | `Jc`, `Jmin`, `Jmax`, `S1`, `S2`, `H1`–`H4` (single values) |
| AWG 2.0 | All above + `S3`, `S4`, `H1`–`H4` (ranges), `I1`–`I5` chains |

---

## Installation

### Build from source

Requires [Go 1.24+](https://go.dev/dl/).

```bash
git clone https://github.com/YOUR_USERNAME/awg-proxy.git
cd awg-proxy
go build -o awg-proxy .
```

### Quick one-liner

```bash
go install github.com/YOUR_USERNAME/awg-proxy@latest
```

---

## Usage

### Interactive proxied shell (recommended)

Spawns a subshell where **all** traffic from that terminal window flows through the VPN:

```bash
./awg-proxy shell -c my_vpn.conf
```

Inside the subshell, use any tool normally — `curl`, `git`, `npm`, `pip`, `wget`, etc.:

```bash
# Verify your exit IP
curl https://ipinfo.io/json

# Clone a repo through the tunnel
git clone https://github.com/example/repo.git

# Type 'exit' or Ctrl+D to close the tunnel and return to normal routing
exit
```

### Run a single command

```bash
./awg-proxy run -c my_vpn.conf -- curl -sL https://ipinfo.io/json
./awg-proxy run -c my_vpn.conf -- git clone https://github.com/example/repo.git
./awg-proxy run -c my_vpn.conf -- npm install
```

### Persistent proxy server

Bind on fixed ports for use with browsers or other apps:

```bash
./awg-proxy server -c my_vpn.conf -s 1080 -h 8080
```

Then configure your browser / app to use:
- **SOCKS5**: `127.0.0.1:1080`
- **HTTP/HTTPS**: `127.0.0.1:8080`

Press `Ctrl+C` to stop.

---

## Configuration

`awg-proxy` reads standard WireGuard / AmneziaWG `.conf` files. See [`example.conf`](example.conf) for a fully annotated template.

> ⚠️ **Never commit your real `.conf` file.** It contains your private key. The `.gitignore` in this repo already excludes all `*.conf` files except `example.conf`.

---

## CLI reference

```
Usage:
  awg-proxy <command> [options]

Commands:
  shell    Start proxies & launch interactive subshell
  run      Start proxies, run one command, then exit
  server   Run persistent proxies in the foreground

Options:
  -c, --config       Path to AmneziaWG .conf file  (required)
  -s, --socks-port   SOCKS5 bind port              (default: auto)
  -h, --http-port    HTTP proxy bind port           (default: auto)
  -d, --debug        Verbose tunnel debug logging
```

---

## Security notes

- Private keys never leave your machine — the tunnel is established locally.
- Only the traffic you explicitly route through the proxy (`shell`, `run`) or applications you configure (`server`) uses the VPN. System-wide routing is untouched.
- Do not share or commit your `.conf` file.

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR.

---

## License

[MIT](LICENSE) — see the file for details.

---

## Acknowledgements

- [amnezia-vpn/amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) — AmneziaWG userspace implementation
- [google/gvisor](https://github.com/google/gvisor) — userspace TCP/IP stack (netstack)
