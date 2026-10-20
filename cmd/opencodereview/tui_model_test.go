// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func testReviewTUI() reviewTUIModel {
	return newReviewTUI(context.Background(), reviewOptions{repoDir: "/repo", outputFormat: "json", audience: "human"}, tuiServices{}, false)
}

func TestTUIBackgroundAndScopeDoNotStartReview(t *testing.T) {
	m := testReviewTUI()
	next, cmd := m.command("Fix retry handling and preserve partial results")
	m = next.(reviewTUIModel)
	if cmd != nil || m.busy || m.opts.background != "Fix retry handling and preserve partial results" {
		t.Fatalf("setting background started work or lost context: %+v", m.opts)
	}
	next, _ = m.command("/scope range main HEAD")
	m = next.(reviewTUIModel)
	if m.opts.from != "main" || m.opts.to != "HEAD" {
		t.Fatal("range was not selected")
	}
	next, _ = m.command("/scope range main")
	m = next.(reviewTUIModel)
	if !m.isError || m.opts.from != "main" || m.opts.to != "HEAD" {
		t.Fatal("invalid scope replaced the current selection")
	}
	m.opts.resume = "old-session"
	next, _ = m.command("/scope commit abc123")
	m = next.(reviewTUIModel)
	if m.opts.commit != "abc123" || m.opts.from != "" || m.opts.to != "" || m.opts.resume != "" {
		t.Fatal("changing scope retained old range/resume")
	}
}

func TestTUIReviewReceivesCurrentOptionsAndFindings(t *testing.T) {
	m := testReviewTUI()
	m.opts.background, m.opts.provider, m.opts.model, m.opts.maxTokensBudget = "Context", "custom", "review-model", 2000
	m.services.review = func(ctx context.Context, opts reviewOptions, progress func(string)) (jsonOutput, error) {
		if opts.background != "Context" || opts.provider != "custom" || opts.model != "review-model" || opts.maxTokensBudget != 2000 {
			t.Errorf("lost review options: %+v", opts)
		}
		progress("Reading file.go")
		return jsonOutput{Status: "ok", SessionID: "saved", Comments: []model.LlmComment{{Path: "file.go", Content: "Check the error."}}}, nil
	}
	next, cmd := m.command("/review")
	m = next.(reviewTUIModel)
	if !m.busy {
		t.Fatal("review is not marked active")
	}
	batch := cmd().(tea.BatchMsg)
	next, _ = m.Update(batch[0]())
	m = next.(reviewTUIModel)
	if m.busy || len(m.result.Comments) != 1 || m.result.SessionID != "saved" || m.screen != tuiResults {
		t.Fatalf("result was not retained: %+v", m.result)
	}
	if logs := m.progress.snapshot(); len(logs) != 1 || logs[0] != "Reading file.go" {
		t.Fatalf("logs = %v", logs)
	}
}

func TestTUIReviewCommandsRejectArguments(t *testing.T) {
	for _, command := range []string{"/review other-scope", "/preview --commit HEAD"} {
		t.Run(command, func(t *testing.T) {
			m := testReviewTUI()
			m.opts.commit, m.opts.resume = "selected-commit", "selected-session"
			next, cmd := m.command(command)
			m = next.(reviewTUIModel)
			if cmd != nil || m.busy || !m.isError || !strings.Contains(m.notice, "without arguments") {
				t.Fatal("unexpected arguments started a task or lacked guidance")
			}
			if m.opts.commit != "selected-commit" || m.opts.resume != "selected-session" {
				t.Fatal("unexpected arguments changed the selected scope")
			}
		})
	}
}

