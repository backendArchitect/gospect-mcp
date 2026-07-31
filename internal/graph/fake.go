package graph

import "context"

// Fake is an in-memory Graph for tests.
type Fake struct {
	Untested    []Symbol
	UntestedErr error

	HotSpots    []HotSpot
	HotSpotErr  error
}

func (f *Fake) UntestedExports(ctx context.Context, scope string) ([]Symbol, error) {
	return f.Untested, f.UntestedErr
}

func (f *Fake) HighComplexity(ctx context.Context, scope string, minCyclomatic, minCognitive int) ([]HotSpot, error) {
	return f.HotSpots, f.HotSpotErr
}
