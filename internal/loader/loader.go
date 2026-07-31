// Package loader wraps go/packages. This is the risky core: loading + type-checking
// real (often multi-module) Go code is where the surprises live. Keep it thin and honest —
// surface package errors rather than hiding them.
package loader

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// Stats reports what the load cost and how clean it was.
type Stats struct {
	Packages int           // number of root packages loaded
	Errors   int           // total package errors across the import graph
	Duration time.Duration // wall time for the load
	Roots    []string      // module roots that were loaded (>1 for a multi-module repo)
	Skipped  []string      // "module: reason" for modules that failed to load (multi-module only)
}

// NotGoError means the target has no Go code at all. It names what was found instead so the
// caller can tell the user gospect is Go-only rather than failing cryptically.
type NotGoError struct {
	Dir  string
	Hint string // e.g. "package.json (JavaScript/TypeScript)"; empty if nothing recognizable
}

func (e *NotGoError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("no Go code found in %s — looks like a %s project. gospect-mcp is Go-only.", e.Dir, e.Hint)
	}
	return fmt.Sprintf("no Go code found in %s. gospect-mcp is Go-only.", e.Dir)
}

// Load type-checks the packages matching patterns under dir. LoadAllSyntax pulls in
// syntax + types + deps, which the analysis drivers require (the lighter export-data mode makes
// SSA/ctrlflow-based analyzers panic). A load error (e.g. the module doesn't build) is returned;
// per-package errors are counted, not fatal, so a partially-broken repo still yields findings.
//
// Multi-module: when scanning the whole tree (the default "./..." pattern), nested go.mod files
// are separate modules that a parent's "./..." never reaches. Load discovers each module root and
// loads them all, so one `scan <monorepo>` covers every service. An explicit non-"./..." pattern
// keeps single-module behavior.
func Load(dir string, patterns ...string) ([]*packages.Package, Stats, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	roots, hasGo, hint := survey(dir)
	if !hasGo && len(roots) == 0 {
		return nil, Stats{}, &NotGoError{Dir: dir, Hint: hint}
	}
	if !isWholeTree(patterns) || len(roots) == 0 {
		roots = []string{dir} // explicit patterns, or loose .go files with no go.mod
	}

	start := time.Now()
	var all []*packages.Package
	var skipped []string
	loaded := roots[:0:0] // module roots that actually loaded (excludes the skipped ones)
	for _, root := range roots {
		cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: root}
		pkgs, err := packages.Load(cfg, patterns...)
		if err != nil {
			// A single explicit target failing to load is the scan's failure — surface it.
			// In a monorepo, one broken module (bad vendoring, private dep) must not abort
			// the rest: record why it was skipped and keep going.
			if len(roots) == 1 {
				return nil, Stats{}, err
			}
			skipped = append(skipped, fmt.Sprintf("%s: %s", root, firstLine(err)))
			continue
		}
		all = append(all, pkgs...)
		loaded = append(loaded, root)
	}
	if len(all) == 0 {
		return nil, Stats{}, fmt.Errorf("all %d module(s) failed to load:\n  %s", len(roots), strings.Join(skipped, "\n  "))
	}

	stats := Stats{Duration: time.Since(start), Packages: len(all), Roots: loaded, Skipped: skipped}
	packages.Visit(all, nil, func(p *packages.Package) {
		stats.Errors += len(p.Errors)
	})
	return all, stats, nil
}

// firstLine collapses a multi-line loader error to its first line so skip reasons stay compact.
func firstLine(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// isWholeTree reports whether patterns is the default recurse-everything pattern, the only case
// where crossing nested module boundaries makes sense.
func isWholeTree(patterns []string) bool {
	return len(patterns) == 1 && patterns[0] == "./..."
}

// foreignMarkers maps a project-root filename to the language/ecosystem it signals. Used only to
// give a helpful "this looks like X" message when no Go is present.
var foreignMarkers = map[string]string{
	"package.json":     "JavaScript/TypeScript (Node/React)",
	"tsconfig.json":    "TypeScript",
	"pom.xml":          "Java (Maven)",
	"build.gradle":     "Java/Kotlin (Gradle)",
	"composer.json":    "PHP",
	"Cargo.toml":       "Rust",
	"pyproject.toml":   "Python",
	"requirements.txt": "Python",
	"Gemfile":          "Ruby",
	"CMakeLists.txt":   "C/C++",
	"Package.swift":    "Swift",
	"Podfile":          "Swift/Objective-C (CocoaPods)",
}

// foreignDirSuffixes recognizes ecosystems that mark themselves with a directory rather than a
// file (e.g. an Xcode project bundle).
var foreignDirSuffixes = map[string]string{
	".xcodeproj":   "Xcode (iOS/macOS)",
	".xcworkspace": "Xcode (iOS/macOS)",
}

// survey walks dir once and reports: every module root (dir containing go.mod), whether any .go
// file exists at all, and — if no Go is found — a hint at what language the tree looks like.
// Noise dirs (vendor, node_modules, .git) are skipped.
func survey(dir string) (roots []string, hasGo bool, hint string) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry — skip, don't abort the walk
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "node_modules", ".git":
				return filepath.SkipDir
			}
			if hint == "" {
				if lang, ok := foreignDirSuffixes[filepath.Ext(d.Name())]; ok {
					hint = lang
				}
			}
			return nil
		}
		switch {
		case d.Name() == "go.mod":
			roots = append(roots, filepath.Dir(path))
		case strings.HasSuffix(d.Name(), ".go"):
			hasGo = true
		case hint == "":
			if lang, ok := foreignMarkers[d.Name()]; ok {
				hint = lang
			}
		}
		return nil
	})
	if len(roots) > 0 {
		hasGo = true
	}
	return roots, hasGo, hint
}
