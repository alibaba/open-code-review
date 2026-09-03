// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// SubprocessTerminateDuration bounds each escalation stage of the SDK's
	// stdio subprocess shutdown: the wait after closing stdin, the wait after
	// SIGTERM, and the wait after SIGKILL. The SDK default is 5s per stage,
	// so one unresponsive server could hold Close for ~15s (#1141). 800ms
	// per stage keeps a single server's worst case at ~2.4s while still
	// giving a well-behaved server ample time to exit on its own.
	SubprocessTerminateDuration = 800 * time.Millisecond

	// CloseTimeout bounds the whole MCP close phase regardless of how many
	// servers are configured: CloseAll stops waiting once it expires and
	// leaves any remaining subprocess to the OS. It sits above the
	// per-server worst case (3 stages x SubprocessTerminateDuration = 2.4s)
	// with headroom for scheduling jitter, yet still inside the 3s shutdown
	// budget, docker stop's 10s grace period, and the VS Code extension's 3s
	// SIGKILL escalation.
	CloseTimeout = 2800 * time.Millisecond
)

// Client wraps a single MCP server connection.
type Client struct {
	name    string
	session *mcp.ClientSession
	tools   []*mcp.Tool
}

// subprocessTerminateDuration is the per-stage shutdown budget handed to every
// CommandTransport. It defaults to SubprocessTerminateDuration and is widened
// only by tests running under the race detector, whose re-exec'd children pay
// ~1s of runtime overhead before noticing stdin EOF.
var subprocessTerminateDuration = SubprocessTerminateDuration

// NewClient starts an MCP server subprocess (stdio transport), initializes the
// connection, and caches the list of available tools. The context governs the
// initialization timeout (Connect + ListTools), NOT the subprocess
// lifetime — the subprocess stays alive until Close is called.
// When dir is non-empty, the subprocess runs with that working directory.
func NewClient(ctx context.Context, name, command string, args, env []string, dir, version string) (*Client, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	if dir != "" {
		cmd.Dir = dir
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "open-code-review", Version: version},
		nil,
	)

	transport := &mcp.CommandTransport{Command: cmd, TerminateDuration: subprocessTerminateDuration}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP server %q: %w", name, err)
	}

	var success bool
	defer func() {
		if !success {
			session.Close()
		}
	}()

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools from MCP server %q: %w", name, err)
	}

	success = true
	return &Client{
		name:    name,
		session: session,
		tools:   toolsResult.Tools,
	}, nil
}

// NewRemoteClient connects to a remote MCP server via Streamable HTTP transport.
// Header values may contain $ENV_VAR references which are expanded at runtime.
// Returns an error if any header value expands to an empty string.
func NewRemoteClient(ctx context.Context, name, url string, headers map[string]string, version string) (*Client, error) {
	var expanded map[string]string
	if len(headers) > 0 {
		expanded = make(map[string]string, len(headers))
		for k, v := range headers {
			expanded[k] = os.Expand(v, os.Getenv)
			if expanded[k] == "" {
				return nil, fmt.Errorf("MCP server %q header %q expanded to empty value — check your environment variables", name, k)
			}
		}
	}
	httpClient := &http.Client{
		Transport: &headerTransport{
			base:       http.DefaultTransport,
			headers:    expanded,
			serverName: name,
		},
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "open-code-review", Version: version},
		nil,
	)

	transport := &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: httpClient,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to remote MCP server %q at %s: %w", name, url, err)
	}

	var success bool
	defer func() {
		if !success {
			session.Close()
		}
	}()

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools from remote MCP server %q: %w", name, err)
	}

	success = true
	return &Client{
		name:    name,
		session: session,
		tools:   toolsResult.Tools,
	}, nil
}

// headerTransport injects custom headers into every HTTP request and surfaces
// clear authentication errors for 401/403 responses.
type headerTransport struct {
	base       http.RoundTripper
	headers    map[string]string
	serverName string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	for k, v := range t.headers {
		cloned.Header.Set(k, v)
	}
	resp, err := t.base.RoundTrip(cloned)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("remote MCP server %q returned HTTP 401 Unauthorized — check your token/header configuration", t.serverName)
	case http.StatusForbidden:
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("remote MCP server %q returned HTTP 403 Forbidden — your credentials may lack required permissions", t.serverName)
	}
	return resp, nil
}

func (c *Client) Name() string       { return c.name }
func (c *Client) Tools() []*mcp.Tool { return c.tools }

// CallTool invokes a tool on the MCP server and returns the text result.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	params := &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	}

	result, err := c.session.CallTool(ctx, params)
	if err != nil {
		return "", fmt.Errorf("call MCP tool %q: %w", name, err)
	}

	if result.IsError {
		return fmt.Sprintf("MCP tool %q returned an error: %s", name, contentToText(result.Content)), nil
	}

	return contentToText(result.Content), nil
}

func (c *Client) Close() error {
	return c.session.Close()
}

// CloseAll closes every client concurrently and waits for all of them, or for
// ctx to expire, whichever comes first. Serial closing multiplied the SDK's
// per-server shutdown bound by the number of configured servers (#1141);
// closing in parallel keeps the whole phase at the single slowest server's
// cost, and the ctx deadline caps even that.
//
// Each per-client error is wrapped with the server name, and on the happy path
// the results are joined in configuration order, so one warning can report
// every failed server. When ctx expires first, a final non-blocking check
// prefers completion over a timeout report (the last Close can finish at the
// same instant the deadline fires); on a genuine timeout the returned error
// wraps the deadline failure, reports how many servers finished, how many of
// those had errors, and how many were still shutting down, keeping any errors
// already collected from servers that finished in time: their
// Close goroutines keep running and the SDK still escalates SIGTERM to
// SIGKILL, but if the process exits first the remaining subprocesses are left
// to the OS.
func CloseAll(ctx context.Context, clients []*Client) error {
	if len(clients) == 0 {
		return nil
	}

	errs := make([]error, len(clients))
	remaining := len(clients)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for i, c := range clients {
		go func() {
			defer wg.Done()
			err := c.Close()
			mu.Lock()
			remaining--
			if err != nil {
				errs[i] = fmt.Errorf("close MCP server %q: %w", c.name, err)
			}
			mu.Unlock()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		return errors.Join(errs...)
	case <-ctx.Done():
		// The last Close() can complete at nearly the same instant the
		// deadline expires; with both cases ready Go picks one
		// pseudo-randomly, so prefer completion over reporting a spurious
		// timeout for a fully successful close (#1141 review feedback).
		select {
		case <-done:
			mu.Lock()
			defer mu.Unlock()
			return errors.Join(errs...)
		default:
		}
		mu.Lock()
		defer mu.Unlock()
		errorCount := 0
		for _, err := range errs {
			if err != nil {
				errorCount++
			}
		}
		if joined := errors.Join(errs...); joined != nil {
			return fmt.Errorf("%w; %d of %d server(s) closed before the deadline, %d with errors, %d still shutting down: %w",
				ctx.Err(), len(clients)-remaining, len(clients), errorCount, remaining, joined)
		}
		return fmt.Errorf("timed out closing %d MCP server(s): %w; %d still shutting down, subprocesses left to the OS", len(clients), ctx.Err(), remaining)
	}
}

func contentToText(contents []mcp.Content) string {
	var parts []string
	for _, item := range contents {
		switch v := item.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		default:
			parts = append(parts, fmt.Sprintf("[unsupported content type: %T]", item))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}
