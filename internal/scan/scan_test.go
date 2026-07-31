package scan

import (
	"path/filepath"
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
	for _, want := range []string{"nilness", "stub", "unchecked-error", "todo", "go-version"} {
		if !got[want] {
			t.Errorf("expected a %q finding; got detectors %v (findings: %+v)", want, got, rep.Findings)
		}
	}
}
