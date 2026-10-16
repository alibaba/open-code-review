// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func submitTUIReviewFlow(m reviewTUIModel, line string) (reviewTUIModel, tea.Cmd) {
	m.input.SetValue(line)
	return pressTUI(m, tea.KeyEnter)
}

// Execute the service command and deliver its completion as Bubble Tea would,
// without waiting for the independent animation tick in the same batch.
func finishTUIReviewFlow(t *testing.T, m reviewTUIModel, cmd tea.Cmd) reviewTUIModel {
	t.Helper()
	if cmd == nil || !m.busy {
		t.Fatal("service request did not start an active task")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatal("service request did not schedule its completion")
	}
	next, _ := m.Update(batch[0]())
	m = next.(reviewTUIModel)
	if m.busy {
		t.Fatal("completion left the task active")
	}
	return m
}

func TestTUIReviewFlowRecoversPreviewAndUsesChangedScope(t *testing.T) {
	m := testReviewTUI()
	var previews, reviews []reviewOptions
	m.services.preview = func(ctx context.Context, opts reviewOptions) (model.Preview, error) {
		previews = append(previews, opts)
		if len(previews) == 1 {
			return model.Preview{}, errors.New("diff input failed\nunknown revision: missing-base")
		}
		return model.Preview{
			TotalFiles: 2, ReviewableCount: 1, ExcludedCount: 1,
			TotalInsertions: 4, TotalDeletions: 1,
			Entries: []model.PreviewEntry{
				{Path: "changed.go", Status: "modified", WillReview: true, Insertions: 4, Deletions: 1},
				{Path: "generated.go", Status: "added", ExcludeReason: model.ExcludeUserRule},
			},
		}, nil
	}
	m.services.review = func(ctx context.Context, opts reviewOptions, progress func(string)) (jsonOutput, error) {
		reviews = append(reviews, opts)
		progress("Reviewing the selected commit")
		return jsonOutput{
			Status: "complete", SessionID: "commit-run",
			Comments: []model.LlmComment{{Path: "committed.go", StartLine: 8, Content: "Check the returned error."}},
		}, nil
	}

	m, cmd := submitTUIReviewFlow(m, "/preview")
	m = finishTUIReviewFlow(t, m, cmd)
	if m.screen != tuiPreview || !m.isError || !strings.Contains(m.View().Content, "unknown revision: missing-base") {
		t.Fatal("preview failure was not displayed")
	}
	if strings.Contains(m.content(), "0 reviewable files") {
		t.Fatal("failed preview advertised an empty successful scope")
	}

	m, _ = submitTUIReviewFlow(m, "/scope range main feature")
	m, cmd = submitTUIReviewFlow(m, "/preview")
	m = finishTUIReviewFlow(t, m, cmd)
	if len(previews) != 2 || previews[1].from != "main" || previews[1].to != "feature" || previews[1].commit != "" {
		t.Fatalf("preview did not use the changed range: %+v", previews)
	}
	if m.previewError != "" || m.isError || !strings.Contains(m.content(), "1 reviewable files, 1 excluded") ||
		!strings.Contains(m.content(), "changed.go") || !strings.Contains(m.content(), "generated.go") {
		t.Fatal("successful preview retained the failure or lost selected/excluded files")
	}

	m, _ = submitTUIReviewFlow(m, "/background Verify the committed retry fix")
	m, _ = submitTUIReviewFlow(m, "/scope commit abc123")
	m, cmd = submitTUIReviewFlow(m, "/review")
	m = finishTUIReviewFlow(t, m, cmd)
	if len(reviews) != 1 || reviews[0].commit != "abc123" || reviews[0].from != "" || reviews[0].to != "" ||
		reviews[0].resume != "" || reviews[0].background != "Verify the committed retry fix" {
		t.Fatalf("review reused the preview's former scope or lost background: %+v", reviews)
	}
	if m.screen != tuiResults || m.result.SessionID != "commit-run" || !strings.Contains(m.View().Content, "committed.go:8") {
		t.Fatal("review completion did not display the current scope's findings")
	}
}

