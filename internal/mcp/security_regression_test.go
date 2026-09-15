// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const commandCredentialSentinel = "argument_secret_sentinel"

func TestConnectionSecretToolNamesAreRejected(t *testing.T) {
	const secret = "reflectedcredential"
	for _, name := range []string{secret, "prefix_" + secret, "read"} {
		tools, discovered, err := discoverToolPages(context.Background(), "test", MCPServerConfig{Command: "test"}, []string{secret}, "", func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{Name: name, InputSchema: map[string]any{"type": "object"}}}}, nil
		})
		if name == "read" {
			if err != nil || len(discovered) != 1 {
				t.Fatalf("ordinary tool rejected: %v", err)
			}
			continue
		}
		if err == nil || len(tools) != 0 || len(discovered) != 0 {
			t.Fatalf("secret-bearing name accepted: %s", name)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatal("secret in error")
		}
	}
}

func TestCommandCredentialFormsCannotReachCatalog(t *testing.T) {
	const secret = commandCredentialSentinel
	for _, args := range [][]string{
		{"--url=https://example.com/mcp?key=" + secret},
		{"https://user:" + secret + "@example.com/mcp"},
		{"--header", "Authorization: Bearer " + secret},
		{"--env", "API_TOKEN=" + secret},
	} {
		if strings.Contains(strings.Join(SafeCommandArguments(args), " "), secret) {
			t.Errorf("credential visible in command preview: %v", args)
		}
		values := commandArgumentRedactions(args)
		tools, _, err := discoverToolPages(context.Background(), "test", MCPServerConfig{Command: "test", Args: args}, values, "", func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
			return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{Name: secret, InputSchema: map[string]any{"type": "object"}}}}, nil
		})
		if err == nil || len(tools) > 0 {
			t.Errorf("reflected credential accepted for %v", args)
		}
		if strings.Contains(sanitizeMCPResultText(secret, values, ""), secret) {
			t.Errorf("reflected credential escaped result for %v", args)
		}
	}
}

func TestStdioCommandCredentialReflection(t *testing.T) {
	requireSubprocess(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const secret = commandCredentialSentinel
	for _, args := range [][]string{{"--url=https://example.com/mcp?key=" + secret}, {"https://user:" + secret + "@example.com/mcp"}, {"--header", "Authorization: Bearer " + secret}, {"--env", "API_TOKEN=" + secret}} {
		for _, mode := range []string{"name", "result", "error"} {
			config := MCPServerConfig{Command: executable, Args: args, Env: []string{runAsServerEnv + "=1", "_OCR_MCP_TEST_REFLECTION=" + mode}}
			client, err := NewConfiguredClient(context.Background(), "reflection", config, "", "1")
			if mode == "name" {
				if err == nil {
					_ = client.Close()
					t.Fatal("stdio catalog reflected credential")
				}
				if strings.Contains(err.Error(), secret) {
					t.Fatal("stdio discovery error leaked")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			catalog := client.DiscoveredTools()
			if len(catalog) != 1 || strings.Contains(catalog[0].Description, secret) {
				_ = client.Close()
				t.Fatal("stdio description leaked")
			}
			result, callErr := client.callTool(context.Background(), "echo", map[string]any{})
			_ = client.Close()
			if (mode == "error") != (callErr != nil) {
				t.Fatalf("tool error state changed: %v", callErr)
			}
			if strings.Contains(result, secret) || (callErr != nil && strings.Contains(callErr.Error(), secret)) {
				t.Fatal("stdio execution reflected credential")
			}
		}
	}
}

func TestCompositeCredentialsAcrossRemoteDiscoveryAndExecution(t *testing.T) {
	const secret = "remote_component_sentinel"
	t.Setenv("MCP_REMOTE_COMPONENT", secret)
	for _, mode := range []string{"tool-name", "result", "tool-error"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "credential-fixture", Version: "1"}, nil)
			name := "read"
			if mode == "tool-name" {
				name = "prefix_" + secret
			}
			server.AddTool(&sdkmcp.Tool{Name: name, Description: secret, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				calls.Add(1)
				return &sdkmcp.CallToolResult{IsError: mode == "tool-error", Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: secret}}}, nil
			})
			handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil)
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "session="+secret {
					t.Error("legitimate expanded header changed")
				}
				handler.ServeHTTP(w, r)
			}))
			defer endpoint.Close()
			config := MCPServerConfig{Type: "remote", URL: endpoint.URL, Headers: map[string]string{"Cookie": "session=${MCP_REMOTE_COMPONENT}"}}
			client, err := NewConfiguredClient(context.Background(), "remote", config, "", "1")
			if mode == "tool-name" {
				if err == nil {
					_ = client.Close()
					t.Fatal("secret-bearing catalog accepted")
				}
				if strings.Contains(err.Error(), secret) || calls.Load() != 0 {
					t.Fatal("unsafe discovery error or execution")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.Close() }()
			discovered := client.DiscoveredTools()
			if len(discovered) != 1 || strings.Contains(discovered[0].Description, secret) || calls.Load() != 0 {
				t.Fatal("metadata leaked or discovery invoked tool")
			}
			config.Tools = []string{name}
			config.DefaultPermission = PermissionAllow
			config.ToolDefinitionSHA256 = map[string]string{name: discovered[0].DefinitionSHA256}
			registry := tool.NewRegistry()
			registered, err := RegisterSelected(registry, []*Client{client}, MCPConfig{Version: 1}, map[string]MCPServerConfig{"remote": config}, NewRuntimeAuthorizer(false, time.Second, nil), false)
			if err != nil {
				t.Fatal(err)
			}
			provider, ok := registry.Get(registered.ToolDefs[0].Function.Name)
			if !ok {
				t.Fatal("missing provider")
			}
			result, err := provider.Execute(context.Background(), map[string]any{})
			if mode == "tool-error" && err == nil {
				t.Fatal("IsError was not propagated")
			}
			if mode == "result" && err != nil {
				t.Fatal(err)
			}
			if strings.Contains(result, secret) || (err != nil && strings.Contains(err.Error(), secret)) || calls.Load() != 1 {
				t.Fatal("credential escaped or tool call count changed")
			}
		})
	}
}

func TestCompositeCredentialsAreRedacted(t *testing.T) {
	t.Setenv("MCP_TEST_COMPONENT", "component_sentinel")
	t.Setenv("MCP_TEST_SECOND", "second_sentinel")
	for _, template := range []string{"session=${MCP_TEST_COMPONENT}", "prefix-${MCP_TEST_COMPONENT}-suffix", "${MCP_TEST_COMPONENT}:${MCP_TEST_SECOND}", "Bearer ${MCP_TEST_COMPONENT}"} {
		_, redactions, err := validateAndExpandHeaders(map[string]string{"Cookie": template})
		if err != nil {
			t.Fatal(err)
		}
		_, envRedactions, err := buildStdioEnv([]string{"EXPLICIT=" + template})
		if err != nil {
			t.Fatal(err)
		}
		for _, values := range [][]string{redactions, envRedactions} {
			if strings.Contains(redactError(errors.New("component_sentinel"), values, ""), "component_sentinel") {
				t.Fatalf("error not redacted for %s", template)
			}
			if strings.Contains(sanitizeMCPResultText("component_sentinel", values, ""), "component_sentinel") {
				t.Fatalf("result not redacted for %s", template)
			}
		}
	}
}
