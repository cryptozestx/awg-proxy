package app

import (
	"fmt"
	"io"
)

const Version = "1.0.0"

func PrintUsage(w io.Writer) {
	if w == nil {
		w = io.Discard
	}

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
