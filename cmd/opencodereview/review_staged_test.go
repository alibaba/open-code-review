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
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func stagedWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func stagedGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGitCmdStdout(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func stagedPreview(t *testing.T, dir string, extra ...string) model.Preview {
	t.Helper()
	args := append([]string{"--repo", dir, "--staged", "--preview", "--format", "json"}, extra...)
	out := captureStdout(t, func() {
		if err := runReview(args); err != nil {
			t.Errorf("staged preview: %v", err)
		}
	})
	return decodeSinglePreviewJSON(t, out)
}

func TestStagedFlagsRejectOtherModesBeforeArtifacts(t *testing.T) {
	for _, flag := range []string{"--from", "--to", "--commit", "--resume"} {
		t.Run(flag, func(t *testing.T) {
			home := t.TempDir()
			setTestHome(t, home)
			if _, err := parseReviewFlags([]string{"--staged", flag, "HEAD"}); err == nil || !strings.Contains(err.Error(), "--staged") {
				t.Fatalf("expected staged conflict, got %v", err)
			}
			opts := reviewOptions{staged: true, repoDir: filepath.Join(home, "missing")}
			switch flag {
			case "--from":
				opts.from = "HEAD"
			case "--to":
				opts.to = "HEAD"
			case "--commit":
				opts.commit = "HEAD"
			case "--resume":
				opts.resume = "HEAD"
			}
			if err := executeReviewContext(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "--staged") {
				t.Fatalf("direct execution must reject before resolving repository: %v", err)
			}
			if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
				t.Fatalf("flag rejection persisted artifacts: %v", err)
			}
		})
	}
	opts, err := parseReviewFlags([]string{"--staged", "--preview"})
	if err != nil || !opts.staged || reviewModeFromOptions(opts) != session.ReviewModeStaged {
		t.Fatalf("staged preview flags: %+v, %v", opts, err)
	}
	if _, err := parseScanFlags([]string{"--staged"}); err == nil {
		t.Fatal("scan must not advertise unsupported staged mode")
	}
	if delegateCmd.PersistentFlags().Lookup("staged") != nil || delegateCmd.Flags().Lookup("staged") != nil {
		t.Fatal("delegate must not advertise unsupported staged mode")
	}
}

func TestStagedPreviewRequiresFrozenSnapshot(t *testing.T) {
	var out bytes.Buffer
	err := runPreviewContext(context.Background(), nil, reviewOptions{staged: true}, &out, nil)
	if err == nil || !strings.Contains(err.Error(), "requires a frozen snapshot") {
		t.Fatalf("missing staged snapshot must fail before loading context: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("missing snapshot emitted a preview: %s", out.String())
	}
}

func TestStagedPreviewUsesIndexAndSnapshotRules(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	stagedWrite(t, dir, "target.go", "package p\n// STAGED_TARGET\n")
	stagedWrite(t, dir, "excluded.go", "package p\n")
	stagedWrite(t, dir, ".opencodereview/rule.json", `{"exclude":["excluded.go"]}`)
	retryTestGit(t, dir, "add", ".")
	tree := stagedGitOutput(t, dir, "write-tree")
	base := stagedGitOutput(t, dir, "rev-parse", "HEAD")
	stagedWrite(t, dir, "target.go", "package p\n// LIVE_TARGET\n// extra line\n")
	stagedWrite(t, dir, "untracked.go", "package p\n")
	stagedWrite(t, dir, ".opencodereview/rule.json", `{"exclude":["target.go"]}`)
	// A subdirectory invocation still reviews the root-relative index.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := stagedPreview(t, filepath.Join(dir, "subdir"))
	if p.Input == nil || p.Input.Mode != "staged" || p.Input.SnapshotTree != tree || p.Input.ResolvedBase != base {
		t.Fatalf("preview input = %+v, want base=%s tree=%s", p.Input, base, tree)
	}
	seen := map[string]model.PreviewEntry{}
	for _, entry := range p.Entries {
		seen[entry.Path] = entry
	}
	if target := seen["target.go"]; !target.WillReview || target.Insertions != 2 {
		t.Fatalf("target must use staged content and rules: %+v", target)
	}
	if excluded := seen["excluded.go"]; excluded.WillReview || excluded.ExcludeReason != model.ExcludeUserRule {
		t.Fatalf("staged exclusion lost: %+v", excluded)
	}
	if _, found := seen["untracked.go"]; found {
		t.Fatal("untracked file leaked into staged preview")
	}
	var text bytes.Buffer
	outputPreviewText(&p, &text)
	for _, identity := range []string{"Review mode: staged", base, tree} {
		if !strings.Contains(text.String(), identity) {
			t.Errorf("text preview missing %q: %s", identity, text.String())
		}
	}
	sessions, err := session.ListSessions(dir)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("preview must create no sessions: %+v, %v", sessions, err)
	}
}

