package detect

import (
	"context"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

// RunMissingHandlers asks the graph for HTTP routes with no handler and maps them to findings.
// Graph-wide (routes carry no file path). Requires a configured graph.
func RunMissingHandlers(ctx context.Context, g graph.Graph) ([]Finding, error) {
	routes, err := g.UnhandledRoutes(ctx)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(routes))
	for _, r := range routes {
		findings = append(findings, Finding{
			Category: "missing",
			Detector: "unhandled-route",
			Severity: "medium",
			Message:  "route " + r.Method + " " + r.Path + " has no handler",
			Package:  r.QualifiedName,
		})
	}
	return findings, nil
}
