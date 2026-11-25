// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type manifestFlowClient struct{}

func (manifestFlowClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	switch {
	case strings.Contains(prompt, "panic.go"):
		panic("manifest integration panic")
	case strings.Contains(prompt, "bad.go"):
		return nil, errors.New("provider rejected api_key=LEAKED at /Users/example/private")
	case strings.Contains(prompt, "slow.go"):
		// A per-item deadline (wrapped, not a cancelled dispatch ctx) so the
		// timeout stays isolated to this file while its sibling succeeds.
		return nil, fmt.Errorf("provider call timed out: %w", context.DeadlineExceeded)
	default:
		return agentTaskDoneResponse(), nil
	}
}

type cancellationFlowClient struct {
	blocked chan struct{}
	once    sync.Once
}

func (c *cancellationFlowClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	if strings.Contains(prompt, "blocked.go") {
		c.once.Do(func() { close(c.blocked) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return agentTaskDoneResponse(), nil
}

func newManifestFlowAgent(t *testing.T, diffs []model.Diff, resume *session.ResumeState) *Agent {
	t.Helper()
	return newManifestFlowAgentWithClient(t, diffs, resume, manifestFlowClient{})
}

func newManifestFlowAgentWithClient(t *testing.T, diffs []model.Diff, resume *session.ResumeState, client llm.LLMClient) *Agent {
	t.Helper()
	setTestHome(t, t.TempDir())
	return newManifestFlowAgentInRepo(t, t.TempDir(), diffs, resume, client, nil)
}

func newManifestFlowAgentInRepo(t *testing.T, repoDir string, diffs []model.Diff, resume *session.ResumeState, client llm.LLMClient, configure func(*Args)) *Agent {
	t.Helper()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode:  session.ReviewModeRange,
		DiffFrom:    "main",
		DiffTo:      "feature",
		ResumedFrom: resumedFromSession(resume),
		Operation:   session.OperationReview,
	})
	t.Cleanup(func() { _ = sh.Finalize() })
	args := Args{
		RepoDir:    repoDir,
		From:       "main",
		To:         "feature",
		ReviewMode: session.ReviewModeRange,
		LLMClient:  client,
		Model:      "fake",
		Session:    sh,
		Resume:     resume,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "task_done",
				Description: "finish the review",
			},
		}},
	}
	if configure != nil {
		configure(&args)
	}
	a := New(args)
	a.diffs = diffs
	a.inputResolution.ResolvedBase = "base-parent"
	a.inputResolution.ResolvedHead = "head-parent"
	a.currentDate = "2026-07-26 12:00"
	return a
}

type relocationReviewClient func(context.Context, llm.RequestMeta, llm.ChatRequest) (*llm.ChatResponse, error)

func (f relocationReviewClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, _ := llm.RequestMetaFromContext(ctx)
	return f(ctx, meta, req)
}

func relocationReviewToolResponse(name, args string) *llm.ChatResponse {
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{
		ToolCalls: []llm.ToolCall{{
			ID: "call_" + name, Type: "function",
			Function: llm.FunctionCall{Name: name, Arguments: args},
		}},
	}}}}
}

func relocationReviewComment(path, code, content string) *llm.ChatResponse {
	args, _ := json.Marshal(map[string][]model.LlmComment{
		"comments": {{Path: path, ExistingCode: code, Content: content}},
	})
	return relocationReviewToolResponse("code_comment", string(args))
}

func configureRelocationReview(args *Args) {
	args.MaxConcurrency = 1
	args.CommentWorkerPool = NewCommentWorkerPool(2)
	args.Tools = tool.NewRegistry()
	args.Tools.Register(&tool.CodeCommentProvider{})
	args.MainToolDefs = append(args.MainToolDefs, llm.ToolDef{
		Type: "function", Function: llm.FunctionDef{Name: "code_comment"},
	})
	args.Template.ReviewFilterTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "{{path}}\n{{diff}}\n{{comments}}"}},
	}
}

