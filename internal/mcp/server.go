// Package mcp is a minimal MCP server over stdio: newline-delimited JSON-RPC 2.0.
// Hand-rolled on the stdlib (ponytail rung 3) — the protocol subset we need
// (initialize, tools/list, tools/call) is small enough not to warrant an SDK dependency.
// The server itself uses no AI; the connecting host supplies the model.
package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

const protocolVersion = "2024-11-05"

// Tool is a callable exposed over tools/list + tools/call. Handler returns text
// (the report) or an error; it must never mutate the caller's code.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(args json.RawMessage) (string, error)
}

// Server is a tiny JSON-RPC dispatcher.
type Server struct {
	name    string
	version string
	tools   map[string]Tool
}

func NewServer(name, version string) *Server {
	return &Server{name: name, version: version, tools: map[string]Tool{}}
}

func (s *Server) Register(t Tool) { s.tools[t.Name] = t }

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads newline-delimited JSON-RPC requests from r and writes responses to w
// until EOF. Notifications (no id) get no response.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	enc := json.NewEncoder(w)
	for {
		line, err := br.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			s.handleLine(line, enc)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (s *Server) handleLine(line []byte, enc *json.Encoder) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return // unparseable line; nothing to respond to
	}
	notification := len(req.ID) == 0 || string(req.ID) == "null"
	result, rerr := s.dispatch(req)
	if notification {
		return
	}
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	_ = enc.Encode(&resp) // Encoder.Encode appends '\n' → newline-delimited framing
}

func (s *Server) dispatch(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}, nil

	case "notifications/initialized", "notifications/cancelled":
		return nil, nil

	case "ping":
		return map[string]any{}, nil

	case "tools/list":
		tools := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		return map[string]any{"tools": tools}, nil

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		t, ok := s.tools[p.Name]
		if !ok {
			return nil, &rpcError{Code: -32601, Message: "unknown tool: " + p.Name}
		}
		text, err := t.Handler(p.Arguments)
		if err != nil {
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}, nil
		}
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": false,
		}, nil

	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}
