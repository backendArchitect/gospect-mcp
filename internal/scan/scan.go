// Package scan orchestrates load -> detect -> report. Report-first: the output is a
// Report of findings, never a mutation.
package scan

import (
	"context"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
	"github.com/backendArchitect/gospect-mcp/internal/graph"
	"github.com/backendArchitect/gospect-mcp/internal/graph/local"
	"github.com/backendArchitect/gospect-mcp/internal/loader"
	"golang.org/x/tools/go/packages"
)

// Complexity thresholds for the graph-backed over-engineering detector. Conservative so it flags
// genuinely gnarly functions, not merely large ones.
const (
	defaultMinCyclomatic = 20
	defaultMinCognitive  = 30
)

// Options configures a scan. Graph is optional: when nil, only the local (single-package)
// detectors run and gospect works fully standalone.
type Options struct {
	Patterns   []string
	Graph      graph.Graph  // optional code-intelligence graph (e.g. codebase-memory-mcp)
	GraphScope string       // file-path substring used to scope graph queries
	Progress   func(string) // optional; called with human-readable progress lines (verbose mode)
	Vuln       bool         // run govulncheck (opt-in: slow, needs the vuln DB)

	IncludeGenerated bool // scan generated files too (default: findings in "DO NOT EDIT" files are dropped)
	Staticcheck      bool // also run the staticcheck SA analyzers (opt-in: much deeper, but slower)
	Untested         bool // also report exported functions with no test (opt-in: noisy on large repos)
	Pedantic         bool // also run the opinionated/hygiene heuristics (opt-in: noisy on real code)

	// Diff mode: when DiffMode is set, only the packages containing ChangedFiles are loaded and
	// scanned (fast PR checks). An empty ChangedFiles list yields an empty report.
	DiffMode     bool
	ChangedFiles []string
}

// Report is the report-only output of a scan.
type Report struct {
	Path           string           `json:"path"`
	Patterns       []string         `json:"patterns"`
	PackagesLoaded int              `json:"packages_loaded"`
	ModulesLoaded  int              `json:"modules_loaded"`
	LoadErrors     int              `json:"load_errors"`
	LoadMillis     int64            `json:"load_millis"`
	ScanMillis     int64            `json:"scan_millis"`
	FindingCount   int              `json:"finding_count"`
	Suppressed     int              `json:"suppressed"`
	Generated      int              `json:"generated,omitempty"` // dropped as findings in generated ("DO NOT EDIT") files
	Ignored        int              `json:"ignored,omitempty"`   // dropped by a .gospectignore rule
	SkippedModules []string         `json:"skipped_modules,omitempty"`
	ByCategory     map[string]int   `json:"by_category"`
	BySeverity     map[string]int   `json:"by_severity"`
	GraphError     string           `json:"graph_error,omitempty"`
	Findings       []detect.Finding `json:"findings"`
}

// recount refreshes FindingCount and the by-category / by-severity summaries from Findings. Called
// after the finding set changes (initial build, filtering, baseline diff).
func (r *Report) recount() {
	r.FindingCount = len(r.Findings)
	r.ByCategory = map[string]int{}
	r.BySeverity = map[string]int{}
	for _, f := range r.Findings {
		r.ByCategory[f.Category]++
		r.BySeverity[f.Severity]++
	}
}

// Scan is the standalone entry point (no graph).
func Scan(dir string, patterns ...string) (*Report, error) {
	return ScanWithOptions(dir, Options{Patterns: patterns})
}