func TestCrossFileRelocationReview(t *testing.T) {
	source := model.Diff{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -0,0 +2 @@\n+source()", Insertions: 1}
	target := model.Diff{OldPath: "b.go", NewPath: "b.go", Diff: "@@ -0,0 +7 @@\n+target()", Insertions: 1}
	finding := model.LlmComment{Path: "b.go", ExistingCode: "target()", Content: "Validate the target argument", StartLine: 7, EndLine: 7}

	testRelocationReviewSerial(t, source, target, finding)
	t.Run("two sources interleaved with the target", func(t *testing.T) {
		testRelocationReviewConcurrent(t, source, target, finding)
	})
	testRelocationReviewConditions(t, source, target, finding)
}

func testRelocationReviewSerial(t *testing.T, source, target model.Diff, finding model.LlmComment) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	diffs := []model.Diff{source, target}
	historical := finding
	historical.Content = "Existing finding from the parent"

	parent := newManifestFlowAgentInRepo(t, repoDir, diffs, nil, nil, nil)
	if err := parent.registerCoverage(diffs); err != nil {
		t.Fatal(err)
	}
	parent.markCompleted(target)
	fingerprint := reviewItemFingerprint(parent.reviewMode(), target)
	parent.session.RecordReviewItemDone(target.NewPath, target.OldPath, target.NewPath, fingerprint, []model.LlmComment{historical})
	state := assertRelocationReviewHistory(t, parent, []model.LlmComment{historical})

	for _, tc := range []struct {
		name   string
		diffs  []model.Diff
		resume *session.ResumeState
		remove bool
		want   []model.LlmComment
	}{
		{name: "target finishes first", diffs: []model.Diff{target, source}, want: []model.LlmComment{finding}},
		{name: "source finishes first", diffs: diffs, want: []model.LlmComment{finding}},
		{name: "keep new finding on reused target", diffs: diffs, resume: state, want: []model.LlmComment{historical, finding}},
		{name: "remove new finding on reused target", diffs: diffs, resume: state, remove: true, want: []model.LlmComment{historical}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var filterCalls atomic.Int64
			client := relocationReviewClient(func(_ context.Context, meta llm.RequestMeta, req llm.ChatRequest) (*llm.ChatResponse, error) {
				switch session.TaskType(meta.TaskType) {
				case session.MainTask:
					if meta.FilePath == source.NewPath && meta.RequestNo == 1 {
						return relocationReviewComment(source.NewPath, finding.ExistingCode, finding.Content), nil
					}
					if tc.resume != nil && meta.FilePath == target.NewPath {
						t.Error("reused target was dispatched")
					}
					return agentTaskDoneResponse(), nil
				case session.ReviewFilterTask:
					filterCalls.Add(1)
					ids := relocationReviewFilterIDs(t, meta, req, target.NewPath, []model.Diff{target}, finding)
					if tc.remove {
						return removeRelocationReviewFinding(ids[finding.ExistingCode]), nil
					}
					return relocationReviewToolResponse("approve_all_comments", "{}"), nil
				default:
					return nil, fmt.Errorf("unexpected request: %+v", meta)
				}
			})
			a := newManifestFlowAgentInRepo(t, repoDir, tc.diffs, tc.resume, client, configureRelocationReview)
			if tc.resume != nil {
				if err := tc.resume.ValidateResume(session.ResumeRequest{Identity: a.runIdentity(), Model: a.args.Model}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := a.dispatchSubtasks(t.Context())
			if err != nil || !slices.Equal(got, tc.want) {
				t.Errorf("output = %+v, error = %v, want %+v", got, err, tc.want)
			}
			if got := filterCalls.Load(); got != 1 {
				t.Errorf("filter calls = %d, want 1", got)
			}
			state := assertRelocationReviewHistory(t, a, tc.want)
			assertRelocationReviewResume(t, a, state, tc.want)
		})
	}
}

func removeRelocationReviewFinding(id string) *llm.ChatResponse {
	return relocationReviewToolResponse("report_incorrect_comments", fmt.Sprintf(`{"comment_ids":[%q]}`, id))
}

func relocationReviewFilterIDs(t *testing.T, meta llm.RequestMeta, req llm.ChatRequest, key string, diffs []model.Diff, want ...model.LlmComment) map[string]string {
	t.Helper()
	prompt, _ := req.Messages[0].Content.(string)
	raw, ok := strings.CutPrefix(prompt, key+"\n"+buildConcatenatedDiffs(diffs)+"\n")
	if meta.FilePath != key || !ok {
		t.Errorf("unexpected filter for %q: %s", meta.FilePath, prompt)
		return nil
	}
	var candidates []struct {
		model.LlmComment
		ID string
	}
	if err := json.Unmarshal([]byte(raw), &candidates); err != nil {
		t.Errorf("decode filter candidates: %v", err)
		return nil
	}
	ids := make(map[string]string, len(candidates))
	for _, c := range candidates {
		if !slices.ContainsFunc(want, func(cm model.LlmComment) bool {
			return cm.Path == c.Path && cm.Content == c.Content && cm.ExistingCode == c.ExistingCode
		}) {
			t.Errorf("unexpected filter candidate: %+v", c)
		}
		ids[c.ExistingCode] = c.ID
	}
	if len(candidates) != len(want) || len(ids) != len(want) {
		t.Errorf("filter candidates = %+v, want %+v", candidates, want)
	}
	return ids
}

func assertRelocationReviewHistory(t *testing.T, a *Agent, want []model.LlmComment) *session.ResumeState {
	t.Helper()
	finishManifestFlow(t, a)
	history, err := session.LoadComments(a.args.RepoDir, a.SessionID())
	if err != nil || !slices.Equal(history, want) {
		t.Errorf("history comments = %+v, error = %v, want %+v", history, err, want)
	}
	summary, err := session.LoadSummary(a.args.RepoDir, a.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalComments != len(want) {
		t.Errorf("summary comments = %d, want %d", summary.TotalComments, len(want))
	}
	state, err := session.LoadReviewResumeState(a.args.RepoDir, a.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func testRelocationReviewConcurrent(t *testing.T, source, target model.Diff, finding model.LlmComment) {
	setTestHome(t, t.TempDir())
	target.Diff = "@@ -0,0 +7,3 @@\n+target()\n+secondTarget()\n+localTarget()"
	target.Insertions = 3
	secondSource := model.Diff{OldPath: "c.go", NewPath: "c.go", Diff: "@@ -0,0 +3 @@\n+secondSource()", Insertions: 1}
	secondFinding := finding
	secondFinding.ExistingCode = "secondTarget()"
	secondFinding.StartLine, secondFinding.EndLine = 8, 8
	local := model.LlmComment{Path: target.NewPath, ExistingCode: "localTarget()", Content: "Target group's own finding", StartLine: 9, EndLine: 9}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	targetStarted := make(chan struct{})
	secondSourceStarted := make(chan struct{})
	targetFiltered := make(chan struct{})
	var filterCalls atomic.Int64
	client := relocationReviewClient(func(_ context.Context, meta llm.RequestMeta, req llm.ChatRequest) (*llm.ChatResponse, error) {
		switch session.TaskType(meta.TaskType) {
		case session.MainTask:
			if meta.RequestNo != 1 {
				return agentTaskDoneResponse(), nil
			}
			var ready <-chan struct{}
			var comment model.LlmComment
			switch meta.FilePath {
			case source.NewPath:
				ready, comment = targetStarted, finding
			case target.NewPath:
				close(targetStarted)
				ready, comment = secondSourceStarted, local
			case secondSource.NewPath:
				close(secondSourceStarted)
				ready, comment = targetFiltered, secondFinding
			default:
				return nil, fmt.Errorf("unexpected main request: %+v", meta)
			}
			select {
			case <-ready:
				return relocationReviewComment(meta.FilePath, comment.ExistingCode, comment.Content), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case session.ReviewFilterTask:
			if filterCalls.Add(1) == 1 {
				relocationReviewFilterIDs(t, meta, req, target.NewPath, []model.Diff{target}, local)
				close(targetFiltered)
				return relocationReviewToolResponse("approve_all_comments", "{}"), nil
			}
			ids := relocationReviewFilterIDs(t, meta, req, target.NewPath, []model.Diff{target}, finding, secondFinding)
			return removeRelocationReviewFinding(ids[finding.ExistingCode]), nil
		}
		return nil, fmt.Errorf("unexpected request: %+v", meta)
	})
	a := newManifestFlowAgentInRepo(t, t.TempDir(), []model.Diff{source, target, secondSource}, nil, client, func(args *Args) {
		configureRelocationReview(args)
		args.MaxConcurrency = 2
	})
	want := []model.LlmComment{local, secondFinding}
	got, err := a.dispatchSubtasks(ctx)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("concurrent output = %+v, error = %v, want %+v", got, err, want)
	}
	if got := filterCalls.Load(); got != 2 {
		t.Errorf("filter calls = %d, want 2", got)
	}
	state := assertRelocationReviewHistory(t, a, want)
	assertRelocationReviewResume(t, a, state, want)
}

func testRelocationReviewConditions(t *testing.T, source, target model.Diff, finding model.LlmComment) {
	setTestHome(t, t.TempDir())
	const budget int64 = 25000
	for _, tc := range []struct {
		name           string
		grouped        bool
		noFilter       bool
		noPrompt       bool
		exhausted      bool
		stopped        bool
		undispatched   bool
		deleted        bool
		runErr         error
		filterErr      error
		filterResponse *llm.ChatResponse
		calls          int64
	}{
		{name: "same group", grouped: true, calls: 1},
		{name: "no filter", noFilter: true},
		{name: "no filter template", noPrompt: true},
		{name: "cancelled", runErr: context.Canceled},
		{name: "budget exhausted", exhausted: true},
		{name: "provider error", filterErr: errors.New("filter rejected by provider"), calls: 1},
		{name: "filter timeout", filterErr: context.DeadlineExceeded, calls: 1},
		{name: "unparseable response", filterResponse: &llm.ChatResponse{}, calls: 1},
		{name: "target failed without findings", stopped: true, calls: 1},
		{name: "target not dispatched with budget remaining", undispatched: true, calls: 1},
		{name: "deleted target has no unique path", deleted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, want := target, finding
			diffs := []model.Diff{target, source}
			switch {
			case tc.undispatched:
				target.Diff += "\n+" + strings.Repeat("targetMayExceedTokenBudget ", 2000)
				diffs = []model.Diff{source, target}
			case tc.stopped:
				diffs = []model.Diff{source, target}
			case tc.deleted:
				unrelated := model.Diff{OldPath: "other.go", NewPath: "/dev/null", Diff: "@@ -1 +0,0 @@\n-unrelated()", IsDeleted: true}
				target.NewPath, target.Diff, target.IsDeleted = "/dev/null", "@@ -7 +0,0 @@\n-target()", true
				diffs = []model.Diff{unrelated, source, target}
				want.Path = target.NewPath
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls atomic.Int64
			client := relocationReviewClient(func(ctx context.Context, meta llm.RequestMeta, req llm.ChatRequest) (*llm.ChatResponse, error) {
				switch session.TaskType(meta.TaskType) {
				case session.MainTask:
					if meta.FilePath == target.NewPath {
						if tc.undispatched {
							t.Error("target was dispatched")
						}
						if tc.stopped {
							return &llm.ChatResponse{}, nil
						}
						return agentTaskDoneResponse(), nil
					}
					if meta.RequestNo == 1 {
						return relocationReviewComment(source.NewPath, finding.ExistingCode, finding.Content), nil
					}
					if tc.runErr != nil {
						cancel()
						return nil, ctx.Err()
					}
					resp := agentTaskDoneResponse()
					if tc.exhausted {
						resp.Usage = &llm.UsageInfo{PromptTokens: budget - 15}
					}
					return resp, nil
				case session.ReviewFilterTask:
					calls.Add(1)
					if tc.grouped {
						relocationReviewFilterIDs(t, meta, req, "a.go,b.go", diffs, want)
					}
					if tc.filterErr != nil || tc.filterResponse != nil {
						return tc.filterResponse, tc.filterErr
					}
					return relocationReviewToolResponse("approve_all_comments", "{}"), nil
				}
				return nil, fmt.Errorf("unexpected request: %+v", meta)
			})
			a := newManifestFlowAgentInRepo(t, t.TempDir(), diffs, nil, client, func(args *Args) {
				configureRelocationReview(args)
				args.SkipFilter = tc.noFilter
				if tc.exhausted || tc.undispatched {
					args.MaxTokensBudget = budget
				}
				if tc.noPrompt {
					args.Template.ReviewFilterTask = nil
				}
				if tc.grouped {
					args.Template.GroupingMinFiles, args.Template.GroupingBundleLineThreshold = 3, 10
				}
			})
			got, err := a.dispatchSubtasks(ctx)
			if !errors.Is(err, tc.runErr) || !slices.Equal(got, []model.LlmComment{want}) {
				t.Errorf("output = %+v, error = %v, want %+v and %v", got, err, want, tc.runErr)
			}
			if calls.Load() != tc.calls {
				t.Errorf("filter calls = %d, want %d", calls.Load(), tc.calls)
			}
			if tc.stopped || tc.undispatched {
				state := assertRelocationReviewHistory(t, a, nil)
				if _, reusable := state.ReusableItem(reviewItemFingerprint(a.reviewMode(), target)); reusable {
					t.Error("target has a reusable checkpoint")
				}
			} else if tc.runErr != nil {
				assertRelocationReviewHistory(t, a, []model.LlmComment{want})
			}
		})
	}
}

func assertRelocationReviewResume(t *testing.T, parent *Agent, state *session.ResumeState, want []model.LlmComment) {
	t.Helper()
	client := relocationReviewClient(func(_ context.Context, meta llm.RequestMeta, _ llm.ChatRequest) (*llm.ChatResponse, error) {
		t.Errorf("unexpected request on full resume: %+v", meta)
		return nil, errors.New("unexpected request on full resume")
	})
	continued := newManifestFlowAgentInRepo(t, parent.args.RepoDir, parent.diffs, state, client, configureRelocationReview)
	if err := state.ValidateResume(session.ResumeRequest{Identity: continued.runIdentity(), Model: continued.args.Model}); err != nil {
		t.Fatalf("validate compatible resume: %v", err)
	}
	got, err := continued.dispatchSubtasks(t.Context())
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("resume comments = %+v, error = %v, want %+v", got, err, want)
	}
	if info := continued.ResumeInfo(); info == nil || info.ReusedFiles != int64(len(parent.diffs)) || info.RerunFiles != 0 {
		t.Errorf("resume info = %+v, want all files reused", info)
	}
	finishManifestFlow(t, continued)
}

// seedParentManifest gives a hand-built ResumeState the parent manifest a real
// resume always has, matching the identity a already resolved. Without one the
// agent reuses nothing at all: dispatch re-verifies the input against the parent
// manifest, and only fingerprints the parent's own coverage settled are eligible.
//
// It must be called once a.diffs is final — the identity hashes exactly that set.
func seedParentManifest(t *testing.T, a *Agent, fingerprints ...string) {
	t.Helper()
	resume := a.args.Resume
	if resume == nil {
		t.Fatal("seedParentManifest needs an agent built with a resume state")
	}
	id := a.runIdentity()
	items := make([]session.CoverageItem, 0, len(fingerprints))
	for _, fp := range fingerprints {
		items = append(items, session.CoverageItem{ItemID: fp, Fingerprint: fp})
	}
	resume.Closed = true
	resume.Manifest = &session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		RunID:         resume.SessionID,
		Operation:     session.OperationReview,
		Repository:    session.ManifestRepository{IdentitySHA256: id.RepositorySHA256},
		Input:         session.ManifestInput{Mode: id.Mode, SourceArtifactSHA256: id.SourceArtifactSHA256},
		Execution:     session.ManifestExecution{RuleConfigSHA256: id.RuleConfigSHA256},
		Coverage:      session.Coverage{Selected: items, Completed: items},
	}
}

func finishManifestFlow(t *testing.T, a *Agent) *session.RunManifest {
	t.Helper()
	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	if err := a.session.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil {
		t.Fatal("expected frozen manifest")
	}
	return manifest
}

func TestManifestFlowCompleteAndPartial(t *testing.T) {
	t.Run("complete with zero findings", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1}}, nil)
		comments, err := a.dispatchSubtasks(context.Background())
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if len(comments) != 0 {
			t.Fatalf("comments = %d, want 0", len(comments))
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StateComplete || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
			t.Fatalf("manifest = %+v", manifest)
		}
		if manifest.Execution.MaxToolRequestTimes != 5 {
			t.Fatalf("max_tool_request_times = %d, want 5", manifest.Execution.MaxToolRequestTimes)
		}
	})

	t.Run("mixed provider result is partial", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{
			{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1},
			{OldPath: "bad.go", NewPath: "bad.go", Diff: "+bad", Insertions: 1},
		}, nil)
		if _, err := a.dispatchSubtasks(context.Background()); err != nil {
			t.Fatalf("mixed dispatch must continue: %v", err)
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StatePartial || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 1 {
			t.Fatalf("manifest = %+v", manifest)
		}
		if manifest.Coverage.Failed[0].Classification != session.FailureProvider {
			t.Fatalf("failure = %+v", manifest.Coverage.Failed[0])
		}
		if strings.Contains(manifest.Coverage.Failed[0].Reason, "LEAKED") || strings.Contains(manifest.Coverage.Failed[0].Reason, "/Users/") {
			t.Fatalf("manifest exposed raw provider error: %+v", manifest.Coverage.Failed[0])
		}
	})
}

