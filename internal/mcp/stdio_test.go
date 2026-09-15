// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	runAsServerEnv = "_OCR_MCP_TEST_SERVER"
	markerPathEnv  = "_OCR_MCP_TEST_MARKER"
	ambientTestEnv = "_OCR_MCP_TEST_AMBIENT_SECRET"
)

func TestMain(m *testing.M) {
	if os.Getenv(runAsServerEnv) == "1" {
		if err := runTestMCPServer(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	os.Exit(m.Run())
}

func runTestMCPServer() error {
	if marker := os.Getenv("_OCR_MCP_TEST_STARTED"); marker != "" {
		if err := os.WriteFile(marker, []byte("started"), 0o600); err != nil {
			return err
		}
	}
	description := "ambient-clean"
	if os.Getenv(ambientTestEnv) != "" {
		description = "ambient-leaked"
	}
	reflection := os.Getenv("_OCR_MCP_TEST_REFLECTION")
	toolName := "echo"
	if reflection != "" {
		// Prove credential-bearing process arguments reach the child unchanged,
		// while only their presentation and downstream reflection are redacted.
		if !strings.Contains(strings.Join(os.Args[1:], "\x00"), commandCredentialSentinel) {
			return fmt.Errorf("test process arguments were modified")
		}
		description = commandCredentialSentinel
		if reflection == "name" {
			toolName = commandCredentialSentinel
		}
	}
	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "open-code-review-test", Version: "v0.0.1"}, nil,
	)
	server.AddTool(&sdkmcp.Tool{
		Name: toolName, Description: description,
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if reflection != "" {
			return &sdkmcp.CallToolResult{IsError: reflection == "error", Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: commandCredentialSentinel}}}, nil
		}
		var arguments struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		if marker := os.Getenv(markerPathEnv); marker != "" {
			if err := os.WriteFile(marker, []byte("called"), 0o600); err != nil {
				return nil, err
			}
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: "echo: " + arguments.Message},
		}}, nil
	})
	// The PTY smoke test reuses this SDK fixture for both transports.
	if os.Getenv("_OCR_MCP_TEST_HTTP") == "1" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		defer func() { _ = listener.Close() }()
		fmt.Println("http://" + listener.Addr().String())
		return http.Serve(listener, sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil))
	}
	return server.Run(context.Background(), &sdkmcp.StdioTransport{})
}

func TestStdioDiscoveryDoesNotInvokeToolAndAuthorizedProviderDoes(t *testing.T) {
	requireSubprocess(t)
	t.Setenv(ambientTestEnv, "must-not-be-inherited")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + string(os.PathSeparator) + "called"
	config := MCPServerConfig{
		Type: "stdio", Command: executable,
		Env: []string{runAsServerEnv + "=1", markerPathEnv + "=" + marker},
	}
	client, err := NewConfiguredClient(context.Background(), "stdio-test", config, "", "v0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	discovered := client.DiscoveredTools()
	if len(discovered) != 1 || discovered[0].Name != "echo" || discovered[0].Description != "ambient-clean" {
		t.Fatalf("discovered tools = %#v", discovered)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("business tool ran during discovery; marker stat error = %v", err)
	}

	config.Tools = []string{"echo"}
	config.ToolDefinitionSHA256 = map[string]string{"echo": discovered[0].DefinitionSHA256}
	var prompts atomic.Int32
	authorizer := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
		prompts.Add(1)
		return DecisionAllowOnce, nil
	}))
	registry := tool.NewRegistry()
	registration, err := RegisterSelected(
		registry, []*Client{client}, MCPConfig{Version: 1},
		map[string]MCPServerConfig{"stdio-test": config}, authorizer, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := registry.Get(registration.ToolDefs[0].Function.Name)
	if !ok {
		t.Fatal("authorized provider is not registered")
	}
	result, err := provider.Execute(context.Background(), map[string]any{"message": "hello"})
	if err != nil || result != "echo: hello" {
		t.Fatalf("Execute() = (%q, %v)", result, err)
	}
	if prompts.Load() != 1 {
		t.Fatalf("approval prompts = %d, want 1", prompts.Load())
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "called" {
		t.Fatalf("tool marker = %q, error=%v", data, err)
	}
}

func requireSubprocess(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		t.Skip("subprocess integration test is unsupported on this platform")
	}
}
