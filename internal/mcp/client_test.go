// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiscoverToolPagesSanitizesAndPaginates(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	list := func(_ context.Context, params *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
		switch calls.Add(1) {
		case 1:
			if params.Cursor != "" {
				t.Fatalf("first cursor = %q, want empty", params.Cursor)
			}
			return &sdkmcp.ListToolsResult{
				NextCursor: "next",
				Tools: []*sdkmcp.Tool{{
					Name: "search", Description: "find secret-value\x1b[31m now",
					InputSchema: map[string]any{
						"properties": map[string]any{"q": map[string]any{
							"type": "string", "description": "secret-value\x00 query",
						}, "bad\x1b[31m": map[string]any{"type": "string"}},
					},
					Annotations: &sdkmcp.ToolAnnotations{}, Title: "untrusted title",
				}},
			}, nil
		case 2:
			if params.Cursor != "next" {
				t.Fatalf("second cursor = %q, want next", params.Cursor)
			}
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{
				Name: "write", InputSchema: map[string]any{"type": "object"},
			}}}, nil
		default:
			t.Fatal("unexpected discovery request")
			return nil, nil
		}
	}
	config := MCPServerConfig{Type: "remote", URL: "https://example.test/mcp"}
	tools, discovered, err := discoverToolPages(context.Background(), "server", config, []string{"secret-value"}, "", list)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || len(discovered) != 2 || calls.Load() != 2 {
		t.Fatalf("discovery = %d tools/%d records/%d calls, want 2/2/2", len(tools), len(discovered), calls.Load())
	}
	if !discovered[0].ServerProvidedHint {
		t.Error("annotations/title should set an informational hint")
	}
	for _, text := range []string{discovered[0].Description, string(discovered[0].InputSchema)} {
		if strings.Contains(text, "secret-value") || strings.Contains(text, "\x1b") || strings.Contains(text, "\x00") {
			t.Fatalf("catalog retained secret/control metadata: %q", text)
		}
	}
	if tools[0].Annotations != nil || tools[0].Title != "" || len(tools[0].Icons) != 0 {
		t.Fatal("server annotations/title/icons escaped into the safe tool view")
	}
	if discovered[0].DefinitionSHA256 == "" {
		t.Fatal("discovery did not fingerprint the sanitized definition")
	}
}

func TestDiscoverToolPagesRejectsMalformedCatalogs(t *testing.T) {
	t.Parallel()

	tool := func(name string) *sdkmcp.Tool {
		return &sdkmcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}
	}
	tests := []struct {
		name string
		list listToolsFunc
	}{
		{name: "nil result", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) { return nil, nil }},
		{name: "list error is redacted", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return nil, errors.New("Bearer xy at https://example.test/mcp?token=xy")
		}},
		{name: "nil tool", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{nil}}, nil
		}},
		{name: "duplicate tool", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{tool("same"), tool("same")}}, nil
		}},
		{name: "repeated cursor", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{NextCursor: "again"}, nil
		}},
		{name: "too many tools", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			items := make([]*sdkmcp.Tool, maxDiscoveredTools+1)
			for i := range items {
				items[i] = tool(fmt.Sprintf("tool-%d", i))
			}
			return &sdkmcp.ListToolsResult{Tools: items}, nil
		}},
		{name: "too many pages", list: func(_ context.Context, params *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{NextCursor: params.Cursor + "x"}, nil
		}},
		{name: "oversized ignored metadata", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			item := tool("metadata")
			item.Title = strings.Repeat("x", maxCatalogBytes+1)
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{item}}, nil
		}},
		{name: "schema key collision after sanitization", list: func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{
				Name: "collision",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"safe": map[string]any{"type": "string"}, "safe\x1b[31m": map[string]any{"type": "string"},
				}},
			}}}, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := discoverToolPages(
				context.Background(), "server",
				MCPServerConfig{Type: "remote", URL: "https://example.test/mcp?token=xy"},
				[]string{"Bearer xy", "xy"}, "https://example.test/mcp?token=xy", tt.list,
			)
			if err == nil {
				t.Fatal("expected malformed discovery to fail")
			}
			if strings.Contains(err.Error(), "Bearer xy") || strings.Contains(err.Error(), "token=xy") {
				t.Fatalf("discovery error leaked a credential: %v", err)
			}
		})
	}
}

