package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
)

// Client is a minimal MCP client over newline-delimited JSON-RPC — the client half of Server.
// gospect uses it to talk to another MCP server (e.g. codebase-memory-mcp) as a data source.
type Client struct {
	w  io.Writer
	br *bufio.Reader
	cl io.Closer
	mu sync.Mutex
	id int
}

// NewClient builds a client over an existing pair (r = server's output, w = server's input).
func NewClient(r io.Reader, w io.Writer, closer io.Closer) *Client {
	return &Client{w: w, br: bufio.NewReaderSize(r, 1<<20), cl: closer}
}

// DialCommand spawns name+args and speaks MCP over its stdio. Close terminates the subprocess.
func DialCommand(ctx context.Context, name string, args ...string) (*Client, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return NewClient(stdout, stdin, &cmdCloser{cmd: cmd, stdin: stdin}), nil
}

type cmdCloser struct {
	cmd   *exec.Cmd
	stdin io.Closer
}

func (c *cmdCloser) Close() error {
	_ = c.stdin.Close()
	_ = c.cmd.Process.Kill()
	return c.cmd.Wait()
}

// Close releases the transport (and subprocess, if this client owns one).
func (c *Client) Close() error {
	if c.cl != nil {
		return c.cl.Close()
	}
	return nil
}

func (c *Client) rpc(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	id := c.id
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.w.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	for {
		line, readErr := c.br.ReadBytes('\n')
		if len(line) > 0 {
			var resp struct {
				ID     json.RawMessage `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(line, &resp) == nil && len(resp.ID) > 0 && string(resp.ID) == strconv.Itoa(id) {
				if resp.Error != nil {
					return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
				}
				return resp.Result, nil
			}
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func (c *Client) notify(method string) error {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.w.Write(append(b, '\n'))
	return err
}

// Initialize performs the MCP handshake (initialize request + initialized notification).
func (c *Client) Initialize() error {
	if _, err := c.rpc("initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gospect-mcp", "version": "0.1.0"},
	}); err != nil {
		return err
	}
	return c.notify("notifications/initialized")
}

// CallTool invokes a tool and returns its first text content block.
func (c *Client) CallTool(name string, args any) (string, error) {
	res, err := c.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return "", err
	}
	if len(out.Content) == 0 {
		return "", nil
	}
	if out.IsError {
		return "", fmt.Errorf("tool %q error: %s", name, out.Content[0].Text)
	}
	return out.Content[0].Text, nil
}
