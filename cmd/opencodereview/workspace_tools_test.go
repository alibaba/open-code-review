// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestWorkspaceToolsReachModelAndManifest(test *testing.T) {
	for _, unborn := range []bool{false, true} {
		test.Run(fmt.Sprint(unborn), func(test *testing.T) {
			freshOCRHome(test)
			repo := test.TempDir()
			gitIn(test, repo, "init", "-q")
			base := ""
			prefix := "SIDE: old\nSOURCE: empty-tree\n"
			if !unborn {
				commitFile(test, repo, "version.go", "package example\nconst Value = 1\n", "base")
				base = revParse(test, repo, "HEAD")
				prefix = "SIDE: old\nREVISION: " + base + "\n"
			}
			path := filepath.Join(repo, "version.go")
			if err := os.WriteFile(path, []byte("package example\nconst Value = 2\n"), 0o644); err != nil {
				test.Fatal(err)
			}
			gitIn(test, repo, "add", "version.go")
			if err := os.WriteFile(path, []byte("package example\nconst Value = 3\n"), 0o644); err != nil {
				test.Fatal(err)
			}
			var mutex sync.Mutex
			called := false
			seen := make(map[string]string)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var body struct {
					Tools    []llm.ToolDef `json:"tools"`
					Messages []llm.Message `json:"messages"`
				}
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					test.Error(err)
					http.Error(writer, "bad request", http.StatusBadRequest)
					return
				}
				mutex.Lock()
				defer mutex.Unlock()
				for _, message := range body.Messages {
					if message.Role == "tool" {
						seen[message.ToolCallID] = message.ExtractText()
					}
				}
				for _, definition := range body.Tools {
					switch definition.Function.Name {
					case "file_read", "code_search", "file_find":
						if !strings.Contains(definition.Function.Description, "live working tree") || definition.Function.Parameters["properties"].(map[string]any)["side"] == nil {
							test.Errorf("model received wrong workspace schema: %s", definition.Function.Name)
						}
					}
				}
				message := map[string]any{"role": "assistant", "content": "Inspect the changed file."}
				finish := "stop"
				if len(body.Tools) > 0 {
					finish = "tool_calls"
					calls := []llm.ToolCall{{ID: "done", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: `{}`}}}
					if !called {
						called = true
						calls = []llm.ToolCall{
							{ID: "old-read", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{"file_path":"version.go","side":"old"}`}},
							{ID: "old-search", Type: "function", Function: llm.FunctionCall{Name: "code_search", Arguments: `{"search_text":"Value","side":"old"}`}},
							{ID: "old-find", Type: "function", Function: llm.FunctionCall{Name: "file_find", Arguments: `{"query_name":"version.go","side":"old"}`}},
							{ID: "new-read", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{"file_path":"version.go"}`}},
						}
					}
					message["tool_calls"] = calls
				}
				writer.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(writer).Encode(map[string]any{
					"id": "workspace", "object": "chat.completion", "model": "test-model",
					"choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}},
					"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
				}); err != nil {
					test.Error(err)
				}
			}))
			defer server.Close()
			test.Setenv("OCR_LLM_URL", server.URL+"/v1/chat/completions")
			test.Setenv("OCR_LLM_TOKEN", "test-token")
			test.Setenv("OCR_LLM_MODEL", "test-model")
			test.Setenv("OCR_LLM_PROTOCOL", "openai")
			test.Setenv("OCR_USE_ANTHROPIC", "false")
			output := filepath.Join(test.TempDir(), "result.json")
			opts, err := parseReviewFlags([]string{"--repo", repo, "--format", "json", "--audience", "agent", "--output", output})
			if err != nil {
				test.Fatal(err)
			}
			if err := executeReview(opts); err != nil {
				test.Fatal(err)
			}
			mutex.Lock()
			for _, name := range []string{"old-read", "old-search", "old-find"} {
				if !strings.Contains(seen[name], prefix) {
					test.Errorf("%s lost the old source: %q", name, seen[name])
				}
			}
			if !strings.Contains(seen["new-read"], "Value = 3") {
				test.Errorf("new tool read staged content: %q", seen["new-read"])
			}
			if !unborn && (!strings.Contains(seen["old-read"], "Value = 1") || !strings.Contains(seen["old-search"], "Value = 1") || !strings.Contains(seen["old-find"], "version.go")) {
				test.Errorf("old tools did not use HEAD: %v", seen)
			}
			mutex.Unlock()
			raw, err := os.ReadFile(output)
			if err != nil {
				test.Fatal(err)
			}
			var result jsonOutput
			if err := json.Unmarshal(raw, &result); err != nil {
				test.Fatal(err)
			}
			if result.Manifest == nil || result.Manifest.Input.Mode != "workspace" || result.Manifest.Input.ResolvedBase != base || result.Manifest.Input.ResolvedHead != "" || result.Manifest.Input.ExactRange != "" || len(result.Manifest.Coverage.Completed) != 1 {
				test.Fatalf("wrong workspace manifest: %+v", result.Manifest)
			}
			resolvedRepo, err := resolveRepoDir(repo)
			if err != nil {
				test.Fatal(err)
			}
			sessionPath, err := session.SessionFilePath(resolvedRepo, result.SessionID)
			if err != nil {
				test.Fatal(err)
			}
			persisted, err := os.ReadFile(sessionPath)
			if err != nil || !strings.Contains(string(persisted), `"mode":"workspace"`) || (!unborn && !strings.Contains(string(persisted), `"resolved_base":"`+base+`"`)) {
				test.Fatalf("workspace baseline not persisted: %v", err)
			}
		})
	}
}
