package scan

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestScan_Detectors is the end-to-end proof: load a real (separate) module and confirm each
// deterministic detector fires on the deliberate issues in testdata/buggy.
func TestScan_Detectors(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "buggy")

	rep, err := Scan(dir, "./...")
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if rep.PackagesLoaded == 0 {
		t.Fatalf("expected to load packages, got 0 (load errors: %d)", rep.LoadErrors)
	}

	got := map[string]bool{}
	for _, f := range rep.Findings {
		got[f.Detector] = true
	}

	// The DEFAULT set is high-signal only: bug analyzers + stub. It must NOT include the pedantic
	// heuristics (they're noisy on real code — see the real-world tuning).
	for _, want := range []string{
		"nilness", "copylocks", "lostcancel", "stub",
		"printf", "atomic", "sortslice", "unusedresult", "stringintconv", "timeformat",
		"sigchanyzer", "appends", "shift", "bools",
	} {
		if !got[want] {
			t.Errorf("expected a %q finding by default; got detectors %v", want, keys(got))
		}
	}
	for _, notWant := range []string{"unchecked-error", "todo", "go-version", "high-complexity"} {
		if got[notWant] {
			t.Errorf("detector %q should be off by default (pedantic-only), but it fired", notWant)
		}
	}
}

// TestScan_Pedantic proves the -pedantic heuristics are added on opt-in and absent by default.
func TestScan_Pedantic(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "buggy")
	rep, err := ScanWithOptions(dir, Options{Patterns: []string{"./..."}, Pedantic: true})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	got := map[string]bool{}
	for _, f := range rep.Findings {
		got[f.Detector] = true
	}
	for _, want := range []string{"unchecked-error", "todo", "go-version"} {
		if !got[want] {
			t.Errorf("expected %q under -pedantic; got %v", want, keys(got))
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestScan_Staticcheck verifies the opt-in staticcheck pass adds SA-prefixed findings that the
// default run does not.
func TestScan_Staticcheck(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "buggy")
	rep, err := ScanWithOptions(dir, Options{Patterns: []string{"./..."}, Staticcheck: true})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	var sa bool
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.Detector, "SA") {
			sa = true
			break
		}
	}
	if !sa {
		t.Errorf("expected at least one staticcheck (SA*) finding with -staticcheck; got %+v", rep.Findings)
	}

	// Default (no staticcheck) must NOT include SA findings.
	def, err := Scan(dir, "./...")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range def.Findings {
		if strings.HasPrefix(f.Detector, "SA") {
			t.Errorf("default scan should not run staticcheck, but found %s", f.Detector)
		}
	}
}
