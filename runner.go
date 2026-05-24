package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// RunCommand runs a single CLI command with proxy env variables configured
func RunCommand(command []string, socksPort, httpPort int) error {
	if len(command) == 0 {
		return fmt.Errorf("no command provided")
	}

	cmdName := command[0]
	cmdArgs := command[1:]

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Inherit existing environment and inject our proxy variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("ALL_PROXY=socks5://127.0.0.1:%d", socksPort),
		fmt.Sprintf("all_proxy=socks5://127.0.0.1:%d", socksPort),
		fmt.Sprintf("HTTP_PROXY=http://127.0.0.1:%d", httpPort),
		fmt.Sprintf("http_proxy=http://127.0.0.1:%d", httpPort),
		fmt.Sprintf("HTTPS_PROXY=http://127.0.0.1:%d", httpPort),
		fmt.Sprintf("https_proxy=http://127.0.0.1:%d", httpPort),
	)

	// Forward standard termination signals (SIGINT, SIGTERM) to the child command
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigs {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		}
	}()

	return cmd.Run()
}

// RunShell spawns a fully functional subshell with the proxy env variables configured
func RunShell(socksPort, httpPort int) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
		if _, err := os.Stat(shell); os.IsNotExist(err) {
			shell = "/bin/bash"
		}
	}

	fmt.Println("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m")
	fmt.Printf("\x1b[1;36m│          🚀  AWG-PROXY SECURE SHELL SESSION            │\x1b[0m\n")
	fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mShell:\x1b[0m      %-41s \x1b[1;36m│\x1b[0m\n", shell)
	fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5:\x1b[0m     socks5://127.0.0.1:%-19d \x1b[1;36m│\x1b[0m\n", socksPort)
	fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mHTTP/HTTPS:\x1b[0m http://127.0.0.1:%-21d \x1b[1;36m│\x1b[0m\n", httpPort)
	fmt.Println("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[33m• All terminal traffic in this session is proxied.   \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m│\x1b[0m  \x1b[33m• Type 'exit' or press Ctrl+D to return to normal.    \x1b[1;36m│\x1b[0m")
	fmt.Println("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m")
	fmt.Println()

	err := RunCommand([]string{shell}, socksPort, httpPort)

	fmt.Println()
	fmt.Println("\x1b[1;31m┌────────────────────────────────────────────────────────┐\x1b[0m")
	fmt.Println("\x1b[1;31m│           🛑  SECURE PROXY SESSION TERMINATED          │\x1b[0m")
	fmt.Println("\x1b[1;31m├────────────────────────────────────────────────────────┤\x1b[0m")
	fmt.Println("\x1b[1;31m│\x1b[0m  Normal direct system routing has been restored.       \x1b[1;31m│\x1b[0m")
	fmt.Println("\x1b[1;31m└────────────────────────────────────────────────────────┘\x1b[0m")

	return err
}
