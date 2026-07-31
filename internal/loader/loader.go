// Package loader wraps go/packages. This is the risky core: loading + type-checking
// real (often multi-module) Go code is where the surprises live. Keep it thin and honest —
// surface package errors rather than hiding them.
package loader

import (
	"time"

	"golang.org/x/tools/go/packages"
)

// Stats reports what the load cost and how clean it was.
type Stats struct {
	Packages int           // number of root packages loaded
	Errors   int           // total package errors across the import graph
	Duration time.Duration // wall time for the load
}

// Load type-checks the packages matching patterns under dir. LoadAllSyntax pulls in
// syntax + types + deps, which the analysis drivers require. A load error (e.g. the
// module doesn't build) is returned; per-package errors are counted, not fatal, so a
// partially-broken repo still yields findings for the parts that do type-check.
func Load(dir string, patterns ...string) ([]*packages.Package, Stats, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  dir,
	}
	start := time.Now()
	pkgs, err := packages.Load(cfg, patterns...)
	stats := Stats{Duration: time.Since(start)}
	if err != nil {
		return nil, stats, err
	}
	stats.Packages = len(pkgs)
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		stats.Errors += len(p.Errors)
	})
	return pkgs, stats, nil
}
