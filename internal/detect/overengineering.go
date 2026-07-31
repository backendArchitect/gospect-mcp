package detect

import (
	"context"
	"fmt"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

// RunOverEngineering asks the graph for functions/methods under scope whose complexity exceeds
// the thresholds and maps them to findings. Requires a configured graph.
func RunOverEngineering(ctx context.Context, g graph.Graph, scope string, minCyclomatic, minCognitive int) ([]Finding, error) {
	spots, err := g.HighComplexity(ctx, scope, minCyclomatic, minCognitive)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(spots))
	for _, h := range spots {
		sev := "medium"
		if h.Cyclomatic >= 2*minCyclomatic || h.Cognitive >= 2*minCognitive {
			sev = "high"
		}
		findings = append(findings, Finding{
			Category: "over-engineered",
			Detector: "high-complexity",
			Severity: sev,
			File:     h.File,
			Line:     h.Line,
			Message:  fmt.Sprintf("%s has high complexity (cyclomatic=%d, cognitive=%d) — consider splitting", h.Name, h.Cyclomatic, h.Cognitive),
			Package:  h.QualifiedName,
		})
	}
	return findings, nil
}
