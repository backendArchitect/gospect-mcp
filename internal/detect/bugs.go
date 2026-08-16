package detect

import (
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/defers"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/reflectvaluecompare"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	"golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/timeformat"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/analysis/passes/waitgroup"
	"golang.org/x/tools/go/packages"

	"github.com/gordonklaus/ineffassign/pkg/ineffassign"
	"github.com/timakin/bodyclose/passes/bodyclose"
)

type analyzerMeta struct {
	category   string
	severity   string
	confidence string
}

// bugAnalyzers is the curated, high-precision analyzer set (bug-only, not style). Each entry
// carries the category/severity/confidence we report it under. Type/SSA-backed checks are high
// confidence; the two more heuristic ones (nilfunc, unreachable) are medium.
var bugAnalyzers = map[*analysis.Analyzer]analyzerMeta{
	nilness.Analyzer:             {"bug", "high", "high"},         // SSA nil dereference
	lostcancel.Analyzer:          {"bug", "high", "high"},         // context CancelFunc never called -> leak
	httpresponse.Analyzer:        {"bug", "high", "high"},         // using resp before checking err / not closing body
	unmarshal.Analyzer:           {"bug", "high", "high"},         // non-pointer passed to Unmarshal
	copylock.Analyzer:            {"bug", "high", "high"},         // a value containing a lock is copied
	errorsas.Analyzer:            {"bug", "high", "high"},         // errors.As target is not a pointer to an error
	bodyclose.Analyzer:           {"bug", "high", "medium"},       // HTTP body not closed -> leak (heuristic: FPs on hijacked conns, e.g. net/rpc CONNECT)
	printf.Analyzer:              {"bug", "high", "high"},         // Printf format/argument mismatch or non-constant format
	atomic.Analyzer:              {"bug", "high", "high"},         // sync/atomic result assigned back (lost update)
	sortslice.Analyzer:           {"bug", "high", "high"},         // sort.Slice on a non-slice -> runtime panic
	unusedresult.Analyzer:        {"bug", "medium", "high"},       // ignored result of errors.New/fmt.Errorf/etc.
	stringintconv.Analyzer:       {"bug", "medium", "high"},       // string(int) — usually meant strconv.Itoa (has a fix)
	timeformat.Analyzer:          {"bug", "medium", "high"},       // wrong time layout, e.g. 2006-02-01 (has a fix)
	sigchanyzer.Analyzer:         {"bug", "medium", "high"},       // unbuffered channel passed to signal.Notify
	appends.Analyzer:             {"bug", "medium", "high"},       // append with no values to append
	shift.Analyzer:               {"bug", "medium", "high"},       // shift amount >= the operand's width
	bools.Analyzer:               {"bug", "medium", "high"},       // redundant/suspicious boolean expression
	waitgroup.Analyzer:           {"bug", "high", "high"},         // sync.WaitGroup.Add called inside the goroutine
	deepequalerrors.Analyzer:     {"bug", "high", "high"},         // reflect.DeepEqual on errors (use errors.Is)
	reflectvaluecompare.Analyzer: {"bug", "high", "high"},         // comparing reflect.Value with == (compare .Interface())
	slog.Analyzer:                {"bug", "medium", "high"},       // mismatched key/value args to log/slog
	defers.Analyzer:              {"bug", "medium", "high"},       // common defer mistakes (e.g. time.Since in defer)
	shadow.Analyzer:              {"bug", "low", "medium"},        // shadowed variable (noisy → -pedantic only)
	nilfunc.Analyzer:             {"bug", "medium", "high"},       // useless comparison of func value to nil
	unreachable.Analyzer:         {"bug", "medium", "high"},       // unreachable code
	ineffassign.Analyzer:         {"bug", "low", "medium"},        // value assigned but never used
	loopclosure.Analyzer:         {"modernize", "medium", "high"}, // pre-1.22 loop-var capture (no-op on go>=1.22)
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
			detector, sev, conf := act.Analyzer.Name, meta.severity, meta.confidence
			// nilness emits two kinds: a nil *dereference* (a real crash — keep it a default
			// high bug) and an *impossible/tautological condition* (e.g. "nil != nil"). The latter
			// misfires on correct defensive code — it flagged heavily-reviewed go/types as buggy in
			// the real-world shakedown — so split it into its own low, -pedantic-only detector.
			if act.Analyzer == nilness.Analyzer && !strings.Contains(d.Message, "dereference") {
				detector, sev, conf = "nil-condition", "low", "medium"
			}
			findings = append(findings, Finding{
				Category:   meta.category,
				Detector:   detector,
				Severity:   sev,
				Confidence: conf,
				File:       pos.Filename,
				Line:       pos.Line,
				Col:        pos.Column,
				Message:    d.Message,
				Package:    act.Package.PkgPath,
			})
		}
	}
	return findings, nil
}
