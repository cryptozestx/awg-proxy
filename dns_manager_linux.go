//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LinuxDNSManager struct {
	ResolvConfPath string
}

func (m LinuxDNSManager) Apply(ctx context.Context, servers []string, cleanup *CleanupStack) error {
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

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply DNS to resolv.conf %s: %w", path, err)
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read resolv.conf %s: %w", path, err)
	}

	cleanup.Add("restore resolv.conf", func() error {
		return writeFileAtomic(path, original, info.Mode().Perm())
	})

	lines := make([]string, 0, len(servers))
	for _, server := range servers {
		lines = append(lines, "nameserver "+server)
	}
	contents := strings.Join(lines, "\n") + "\n"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply DNS to resolv.conf %s: %w", path, err)
	}
	if err := writeFileAtomic(path, []byte(contents), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write resolv.conf %s: %w", path, err)
	}

	return nil
}

func writeFileAtomic(path string, contents []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)

	if _, err := file.Write(contents); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(perm); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	return nil
}
