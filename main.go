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
	"fmt"
	"os"
	"strings"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
	"github.com/backendArchitect/gospect-mcp/internal/graph/cbm"
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
		}
	}

	ctx := context.Background()
	g, cleanup, scope := buildGraph(ctx)
	defer cleanup()

	// Standalone CLI mode for quick local use / testing.
	if len(os.Args) >= 3 && os.Args[1] == "scan" {
		rep, err := scan.ScanWithOptions(os.Args[2], scan.Options{
			Patterns: os.Args[3:], Graph: g, GraphScope: scope,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "scan error:", err)
			os.Exit(1)
		}
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

	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
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