func TestTUIReviewFlowLoadsHistoryAndResumesRange(t *testing.T) {
	m := testReviewTUI()
	m.opts.commit, m.opts.provider, m.opts.model = "current-commit", "selected-provider", "selected-model"
	m.opts.background = "Keep the current requirement context"
	workspace := session.Summary{
		SessionID: "workspace-run", ReviewMode: session.ReviewModeWorkspace,
		StartTime:   time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC),
		RunManifest: &session.RunManifest{Operation: session.OperationReview, TerminalState: session.StateComplete},
	}
	savedRange := session.Summary{
		SessionID: "range-run", ReviewMode: session.ReviewModeRange, DiffFrom: "main", DiffTo: "saved-feature",
		Model: "old-model", TotalComments: 1,
		StartTime:   time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC),
		RunManifest: &session.RunManifest{Operation: session.OperationReview, TerminalState: session.StatePartial},
	}
	finding := model.LlmComment{Path: "saved.go", StartLine: 3, Content: "Keep this saved finding."}
	var loaded []string
	var previewOpts, reviewOpts reviewOptions
	m.services.sessions = func(repo string) ([]session.Summary, error) {
		if repo != m.opts.repoDir {
			t.Errorf("history requested a different repository: %s", repo)
		}
		return []session.Summary{workspace, savedRange}, nil
	}
	m.services.load = func(repo, id string) (*session.Summary, []model.LlmComment, error) {
		if repo != m.opts.repoDir {
			t.Errorf("session load requested a different repository: %s", repo)
		}
		loaded = append(loaded, id)
		if id == workspace.SessionID {
			return &workspace, nil, nil
		}
		return &savedRange, []model.LlmComment{finding}, nil
	}
	m.services.preview = func(ctx context.Context, opts reviewOptions) (model.Preview, error) {
		previewOpts = opts
		return model.Preview{TotalFiles: 1, ReviewableCount: 1, Entries: []model.PreviewEntry{{Path: "saved.go", WillReview: true}}}, nil
	}
	m.services.review = func(ctx context.Context, opts reviewOptions, progress func(string)) (jsonOutput, error) {
		reviewOpts = opts
		return jsonOutput{Status: "complete", SessionID: "resumed-run", Comments: []model.LlmComment{finding}}, nil
	}

	m, cmd := submitTUIReviewFlow(m, "/sessions")
	m = finishTUIReviewFlow(t, m, cmd)
	if m.screen != tuiSessions || len(m.sessions) != 2 || !strings.Contains(m.View().Content, "1 findings") {
		t.Fatal("session history did not display the saved runs")
	}
	m, cmd = pressTUI(m, tea.KeyEnter)
	m = finishTUIReviewFlow(t, m, cmd)
	if m.session == nil || m.session.SessionID != workspace.SessionID {
		t.Fatal("Enter did not open the selected workspace session")
	}
	m, cmd = submitTUIReviewFlow(m, "/resume")
	m = finishTUIReviewFlow(t, m, cmd)
	if !m.isError || !strings.Contains(m.notice, "workspace sessions can be viewed") ||
		m.opts.commit != "current-commit" || m.opts.resume != "" {
		t.Fatal("unsupported workspace resume changed the current review scope")
	}

	m, cmd = submitTUIReviewFlow(m, "/sessions")
	m = finishTUIReviewFlow(t, m, cmd)
	m, _ = pressTUI(m, tea.KeyDown)
	m, cmd = pressTUI(m, tea.KeyEnter)
	m = finishTUIReviewFlow(t, m, cmd)
	if m.session == nil || m.session.SessionID != savedRange.SessionID || m.result.SessionID != savedRange.SessionID ||
		!reflect.DeepEqual(m.result.Comments, []model.LlmComment{finding}) || !strings.Contains(m.content(), "saved.go:3") {
		t.Fatal("opening the range session lost its saved findings or persistence identity")
	}
	m, cmd = submitTUIReviewFlow(m, "/resume")
	m = finishTUIReviewFlow(t, m, cmd)
	if m.screen != tuiHome || m.isError || m.opts.from != "main" || m.opts.to != "saved-feature" ||
		m.opts.commit != "" || m.opts.resume != savedRange.SessionID {
		t.Fatal("resume did not restore the selected session's range")
	}

	m, cmd = submitTUIReviewFlow(m, "/preview")
	m = finishTUIReviewFlow(t, m, cmd)
	if previewOpts.from != "main" || previewOpts.to != "saved-feature" || previewOpts.resume != "" || m.opts.resume != savedRange.SessionID {
		t.Fatal("preview failed to inspect the resumed scope or cleared its checkpoint selection")
	}
	m, cmd = submitTUIReviewFlow(m, "/review")
	m = finishTUIReviewFlow(t, m, cmd)
	if reviewOpts.from != "main" || reviewOpts.to != "saved-feature" || reviewOpts.commit != "" || reviewOpts.resume != savedRange.SessionID ||
		reviewOpts.provider != "selected-provider" || reviewOpts.model != "selected-model" || reviewOpts.background != "Keep the current requirement context" {
		t.Fatalf("resumed review lost the historical range or current overrides: %+v", reviewOpts)
	}
	if m.result.SessionID != "resumed-run" || m.screen != tuiResults ||
		!reflect.DeepEqual(loaded, []string{workspace.SessionID, workspace.SessionID, savedRange.SessionID, savedRange.SessionID}) {
		t.Fatalf("history/resume completion did not retain the new run: result=%+v, loads=%v", m.result, loaded)
	}
}

