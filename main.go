package main

import (
	"awg-proxy/internal/awgnet"
	"awg-proxy/internal/config"
	"awg-proxy/internal/proxy"
	"awg-proxy/internal/tunnel"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const version = "1.0.0"

func printUsage() {
	fmt.Println("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m")
	fmt.Printf("\x1b[1;36m│          🛠️   AWG-PROXY CLI UTILITY v%-10s         │\x1b[0m\n", version)
	fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[1;33mUsage:\x1b[0m                                                \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy <command> [options]                       \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[1;33mCommands:\x1b[0m                                             \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    shell   Start proxies & launch interactive subshell \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m            (default mode if no command specified)      \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    run     Start proxies, run a single command, exit   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    app     Start proxies, launch specific macOS app,   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m            keep alive until app is closed              \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    server  Start persistent proxies in foreground      \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    tunnel  Route system traffic via native TUN          \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[1;33mOptions:\x1b[0m                                              \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -c, --config      Path to AmneziaWG .conf file      \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      (required)                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -a, --app         macOS App name or path to proxy   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      (only for 'app' command)          \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -s, --socks-port  SOCKS5 port to bind (default: 0,   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      which auto-selects a free port)   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -h, --http-port   HTTP proxy port to bind (default: \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      0, which auto-selects a free port)\x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -d, --debug       Enable verbose connection logging \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    tunnel options:  --rules PATH, --dry-run, --no-dns, \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      --verbose                         \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[1;33mExamples:\x1b[0m                                             \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy shell -c vpn.conf                         \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy run -c vpn.conf -- curl ipinfo.io/json    \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy app -c vpn.conf -a \"Google Chrome\"        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy app -c vpn.conf -- Telegram               \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy server -c vpn.conf -s 1080 -h 8080        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy tunnel -c vpn.conf --dry-run              \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")
}

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

	// 1. Parse AmneziaWG Config
	fmt.Printf("[awg-proxy] Parsing configuration: %s...\n", opts.ConfigPath)
	cfg, err := config.Parse(opts.ConfigPath)
	if err != nil {
		log.Fatalf("Configuration parse error: %v", err)
	}
	fmt.Println("[awg-proxy] Configuration parsed successfully.")

	if opts.Command == "tunnel" {
		if err := tunnel.Run(cfg, opts.Tunnel); err != nil {
			log.Fatalf("Tunnel error: %v", err)
		}
		return
	}

	if err := runProxyMode(cfg, opts); err != nil {
		log.Fatalf("Proxy mode error: %v", err)
	}
}

func runProxyMode(cfg *config.AWGConfig, opts CLIOptions) error {
	session, err := awgnet.Start(cfg, opts.Debug)
	if err != nil {
		return err
	}
	defer session.Close()

	// 2. Launch proxy servers on top of userspace netstack dialer
	socksServer, socksActualPort, err := proxy.NewSOCKS5Server(opts.SocksPort, session.Dialer)
	if err != nil {
		return fmt.Errorf("failed to start SOCKS5 proxy server: %w", err)
	}
	defer socksServer.Close()
	go socksServer.Start()

	httpServer, httpActualPort, err := proxy.NewHTTPProxyServer(opts.HTTPPort, session.Dialer)
	if err != nil {
		return fmt.Errorf("failed to start HTTP proxy server: %w", err)
	}
	defer httpServer.Close()

	// 3. Route based on command
	switch opts.Command {
	case "server":
		waitForProxyInterrupt(socksActualPort, httpActualPort)

	case "run":
		// Run a single command under the proxy
		err := proxy.RunCommand(opts.CommandArgs, socksActualPort, httpActualPort)
		if err != nil {
			return fmt.Errorf("command returned exit error: %w", err)
		}

	case "app":
		// Run a macOS application under the proxy
		err := proxy.RunApp(opts.AppTarget, opts.AppArgs, socksActualPort, httpActualPort)
		if err != nil {
			return fmt.Errorf("app returned exit error: %w", err)
		}

	case "shell":
		// Spawns an interactive shell
		err := proxy.RunShell(socksActualPort, httpActualPort)
		if err != nil {
			return fmt.Errorf("shell session error: %w", err)
		}
	}

	return nil
}

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
