package scan

import (
	"bytes"
	"encoding/json"
	"testing"
)

// GitHub's SARIF upload validates against the 2.1.0 schema, where results and
// rules must be arrays. A clean scan has to emit [], never null, or the upload
// (and the gospect Action with sarif: true) fails on every clean run.
func TestSARIF_EmptyReportHasNoNulls(t *testing.T) {
	out, err := json.Marshal((&Report{Path: "."}).SARIF("v1.2.3"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("null")) {
		t.Fatalf("SARIF for a clean scan contains null: %s", out)
	}
}

func TestSARIF_Shape(t *testing.T) {
	r := sampleReport()
	r.Path = "."
	log := r.SARIF("v1.2.3")
	if log["version"] != "2.1.0" {
		t.Fatalf("version = %v, want 2.1.0", log["version"])
	}
	runs := log["runs"].([]map[string]any)
	results := runs[0]["results"].([]map[string]any)
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}
	// The high-severity bug must map to SARIF level "error".
	if results[0]["ruleId"] != "nilness" || results[0]["level"] != "error" {
		t.Fatalf("first result = %v/%v, want nilness/error", results[0]["ruleId"], results[0]["level"])
	}
}

func TestSarifLevel(t *testing.T) {
	for sev, want := range map[string]string{"high": "error", "medium": "warning", "low": "note", "": "note"} {
		if got := sarifLevel(sev); got != want {
			t.Errorf("sarifLevel(%q) = %q, want %q", sev, got, want)
		}
	}
}
