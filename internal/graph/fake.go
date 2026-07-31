package graph

import "context"

// Fake is an in-memory Graph for tests.
type Fake struct {
	Untested    []Symbol
	UntestedErr error

	HotSpots   []HotSpot
	HotSpotErr error

	Unhandled    []Route
	UnhandledErr error

	AllRoutes    []Route
	AllRoutesErr error
}

func (f *Fake) UntestedExports(ctx context.Context, scope string) ([]Symbol, error) {
	return f.Untested, f.UntestedErr
}

func (f *Fake) HighComplexity(ctx context.Context, scope string, minCyclomatic, minCognitive int) ([]HotSpot, error) {
	return f.HotSpots, f.HotSpotErr
}

func (f *Fake) UnhandledRoutes(ctx context.Context) ([]Route, error) {
	return f.Unhandled, f.UnhandledErr
}

func (f *Fake) Routes(ctx context.Context) ([]Route, error) {
	return f.AllRoutes, f.AllRoutesErr
}