// Two paths that differ only in separator normalize to one item_id. When
// selection excludes one and selects the other, the excluded record must give
// way: the run keeps its full denominator and still ends complete. An unrelated
// exclusion is kept.
func TestManifestFlowExcludedItemIDCollisionKeepsSelected(t *testing.T) {
	a := newManifestFlowAgent(t, nil, nil)
	decisions := a.selectFiles([]model.Diff{
		{OldPath: `src\a.go`, NewPath: `src\a.go`, IsBinary: true},
		{OldPath: "src/a.go", NewPath: "src/a.go", Diff: "+ok", Insertions: 1},
		{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1},
		{OldPath: "blob.go", NewPath: "blob.go", IsBinary: true},
	})
	if err := a.registerExclusions(decisions); err != nil {
		t.Fatalf("registerExclusions: %v", err)
	}
	a.diffs, _ = summarizeSelection(decisions)
	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StateComplete || manifest.RunFailure != nil {
		t.Fatalf("terminal = %q, run_failure = %+v", manifest.TerminalState, manifest.RunFailure)
	}
	if len(manifest.Coverage.Selected) != 2 || len(manifest.Coverage.Completed) != 2 {
		t.Fatalf("coverage = %+v", manifest.Coverage)
	}
	ex := manifest.Coverage.Excluded
	if len(ex) != 1 || ex[0].Path != "blob.go" || ex[0].ExcludeReason != model.ExcludeBinary {
		t.Fatalf("excluded = %+v, want only blob.go: the colliding record is dropped", ex)
	}
}

