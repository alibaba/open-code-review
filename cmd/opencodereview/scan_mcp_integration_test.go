// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const scanMCPServerEnv = "_OCR_SCAN_MCP_SERVER"

func scanTestMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "scan-test", Version: "v1"}, nil)
	server.AddTool(&mcp.Tool{
		Name: "lookup_context", Description: "Return test context",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "scan-mcp-context"}}}, nil
	})
	for _, name := range []string{"not_allowed", "file_read"} {
		server.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: name}}}, nil
		})
	}
	return server
}

func TestScanMCPServerProcess(t *testing.T) {
	if os.Getenv(scanMCPServerEnv) == "" {
		return
	}
	if err := scanTestMCPServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

type scanMCPFakeLLM struct {
	mu               sync.Mutex
	sawTool          bool
	sawResult        bool
	sawDiffTool      bool
	sawForbidden     bool
	sawToolFreePlan  bool
	sawBuiltIn       bool
	duplicateBuiltIn bool
	requestCount     int
}

func (f *scanMCPFakeLLM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requestCount++
	if len(request.Tools) == 0 {
		f.sawToolFreePlan = true
	}
	builtIns := 0
	for _, tool := range request.Tools {
		if tool.Name == "lookup_context" {
			f.sawTool = true
		}
		if tool.Name == "file_read_diff" {
			f.sawDiffTool = true
		}
		if tool.Name == "not_allowed" {
			f.sawForbidden = true
		}
		if tool.Name == "file_read" {
			builtIns++
		}
	}
	f.sawBuiltIn = f.sawBuiltIn || builtIns == 1
	f.duplicateBuiltIn = f.duplicateBuiltIn || builtIns > 1
	f.sawResult = f.sawResult || strings.Contains(string(request.Messages), "scan-mcp-context")
	sawResult := f.sawResult
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if len(request.Tools) == 0 {
		_, _ = fmt.Fprint(w, `{"id":"scan_plan","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"Inspect the file and then finish the scan."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
		return
	}
	if sawResult {
		_, _ = fmt.Fprint(w, `{"id":"scan_done","type":"message","role":"assistant","model":"claude-test","content":[{"type":"tool_use","id":"done_1","name":"task_done","input":{"state":"DONE"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`)
		return
	}
	_, _ = fmt.Fprint(w, `{"id":"scan_lookup","type":"message","role":"assistant","model":"claude-test","content":[{"type":"tool_use","id":"lookup_1","name":"lookup_context","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`)
}

func TestScanUsesConfiguredMCPTool(t *testing.T) {
	for _, transport := range []string{"stdio", "remote"} {
		t.Run(transport, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

			llmStub := &scanMCPFakeLLM{}
			llmServer := httptest.NewServer(llmStub)
			defer llmServer.Close()
			t.Setenv("OCR_LLM_URL", llmServer.URL+"/v1/messages")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "claude-test")
			t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
			t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")

			serverCfg := MCPServerConfig{Type: transport, Tools: []string{"lookup_context", "file_read"}}
			if transport == "remote" {
				remote := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
					return scanTestMCPServer()
				}, nil))
				defer remote.Close()
				serverCfg.URL = remote.URL
			} else {
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				serverCfg.Command = exe
				serverCfg.Args = []string{"-test.run=^TestScanMCPServerProcess$"}
				serverCfg.Env = []string{scanMCPServerEnv + "=1"}
			}
			cfg := &Config{MCPServers: map[string]MCPServerConfig{
				"test":   serverCfg,
				"broken": {Command: "ocr-nonexistent-mcp-server-binary"},
			}}
			cfgPath, err := defaultConfigPath()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
				t.Fatal(err)
			}

			repo := initTestGitRepo(t)
			gitCommitFile(t, repo, "sample.go", "package sample\n", "add sample")
			output := filepath.Join(t.TempDir(), "scan.json")
			originalClose := closeReviewMCPClients
			closedClients := -1
			closeReviewMCPClients = func(clients []*ocrmcp.Client) {
				closedClients = len(clients)
				originalClose(clients)
			}
			t.Cleanup(func() { closeReviewMCPClients = originalClose })
			if err := executeScan(scanOptions{repoDir: repo, paths: "sample.go", outputFormat: "json", outputPath: output, noDedup: true, noSummary: true}); err != nil {
				t.Fatalf("executeScan: %v", err)
			}
			if closedClients != 1 {
				t.Errorf("closed clients = %d, want 1", closedClients)
			}
			llmStub.mu.Lock()
			defer llmStub.mu.Unlock()
			if !llmStub.sawTool || !llmStub.sawResult || !llmStub.sawToolFreePlan || !llmStub.sawBuiltIn || llmStub.duplicateBuiltIn || llmStub.sawDiffTool || llmStub.sawForbidden {
				t.Errorf("tool=%v result=%v toolFreePlan=%v builtIn=%v duplicateBuiltIn=%v diffTool=%v forbidden=%v requests=%d", llmStub.sawTool, llmStub.sawResult, llmStub.sawToolFreePlan, llmStub.sawBuiltIn, llmStub.duplicateBuiltIn, llmStub.sawDiffTool, llmStub.sawForbidden, llmStub.requestCount)
			}
		})
	}
}

func TestScanPreviewDoesNotStartMCP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	marker := filepath.Join(t.TempDir(), "started")
	cfg := &Config{MCPServers: map[string]MCPServerConfig{
		"test": {Command: "ocr-nonexistent-mcp-server-binary", Setup: "touch " + marker},
	}}
	cfgPath, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	repo := initTestGitRepo(t)
	gitCommitFile(t, repo, "sample.go", "package sample\n", "add sample")
	output := filepath.Join(t.TempDir(), "preview.json")
	if err := executeScan(scanOptions{repoDir: repo, paths: "sample.go", outputFormat: "json", outputPath: output, preview: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("MCP setup ran during preview: %v", err)
	}
}

func TestScanClosesMCPOnCancellation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	remote := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return scanTestMCPServer()
	}, nil))
	defer remote.Close()
	cfgPath, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{MCPServers: map[string]MCPServerConfig{"test": {Type: "remote", URL: remote.URL}}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer llmServer.Close()
	t.Setenv("OCR_LLM_URL", llmServer.URL+"/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-test")
	t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
	t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
	repo := initTestGitRepo(t)
	gitCommitFile(t, repo, "sample.go", "package sample\n", "add sample")
	originalClose := closeReviewMCPClients
	closedClients := -1
	closeReviewMCPClients = func(clients []*ocrmcp.Client) {
		closedClients = len(clients)
		originalClose(clients)
	}
	t.Cleanup(func() { closeReviewMCPClients = originalClose })
	output := filepath.Join(t.TempDir(), "scan.json")
	err = executeScanContext(ctx, scanOptions{repoDir: repo, paths: "sample.go", outputFormat: "json", outputPath: output, noPlan: true, noDedup: true, noSummary: true})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if closedClients != 1 {
		t.Errorf("closed clients = %d, want 1", closedClients)
	}
}
