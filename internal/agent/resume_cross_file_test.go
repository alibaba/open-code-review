// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestAuditResumePreservesCrossFileFinding(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1 +1 @@\n-old()\n+foo := bar.Baz()", NewFileContent: "foo := bar.Baz()\n", Insertions: 1},
		{OldPath: "b.go", NewPath: "b.go", Diff: "@@ -1 +1 @@\n-old()\n+unrelated()", NewFileContent: "unrelated()\n", Insertions: 1},
	}
	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		agentTaskDoneResponse(),
		codeCommentResponse("b.go"),
		agentTaskDoneResponse(),
	}}
	parent := newManifestFlowAgentWithClient(t, diffs, nil, client)
	parent.args.MaxConcurrency = 1
	parent.args.Tools.Register(&tool.CodeCommentProvider{Collector: parent.args.CommentCollector})
	comments, err := parent.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Path != "a.go" {
		t.Fatalf("parent comments = %+v, want one finding relocated to a.go", comments)
	}
	manifest := finishManifestFlow(t, parent)
	if manifest.TerminalState != session.StateComplete {
		t.Fatalf("parent state = %s", manifest.TerminalState)
	}
	state, err := session.LoadReviewResumeState(parent.args.RepoDir, parent.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	childClient := &fakeAgentClient{}
	child := newManifestFlowAgentWithClient(t, diffs, state, childClient)
	resumedComments, err := child.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if childClient.calls != 0 || child.ResumeInfo().ReusedFiles != 2 {
		t.Fatalf("expected full reuse; calls=%d, resume=%+v", childClient.calls, child.ResumeInfo())
	}
	if len(resumedComments) != len(comments) {
		t.Fatalf("resume silently lost a finding: parent=%+v, resumed=%+v", comments, resumedComments)
	}
}

type crossFileResumeClient struct {
	failTarget bool
	cancel     context.CancelFunc
	reported   bool
}

func (c *crossFileResumeClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, _ := llm.RequestMetaFromContext(ctx)
	switch meta.FilePath {
	case "a.go":
		if c.failTarget {
			return nil, errors.New("target review failed")
		}
	case "b.go":
		if !c.reported {
			c.reported = true
			return codeCommentResponse("b.go"), nil
		}
	case "c.go":
		c.cancel()
		return nil, ctx.Err()
	}
	return agentTaskDoneResponse(), nil
}

func TestFinalReviewCheckpointsPreserveCoverage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reuse      bool
		failTarget bool
		cancel     bool
	}{
		{name: "reused target", reuse: true},
		{name: "failed target stays failed", failTarget: true},
		{name: "cancelled run retains completed target", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diffs := []model.Diff{
				{OldPath: "a.go", NewPath: "a.go", Diff: "+foo := bar.Baz()", NewFileContent: "foo := bar.Baz()\n", Insertions: 1},
				{OldPath: "b.go", NewPath: "b.go", Diff: "+unrelated()", NewFileContent: "unrelated()\n", Insertions: 1},
			}
			if tc.cancel {
				diffs = append(diffs, model.Diff{OldPath: "c.go", NewPath: "c.go", Diff: "+pending()", NewFileContent: "pending()\n", Insertions: 1})
			}
			fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
			var previous *session.ResumeState
			if tc.reuse {
				previous = &session.ResumeState{
					SessionID: "previous-review",
					Items: map[string]session.ResumeItem{fingerprint: {
						FilePath: "a.go", OldPath: "a.go", NewPath: "a.go", Fingerprint: fingerprint,
						Comments: []model.LlmComment{{Path: "a.go", Content: "previous finding", StartLine: 1, EndLine: 1}},
					}},
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &crossFileResumeClient{failTarget: tc.failTarget, cancel: cancel}
			parent := newManifestFlowAgentWithClient(t, diffs, previous, client)
			parent.args.MaxConcurrency = 1
			parent.args.Tools.Register(&tool.CodeCommentProvider{Collector: parent.args.CommentCollector})
			if previous != nil {
				seedParentManifest(t, parent, fingerprint)
			}
			comments, err := parent.dispatchSubtasks(ctx)
			if tc.cancel {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("dispatch error = %v, want cancellation", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			wantComments := 1
			if tc.reuse {
				wantComments++
			}
			if len(comments) != wantComments || comments[len(comments)-1].Path != "a.go" {
				t.Fatalf("parent comments = %+v", comments)
			}
			manifest := finishManifestFlow(t, parent)
			state, err := session.LoadReviewResumeState(parent.args.RepoDir, parent.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			item, reusable := state.ReusableItem(fingerprint)
			if tc.failTarget {
				if reusable || len(manifest.Coverage.Failed) != 1 {
					t.Fatalf("failed target became reusable: item=%+v, coverage=%+v", item, manifest.Coverage)
				}
				if _, exists := state.Item(fingerprint); exists {
					t.Fatal("failed checkpoint was overwritten")
				}
				return
			}
			if !reusable || len(item.Comments) != wantComments || item.Comments[wantComments-1].Content != comments[wantComments-1].Content {
				t.Fatalf("final reusable checkpoint lost finding: %+v", item)
			}
			summary, err := session.LoadSummary(parent.args.RepoDir, parent.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			if summary.TotalComments != wantComments {
				t.Fatalf("summary comments = %d, want %d", summary.TotalComments, wantComments)
			}
			persistedComments, err := session.LoadComments(parent.args.RepoDir, parent.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			if len(persistedComments) != wantComments {
				t.Fatalf("persisted comments = %d, want %d", len(persistedComments), wantComments)
			}
			if tc.reuse {
				if len(manifest.Coverage.Reused) != 1 {
					t.Fatalf("reused coverage changed: %+v", manifest.Coverage)
				}
				path, err := session.SessionFilePath(parent.args.RepoDir, parent.SessionID())
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var checkpoint map[string]any
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					var record map[string]any
					if err := json.Unmarshal([]byte(line), &record); err != nil {
						t.Fatal(err)
					}
					if record["fingerprint"] == fingerprint {
						checkpoint = record
					}
				}
				if checkpoint["type"] != "review_item_reused" || checkpoint["sourceSessionId"] != previous.SessionID {
					t.Fatalf("reused checkpoint lineage changed: %+v", checkpoint)
				}
			}
		})
	}
}
