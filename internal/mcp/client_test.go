package mcp

import (
	"encoding/json"
	"io"
	"testing"
)

// TestClientServerRoundTrip wires our Client to our Server over in-process pipes and confirms
// the handshake + a tool call work end-to-end (no external process).
func TestClientServerRoundTrip(t *testing.T) {
	c2sR, c2sW := io.Pipe() // client -> server
	s2cR, s2cW := io.Pipe() // server -> client

	srv := NewServer("fake-graph", "1.0.0")
	srv.Register(Tool{
		Name: "query_graph",
		Handler: func(args json.RawMessage) (string, error) {
			return `{"columns":["qn"],"rows":[["pkg.A"]]}`, nil
		},
	})
	go func() { _ = srv.Serve(c2sR, s2cW) }()
	defer c2sW.Close() // EOF -> server loop exits

	client := NewClient(s2cR, c2sW, nil)
	if err := client.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	text, err := client.CallTool("query_graph", map[string]any{"query": "MATCH ...", "project": "p"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	want := `{"columns":["qn"],"rows":[["pkg.A"]]}`
	if text != want {
		t.Fatalf("unexpected tool text:\n got: %s\nwant: %s", text, want)
	}
}
