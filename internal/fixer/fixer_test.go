package fixer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/agent"
	"github.com/backendArchitect/gospect-mcp/internal/detect"
	"github.com/backendArchitect/gospect-mcp/internal/fix"
)

func TestBuildPrompt(t *testing.T) {
	p := buildPrompt(fix.Build(detect.Finding{Detector: "nilness", File: "buggy.go", Line: 8, Message: "nil dereference in load"}))
	for _, want := range []string{"nilness", "nil dereference in load", "buggy.go:8", "Verify it's REAL", "Do NOT create a git commit"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n%s", want, p)
		}
	}
}

func TestModuleRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0o755)
	if got := moduleRoot(filepath.Join(sub, "x.go")); got != dir {
		t.Fatalf("moduleRoot = %q, want %q", got, dir)
	}
	if got := moduleRoot(filepath.Join(t.TempDir(), "y.go")); got != "" {
		t.Fatalf("moduleRoot with no go.mod = %q, want empty", got)
	}
}

func TestApplyEdits(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.txt")
	writeFile(t, f, "hello world")
	// Two edits given out of order; applyEdits must apply them back-to-front so offsets stay valid.
	err := applyEdits([]detect.TextEdit{
		{File: f, Start: 6, End: 11, NewText: "gophers"},
		{File: f, Start: 0, End: 5, NewText: "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(f)
	if string(got) != "hi gophers" {
		t.Fatalf("applyEdits = %q, want %q", got, "hi gophers")
	}
}

func TestApplyEdits_OutOfBounds(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.txt")
	writeFile(t, f, "abc")
	if err := applyEdits([]detect.TextEdit{{File: f, Start: 0, End: 99, NewText: "x"}}); err == nil {
		t.Fatal("expected an out-of-bounds error")
	}
}

func TestNewFindings(t *testing.T) {
	before := []detect.Finding{{Fingerprint: "a"}, {Fingerprint: "b"}}
	after := []detect.Finding{{Fingerprint: "a"}, {Fingerprint: "c"}} // b fixed, c is new
	got := newFindings(before, after, fingerprintSet(before))
	if len(got) != 1 || got[0].Fingerprint != "c" {
		t.Fatalf("newFindings = %+v, want just c", got)
	}
}

// TestFix_Integration exercises the full apply / verify / rollback loop with a scripted "agent".
func TestFix_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell agents")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	newRepo := func(t *testing.T) string {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.21\n")
		writeFile(t, filepath.Join(dir, "app.go"), "package app\n\n// TODO: x\nfunc F() int { return 1 }\n")
		run(t, dir, "git", "init", "-q")
		run(t, dir, "git", "config", "user.email", "t@t")
		run(t, dir, "git", "config", "user.name", "t")
		run(t, dir, "git", "add", "-A")
		run(t, dir, "git", "commit", "-qm", "init")
		return dir
	}
	todo := detect.Finding{Detector: "todo", Severity: "low"}

	t.Run("good fix kept", func(t *testing.T) {
		dir := newRepo(t)
		todo.File = filepath.Join(dir, "app.go")
		script := writeScript(t, "sed -i '/TODO/d' app.go\n")
		ag, _ := agent.Parse(script + " {prompt}")
		res, err := Fix(context.Background(), dir, Options{Finding: todo, Agent: ag})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Applied {
			t.Fatalf("expected applied, got %+v", res)
		}
		if st := status(t, dir); st == "" {
			t.Fatal("expected the fix to remain in the working tree")
		}
	})

	t.Run("deterministic safe fix", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.21\n")
		writeFile(t, filepath.Join(dir, "app.go"), "package app\n\nimport \"fmt\"\n\nfunc F(s string) { fmt.Printf(s) }\n")
		run(t, dir, "git", "init", "-q")
		run(t, dir, "git", "config", "user.email", "t@t")
		run(t, dir, "git", "config", "user.name", "t")
		run(t, dir, "git", "add", "-A")
		run(t, dir, "git", "commit", "-qm", "init")

		// SA1006 (Printf with a dynamic format) has exactly one suggested fix -> deterministically applied.
		f := detect.Finding{Detector: "SA1006", File: filepath.Join(dir, "app.go"), Line: 5}
		res, err := Fix(context.Background(), dir, Options{Finding: f, Deterministic: true})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Applied {
			t.Fatalf("expected the deterministic fix to apply, got %+v", res)
		}
	})

	t.Run("build break rolled back", func(t *testing.T) {
		dir := newRepo(t)
		todo.File = filepath.Join(dir, "app.go")
		script := writeScript(t, "sed -i '/TODO/d' app.go\necho 'not valid go' >> app.go\n")
		ag, _ := agent.Parse(script + " {prompt}")
		res, err := Fix(context.Background(), dir, Options{Finding: todo, Agent: ag})
		if err != nil {
			t.Fatal(err)
		}
		if res.Applied || !res.RolledBack {
			t.Fatalf("expected rollback, got %+v", res)
		}
		if st := status(t, dir); st != "" {
			t.Fatalf("working tree not restored after rollback: %q", st)
		}
	})
}

// --- helpers ---

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(p, []byte("#!/bin/bash\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}

func status(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
