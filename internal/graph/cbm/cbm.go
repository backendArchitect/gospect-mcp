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
		line, _ := strconv.Atoi(row[3])
		out = append(out, graph.Symbol{
			QualifiedName: row[0],
			Name:          row[1],
			File:          row[2],
			Line:          line,
		})
	}
	return out, nil
}

// cypherEscape neutralizes single quotes so a scope substring can't break out of the string literal.
func cypherEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
