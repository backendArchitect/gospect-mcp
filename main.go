// Command gospect-mcp is a report-first, Go-only code scanner exposed as an MCP server.
//
// Two ways to run it:
//
//	gospect-mcp                 # MCP server over stdio (default)
//	gospect-mcp scan <dir> ...  # standalone CLI: print a JSON report
//
// It never modifies code. Fixes are a separate, explicitly-invoked step (not yet implemented).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/backendArchitect/gospect-mcp/internal/mcp"
	"github.com/backendArchitect/gospect-mcp/internal/scan"
)

const version = "0.1.0"

func main() {
	// Standalone CLI mode for quick local use / testing.
	if len(os.Args) >= 3 && os.Args[1] == "scan" {
		rep, err := scan.Scan(os.Args[2], os.Args[3:]...)
		if err != nil {
			fmt.Fprintln(os.Stderr, "scan error:", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
		return
	}

	// Default: MCP server over stdio.
	srv := mcp.NewServer("gospect-mcp", version)
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
			rep, err := scan.Scan(a.Path, a.Patterns...)
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
