// Package scan orchestrates load -> detect -> report. Report-first: the output is a
// Report of findings, never a mutation.
package scan

import (
	"context"
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
	Graph      graph.Graph // optional code-intelligence graph (e.g. codebase-memory-mcp)
	GraphScope string      // file-path substring used to scope graph queries
}

// Report is the report-only output of a scan.
type Report struct {
	Path           string           `json:"path"`
	Patterns       []string         `json:"patterns"`
	PackagesLoaded int              `json:"packages_loaded"`
	LoadErrors     int              `json:"load_errors"`
	LoadMillis     int64            `json:"load_millis"`
	ScanMillis     int64            `json:"scan_millis"`
	FindingCount   int              `json:"finding_count"`
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
	pkgs, stats, err := loader.Load(dir, patterns...)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var findings []detect.Finding
	// Local detectors are independent and report-only. A local detector erroring aborts the scan
	// rather than returning a partial, misleading "all clear".
	for _, run := range []func() ([]detect.Finding, error){
		func() ([]detect.Finding, error) { return detect.RunBugDetectors(pkgs) },
		func() ([]detect.Finding, error) { return detect.RunMissingCode(pkgs) },
		func() ([]detect.Finding, error) { return detect.RunModernize(dir) },
	} {
		fs, err := run()
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}

	// Graph-backed detectors are additive: a graph failure is recorded but never fails the
	// local scan, since the local findings are valuable on their own.
	var graphErr string
	if opt.Graph != nil {
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

	sortFindings(findings)
	byCat := map[string]int{}
	for _, f := range findings {
		byCat[f.Category]++
	}

	return &Report{
		Path:           dir,
		Patterns:       patterns,
		PackagesLoaded: stats.Packages,
		LoadErrors:     stats.Errors,
		LoadMillis:     stats.Duration.Milliseconds(),
		ScanMillis:     time.Since(start).Milliseconds(),
		FindingCount:   len(findings),
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
