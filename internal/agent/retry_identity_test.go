// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// metaCaptureClient records the RequestMeta carried by each request's context.
// Identity travels in the context, so the client is the only place it becomes
// observable from outside package llm.
type metaCaptureClient struct {
	mu       sync.Mutex
	metas    []llm.RequestMeta
	haveMeta []bool
	reply    string
}

func (c *metaCaptureClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, ok := llm.RequestMetaFromContext(ctx)
	c.mu.Lock()
	c.metas = append(c.metas, meta)
	c.haveMeta = append(c.haveMeta, ok)
	c.mu.Unlock()

	reply := c.reply
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}},
		Usage:   &llm.UsageInfo{PromptTokens: 1, CompletionTokens: 1},
	}, nil
}

func (c *metaCaptureClient) only(t *testing.T) (llm.RequestMeta, bool) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.metas) != 1 {
		t.Fatalf("got %d requests, want 1", len(c.metas))
	}
	return c.metas[0], c.haveMeta[0]
}

// TestExecuteGroupPlanPhase_Identity checks the plan request carries the identity
// of the record executeGroupPlanPhase created for it. The empty-provider case is
// covered too: an unnamed endpoint legitimately has no provider label, and that
// must not suppress identity. A two-file group is used deliberately — FilePath
// must be the group key, because that is the string the file session was opened
// under and the retry report joins the session JSONL on all three fields.
func TestExecuteGroupPlanPhase_Identity(t *testing.T) {
	for _, provider := range []string{"openai", ""} {
		name := provider
		if name == "" {
			name = "empty-provider"
		}
		t.Run(name, func(t *testing.T) {
			sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
			client := &metaCaptureClient{reply: "plan output"}
			a := New(Args{
				LLMClient: client,
				Provider:  provider,
				Model:     "test",
				Session:   sess,
				Template: template.Template{
					PlanTask: &template.LlmConversation{
						Messages: []template.ChatMessage{{Role: "user", Content: "plan {{diffs}}"}},
					},
					MaxTokens:           10000,
					MaxToolRequestTimes: 5,
					MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
				},
			})
			a.currentDate = "2026-08-07 10:00"

			g := FileGroup{Label: "core", Diffs: []model.Diff{
				{NewPath: "main.go", Diff: "+x"},
				{NewPath: "helper.go", Diff: "+y"},
			}}
			groupKey := fileGroupKey(g.Diffs)
			if _, err := a.executeGroupPlanPhase(context.Background(), g, buildConcatenatedDiffs(g.Diffs), "", ""); err != nil {
				t.Fatalf("executeGroupPlanPhase: %v", err)
			}

			meta, ok := client.only(t)
			if !ok {
				t.Fatal("plan request carried no identity")
			}
			want := llm.RequestMeta{
				Provider:  provider,
				Model:     "test",
				FilePath:  groupKey,
				TaskType:  string(session.PlanTask),
				RequestNo: 1,
			}
			if meta != want {
				t.Errorf("meta = %+v, want %+v", meta, want)
			}

			// The report joins against the session JSONL, so the meta must match
			// the record that was actually written, not merely look plausible.
			recs := sess.GetOrCreateFileSession(groupKey).TaskRecords[session.PlanTask]
			if len(recs) != 1 {
				t.Fatalf("session holds %d plan records, want 1", len(recs))
			}
			if meta.RequestNo != recs[0].RequestNo {
				t.Errorf("meta RequestNo = %d, record = %d", meta.RequestNo, recs[0].RequestNo)
			}
		})
	}
}

// TestExecuteReviewFilter_Identity is the review-filter counterpart. The filter
// only runs when comments exist for the file, so one is seeded first.
func TestExecuteReviewFilter_Identity(t *testing.T) {
	sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "keep this"})

	client := &metaCaptureClient{reply: `[]`}
	a := New(Args{
		LLMClient:        client,
		Provider:         "openai",
		Model:            "test",
		Session:          sess,
		CommentCollector: collector,
		Template: template.Template{
			ReviewFilterTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "filter {{comments}} {{path}} {{diff}}"}},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	a.executeGroupReviewFilter(context.Background(), FileGroup{Label: "a.go", Diffs: []model.Diff{{NewPath: "a.go", Diff: "+x"}}}, nil)

	meta, ok := client.only(t)
	if !ok {
		t.Fatal("review filter request carried no identity")
	}
	want := llm.RequestMeta{
		Provider:  "openai",
		Model:     "test",
		FilePath:  "a.go",
		TaskType:  string(session.ReviewFilterTask),
		RequestNo: 1,
	}
	if meta != want {
		t.Errorf("meta = %+v, want %+v", meta, want)
	}

	recs := sess.GetOrCreateFileSession("a.go").TaskRecords[session.ReviewFilterTask]
	if len(recs) != 1 {
		t.Fatalf("session holds %d review filter records, want 1", len(recs))
	}
	if meta.RequestNo != recs[0].RequestNo {
		t.Errorf("meta RequestNo = %d, record = %d", meta.RequestNo, recs[0].RequestNo)
	}
}

// TestNewRequestMeta_IsSingleSourceOfProviderAndModel guards the reason the
// helper exists: five request types read provider and model through it, so a
// change to Args must not leave some of them behind.
func TestNewRequestMeta_IsSingleSourceOfProviderAndModel(t *testing.T) {
	a := New(Args{Provider: "my-gateway", Model: "m1"})
	got := a.newRequestMeta("dir/f.go", session.MainTask, 3)
	want := llm.RequestMeta{
		Provider:  "my-gateway",
		Model:     "m1",
		FilePath:  "dir/f.go",
		TaskType:  string(session.MainTask),
		RequestNo: 3,
	}
	if got != want {
		t.Errorf("newRequestMeta = %+v, want %+v", got, want)
	}
}
