//go:build linux

package dns

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxManagerWritesAndRestoresRegularResolvConf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	original := []byte("nameserver 9.9.9.9\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cleanup := newCleanupStack()
	manager := LinuxManager{ResolvConfPath: path}
	if err := manager.Apply(context.Background(), []string{"1.1.1.1", "8.8.8.8"}, cleanup); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "nameserver 1.1.1.1\nnameserver 8.8.8.8\n"
	if string(got) != want {
		t.Fatalf("resolv.conf = %q, want %q", string(got), want)
	}

	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup.Run() error = %v", err)
	}

	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after cleanup error = %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("restored resolv.conf = %q, want %q", string(got), string(original))
	}
}

func TestLinuxManagerRejectsSymlinkUnlessNoDNS(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "managed-resolv.conf")
	if err := os.WriteFile(target, []byte("nameserver 9.9.9.9\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	path := filepath.Join(dir, "resolv.conf")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := LinuxManager{ResolvConfPath: path}.Apply(context.Background(), []string{"1.1.1.1"}, newCleanupStack())
	if err == nil {
		t.Fatal("Apply() error = nil, want managed or symlink error")
	}
	if !strings.Contains(err.Error(), "managed or symlink") {
		t.Fatalf("Apply() error = %v, want managed or symlink", err)
	}
}
