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
git clone https://github.com/cryptozestx/awg-proxy.git
cd awg-proxy
go build -o awg-proxy .
```

### Quick one-liner

```bash
go install github.com/cryptozestx/awg-proxy@latest
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

### Launch specific macOS applications (macOS only)

You can launch specific GUI or CLI applications routed entirely through the secure userspace VPN tunnel. Closing the application automatically terminates the tunnel.

```bash
# Launch Google Chrome with a separate, isolated proxied profile
./awg-proxy app -c my_vpn.conf -a "Google Chrome"

# Launch Telegram (automatically registers the SOCKS5 proxy in Telegram via URL scheme!)
./awg-proxy app -c my_vpn.conf -a Telegram

# Launch Spotify with pre-configured proxy flags
./awg-proxy app -c my_vpn.conf -a Spotify

# Launch Slack, VS Code, Discord, or any other Electron/GUI app with environment variables
./awg-proxy app -c my_vpn.conf -a Slack
./awg-proxy app -c my_vpn.conf -- "/Applications/Visual Studio Code.app"
```

#### How it handles applications:
1. **Chromium-based Browsers** (`Chrome`, `Brave`, `Edge`, `Arc`): Spawns a dedicated browser window with `--proxy-server` set to your secure tunnel. It uses a **persistent, isolated profile** under `~/.awg-proxy/profiles/` so your secure session runs smoothly side-by-side with your unproxied browser and keeps cookies/logins saved.
2. **Telegram**: Automatically opens a `tg://socks?server=127.0.0.1&port=<port>` link. Telegram will prompt you to single-click **"Enable"** to route all telegram chats securely.
3. **Spotify**: Runs the binary with Spotify SOCKS5 CLI arguments appended.
4. **General Apps** (Slack, Obsidian, Discord, etc.): Automatically locates the bundle executable using macOS `plutil`, spawns the process, and injects `ALL_PROXY`, `HTTP_PROXY`, and `HTTPS_PROXY` environment variables.

### Persistent proxy server

Bind on fixed ports for use with browsers or other apps:

```bash
./awg-proxy server -c my_vpn.conf -s 1080 -h 8080
```

Then configure your browser / app to use:
- **SOCKS5**: `127.0.0.1:1080`
- **HTTP/HTTPS**: `127.0.0.1:8080`

Press `Ctrl+C` to stop.

### Transparent system tunnel

`tunnel` creates a native TUN interface and routes IPv4 system traffic through AmneziaWG. It requires elevated privileges because it changes system routes and DNS.

```bash
sudo ./awg-proxy tunnel -c my_vpn.conf --dry-run
sudo ./awg-proxy tunnel -c my_vpn.conf
sudo ./awg-proxy tunnel -c my_vpn.conf --no-dns
```

The tunnel mode resolves the peer endpoint before changing routes, rewrites the device endpoint to the resolved IPv4 address, adds a host bypass route for that endpoint, then adds `0.0.0.0/1` and `128.0.0.0/1` through the TUN interface. DNS from `[Interface] DNS` is applied by default and must contain IP address entries. Use `--no-dns` only when you accept existing DNS behavior.

#### Tunnel bypass rules

Use `--rules` to route selected destinations outside the tunnel:

```bash
sudo ./awg-proxy tunnel -c my_vpn.conf --rules tunnel.rules
```

Example `tunnel.rules`:

```conf
exclude_ip = 203.0.113.10
exclude_cidr = 198.51.100.0/24
exclude_domain = *.delimobil.*
```

`exclude_ip` and `exclude_cidr` add direct routes through the original default gateway. `exclude_domain` requires tunnel DNS control; it is incompatible with `--no-dns`. Domain rules work only for applications that use system DNS. Applications using DNS-over-HTTPS or a private resolver may not be bypassed by domain rules.

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
  app      Start proxies, launch specific macOS app, keep alive until app is closed
  server   Run persistent proxies in the foreground
  tunnel   Route system traffic via native TUN

Options:
  -c, --config       Path to AmneziaWG .conf file  (default: amnezia.conf)
  -a, --app          macOS App name or path to proxy (only for 'app')
  -s, --socks-port   SOCKS5 bind port              (default: auto)
  -h, --http-port    HTTP proxy bind port           (default: auto)
  -d, --debug        Verbose tunnel debug logging
  --dry-run          Print tunnel changes without applying them
  --no-dns           Do not change system DNS in tunnel mode
  --rules            Path to tunnel bypass rules file
```

---

## Security notes

- Private keys never leave your machine — the tunnel is established locally.
- Proxy modes (`shell`, `run`, `app`, `server`) do not change system routing. `tunnel` is the explicit privileged mode that routes system IPv4 traffic and may change DNS.
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
