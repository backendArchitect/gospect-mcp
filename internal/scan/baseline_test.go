package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyBaseline_HidesKnown(t *testing.T) {
	r := sampleReport()
	for i := range r.Findings { // give them fingerprints like a real scan
		r.Findings[i].Fingerprint = fingerprint(".", r.Findings[i])
	}
	r.recount()

	// Baseline everything except the bug, then confirm only the bug remains.
	base := map[string]bool{}
	for _, f := range r.Findings {
		if f.Category != "bug" {
			base[f.Fingerprint] = true
		}
	}
	hidden := r.ApplyBaseline(base)
	if hidden != 3 || r.FindingCount != 1 || r.Findings[0].Category != "bug" {
		t.Fatalf("hidden=%d count=%d first=%s; want 3/1/bug", hidden, r.FindingCount, r.Findings[0].Category)
	}
}

func TestLoadBaseline_RoundTrip(t *testing.T) {
	r := sampleReport()
	for i := range r.Findings {
		r.Findings[i].Fingerprint = fingerprint(".", r.Findings[i])
	}
	path := filepath.Join(t.TempDir(), "base.json")
	data, _ := json.Marshal(r)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != len(r.Findings) {
		t.Fatalf("loaded %d fingerprints, want %d", len(set), len(r.Findings))
	}
}
