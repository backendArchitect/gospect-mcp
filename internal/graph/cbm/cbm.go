// Package cbm adapts a codebase-memory-mcp server to the graph.Graph interface, reached via an
// mcp.Client and the server's query_graph (Cypher) tool.
//
// Contract notes verified against a live codebase-memory-mcp:
//   - query_graph returns {"columns":[...],"rows":[[...string...]],"total":N} (values are strings).
//   - Function nodes store is_exported / is_test as the strings "true"/"false".
//   - TESTS edges point (test)-[:TESTS]->(code).
//   - This engine rejects NOT-pattern predicates and miscounts count(null), so "untested" is
//     computed as a set-difference in Go rather than negated in Cypher.
package cbm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
)

// toolCaller is the slice of *mcp.Client this adapter needs (also lets tests inject a fake).
type toolCaller interface {
	CallTool(name string, args any) (string, error)
}

// Client adapts codebase-memory-mcp to graph.Graph.
type Client struct {
	mc      toolCaller
	project string
}

func New(mc toolCaller, project string) *Client {
	return &Client{mc: mc, project: project}
}

type queryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func (c *Client) query(cypher string) (*queryResult, error) {
	text, err := c.mc.CallTool("query_graph", map[string]any{"query": cypher, "project": c.project})
	if err != nil {
		return nil, err
	}
	var qr queryResult
	if err := json.Unmarshal([]byte(text), &qr); err != nil {
		return nil, fmt.Errorf("parse query_graph result: %w", err)
	}
	return &qr, nil
}

// UntestedExports returns exported, non-test functions under scope with no incoming TESTS edge.
func (c *Client) UntestedExports(ctx context.Context, scope string) ([]graph.Symbol, error) {
	s := cypherEscape(scope)

	exported, err := c.query(fmt.Sprintf(
		"MATCH (f:Function) WHERE f.is_exported = 'true' AND f.is_test = 'false' "+
			"AND f.file_path CONTAINS '%s' "+
			"RETURN f.qualified_name, f.name, f.file_path, f.start_line", s))
	if err != nil {
		return nil, err
	}

	tested, err := c.query(fmt.Sprintf(
		"MATCH (t)-[:TESTS]->(f:Function) WHERE f.file_path CONTAINS '%s' "+
			"RETURN DISTINCT f.qualified_name", s))
	if err != nil {
		return nil, err
	}
	testedSet := make(map[string]bool, len(tested.Rows))
	for _, row := range tested.Rows {
		if len(row) > 0 {
			testedSet[row[0]] = true
		}
	}

	var out []graph.Symbol
	for _, row := range exported.Rows {
		if len(row) < 4 || testedSet[row[0]] {
			continue
		}
		out = append(out, graph.Symbol{
			QualifiedName: row[0],
			Name:          row[1],
			File:          row[2],
			Line:          atoi(row[3]),
		})
	}
	return out, nil
}

// HighComplexity returns non-test functions/methods under scope whose cyclomatic OR cognitive
// complexity meets the minimums. The metrics are stored as strings and this dialect has no
// toInteger(), so we fetch and threshold in Go.
func (c *Client) HighComplexity(ctx context.Context, scope string, minCyclomatic, minCognitive int) ([]graph.HotSpot, error) {
	s := cypherEscape(scope)
	var out []graph.HotSpot
	for _, label := range []string{"Function", "Method"} {
		qr, err := c.query(fmt.Sprintf(
			"MATCH (f:%s) WHERE f.is_test = 'false' AND f.file_path CONTAINS '%s' "+
				"RETURN f.qualified_name, f.name, f.file_path, f.start_line, f.complexity, f.cognitive", label, s))
		if err != nil {
			return nil, err
		}
		for _, row := range qr.Rows {
			if len(row) < 6 {
				continue
			}
			cyc, cog := atoi(row[4]), atoi(row[5])
			if cyc < minCyclomatic && cog < minCognitive {
				continue
			}
			out = append(out, graph.HotSpot{
				Symbol:     graph.Symbol{QualifiedName: row[0], Name: row[1], File: row[2], Line: atoi(row[3])},
				Cyclomatic: cyc,
				Cognitive:  cog,
			})
		}
	}
	return out, nil
}

// UnhandledRoutes returns HTTP routes (non-empty method) with no incoming HANDLES edge. Routes
// have no file_path in the graph, so this is graph-wide; missing handlers are computed as a
// set-difference (all routes − handled routes) keyed by qualified_name.
func (c *Client) UnhandledRoutes(ctx context.Context) ([]graph.Route, error) {
	all, err := c.query("MATCH (r:Route) RETURN r.qualified_name, r.name, r.method")
	if err != nil {
		return nil, err
	}
	handled, err := c.query("MATCH (h)-[:HANDLES]->(r:Route) RETURN DISTINCT r.qualified_name")
	if err != nil {
		return nil, err
	}
	handledSet := make(map[string]bool, len(handled.Rows))
	for _, row := range handled.Rows {
		if len(row) > 0 {
			handledSet[row[0]] = true
		}
	}

	var out []graph.Route
	for _, row := range all.Rows {
		if len(row) < 3 {
			continue
		}
		qn, name, method := row[0], row[1], row[2]
		// Skip non-HTTP routes (gRPC/external targets carry an empty method) and handled ones.
		if method == "" || handledSet[qn] {
			continue
		}
		out = append(out, graph.Route{Method: method, Path: name, QualifiedName: qn})
	}
	return out, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// cypherEscape neutralizes single quotes so a scope substring can't break out of the string literal.
func cypherEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
