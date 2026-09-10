// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

func runPreview(cc *commonContext, opts reviewOptions, out io.Writer) error {
	return runPreviewContext(context.Background(), cc, opts, out)
}

func TestRunPreview(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n", "add x")
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	silenceStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD"}, os.Stdout); err != nil {
			t.Fatalf("runPreview error: %v", err)
		}
	})
}

func TestRunPreviewJSONFormat(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "main.go", "package main\n", "add main")
	gitCommitFile(t, dir, "notes.md", "# notes\n", "add notes")
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD", outputFormat: "json"}, os.Stdout); err != nil {
			t.Errorf("runPreview error: %v", err)
		}
	})

	got := decodeSinglePreviewJSON(t, out)
	if got.TotalFiles != 1 {
		t.Fatalf("total_files = %d, want 1 (HEAD adds notes.md only)", got.TotalFiles)
	}
	if got.Entries[0].Path != "notes.md" {
		t.Errorf("path = %q, want notes.md", got.Entries[0].Path)
	}
	if got.Entries[0].WillReview {
		t.Error("notes.md should be excluded by the extension allowlist")
	}
	if got.Entries[0].ExcludeReason != model.ExcludeExtension {
		t.Errorf("exclude_reason = %q, want %q", got.Entries[0].ExcludeReason, model.ExcludeExtension)
	}
}

// TestRunPreviewAppliesResolvedMaxTokens pins that the preview path resolves
// the per-file token ceiling and hands it to selection: without it the template
// default reaches the agent as zero, the size gate never runs, and preview
// reports a file the review would drop before dispatch (#782). It also pins
// that resolving the limit needs no provider — this test configures no
// endpoint and no API key.
func TestRunPreviewAppliesResolvedMaxTokens(t *testing.T) {
	// A ceiling of 100 max_tokens means 80 usable, well under huge.go's diff,
	// whichever of the two sources supplies it.
	tests := []struct {
		name        string
		savedTokens int
		opts        reviewOptions
	}{
		{
			name: "cli override",
			opts: reviewOptions{commit: "HEAD", outputFormat: "json", maxTokens: 100},
		},
		{
			name:        "saved app config",
			savedTokens: 100,
			opts:        reviewOptions{commit: "HEAD", outputFormat: "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := freshOCRHome(t)
			if tt.savedTokens > 0 {
				writeAppConfigMaxTokens(t, home, tt.savedTokens)
			}

			dir := initTestGitRepo(t)
			gitCommitFile(t, dir, "huge.go", strings.Repeat("token ", 500), "add huge")
			cc, err := loadCommonContext(dir, "", "", 0, 0, true)
			if err != nil {
				t.Fatalf("loadCommonContext: %v", err)
			}

			out := captureStdout(t, func() {
				if err := runPreview(cc, tt.opts, os.Stdout); err != nil {
					t.Errorf("runPreview error: %v", err)
				}
			})

			got := decodeSinglePreviewJSON(t, out)
			if len(got.Entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(got.Entries))
			}
			if got.Entries[0].WillReview {
				t.Error("huge.go exceeds the resolved token ceiling and must not be listed as reviewable")
			}
			if got.Entries[0].ExcludeReason != model.ExcludeTooLarge {
				t.Errorf("exclude_reason = %q, want %q", got.Entries[0].ExcludeReason, model.ExcludeTooLarge)
			}
			if got.ReviewableCount != 0 || got.ExcludedCount != 1 {
				t.Errorf("reviewable=%d excluded=%d, want 0/1", got.ReviewableCount, got.ExcludedCount)
			}
		})
	}
}

