// Package local implements graph.Graph directly from loaded Go packages — no external
// code-intelligence server. It answers the two whole-repo questions that don't need a full call
// graph: which exported functions have no test, and which functions are overly complex. Route-based
// questions (UnhandledRoutes/Routes) return empty here; those need the richer external graph.
package local

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
	"golang.org/x/tools/go/packages"
)

// Local is a graph.Graph computed from the packages a scan already loaded.
type Local struct {
	pkgs []*packages.Package
}

var _ graph.Graph = (*Local)(nil)

// New builds a local graph over the given loaded packages.
func New(pkgs []*packages.Package) *Local { return &Local{pkgs: pkgs} }

// UntestedExports returns exported, non-test functions whose name is never referenced from a
// _test.go file in the same directory. Test files aren't in the loaded (Tests=false) package, so
// they're parsed from disk. Name-based matching is a heuristic — hence the detector's low
// severity / medium confidence — but it catches genuinely untouched exports well.
func (l *Local) UntestedExports(_ context.Context, scope string) ([]graph.Symbol, error) {
	testedByDir := map[string]map[string]bool{}
	var out []graph.Symbol
	for _, pkg := range l.pkgs {
		if len(pkg.GoFiles) == 0 || pkg.Fset == nil {
			continue
		}
		dir := filepath.Dir(pkg.GoFiles[0])
		tested, ok := testedByDir[dir]
		if !ok {
			tested = testedNames(dir)
			testedByDir[dir] = tested
		}
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil || !fn.Name.IsExported() {
					continue
				}
				name := fn.Name.Name
				if name == "" || tested[name] {
					continue
				}
				pos := pkg.Fset.Position(fn.Pos())
				out = append(out, graph.Symbol{
					Name:          name,
					QualifiedName: pkg.PkgPath + "." + name,
					File:          pos.Filename,
					Line:          pos.Line,
				})
			}
		}
	}
	return out, nil
}

// HighComplexity returns non-test functions whose cyclomatic OR cognitive complexity meets the
// given minimums, computed from the AST.
func (l *Local) HighComplexity(_ context.Context, scope string, minCyclomatic, minCognitive int) ([]graph.HotSpot, error) {
	var out []graph.HotSpot
	for _, pkg := range l.pkgs {
		if pkg.Fset == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				cyc := cyclomatic(fn.Body)
				cog := cognitive(fn.Body, 0)
				if cyc < minCyclomatic && cog < minCognitive {
					continue
				}
				pos := pkg.Fset.Position(fn.Pos())
				out = append(out, graph.HotSpot{
					Symbol: graph.Symbol{
						Name:          funcName(fn),
						QualifiedName: pkg.PkgPath + "." + funcName(fn),
						File:          pos.Filename,
						Line:          pos.Line,
					},
					Cyclomatic: cyc,
					Cognitive:  cog,
				})
			}
		}
	}
	return out, nil
}

// UnhandledRoutes / Routes need HTTP-route knowledge the local graph doesn't build yet.
func (l *Local) UnhandledRoutes(context.Context) ([]graph.Route, error) { return nil, nil }
func (l *Local) Routes(context.Context) ([]graph.Route, error)          { return nil, nil }

// testedNames parses the _test.go files in dir and returns the set of identifiers they reference.
// A referenced identifier includes selector fields (pkg.Foo -> "Foo"), so it catches both in-package
// and external (_test package) test calls.
func testedNames(dir string) map[string]bool {
	set := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return set
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				set[id.Name] = true
			}
			return true
		})
	}
	return set
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		return recvType(fn.Recv.List[0].Type) + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func recvType(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return "*" + recvType(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic receiver Foo[T]
		return recvType(t.X)
	}
	return ""
}

// cyclomatic is McCabe complexity: 1 + one per decision point.
func cyclomatic(body *ast.BlockStmt) int {
	c := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			c++
		case *ast.BinaryExpr:
			if s.Op == token.LAND || s.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}

// cognitive is a nesting-weighted complexity approximation (SonarSource-style): control-flow
// structures cost 1 + their nesting depth, logical-operator sequences and labeled jumps cost 1.
func cognitive(n ast.Node, nesting int) int {
	total := 0
	switch s := n.(type) {
	case *ast.BlockStmt:
		for _, st := range s.List {
			total += cognitive(st, nesting)
		}
	case *ast.IfStmt:
		total += 1 + nesting + logicalOps(s.Cond)
		total += cognitive(s.Body, nesting+1)
		switch e := s.Else.(type) {
		case *ast.IfStmt: // else-if: +1, no extra nesting for the chain
			total += 1 + logicalOps(e.Cond) + cognitive(e.Body, nesting+1)
			total += cognitiveElse(e.Else, nesting)
		case *ast.BlockStmt: // else
			total += 1 + cognitive(e, nesting+1)
		}
	case *ast.ForStmt:
		total += 1 + nesting + logicalOps(s.Cond) + cognitive(s.Body, nesting+1)
	case *ast.RangeStmt:
		total += 1 + nesting + cognitive(s.Body, nesting+1)
	case *ast.SwitchStmt:
		total += 1 + nesting + cognitive(s.Body, nesting+1)
	case *ast.TypeSwitchStmt:
		total += 1 + nesting + cognitive(s.Body, nesting+1)
	case *ast.SelectStmt:
		total += 1 + nesting + cognitive(s.Body, nesting+1)
	case *ast.CaseClause:
		for _, st := range s.Body {
			total += cognitive(st, nesting)
		}
	case *ast.CommClause:
		for _, st := range s.Body {
			total += cognitive(st, nesting)
		}
	case *ast.BranchStmt:
		if s.Label != nil { // labeled break/continue/goto
			total += 1
		}
	case *ast.LabeledStmt:
		total += cognitive(s.Stmt, nesting)
	}
	return total
}

// cognitiveElse handles the tail of an else-if chain.
func cognitiveElse(else_ ast.Stmt, nesting int) int {
	switch e := else_.(type) {
	case *ast.IfStmt:
		return 1 + logicalOps(e.Cond) + cognitive(e.Body, nesting+1) + cognitiveElse(e.Else, nesting)
	case *ast.BlockStmt:
		return 1 + cognitive(e, nesting+1)
	}
	return 0
}

// logicalOps counts &&/|| operators in an expression (each adds cognitive load). A nil expression
// (e.g. a `for {}` with no condition) contributes nothing.
func logicalOps(e ast.Expr) int {
	if e == nil {
		return 0
	}
	n := 0
	ast.Inspect(e, func(node ast.Node) bool {
		if b, ok := node.(*ast.BinaryExpr); ok && (b.Op == token.LAND || b.Op == token.LOR) {
			n++
		}
		return true
	})
	return n
}
