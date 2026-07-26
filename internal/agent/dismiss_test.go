package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// readSessionJSONL reads the JSONL for a session id from the test-dismissals
// dir under HOME. Comparison is over the comment-bearing records only; we trim
// volatile fields (uuid, parentUuid, timestamp, sessionId) so two runs of the
// same review produce comparable bytes.
func readSessionJSONL(t *testing.T, repoDir, sessionID string) string {
	t.Helper()
	path, err := session.SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		trimmed := stripVolatileFields(line)
		if trimmed != "" {
			out.WriteString(trimmed)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// initGitWorkspaceRepo creates a temp git repo with one committed file and an
// uncommitted modification, so a workspace-mode review has exactly one diff.
func initGitWorkspaceRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")
	// Uncommitted change -> workspace diff.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("modify main.go: %v", err)
	}
	return dir
}

// codeCommentsResponse emits a code_comment tool call whose comments slice is
// the provided set. Mirrors codeCommentResponse but with multiple comments and
// explicit path/content so fingerprints are deterministic.
func codeCommentsResponse(path string, comments []model.LlmComment) *llm.ChatResponse {
	content := ""
	raw := make([]map[string]any, len(comments))
	for i, c := range comments {
		raw[i] = map[string]any{
			"content": c.Content,
			"path":    firstNonEmpty(c.Path, path),
		}
	}
	args, _ := json.Marshal(map[string]any{"path": path, "comments": raw})
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_comment",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: string(args),
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 50, CompletionTokens: 20},
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// stripVolatileFields removes run-to-run-volatile keys (uuid/parentUuid/
// timestamp/sessionId/cwd/duration_seconds) from a JSONL record so two runs of
// the same review can be compared on their comment-bearing content (D3/AS4).
func stripVolatileFields(line string) string {
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return line
	}
	for _, k := range []string{"uuid", "parentUuid", "timestamp", "sessionId", "cwd", "duration_seconds"} {
		delete(rec, k)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return line
	}
	return string(b)
}

// buildDismissTestAgent constructs an Agent over repoDir with a fake LLM that
// emits the given comments for the single workspace diff. Mirrors the
// scaffolding in TestDispatchSubtasks_WithFakeLLM but with a real repo so Run
// can load diffs.
func buildDismissTestAgent(t *testing.T, repoDir string, emitted []model.LlmComment, filter *session.DismissalFilter) *Agent {
	t.Helper()
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})

	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		codeCommentsResponse("main.go", emitted),
		agentTaskDoneResponse(),
	}}
	a := New(Args{
		RepoDir:          repoDir,
		LLMClient:        client,
		Model:            "fake",
		CommentCollector: collector,
		Tools:            reg,
		Dismissals:       filter,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 10,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{diff}} for {{current_file_path}}"},
				},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})
	return a
}

// TestAgentRunSuppressesDismissed runs the full Agent.Run pipeline against a
// temp repo. The fake LLM emits two findings (D1/D2/D3): with a filter that
// dismisses the first, Run must return only the second; with a nil filter, Run
// must return both unchanged (byte-identical to the stateless default).
func TestAgentRunSuppressesDismissed(t *testing.T) {
	emitted := []model.LlmComment{
		{Path: "main.go", Content: "dismiss me", StartLine: 0, EndLine: 0},
		{Path: "main.go", Content: "keep me", StartLine: 0, EndLine: 0},
	}

	t.Run("filter suppresses dismissed finding", func(t *testing.T) {
		repo := initGitWorkspaceRepo(t)
		// Build a filter dismissing the first emitted comment's fingerprint.
		store, err := session.LoadDismissals(repo)
		if err != nil {
			t.Fatalf("LoadDismissals: %v", err)
		}
		store.Record(session.DismissalEntry{Fingerprint: session.DismissalFingerprint(emitted[0])})
		filter := session.NewDismissalFilter(store)

		a := buildDismissTestAgent(t, repo, emitted, filter)
		got, err := a.Run(context.Background())
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 comment (dismissed one suppressed), got %d: %+v", len(got), got)
		}
		if got[0].Content != "keep me" {
			t.Errorf("surviving comment = %q, want %q", got[0].Content, "keep me")
		}
	})

	t.Run("nil filter is byte-identical (stateless default)", func(t *testing.T) {
		repo := initGitWorkspaceRepo(t)
		a := buildDismissTestAgent(t, repo, emitted, nil)
		got, err := a.Run(context.Background())
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 comments with nil filter (no suppression), got %d: %+v", len(got), got)
		}
	})
}

// TestAgentRunEmptyFilterSuppressesNothing guards the empty-set fast path: an
// empty (but non-nil) store must not drop any findings.
func TestAgentRunEmptyFilterSuppressesNothing(t *testing.T) {
	repo := initGitWorkspaceRepo(t)
	emitted := []model.LlmComment{{Path: "main.go", Content: "keep", StartLine: 0, EndLine: 0}}
	store, err := session.LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	filter := session.NewDismissalFilter(store) // empty set
	a := buildDismissTestAgent(t, repo, emitted, filter)
	got, err := a.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("empty filter dropped comments: got %d, want 1", len(got))
	}
}

// TestAgentRunSessionJSONLUnchangedWithDismissalStore verifies D3/AS4: the
// recorded session JSONL is byte-identical whether or not a dismissal filter is
// applied (suppression is read-side only; the collector and session writes are
// untouched).
func TestAgentRunSessionJSONLUnchangedWithDismissalStore(t *testing.T) {
	emitted := []model.LlmComment{
		{Path: "main.go", Content: "dismiss me", StartLine: 0, EndLine: 0},
		{Path: "main.go", Content: "keep me", StartLine: 0, EndLine: 0},
	}

	// Run 1: nil filter (stateless default).
	repoNoFilter := initGitWorkspaceRepo(t)
	a1 := buildDismissTestAgent(t, repoNoFilter, emitted, nil)
	if _, err := a1.Run(context.Background()); err != nil {
		t.Fatalf("Run (no filter): %v", err)
	}
	noFilterJSONL := readSessionJSONL(t, repoNoFilter, a1.SessionID())

	// Run 2: active dismissal filter.
	repoWithFilter := initGitWorkspaceRepo(t)
	store, err := session.LoadDismissals(repoWithFilter)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	store.Record(session.DismissalEntry{Fingerprint: session.DismissalFingerprint(emitted[0])})
	a2 := buildDismissTestAgent(t, repoWithFilter, emitted, session.NewDismissalFilter(store))
	if _, err := a2.Run(context.Background()); err != nil {
		t.Fatalf("Run (with filter): %v", err)
	}
	withFilterJSONL := readSessionJSONL(t, repoWithFilter, a2.SessionID())

	// The comments recorded in the JSONL must be identical: the dismissal
	// filter suppresses only the returned slice, not what the collector wrote.
	if noFilterJSONL != withFilterJSONL {
		t.Errorf("session JSONL differs with dismissal store present (D3/AS4 violation):\n--- no filter ---\n%s\n--- with filter ---\n%s", noFilterJSONL, withFilterJSONL)
	}
	// And specifically: the dismissed comment must still be present in the JSONL
	// (suppression is not destructive to the recorded record).
	if !strings.Contains(withFilterJSONL, "dismiss me") {
		t.Errorf("dismissed finding absent from session JSONL (D3: suppression must not remove recorded findings): %s", withFilterJSONL)
	}
}
