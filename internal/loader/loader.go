// Package loader wraps go/packages. This is the risky core: loading + type-checking
// real (often multi-module) Go code is where the surprises live. Keep it thin and honest —
// surface package errors rather than hiding them.
package loader

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	return LoadWithProgress(dir, nil, patterns...)
}

// LoadWithProgress is Load with an optional progress callback, invoked before each module loads
// (a monorepo has several) so a caller can stream reassurance during a slow cold compile. progress
// may be nil.
func LoadWithProgress(dir string, progress func(string), patterns ...string) ([]*packages.Package, Stats, error) {
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

	// A single explicit target failing to load is the scan's own failure — surface it directly
	// (and keep the simple synchronous path).
	if len(roots) == 1 {
		if progress != nil {
			progress(fmt.Sprintf("loading %s", relDir(dir, roots[0])))
		}
		pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Dir: roots[0]}, patterns...)
		if err != nil {
			return nil, Stats{}, err
		}
		return finalize(pkgs, roots, nil, start)
	}

	// Multi-module (monorepo): each nested module is a separate job loaded concurrently.
	jobs := make([]loadJob, len(roots))
	for i, root := range roots {
		jobs[i] = loadJob{root: root, patterns: patterns}
	}
	all, loaded, skipped := runLoads(jobs, dir, progress)
	if len(all) == 0 {
		return nil, Stats{}, fmt.Errorf("all %d module(s) failed to load:\n  %s", len(roots), strings.Join(skipped, "\n  "))
	}
	return finalize(all, loaded, skipped, start)
}

// loadJob is one packages.Load invocation: a module root and the patterns to load within it.
type loadJob struct {
	root     string
	patterns []string
}

// LoadChanged loads only the packages that contain the given changed files (diff mode) — far
// faster than a whole-repo scan on a PR. Each file is attributed to its module (nested go.mod
// aware), and one packages.Load per module loads just the affected package directories. Non-Go or
// non-existent paths are ignored; an empty result (nothing relevant changed) yields zero packages.
func LoadChanged(dir string, changed []string, progress func(string)) ([]*packages.Package, Stats, error) {
	roots, hasGo, hint := survey(dir)
	if !hasGo && len(roots) == 0 {
		return nil, Stats{}, &NotGoError{Dir: dir, Hint: hint}
	}
	if len(roots) == 0 {
		roots = []string{dir} // loose .go files with no go.mod
	}
	// survey() returns roots relative to dir; the changed files are absolute, so compare on
	// absolute paths.
	for i, r := range roots {
		if abs, err := filepath.Abs(r); err == nil {
			roots[i] = abs
		}
	}

	// Group each changed .go file's package directory under the module root that owns it.
	byRoot := map[string]map[string]bool{} // root -> set of "./pkg" patterns
	for _, f := range changed {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			continue
		}
		root := owningRoot(abs, roots)
		if root == "" {
			continue // outside the scanned tree
		}
		rel, err := filepath.Rel(root, filepath.Dir(abs))
		if err != nil {
			continue
		}
		pat := "./" + filepath.ToSlash(rel)
		if rel == "." {
			pat = "."
		}
		if byRoot[root] == nil {
			byRoot[root] = map[string]bool{}
		}
		byRoot[root][pat] = true
	}

	start := time.Now()
	if len(byRoot) == 0 {
		return nil, Stats{Duration: time.Since(start)}, nil // nothing relevant changed
	}

	var jobs []loadJob
	for root, pats := range byRoot {
		patterns := make([]string, 0, len(pats))
		for p := range pats {
			patterns = append(patterns, p)
		}
		jobs = append(jobs, loadJob{root: root, patterns: patterns})
	}
	all, loaded, skipped := runLoads(jobs, dir, progress)
	if len(all) == 0 && len(skipped) > 0 {
		return nil, Stats{}, fmt.Errorf("all changed package(s) failed to load:\n  %s", strings.Join(skipped, "\n  "))
	}
	return finalize(all, loaded, skipped, start)
}

// owningRoot returns the deepest module root that is a directory prefix of file, or "" if none.
func owningRoot(file string, roots []string) string {
	best := ""
	for _, r := range roots {
		if underDir(file, r) && len(r) > len(best) {
			best = r
		}
	}
	return best
}

// underDir reports whether file is inside dir (path-prefix aware, not a mere string prefix).
func underDir(file, dir string) bool {
	rel, err := filepath.Rel(dir, file)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// runLoads executes load jobs concurrently (the load is the slow half of a big scan, and the jobs
// are independent). A job that fails is recorded in skipped and dropped, never fatal — one broken
// module must not abort the rest. dir is only used to render compact progress labels.
func runLoads(jobs []loadJob, dir string, progress func(string)) (all []*packages.Package, loaded, skipped []string) {
	type result struct {
		root string
		pkgs []*packages.Package
		err  error
	}
	results := make([]result, len(jobs))
	sem := make(chan struct{}, loadConcurrency(len(jobs)))
	var wg sync.WaitGroup
	var done int32
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j loadJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Dir: j.root}, j.patterns...)
			results[i] = result{j.root, pkgs, err}
			if progress != nil {
				progress(fmt.Sprintf("loaded %d/%d: %s", atomic.AddInt32(&done, 1), len(jobs), relDir(dir, j.root)))
			}
		}(i, j)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %s", r.root, firstLine(r.err)))
			continue
		}
		all = append(all, r.pkgs...)
		loaded = append(loaded, r.root)
	}
	return all, loaded, skipped
}

// finalize builds Stats for a completed load.
func finalize(pkgs []*packages.Package, roots, skipped []string, start time.Time) ([]*packages.Package, Stats, error) {
	stats := Stats{Duration: time.Since(start), Packages: len(pkgs), Roots: roots, Skipped: skipped}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		stats.Errors += len(p.Errors)
	})
	return pkgs, stats, nil
}

// loadConcurrency bounds how many modules load at once. packages.Load already parallelizes
// internally, so this stays modest to avoid oversubscribing the CPU while still overlapping the
// per-module compile/type-check latency.
func loadConcurrency(modules int) int {
	n := runtime.NumCPU() / 2
	if n < 2 {
		n = 2
	}
	if n > modules {
		n = modules
	}
	return n
}

// relDir renders root relative to base for compact progress lines, falling back to root itself
// (e.g. base is the module root, so root == base yields ".").
func relDir(base, root string) string {
	if rel, err := filepath.Rel(base, root); err == nil {
		if rel == "." {
			return filepath.Base(root)
		}
		return rel
	}
	return root
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
