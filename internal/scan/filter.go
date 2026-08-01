package scan

import (
	"path/filepath"
	"strings"
)

// FilterOptions narrows a report's findings for display. Zero value = no filtering. Filtering is a
// presentation concern applied after the scan, so the underlying detection is unaffected.
type FilterOptions struct {
	MinSeverity      string   // drop findings below this severity ("low"|"medium"|"high"); "" = keep all
	MinConfidence    string   // drop findings below this confidence ("low"|"medium"|"high"); "" = keep all
	Categories       []string // keep only these categories, if non-empty
	Detectors        []string // keep only these detectors, if non-empty
	ExcludeGlob      []string // drop findings whose file path matches any of these globs (e.g. "*.pb.go")
	ExcludeDetectors []string // drop findings from these detectors (used by .gospectignore)
}

// Empty reports whether the options would filter nothing (lets callers skip work and reporting).
func (o FilterOptions) Empty() bool {
	return o.MinSeverity == "" && o.MinConfidence == "" && len(o.Categories) == 0 && len(o.Detectors) == 0 &&
		len(o.ExcludeGlob) == 0 && len(o.ExcludeDetectors) == 0
}

// Apply filters the report's findings in place and recomputes FindingCount + ByCategory. It returns
// how many findings were removed so callers can report it.
func (r *Report) Apply(o FilterOptions) (removed int) {
	if o.Empty() {
		return 0
	}
	minRank := severityRank(o.MinSeverity)
	minConf := severityRank(o.MinConfidence) // confidence uses the same high/medium/low scale
	cats := toSet(o.Categories)
	dets := toSet(o.Detectors)
	exclDets := toSet(o.ExcludeDetectors)

	kept := r.Findings[:0]
	for _, f := range r.Findings {
		switch {
		case minRank > 0 && severityRank(f.Severity) < minRank:
		case minConf > 0 && severityRank(f.Confidence) < minConf:
		case len(cats) > 0 && !cats[f.Category]:
		case len(dets) > 0 && !dets[f.Detector]:
		case exclDets[f.Detector]:
		case matchesAnyGlob(f.File, o.ExcludeGlob):
		default:
			kept = append(kept, f)
			continue
		}
		removed++
	}
	r.Findings = kept
	r.recount()
	return removed
}

// matchesAnyGlob reports whether path matches any glob, testing both the full path and the base name
// so "*.pb.go" matches a file anywhere in the tree.
func matchesAnyGlob(path string, globs []string) bool {
	base := filepath.Base(path)
	for _, g := range globs {
		if ok, _ := filepath.Match(g, path); ok {
			return true
		}
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
		if strings.Contains(path, g) { // also honor a plain substring (e.g. "/mocks/")
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]bool, len(items))
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			m[s] = true
		}
	}
	return m
}