func TestTUICancelPreservesPartialResultsAndDiagnostics(t *testing.T) {
	m := testReviewTUI()
	ctx, id := m.beginJob("Reviewing")
	m.opts.model = "original"
	next, _ := m.command("/model changed")
	m = next.(reviewTUIModel)
	if m.opts.model != "original" {
		t.Fatal("running review options changed")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(reviewTUIModel)
	if ctx.Err() == nil || !m.busy || !m.stopping {
		t.Fatal("cancel did not wait for completion")
	}
	partial := jsonOutput{Status: "partial", Comments: []model.LlmComment{{Path: "done.go", Content: "Existing finding"}}, Manifest: &session.RunManifest{RunID: "logical-only", TerminalState: session.StatePartial}}
	next, _ = m.Update(tuiReviewDoneMsg{id: id, result: partial, err: errors.New("session save failed\npermission denied")})
	m = next.(reviewTUIModel)
	if m.busy || !m.isError || !strings.Contains(m.resultContent(), "permission denied") || len(m.result.Comments) != 1 {
		t.Fatal("cancellation hid partial result or failure")
	}
	if strings.Contains(m.resultContent(), "Saved session: logical-only") {
		t.Fatal("logical run ID advertised as saved session")
	}
	next, _ = m.command("/results")
	if !strings.Contains(next.(reviewTUIModel).resultContent(), "permission denied") {
		t.Fatal("reopening results lost diagnostic")
	}
}

func TestTUIStaleCompletionDoesNotReplaceCurrentRun(t *testing.T) {
	m := testReviewTUI()
	_, id := m.beginJob("Reviewing")
	defer m.cancel()
	next, _ := m.Update(tuiReviewDoneMsg{id: id - 1, result: jsonOutput{SessionID: "stale"}})
	m = next.(reviewTUIModel)
	if !m.busy || m.result.SessionID != "" {
		t.Fatal("old completion replaced active job")
	}
}

func TestTUISetupFailureIsActionableAndNotDuplicated(t *testing.T) {
	m := testReviewTUI()
	_, id := m.beginJob("Reviewing")
	diagnostic := "resolve LLM endpoint: provider codex is not configured"
	next, _ := m.Update(tuiReviewDoneMsg{id: id, err: errors.New(diagnostic)})
	m = next.(reviewTUIModel)
	view := m.View().Content
	if !strings.Contains(view, "Review failed") || !strings.Contains(view, "/provider") || strings.Contains(view, "No review result available.") {
		t.Fatal("setup failure looked like an empty successful result or omitted recovery")
	}
	if strings.Count(view, diagnostic) != 1 || strings.Contains(view, "Enter open") {
		t.Fatal("failure duplicated its diagnostic or advertised nonexistent findings")
	}
	if m.input.Prompt != "> " {
		t.Fatal("input prompt requires a glyph unavailable in classic Windows consoles")
	}
	for _, language := range tuiLanguageIDs {
		if tuiLocales[language].Messages["Review failed"] == "" || tuiLocales[language].Messages["Use /provider to check model settings, or /review to retry."] == "" {
			t.Fatalf("missing failure guidance for %s", language)
		}
	}
}

func TestTUIPreviewFailureShowsFullDiagnostic(t *testing.T) {
	m := testReviewTUI()
	_, id := m.beginJob("Inspecting changes")
	next, _ := m.Update(tuiPreviewDoneMsg{id: id, err: errors.New("review process: exit status 1\nunknown revision: missing-branch")})
	m = next.(reviewTUIModel)
	if m.busy || !strings.Contains(m.content(), "unknown revision: missing-branch") || strings.Contains(m.content(), "0 reviewable files") {
		t.Fatal("failed preview hid the error or advertised an empty scope")
	}
}

func TestTUIResumePreservesReviewContract(t *testing.T) {
	m := testReviewTUI()
	s := &session.Summary{SessionID: "saved", ReviewMode: session.ReviewModeWorkspace, RunManifest: &session.RunManifest{Operation: session.OperationReview}}
	if err := m.prepareResume(s); err == nil {
		t.Fatal("workspace resume should be rejected")
	}
	s.ReviewMode, s.DiffFrom, s.DiffTo = session.ReviewModeRange, "main", "feature"
	if err := m.prepareResume(s); err != nil {
		t.Fatal(err)
	}
	if m.opts.resume != "saved" || m.opts.from != "main" || m.opts.to != "feature" {
		t.Fatal("resume did not recover saved scope")
	}
	s.RunManifest.Operation = "scan"
	if err := m.prepareResume(s); err == nil {
		t.Fatal("scan session accepted by diff review")
	}
}

func TestTUISessionFailureRefreshesShortLayout(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(fmt.Sprintf("resume=%v", resume), func(t *testing.T) {
			m := testReviewTUI()
			m.screen = tuiSessions
			m.height = 10
			for i := 0; i < 8; i++ {
				m.sessions = append(m.sessions, session.Summary{SessionID: fmt.Sprintf("session-%d", i)})
			}
			_, id := m.beginJob("Loading session")
			m.refresh(false)
			msg := tuiSessionMsg{id: id, err: errors.New("session could not be loaded")}
			if resume {
				msg.err = nil
				msg.resume = true
				msg.summary = &session.Summary{RunManifest: &session.RunManifest{Operation: "scan"}}
			}
			next, _ := m.Update(msg)
			m = next.(reviewTUIModel)
			view := m.View().Content
			if m.busy || !m.isError || m.notice == "" || !strings.Contains(view, m.notice) {
				t.Fatal("session failure did not finish the job and display its diagnostic")
			}
			if rows := len(strings.Split(view, "\n")); rows > m.height {
				t.Fatalf("session failure pushed the footer outside the terminal: %d > %d", rows, m.height)
			}
		})
	}
}

