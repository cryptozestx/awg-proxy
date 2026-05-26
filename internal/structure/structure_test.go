package structure_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
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

func packageImports(t *testing.T, root string, name string) ([]string, bool) {
	t.Helper()
	dir := filepath.Join(root, "internal", name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		t.Fatalf("os.ReadDir(%s) returned error: %v", dir, err)
	}
	var imports []string
	var foundGoFiles bool
	for _, entry := range entries {
		fileName := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(fileName, ".go") || strings.HasSuffix(fileName, "_test.go") {
			continue
		}
		foundGoFiles = true
		filePath := filepath.Join(dir, fileName)
		file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parser.ParseFile(%s) returned error: %v", filePath, err)
		}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("strconv.Unquote(%s) returned error: %v", imp.Path.Value, err)
			}
			imports = append(imports, path)
		}
	}
	if !foundGoFiles {
		return nil, false
	}
	return imports, true
}

func importsPackage(importPath string, target string) bool {
	return importPath == target || strings.HasPrefix(importPath, target+"/")
}

func TestInternalPlatformHasNoHighLevelImports(t *testing.T) {
	root := repoRoot(t)
	imports, exists := packageImports(t, root, "platform")
	if !exists {
		return
	}
	disallowed := []string{
		"awg-proxy/internal/app",
		"awg-proxy/internal/tunnel",
		"awg-proxy/internal/routing",
		"awg-proxy/internal/dns",
	}
	for _, imp := range imports {
		for _, target := range disallowed {
			if importsPackage(imp, target) {
				t.Fatalf("internal/platform imports high-level package %s", imp)
			}
		}
	}
}

func TestLowerLevelPackagesDoNotImportApp(t *testing.T) {
	root := repoRoot(t)
	packages := []string{"awgnet", "config", "dns", "platform", "proxy", "routing", "tunnel"}
	for _, name := range packages {
		imports, exists := packageImports(t, root, name)
		if !exists {
			continue
		}
		for _, imp := range imports {
			if importsPackage(imp, "awg-proxy/internal/app") {
				t.Fatalf("internal/%s imports internal/app", name)
			}
		}
	}
}
