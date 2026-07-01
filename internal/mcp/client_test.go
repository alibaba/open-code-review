package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestContentToText_SingleText(t *testing.T) {
	contents := []mcp.Content{
		&mcp.TextContent{Text: "hello"},
	}
	got := contentToText(contents)
	if got != "hello" {
		t.Errorf("contentToText() = %q, want %q", got, "hello")
	}
}

func TestContentToText_MultipleText(t *testing.T) {
	contents := []mcp.Content{
		&mcp.TextContent{Text: "line1"},
		&mcp.TextContent{Text: "line2"},
	}
	got := contentToText(contents)
	want := "line1\nline2"
	if got != want {
		t.Errorf("contentToText() = %q, want %q", got, want)
	}
}

func TestContentToText_Empty(t *testing.T) {
	got := contentToText(nil)
	if got != "" {
		t.Errorf("contentToText(nil) = %q, want empty", got)
	}

	got = contentToText([]mcp.Content{})
	if got != "" {
		t.Errorf("contentToText([]) = %q, want empty", got)
	}
}

func TestContentToText_UnsupportedType(t *testing.T) {
	contents := []mcp.Content{
		&mcp.ImageContent{MIMEType: "image/png", Data: []byte("data")},
	}
	got := contentToText(contents)
	if got == "" {
		t.Error("expected non-empty for unsupported type")
	}
}

func TestClient_NameAndTools(t *testing.T) {
	tools := []*mcp.Tool{{Name: "t1"}, {Name: "t2"}}
	c := &Client{name: "test-srv", tools: tools}

	if c.Name() != "test-srv" {
		t.Errorf("Name() = %q, want %q", c.Name(), "test-srv")
	}
	if len(c.Tools()) != 2 {
		t.Errorf("Tools() len = %d, want 2", len(c.Tools()))
	}
}

func TestProvider_Execute_Integration(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "v0.0.1"},
		nil,
	)
	server.AddTool(
		&mcp.Tool{
			Name:        "greet",
			Description: "Greets",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "hello " + args.Name}},
			}, nil
		},
	)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	sdkClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v0.0.1"},
		nil,
	)
	session, err := sdkClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	c := &Client{name: "test-srv", session: session, tools: toolsResult.Tools}
	defer c.Close()

	p := &Provider{toolName: "greet", client: c}
	result, err := p.Execute(ctx, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Execute result = %q, want %q", result, "hello world")
	}
}

func TestNewClient_Integration(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "v0.0.1"},
		nil,
	)
	server.AddTool(
		&mcp.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + args.Message}},
			}, nil
		},
	)

	server.AddTool(
		&mcp.Tool{
			Name:        "fail",
			Description: "Always fails",
			InputSchema: map[string]any{"type": "object"},
		},
		func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "something went wrong"}},
			}, nil
		},
	)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	sdkClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v0.0.1"},
		nil,
	)
	session, err := sdkClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	c := &Client{
		name:    "test-srv",
		session: session,
		tools:   toolsResult.Tools,
	}
	defer c.Close()

	if c.Name() != "test-srv" {
		t.Errorf("Name() = %q", c.Name())
	}
	if len(c.Tools()) != 2 {
		t.Errorf("Tools() len = %d, want 2", len(c.Tools()))
	}

	t.Run("CallTool_success", func(t *testing.T) {
		result, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result != "echo: hi" {
			t.Errorf("CallTool result = %q, want %q", result, "echo: hi")
		}
	})

	t.Run("CallTool_error_result", func(t *testing.T) {
		result, err := c.CallTool(ctx, "fail", nil)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("Close", func(t *testing.T) {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}