func TestManifestFlowCancellationPersistsResumableSession(t *testing.T) {
	done := model.Diff{OldPath: "done.go", NewPath: "done.go", Diff: "+done", Insertions: 1}
	blocked := model.Diff{OldPath: "blocked.go", NewPath: "blocked.go", Diff: "+blocked", Insertions: 1}
	pending := model.Diff{OldPath: "pending.go", NewPath: "pending.go", Diff: "+pending", Insertions: 1}
	client := &cancellationFlowClient{blocked: make(chan struct{})}
	a := newManifestFlowAgentWithClient(t, []model.Diff{done, blocked, pending}, nil, client)
	a.args.MaxConcurrency = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchErr := make(chan error, 1)
	go func() {
		_, err := a.dispatchSubtasks(ctx)
		dispatchErr <- err
	}()

	select {
	case <-client.blocked:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("blocked review did not start")
	}
	if err := <-dispatchErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", err)
	}

	manifest := finishManifestFlow(t, a)
	if manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureCancelled {
		t.Fatalf("run failure = %+v, want cancelled", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 2 {
		t.Fatalf("coverage = %+v, want one completed and two failed", manifest.Coverage)
	}
	for _, item := range manifest.Coverage.Failed {
		if item.Classification != session.FailureCancelled {
			t.Fatalf("failed item = %+v, want cancelled", item)
		}
	}

	state, err := session.LoadReviewResumeState(a.args.RepoDir, a.session.SessionID)
	if err != nil {
		t.Fatalf("load cancelled session: %v", err)
	}
	identity := session.RunIdentity{
		Mode:                 manifest.Input.Mode,
		ResolvedBase:         manifest.Input.ResolvedBase,
		ResolvedHead:         manifest.Input.ResolvedHead,
		SourceArtifactSHA256: manifest.Input.SourceArtifactSHA256,
		RuleConfigSHA256:     manifest.Execution.RuleConfigSHA256,
		RepositorySHA256:     manifest.Repository.IdentitySHA256,
	}
	if err := state.ValidateResume(session.ResumeRequest{
		Identity: identity,
		Provider: manifest.Execution.Provider,
		Model:    manifest.Execution.Model,
	}); err != nil {
		t.Fatalf("cancelled session should remain resumable: %v", err)
	}
	if _, ok := state.ReusableItem(reviewItemFingerprint(a.reviewMode(), done)); !ok {
		t.Fatal("completed item is not reusable after cancellation")
	}
	if _, ok := state.ReusableItem(reviewItemFingerprint(a.reviewMode(), blocked)); ok {
		t.Fatal("cancelled item must be reviewed again")
	}
}

func TestManifestFlowCancellationBeforeDispatchStartsNoSubtask(t *testing.T) {
	pending := model.Diff{OldPath: "blocked.go", NewPath: "blocked.go", Diff: "+blocked", Insertions: 1}
	client := &cancellationFlowClient{blocked: make(chan struct{})}
	a := newManifestFlowAgentWithClient(t, []model.Diff{pending}, nil, client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.dispatchSubtasks(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", err)
	}
	if got := atomic.LoadInt64(&a.subtaskFailed); got != 0 {
		t.Fatalf("subtask failures = %d, want 0 because no subtask should start", got)
	}
	select {
	case <-client.blocked:
		t.Fatal("LLM client was called after cancellation")
	default:
	}

	manifest := finishManifestFlow(t, a)
	if manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureCancelled {
		t.Fatalf("run failure = %+v, want cancelled", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 0 || len(manifest.Coverage.Failed) != 1 {
		t.Fatalf("coverage = %+v, want one cancelled item and no completed items", manifest.Coverage)
	}
	if got := manifest.Coverage.Failed[0].Classification; got != session.FailureCancelled {
		t.Fatalf("failure classification = %q, want %q", got, session.FailureCancelled)
	}
}

func TestManifestFlowRunInputFailureIsPersisted(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "missing-base",
		DiffTo:     "missing-head",
		Operation:  session.OperationReview,
	})
	a := New(Args{
		RepoDir:    repoDir,
		From:       "missing-base",
		To:         "missing-head",
		ReviewMode: session.ReviewModeRange,
		GitRunner:  gitcmd.New(1),
		LLMClient:  manifestFlowClient{},
		Model:      "fake",
		Session:    sh,
	})

	if _, err := a.Run(context.Background()); err == nil {
		t.Fatal("invalid review input must fail")
	}
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateFailed || manifest.RunFailure == nil {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.RunFailure.Classification != session.RunFailureInput || len(manifest.Coverage.Selected) != 0 {
		t.Fatalf("run failure = %+v, coverage = %+v", manifest.RunFailure, manifest.Coverage)
	}

	summary, err := session.LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("load persisted summary: %v", err)
	}
	if summary.RunManifest == nil || summary.RunManifest.TerminalState != session.StateFailed || summary.Aborted {
		t.Fatalf("persisted summary = %+v", summary)
	}
}

func TestManifestFlowAllFailedTimeoutAndPanic(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		ctx       func() context.Context
		wantClass session.FailureClass
	}{
		{name: "provider", path: "bad.go", ctx: context.Background, wantClass: session.FailureProvider},
		{name: "timeout", path: "timeout.go", ctx: func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			t.Cleanup(cancel)
			return ctx
		}, wantClass: session.FailureTimeout},
		{name: "panic", path: "panic.go", ctx: context.Background, wantClass: session.FailurePanic},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newManifestFlowAgent(t, []model.Diff{{OldPath: tc.path, NewPath: tc.path, Diff: "+x", Insertions: 1}}, nil)
			if _, err := a.dispatchSubtasks(tc.ctx()); err == nil {
				t.Fatal("all-failed dispatch must return an error")
			}
			manifest := finishManifestFlow(t, a)
			if manifest.TerminalState != session.StateFailed || manifest.RunFailure != nil || len(manifest.Coverage.Failed) != 1 {
				t.Fatalf("manifest = %+v", manifest)
			}
			if got := manifest.Coverage.Failed[0].Classification; got != tc.wantClass {
				t.Fatalf("classification = %q, want %q", got, tc.wantClass)
			}
		})
	}
}

