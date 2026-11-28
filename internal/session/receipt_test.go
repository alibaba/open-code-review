// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func readReceipt(t *testing.T, repoDir, sessionID string) Receipt {
	t.Helper()
	dir, err := SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, sessionID+".receipt.json"))
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("receipt is not valid JSON: %v (raw: %s)", err, data)
	}
	return r
}

func TestReceipt_InitialPendingWrite(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "feature", "test-model", SessionOptions{ReviewMode: ReviewModeRange, DiffFrom: "main", DiffTo: "feature"})

	sh.SetReceiptGroups([]ReceiptGroup{
		{GroupID: "0", Label: "auth", Files: []string{"a.go", "b.go"}},
		{GroupID: "1", Label: "docs", Files: []string{"c.md"}},
	})

	r := readReceipt(t, repoDir, sh.SessionID)
	if r.Schema != ReceiptSchema {
		t.Errorf("schema = %q", r.Schema)
	}
	if r.Status != ReceiptStatusRunning {
		t.Errorf("status = %q, want running", r.Status)
	}
	if r.SessionID != sh.SessionID || r.RepoDir != repoDir {
		t.Errorf("identity fields wrong: %+v", r)
	}
	if len(r.Groups) != 2 || len(r.Files) != 3 {
		t.Fatalf("groups/files = %d/%d, want 2/3", len(r.Groups), len(r.Files))
	}
	for _, f := range r.Files {
		if f.State != ReceiptItemPending {
			t.Errorf("file %s state = %q, want pending", f.Path, f.State)
		}
		if f.GroupID == "" {
			t.Errorf("file %s missing group_id", f.Path)
		}
	}
	if r.Groups[0].State != "pending" || r.Groups[0].CompletedFiles != 0 || len(r.Groups[0].MissingFiles) != 2 {
		t.Errorf("group rollup wrong: %+v", r.Groups[0])
	}
}

func TestReceipt_TransitionsRewriteStates(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeFullScan})

	sh.SetReceiptGroups([]ReceiptGroup{
		{GroupID: "batch-0", Label: "batch 0", Files: []string{"a.go", "b.go", "c.go"}},
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "x"}})
	sh.RecordReviewItemReused("b.go", "b.go", "b.go", "fp-b", "old-session", nil)
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "provider 500")

	r := readReceipt(t, repoDir, sh.SessionID)
	states := map[string]string{}
	for _, f := range r.Files {
		states[f.Path] = f.State
	}
	if states["a.go"] != ReceiptItemDone || states["b.go"] != ReceiptItemReused || states["c.go"] != ReceiptItemFailed {
		t.Fatalf("states = %v", states)
	}
	g := r.Groups[0]
	if g.CompletedFiles != 2 || g.State != "partial" {
		t.Errorf("rollup = %+v, want completed=2 state=partial", g)
	}
	if len(g.MissingFiles) != 1 || g.MissingFiles[0] != "c.go" {
		t.Errorf("missing files = %v, want [c.go]", g.MissingFiles)
	}
	if g.FailureCode == "" {
		t.Errorf("failure_code must carry the failed item's reason")
	}
}

