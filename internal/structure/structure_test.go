package structure_test

import (
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

func TestNoRootProductionGoFiles(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("os.ReadDir returned error: %v", err)
	}
	var got []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		got = append(got, name)
	}
	if len(got) != 0 {
		t.Fatalf("root production Go files = %v, want none", got)
	}
}

func TestInternalPlatformHasNoHighLevelImports(t *testing.T) {
	root := repoRoot(t)
	pkg, err := build.ImportDir(filepath.Join(root, "internal", "platform"), 0)
	if err != nil {
		t.Fatalf("build.ImportDir returned error: %v", err)
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "/internal/app") ||
			strings.Contains(imp, "/internal/tunnel") ||
			strings.Contains(imp, "/internal/routing") ||
			strings.Contains(imp, "/internal/dns") {
			t.Fatalf("internal/platform imports high-level package %s", imp)
		}
	}
}

func TestLowerLevelPackagesDoNotImportApp(t *testing.T) {
	root := repoRoot(t)
	packages := []string{"awgnet", "config", "dns", "platform", "proxy", "routing", "tunnel"}
	for _, name := range packages {
		pkg, err := build.ImportDir(filepath.Join(root, "internal", name), 0)
		if err != nil {
			if strings.Contains(err.Error(), "no Go files") ||
				strings.Contains(err.Error(), `cannot find package "."`) {
				continue
			}
			t.Fatalf("build.ImportDir(%s) returned error: %v", name, err)
		}
		if slices.Contains(pkg.Imports, "awg-proxy/internal/app") {
			t.Fatalf("internal/%s imports internal/app", name)
		}
	}
}
