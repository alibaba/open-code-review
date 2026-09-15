// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPReviewAcceptance uses the actual review pipeline and HTTP transports.
// Only terminal input is injected; permission resolution and the call sink are real.
func TestMCPReviewAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name, answer   string
		calls, prompts int32
		hidden         bool
	}{
		{"allow-once", "1", 4, 4, false},
		{"allow-review", "2", 4, 2, false},
		{"deny-once", "3", 0, 4, false},
		{"deny-review", "4", 0, 2, false},
		{"escape", "\x1b", 0, 4, false},
		{"ctrl-c", "\x03", 0, 4, false},
		{"eof", "", 0, 4, false},
		{"timeout", "timeout", 0, 4, false},
		{"ci-ask", "", 0, 0, true},
		{"ci-allow", "", 4, 0, false},
		{"disabled", "", 0, 0, true},
		{"unselected", "", 0, 0, true},
		{"parent-deny", "", 0, 0, true},
		{"fingerprint-drift", "", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			for _, k := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "TF_BUILD", "BUILDKITE", "JENKINS_URL"} {
				t.Setenv(k, "")
			}
			setMCPTestInteractive(t, true)
			if strings.HasPrefix(tc.name, "ci-") {
				t.Setenv("CI", "true")
				mcpInteractiveTerminal = defaultMCPInteractiveTerminal
			}
			t.Setenv("OCR_RAW_LOGGING", "1")
			var calls, prompts, rounds, visible atomic.Int32
			mcpServer := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "acceptance", Version: "1"}, nil)
			mcpServer.AddTool(&sdkmcp.Tool{Name: "probe", Description: "Read a fixed fixture", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"token": map[string]any{"type": "string"}}}}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				calls.Add(1)
				return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "fixture-result"}}}, nil
			})
			endpoint := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return mcpServer }, nil))
			defer endpoint.Close()
			server := MCPServerConfig{Type: "remote", URL: endpoint.URL, Tools: []string{"probe"}}
			client, err := ocrmcp.NewConfiguredClient(context.Background(), "fixture", server, "", "test")
			if err != nil {
				t.Fatal(err)
			}
			server.ToolDefinitionSHA256 = map[string]string{"probe": client.DiscoveredTools()[0].DefinitionSHA256}
			_ = client.Close()
			if calls.Load() != 0 {
				t.Fatal("discovery executed a business tool")
			}
			cfg := &Config{MCP: &ocrmcp.MCPConfig{Version: 1, ApprovalTimeoutSeconds: 1}, MCPServers: map[string]MCPServerConfig{}}
			switch tc.name {
			case "ci-allow":
				server.DefaultPermission = ocrmcp.PermissionAllow
			case "disabled":
				server.Enabled = boolPointer(false)
			case "unselected":
				server.Tools = nil
				server.ToolDefinitionSHA256 = nil
			case "parent-deny":
				cfg.MCP.DefaultPermission = ocrmcp.PermissionDeny
				server.DefaultPermission = ocrmcp.PermissionAllow
			case "fingerprint-drift":
				server.ToolDefinitionSHA256["probe"] = strings.Repeat("a", 64)
			}
			cfg.MCPServers["fixture"] = server
			configPath := filepath.Join(home, "config.json")
			t.Setenv("OCR_CONFIG_PATH", configPath)
			if err := saveConfig(configPath, cfg); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			previous := runMCPApprovalPrompt
			t.Cleanup(func() { runMCPApprovalPrompt = previous })
			runMCPApprovalPrompt = func(ctx context.Context, inv ocrmcp.Invocation, _ io.Reader, _ io.Writer) (ocrmcp.Decision, error) {
				prompts.Add(1)
				if tc.answer == "timeout" {
					<-ctx.Done()
					return ocrmcp.DecisionDenyOnce, ctx.Err()
				}
				if tc.answer == "" {
					return ocrmcp.DecisionDenyOnce, io.EOF
				}
				return runMCPApprovalBubbleTea(ctx, inv, strings.NewReader(tc.answer), io.Discard)
			}
			alias := ocrmcp.ModelAlias(ocrmcp.ToolID{Server: "fixture", Name: "probe"})
			llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Tools []struct {
						Name string `json:"name"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					http.Error(w, "bad JSON", 400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				content := []map[string]any{{"type": "text", "text": "Review this small fixture."}}
				stop := "end_turn"
				if len(req.Tools) > 0 {
					for _, tool := range req.Tools {
						if tool.Name == alias {
							visible.Add(1)
						}
						if tool.Name == "probe" {
							t.Error("raw MCP tool name exposed")
						}
					}
					n := rounds.Add(1)
					name := alias
					input := map[string]any{"token": "acceptance-secret-sentinel"}
					if n > 2 {
						name = "task_done"
						input = map[string]any{"state": "DONE"}
					}
					content = []map[string]any{{"type": "tool_use", "id": fmt.Sprintf("call_%d", n), "name": name, "input": input}}
					stop = "tool_use"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "acceptance", "type": "message", "role": "assistant", "model": "test", "content": content, "stop_reason": stop, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5}})
			}))
			defer llmServer.Close()
			t.Setenv("OCR_LLM_URL", llmServer.URL+"/v1/messages")
			t.Setenv("OCR_LLM_TOKEN", "test")
			t.Setenv("OCR_LLM_MODEL", "test")
			t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
			t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
			repo := t.TempDir()
			retryTestGit(t, repo, "init", "-q", "-b", "main")
			file := filepath.Join(repo, "main.go")
			if err := os.WriteFile(file, []byte("package main\nfunc value() int { return 1 }\n"), 0600); err != nil {
				t.Fatal(err)
			}
			retryTestGit(t, repo, "add", ".")
			retryTestGit(t, repo, "commit", "-q", "-m", "base")
			if err := os.WriteFile(file, []byte("package main\nfunc value() int { return 2 }\n"), 0600); err != nil {
				t.Fatal(err)
			}
			retryTestGit(t, repo, "add", ".")
			retryTestGit(t, repo, "commit", "-q", "-m", "change")
			for i := 0; i < 2; i++ {
				rounds.Store(0)
				var reviewErr error
				var out string
				stderr := captureStderr(t, func() {
					out = captureStdout(t, func() {
						reviewErr = runReview([]string{"--repo", repo, "--from", "HEAD~1", "--to", "HEAD", "--format", "json", "--no-filter", "--concurrency", "1"})
					})
				})
				if reviewErr != nil {
					t.Fatalf("review failed: %v\n%s", reviewErr, stderr)
				}
				if strings.Contains(out+stderr, "acceptance-secret-sentinel") {
					t.Fatal("secret in review output")
				}
				var doc map[string]any
				if err := json.Unmarshal([]byte(out), &doc); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
			}
			if calls.Load() != tc.calls || prompts.Load() != tc.prompts {
				t.Fatalf("calls=%d prompts=%d; want %d %d", calls.Load(), prompts.Load(), tc.calls, tc.prompts)
			}
			if (visible.Load() == 0) != tc.hidden {
				t.Fatalf("visibility=%d hidden=%v", visible.Load(), tc.hidden)
			}
			after, _ := os.ReadFile(configPath)
			if !bytes.Equal(before, after) {
				t.Fatal("runtime approval changed config")
			}
			if !tc.hidden {
				if err := filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
						b, e := os.ReadFile(path)
						if e != nil {
							return e
						}
						if bytes.Contains(b, []byte("acceptance-secret-sentinel")) {
							t.Errorf("secret in session file %s", filepath.Base(path))
						}
						if strings.Contains(filepath.Base(path), "raw") {
							t.Error("raw capture enabled for MCP")
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
