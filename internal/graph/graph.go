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

// HotSpot is a Symbol with its complexity metrics attached.
type HotSpot struct {
	Symbol
	Cyclomatic int
	Cognitive  int
}

// Graph answers the whole-repo questions that single-package analysis can't.
// More methods (Callers for reachability, Routes for swagger-drift) get added as the
// graph-backed detectors land.
type Graph interface {
	// UntestedExports returns exported, non-test functions under scope (a file-path substring)
	// that have no incoming TESTS edge.
	UntestedExports(ctx context.Context, scope string) ([]Symbol, error)

	// HighComplexity returns non-test functions/methods under scope whose cyclomatic OR cognitive
	// complexity meets the given minimums.
	HighComplexity(ctx context.Context, scope string, minCyclomatic, minCognitive int) ([]HotSpot, error)
}
