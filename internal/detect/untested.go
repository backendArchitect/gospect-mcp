package detect

import (
	"context"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

// RunUntestedExports asks the graph for exported functions under scope that have no test, and
// maps them to findings. This detector requires a configured graph; callers skip it when the
// graph is nil (gospect still works standalone without one).
func RunUntestedExports(ctx context.Context, g graph.Graph, scope string) ([]Finding, error) {
	syms, err := g.UntestedExports(ctx, scope)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(syms))
	for _, s := range syms {
		findings = append(findings, Finding{
			Category: "missing",
			Detector: "untested-export",
			Severity: "low",
			File:     s.File,
			Line:     s.Line,
			Message:  "exported function " + s.Name + " has no test",
			Package:  s.QualifiedName,
		})
	}
	return findings, nil
}
