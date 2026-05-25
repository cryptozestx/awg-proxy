//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type LinuxDNSManager struct {
	ResolvConfPath string
}

func (m LinuxDNSManager) Apply(_ context.Context, servers []string, cleanup *CleanupStack) error {
	path := m.ResolvConfPath
	if path == "" {
		path = "/etc/resolv.conf"
	}

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat resolv.conf %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("resolv.conf %s appears managed or symlink; use --no-dns to skip DNS changes", path)
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read resolv.conf %s: %w", path, err)
	}

	lines := make([]string, 0, len(servers))
	for _, server := range servers {
		lines = append(lines, "nameserver "+server)
	}
	contents := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(contents), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write resolv.conf %s: %w", path, err)
	}

	cleanup.Add("restore resolv.conf", func() error {
		return os.WriteFile(path, original, info.Mode().Perm())
	})

	return nil
}
