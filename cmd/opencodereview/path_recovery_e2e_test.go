// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

const recoveredComment = "The changed code ignores the error returned by run."

type pathRecoveryScenario struct {
	name                string
	missingPath         string
	recoveryReadPath    string
	baseFiles           map[string]string
	headFiles           map[string]*string
	prepareBeforeReview bool
	wantRecovery        []string
}

type pathRecoveryFakeLLM struct {
	mu                sync.Mutex
	scenario          pathRecoveryScenario
	toolRounds        int
	sawRecovery       bool
	sawSuccessfulRead bool
	submittedComment  bool
	completed         bool
}

type pathRecoverySnapshot struct {
	toolRounds        int
	sawRecovery       bool
	sawSuccessfulRead bool
	submittedComment  bool
	completed         bool
}

func (f *pathRecoveryFakeLLM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body bytes.Buffer
	_, _ = body.ReadFrom(r.Body)
	raw := body.String()
	hasTools := strings.Contains(raw, `"tools"`)

	w.Header().Set("Content-Type", "application/json")
	if !hasTools {
		writeAnthropicText(w, "review the changed file")
		return
	}

	f.mu.Lock()
	f.toolRounds++
	round := f.toolRounds
	f.mu.Unlock()
	switch {
	case strings.Contains(raw, tool.CommentSucceed):
		f.mu.Lock()
		f.completed = true
		f.mu.Unlock()
		writeAnthropicTool(w, round, "task_done", `{"state":"DONE"}`)
	case containsFragment(raw, "File: "+f.scenario.recoveryReadPath+" (Total lines:"):
		f.mu.Lock()
		f.sawSuccessfulRead = true
		f.submittedComment = true
		f.mu.Unlock()
		writeAnthropicTool(w, round, "code_comment",
			`{"comments":[{"content":"`+recoveredComment+`","existing_code":"func main() { run() }","category":"bug","severity":"high"}]}`)
	case containsAll(raw, append([]string{"OCR path recovery"}, f.scenario.wantRecovery...)...):
		f.mu.Lock()
		f.sawRecovery = true
		f.mu.Unlock()
		writeAnthropicTool(w, round, "file_read",
			fmt.Sprintf(`{"file_path":%q}`, f.scenario.recoveryReadPath))
	default:
		writeAnthropicTool(w, round, "file_read",
			fmt.Sprintf(`{"file_path":%q}`, f.scenario.missingPath))
	}
}

func (f *pathRecoveryFakeLLM) snapshot() pathRecoverySnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return pathRecoverySnapshot{
		toolRounds:        f.toolRounds,
		sawRecovery:       f.sawRecovery,
		sawSuccessfulRead: f.sawSuccessfulRead,
		submittedComment:  f.submittedComment,
		completed:         f.completed,
	}
}

func containsAll(text string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !containsFragment(text, fragment) {
			return false
		}
	}
	return true
}

func containsFragment(text, fragment string) bool {
	if strings.Contains(text, fragment) {
		return true
	}
	quoted := strconv.QuoteToASCII(fragment)
	return len(quoted) >= 2 && strings.Contains(text, quoted[1:len(quoted)-1])
}

func writeAnthropicText(w http.ResponseWriter, content string) {
	_, _ = fmt.Fprintf(w, `{"id":"plan","type":"message","role":"assistant","model":"claude-test",
		"content":[{"type":"text","text":%q}],
		"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`, content)
}

func writeAnthropicTool(w http.ResponseWriter, round int, name, input string) {
	_, _ = fmt.Fprintf(w, `{"id":"round_%d","type":"message","role":"assistant","model":"claude-test",
		"content":[{"type":"tool_use","id":"tool_%d","name":%q,"input":%s}],
		"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`,
		round, round, name, input)
}