func TestClientCallToolEnforcesResultAndRedactionBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("redacts successful content", func(t *testing.T) {
		client := &Client{
			redactValues: []string{"Bearer xy", "xy"},
			redactURL:    "https://example.test/mcp?token=xy",
			callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
				return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{
					Text: "Bearer xy https://example.test/mcp?token=xy call-secret visible\x1b[2J",
				}}}, nil
			},
		}
		got, err := client.callTool(context.Background(), "echo", map[string]any{
			"nested": map[string]any{"api_key": "call-secret", "query": "visible"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "xy") || strings.Contains(got, "token=") || strings.Contains(got, "call-secret") || strings.Contains(got, "\x1b") {
			t.Fatalf("result was not sanitized: %q", got)
		}
		if !strings.Contains(got, "visible") {
			t.Fatalf("non-sensitive argument value was unnecessarily redacted: %q", got)
		}
	})

	t.Run("redacts transport error", func(t *testing.T) {
		client := &Client{redactValues: []string{"tiny"}, callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
			return nil, errors.New("server echoed tiny and call-secret\ntoken=server-created-secret")
		}}
		_, err := client.callTool(context.Background(), "echo", map[string]any{"api_key": "call-secret"})
		if err == nil || strings.Contains(err.Error(), "tiny") || strings.Contains(err.Error(), "call-secret") || strings.Contains(err.Error(), "server-created-secret") {
			t.Fatalf("transport error was not redacted: %v", err)
		}
	})

	t.Run("redacts newly returned structured credentials", func(t *testing.T) {
		client := &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{
				Text: `{"value":"visible","token":"server-created-secret","nested":{"headers":{"X-Key":"also-secret"}}}`,
			}}}, nil
		}}
		got, err := client.callTool(context.Background(), "echo", nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "server-created-secret") || strings.Contains(got, "also-secret") || !strings.Contains(got, "visible") || !strings.Contains(got, "[redacted]") {
			t.Fatalf("structured result was not safely redacted: %q", got)
		}
	})

	t.Run("server error is typed", func(t *testing.T) {
		client := &Client{redactValues: []string{"secret"}, callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{IsError: true, Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "failed secret call-secret"}}}, nil
		}}
		_, err := client.callTool(context.Background(), "echo", map[string]any{"payload": []any{map[string]any{"api_key": "call-secret"}}})
		var executionErr *ToolExecutionError
		if !errors.As(err, &executionErr) || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "call-secret") {
			t.Fatalf("error = %v, want redacted ToolExecutionError", err)
		}
	})

	t.Run("oversized result", func(t *testing.T) {
		client := &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: strings.Repeat("x", maxToolResultBytes+1)}}}, nil
		}}
		if _, err := client.callTool(context.Background(), "echo", nil); err == nil {
			t.Fatal("expected oversized result to fail")
		}
	})

	t.Run("oversized ignored structured result", func(t *testing.T) {
		client := &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{
				Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: "small"}},
				StructuredContent: map[string]any{"ignored": strings.Repeat("x", maxToolResultBytes)},
			}, nil
		}}
		if _, err := client.callTool(context.Background(), "echo", nil); err == nil {
			t.Fatal("expected oversized structured result to fail")
		}
	})

	t.Run("canceled response is discarded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client := &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
			cancel()
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "must not escape"}}}, nil
		}}
		if got, err := client.callTool(ctx, "echo", nil); got != "" || !errors.Is(err, context.Canceled) {
			t.Fatalf("callTool() = (%q, %v), want canceled without output", got, err)
		}
	})
}

func TestRemoteConnectionValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawURL        string
		allowInsecure bool
		wantErr       bool
	}{
		{name: "https", rawURL: "https://example.test/mcp"},
		{name: "loopback http", rawURL: "http://127.0.0.1:8080/mcp"},
		{name: "localhost http", rawURL: "http://localhost:8080/mcp"},
		{name: "remote http denied", rawURL: "http://example.test/mcp", wantErr: true},
		{name: "remote http explicit", rawURL: "http://example.test/mcp", allowInsecure: true},
		{name: "userinfo", rawURL: "https://user:secret@example.test/mcp", wantErr: true},
		{name: "fragment", rawURL: "https://example.test/mcp#secret", wantErr: true},
		{name: "unsupported scheme", rawURL: "file:///tmp/mcp", wantErr: true},
		{name: "missing host", rawURL: "https:///mcp", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateRemoteURL(tt.rawURL, tt.allowInsecure)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateRemoteURL() error = %v, wantErr=%v", err, tt.wantErr)
			}
		})

	}
}

func TestValidateMCPConnectionRejectsMixedAndDuplicateFields(t *testing.T) {
	t.Parallel()

	tests := []MCPServerConfig{
		{Type: "stdio", Command: "server", URL: "https://example.test/mcp"},
		{Type: "remote", URL: "https://example.test/mcp", Command: "server"},
		{Type: "stdio", Command: "server", Env: []string{"TOKEN=one", "TOKEN=two"}},
		{Type: "remote", URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one", "x-test": "two"}},
	}
	for _, config := range tests {
		if err := ValidateMCPConnection(config); err == nil {
			t.Errorf("ValidateMCPConnection(%+v) succeeded", config)
		}
	}
}

func TestNewConfiguredClientRejectsUnsafeServerNameBeforeConnection(t *testing.T) {
	t.Parallel()

	_, err := NewConfiguredClient(
		context.Background(), "server\x1b[2J",
		MCPServerConfig{Type: "stdio", Command: "must-not-run"}, "", "test",
	)
	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("NewConfiguredClient() error = %v, want unsafe-name rejection", err)
	}
}