// A single item failing by timeout, panic, or budget must stay isolated: the run
// is partial (not failed), the sibling still completes, and run_failure is nil —
// per-item outcomes never escalate to a run-level failure.
func TestManifestFlowMixedFailureIsIsolatedToPartial(t *testing.T) {
	// The budget item's diff stays under the diff-size pre-filter (80% of
	// MaxTokens), while its long path inflates the rendered prompt past the same
	// threshold — so only this file trips the budget stop, without touching the
	// shared template that its sibling also renders.
	longPath := strings.Repeat("nested/", 20) + "budget.go"
	for _, tc := range []struct {
		name      string
		failDiff  model.Diff
		setup     func(*Agent)
		wantClass session.FailureClass
	}{
		{
			name:      "timeout",
			failDiff:  model.Diff{OldPath: "slow.go", NewPath: "slow.go", Diff: "+slow", Insertions: 1},
			wantClass: session.FailureTimeout,
		},
		{
			name:      "panic",
			failDiff:  model.Diff{OldPath: "panic.go", NewPath: "panic.go", Diff: "+boom", Insertions: 1},
			wantClass: session.FailurePanic,
		},
		{
			name:      "budget",
			failDiff:  model.Diff{OldPath: longPath, NewPath: longPath, Diff: strings.Repeat("token ", 50), Insertions: 1},
			setup:     func(a *Agent) { a.args.Template.MaxTokens = 100 },
			wantClass: session.FailureBudget,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			good := model.Diff{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1}
			a := newManifestFlowAgent(t, []model.Diff{good, tc.failDiff}, nil)
			if tc.setup != nil {
				tc.setup(a)
			}
			// One item's failure must not fail the whole run: dispatch continues.
			if _, err := a.dispatchSubtasks(context.Background()); err != nil {
				t.Fatalf("mixed dispatch must continue past a single-item failure: %v", err)
			}
			manifest := finishManifestFlow(t, a)
			if manifest.TerminalState != session.StatePartial {
				t.Fatalf("terminal = %q, want partial; manifest = %+v", manifest.TerminalState, manifest)
			}
			if manifest.RunFailure != nil {
				t.Fatalf("item-level failure must not set run_failure: %+v", manifest.RunFailure)
			}
			if len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 1 {
				t.Fatalf("coverage = %+v", manifest.Coverage)
			}
			if got := manifest.Coverage.Failed[0].Classification; got != tc.wantClass {
				t.Fatalf("classification = %q, want %q", got, tc.wantClass)
			}
		})
	}
}

