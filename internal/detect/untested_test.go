package detect

import (
	"context"
	"errors"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

func TestRunUntestedExports(t *testing.T) {
	g := &graph.Fake{Untested: []graph.Symbol{
		{Name: "DoThing", QualifiedName: "pkg.DoThing", File: "a.go", Line: 10},
	}}
	fs, err := RunUntestedExports(context.Background(), g, "pkg")
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("want 1 finding, got %d", len(fs))
	}
	f := fs[0]
	if f.Detector != "untested-export" || f.Category != "missing" || f.Line != 10 || f.Package != "pkg.DoThing" {
		t.Fatalf("unexpected finding: %+v", f)
	}
}

func TestRunUntestedExports_PropagatesError(t *testing.T) {
	g := &graph.Fake{Err: errors.New("graph down")}
	if _, err := RunUntestedExports(context.Background(), g, "pkg"); err == nil {
		t.Fatal("expected error to propagate")
	}
}
