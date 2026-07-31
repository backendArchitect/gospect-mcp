package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	const body = "# noise\n*.pb.go\nmocks/\n\ndetector:todo\ndetector:go-version\n"
	if err := os.WriteFile(filepath.Join(dir, IgnoreFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	opt := loadIgnoreFile(dir)
	if len(opt.ExcludeGlob) != 2 {
		t.Fatalf("ExcludeGlob = %v, want 2 entries", opt.ExcludeGlob)
	}
	if len(opt.ExcludeDetectors) != 2 {
		t.Fatalf("ExcludeDetectors = %v, want 2 entries", opt.ExcludeDetectors)
	}
}

func TestLoadIgnoreFile_Missing(t *testing.T) {
	if !loadIgnoreFile(t.TempDir()).Empty() {
		t.Fatal("a missing .gospectignore should yield an empty (no-op) filter")
	}
}
