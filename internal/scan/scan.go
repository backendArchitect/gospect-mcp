// Package scan orchestrates load -> detect -> report. Report-first: the output is a
// Report of findings, never a mutation.
package scan

import (
	"sort"
	"time"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
	"github.com/backendArchitect/gospect-mcp/internal/loader"
)

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
	Findings       []detect.Finding `json:"findings"`
}

// Scan loads the module at dir and runs the deterministic detectors.
func Scan(dir string, patterns ...string) (*Report, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	pkgs, stats, err := loader.Load(dir, patterns...)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var findings []detect.Finding
	// Each detector is independent and report-only. A detector erroring aborts the scan rather
	// than returning a partial, misleading "all clear".
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
