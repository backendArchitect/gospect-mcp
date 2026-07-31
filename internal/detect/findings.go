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
	// Fingerprint is a stable identity for the finding (detector + relative path + message),
	// deliberately independent of line number so it survives edits that shift lines. Used to match
	// findings across runs (baseline mode) and to dedupe in SARIF. Set by the scan layer.
	Fingerprint string `json:"fingerprint,omitempty"`
}
