package detect

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

var errorType = types.Universe.Lookup("error").Type()

// RunMissingCode walks the root packages' syntax for "missing" signals that are self-contained
// (no graph needed): unimplemented stubs, TODO/FIXME markers, and unchecked error returns.
// Route-with-no-handler and untested-exports come later via codebase-memory composition.
func RunMissingCode(pkgs []*packages.Package) ([]Finding, error) {
	var findings []Finding
	for _, p := range pkgs { // pkgs are the roots; deps are not scanned
		if p.TypesInfo == nil || p.Fset == nil {
			continue
		}
		for _, file := range p.Syntax {
			findings = append(findings, scanFileForMissing(p, file)...)
		}
	}
	return findings, nil
}

func scanFileForMissing(p *packages.Package, file *ast.File) []Finding {
	var out []Finding
	at := func(n ast.Node) token.Position { return p.Fset.Position(n.Pos()) }

	// TODO / FIXME markers.
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if marker := commentMarker(c.Text); marker != "" {
				pos := at(c)
				out = append(out, Finding{
					Category: "missing", Detector: "todo", Severity: "low",
					File: pos.Filename, Line: pos.Line, Col: pos.Column,
					Message: strings.TrimSpace(strings.TrimLeft(c.Text, "/* \t")),
					Package: p.PkgPath,
				})
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			// Unimplemented stub: panic("not implemented")-style.
			if id, ok := node.Fun.(*ast.Ident); ok && id.Name == "panic" && len(node.Args) == 1 {
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s := strings.ToLower(lit.Value); isStubMessage(s) {
						pos := at(node)
						out = append(out, Finding{
							Category: "missing", Detector: "stub", Severity: "medium",
							File: pos.Filename, Line: pos.Line, Col: pos.Column,
							Message: "unimplemented stub: panic(" + lit.Value + ")",
							Package: p.PkgPath,
						})
					}
				}
			}
		case *ast.ExprStmt:
			// Unchecked error: a bare call statement whose (last) result is error.
			call, ok := node.X.(*ast.CallExpr)
			if ok && resultIsError(p, call) && !isBenignErrorCall(p, call) {
				pos := at(call)
				out = append(out, Finding{
					Category: "missing", Detector: "unchecked-error", Severity: "medium",
					File: pos.Filename, Line: pos.Line, Col: pos.Column,
					Message: "error return value is not checked",
					Package: p.PkgPath,
				})
			}
		}
		return true
	})
	return out
}

func commentMarker(text string) string {
	up := strings.ToUpper(text)
	for _, m := range []string{"TODO", "FIXME"} {
		if strings.Contains(up, m) {
			return m
		}
	}
	return ""
}

func isStubMessage(s string) bool {
	for _, needle := range []string{"not implemented", "unimplemented", "todo", "not yet"} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// resultIsError reports whether the call's (last) result is exactly the error interface.
func resultIsError(p *packages.Package, call *ast.CallExpr) bool {
	t := p.TypesInfo.TypeOf(call)
	if t == nil {
		return false
	}
	if tup, ok := t.(*types.Tuple); ok {
		if tup.Len() == 0 {
			return false
		}
		return types.Identical(tup.At(tup.Len()-1).Type(), errorType)
	}
	return types.Identical(t, errorType)
}

// isBenignErrorCall skips calls whose discarded error is idiomatic to ignore (the fmt printers),
// which keeps the unchecked-error detector's noise down.
func isBenignErrorCall(p *packages.Package, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := p.TypesInfo.ObjectOf(sel.Sel).(*types.Func)
	if !ok || fn.Pkg() == nil {
		return false
	}
	switch fn.Pkg().Path() + "." + fn.Name() {
	case "fmt.Print", "fmt.Printf", "fmt.Println",
		"fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln":
		return true
	}
	return false
}
