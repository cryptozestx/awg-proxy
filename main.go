package main

import (
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun/netstack"
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
	fmt.Println("\x1b[1;36m│\x1b[0m    server  Start persistent proxies in foreground      \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[1;33mOptions:\x1b[0m                                              \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -c, --config      Path to AmneziaWG .conf file      \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      (required)                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -s, --socks-port  SOCKS5 port to bind (default: 0,   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      which auto-selects a free port)   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -h, --http-port   HTTP proxy port to bind (default: \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                      0, which auto-selects a free port)\x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    -d, --debug       Enable verbose connection logging \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m                                                        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[1;33mExamples:\x1b[0m                                             \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy shell -c vpn.conf                         \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy run -c vpn.conf -- curl ipinfo.io/json    \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m    awg-proxy server -c vpn.conf -s 1080 -h 8080        \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := ""
	argsStart := 1

	switch os.Args[1] {
	case "shell", "run", "server":
		command = os.Args[1]
		argsStart = 2
	default:
		// Check if first arg is a flag. If so, default command to "shell"
		if os.Args[1][0] == '-' {
			command = "shell"
		} else {
			fmt.Printf("\x1b[1;31mUnknown command: %s\x1b[0m\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	// Setup custom FlagSet for parsing
	fs := flag.NewFlagSet("awg-proxy", flag.ExitOnError)
	var configPath string
	var socksPort int
	var httpPort int
	var debug bool

	fs.StringVar(&configPath, "config", "", "Path to AmneziaWG configuration file")
	fs.StringVar(&configPath, "c", "", "Path to AmneziaWG configuration file (shorthand)")
	fs.IntVar(&socksPort, "socks-port", 0, "SOCKS5 port to bind (default: 0 - auto select)")
	fs.IntVar(&socksPort, "s", 0, "SOCKS5 port to bind (shorthand)")
	fs.IntVar(&httpPort, "http-port", 0, "HTTP port to bind (default: 0 - auto select)")
	fs.IntVar(&httpPort, "h", 0, "HTTP port to bind (shorthand)")
	fs.BoolVar(&debug, "debug", false, "Enable verbose debug logs")
	fs.BoolVar(&debug, "d", false, "Enable verbose debug logs (shorthand)")

	// Parse custom sub-arguments
	var commandArgs []string
	if command == "run" {
		// Find "--" separator for execution
		sepIdx := -1
		for i := argsStart; i < len(os.Args); i++ {
			if os.Args[i] == "--" {
				sepIdx = i
				break
			}
		}
		if sepIdx == -1 {
			fmt.Println("\x1b[1;31mError: Command 'run' requires '--' followed by the CLI command to run.\x1b[0m")
			fmt.Println("Example: awg-proxy run -c vpn.conf -- curl ipinfo.io/json")
			os.Exit(1)
		}
		_ = fs.Parse(os.Args[argsStart:sepIdx])
		commandArgs = os.Args[sepIdx+1:]
		if len(commandArgs) == 0 {
			fmt.Println("\x1b[1;31mError: No target command specified after '--'.\x1b[0m")
			os.Exit(1)
		}
	} else {
		_ = fs.Parse(os.Args[argsStart:])
	}

	if configPath == "" {
		fmt.Println("\x1b[1;31mError: Configuration file path is required.\x1b[0m")
		fs.Usage()
		os.Exit(1)
	}

	// 1. Parse AmneziaWG Config
	fmt.Printf("[awg-proxy] Parsing configuration: %s...\n", configPath)
	cfg, err := ParseConfig(configPath)
	if err != nil {
		log.Fatalf("Configuration parse error: %v", err)
	}

	// 2. Parse address sets
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

	// 3. Create userspace TUN device bound to netstack
	fmt.Println("[awg-proxy] Initializing userspace network stack...")
	tunDev, tnet, err := netstack.CreateNetTUN(localAddrs, dnsAddrs, mtu)
	if err != nil {
		log.Fatalf("Failed to create userspace network stack: %v", err)
	}

	// 4. Create WireGuard device
	logLevel := device.LogLevelSilent
	if debug {
		logLevel = device.LogLevelVerbose
	}
	logger := device.NewLogger(logLevel, "[AWG] ")
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	// 5. Apply configuration via UAPI
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

	// 6. Launch proxy servers on top of userspace netstack dialer
	socksServer, socksActualPort, err := NewSOCKS5Server(socksPort, tnet)
	if err != nil {
		log.Fatalf("Failed to start SOCKS5 proxy server: %v", err)
	}
	defer socksServer.Close()
	go socksServer.Start()

	httpServer, httpActualPort, err := NewHTTPProxyServer(httpPort, tnet)
	if err != nil {
		log.Fatalf("Failed to start HTTP proxy server: %v", err)
	}
	defer httpServer.Close()

	// 7. Route based on command
	switch command {
	case "server":
		fmt.Println("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m")
		fmt.Printf("\x1b[1;36m│          🚀  AWG-PROXY SERVER RUNNING IN FG            │\x1b[0m\n")
		fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5 proxy:\x1b[0m     socks5://127.0.0.1:%-19d \x1b[1;36m│\x1b[0m\n", socksActualPort)
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mHTTP/HTTPS proxy:\x1b[0m  http://127.0.0.1:%-21d \x1b[1;36m│\x1b[0m\n", httpActualPort)
		fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
		fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[33mPress Ctrl+C to terminate proxy servers.             \x1b[1;36m│\x1b[0m")
		fmt.Println("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")

		// Keep main running until interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		fmt.Println("\n[awg-proxy] Shutting down proxy servers...")

	case "run":
		// Run a single command under the proxy
		err := RunCommand(commandArgs, socksActualPort, httpActualPort)
		if err != nil {
			log.Fatalf("Command returned exit error: %v", err)
		}

	case "shell":
		// Spawns an interactive shell
		err := RunShell(socksActualPort, httpActualPort)
		if err != nil {
			log.Fatalf("Shell session error: %v", err)
		}
	}
}
