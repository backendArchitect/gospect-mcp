// Package gate turns a scan report into a CI pass/fail decision: findings at or above a severity
// threshold (minus any ignored detectors) block; everything else is informational.
package gate

import "github.com/backendArchitect/gospect-mcp/internal/detect"

// Policy is the gating configuration.
type Policy struct {
	FailOn string          // minimum blocking severity: "high" | "medium" | "low"
	Ignore map[string]bool // detector names to never block on
}

// Result is the outcome of evaluating findings against a Policy.
type Result struct {
	Blocking   []detect.Finding // findings that fail the gate
	Total      int
	BySeverity map[string]int
}

// Pass reports whether the gate passes (nothing blocking).
func (r Result) Pass() bool { return len(r.Blocking) == 0 }

func rank(sev string) int {
	switch sev {
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

// Evaluate applies the policy to findings. An unknown/empty FailOn defaults to "high".
func Evaluate(findings []detect.Finding, p Policy) Result {
	threshold := rank(p.FailOn)
	if threshold == 0 {
		threshold = rank("high")
	}
	res := Result{BySeverity: map[string]int{}}
	for _, f := range findings {
		res.Total++
		res.BySeverity[f.Severity]++
		if p.Ignore[f.Detector] {
			continue
		}
		if rank(f.Severity) >= threshold {
			res.Blocking = append(res.Blocking, f)
		}
	}
	return res
}
