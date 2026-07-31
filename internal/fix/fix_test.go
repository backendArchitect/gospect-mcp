package fix

import (
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
)

func TestBuild_KnownDetector(t *testing.T) {
	e := Build(detect.Finding{Detector: "nilness", Category: "bug", File: "a.go", Line: 8})
	if e.RootCause == "" || e.ExpectedScope == "" {
		t.Fatal("expected root cause and scope for a known detector")
	}
	if len(e.Constraints) == 0 {
		t.Fatal("expected ponytail constraints")
	}
	// The adversarial-first item must always lead the verify checklist.
	if len(e.VerifyFirst) == 0 || e.VerifyFirst[0] == "" {
		t.Fatal("expected verify-first checklist led by the adversarial item")
	}
	if e.Finding.Detector != "nilness" {
		t.Fatal("finding should be echoed back")
	}
}

func TestBuild_UnknownDetectorFallsBack(t *testing.T) {
	e := Build(detect.Finding{Detector: "totally-new-thing"})
	if e.RootCause == "" || e.ExpectedScope == "" || len(e.VerifyFirst) == 0 {
		t.Fatal("unknown detector should still get a generic envelope")
	}
}
