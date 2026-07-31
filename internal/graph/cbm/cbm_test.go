package cbm

import (
	"context"
	"strings"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

// fakeCaller returns canned query_graph JSON, matched by a substring of the Cypher. It mirrors
// the exact response shape of a real codebase-memory-mcp server.
type fakeCaller struct{ byQuery map[string]string }

func (f *fakeCaller) CallTool(name string, args any) (string, error) {
	q := args.(map[string]any)["query"].(string)
	for key, resp := range f.byQuery {
		if strings.Contains(q, key) {
			return resp, nil
		}
	}
	return `{"columns":[],"rows":[]}`, nil
}

func TestUntestedExports_SetDifference(t *testing.T) {
	fc := &fakeCaller{byQuery: map[string]string{
		// A: exported, non-test functions
		"f.is_exported = 'true'": `{"columns":["qn","name","file","line"],"rows":[["pkg.A","A","a.go","10"],["pkg.B","B","b.go","20"]]}`,
		// B: functions that ARE tested
		"[:TESTS]": `{"columns":["qn"],"rows":[["pkg.B"]]}`,
	}}

	syms, err := New(fc, "proj").UntestedExports(context.Background(), "pkg")
	if err != nil {
		t.Fatal(err)
	}
	// B is tested, so only A should remain.
	if len(syms) != 1 {
		t.Fatalf("want 1 untested, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "A" || syms[0].QualifiedName != "pkg.A" || syms[0].File != "a.go" || syms[0].Line != 10 {
		t.Fatalf("unexpected symbol: %+v", syms[0])
	}
}

func TestHighComplexity_ThresholdAndLabels(t *testing.T) {
	fc := &fakeCaller{byQuery: map[string]string{
		// Function label: one over, one under threshold
		"(f:Function)": `{"columns":["qn","name","file","line","cyc","cog"],"rows":[["p.Big","Big","a.go","5","30","10"],["p.Small","Small","a.go","40","3","2"]]}`,
		// Method label: one over on cognitive only
		"(f:Method)": `{"columns":["qn","name","file","line","cyc","cog"],"rows":[["p.M","M","b.go","7","1","40"]]}`,
	}}
	spots, err := New(fc, "proj").HighComplexity(context.Background(), "scope", 20, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(spots) != 2 {
		t.Fatalf("want 2 hotspots (Big + M), got %d: %+v", len(spots), spots)
	}
	byName := map[string]graph.HotSpot{}
	for _, s := range spots {
		byName[s.Name] = s
	}
	if byName["Big"].Cyclomatic != 30 || byName["M"].Cognitive != 40 {
		t.Fatalf("unexpected metrics: %+v", spots)
	}
	if _, ok := byName["Small"]; ok {
		t.Fatal("Small is under both thresholds and should be excluded")
	}
}

func TestCypherEscape(t *testing.T) {
	if got := cypherEscape("a'b"); got != "a''b" {
		t.Fatalf("cypherEscape: got %q", got)
	}
}
