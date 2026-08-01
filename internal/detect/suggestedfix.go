package detect

import (
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// TextEdit is a resolved edit (byte offsets into a file) from an analyzer's SuggestedFix.
type TextEdit struct {
	File    string
	Start   int
	End     int
	NewText string
}

// SuggestedEdits runs the analyzer that could have produced target and returns the edits of its
// suggested fix — but ONLY when the analyzer offers exactly ONE fix. Multiple fixes are ambiguous
// alternatives (e.g. SA4013 offers both "!b" and "b" for "!!b"), and picking one blindly can change
// behavior in a way the verify harness can't catch, so those are left to a human or an AI agent.
// This is why deterministic coverage is intentionally conservative: ok is false for ambiguous or
// fixless findings.
func SuggestedEdits(pkgs []*packages.Package, target Finding) (edits []TextEdit, ok bool) {
	analyzers := analyzersFor(target.Detector)
	if len(analyzers) == 0 {
		return nil, false
	}
	graph, err := checker.Analyze(analyzers, pkgs, nil)
	if err != nil {
		return nil, false
	}
	for act := range graph.All() {
		if !act.IsRoot || act.Err != nil || act.Analyzer.Name != target.Detector {
			continue
		}
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			// Exactly one fix — see the doc comment on ambiguity.
			if !matchesLoc(pos.Filename, pos.Line, target) || len(d.SuggestedFixes) != 1 {
				continue
			}
			for _, e := range d.SuggestedFixes[0].TextEdits {
				s := act.Package.Fset.Position(e.Pos)
				en := act.Package.Fset.Position(e.End)
				edits = append(edits, TextEdit{File: s.Filename, Start: s.Offset, End: en.Offset, NewText: string(e.NewText)})
			}
			if len(edits) > 0 {
				return edits, true
			}
		}
	}
	return nil, false
}

// analyzersFor returns the analyzer set that can produce a finding with the given detector name.
func analyzersFor(detector string) []*analysis.Analyzer {
	if strings.HasPrefix(detector, "SA") {
		_, list := staticcheckSet()
		return list
	}
	for a := range bugAnalyzers {
		if a.Name == detector {
			return []*analysis.Analyzer{a}
		}
	}
	return nil
}

// matchesLoc reports whether a diagnostic position corresponds to the target finding. The caller has
// scoped analysis to the finding's package (unique file basenames), so basename + line suffices.
func matchesLoc(file string, line int, target Finding) bool {
	if filepath.Base(file) != filepath.Base(target.File) {
		return false
	}
	return target.Line == 0 || line == target.Line
}
