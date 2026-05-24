package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// ResolveAppPath searches `/Applications`, `/System/Applications`, and `~/Applications`
// for matching .app bundles (case-insensitive fuzzy matching). It also supports direct paths.
func ResolveAppPath(appName string) (string, error) {
	// 1. If it's already an absolute or relative path that exists directly
	if _, err := os.Stat(appName); err == nil {
		return appName, nil
	}

	// 2. If it's a short name, search standard macOS application directories
	searchDirs := []string{
		"/Applications",
		"/System/Applications",
	}
	if home, err := os.UserHomeDir(); err == nil {
		searchDirs = append(searchDirs, filepath.Join(home, "Applications"))
	}

	// Try exact/direct match in standard directories first
	for _, dir := range searchDirs {
		path := filepath.Join(dir, appName)
		if !strings.HasSuffix(strings.ToLower(path), ".app") {
			path += ".app"
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try fuzzy case-insensitive match inside those directories
	appNameLower := strings.ToLower(appName)
	for _, dir := range searchDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".app") {
				continue
			}
			fName := f.Name()
			fNameLower := strings.ToLower(fName)
			fNameNoExt := strings.TrimSuffix(fNameLower, ".app")

			// Check if name matches exactly or is contained within the app bundle name
			if fNameNoExt == appNameLower || strings.Contains(fNameNoExt, appNameLower) {
				return filepath.Join(dir, fName), nil
			}
		}
	}

	return "", fmt.Errorf("could not find application '%s' in standard directories", appName)
}

// GetBundleExecutable parses the bundle Info.plist using macOS built-in plutil
// to extract the actual executable name inside Contents/MacOS.
func GetBundleExecutable(appPath string) (string, error) {
	plistPath := filepath.Join(appPath, "Contents", "Info.plist")
	if _, err := os.Stat(plistPath); err != nil {
		return "", fmt.Errorf("Info.plist not found in bundle: %w", err)
	}

	cmd := exec.Command("plutil", "-extract", "CFBundleExecutable", "raw", plistPath)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to extract CFBundleExecutable: %w (output: %s)", err, string(out))
	}

	execName := strings.TrimSpace(string(out))
	if execName == "" {
		return "", fmt.Errorf("CFBundleExecutable is empty inside Info.plist")
	}

	return filepath.Join(appPath, "Contents", "MacOS", execName), nil
}

