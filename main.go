// Command gospect-mcp is a report-first, Go-only code scanner exposed as an MCP server.
//
// Two ways to run it:
//
//	gospect-mcp                 # MCP server over stdio (default)
//	gospect-mcp scan <dir> ...  # standalone CLI: print a JSON report
//
// It never modifies code. Fixes are a separate, explicitly-invoked step (not yet implemented).
//
// Optional graph composition — set these to enable graph-backed detectors (e.g. untested-exports)
// by spawning a codebase-memory-mcp-compatible server:
//
//	GOSPECT_GRAPH_CMD      command to launch the graph MCP server (e.g. "codebase-memory-mcp")
//	GOSPECT_GRAPH_PROJECT  project name to query
//	GOSPECT_GRAPH_SCOPE    file-path substring to scope graph queries (optional)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
	"github.com/backendArchitect/gospect-mcp/internal/fix"
	"github.com/backendArchitect/gospect-mcp/internal/gate"
	"github.com/backendArchitect/gospect-mcp/internal/graph"
	"github.com/backendArchitect/gospect-mcp/internal/graph/cbm"
	"github.com/backendArchitect/gospect-mcp/internal/loader"
	"github.com/backendArchitect/gospect-mcp/internal/mcp"
	"github.com/backendArchitect/gospect-mcp/internal/scan"
	"github.com/backendArchitect/gospect-mcp/internal/selfupdate"
)

