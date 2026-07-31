package detect

import (
	"context"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

func TestRunOverEngineering(t *testing.T) {
	g := &graph.Fake{HotSpots: []graph.HotSpot{
		{Symbol: graph.Symbol{Name: "Gnarly", QualifiedName: "pkg.Gnarly", File: "a.go", Line: 5}, Cyclomatic: 45, Cognitive: 60},
		{Symbol: graph.Symbol{Name: "Meh", QualifiedName: "pkg.Meh", File: "b.go", Line: 9}, Cyclomatic: 22, Cognitive: 10},
	}}
	fs, err := RunOverEngineering(context.Background(), g, "pkg", 20, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 {
		t.Fatalf("want 2 findings, got %d", len(fs))
	}
	// Gnarly is >= 2x thresholds -> high; Meh is over one threshold but below 2x -> medium.
	got := map[string]string{}
	for _, f := range fs {
		if f.Category != "over-engineered" || f.Detector != "high-complexity" {
			t.Fatalf("unexpected finding: %+v", f)
		}
		got[f.Package] = f.Severity
	}
	if got["pkg.Gnarly"] != "high" || got["pkg.Meh"] != "medium" {
		t.Fatalf("unexpected severities: %v", got)
	}
}
