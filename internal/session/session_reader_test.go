// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

// Exercise the persistence and public loading paths together. The first long
// checkpoint must remain intact after many larger response records are read.
func TestLoadSessionLongRecords(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	id, comments := writeLongRecordSession(t, repoDir, 64<<10, 32)
	path, err := SessionFilePath(repoDir, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, crlf := range []bool{false, true} {
		t.Run(fmt.Sprintf("CRLF=%t", crlf), func(t *testing.T) {
			content := data
			if crlf {
				content = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
			}
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
			summary, details, err := LoadDetail(repoDir, id)
			if err != nil {
				t.Fatal(err)
			}
			checkLongRecordSummary(t, summary, id, repoDir)
			if len(details) != 2 || details[0].FilePath != "first.go" || details[1].FilePath != "second.go" || details[0].Comments != 1 || details[1].Comments != 1 {
				t.Fatalf("details = %+v", details)
			}
			gotComments, err := LoadComments(repoDir, id)
			if err != nil || !reflect.DeepEqual(gotComments, comments) {
				t.Fatalf("comments changed after reading later records: count=%d, err=%v", len(gotComments), err)
			}
			for _, load := range []func(string, string) (*ResumeState, error){LoadResumeState, LoadReviewResumeState} {
				state, err := load(repoDir, id)
				if err != nil {
					t.Fatal(err)
				}
				checkLongRecordResume(t, state, id, repoDir)
				for _, comment := range comments {
					item, ok := state.ReusableItem("fp-" + comment.Path)
					if !ok || item.FilePath != comment.Path || !reflect.DeepEqual(item.Comments, []model.LlmComment{comment}) {
						t.Fatalf("checkpoint %s changed after reading later records", comment.Path)
					}
				}
			}
		})
	}

	// Summary display accepts an unterminated valid record. Resume drops it as
	// a torn write. A record larger than the buffer must keep that distinction.
	tail, err := json.Marshal(map[string]any{
		"type": "review_item_done", "filePath": "tail.go", "fingerprint": "fp-tail",
		"comments": []model.LlmComment{{Path: "tail.go", Content: strings.Repeat("tail", 32<<10)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, tail...), 0600); err != nil {
		t.Fatal(err)
	}
	_, details, err := LoadDetail(repoDir, id)
	if err != nil || len(details) != 3 || details[2].FilePath != "tail.go" {
		t.Fatalf("unterminated display record: details=%+v err=%v", details, err)
	}
	state, err := LoadResumeState(repoDir, id)
	if err != nil {
		t.Fatal(err)
	}
	checkLongRecordResume(t, state, id, repoDir)

	// A complete malformed line remains fatal to scan replay and skippable by
	// review replay and display; it is not mistaken for an incomplete fragment.
	malformed := append([]byte(strings.Repeat("!", 128<<10)), '\n')
	if err := os.WriteFile(path, append(data, malformed...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResumeState(repoDir, id); err == nil {
		t.Fatal("strict resume accepted a malformed complete record")
	}
	state, err = LoadReviewResumeState(repoDir, id)
	if err != nil {
		t.Fatal(err)
	}
	checkLongRecordResume(t, state, id, repoDir)
	summary, err := LoadSummary(repoDir, id)
	if err != nil {
		t.Fatal(err)
	}
	checkLongRecordSummary(t, summary, id, repoDir)
}

func writeLongRecordSession(tb testing.TB, repoDir string, responseBytes, responses int) (string, []model.LlmComment) {
	tb.Helper()
	sh := New(repoDir, "reader-test", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace, Operation: OperationReview})
	comments := []model.LlmComment{
		{Path: "first.go", Content: strings.Repeat("first ", 1024), StartLine: 1, EndLine: 2},
		{Path: "second.go", Content: strings.Repeat("second ", 2048), StartLine: 3, EndLine: 4},
	}
	manifest := sh.Manifest()
	manifest.SetInput(ManifestInput{Mode: InputModeWorkspace})
	for _, c := range comments {
		if err := manifest.RegisterSelected(CoverageItem{ItemID: c.Path, Path: c.Path, Fingerprint: "fp-" + c.Path}); err != nil {
			tb.Fatal(err)
		}
	}
	if err := manifest.SealSelected(); err != nil {
		tb.Fatal(err)
	}
	sh.RecordReviewItemDone("first.go", "first.go", "first.go", "fp-first.go", comments[:1])
	content := strings.Repeat("x", responseBytes)
	file := sh.GetOrCreateFileSession("first.go")
	for range responses {
		record := file.AppendTaskRecord(MainTask, []llm.Message{{Role: "user", Content: "review this file"}})
		record.SetResponse(&llm.ChatResponse{
			Model: "test-model", Usage: &llm.UsageInfo{PromptTokens: 4, CompletionTokens: 16},
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Role: "assistant", Content: &content}}},
		}, time.Millisecond)
	}
	sh.RecordReviewItemDone("second.go", "second.go", "second.go", "fp-second.go", comments[1:])
	for _, c := range comments {
		if err := manifest.MarkCompleted(c.Path); err != nil {
			tb.Fatal(err)
		}
	}
	frozen, err := manifest.Finalize(time.Second)
	if err != nil {
		tb.Fatal(err)
	}
	sh.SetFinalManifest(&frozen)
	if err := sh.Finalize(); err != nil {
		tb.Fatal(err)
	}
	return sh.SessionID, comments
}

func checkLongRecordSummary(tb testing.TB, summary *Summary, id, repoDir string) {
	tb.Helper()
	if summary.SessionID != id || summary.RepoDir != repoDir || summary.GitBranch != "reader-test" || summary.Model != "test-model" || summary.ReviewMode != ReviewModeWorkspace || summary.Aborted || summary.Legacy || summary.CompletedFiles != 2 || summary.TotalComments != 2 || summary.RunManifest == nil || summary.RunManifest.TerminalState != StateComplete {
		tb.Fatalf("summary = %+v", summary)
	}
}

func checkLongRecordResume(tb testing.TB, state *ResumeState, id, repoDir string) {
	tb.Helper()
	if state.SessionID != id || state.RepoDir != repoDir || state.GitBranch != "reader-test" || state.Model != "test-model" || state.ReviewMode != ReviewModeWorkspace || !state.Closed || state.CompletedCount() != 2 || state.Manifest == nil || state.Manifest.TerminalState != StateComplete {
		tb.Fatalf("resume metadata changed: id=%s repo=%s branch=%s model=%s mode=%s closed=%t checkpoints=%d", state.SessionID, state.RepoDir, state.GitBranch, state.Model, state.ReviewMode, state.Closed, state.CompletedCount())
	}
}

func BenchmarkLoadSessionResponseRecords(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	b.Setenv("USERPROFILE", os.Getenv("HOME"))
	for _, responseBytes := range []int{1024, 64 << 10, 256 << 10} {
		b.Run(fmt.Sprintf("bytes=%d", responseBytes), func(b *testing.B) {
			repoDir := b.TempDir()
			id, _ := writeLongRecordSession(b, repoDir, responseBytes, 64)
			b.Run("summary", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					summary, err := LoadSummary(repoDir, id)
					if err != nil {
						b.Fatal(err)
					}
					checkLongRecordSummary(b, summary, id, repoDir)
				}
			})
			b.Run("resume", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					state, err := LoadResumeState(repoDir, id)
					if err != nil {
						b.Fatal(err)
					}
					checkLongRecordResume(b, state, id, repoDir)
				}
			})
		})
	}
}
