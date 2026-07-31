package graph

import "context"

// Fake is an in-memory Graph for tests.
type Fake struct {
	Untested []Symbol
	Err      error
}

func (f *Fake) UntestedExports(ctx context.Context, scope string) ([]Symbol, error) {
	return f.Untested, f.Err
}
