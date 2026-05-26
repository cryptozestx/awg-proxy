package app

import (
	"awg-proxy/internal/awgnet"
	"awg-proxy/internal/config"
	"awg-proxy/internal/proxy"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func (r Runtime) runProxyMode(cfg *config.AWGConfig, opts Options) error {
	session, err := awgnet.Start(cfg, opts.Debug)
	if err != nil {
		return err
	}
	defer session.Close()

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

	switch opts.Command {
	case "server":
		waitForProxyInterrupt(r.stdout(), socksActualPort, httpActualPort)

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

func waitForProxyInterrupt(w io.Writer, socksActualPort, httpActualPort int) {
	if w == nil {
		w = io.Discard
	}

	fmt.Fprintln(w, "\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m")
	fmt.Fprintf(w, "\x1b[1;36m│          🚀  AWG-PROXY SERVER RUNNING IN FG            │\x1b[0m\n")
	fmt.Fprintln(w, "\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Fprintf(w, "\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5 proxy:\x1b[0m     socks5://127.0.0.1:%-19d \x1b[1;36m│\x1b[0m\n", socksActualPort)
	fmt.Fprintf(w, "\x1b[1;36m│\x1b[0m  \x1b[32mHTTP/HTTPS proxy:\x1b[0m  http://127.0.0.1:%-21d \x1b[1;36m│\x1b[0m\n", httpActualPort)
	fmt.Fprintln(w, "\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m│\x1b[0m  \x1b[33mPress Ctrl+C to terminate proxy servers.             \x1b[1;36m│\x1b[0m")
	fmt.Fprintln(w, "\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	<-sigs
	fmt.Fprintln(w, "\n[awg-proxy] Shutting down proxy servers...")
}
