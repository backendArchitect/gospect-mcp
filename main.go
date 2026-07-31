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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "version":
			fmt.Println("gospect-mcp", selfupdate.Current())
			return
		case "update":
			runSelfUpdate()
			return
		case "propose-fix":
			runProposeFix()
			return
		case "uninstall":
			runUninstall()
			return
		case "check":
			runCheck()
			return
		case "scan":
			// A bare `scan` with no path must NOT silently fall through to MCP server mode
			// (which blocks on stdin and looks like a hang). Require a directory.
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: gospect-mcp scan <dir> [patterns...]")
				fmt.Fprintln(os.Stderr, "(run gospect-mcp with no arguments to start the MCP server)")
				os.Exit(2)
			}
			// A valid scan continues to the handler below.
		}
	}

	ctx := context.Background()
	g, cleanup, scope := buildGraph(ctx)
	defer cleanup()

	// Standalone CLI mode for quick local use / testing.
	if len(os.Args) >= 3 && os.Args[1] == "scan" {
		fmt.Fprintf(os.Stderr, "gospect: loading %s … (first run may compile dependencies; large modules take a few seconds)\n", os.Args[2])
		rep, err := scan.ScanWithOptions(os.Args[2], scan.Options{
			Patterns: os.Args[3:], Graph: g, GraphScope: scope,
		})
		if err != nil {
			exitScanError(err)
		}
		warnSkipped(rep)
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
		return
	}

	// Default: MCP server over stdio.
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
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	failOn := fs.String("fail-on", "high", "minimum severity that fails the check: high|medium|low")
	format := fs.String("format", "text", "output format: text|json")
	ignore := fs.String("ignore", "", "comma-separated detector names to ignore")
	_ = fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gospect-mcp check [flags] <dir> [patterns...]")
		os.Exit(2)
	}

	ctx := context.Background()
	g, cleanup, scope := buildGraph(ctx)
	defer cleanup()

	fmt.Fprintf(os.Stderr, "gospect: loading %s … (first run may compile dependencies)\n", args[0])
	rep, err := scan.ScanWithOptions(args[0], scan.Options{Patterns: args[1:], Graph: g, GraphScope: scope})
	if err != nil {
		exitScanError(err)
	}
	warnSkipped(rep)

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
