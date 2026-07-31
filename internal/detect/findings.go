// Package detect holds the finding model and the deterministic detectors.
package detect

// Finding is one reported issue. It is intentionally flat and JSON-friendly:
// the MCP is report-first, so a finding carries evidence, never a fix.
type Finding struct {
	Category string `json:"category"` // "bug", "over-engineered", "stale-doc", "missing", "modernize"
	Detector string `json:"detector"` // analyzer that produced it, e.g. "nilness"
	Severity string `json:"severity"` // "high" | "medium" | "low"
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Message  string `json:"message"`
	Package  string `json:"package"`
}