func TestTUIFindingNavigation(t *testing.T) {
	m := testReviewTUI()
	m.screen = tuiResults
	m.result.Comments = []model.LlmComment{{Path: "first.go"}, {Path: "second.go", ExistingCode: "before", SuggestionCode: "after"}}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(reviewTUIModel)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(reviewTUIModel)
	if m.screen != tuiFinding || !strings.Contains(m.findingContent(), "second.go") || !strings.Contains(m.findingContent(), "after") {
		t.Fatal("selected finding did not open")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(reviewTUIModel).screen != tuiResults {
		t.Fatal("escape did not return to findings")
	}
}

func TestTUILayoutFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 32}, {80, 24}, {40, 18}, {24, 10}, {12, 5}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := testReviewTUI()
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(reviewTUIModel)
			m.notice = "Error\nmultiple lines\nshould stay in one footer line"
			for _, screen := range []tuiScreen{tuiHome, tuiHelp, tuiResults, tuiLog} {
				m.screen = screen
				m.refresh(false)
				view := m.View().Content
				if view != ansi.Strip(view) {
					t.Fatal("color-disabled view contains ANSI styles")
				}
				lines := strings.Split(view, "\n")
				if len(lines) > size[1] {
					t.Fatalf("screen %d exceeds height: %d > %d", screen, len(lines), size[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("screen %d exceeds width: %q", screen, line)
					}
				}
			}
		})
	}
}

func TestTUIProgressIsBoundedAndSanitized(t *testing.T) {
	p := &tuiProgressTail{}
	for i := 0; i < 400; i++ {
		p.append(fmt.Sprintf("line %d\x1b", i))
	}
	lines := p.snapshot()
	if len(lines) != 250 || strings.Contains(strings.Join(lines, ""), "\x1b") || lines[0] != "line 150" {
		t.Fatalf("unexpected log tail: %d %q", len(lines), lines[0])
	}
}

func TestTUIWorkerShutdownJoinsAndRejectsLateWork(t *testing.T) {
	w := &tuiWorkers{}
	if !w.start() {
		t.Fatal("first worker rejected")
	}
	done := make(chan struct{})
	go func() { w.closeAndWait(); close(done) }()
	select {
	case <-done:
		t.Fatal("shutdown did not wait for active worker")
	case <-time.After(10 * time.Millisecond):
	}
	w.wg.Done()
	<-done
	if w.start() {
		t.Fatal("late worker started after shutdown")
	}
}

func TestTUIConfigCancellationKeepsOverrides(t *testing.T) {
	for _, changed := range []bool{false, true} {
		m := testReviewTUI()
		m.opts.provider, m.opts.model = "selected-provider", "selected-model"
		m.busy = true
		next, _ := m.Update(tuiConfigDoneMsg{changed: changed})
		m = next.(reviewTUIModel)
		if m.busy {
			t.Fatal("configuration did not return control")
		}
		if changed {
			if m.opts.provider != "" || m.opts.model != "" {
				t.Fatal("saved configuration did not replace overrides")
			}
		} else if m.opts.provider != "selected-provider" || m.opts.model != "selected-model" {
			t.Fatal("cancelled configuration discarded overrides")
		}
	}
}

func TestTUIColorProfileHonorsTerminalAndFlags(t *testing.T) {
	noColor := colorprofile.Env([]string{"TERM=xterm-256color", "COLORTERM=truecolor", "NO_COLOR=1"})
	if got := tuiColorProfile(colorModeAuto, noColor); got >= colorprofile.ANSI {
		t.Fatal("NO_COLOR must select monochrome pixel shapes")
	}
	if got := tuiColorProfile(colorModeAlways, noColor); got != colorprofile.TrueColor {
		t.Fatal("explicit color flag did not override NO_COLOR")
	}
	if got := tuiColorProfile(colorModeNever, colorprofile.TrueColor); got >= colorprofile.ANSI {
		t.Fatal("explicit no-color flag was ignored")
	}
}