// TestPreviewMaxTokensMatchesRun pins the one input preview and the run do not
// share. Both consume the same selection, but each resolves the max_tokens it
// hands that selection on its own path: the run from the app config its LLM
// runtime loaded, preview through previewMaxTokens, which builds no runtime. The
// run's value is read off the Args it hands the agent rather than recomputed
// here, so the assertion follows whatever the run actually applies.
func TestPreviewMaxTokensMatchesRun(t *testing.T) {
	tests := []struct {
		name        string
		savedTokens int
		cliTokens   int
	}{
		{name: "template default"},
		{name: "saved app config", savedTokens: 4096},
		{name: "cli override", cliTokens: 2048},
		{name: "cli override beats saved app config", savedTokens: 4096, cliTokens: 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := freshOCRHome(t)
			if tt.savedTokens > 0 {
				writeAppConfigMaxTokens(t, home, tt.savedTokens)
			}
			// The run resolves this endpoint but never calls it: the only change is
			// a file the static gates exclude, so it skips before any request.
			t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test-model")

			dir := initTestGitRepo(t)
			gitCommitFile(t, dir, "notes.md", "# notes\n", "add notes")

			applied := 0
			build := newReviewAgent
			t.Cleanup(func() { newReviewAgent = build })
			newReviewAgent = func(args agent.Args) *agent.Agent {
				applied = args.Template.MaxTokens
				return build(args)
			}

			args := []string{"--repo", dir, "--commit", "HEAD"}
			if tt.cliTokens > 0 {
				args = append(args, "--max-tokens", strconv.Itoa(tt.cliTokens))
			}
			var err error
			silenceStderr(t, func() {
				silenceStdout(t, func() { err = runReview(args) })
			})
			if err != nil {
				t.Fatalf("review: %v", err)
			}
			if applied == 0 {
				t.Fatal("the run never built its agent")
			}

			cc, err := loadCommonContext(dir, "", "", 0, 0, true)
			if err != nil {
				t.Fatalf("loadCommonContext: %v", err)
			}
			got, err := previewMaxTokens(cc.Template.MaxTokens, tt.cliTokens)
			if err != nil {
				t.Fatalf("previewMaxTokens: %v", err)
			}
			if got != applied {
				t.Errorf("previewMaxTokens = %d, but the run applied %d", got, applied)
			}
			// Agreeing on the template default would pass the check above without
			// exercising the source this case configures.
			configured := tt.savedTokens > 0 || tt.cliTokens > 0
			if configured && got == cc.Template.MaxTokens && applied == cc.Template.MaxTokens {
				t.Error("fixture proves nothing: neither path saw the configured limit")
			}
		})
	}
}

func writeAppConfigMaxTokens(t *testing.T, home string, maxTokens int) {
	t.Helper()
	dir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create OCR home: %v", err)
	}
	body := fmt.Sprintf(`{"max_tokens":%d}`, maxTokens)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write app config: %v", err)
	}
}

// TestRunPreviewCreatesNoSession pins that previewing never opens session
// persistence. Building the agent with agent.New auto-created a session, which
// left an unfinalized (and usually empty) JSONL file under the OCR home even
// though preview never runs or finalizes a review.
func TestRunPreviewCreatesNoSession(t *testing.T) {
	home := freshOCRHome(t)

	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n", "add x")
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	silenceStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD"}, os.Stdout); err != nil {
			t.Fatalf("runPreview error: %v", err)
		}
	})

	assertNoSessionStore(t, home)
}

func TestLoadReviewResumeState(t *testing.T) {
	dir := initTestGitRepo(t)

	t.Run("empty resume returns nil", func(t *testing.T) {
		state, err := loadReviewResumeState(dir, reviewOptions{})
		if err != nil || state != nil {
			t.Errorf("got state=%v err=%v, want nil,nil", state, err)
		}
	})

	t.Run("workspace resume rejected", func(t *testing.T) {
		_, err := loadReviewResumeState(dir, reviewOptions{resume: "sess-1"})
		if err == nil {
			t.Fatal("expected error for workspace-mode resume")
		}
	})

	t.Run("missing session load fails", func(t *testing.T) {
		_, err := loadReviewResumeState(dir, reviewOptions{resume: "does-not-exist", commit: "HEAD"})
		if err == nil {
			t.Fatal("expected error loading nonexistent resume session")
		}
	})
}

func TestInitMCPClients(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()

	t.Run("nil config", func(t *testing.T) {
		if got := initMCPClients(ctx, nil, reg, "/tmp", "v"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("empty servers", func(t *testing.T) {
		if got := initMCPClients(ctx, &Config{}, reg, "/tmp", "v"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("remote without url skipped", func(t *testing.T) {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"r": {Type: "remote"},
		}}
		if got := initMCPClients(ctx, cfg, reg, "/tmp", "v"); len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})

	t.Run("stdio without command skipped", func(t *testing.T) {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"s": {Type: "stdio"},
		}}
		if got := initMCPClients(ctx, cfg, reg, "/tmp", "v"); len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})
}