func TestTUIReviewFlowReturnsToResultsFromHistory(t *testing.T) {
	for _, height := range []int{10, 40} {
		t.Run(fmt.Sprintf("height=%d", height), func(t *testing.T) {
			m := testReviewTUI()
			next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: height})
			m = next.(reviewTUIModel)
			finding := model.LlmComment{Path: "only.go", StartLine: 1, Content: "Preserved finding"}
			m.services.review = func(context.Context, reviewOptions, func(string)) (jsonOutput, error) {
				return jsonOutput{
					Status: "complete", SessionID: "latest-run", Comments: []model.LlmComment{finding},
					Manifest: &session.RunManifest{TerminalState: session.StateComplete},
				}, nil
			}
			m.services.sessions = func(string) ([]session.Summary, error) {
				return []session.Summary{{SessionID: "first"}, {SessionID: "second"}}, nil
			}

			m, cmd := submitTUIReviewFlow(m, "/review")
			m = finishTUIReviewFlow(t, m, cmd)
			m, cmd = submitTUIReviewFlow(m, "/sessions")
			m = finishTUIReviewFlow(t, m, cmd)
			m, _ = pressTUI(m, tea.KeyDown)
			if m.selected != 1 {
				t.Fatal("second saved session was not selected")
			}
			m, _ = pressTUI(m, tea.KeyPgDown)
			m, _ = submitTUIReviewFlow(m, "/results")
			if m.screen != tuiResults || !strings.Contains(m.viewport.View(), "> only.go:1") {
				t.Fatal("returning to results did not display the selected finding")
			}
			if m.result.SessionID != "latest-run" || !reflect.DeepEqual(m.result.Comments, []model.LlmComment{finding}) {
				t.Fatal("browsing history replaced the latest review result")
			}
			m, _ = pressTUI(m, tea.KeyEnter)
			if m.screen != tuiFinding || !strings.Contains(m.viewport.View(), finding.Content) {
				t.Fatal("Enter did not open the preserved finding")
			}
		})
	}
}
