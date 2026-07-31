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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
