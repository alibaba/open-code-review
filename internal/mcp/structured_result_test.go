// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCallTool_StructuredResults(t *testing.T) {
	const limit = 10 * 1024
	const notice = "\n[Structured result truncated at 10 KiB]"
	const objectPrefix = `{"value":"`
	const errorPrefix = `MCP tool "result" returned an error: `
	type testCase struct {
		name   string
		result mcp.CallToolResult
		want   string
	}
	tests := []testCase{
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
			name:   "just below limit",
			result: mcp.CallToolResult{StructuredContent: map[string]any{"value": strings.Repeat("x", limit-13)}},
			want:   objectPrefix + strings.Repeat("x", limit-13) + `"}`,
		},
		{
			name:   "exactly at limit",
			result: mcp.CallToolResult{StructuredContent: map[string]any{"value": strings.Repeat("x", limit-12)}},
			want:   objectPrefix + strings.Repeat("x", limit-12) + `"}`,
		},
		{
			name:   "one byte over limit",
			result: mcp.CallToolResult{StructuredContent: map[string]any{"value": strings.Repeat("x", limit-11)}},
			want:   objectPrefix + strings.Repeat("x", limit-len(objectPrefix)-len(notice)) + notice,
		},
		{
			name: "large structured error includes prefix in limit",
			result: mcp.CallToolResult{
				IsError:           true,
				StructuredContent: map[string]any{"value": strings.Repeat("x", limit)},
			},
			want: errorPrefix + objectPrefix + strings.Repeat("x", limit-len(errorPrefix)-len(objectPrefix)-len(notice)) + notice,
		},
		{
			name: "existing large text has priority",
			result: mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", limit+1)}},
				StructuredContent: map[string]any{"value": "ignored"},
			},
			want: strings.Repeat("x", limit+1),
		},
		{
			name: "existing empty text has priority",
			result: mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: ""}},
				StructuredContent: map[string]any{"value": "ignored"},
			},
			want: "",
		},
		{
			name: "empty result",
			want: "",
		},
	}
	for _, character := range []string{"\u00e9", "\u754c", "\U0001f30d"} {
		tests = append(tests, testCase{
			name:   "truncate at UTF-8 boundary " + character,
			result: mcp.CallToolResult{StructuredContent: map[string]any{"value": strings.Repeat(character, limit/len(character)+1)}},
			want:   objectPrefix + strings.Repeat(character, (limit-len(objectPrefix)-len(notice))/len(character)) + notice,
		})
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
				t.Errorf("CallTool() = %.200q (%d bytes), want %.200q (%d bytes)", got, len(got), tt.want, len(tt.want))
			}
			if !utf8.ValidString(got) {
				t.Error("CallTool() returned invalid UTF-8")
			}
		})
	}
}