func TestManifestFlowBudgetAndSkipped(t *testing.T) {
	t.Run("single-item budget stop", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{{OldPath: "budget.go", NewPath: "budget.go", Diff: "+x", Insertions: 1}}, nil)
		a.args.Template.MaxTokens = 100
		a.args.Template.MainTask.Messages[0].Content = strings.Repeat("context ", 200) + "{{diffs}}"
		if _, err := a.dispatchSubtasks(context.Background()); err != nil {
			t.Fatalf("business stop is represented by coverage, not a dispatch error: %v", err)
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StateFailed || len(manifest.Coverage.Failed) != 1 || manifest.Coverage.Failed[0].Classification != session.FailureBudget {
			t.Fatalf("manifest = %+v", manifest)
		}
	})

	// Driven through Run, not dispatchSubtasks: Run is where the size gate is
	// applied, so only this path shows that an all-oversized changeset really
	// reaches a skipped manifest rather than being dispatched.
	t.Run("all oversized is skipped", func(t *testing.T) {
		dir := initPreviewRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "large.go"), []byte(strings.Repeat("word ", 500)), 0o644); err != nil {
			t.Fatalf("write large.go: %v", err)
		}
		a := New(Args{
			RepoDir:   dir,
			LLMClient: manifestFlowClient{},
			Model:     "fake",
			Template:  template.Template{MaxTokens: 10},
		})
		comments, err := a.Run(context.Background())
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(comments) != 0 {
			t.Fatalf("comments = %d, want 0", len(comments))
		}
		manifest := a.RunManifest()
		if manifest == nil || manifest.TerminalState != session.StateSkipped || len(manifest.Coverage.Selected) != 0 {
			t.Fatalf("manifest = %+v", manifest)
		}
		excluded := manifest.Coverage.Excluded
		if len(excluded) != 1 || excluded[0].Path != "large.go" || excluded[0].ExcludeReason != model.ExcludeTooLarge {
			t.Fatalf("excluded = %+v, want large.go as too_large", excluded)
		}
	})
}

