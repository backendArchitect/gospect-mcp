package gate

import (
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
)

var sample = []detect.Finding{
	{Detector: "nilness", Severity: "high"},
	{Detector: "unchecked-error", Severity: "medium"},
	{Detector: "todo", Severity: "low"},
}

func TestEvaluate_Threshold(t *testing.T) {
	// fail-on high: only the nilness high blocks.
	if r := Evaluate(sample, Policy{FailOn: "high"}); len(r.Blocking) != 1 || r.Blocking[0].Detector != "nilness" {
		t.Fatalf("fail-on high: want 1 blocking (nilness), got %+v", r.Blocking)
	}
	// fail-on medium: high + medium block.
	if r := Evaluate(sample, Policy{FailOn: "medium"}); len(r.Blocking) != 2 {
		t.Fatalf("fail-on medium: want 2 blocking, got %d", len(r.Blocking))
	}
	// fail-on low: everything blocks.
	if r := Evaluate(sample, Policy{FailOn: "low"}); r.Pass() || len(r.Blocking) != 3 {
		t.Fatalf("fail-on low: want 3 blocking, got %d", len(r.Blocking))
	}
	// empty FailOn defaults to high.
	if r := Evaluate(sample, Policy{}); len(r.Blocking) != 1 {
		t.Fatalf("default: want 1 blocking, got %d", len(r.Blocking))
	}
}

func TestEvaluate_Ignore(t *testing.T) {
	r := Evaluate(sample, Policy{FailOn: "low", Ignore: map[string]bool{"todo": true, "nilness": true}})
	if len(r.Blocking) != 1 || r.Blocking[0].Detector != "unchecked-error" {
		t.Fatalf("ignore: want only unchecked-error blocking, got %+v", r.Blocking)
	}
	if r.Total != 3 {
		t.Fatalf("Total should count all findings incl. ignored, got %d", r.Total)
	}
}
