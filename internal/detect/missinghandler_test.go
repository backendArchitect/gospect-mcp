package detect

import (
	"context"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

func TestRunMissingHandlers(t *testing.T) {
	g := &graph.Fake{Routes: []graph.Route{
		{Method: "GET", Path: "/widgets", QualifiedName: "__route__GET__/widgets"},
	}}
	fs, err := RunMissingHandlers(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("want 1 finding, got %d", len(fs))
	}
	if fs[0].Detector != "unhandled-route" || fs[0].Category != "missing" {
		t.Fatalf("unexpected finding: %+v", fs[0])
	}
}
