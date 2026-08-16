package loader

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSurvey_MultiModule verifies nested go.mod files are each discovered as a module root and
// that vendor/node_modules are pruned.
func TestSurvey_MultiModule(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module root\n")
	mustWrite(t, filepath.Join(dir, "svc-a", "go.mod"), "module a\n")
	mustWrite(t, filepath.Join(dir, "svc-b", "go.mod"), "module b\n")
	mustWrite(t, filepath.Join(dir, "vendor", "dep", "go.mod"), "module vendored\n") // must be skipped
	mustWrite(t, filepath.Join(dir, "svc-a", "main.go"), "package a\n")

	roots, hasGo, hint := survey(dir)
	if !hasGo {
		t.Fatal("hasGo = false, want true")
	}
	if hint != "" {
		t.Fatalf("hint = %q, want empty for a Go project", hint)
	}
	if len(roots) != 3 {
		t.Fatalf("found %d module roots, want 3 (vendored one must be pruned): %v", len(roots), roots)
	}
}

// TestSurvey_NonGo verifies a foreign project is recognized and hinted.
func TestSurvey_NonGo(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "package.json"), "{}\n")
	mustWrite(t, filepath.Join(dir, "src", "app.js"), "console.log(1)\n")

	roots, hasGo, hint := survey(dir)
	if hasGo || len(roots) != 0 {
		t.Fatalf("expected no Go; hasGo=%v roots=%v", hasGo, roots)
	}
	if hint == "" {
		t.Fatal("expected a language hint for a package.json project")
	}
}

// TestLoad_NotGoError verifies Load returns a typed NotGoError for a non-Go directory.
func TestLoad_NotGoError(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "composer.json"), "{}\n")

	_, _, err := Load(dir)
	var notGo *NotGoError
	if !errors.As(err, &notGo) {
		t.Fatalf("want *NotGoError, got %v", err)
	}
	if notGo.Hint == "" {
		t.Error("NotGoError should carry a PHP hint")
	}
}

// TestLoad_ParallelMultiModule verifies every module in a monorepo is loaded (concurrently) and
// reported in Stats.Roots.
func TestLoad_ParallelMultiModule(t *testing.T) {
	dir := t.TempDir()
	for _, m := range []string{"svc-a", "svc-b", "svc-c"} {
		mustWrite(t, filepath.Join(dir, m, "go.mod"), "module example.com/"+m+"\n\ngo 1.21\n")
		mustWrite(t, filepath.Join(dir, m, "main.go"), "package main\n\nfunc main() {}\n")
	}
	pkgs, stats, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Roots) != 3 {
		t.Fatalf("loaded %d module roots, want 3: %v", len(stats.Roots), stats.Roots)
	}
	if len(pkgs) < 3 {
		t.Fatalf("loaded %d packages, want at least 3 (one per module)", len(pkgs))
	}
	if len(stats.Skipped) != 0 {
		t.Fatalf("unexpected skips: %v", stats.Skipped)
	}
}

// TestLoad_GoWorkScopesToUseSet verifies that with a go.work present, only the workspace's `use`
// modules are loaded — a nested go.mod outside the workspace must not error the load or contribute.
func TestLoad_GoWorkScopesToUseSet(t *testing.T) {
	dir := t.TempDir()
	for _, m := range []string{"a", "b", "outside"} {
		mustWrite(t, filepath.Join(dir, m, "go.mod"), "module example.com/"+m+"\n\ngo 1.25\n")
		mustWrite(t, filepath.Join(dir, m, "x.go"), "package "+m+"\n\nfunc F() {}\n")
	}
	// The workspace uses only a and b; "outside" is a stray module.
	mustWrite(t, filepath.Join(dir, "go.work"), "go 1.25\n\nuse (\n\t./a\n\t./b\n)\n")

	pkgs, stats, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Roots) != 2 {
		t.Fatalf("loaded %d roots, want 2 (the use set): %v", len(stats.Roots), stats.Roots)
	}
	if stats.Errors != 0 || len(stats.Skipped) != 0 {
		t.Fatalf("workspace load had errors/skips: errors=%d skipped=%v", stats.Errors, stats.Skipped)
	}
	for _, p := range pkgs {
		if p.PkgPath == "example.com/outside" {
			t.Fatalf("scanned a module outside the workspace use set: %s", p.PkgPath)
		}
	}
}

func TestLoadConcurrency(t *testing.T) {
	// Never below 2, never above the module count.
	if got := loadConcurrency(1); got != 1 {
		t.Errorf("loadConcurrency(1) = %d, want 1", got)
	}
	if got := loadConcurrency(100); got < 2 {
		t.Errorf("loadConcurrency(100) = %d, want >= 2", got)
	}
}

// TestLoadChanged_ScopesToChangedPackages verifies diff mode loads only the package(s) containing
// changed files, attributing each to the right nested module.
func TestLoadChanged_ScopesToChangedPackages(t *testing.T) {
	dir := t.TempDir()
	// Two modules; svc-a has two packages, only one of which "changed".
	mustWrite(t, filepath.Join(dir, "svc-a", "go.mod"), "module example.com/a\n\ngo 1.21\n")
	mustWrite(t, filepath.Join(dir, "svc-a", "main.go"), "package main\n\nfunc main() {}\n")
	mustWrite(t, filepath.Join(dir, "svc-a", "util", "util.go"), "package util\n\nfunc F() {}\n")
	mustWrite(t, filepath.Join(dir, "svc-b", "go.mod"), "module example.com/b\n\ngo 1.21\n")
	mustWrite(t, filepath.Join(dir, "svc-b", "main.go"), "package main\n\nfunc main() {}\n")

	changed := []string{filepath.Join(dir, "svc-a", "util", "util.go")}
	pkgs, stats, err := LoadChanged(dir, changed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("loaded %d packages, want exactly 1 (the changed util package)", len(pkgs))
	}
	if pkgs[0].Name != "util" {
		t.Fatalf("loaded package %q, want util", pkgs[0].Name)
	}
	if len(stats.Roots) != 1 {
		t.Fatalf("touched %d module roots, want 1 (svc-a)", len(stats.Roots))
	}
}

func TestLoadChanged_Empty(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module example.com/x\n\ngo 1.21\n")
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	// No .go files in the changed set -> nothing to load, no error.
	pkgs, _, err := LoadChanged(dir, []string{filepath.Join(dir, "README.md")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages for a non-Go change, got %d", len(pkgs))
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