func pathRecoveryTestRepo(t *testing.T, scenario pathRecoveryScenario) string {
	t.Helper()
	dir := t.TempDir()
	retryTestGit(t, dir, "init", "-q", "-b", "main")
	writeRecoveryFixture(t, dir, "main.go", "package main\n\nfunc run() error { return nil }\n\nfunc main() { _ = run() }\n")
	for path, content := range scenario.baseFiles {
		writeRecoveryFixture(t, dir, path, content)
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")

	applyHeadFiles := func() {
		for path, content := range scenario.headFiles {
			fullPath := filepath.Join(dir, filepath.FromSlash(path))
			if content == nil {
				if err := os.Remove(fullPath); err != nil {
					t.Fatalf("remove %s: %v", path, err)
				}
				continue
			}
			writeRecoveryFixture(t, dir, path, *content)
		}
	}
	if scenario.prepareBeforeReview {
		applyHeadFiles()
		retryTestGit(t, dir, "add", "-A")
		retryTestGit(t, dir, "commit", "-q", "-m", "prepare current tree")
	}

	writeRecoveryFixture(t, dir, "main.go", "package main\n\nfunc run() error { return nil }\n\nfunc main() { run() }\n")
	if !scenario.prepareBeforeReview {
		applyHeadFiles()
	}
	retryTestGit(t, dir, "add", "-A")
	retryTestGit(t, dir, "commit", "-q", "-m", "change")
	return dir
}

func writeRecoveryFixture(t *testing.T, dir, path, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func textPointer(value string) *string { return &value }

func TestReviewReturnsToMainlineAfterMissingFileReads(t *testing.T) {
	unicodeDir := "DOC/\u4e91\u914d\u7f6e\u9879"
	scenarios := []pathRecoveryScenario{
		{
			name:             "invented unicode config path",
			missingPath:      unicodeDir + "/project.yct",
			recoveryReadPath: unicodeDir + "/project.yaml",
			baseFiles: map[string]string{
				unicodeDir + "/project.yaml": "name: demo\n",
			},
			wantRecovery: []string{unicodeDir + "/project.yaml"},
		},
		{
			name:             "file deleted in reviewed commit",
			missingPath:      "App/MID/RF/CendricApp/src/phscaCendricCadsApp.c",
			recoveryReadPath: "main.go",
			baseFiles: map[string]string{
				"App/MID/RF/CendricApp/src/phscaCendricCadsApp.c": "int legacy(void) { return 0; }\n",
			},
			headFiles: map[string]*string{
				"App/MID/RF/CendricApp/src/phscaCendricCadsApp.c": nil,
			},
			wantRecovery: []string{"No candidate path was found in the reviewed ref"},
		},
		{
			name:             "file moved to a new directory",
			missingPath:      "old/services/auth/config.go",
			recoveryReadPath: "new/services/auth/config.go",
			baseFiles: map[string]string{
				"old/services/auth/config.go": "package auth\n\nconst Enabled = true\n",
			},
			headFiles: map[string]*string{
				"old/services/auth/config.go": nil,
				"new/services/auth/config.go": textPointer("package auth\n\nconst Enabled = true\n"),
			},
			prepareBeforeReview: true,
			wantRecovery:        []string{"new/services/auth/config.go"},
		},
		{
			name:             "multiple same-stem candidates",
			missingPath:      "DOC/cloud/project.yct",
			recoveryReadPath: "configs/service-b/project.json",
			baseFiles: map[string]string{
				"configs/service-a/project.yaml": "name: service-a\n",
				"configs/service-b/project.json": "{\"name\":\"service-b\"}\n",
			},
			wantRecovery: []string{
				"configs/service-a/project.yaml",
				"configs/service-b/project.json",
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			runPathRecoveryScenario(t, scenario)
		})
	}
}

func runPathRecoveryScenario(t *testing.T, scenario pathRecoveryScenario) {
	t.Helper()
	repoDir := pathRecoveryTestRepo(t, scenario)
	fake := &pathRecoveryFakeLLM{scenario: scenario}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-test")
	t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
	t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
	t.Setenv("OCR_LLM_TIMEOUT", "30")

	cc, err := loadCommonContext(repoDir, "", 0, 1, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	cc.Template.MaxToolRequestTimes = 3
	cc.Template.ReviewFilterTask = nil
	rt, err := loadLLMRuntime(cc.Template, "", llm.ResolveOptions{})
	if err != nil {
		t.Fatalf("loadLLMRuntime: %v", err)
	}
	fr := &tool.FileReader{
		RepoDir: cc.RepoDir,
		Mode:    tool.ModeRange,
		Ref:     "HEAD",
		Runner:  cc.GitRunner,
	}
	ag := agent.New(agent.Args{
		RepoDir:               cc.RepoDir,
		From:                  "HEAD~1",
		To:                    "HEAD",
		ReviewMode:            session.ReviewModeRange,
		Template:              *cc.Template,
		SystemRule:            cc.Resolver,
		FileFilter:            cc.FileFilter,
		LLMClient:             rt.Client,
		Tools:                 buildToolRegistry(rt.Collector, fr),
		PlanToolDefs:          rt.PlanToolDefs,
		MainToolDefs:          rt.MainToolDefs,
		CommentCollector:      rt.Collector,
		CommentWorkerPool:     agent.NewCommentWorkerPool(1),
		MaxConcurrency:        1,
		ConcurrentTaskTimeout: 120,
		Model:                 rt.Model,
		Provider:              rt.Provider,
		GitRunner:             cc.GitRunner,
		RuntimeConfig:         rt.RuntimeConfig,
	})

	comments, err := ag.Run(context.Background())
	if err != nil {
		t.Fatalf("agent run: %v", err)
	}
	if len(comments) != 1 || comments[0].Content != recoveredComment || comments[0].Path != "main.go" {
		t.Fatalf("comments = %+v, want recovered finding on main.go", comments)
	}
	manifest := ag.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateComplete {
		t.Fatalf("manifest = %+v, want terminal state complete", manifest)
	}
	if len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v, want one completed and no failed items", manifest.Coverage)
	}
	snapshot := fake.snapshot()
	if snapshot.toolRounds != 6 {
		t.Fatalf("tool rounds = %d, want 3 rejected reads, recovery read, comment, and task_done", snapshot.toolRounds)
	}
	if !snapshot.sawRecovery || !snapshot.sawSuccessfulRead || !snapshot.submittedComment || !snapshot.completed {
		t.Fatalf("agent did not return to the review mainline: %+v", snapshot)
	}
}
