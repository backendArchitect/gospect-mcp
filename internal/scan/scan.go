// Package scan orchestrates load -> detect -> report. Report-first: the output is a
// Report of findings, never a mutation.
package scan

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
	"github.com/backendArchitect/gospect-mcp/internal/graph"
	"github.com/backendArchitect/gospect-mcp/internal/loader"
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
	SkippedModules []string         `json:"skipped_modules,omitempty"`
	ByCategory     map[string]int   `json:"by_category"`
	GraphError     string           `json:"graph_error,omitempty"`
	Findings       []detect.Finding `json:"findings"`
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
	pkgs, stats, err := loader.LoadWithProgress(dir, opt.Progress, patterns...)
	if err != nil {
		return nil, err
	}
	progress(fmt.Sprintf("loaded %d package(s) across %d module(s) in %dms; running detectors…",
		stats.Packages, len(stats.Roots), stats.Duration.Milliseconds()))

	start := time.Now()
	var findings []detect.Finding
	// Local detectors are independent and report-only. A local detector erroring aborts the scan
	// rather than returning a partial, misleading "all clear".
	for _, run := range []func() ([]detect.Finding, error){
		func() ([]detect.Finding, error) { return detect.RunBugDetectors(pkgs) },
		func() ([]detect.Finding, error) { return detect.RunMissingCode(pkgs) },
	} {
		fs, err := run()
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}
	// Modernize reads each module's go.mod, so it runs once per loaded module root
	// (a monorepo has several); fall back to dir when the loader reported none.
	modRoots := stats.Roots
	if len(modRoots) == 0 {
		modRoots = []string{dir}
	}
	for _, root := range modRoots {
		fs, err := detect.RunModernize(root)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}

	// Graph-backed detectors are additive: a graph failure is recorded but never fails the
	// local scan, since the local findings are valuable on their own.
	var graphErr string
	if opt.Graph != nil {
		progress("running graph-backed detectors…")
		ctx := context.Background()
		scope := opt.GraphScope
		if scope == "" {
			scope = dir
		}
		graphRuns := []func() ([]detect.Finding, error){
			func() ([]detect.Finding, error) { return detect.RunUntestedExports(ctx, opt.Graph, scope) },
			func() ([]detect.Finding, error) {
				return detect.RunOverEngineering(ctx, opt.Graph, scope, defaultMinCyclomatic, defaultMinCognitive)
			},
			func() ([]detect.Finding, error) { return detect.RunMissingHandlers(ctx, opt.Graph) },
			func() ([]detect.Finding, error) { return detect.RunStaleSwagger(ctx, opt.Graph, dir) },
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

	sortFindings(findings)
	byCat := map[string]int{}
	for _, f := range findings {
		byCat[f.Category]++
	}

	return &Report{
		Path:           dir,
		Patterns:       patterns,
		PackagesLoaded: stats.Packages,
		ModulesLoaded:  len(stats.Roots),
		LoadErrors:     stats.Errors,
		LoadMillis:     stats.Duration.Milliseconds(),
		ScanMillis:     time.Since(start).Milliseconds(),
		FindingCount:   len(findings),
		Suppressed:     suppressed,
		SkippedModules: stats.Skipped,
		ByCategory:     byCat,
		GraphError:     graphErr,
		Findings:       findings,
	}, nil
}

func sortFindings(fs []detect.Finding) {
	sort.Slice(fs, func(i, j int) bool {
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
