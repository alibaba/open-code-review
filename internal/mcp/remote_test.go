// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRemoteDiscoveryAndAuthorizedCall(t *testing.T) {
	var calls atomic.Int32
	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "open-code-review-test", Version: "v0.0.1"}, nil,
	)
	server.AddTool(&sdkmcp.Tool{
		Name: "echo", Description: "remote echo",
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		var arguments struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: "echo: " + arguments.Message},
		}}, nil
	})
	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer remote-secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	defer httpServer.Close()

	config := MCPServerConfig{
		Type: "remote", URL: httpServer.URL,
		Headers: map[string]string{"Authorization": "Bearer remote-secret"},
	}
	client, err := NewConfiguredClient(context.Background(), "remote-test", config, "", "v0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	discovered := client.DiscoveredTools()
	if len(discovered) != 1 || discovered[0].Name != "echo" || calls.Load() != 0 {
		t.Fatalf("discovery = %#v, tool calls=%d", discovered, calls.Load())
	}

	config.Tools = []string{"echo"}
	config.DefaultPermission = PermissionAllow
	config.ToolDefinitionSHA256 = map[string]string{"echo": discovered[0].DefinitionSHA256}
	registry := tool.NewRegistry()
	registration, err := RegisterSelected(
		registry, []*Client{client}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow},
		map[string]MCPServerConfig{"remote-test": config}, NewRuntimeAuthorizer(false, time.Second, nil), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := registry.Get(registration.ToolDefs[0].Function.Name)
	if !ok {
		t.Fatal("remote provider is not registered")
	}
	result, err := provider.Execute(context.Background(), map[string]any{"message": "hello"})
	if err != nil || result != "echo: hello" || calls.Load() != 1 {
		t.Fatalf("Execute() = (%q, %v), tool calls=%d", result, err, calls.Load())
	}
	if strings.Contains(result, "remote-secret") {
		t.Fatal("remote credential leaked into tool output")
	}
}