func TestReceipt_FinalizeStatusMatrix(t *testing.T) {
	tests := []struct {
		name       string
		record     func(sh *SessionHistory)
		cancel     bool
		noReceipt  bool
		wantStatus string
		verify     func(t *testing.T, r Receipt)
	}{
		{
			// A run that never reached dispatch writes no receipt at all —
			// there is nothing it could honestly report (the CLI surfaces the
			// run's own error instead).
			name:       "no receipt when nothing was ever tracked",
			record:     func(sh *SessionHistory) {},
			noReceipt:  true,
			wantStatus: "",
		},
		{
			name: "complete when every item done",
			record: func(sh *SessionHistory) {
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})
				sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", nil)
			},
			wantStatus: ReceiptStatusComplete,
		},
		{
			name: "complete counts reused",
			record: func(sh *SessionHistory) {
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})
				sh.RecordReviewItemReused("a.go", "a.go", "a.go", "fp", "old", nil)
			},
			wantStatus: ReceiptStatusComplete,
		},
		{
			name: "failed when every item failed",
			record: func(sh *SessionHistory) {
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})
				sh.RecordReviewItemFailed("a.go", "a.go", "a.go", "fp", "boom")
			},
			wantStatus: ReceiptStatusFailed,
		},
		{
			name: "partial on mixed outcomes",
			record: func(sh *SessionHistory) {
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go", "b.go"}}})
				sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", nil)
				sh.RecordReviewItemFailed("b.go", "b.go", "b.go", "fp", "boom")
			},
			wantStatus: ReceiptStatusPartial,
		},
		{
			name: "cancelled override wins over partial",
			record: func(sh *SessionHistory) {
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go", "b.go"}}})
				sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", nil)
				sh.MarkReceiptCancelled()
			},
			wantStatus: ReceiptStatusCancelled,
		},
		{
			name: "running survives a kill (no finalize)",
			record: func(sh *SessionHistory) {
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})
				sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", nil)
				sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go", "b.go"}}})
			},
			wantStatus: ReceiptStatusRunning,
			verify: func(t *testing.T, r Receipt) {
				for _, f := range r.Files {
					if f.Path == "a.go" && f.State != ReceiptItemDone {
						t.Errorf("re-installing the expected set must preserve recorded state, got %s: %s", f.Path, f.State)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir := t.TempDir()
			sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeRange})
			tt.record(sh)
			if tt.wantStatus != ReceiptStatusRunning {
				sh.Finalize()
			}
			if tt.noReceipt {
				dir, _ := SessionsDir(repoDir)
				if _, err := os.Stat(filepath.Join(dir, sh.SessionID+".receipt.json")); !os.IsNotExist(err) {
					t.Errorf("expected no receipt for a run that never dispatched, stat err = %v", err)
				}
				return
			}
			r := readReceipt(t, repoDir, sh.SessionID)
			if r.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", r.Status, tt.wantStatus)
			}
			if tt.verify != nil {
				tt.verify(t, r)
			}
		})
	}
}

func TestReceipt_InvisibleToSessionList(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeRange})
	sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})
	sh.Finalize()

	summaries, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("session list enumerated %d entries, want exactly 1 (the .jsonl journal)", len(summaries))
	}
	if summaries[0].SessionID != sh.SessionID || summaries[0].Aborted {
		t.Errorf("listed session = %+v", summaries[0])
	}
	// The receipt file must exist but never be enumerated as a journal.
	dir, _ := SessionsDir(repoDir)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.receipt.json"))
	if len(matches) != 1 {
		t.Errorf("expected exactly one receipt file, got %v", matches)
	}
	residue, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(residue) != 0 {
		t.Errorf("atomic rename left tmp residue: %v", residue)
	}
}

func TestReceipt_WriteFailureNeverFailsRecording(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeRange})
	sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})

	// Point HOME at an unwritable location so receipt writes fail; the
	// checkpoint record must still succeed.
	dir, _ := SessionsDir(repoDir)
	_ = os.RemoveAll(dir)
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Skipf("cannot prepare read-only dir: %v", err)
	}
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", nil)
	sh.Finalize() // must not panic or error out from receipt failures
}

func TestReceipt_UntrackedPathStillTracked(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeRange})
	sh.SetReceiptGroups([]ReceiptGroup{{GroupID: "0", Files: []string{"a.go"}}})
	// A resume flow can record a path that was never installed as expected.
	sh.RecordReviewItemReused("extra.go", "extra.go", "extra.go", "fp", "old", nil)

	r := readReceipt(t, repoDir, sh.SessionID)
	found := false
	for _, f := range r.Files {
		if f.Path == "extra.go" {
			found = true
			if f.State != ReceiptItemReused {
				t.Errorf("untracked path state = %q, want reused", f.State)
			}
			if f.GroupID != "" {
				t.Errorf("untracked path must carry no group_id, got %q", f.GroupID)
			}
		}
	}
	if !found {
		t.Error("untracked path missing from the receipt")
	}
}