func TestStagedPreviewEmptyUnbornAndUntrackedRules(t *testing.T) {
	for _, unborn := range []bool{false, true} {
		t.Run(fmt.Sprintf("unborn=%t", unborn), func(t *testing.T) {
			freshOCRHome(t)
			dir := t.TempDir()
			retryTestGit(t, dir, "init", "-q")
			if !unborn {
				retryTestGit(t, dir, "commit", "--allow-empty", "-qm", "base")
			}
			stagedWrite(t, dir, "untracked.go", "package p\n")
			p := stagedPreview(t, dir)
			if p.TotalFiles != 0 || p.Input == nil || p.Input.SnapshotTree == "" {
				t.Fatalf("empty index preview: %+v", p)
			}
			if (p.Input.ResolvedBase == "") != unborn {
				t.Fatalf("unexpected base identity: %+v", p.Input)
			}
			stagedWrite(t, dir, "target.go", "package p\n")
			retryTestGit(t, dir, "add", "target.go")
			// A default rule file absent from the index must not fall back to disk.
			stagedWrite(t, dir, ".opencodereview/rule.json", `{"exclude":["target.go"]}`)
			p = stagedPreview(t, dir)
			if p.TotalFiles != 1 || p.ReviewableCount != 1 || p.Entries[0].Path != "target.go" {
				t.Fatalf("unexpected staged preview: %+v", p)
			}
			// Explicit --rule intentionally remains a local override.
			p = stagedPreview(t, dir, "--rule", filepath.Join(dir, ".opencodereview/rule.json"))
			if p.ReviewableCount != 0 || p.Entries[0].ExcludeReason != model.ExcludeUserRule {
				t.Fatalf("explicit local rule was ignored: %+v", p)
			}
		})
	}
}