// ScanWithOptions loads the module at dir and runs the detectors, plus any graph-backed
// detectors when opt.Graph is set.
func ScanWithOptions(dir string, opt Options) (*Report, error) {
	patterns := opt.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	progress := opt.Progress
	if progress == nil {
		progress = func(string) {} // no-op keeps the call sites clean
	}
	var pkgs []*packages.Package
	var stats loader.Stats
	var err error
	if opt.DiffMode {
		pkgs, stats, err = loader.LoadChanged(dir, opt.ChangedFiles, opt.Progress)
	} else {
		pkgs, stats, err = loader.LoadWithProgress(dir, opt.Progress, patterns...)
	}
	if err != nil {
		return nil, err
	}
	progress(fmt.Sprintf("loaded %d package(s) across %d module(s) in %dms; running detectors…",
		stats.Packages, len(stats.Roots), stats.Duration.Milliseconds()))

	start := time.Now()
	var findings []detect.Finding
	// Local detectors are independent and report-only. A local detector erroring aborts the scan
	// rather than returning a partial, misleading "all clear".
	localRuns := []func() ([]detect.Finding, error){
		func() ([]detect.Finding, error) { return detect.RunBugDetectors(pkgs) },
		// RunMissingCode emits stub + todo + unchecked-error. Only `stub` is high-signal on real
		// code; todo and unchecked-error are noisy (unchecked-error fired ~193× on net/http alone),
		// so they're gated behind -pedantic. Bugs stay the default.
		func() ([]detect.Finding, error) {
			fs, err := detect.RunMissingCode(pkgs)
			if err != nil || opt.Pedantic {
				return fs, err
			}
			return withoutDetectors(fs, "todo", "unchecked-error"), nil
		},
	}
	if opt.Staticcheck {
		localRuns = append(localRuns, func() ([]detect.Finding, error) {
			progress("running staticcheck analyzers… (deeper, but slower)")
			return detect.RunStaticcheck(pkgs)
		})
	}
	if opt.Pedantic {
		// Spell-check comments + identifier names (known-typo dictionary; report-only).
		localRuns = append(localRuns, func() ([]detect.Finding, error) { return detect.RunMisspell(pkgs) })
	}
	for _, run := range localRuns {
		fs, err := run()
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}
	modRoots := stats.Roots
	if len(modRoots) == 0 {
		modRoots = []string{dir}
	}
	// Modernize emits only `go-version` (an outdated go.mod directive) — it fires on nearly every
	// real project, so it's -pedantic-only. It reads each module's go.mod (a monorepo has several).
	if opt.Pedantic {
		for _, root := range modRoots {
			fs, err := detect.RunModernize(root)
			if err != nil {
				return nil, err
			}
			findings = append(findings, fs...)
		}
	}
	// Vulnerability scan is opt-in (slow, needs the vuln DB); off by default.
	if opt.Vuln {
		progress("running govulncheck (may fetch the vulnerability database)…")
		fs, err := detect.RunVuln(modRoots)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}

	// Graph-backed detectors are additive: a graph failure is recorded but never fails the local
	// scan. They're whole-repo, so they're skipped in diff mode (a PR check should only surface
	// findings in the packages it touched).
	//
	// untested-exports and over-engineering run against a built-in graph computed from the loaded
	// packages — no external server needed. When an external graph IS configured, it's used instead
	// (richer) and additionally powers the route-based detectors (missing-handler, stale-swagger)
	// that the built-in graph can't answer yet.
	var graphErr string
	if !opt.DiffMode {
		ctx := context.Background()
		scope := opt.GraphScope
		if scope == "" {
			scope = dir
		}
		g := opt.Graph
		if g == nil {
			g = local.New(pkgs)
			progress("running built-in graph detectors…")
		} else {
			progress("running graph-backed detectors…")
		}
		// Over-engineering (complexity hotspots) is a design opinion, not a defect, and fires on
		// inherently-complex-but-fine code (e.g. ~29× on net/http), so it's -pedantic-only.
		var graphRuns []func() ([]detect.Finding, error)
		if opt.Pedantic {
			graphRuns = append(graphRuns, func() ([]detect.Finding, error) {
				return detect.RunOverEngineering(ctx, g, scope, defaultMinCyclomatic, defaultMinCognitive)
			})
		}
		// Untested-exports is opt-in: the built-in name heuristic is noisy on large repos (misses
		// cross-package tests). An external graph's TESTS edges are accurate, so run it there too.
		if opt.Untested || opt.Graph != nil {
			graphRuns = append(graphRuns,
				func() ([]detect.Finding, error) { return detect.RunUntestedExports(ctx, g, scope) },
			)
		}
		if opt.Graph != nil {
			// External graph: full route detectors (it models HANDLES edges for missing-handler).
			graphRuns = append(graphRuns,
				func() ([]detect.Finding, error) { return detect.RunMissingHandlers(ctx, g) },
				func() ([]detect.Finding, error) { return detect.RunStaleSwagger(ctx, g, dir) },
			)
		} else if rts, _ := g.Routes(ctx); len(rts) > 0 {
			// Built-in graph: run stale-swagger only when routes were actually extracted — with an
			// empty route set every documented endpoint would be a false "drift" positive.
			graphRuns = append(graphRuns,
				func() ([]detect.Finding, error) { return detect.RunStaleSwagger(ctx, g, dir) },
			)
		}
		for _, run := range graphRuns {
			if fs, err := run(); err != nil {
				if graphErr == "" {
					graphErr = err.Error()
				}
			} else {
				findings = append(findings, fs...)
			}
		}
	}

	// Honor //gospect:ignore directives: authors mark intentional code so it drops from the report.
	findings, suppressed := detect.ApplySuppressions(findings)

	// Findings in generated code are noise (fix belongs in the generator); drop them by default.
	var generated int
	if !opt.IncludeGenerated {
		findings, generated = dropGenerated(findings)
	}

	// Stable identity per finding (line-independent) for baseline matching and SARIF dedup.
	for i := range findings {
		findings[i].Fingerprint = fingerprint(dir, findings[i])
	}

	sortFindings(findings)

	rep := &Report{
		Path:           dir,
		Patterns:       patterns,
		PackagesLoaded: stats.Packages,
		ModulesLoaded:  len(stats.Roots),
		LoadErrors:     stats.Errors,
		LoadMillis:     stats.Duration.Milliseconds(),
		ScanMillis:     time.Since(start).Milliseconds(),
		Suppressed:     suppressed,
		Generated:      generated,
		SkippedModules: stats.Skipped,
		GraphError:     graphErr,
		Findings:       findings,
	}
	// A checked-in .gospectignore suppresses repo-declared noise for every consumer (CLI + MCP).
	rep.Ignored = rep.Apply(loadIgnoreFile(dir))
	rep.recount()
	return rep, nil
}

