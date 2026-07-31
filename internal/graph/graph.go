// Package graph is the composition seam to an external code-intelligence graph (e.g.
// codebase-memory-mcp). Detectors depend on this interface, not on any concrete backend, so the
// graph is optional and swappable — and detectors stay unit-testable against a fake.
package graph

import "context"

// Symbol is a code entity returned by the graph.
type Symbol struct {
	Name          string
	QualifiedName string
	File          string
	Line          int
}

// Graph answers the whole-repo questions that single-package analysis can't.
// More methods (Callers for reachability, Routes for swagger-drift) get added as the
// graph-backed detectors land.
type Graph interface {
	// UntestedExports returns exported, non-test functions under scope (a file-path substring)
	// that have no incoming TESTS edge.
	UntestedExports(ctx context.Context, scope string) ([]Symbol, error)
}
