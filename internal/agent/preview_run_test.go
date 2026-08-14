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
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
)

func initPreviewRepo(t *testing.T) string {
	t.Helper()
	// A temporary OCR home keeps any session these tests open out of the real
	// one; git resolves its global config independently of HOME, so neutralize
	// that too or a developer's signing directive fails the fixture commit.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# r\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	// Committed so a test can delete it and exercise the deletion gate.
	if err := os.WriteFile(filepath.Join(dir, "gone.go"), []byte("package gone\n"), 0o644); err != nil {
		t.Fatalf("write gone.go: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// TestPreview exercises Agent.Preview against a real workspace diff so the
// full preview-building path (loadDiffs + whyExcluded + entry assembly) runs.
func TestPreview(t *testing.T) {
	dir := initPreviewRepo(t)

	// A reviewable Go file and an excluded binary-ish/extension file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write data.bin: %v", err)
	}

	a := New(Args{RepoDir: dir})
	preview, err := a.preview(context.Background())
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.TotalFiles == 0 {
		t.Fatal("Preview reported zero files despite workspace changes")
	}
	if preview.ReviewableCount == 0 {
		t.Error("expected at least one reviewable entry (main.go)")
	}
	if len(preview.Entries) != preview.TotalFiles {
		t.Errorf("entries=%d totalFiles=%d, want equal", len(preview.Entries), preview.TotalFiles)
	}
}

// TestPreviewMarksOversizedDiffTooLarge pins that preview applies the per-file
// diff-size ceiling the real run applies before dispatch, and reports it under
// its own reason rather than silently listing the file as reviewable.
func TestPreviewMarksOversizedDiffTooLarge(t *testing.T) {
	dir := initPreviewRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("package small\n"), 0o644); err != nil {
		t.Fatalf("write small.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "huge.go"), []byte(strings.Repeat("token ", 500)), 0o644); err != nil {
		t.Fatalf("write huge.go: %v", err)
	}

	// MaxTokens=100 puts the per-file ceiling at 80 tokens: small.go stays well
	// under it, huge.go is far over.
	preview, err := Preview(context.Background(), Args{
		RepoDir:  dir,
		Template: template.Template{MaxTokens: 100},
	})
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}

	reasons := make(map[string]ExcludeReason, len(preview.Entries))
	willReview := make(map[string]bool, len(preview.Entries))
	for _, e := range preview.Entries {
		reasons[e.Path] = e.ExcludeReason
		willReview[e.Path] = e.WillReview
	}

	if !willReview["small.go"] {
		t.Errorf("small.go should be reviewable, got exclude_reason=%q", reasons["small.go"])
	}
	if willReview["huge.go"] {
		t.Error("huge.go exceeds the per-file token ceiling and must not be listed as reviewable")
	}
	if reasons["huge.go"] != ExcludeTooLarge {
		t.Errorf("huge.go exclude_reason = %q, want %q", reasons["huge.go"], ExcludeTooLarge)
	}
}

// TestPreviewMatchesRunSelectedCoverage is the property #782 asked for:
// preview's will_review set must equal the set a fresh, non-resumed,
// unbudgeted run registers as selected coverage. The changeset deliberately
// mixes every outcome — reviewable, extension-filtered, deleted, and oversized
// — so a gate applied on only one of the two paths breaks the comparison.
func TestPreviewMatchesRunSelectedCoverage(t *testing.T) {
	dir := initPreviewRepo(t)

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("main.go", "package main\n")
	write("notes.md", "# notes\n")
	write("huge.go", strings.Repeat("token ", 2000))
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatalf("remove gone.go: %v", err)
	}

	// The ceiling lands at 800 tokens: huge.go is over it, main.go's rendered
	// prompt stays far under, so only the size gate separates the two.
	args := Args{
		RepoDir:   dir,
		LLMClient: manifestFlowClient{},
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           1000,
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
	var previewed []string
	byPath := make(map[string]DiffPreviewEntry, len(preview.Entries))
	for _, e := range preview.Entries {
		byPath[e.Path] = e
		if e.WillReview {
			previewed = append(previewed, e.Path)
		}
	}
	if !slices.Equal(previewed, []string{"main.go"}) {
		t.Fatalf("preview will_review = %v, want [main.go]; entries = %+v", previewed, preview.Entries)
	}
	// Pin the fixture itself: the comparison below is only meaningful while each
	// exclusion path is actually represented.
	for path, want := range map[string]ExcludeReason{
		"notes.md": ExcludeExtension,
		"gone.go":  ExcludeDeleted,
		"huge.go":  ExcludeTooLarge,
	} {
		if got := byPath[path].ExcludeReason; got != want {
			t.Fatalf("%s exclude_reason = %q, want %q", path, got, want)
		}
	}

	a := New(args)
	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil {
		t.Fatal("expected a frozen run manifest")
	}
	var selected []string
	for _, item := range manifest.Coverage.Selected {
		selected = append(selected, item.Path)
	}

	slices.Sort(previewed)
	slices.Sort(selected)
	if !slices.Equal(previewed, selected) {
		t.Errorf("preview will_review = %v but the run selected %v", previewed, selected)
	}
}

// TestPreviewEmptyEntriesNotNil pins that a clean workspace still yields a
// non-nil Entries slice, so JSON output marshals `"files":[]` instead of
// `"files":null`. Scan preview already guarantees this; review must agree.
func TestPreviewEmptyEntriesNotNil(t *testing.T) {
	dir := initPreviewRepo(t)

	a := New(Args{RepoDir: dir})
	preview, err := a.preview(context.Background())
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.TotalFiles != 0 {
		t.Fatalf("expected a clean workspace, got %d file(s)", preview.TotalFiles)
	}
	if preview.Entries == nil {
		t.Error("Entries is nil; JSON output would emit \"files\":null")
	}
}
