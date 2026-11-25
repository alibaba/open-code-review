// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

const (
	pureRenamePatch = "diff --git a/old/f.go b/pkg/f.go\n" +
		"similarity index 100%\n" +
		"rename from old/f.go\n" +
		"rename to pkg/f.go\n"
	modeChangePatch = "diff --git a/run.sh b/run.sh\n" +
		"old mode 100644\n" +
		"new mode 100755\n"
	emptyNewFilePatch = "diff --git a/empty.go b/empty.go\n" +
		"new file mode 100644\n" +
		"index 0000000..e69de29\n"
	editedRenamePatch = "diff --git a/old/g.go b/pkg/g.go\n" +
		"similarity index 80%\n" +
		"rename from old/g.go\n" +
		"rename to pkg/g.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/old/g.go\n" +
		"+++ b/pkg/g.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" package pkg\n" +
		"-func G() int { return 1 }\n" +
		"+func G() int { return 2 }\n"
)

func parseOne(t *testing.T, patch string) model.Diff {
	t.Helper()
	diffs := diff.ParsePatch(patch)
	if len(diffs) != 1 {
		t.Fatalf("ParsePatch returned %d diffs, want 1", len(diffs))
	}
	return diffs[0]
}

func TestHasNoHunk(t *testing.T) {
	tests := []struct {
		name string
		d    model.Diff
		want bool
	}{
		{"pure rename", parseOne(t, pureRenamePatch), true},
		{"mode change only", parseOne(t, modeChangePatch), true},
		{"empty new file", parseOne(t, emptyNewFilePatch), true},
		{"rename with an edit", parseOne(t, editedRenamePatch), false},
		{"diff text that is not git output", model.Diff{NewPath: "a.go", Diff: "word word"}, false},
		{"no diff text", model.Diff{NewPath: "a.go"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNoHunk(tt.d); got != tt.want {
				t.Errorf("hasNoHunk() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A diff without a hunk is excluded as unchanged but, like a deletion, stays in
// the working set for prompt context and is never dispatched or estimated.
func TestSelectFilesUnchangedGate(t *testing.T) {
	a := New(Args{Template: template.Template{MaxTokens: 100000}})
	diffs := []model.Diff{
		parseOne(t, pureRenamePatch),
		parseOne(t, modeChangePatch),
		parseOne(t, emptyNewFilePatch),
		parseOne(t, editedRenamePatch),
		{OldPath: "gone.go", NewPath: "/dev/null", IsDeleted: true},
	}

	decisions := a.selectFiles(diffs)
	var reasons []ExcludeReason
	for _, dec := range decisions {
		reasons = append(reasons, dec.Reason)
	}
	want := []ExcludeReason{ExcludeUnchanged, ExcludeUnchanged, ExcludeUnchanged, ExcludeNone, ExcludeDeleted}
	if !slices.Equal(reasons, want) {
		t.Fatalf("reasons = %q, want %q", reasons, want)
	}
	if !ExcludeUnchanged.IsExclusion() {
		t.Error("ExcludeUnchanged.IsExclusion() = false, want true")
	}

	kept, counts := summarizeSelection(decisions)
	if counts.Selected != 1 || counts.Unchanged != 3 {
		t.Errorf("counts = %+v, want 1 selected and 3 unchanged", counts)
	}
	if len(kept) != len(diffs) {
		t.Errorf("kept %d diffs, want all %d retained for context", len(kept), len(diffs))
	}
	if n := countDispatchable(kept); n != 1 {
		t.Errorf("countDispatchable = %d, want 1 (pkg/g.go)", n)
	}
	if est := estimateDiffCost(kept); est.Files != 1 {
		t.Errorf("estimateDiffCost files = %d, want 1 (pkg/g.go)", est.Files)
	}
}

type promptRecorder struct {
	mu      sync.Mutex
	prompts []string
}

func (r *promptRecorder) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt strings.Builder
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt.WriteString(text)
		}
	}
	r.mu.Lock()
	r.prompts = append(r.prompts, prompt.String())
	r.mu.Unlock()
	return agentTaskDoneResponse(), nil
}

// TestRunSkipsCommitWithoutContentChanges reviews a real commit that moves one
// file, flips another's executable bit and edits a third. Only the edited file
// may reach the model; the other two are reported as unchanged in both the
// preview and the run manifest.
func TestRunSkipsCommitWithoutContentChanges(t *testing.T) {
	dir := initPreviewRepo(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("moved.go", "package moved\n\nfunc M() int { return 1 }\n")
	write("run.sh", "#!/bin/sh\necho hi\n")
	write("edited.go", "package edited\n\nfunc E() int { return 1 }\n")
	git("add", ".")
	git("commit", "-m", "base")

	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("mv", "moved.go", "pkg/moved.go")
	git("update-index", "--chmod=+x", "run.sh")
	write("edited.go", "package edited\n\nfunc E() int { return 2 }\n")
	git("add", "edited.go")
	git("commit", "-m", "move, chmod and edit")

	client := &promptRecorder{}
	args := Args{
		RepoDir:   dir,
		Commit:    "HEAD",
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{current_file_path}} {{diff}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{{
			Type:     "function",
			Function: llm.FunctionDef{Name: "task_done", Description: "finish the review"},
		}},
	}

	preview, err := Preview(context.Background(), args)
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	got := map[string]ExcludeReason{}
	for _, e := range preview.Entries {
		got[e.Path] = e.ExcludeReason
	}
	wantPreview := map[string]ExcludeReason{
		"pkg/moved.go": ExcludeUnchanged,
		"run.sh":       ExcludeUnchanged,
		"edited.go":    ExcludeNone,
	}
	for path, want := range wantPreview {
		if reason, ok := got[path]; !ok || reason != want {
			t.Errorf("preview %s = %q (present=%v), want %q; entries = %+v", path, reason, ok, want, preview.Entries)
		}
	}

	a := New(args)
	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(client.prompts) == 0 {
		t.Fatal("the edited file was never sent to the model")
	}
	for _, prompt := range client.prompts {
		if strings.Contains(prompt, "moved.go") || strings.Contains(prompt, "run.sh") {
			t.Errorf("a file without content changes reached the model:\n%s", prompt)
		}
	}

	manifest := a.RunManifest()
	if manifest == nil {
		t.Fatal("expected a frozen run manifest")
	}
	var selected []string
	for _, item := range manifest.Coverage.Selected {
		selected = append(selected, item.Path)
	}
	if !slices.Equal(selected, []string{"edited.go"}) {
		t.Errorf("coverage selected = %v, want [edited.go]", selected)
	}
	excluded := map[string]ExcludeReason{}
	for _, item := range manifest.Coverage.Excluded {
		excluded[item.Path] = item.ExcludeReason
	}
	for _, path := range []string{"pkg/moved.go", "run.sh"} {
		if excluded[path] != ExcludeUnchanged {
			t.Errorf("coverage excluded %s = %q, want %q", path, excluded[path], ExcludeUnchanged)
		}
	}
}
