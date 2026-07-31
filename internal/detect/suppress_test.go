package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplySuppressions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.go")
	// line 1: plain; line 2: bare ignore; line 3: scoped ignore (nilness only); line 4: scoped miss.
	const body = "package x\nrisky() //gospect:ignore\nfoo() //gospect:ignore nilness\nbar() //gospect:ignore nilness\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	in := []Finding{
		{Detector: "nilness", File: src, Line: 1},   // no directive -> kept
		{Detector: "nilness", File: src, Line: 2},   // bare directive -> dropped
		{Detector: "nilness", File: src, Line: 3},   // scoped, matches -> dropped
		{Detector: "unmarshal", File: src, Line: 4}, // scoped nilness, detector differs -> kept
	}
	kept, suppressed := ApplySuppressions(in)

	if suppressed != 2 {
		t.Fatalf("suppressed = %d, want 2", suppressed)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %d findings, want 2", len(kept))
	}
	for _, f := range kept {
		if f.Line == 2 || f.Line == 3 {
			t.Errorf("finding on line %d should have been suppressed", f.Line)
		}
	}
}

func TestApplySuppressions_DirectiveOnLineAbove(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "y.go")
	const body = "package y\n//gospect:ignore\nrisky()\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, suppressed := ApplySuppressions([]Finding{{Detector: "nilness", File: src, Line: 3}})
	if suppressed != 1 {
		t.Fatalf("directive on the line above should suppress; suppressed = %d", suppressed)
	}
}
