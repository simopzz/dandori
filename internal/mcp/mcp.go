// Package mcp is a minimal Model Context Protocol client over a server
// subprocess's stdio (ADR-0003): Dandori starts the Playwright MCP server
// and calls its tools to drive the user's logged-in browser.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// protocolVersion is the MCP revision the client speaks.
const protocolVersion = "2025-06-18"

// Client talks to one MCP server subprocess.
type Client struct {
	cmd *exec.Cmd
	in  io.WriteCloser

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan message
	done    chan struct{}
	err     error // why the server went away, once done is closed
}

// message is a JSON-RPC 2.0 message in either direction.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message) }

// Start runs command, a program and its arguments, as an MCP server and
// initialises the session. The server's stderr goes to Dandori's.
func Start(ctx context.Context, command []string) (*Client, error) {
	if len(command) == 0 {
		return nil, errors.New("no MCP server command")
	}
	// Not ctx: the server outlives the call that starts it.
	cmd := exec.Command(command[0], command[1:]...) //nolint:gosec // the user's own configured command
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start MCP server %q: %w", command[0], err)
	}
	c := &Client{cmd: cmd, in: in, pending: map[int64]chan message{}, done: make(chan struct{})}
	go c.read(out)

	var init struct {
		ServerInfo struct{ Name, Version string } `json:"serverInfo"`
	}
	err = c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "dandori", "version": "1"},
	}, &init)
	if err == nil {
		err = c.send(message{JSONRPC: "2.0", Method: "notifications/initialized"})
	}
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("initialise MCP server %q: %w", command[0], err)
	}
	return c, nil
}

// ToolResult is what a tool call returned: its text content, and whether
// the tool reports it failed.
type ToolResult struct {
	Text    string
	IsError bool
}

// CallTool calls the server's tool name with args.
func (c *Client) CallTool(ctx context.Context, name string, args any) (ToolResult, error) {
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &res); err != nil {
		return ToolResult{}, fmt.Errorf("MCP tool %s: %w", name, err)
	}
	var text []string
	for _, b := range res.Content {
		if b.Type == "text" {
			text = append(text, b.Text)
		}
	}
	return ToolResult{Text: strings.Join(text, "\n"), IsError: res.IsError}, nil
}

// Close stops the server.
func (c *Client) Close() error {
	c.in.Close()
	err := c.cmd.Process.Kill()
	c.cmd.Wait() //nolint:errcheck // killed on purpose
	return err
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	reply := make(chan message, 1)
	c.pending[id] = reply
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.send(message{JSONRPC: "2.0", ID: &id, Method: method, Params: params}); err != nil {
		return err
	}
	select {
	case m := <-reply:
		if m.Error != nil {
			return m.Error
		}
		return json.Unmarshal(m.Result, result)
	case <-c.done:
		return c.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) send(m message) error {
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.in.Write(append(line, '\n'))
	return err
}

// read dispatches the server's messages until its stdout closes: replies
// to their calls, and a refusal to any request of its own, which this
// client offers no capabilities for.
func (c *Client) read(out io.Reader) {
	sc := bufio.NewScanner(out)
	// A page snapshot can be large.
	sc.Buffer(nil, 64<<20)
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		switch {
		case m.ID != nil && m.Method == "":
			c.mu.Lock()
			reply, ok := c.pending[*m.ID]
			c.mu.Unlock()
			if ok {
				reply <- m
			}
		case m.ID != nil && m.Method == "ping":
			c.send(message{JSONRPC: "2.0", ID: m.ID, Result: json.RawMessage(`{}`)}) //nolint:errcheck // the server notices
		case m.ID != nil:
			c.send(message{JSONRPC: "2.0", ID: m.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}) //nolint:errcheck // as above
		}
	}
	c.err = errors.New("the MCP server stopped")
	if err := sc.Err(); err != nil {
		c.err = fmt.Errorf("the MCP server stopped: %w", err)
	}
	close(c.done)
}