func main() {
	// No arguments = MCP server over stdio (how MCP hosts launch it). Every other invocation is a
	// subcommand; an unrecognized one errors with usage rather than silently starting the server.
	if len(os.Args) < 2 {
		runServer()
		return
	}
	switch os.Args[1] {
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	case "version", "--version":
		fmt.Println("gospect-mcp", selfupdate.Current())
	case "update":
		runSelfUpdate()
	case "propose-fix":
		runProposeFix()
	case "uninstall":
		runUninstall()
	case "check":
		runCheck()
	case "scan":
		runScan()
	default:
		fmt.Fprintf(os.Stderr, "gospect-mcp: unknown command %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

// printUsage writes the top-level help. It goes to stdout for `help` and to stderr for errors.
func printUsage(w io.Writer) {
	fmt.Fprint(w, `gospect-mcp — report-first, Go-only code scanner (MCP server + CLI)

Usage:
  gospect-mcp                                    start the MCP server (stdio; default)
  gospect-mcp scan  [flags] <dir> [patterns...]  scan a module or monorepo, print a JSON report
  gospect-mcp check [flags] <dir> [patterns...]  CI gate: exit non-zero on findings at/above a severity
  gospect-mcp propose-fix                        read a finding (JSON) on stdin, print a fix envelope
  gospect-mcp version                            print the installed version
  gospect-mcp update                             update to the latest release, if any
  gospect-mcp uninstall                          remove the installed binary
  gospect-mcp help                               show this help

Flags (scan, check):
  -verbose            stream per-module load + detector progress, and a summary, to stderr
  -min-severity <s>   keep only findings at/above a severity: low|medium|high
  -category <a,b>     keep only these categories (e.g. bug,missing)
  -detector <a,b>     keep only these detectors (e.g. nilness)
  -exclude <g,g>      drop findings whose file path matches a glob/substring (e.g. *.pb.go,mocks/)
  -baseline <file>    a saved report; show/gate only findings NOT already in it (adopt on noisy repos)
  -since <git-ref>    diff mode: scan only packages with .go files changed since the ref (fast PR checks)
  -staticcheck        also run the staticcheck SA analyzers — much deeper bug detection, but slower
  -vuln               also run govulncheck for known-CVE dependencies (slow, needs the vuln DB)
  -include-generated  also report findings in generated ("DO NOT EDIT") files (skipped by default)
Flags (scan):
  -format <fmt>       json|text|sarif (default json; sarif = GitHub code-scanning)
  -exit-code          exit 1 if any findings remain (after filters/baseline)
Flags (check):
  -fail-on <sev>      minimum severity that fails: high|medium|low (default high)
  -ignore <a,b>       comma-separated detector names to skip
  -format <fmt>       text|json (default text)

Findings are ordered by importance: bugs (high severity) first, then medium, then low.

Notes:
  <dir> may be a single module or a monorepo root — every nested go.mod is scanned in one run.
  Mark intentional code with a //gospect:ignore [detector...] comment to drop it from the report.
  A checked-in .gospectignore (path globs + detector:<name> rules) suppresses noise repo-wide.

Docs: https://github.com/backendArchitect/gospect-mcp
`)
}

// runScan is the standalone CLI: scan a module/monorepo and print the JSON report. Flags precede
// the positional args: `scan [flags] <dir> [patterns...]`.
func runScan() {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	verbose := fs.Bool("verbose", false, "stream load/detector progress and a summary to stderr")
	format := fs.String("format", "json", "output format: json|text|sarif")
	baseline := fs.String("baseline", "", "path to a saved report; show only findings NOT already in it")
	exitCode := fs.Bool("exit-code", false, "exit 1 if any findings remain (after filters/baseline)")
	vuln := fs.Bool("vuln", false, "also run govulncheck for known-CVE dependencies (slow, needs the vuln DB)")
	inclGen := fs.Bool("include-generated", false, "also report findings in generated (\"DO NOT EDIT\") files")
	staticcheck := fs.Bool("staticcheck", false, "also run staticcheck SA analyzers (much deeper, but slower)")
	since := fs.String("since", "", "diff mode: scan only packages with .go files changed since this git ref (e.g. origin/main)")
	filter := addFilterFlags(fs)
	mustParse(fs)

	args := fs.Args()
	if len(args) == 0 {
		// A bare `scan` must NOT fall through to MCP server mode (which blocks on stdin and looks
		// like a hang). Require a directory.
		fmt.Fprintln(os.Stderr, "usage: gospect-mcp scan [flags] <dir> [patterns...]")
		fmt.Fprintln(os.Stderr, "(run gospect-mcp with no arguments to start the MCP server)")
		os.Exit(2)
	}

	ctx := context.Background()
	g, cleanup, scope := buildGraph(ctx)
	defer cleanup()

	diffMode, changed := diffOptions(args[0], *since)
	if !diffMode {
		fmt.Fprintf(os.Stderr, "gospect: loading %s … (first run may compile dependencies; large modules take a few seconds)\n", args[0])
	}
	rep, err := scan.ScanWithOptions(args[0], scan.Options{
		Patterns: args[1:], Graph: g, GraphScope: scope, Progress: progressFn(*verbose),
		Vuln: *vuln, IncludeGenerated: *inclGen, Staticcheck: *staticcheck, DiffMode: diffMode, ChangedFiles: changed,
	})
	if err != nil {
		exitScanError(err)
	}
	warnSkipped(rep)
	if removed := rep.Apply(filter()); removed > 0 {
		fmt.Fprintf(os.Stderr, "gospect: %d finding(s) filtered out by flags\n", removed)
	}
	applyBaseline(rep, *baseline)
	if *verbose {
		printScanSummary(rep)
	}
	switch *format {
	case "text":
		printFindingsText(rep)
	case "sarif":
		out, _ := json.MarshalIndent(rep.SARIF(selfupdate.Current()), "", "  ")
		fmt.Println(string(out))
	default:
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
	}
	if *exitCode && rep.FindingCount > 0 {
		os.Exit(1)
	}
}

// applyBaseline hides findings already present in a saved baseline report, so only new ones remain.
func applyBaseline(rep *scan.Report, path string) {
	if path == "" {
		return
	}
	base, err := scan.LoadBaseline(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "baseline error:", err)
		os.Exit(2)
	}
	if hidden := rep.ApplyBaseline(base); hidden > 0 {
		fmt.Fprintf(os.Stderr, "gospect: %d known finding(s) hidden by baseline %s\n", hidden, path)
	}
}

// addFilterFlags registers the shared output-filter flags on fs and returns a closure that builds
// the FilterOptions after parsing. Used by both scan and check.
func addFilterFlags(fs *flag.FlagSet) func() scan.FilterOptions {
	minSev := fs.String("min-severity", "", "keep only findings at/above this severity: low|medium|high")
	cats := fs.String("category", "", "keep only these comma-separated categories (e.g. bug,missing)")
	dets := fs.String("detector", "", "keep only these comma-separated detectors (e.g. nilness)")
	exclude := fs.String("exclude", "", "drop findings whose file path matches any comma-separated glob/substring (e.g. *.pb.go,/mocks/)")
	return func() scan.FilterOptions {
		return scan.FilterOptions{
			MinSeverity: strings.TrimSpace(*minSev),
			Categories:  splitCSV(*cats),
			Detectors:   splitCSV(*dets),
			ExcludeGlob: splitCSV(*exclude),
		}
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// printFindingsText renders a report as a human-readable, grouped listing (bugs first, per the
// report's own ordering).
func printFindingsText(rep *scan.Report) {
	fmt.Printf("gospect: %d finding(s) in %s (%d package(s), %d module(s))\n",
		rep.FindingCount, rep.Path, rep.PackagesLoaded, rep.ModulesLoaded)
	if rep.Suppressed > 0 {
		fmt.Printf("  (%d suppressed via //gospect:ignore)\n", rep.Suppressed)
	}
	if rep.GraphError != "" {
		fmt.Printf("  (graph detectors skipped: %s)\n", rep.GraphError)
	}
	for _, f := range rep.Findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Printf("  [%-6s] %-16s %s\n           %s\n", f.Severity, f.Detector, loc, f.Message)
	}
}

// mustParse parses a subcommand's flags (os.Args[2:]) with a friendlier failure than the flag
// package's default. On an unknown flag it appends a version + "run update" hint — the common cause
// is an out-of-date binary that predates the flag (flag already printed the specific error/usage).
func mustParse(fs *flag.FlagSet) {
	err := fs.Parse(os.Args[2:])
	if err == nil {
		return
	}
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0) // -h/--help: flag printed usage already
	}
	fmt.Fprintf(os.Stderr, "\nhint: this gospect-mcp is %s — if that flag is newer than your binary, run `gospect-mcp update`.\n",
		selfupdate.Current())
	os.Exit(2)
}

// diffOptions turns a -since git ref into diff-mode scan options: the .go files changed since the
// ref, as absolute paths. Exits with a clear message if the target isn't a git repo. Pass the ref
// with `...` (e.g. origin/main...) for merge-base semantics on a PR branch.
func diffOptions(dir, ref string) (bool, []string) {
	if ref == "" {
		return false, nil
	}
	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gospect: -since needs a git repository (%v)\n", err)
		os.Exit(2)
	}
	top = strings.TrimSpace(top)
	out, err := gitOutput(dir, "diff", "--name-only", ref, "--", "*.go")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gospect: -since:", err)
		os.Exit(2)
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, filepath.Join(top, line))
		}
	}
	fmt.Fprintf(os.Stderr, "gospect: diff mode — %d changed Go file(s) since %s\n", len(files), ref)
	return true, files
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// progressFn returns a stderr progress printer when verbose, else nil (progress suppressed).
func progressFn(verbose bool) func(string) {
	if !verbose {
		return nil
	}
	return func(m string) { fmt.Fprintln(os.Stderr, "gospect:", m) }
}

