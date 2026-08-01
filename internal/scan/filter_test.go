package scan

import (
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
)

func sampleReport() *Report {
	fs := []detect.Finding{
		{Category: "bug", Detector: "nilness", Severity: "high", File: "a.go", Line: 3},
		{Category: "missing", Detector: "todo", Severity: "low", File: "b.go", Line: 1},
		{Category: "missing", Detector: "unchecked-error", Severity: "medium", File: "mocks/m.go", Line: 9},
		{Category: "modernize", Detector: "go-version", Severity: "low", File: "gen.pb.go", Line: 1},
	}
	sortFindings(fs)
	return &Report{Findings: fs, FindingCount: len(fs)}
}

func TestSortFindings_BugsOnTop(t *testing.T) {
	r := sampleReport()
	if r.Findings[0].Category != "bug" {
		t.Fatalf("first finding should be the bug, got %q", r.Findings[0].Category)
	}
}

func TestApply_MinSeverity(t *testing.T) {
	r := sampleReport()
	removed := r.Apply(FilterOptions{MinSeverity: "medium"})
	if removed != 2 { // the two "low" findings drop
		t.Fatalf("removed = %d, want 2", removed)
	}
	for _, f := range r.Findings {
		if severityRank(f.Severity) < severityRank("medium") {
			t.Errorf("low-severity finding survived: %+v", f)
		}
	}
}

func TestApply_MinConfidence(t *testing.T) {
	r := &Report{Findings: []detect.Finding{
		{Detector: "nilness", Severity: "high", Confidence: "high", File: "a.go"},
		{Detector: "unchecked-error", Severity: "medium", Confidence: "medium", File: "b.go"},
		{Detector: "ineffassign", Severity: "low", Confidence: "low", File: "c.go"},
	}}
	r.recount()
	removed := r.Apply(FilterOptions{MinConfidence: "high"})
	if removed != 2 || r.FindingCount != 1 || r.Findings[0].Detector != "nilness" {
		t.Fatalf("min-confidence=high: removed=%d kept=%d; want 2 removed, only nilness", removed, r.FindingCount)
	}
}

func TestApply_CategoryAndDetector(t *testing.T) {
	r := sampleReport()
	r.Apply(FilterOptions{Categories: []string{"bug"}})
	if r.FindingCount != 1 || r.Findings[0].Detector != "nilness" {
		t.Fatalf("category filter kept %d findings, want just nilness", r.FindingCount)
	}
}

func TestApply_ExcludeGlob(t *testing.T) {
	r := sampleReport()
	r.Apply(FilterOptions{ExcludeGlob: []string{"*.pb.go", "mocks/"}})
	if r.FindingCount != 2 {
		t.Fatalf("exclude kept %d findings, want 2 (a.go, b.go)", r.FindingCount)
	}
	for _, f := range r.Findings {
		if f.File == "gen.pb.go" || f.File == "mocks/m.go" {
			t.Errorf("excluded file survived: %s", f.File)
		}
	}
}

func TestApply_Empty_NoOp(t *testing.T) {
	r := sampleReport()
	if removed := r.Apply(FilterOptions{}); removed != 0 || r.FindingCount != 4 {
		t.Fatalf("empty filter changed the report: removed=%d count=%d", removed, r.FindingCount)
	}
}