func TestStagedReviewFreezesToolsAndPersistsTree(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	stagedWrite(t, dir, "target.go", "package p\n// BASE_TARGET\n")
	stagedWrite(t, dir, "context.go", "package p\n// SNAPSHOT_CONTEXT\n")
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-qm", "base files")
	base := stagedGitOutput(t, dir, "rev-parse", "HEAD")
	stagedWrite(t, dir, "target.go", "package p\n// STAGED_TARGET\n")
	retryTestGit(t, dir, "add", "target.go")
	tree := stagedGitOutput(t, dir, "write-tree")
	stagedWrite(t, dir, "context.go", "package p\n// LIVE_CONTEXT\n")
	stagedWrite(t, dir, "target.go", "package p\n// LIVE_TARGET\n")
	stagedWrite(t, dir, "live_only.go", "package p\n// LIVE_ONLY\n")

	var mutate sync.Once
	var mu sync.Mutex
	var requestBodies []string
	var mutationErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "read request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requestBodies = append(requestBodies, string(body))
		mu.Unlock()
		// Restage after admission but before any tool reads. A reader wired to
		// either the live index or the working directory will now see LIVE_*.
		mutate.Do(func() {
			_, mutationErr = runGitCmd(dir, "add", ".")
		})
		w.Header().Set("Content-Type", "application/json")
		content := `[ {"type":"text","text":"Read the snapshot tools before concluding."} ]`
		stop := "end_turn"
		if bytes.Contains(body, []byte(`"tools"`)) {
			stop = "tool_use"
			if bytes.Contains(body, []byte(`"tool_result"`)) {
				content = `[{"type":"tool_use","id":"done","name":"task_done","input":{"state":"DONE"}}]`
			} else {
				content = `[
					{"type":"tool_use","id":"read","name":"file_read","input":{"file_path":"context.go"}},
					{"type":"tool_use","id":"find","name":"file_find","input":{"query_name":".go"}},
					{"type":"tool_use","id":"search","name":"code_search","input":{"search_text":"CONTEXT"}},
					{"type":"tool_use","id":"diff","name":"file_read_diff","input":{"path_array":["target.go"]}}
				]`
			}
		}
		fmt.Fprintf(w, `{"id":"staged","type":"message","role":"assistant","model":"claude-test","content":%s,"stop_reason":%q,"usage":{"input_tokens":10,"output_tokens":5}}`, content, stop)
	}))
	t.Cleanup(server.Close)
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-test")
	t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
	t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
	t.Setenv("OCR_LLM_TIMEOUT", "30")
	var runErr error
	out := captureStdout(t, func() {
		runErr = runReview([]string{"--repo", dir, "--staged", "--format", "json", "--no-filter", "--concurrency", "1"})
	})
	if runErr != nil {
		t.Fatalf("staged review: %v\n%s", runErr, out)
	}
	if mutationErr != nil {
		t.Fatalf("restage during review: %v", mutationErr)
	}
	var got jsonOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if got.Manifest == nil || got.Manifest.SchemaVersion != session.StagedManifestSchemaVersion ||
		got.Manifest.Input.Mode != "staged" || got.Manifest.Input.SnapshotTree != tree ||
		got.Manifest.Input.ResolvedBase != base || got.Manifest.Input.ResolvedHead != "" ||
		got.Manifest.TerminalState != session.StateComplete || len(got.Manifest.Coverage.Completed) != 1 {
		t.Fatalf("staged manifest: %+v", got.Manifest)
	}
	mu.Lock()
	joined := strings.Join(requestBodies, "\n")
	bodies := append([]string(nil), requestBodies...)
	mu.Unlock()
	if !strings.Contains(joined, `"tool_result"`) || !strings.Contains(joined, "SNAPSHOT_CONTEXT") || !strings.Contains(joined, "STAGED_TARGET") {
		t.Fatalf("snapshot tools did not reach the model:\n%s", joined)
	}
	for _, forbidden := range []string{"LIVE_CONTEXT", "LIVE_TARGET", "live_only.go"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("live content %q leaked into model requests", forbidden)
		}
	}
	// Check each actual tool response, not just the initial diff prompt, so a
	// missing or failed tool cannot make the snapshot assertion pass by accident.
	results := make(map[string]string)
	for _, body := range bodies {
		var request struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			t.Fatal(err)
		}
		for _, message := range request.Messages {
			var blocks []struct {
				Type      string          `json:"type"`
				ToolUseID string          `json:"tool_use_id"`
				Content   json.RawMessage `json:"content"`
			}
			if json.Unmarshal(message.Content, &blocks) != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type == "tool_result" {
					results[block.ToolUseID] = string(block.Content)
				}
			}
		}
	}
	for id, want := range map[string]string{"read": "SNAPSHOT_CONTEXT", "find": "context.go", "search": "SNAPSHOT_CONTEXT", "diff": "STAGED_TARGET"} {
		if !strings.Contains(results[id], want) {
			t.Errorf("tool result %q = %q, want %q", id, results[id], want)
		}
	}
	if got.ToolCalls == nil || got.ToolCalls.Failure != 0 {
		t.Fatalf("snapshot tool failures: %+v", got.ToolCalls)
	}
	for _, name := range []string{"file_read", "file_find", "code_search", "file_read_diff"} {
		if got.ToolCalls.ByTool[name] != 1 {
			t.Errorf("tool %s calls = %d, want 1", name, got.ToolCalls.ByTool[name])
		}
	}
	repoRoot, err := resolveRepoDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := session.LoadSummary(repoRoot, got.SessionID)
	if err != nil || summary.RunManifest == nil || summary.RunManifest.Input.SnapshotTree != tree || summary.ReviewMode != "staged" {
		t.Fatalf("persisted staged identity: %+v, %v", summary, err)
	}
	if current := stagedGitOutput(t, dir, "write-tree"); current == tree {
		t.Fatal("fixture failed to change the live index during review")
	}
}