// printScanSummary writes a human-readable digest of the report to stderr (verbose mode); the
// machine-readable JSON still goes to stdout untouched.
func printScanSummary(rep *scan.Report) {
	fmt.Fprintf(os.Stderr, "gospect: %d finding(s) across %d package(s) / %d module(s); load %dms, scan %dms",
		rep.FindingCount, rep.PackagesLoaded, rep.ModulesLoaded, rep.LoadMillis, rep.ScanMillis)
	if rep.Suppressed > 0 {
		fmt.Fprintf(os.Stderr, "; %d suppressed", rep.Suppressed)
	}
	fmt.Fprintln(os.Stderr)
	cats := make([]string, 0, len(rep.ByCategory))
	for c := range rep.ByCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Fprintf(os.Stderr, "  %-16s %d\n", c, rep.ByCategory[c])
	}
}

// runServer starts the MCP stdio server.
func runServer() {
	ctx := context.Background()
	g, cleanup, scope := buildGraph(ctx)
	defer cleanup()

	srv := mcp.NewServer("gospect-mcp", selfupdate.Current())
	srv.Register(mcp.Tool{
		Name:        "scan",
		Description: "Scan a Go module for issues (report-only). Returns findings as JSON; never modifies code.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Filesystem directory of the Go module to scan",
				},
				"patterns": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Package patterns to scan (default [\"./...\"])",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(args json.RawMessage) (string, error) {
			var a struct {
				Path     string   `json:"path"`
				Patterns []string `json:"patterns"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if a.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			rep, err := scan.ScanWithOptions(a.Path, scan.Options{
				Patterns: a.Patterns, Graph: g, GraphScope: scope,
			})
			if err != nil {
				return "", err
			}
			out, err := json.MarshalIndent(rep, "", "  ")
			if err != nil {
				return "", err
			}
			return string(out), nil
		},
	})

	srv.Register(mcp.Tool{
		Name: "propose_fix",
		Description: "Return a report-first FIX ENVELOPE for a finding (root cause, verify-first checklist, " +
			"expected scope, reuse hint, ponytail constraints). Emits guidance only — never edits code. " +
			"Invoke only when a fix is explicitly requested.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"detector": map[string]any{"type": "string", "description": "the finding's detector, e.g. nilness"},
				"category": map[string]any{"type": "string"},
				"message":  map[string]any{"type": "string"},
				"file":     map[string]any{"type": "string"},
				"line":     map[string]any{"type": "integer"},
				"package":  map[string]any{"type": "string"},
			},
			"required": []string{"detector"},
		},
		Handler: func(args json.RawMessage) (string, error) {
			return envelopeJSON(args)
		},
	})

	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

// warnSkipped tells the user (on stderr) which monorepo modules couldn't be loaded, so a partial
// scan never masquerades as full coverage. Each reason is self-explanatory (e.g. bad vendoring).
func warnSkipped(rep *scan.Report) {
	if len(rep.SkippedModules) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "gospect: %d module(s) skipped (could not load):\n", len(rep.SkippedModules))
	for _, s := range rep.SkippedModules {
		fmt.Fprintf(os.Stderr, "  - %s\n", s)
	}
}

// exitScanError prints a scan failure and exits. A NotGoError (the target has no Go) is a
// user-facing "wrong tool" message, printed as-is; anything else is an internal scan error.
func exitScanError(err error) {
	var notGo *loader.NotGoError
	if errors.As(err, &notGo) {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Fprintln(os.Stderr, "scan error:", err)
	}
	os.Exit(2)
}

// envelopeJSON builds a fix envelope from a finding's JSON and returns it pretty-printed.
func envelopeJSON(raw []byte) (string, error) {
	var f detect.Finding
	if err := json.Unmarshal(raw, &f); err != nil {
		return "", fmt.Errorf("invalid finding: %w", err)
	}
	if f.Detector == "" {
		return "", fmt.Errorf("detector is required")
	}
	out, err := json.MarshalIndent(fix.Build(f), "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// runCheck is the CI-gate mode: scan, then exit non-zero if any finding is at/above the
// configured severity. Flags must precede the positional args: `check [flags] <dir> [patterns...]`.
func runCheck() {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	failOn := fs.String("fail-on", "high", "minimum severity that fails the check: high|medium|low")
	format := fs.String("format", "text", "output format: text|json")
	ignore := fs.String("ignore", "", "comma-separated detector names to ignore")
	verbose := fs.Bool("verbose", false, "stream load/detector progress to stderr")
	baseline := fs.String("baseline", "", "path to a saved report; gate only on findings NOT already in it")
	vuln := fs.Bool("vuln", false, "also run govulncheck for known-CVE dependencies (slow, needs the vuln DB)")
	inclGen := fs.Bool("include-generated", false, "also report findings in generated (\"DO NOT EDIT\") files")
	staticcheck := fs.Bool("staticcheck", false, "also run staticcheck SA analyzers (much deeper, but slower)")
	since := fs.String("since", "", "diff mode: gate only on packages with .go files changed since this git ref (e.g. origin/main)")
	filter := addFilterFlags(fs)
	mustParse(fs)

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gospect-mcp check [flags] <dir> [patterns...]")
		os.Exit(2)
	}

	ctx := context.Background()
	g, cleanup, scope := buildGraph(ctx)
	defer cleanup()

	diffMode, changed := diffOptions(args[0], *since)
	if !diffMode {
		fmt.Fprintf(os.Stderr, "gospect: loading %s … (first run may compile dependencies)\n", args[0])
	}
	rep, err := scan.ScanWithOptions(args[0], scan.Options{
		Patterns: args[1:], Graph: g, GraphScope: scope, Progress: progressFn(*verbose),
		Vuln: *vuln, IncludeGenerated: *inclGen, Staticcheck: *staticcheck, DiffMode: diffMode, ChangedFiles: changed,
	})
	if err != nil {
		exitScanError(err)
	}
	warnSkipped(rep)
	if removed := rep.Apply(filter()); removed > 0 {
		fmt.Fprintf(os.Stderr, "gospect: %d finding(s) filtered out by flags\n", removed)
	}
	applyBaseline(rep, *baseline)

	ignoreSet := map[string]bool{}
	for _, d := range strings.Split(*ignore, ",") {
		if d = strings.TrimSpace(d); d != "" {
			ignoreSet[d] = true
		}
	}
	result := gate.Evaluate(rep.Findings, gate.Policy{FailOn: *failOn, Ignore: ignoreSet})

	if *format == "json" {
		out, _ := json.MarshalIndent(map[string]any{
			"pass": result.Pass(), "fail_on": *failOn, "blocking": result.Blocking, "report": rep,
		}, "", "  ")
		fmt.Println(string(out))
	} else {
		printCheckSummary(rep, result, *failOn)
	}
	if !result.Pass() {
		os.Exit(1)
	}
}

func printCheckSummary(rep *scan.Report, result gate.Result, failOn string) {
	fmt.Printf("gospect check: %d findings in %s\n", rep.FindingCount, rep.Path)
	cats := make([]string, 0, len(rep.ByCategory))
	for c := range rep.ByCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Printf("  %-16s %d\n", c, rep.ByCategory[c])
	}
	if rep.GraphError != "" {
		fmt.Printf("  (graph detectors skipped: %s)\n", rep.GraphError)
	}

	if result.Pass() {
		fmt.Printf("PASS — no findings at or above %q severity.\n", failOn)
		return
	}
	fmt.Printf("\nFAIL — %d finding(s) at or above %q severity:\n", len(result.Blocking), failOn)
	for _, f := range result.Blocking {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Printf("  [%s] %s (%s) %s\n", f.Severity, f.Detector, loc, f.Message)
	}
}

// runUninstall removes the installed gospect-mcp binary from this machine.
func runUninstall() {
	p, err := selfupdate.InstalledPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot locate the binary:", err)
		os.Exit(1)
	}
	if selfupdate.IsEphemeralBuild(p) {
		fmt.Printf("Nothing to uninstall: this is a temporary build (%s).\n", p)
		fmt.Println("To remove an installed binary: rm \"$(command -v gospect-mcp)\"")
		return
	}
	fmt.Printf("This will remove the gospect-mcp binary:\n  %s\n", p)
	if !argHasFlag("--yes") && !argHasFlag("-y") {
		fmt.Print("Proceed? [y/N]: ")
		var resp string
		_, _ = fmt.Scanln(&resp)
		if resp != "y" && resp != "Y" && resp != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}
	if err := selfupdate.Uninstall(p); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
		if runtime.GOOS == "windows" {
			fmt.Fprintf(os.Stderr, "On Windows a running .exe can't delete itself — remove it manually: %s\n", p)
		}
		os.Exit(1)
	}
	fmt.Printf("Removed %s\n", p)
	fmt.Println("Also remove the \"gospect\" entry from your MCP client config and any GOSPECT_GRAPH_* env vars.")
}

func argHasFlag(f string) bool {
	for _, a := range os.Args[1:] {
		if a == f {
			return true
		}
	}
	return false
}

// runProposeFix is the CLI form: it reads a finding as JSON on stdin and prints the envelope.
func runProposeFix() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
	out, err := envelopeJSON(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(out)
}

// runSelfUpdate checks GitHub for a newer release and updates via `go install` when one exists.
func runSelfUpdate() {
	ctx := context.Background()
	cur := selfupdate.Current()
	tag, url, err := selfupdate.Latest(ctx, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "update check failed:", err)
		os.Exit(1)
	}
	if tag == "" {
		fmt.Printf("gospect-mcp %s — no releases published yet.\n", cur)
		return
	}
	if !selfupdate.Newer(cur, tag) {
		fmt.Printf("gospect-mcp %s is up to date (latest release: %s).\n", cur, tag)
		return
	}
	fmt.Printf("Updating gospect-mcp %s -> %s ...\n", cur, tag)
	if err := selfupdate.Install(ctx, tag); err != nil {
		fmt.Fprintf(os.Stderr, "update failed: %v\nDownload manually: %s\n", err, url)
		os.Exit(1)
	}
	fmt.Printf("Updated to %s. Restart gospect-mcp to use the new version.\n", tag)
}

// buildGraph constructs an optional code-intelligence graph from env. It returns a nil Graph
// (and a no-op cleanup) when unconfigured or on any connection error — gospect then runs its
// local detectors only, unaffected.
func buildGraph(ctx context.Context) (graph.Graph, func(), string) {
	cmdline := strings.TrimSpace(os.Getenv("GOSPECT_GRAPH_CMD"))
	project := strings.TrimSpace(os.Getenv("GOSPECT_GRAPH_PROJECT"))
	scope := os.Getenv("GOSPECT_GRAPH_SCOPE")
	if cmdline == "" || project == "" {
		return nil, func() {}, ""
	}
	parts := strings.Fields(cmdline)
	client, err := mcp.DialCommand(ctx, parts[0], parts[1:]...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "graph disabled:", err)
		return nil, func() {}, ""
	}
	if err := client.Initialize(); err != nil {
		fmt.Fprintln(os.Stderr, "graph disabled (init failed):", err)
		_ = client.Close()
		return nil, func() {}, ""
	}
	return cbm.New(client, project), func() { _ = client.Close() }, scope
}
