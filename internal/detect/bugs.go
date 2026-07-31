package detect

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/packages"
)

type analyzerMeta struct {
	category string
	severity string
}

// bugAnalyzers is the curated, high-precision analyzer set (bug-only, not style). Each entry
// carries the category/severity we report it under. All ship in golang.org/x/tools — no extra deps.
var bugAnalyzers = map[*analysis.Analyzer]analyzerMeta{
	nilness.Analyzer:      {"bug", "high"},         // SSA nil dereference
	lostcancel.Analyzer:   {"bug", "high"},         // context CancelFunc never called -> leak
	httpresponse.Analyzer: {"bug", "high"},         // using resp before checking err / not closing body
	unmarshal.Analyzer:    {"bug", "high"},         // non-pointer passed to Unmarshal
	copylock.Analyzer:     {"bug", "high"},         // a value containing a lock is copied
	errorsas.Analyzer:     {"bug", "high"},         // errors.As target is not a pointer to an error
	nilfunc.Analyzer:      {"bug", "medium"},       // useless comparison of func value to nil
	unreachable.Analyzer:  {"bug", "medium"},       // unreachable code
	loopclosure.Analyzer:  {"modernize", "medium"}, // pre-1.22 loop-var capture (no-op on go>=1.22)
}

// RunBugDetectors runs the analyzer set over already-loaded packages and maps each diagnostic
// to a Finding. It reports; it never edits.
func RunBugDetectors(pkgs []*packages.Package) ([]Finding, error) {
	analyzers := make([]*analysis.Analyzer, 0, len(bugAnalyzers))
	for a := range bugAnalyzers {
		analyzers = append(analyzers, a)
	}
	graph, err := checker.Analyze(analyzers, pkgs, nil)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for act := range graph.All() {
		if !act.IsRoot || act.Err != nil {
			continue
		}
		meta, ok := bugAnalyzers[act.Analyzer]
		if !ok {
			continue // a dependency analyzer (buildssa/inspect/...) — no diagnostics of ours
		}
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			findings = append(findings, Finding{
				Category: meta.category,
				Detector: act.Analyzer.Name,
				Severity: meta.severity,
				File:     pos.Filename,
				Line:     pos.Line,
				Col:      pos.Column,
				Message:  d.Message,
				Package:  act.Package.PkgPath,
			})
		}
	}
	return findings, nil
}
