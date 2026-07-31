package detect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

func TestRunStaleSwagger(t *testing.T) {
	dir := t.TempDir()
	// Documented: /reservations/{id} (exists in code, base-path-prefixed) and /legacy (removed).
	os.WriteFile(filepath.Join(dir, "swagger.json"),
		[]byte(`{"paths":{"/reservations/{id}":{"get":{}},"/legacy":{"get":{}}}}`), 0o644)

	g := &graph.Fake{AllRoutes: []graph.Route{
		// router registered it under an /api/v2 group — suffix match should still find it.
		{Method: "GET", Path: "/api/v2/reservations/:id"},
	}}

	fs, err := RunStaleSwagger(context.Background(), g, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("want 1 stale finding (/legacy), got %d: %+v", len(fs), fs)
	}
	if fs[0].Detector != "swagger-drift" || fs[0].Category != "stale-doc" {
		t.Fatalf("unexpected: %+v", fs[0])
	}
	if got := fs[0].Message; !strings.Contains(got, "/legacy") {
		t.Fatalf("expected message about /legacy, got %q", got)
	}
}
