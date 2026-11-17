// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCallTool_StructuredResults(t *testing.T) {
	tests := []struct {
		name   string
		result mcp.CallToolResult
		want   string
	}{
		{
			name:   "object without text",
			result: mcp.CallToolResult{StructuredContent: map[string]any{"count": 1, "files": []string{"main.go"}}},
			want:   `{"count":1,"files":["main.go"]}`,
		},
		{
			name:   "empty object without text",
			result: mcp.CallToolResult{StructuredContent: map[string]any{}},
			want:   `{}`,
		},
		{
			name: "error object without text",
			result: mcp.CallToolResult{
				IsError:           true,
				StructuredContent: map[string]any{"reason": "missing repository"},
			},
			want: `MCP tool "result" returned an error: {"reason":"missing repository"}`,
		},
		{
			name: "existing text has priority",
			result: mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "first"}, &mcp.TextContent{Text: "second"}},
				StructuredContent: map[string]any{"count": 1},
			},
			want: "first\nsecond",
		},
		{
			name: "empty result",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "structured-server", Version: "test"}, nil)
			server.AddTool(&mcp.Tool{Name: "result", InputSchema: map[string]any{"type": "object"}},
				func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &tt.result, nil
				})
			st, ct := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), st, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = serverSession.Close() })
			sdkClient := mcp.NewClient(&mcp.Implementation{Name: "structured-client", Version: "test"}, nil)
			session, err := sdkClient.Connect(t.Context(), ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close() })
			client := &Client{name: "structured", session: session}
			got, err := client.CallTool(t.Context(), "result", nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("CallTool() = %q, want %q", got, tt.want)
			}
		})
	}
}
