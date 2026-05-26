//go:build !darwin

package proxy

import "fmt"

func ResolveAppPath(appName string) (string, error) {
	return "", fmt.Errorf("app command is only supported on macOS: %s", appName)
}

func GetBundleExecutable(appPath string) (string, error) {
	return "", fmt.Errorf("app bundles are only supported on macOS: %s", appPath)
}

func RunApp(appTarget string, extraArgs []string, socksPort, httpPort int) error {
	return fmt.Errorf("app command is only supported on macOS: %s", appTarget)
}
