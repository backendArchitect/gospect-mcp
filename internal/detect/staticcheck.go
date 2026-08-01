package detect

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
	"honnef.co/go/tools/analysis/lint"
	"honnef.co/go/tools/staticcheck"
)

// staticcheckSet returns the default staticcheck "SA" analyzers plus the category/severity we
// report each under. Non-default (experimental/noisy) checks are excluded, matching what the
// standalone `staticcheck` tool runs out of the box. Built once per call — cheap.
func staticcheckSet() (map[*analysis.Analyzer]analyzerMeta, []*analysis.Analyzer) {
	metas := make(map[*analysis.Analyzer]analyzerMeta, len(staticcheck.Analyzers))
	list := make([]*analysis.Analyzer, 0, len(staticcheck.Analyzers))
	for _, la := range staticcheck.Analyzers {
		if la.Doc != nil && la.Doc.NonDefault {
			continue // off by default in staticcheck itself
		}
		metas[la.Analyzer] = analyzerMeta{category: scCategory(la), severity: scSeverity(la)}
		list = append(list, la.Analyzer)
	}
	return metas, list
}

// scCategory maps a staticcheck check to our finding category. Deprecated-API usage (SA1019) is a
// modernization concern; everything else in the SA set is a genuine bug class.
func scCategory(la *lint.Analyzer) string {
	if la.Doc != nil && la.Doc.Severity == lint.SeverityDeprecated {
		return "modernize"
	}
	return "bug"
}

// scSeverity maps staticcheck's own severity onto ours (high|medium|low).
func scSeverity(la *lint.Analyzer) string {
	if la.Doc == nil {
		return "medium"
	}
	switch la.Doc.Severity {
	case lint.SeverityError:
		return "high"
	case lint.SeverityWarning:
		return "medium"
	case lint.SeverityDeprecated:
		return "low"
	default:
		return "low"
	}
}

// RunStaticcheck runs the staticcheck SA analyzers over already-loaded packages and maps each
// diagnostic to a Finding. Report-only, like every other detector.
func RunStaticcheck(pkgs []*packages.Package) ([]Finding, error) {
	metas, analyzers := staticcheckSet()
	graph, err := checker.Analyze(analyzers, pkgs, nil)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for act := range graph.All() {
		if !act.IsRoot || act.Err != nil {
			continue
		}
		meta, ok := metas[act.Analyzer]
		if !ok {
			continue // a dependency analyzer (buildssa/inspect/config/…)
		}
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			findings = append(findings, Finding{
				Category: meta.category,
				Detector: act.Analyzer.Name, // e.g. "SA1019", "SA4006"
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
