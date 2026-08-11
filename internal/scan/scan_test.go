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

	// One per detector we've implemented; testdata/buggy contains a case for each.
	for _, want := range []string{"nilness", "copylocks", "lostcancel", "stub", "unchecked-error", "todo", "go-version"} {
		if !got[want] {
			t.Errorf("expected a %q finding; got detectors %v (findings: %+v)", want, got, rep.Findings)
		}
	}
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