// RunApp resolves the application, sets up specialized proxy arguments or environment variables,
// and spawns it in the foreground, keeping the proxy alive until the application is closed.
func RunApp(appTarget string, extraArgs []string, socksPort, httpPort int) error {
	resolvedPath, err := ResolveAppPath(appTarget)
	var binaryPath string
	isAppBundle := false

	if err == nil && strings.HasSuffix(strings.ToLower(resolvedPath), ".app") {
		isAppBundle = true
		fmt.Printf("[awg-proxy] Resolved macOS App Bundle: %s\n", resolvedPath)
		binaryPath, err = GetBundleExecutable(resolvedPath)
		if err != nil {
			return fmt.Errorf("failed to get executable from bundle: %w", err)
		}
	} else {
		// If resolution failed or it's a direct command/binary, check direct existence or system PATH
		if _, errStat := os.Stat(appTarget); errStat == nil {
			binaryPath = appTarget
		} else if pathBin, errLook := exec.LookPath(appTarget); errLook == nil {
			binaryPath = pathBin
		} else {
			return fmt.Errorf("could not resolve application path or executable binary: %s (resolution error: %v)", appTarget, err)
		}
	}

	fmt.Printf("[awg-proxy] Target executable: %s\n", binaryPath)

	// Determine specialized launch rules based on app bundle name or raw binary name
	var appName string
	if resolvedPath != "" && resolvedPath != "." {
		appName = filepath.Base(resolvedPath)
	} else {
		appName = filepath.Base(binaryPath)
	}
	appNameNoExt := strings.TrimSuffix(strings.ToLower(appName), ".app")
	if appNameNoExt == "" || appNameNoExt == "." {
		appNameNoExt = strings.ToLower(filepath.Base(binaryPath))
	}

	var cmdArgs []string
	cmdArgs = append(cmdArgs, extraArgs...)

	var cmd *exec.Cmd

	// 1. Handle Chromium Browsers (Google Chrome, Brave Browser, Microsoft Edge, Arc)
	isChromium := false
	var profileName string
	switch appNameNoExt {
	case "google chrome", "chrome":
		isChromium = true
		profileName = "chrome"
	case "brave browser", "brave":
		isChromium = true
		profileName = "brave"
	case "microsoft edge", "edge":
		isChromium = true
		profileName = "edge"
	case "arc":
		isChromium = true
		profileName = "arc"
	}

	if isChromium {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		profileDir := filepath.Join(homeDir, ".awg-proxy", "profiles", profileName)
		if err := os.MkdirAll(profileDir, 0755); err != nil {
			return fmt.Errorf("failed to create browser profile directory: %w", err)
		}

		proxyArg := fmt.Sprintf("--proxy-server=socks5://127.0.0.1:%d", socksPort)
		profileArg := fmt.Sprintf("--user-data-dir=%s", profileDir)

		fmt.Printf("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│          🌐  LAUNCHING SECURE CHROMIUM BROWSER         │\x1b[0m\n")
		fmt.Printf("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mBrowser:\x1b[0m    %-41s \x1b[1;36m│\x1b[0m\n", appName)
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5:\x1b[0m     socks5://127.0.0.1:%-19d \x1b[1;36m│\x1b[0m\n", socksPort)
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mProfile:\x1b[0m    %-41s \x1b[1;36m│\x1b[0m\n", "~/.awg-proxy/profiles/"+profileName)
		fmt.Printf("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[33m• Starts isolated, secure session.                    \x1b[1;36m│\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[33m• Keeps your cookies/history separated & persistent. \x1b[1;36m│\x1b[0m\n")
		fmt.Printf("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m\n\n")

		cmdArgs = append([]string{proxyArg, profileArg, "--no-first-run"}, cmdArgs...)
		cmd = exec.Command(binaryPath, cmdArgs...)
	} else if appNameNoExt == "spotify" {
		// 2. Handle Spotify
		fmt.Printf("[awg-proxy] Launching Spotify with SOCKS5 configuration flags...\n\n")
		proxyTypeArg := "--proxy-type=socks5"
		proxyAddrArg := fmt.Sprintf("--proxy-addr=127.0.0.1:%d", socksPort)
		cmdArgs = append([]string{proxyTypeArg, proxyAddrArg}, cmdArgs...)
		cmd = exec.Command(binaryPath, cmdArgs...)
	} else if appNameNoExt == "telegram" {
		// 3. Handle Telegram
		socksLink := fmt.Sprintf("tg://socks?server=127.0.0.1&port=%d", socksPort)
		fmt.Printf("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│          ✈️   TELEGRAM AUTO-PROXY REGISTRATION          │\x1b[0m\n")
		fmt.Printf("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5 Proxy:\x1b[0m  127.0.0.1:%-27d \x1b[1;36m│\x1b[0m\n", socksPort)
		fmt.Printf("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[33m1. Opening deep link in browser/Telegram...           \x1b[1;36m│\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[33m2. Click 'Enable' / 'Применить' in Telegram popup!    \x1b[1;36m│\x1b[0m\n")
		fmt.Printf("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m\n\n")

		// Trigger macOS open to automatically launch the socks registration prompt in Telegram
		_ = exec.Command("open", socksLink).Run()

		cmd = exec.Command(binaryPath, cmdArgs...)
	} else {
		// 4. Handle general applications via SOCKS5 and HTTP environment variables
		fmt.Printf("\x1b[1;36m┌────────────────────────────────────────────────────────┐\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│          🚀  LAUNCHING SECURE APPLICATION              │\x1b[0m\n")
		fmt.Printf("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mApplication:\x1b[0m  %-41s \x1b[1;36m│\x1b[0m\n", appName)
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mSOCKS5 Port:\x1b[0m  %-41d \x1b[1;36m│\x1b[0m\n", socksPort)
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[32mHTTP Port:\x1b[0m    %-41d \x1b[1;36m│\x1b[0m\n", httpPort)
		fmt.Printf("\x1b[1;36m├────────────────────────────────────────────────────────┤\x1b[0m\n")
		fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[33mInjected HTTP_PROXY, HTTPS_PROXY, and ALL_PROXY envs! \x1b[1;36m│\x1b[0m\n")
		if isAppBundle {
			fmt.Printf("\x1b[1;36m│\x1b[0m  \x1b[33m• Closing the app window will terminate the proxy.    \x1b[1;36m│\x1b[0m\n")
		}
		fmt.Printf("\x1b[1;36m└────────────────────────────────────────────────────────┘\x1b[0m\n\n")

		cmd = exec.Command(binaryPath, cmdArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Inject proxy environment variables into the process
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("ALL_PROXY=socks5://127.0.0.1:%d", socksPort),
		fmt.Sprintf("all_proxy=socks5://127.0.0.1:%d", socksPort),
		fmt.Sprintf("HTTP_PROXY=http://127.0.0.1:%d", httpPort),
		fmt.Sprintf("http_proxy=http://127.0.0.1:%d", httpPort),
		fmt.Sprintf("HTTPS_PROXY=http://127.0.0.1:%d", httpPort),
		fmt.Sprintf("https_proxy=http://127.0.0.1:%d", httpPort),
	)

	// Forward standard termination signals (SIGINT, SIGTERM) to the child application
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigs {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		}
	}()

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("application session closed: %w", err)
	}

	fmt.Println("\x1b[1;31m[awg-proxy] Target application exited gracefully. Terminating secure tunnel...\x1b[0m")
	return nil
}
