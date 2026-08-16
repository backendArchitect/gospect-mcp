package detect

import (
	"fmt"
	"go/ast"
	"strings"
	"sync"
	"unicode"

	"github.com/golangci/misspell"
	"golang.org/x/tools/go/packages"
)

// misspell's dictionary is large, so compile the replacer once and only on first use (this detector
// is -pedantic-gated, so most runs never touch it).
var (
	misspellOnce sync.Once
	misspellRepl *misspell.Replacer
)

func replacer() *misspell.Replacer {
	misspellOnce.Do(func() { misspellRepl = misspell.New() })
	return misspellRepl
}

// RunMisspell flags common English misspellings in comments and in function/type names. It uses the
// curated misspell dictionary — a list of KNOWN common typos, not a "is this a real word" check —
// so Go jargon (ctx, mux, unmarshal, cfg) is never flagged. Report-only: it never
// renames identifiers (that would break callers). Comments cover godoc and swagger/swaggo
// annotations. Opinionated hygiene, so it runs only under -pedantic.
func RunMisspell(pkgs []*packages.Package) ([]Finding, error) {
	var findings []Finding
	for _, p := range pkgs {
		if p.Fset == nil {
			continue
		}
		for _, file := range p.Syntax {
			for _, cg := range file.Comments {
				for _, c := range cg.List {
					findings = append(findings, misspellComment(p, c)...)
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch d := n.(type) {
				case *ast.FuncDecl:
					findings = append(findings, misspellIdent(p, d.Name, "function name")...)
				case *ast.TypeSpec:
					findings = append(findings, misspellIdent(p, d.Name, "type name")...)
				}
				return true
			})
		}
	}
	return findings, nil
}

func misspellComment(p *packages.Package, c *ast.Comment) []Finding {
	_, diffs := replacer().Replace(c.Text)
	if len(diffs) == 0 {
		return nil
	}
	base := p.Fset.Position(c.Pos())
	out := make([]Finding, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, Finding{
			Category: "typo", Detector: "misspell", Severity: "low", Confidence: "high",
			File: base.Filename, Line: base.Line + d.Line - 1,
			Message: fmt.Sprintf("possible misspelling in comment: %q → %q", d.Original, d.Corrected),
			Package: p.PkgPath,
		})
	}
	return out
}

func misspellIdent(p *packages.Package, id *ast.Ident, kind string) []Finding {
	var out []Finding
	for _, w := range splitIdentWords(id.Name) {
		corrected, diffs := replacer().Replace(w)
		if len(diffs) == 0 || corrected == w {
			continue
		}
		pos := p.Fset.Position(id.Pos())
		out = append(out, Finding{
			Category: "typo", Detector: "misspell", Severity: "low", Confidence: "high",
			File: pos.Filename, Line: pos.Line, Col: pos.Column,
			Message: fmt.Sprintf("possible misspelling in %s %q: %q → %q", kind, id.Name, diffs[0].Original, diffs[0].Corrected),
			Package: p.PkgPath,
		})
	}
	return out
}

// splitIdentWords breaks an identifier into its constituent words so each can be spell-checked:
// camelCase and snake_case boundaries, e.g. "CalculteTotal" → ["Calculte","Total"]. Acronym runs
// (HTTP) stay whole — they're never in the typo dictionary anyway.
func splitIdentWords(s string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		if r == '_' {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
			flush() // lower→Upper is a camelCase boundary
		}
		cur.WriteRune(r)
	}
	flush()
	return words
}