// fingerprint is a short stable hash of (detector, path-relative-to-scan-root, message). Excluding
// the line number keeps it stable when unrelated edits shift a finding up or down the file.
func fingerprint(dir string, f detect.Finding) string {
	rel := f.File
	if r, err := filepath.Rel(dir, f.File); err == nil {
		rel = r
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s\x00%s", f.Detector, filepath.ToSlash(rel), f.Message)
	return strconv.FormatUint(h.Sum64(), 16)
}

// withoutDetectors returns fs minus any finding whose detector is in drop. Used to keep the default
// scan high-signal by excluding the -pedantic-only heuristics.
func withoutDetectors(fs []detect.Finding, drop ...string) []detect.Finding {
	skip := make(map[string]bool, len(drop))
	for _, d := range drop {
		skip[d] = true
	}
	out := fs[:0]
	for _, f := range fs {
		if !skip[f.Detector] {
			out = append(out, f)
		}
	}
	return out
}

// sortFindings orders the report by importance so the most actionable issues are on top: highest
// severity first, then category priority (bugs above everything), then file:line for stable output.
func sortFindings(fs []detect.Finding) {
	sort.Slice(fs, func(i, j int) bool {
		if a, b := severityRank(fs[i].Severity), severityRank(fs[j].Severity); a != b {
			return a > b // high severity first
		}
		if a, b := categoryRank(fs[i].Category), categoryRank(fs[j].Category); a != b {
			return a < b // bugs before missing/modernize/etc.
		}
		if fs[i].File != fs[j].File {
			return fs[i].File < fs[j].File
		}
		if fs[i].Line != fs[j].Line {
			return fs[i].Line < fs[j].Line
		}
		if fs[i].Col != fs[j].Col {
			return fs[i].Col < fs[j].Col
		}
		return fs[i].Detector < fs[j].Detector
	})
}

// severityRank maps a severity to a sortable weight (higher = more urgent). Unknown → 0.
func severityRank(s string) int {
	switch s {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// categoryRank orders categories by how directly they signal a defect (lower sorts first). Unknown
// categories sort after the known ones but before none.
func categoryRank(c string) int {
	switch c {
	case "bug":
		return 0
	case "vuln":
		return 1
	case "over-engineered":
		return 2
	case "stale-doc":
		return 3
	case "missing":
		return 4
	case "modernize":
		return 5
	default:
		return 6
	}
}