func TestHeaderValidationAndTransportOrigin(t *testing.T) {
	t.Setenv("OCR_MCP_HEADER_TOKEN", "xy")
	expanded, redactions, err := validateAndExpandHeaders(map[string]string{"Authorization": "Bearer $OCR_MCP_HEADER_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if expanded["Authorization"] != "Bearer xy" || !containsString(redactions, "xy") {
		t.Fatalf("expanded headers/redactions = %#v/%#v", expanded, redactions)
	}

	for _, headers := range []map[string]string{
		{"Accept": "application/json"},
		{"MCP-Session-Id": "value"},
		{"Bad Header": "value"},
		{"X-Test": "line\r\ninjected"},
		{"X-Test": "$OCR_MCP_UNSET_VALUE_FOR_TEST"},
		{"X-Dupe": "one", "x-dupe": "two"},
	} {
		if _, _, err := validateAndExpandHeaders(headers); err == nil {
			t.Fatalf("expected headers %#v to fail", headers)
		}
	}

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xy" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	endpoint, err := validateRemoteURL(origin.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	transport := &headerTransport{base: http.DefaultTransport, headers: expanded, serverName: "server", origin: endpointOrigin(endpoint)}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, origin.URL, nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if request.Header.Get("Authorization") != "" {
		t.Fatal("transport mutated the caller's request")
	}

	request, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, "https://other.test/mcp", nil)
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("expected cross-origin request to fail before sending credentials")
	}
}

func TestBuildStdioEnvIsMinimalAndRedacted(t *testing.T) {
	t.Setenv("OCR_MCP_AMBIENT_SECRET", "ambient-secret")
	t.Setenv("LC_SECRET", "locale-shaped-secret")
	t.Setenv("OCR_MCP_SOURCE_SECRET", "z")
	environment, redactions, err := buildStdioEnv([]string{"OCR_CHILD_SECRET=$OCR_MCP_SOURCE_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "OCR_MCP_AMBIENT_SECRET") || strings.Contains(joined, "ambient-secret") ||
		strings.Contains(joined, "LC_SECRET") || strings.Contains(joined, "locale-shaped-secret") {
		t.Fatalf("ambient credential inherited by child: %s", joined)
	}
	if !strings.Contains(joined, "OCR_CHILD_SECRET=z") || !containsString(redactions, "z") {
		t.Fatalf("explicit child environment/redaction missing: %s / %#v", joined, redactions)
	}
	if _, _, err := buildStdioEnv([]string{"BAD-KEY=value"}); err == nil {
		t.Fatal("expected invalid environment key to fail")
	}
	if _, _, err := buildStdioEnv([]string{"A=one", "A=two"}); err == nil {
		t.Fatal("expected duplicate environment key to fail")
	}
}

func TestCommandArgumentRedactions(t *testing.T) {
	t.Parallel()

	values := commandArgumentRedactions([]string{
		"--token=first", "--api-key", "second", "https://example.test/mcp?credential=third", "visible",
	})
	for _, secret := range []string{"first", "second", "third"} {
		if !containsString(values, secret) {
			t.Errorf("redactions %#v do not contain %q", values, secret)
		}
	}
	if containsString(values, "visible") {
		t.Fatal("non-secret argument was marked as a credential")
	}
}

func TestContentToText(t *testing.T) {
	t.Parallel()

	got, err := contentToText([]sdkmcp.Content{
		&sdkmcp.TextContent{Text: "one"}, &sdkmcp.TextContent{Text: "two"},
	})
	if err != nil || got != "one\ntwo" {
		t.Fatalf("contentToText() = (%q, %v), want joined text", got, err)
	}
	if got, err := contentToText([]sdkmcp.Content{&sdkmcp.ImageContent{MIMEType: "image/png", Data: []byte("x")}}); err != nil || got == "" {
		t.Fatalf("unsupported content placeholder = (%q, %v)", got, err)
	}
}

func TestCloseIsIdempotentForEmptyClient(t *testing.T) {
	t.Parallel()

	if err := (*Client)(nil).Close(); err != nil {
		t.Fatal(err)
	}
	client := &Client{
		name: "server", redactValues: []string{"close-secret"},
		closeFunc: func() error { return errors.New("close failed: close-secret") },
	}
	if err := client.Close(); err == nil || strings.Contains(err.Error(), "close-secret") {
		t.Fatalf("Close() error was not redacted: %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
