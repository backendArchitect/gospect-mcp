package local

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/loader"
)

func funcBody(t *testing.T, src string) *ast.BlockStmt {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f.Decls[0].(*ast.FuncDecl).Body
}

func TestCyclomatic(t *testing.T) {
	if got := cyclomatic(funcBody(t, "func f() {}")); got != 1 {
		t.Errorf("empty func cyclomatic = %d, want 1", got)
	}
	// 1 base + if + for + (&&) = 4
	body := funcBody(t, "func f(x int) { if x > 0 && x < 9 { for {} } }")
	if got := cyclomatic(body); got != 4 {
		t.Errorf("cyclomatic = %d, want 4", got)
	}
}

func TestCognitive_NestingCosts(t *testing.T) {
	flat := cognitive(funcBody(t, "func f(x int) { if x>0 {} }"), 0)              // 1
	nested := cognitive(funcBody(t, "func f(x int) { if x>0 { if x>1 {} } }"), 0) // 1 + (1+1) = 3
	if flat != 1 {
		t.Errorf("flat if cognitive = %d, want 1", flat)
	}
	if nested <= flat {
		t.Errorf("nested cognitive (%d) should exceed flat (%d)", nested, flat)
	}
}

// TestLocalGraph_Integration loads a tiny module and checks the two standalone detectors.
func TestLocalGraph_Integration(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/p\n\ngo 1.21\n")
	write(t, filepath.Join(dir, "p.go"), `package p

func Tested() {}
func Lonely() {}
func Branchy(x int) int {
	if x > 0 {
		if x > 5 {
			return 1
		}
	} else if x < 0 {
		return -1
	}
	for i := 0; i < x; i++ {
		x += i
	}
	return x
}
`)
	write(t, filepath.Join(dir, "p_test.go"), "package p\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) { Tested() }\n")

	pkgs, _, err := loader.Load(dir, "./...")
	if err != nil {
		t.Fatal(err)
	}
	g := New(pkgs)

	// UntestedExports: Lonely and Branchy have no test reference; Tested does.
	syms, err := g.UntestedExports(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range syms {
		names[s.Name] = true
	}
	if !names["Lonely"] || !names["Branchy"] {
		t.Errorf("expected Lonely and Branchy untested, got %v", names)
	}
	if names["Tested"] {
		t.Errorf("Tested is referenced from a _test.go and should not be flagged untested")
	}

	// HighComplexity with low thresholds must flag Branchy.
	hot, err := g.HighComplexity(context.Background(), "", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, h := range hot {
		if h.Name == "Branchy" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Branchy in high-complexity hotspots, got %+v", hot)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