// groupedBudgetPartialClient plays two roles for one grouped-review round trip:
// the grouping call (no Tools on the request) always merges every file into one
// group, and the main-task call (Tools present) files a comment for the first
// round then keeps returning an unregistered tool name — a valid, non-empty
// result that is neither task_done nor an empty round — until the round budget
// is exhausted by the caller's MaxToolRequestTimes.
type groupedBudgetPartialClient struct {
	round int32 // atomic
}

func (c *groupedBudgetPartialClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Tools) == 0 {
		content := `[{"label":"pair","files":[0,1]}]`
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
			Model:   "fake",
		}, nil
	}
	if atomic.AddInt32(&c.round, 1) == 1 {
		return &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					ToolCalls: []llm.ToolCall{{
						ID:   "1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "code_comment",
							Arguments: `{"comments":[{"path":"a.go","content":"missing a null check here"}]}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Model: "fake",
		}, nil
	}
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				ToolCalls: []llm.ToolCall{{
					ID:       "x",
					Type:     "function",
					Function: llm.FunctionCall{Name: "keep_exploring", Arguments: "{}"},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
	}, nil
}

// A group that never calls task_done before exhausting its round budget must
// classify coverage per file, not per group: a.go already has a comment from
// round 1, so it belongs in Completed even though the shared conversation
// stopped on StopMaxRounds; b.go never got a comment, so it stays Failed. The
// mixed outcome must read as partial (exit 0), not failed (exit 1) — the
// bitcoin__bitcoin regression this guards against had exactly one file with
// output and one without in the same stuck group, and the run reported "2 of 2
// selected item(s) failed" despite the real comment.
func TestManifestFlowGroupBudgetStopIsPartialWhenSomeFilesHaveComments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	diffs := []model.Diff{
		{OldPath: "a.go", NewPath: "a.go", Diff: "+a", Insertions: 1},
		{OldPath: "b.go", NewPath: "b.go", Diff: "+b", Insertions: 1},
	}
	client := &groupedBudgetPartialClient{}
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Operation:  session.OperationReview,
	})
	// code_comment is only wired up if a CodeCommentProvider is registered in
	// Tools — the loop looks the provider up by name (loop.go:558) before it
	// ever reaches the code_comment-specific handling, even though it is a
	// built-in tool name.
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	// MaxToolRequestTimes and GroupingTask must be set before New(), since
	// reviewtask.Runner snapshots Template by value at construction time — mutating
	// a.args.Template afterward (as other tests in this file do for MaxTokens,
	// which executeGroupSubtask re-reads from a.args.Template directly) would
	// not reach the runner's copy that RunMainTask actually consumes.
	a := New(Args{
		RepoDir:          repoDir,
		From:             "main",
		To:               "feature",
		ReviewMode:       session.ReviewModeRange,
		LLMClient:        client,
		Model:            "fake",
		Session:          sh,
		CommentCollector: collector,
		Tools:            reg,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 3,
			GroupingTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
			},
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "finish the review"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "file a review comment"}},
		},
	})
	a.diffs = diffs
	a.currentDate = "2026-07-26 12:00"

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("a stuck group with partial output must not be a dispatch error: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StatePartial {
		t.Fatalf("terminal = %q, want partial; manifest = %+v", manifest.TerminalState, manifest)
	}
	if manifest.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil for a per-item budget stop", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 1 || manifest.Coverage.Completed[0].Path != "a.go" {
		t.Fatalf("completed coverage = %+v, want exactly a.go", manifest.Coverage.Completed)
	}
	if len(manifest.Coverage.Failed) != 1 || manifest.Coverage.Failed[0].Path != "b.go" {
		t.Fatalf("failed coverage = %+v, want exactly b.go", manifest.Coverage.Failed)
	}
	if manifest.Coverage.Failed[0].Classification != session.FailureBudget {
		t.Fatalf("b.go classification = %q, want budget", manifest.Coverage.Failed[0].Classification)
	}
}

func TestManifestFlowResumeRecordsParentAndReusedItem(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
		{OldPath: "fresh.go", NewPath: "fresh.go", Diff: "+fresh", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
			},
		},
	}
	a := newManifestFlowAgent(t, diffs, resume)
	seedParentManifest(t, a, fingerprint)
	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.ParentRunID != "parent-run" || len(manifest.Coverage.Reused) != 1 || len(manifest.Coverage.Completed) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.Input.RequestedFrom != "main" || manifest.Input.RequestedHead != "feature" || manifest.Input.SourceArtifactSHA256 == "" {
		t.Fatalf("child input identity = %+v", manifest.Input)
	}
}

// promptSpy answers every request the same way and keeps the prompts, so a test
// can assert what never reached the model.
type promptSpy struct {
	mu      sync.Mutex
	prompts []string
}

func (p *promptSpy) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	p.mu.Lock()
	p.prompts = append(p.prompts, prompt)
	p.mu.Unlock()
	return agentTaskDoneResponse(), nil
}

// TestManifestFlowReusedCommentsStayOutOfPrompts pins the isolation half of
// reuse: a reused finding is merged into the output and nothing else. Feeding it
// back into a prompt would let the parent's conclusions steer the child's review
// of a different file, and the finding would also be paid for twice.
func TestManifestFlowReusedCommentsStayOutOfPrompts(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
		{OldPath: "fresh.go", NewPath: "fresh.go", Diff: "+fresh", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	const reusedFinding = "PARENT_FINDING_ABOUT_CACHED_GO"
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
				Comments:    []model.LlmComment{{Path: "cached.go", Content: reusedFinding}},
			},
		},
	}
	spy := &promptSpy{}
	a := newManifestFlowAgentWithClient(t, diffs, resume, spy)
	seedParentManifest(t, a, fingerprint)

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	var merged bool
	for _, c := range comments {
		if c.Content == reusedFinding {
			merged = true
		}
	}
	if !merged {
		t.Fatalf("reused finding must be merged into the output, got %+v", comments)
	}
	// One dispatch, for fresh.go only — cached.go was reused, so it is neither
	// re-reviewed nor mentioned to the model.
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.prompts) != 1 {
		t.Fatalf("prompts = %d, want 1 (fresh.go only)", len(spy.prompts))
	}
	for _, prompt := range spy.prompts {
		if strings.Contains(prompt, reusedFinding) {
			t.Error("a reused finding must not enter a new LLM context")
		}
		if strings.Contains(prompt, "cached.go") {
			t.Error("a reused file must not be sent to the model again")
		}
	}
}

// TestManifestFlowResumeIgnoresCheckpointTheParentManifestDoesNotVouchFor covers
// a checkpoint line whose fingerprint the parent's coverage never settled. The
// manifest is the single source of truth for coverage, so the line is not
// evidence of anything and its file is reviewed again.
func TestManifestFlowResumeIgnoresCheckpointTheParentManifestDoesNotVouchFor(t *testing.T) {
	diffs := []model.Diff{{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1}}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {FilePath: "cached.go", NewPath: "cached.go", Fingerprint: fingerprint},
		},
	}
	a := newManifestFlowAgent(t, diffs, resume)
	// A manifest that settled some *other* item: identity still matches, so the
	// resume is admitted, but this fingerprint is not in completed or reused.
	seedParentManifest(t, a, "fp-some-other-item")

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if len(manifest.Coverage.Reused) != 0 || len(manifest.Coverage.Completed) != 1 {
		t.Fatalf("coverage reused=%d completed=%d, want 0/1 — the unvouched checkpoint must be re-reviewed",
			len(manifest.Coverage.Reused), len(manifest.Coverage.Completed))
	}
}

// TestManifestFlowResumeOfFullyFailedParentRedispatchesEverything pins the case
// resume exists for. The parent's every item failed, so replay left no
// checkpoint behind and its coverage vouches for nothing: each selected item is
// reviewed again, and the child settles them itself.
func TestManifestFlowResumeOfFullyFailedParentRedispatchesEverything(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "one.go", NewPath: "one.go", Diff: "+one", Insertions: 1},
		{OldPath: "two.go", NewPath: "two.go", Diff: "+two", Insertions: 1},
	}
	// No Items: every review_item_done the parent wrote was retracted by the
	// review_item_failed that followed it.
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items:      map[string]session.ResumeItem{},
	}

	a := newManifestFlowAgent(t, diffs, resume)
	fingerprints := make([]string, 0, len(diffs))
	for _, d := range diffs {
		fingerprints = append(fingerprints, reviewItemFingerprint(session.ReviewModeRange, d))
	}
	seedParentManifest(t, a, fingerprints...)
	// Same selection, but the parent completed none of it.
	cov := &a.args.Resume.Manifest.Coverage
	cov.Failed, cov.Completed = cov.Completed, nil

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if len(manifest.Coverage.Reused) != 0 || len(manifest.Coverage.Completed) != len(diffs) {
		t.Fatalf("coverage reused=%d completed=%d, want 0/%d — a fully failed parent must re-dispatch everything",
			len(manifest.Coverage.Reused), len(manifest.Coverage.Completed), len(diffs))
	}
}

func TestManifestFlowResumeWithReusedAndAllRerunsFailedIsPartial(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
		{OldPath: "bad.go", NewPath: "bad.go", Diff: "+bad", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
			},
		},
	}

	a := newManifestFlowAgent(t, diffs, resume)
	seedParentManifest(t, a, fingerprint)
	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("partial resumed dispatch must not return an error: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StatePartial || len(manifest.Coverage.Reused) != 1 || len(manifest.Coverage.Failed) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

// TestManifestFlowResumeWithProviderTransition verifies that a resumed run
// records the *current* provider/model in execution (not the parent's) while
// still linking to the parent via parent_run_id, so a consumer can audit that
// the provider or model changed across a resume chain.
func TestManifestFlowResumeWithProviderTransition(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	// The parent run used a different provider/model; ResumeState only carries
	// the session id and fingerprint, never the parent's execution metadata.
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
			},
		},
	}

	a := newManifestFlowAgent(t, diffs, resume)
	seedParentManifest(t, a, fingerprint)
	// Simulate a provider/model transition on the child run. New() already
	// called initManifest with the default "fake" model; re-seed execution so
	// the frozen manifest reflects the child's actual provider/model.
	a.args.Provider = "beta"
	a.args.Model = "model-b"
	a.initManifest()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)

	// The child manifest records the *current* provider/model, so a consumer
	// can audit that a transition occurred across the resume chain.
	if manifest.Execution.Provider != "beta" || manifest.Execution.Model != "model-b" {
		t.Fatalf("execution = %+v, want provider=beta model=model-b", manifest.Execution)
	}
	// parent_run_id links to the parent, making the transition traceable.
	if manifest.ParentRunID != "parent-run" {
		t.Fatalf("parent_run_id = %q, want parent-run", manifest.ParentRunID)
	}
	// The fingerprint hit is recorded as reused; the child recomputes its
	// own input identity (source_artifact_sha256).
	if len(manifest.Coverage.Reused) != 1 || len(manifest.Coverage.Completed) != 0 {
		t.Fatalf("coverage reused=%d completed=%d, want 1/0",
			len(manifest.Coverage.Reused), len(manifest.Coverage.Completed))
	}
	if manifest.Input.SourceArtifactSHA256 == "" {
		t.Fatal("child source_artifact_sha256 must be populated")
	}
}
