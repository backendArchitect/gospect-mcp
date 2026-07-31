package cbm

import (
	"context"
	"strings"
	"testing"
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

func TestCypherEscape(t *testing.T) {
	if got := cypherEscape("a'b"); got != "a''b" {
		t.Fatalf("cypherEscape: got %q", got)
	}
}
