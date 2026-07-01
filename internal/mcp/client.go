package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps a single MCP server connection via stdio transport.
type Client struct {
	name    string
	session *mcp.ClientSession
	tools   []*mcp.Tool
}

// NewClient starts an MCP server subprocess, initializes the connection,
// and caches the list of available tools. The context governs the
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

	transport := &mcp.CommandTransport{Command: cmd}
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
