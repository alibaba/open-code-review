// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/workflow"
)

func TestAnalysisFlags(test *testing.T) {
	if !strings.Contains(reviewCmd.Flags().Lookup("max-tokens-budget").Usage, "only if Analysis, when requested, also completes") {
		test.Fatal("budget help does not describe combined exit status")
	}
	for _, fixture := range []struct {
		args   []string
		valid  bool
		wantTo string
	}{
		{[]string{}, true, ""},
		{[]string{"--analysis"}, true, ""},
		{[]string{"--analysis", "--from", "main"}, true, "HEAD"},
		{[]string{"--analysis", "--to", "HEAD"}, false, ""},
		{[]string{"--analysis", "--commit", "HEAD"}, true, ""},
		{[]string{"--analysis", "--from", "main", "--to", "topic", "--format", "json"}, true, "topic"},
		{[]string{"--analysis", "--commit", "HEAD", "--preview"}, false, ""},
		{[]string{"--analysis", "--commit", "HEAD", "--resume", "session"}, false, ""},
		{[]string{"--analysis", "--commit", "HEAD", "--format", "sarif"}, false, ""},
	} {
		opts, err := parseReviewFlags(fixture.args)
		if err == nil {
			err = validateReviewOptions(&opts)
		}
		if (err == nil) != fixture.valid {
			test.Fatalf("args %v: %v", fixture.args, err)
		}
		if err == nil && opts.to != fixture.wantTo {
			test.Fatalf("args %v: target = %q, want %q", fixture.args, opts.to, fixture.wantTo)
		}
	}
}

func TestAnalysisPinning(test *testing.T) {
	repo := retryTestRepo(test)
	opts := reviewOptions{repoDir: repo, from: "HEAD~1", to: "HEAD"}
	_, pinned, _, err := pinAnalysisInput(test.Context(), opts)
	if err != nil || len(pinned.ResolvedBase) != 40 || len(pinned.ResolvedHead) != 40 {
		test.Fatalf("pinning: %+v %v", pinned, err)
	}
	opts.from, opts.to, opts.commit = "", "", "HEAD"
	_, pinned, _, err = pinAnalysisInput(test.Context(), opts)
	if err != nil || len(pinned.ResolvedHead) != 40 {
		test.Fatalf("commit: %+v %v", pinned, err)
	}
	pinnedRepo, pinned, _, err := pinAnalysisInput(test.Context(), reviewOptions{repoDir: repo})
	if err != nil || pinned.ResolvedHead != "" || len(pinned.ResolvedBase) != 40 {
		test.Fatalf("workspace must keep its mode: %+v %v", pinned, err)
	}
	repoInfo, err := os.Stat(repo)
	if err != nil {
		test.Fatal(err)
	}
	pinnedInfo, err := os.Stat(pinnedRepo)
	if err != nil {
		test.Fatal(err)
	}
	if !os.SameFile(repoInfo, pinnedInfo) {
		test.Fatalf("workspace repository changed: got %q, want %q", pinnedRepo, repo)
	}
	for _, invalid := range []reviewOptions{{repoDir: repo, from: "HEAD", commit: "HEAD"}, {repoDir: test.TempDir(), commit: "HEAD"}, {repoDir: repo, commit: "--help"}, {repoDir: repo, commit: "missing"}} {
		if _, _, _, err := pinAnalysisInput(test.Context(), invalid); err == nil {
			test.Fatalf("accepted %+v", invalid)
		}
	}
}

func TestRejectedArtifactDirectoryIsNotPublished(test *testing.T) {
	repo := retryTestRepo(test)
	test.Setenv("HOME", repo)
	test.Setenv("USERPROFILE", repo)
	opts := reviewOptions{repoDir: repo, commit: "HEAD"}
	_, resolution, runner, err := pinAnalysisInput(test.Context(), opts)
	if err != nil {
		test.Fatal(err)
	}
	execution := newAnalysisExecution(&commonContext{RepoDir: repo, GitRunner: runner, Template: &template.Template{MaxTokens: 10000, MaxToolRequestTimes: 10}}, opts, &llmRuntime{Model: "fake"}, workflow.NewBudget(0, 1), "never-created", resolution)
	result, err := execution.run(test.Context())
	report := execution.report(result, err)
	if !errors.Is(err, behaviortask.ErrArtifactLocation) || report.ArtifactDir != "" || report.SessionID != "" || !reflect.DeepEqual(report.Errors, []string{behaviortask.ErrArtifactLocation.Error()}) {
		test.Fatalf("preparation: directory=%q err=%v", execution.directory, err)
	}
}

func TestRefValidationPreservesCancellation(test *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		err := validateReviewRefsUsing(reviewOptions{commit: "HEAD"}, func(string) ([]byte, error) { return nil, fmt.Errorf("verify: %w", cause) })
		if !errors.Is(err, cause) {
			test.Fatal(err)
		}
	}
}

func TestCombinedPartialCloneDoesNotFetchMissingBlobs(test *testing.T) {
	source := retryTestRepo(test)
	retryTestGit(test, source, "config", "uploadpack.allowFilter", "true")
	clone := filepath.Join(test.TempDir(), "partial")
	retryTestGit(test, source, "clone", "--quiet", "--no-local", "--no-checkout", "--filter=blob:none", source, clone)
	repo, resolution, runner, err := pinAnalysisInput(test.Context(), reviewOptions{repoDir: clone, commit: "HEAD"})
	if err != nil {
		test.Fatal(err)
	}
	missing := func() string {
		output, err := runner.Output(test.Context(), repo, "rev-list", "--objects", "--missing=print", "HEAD")
		if err != nil {
			test.Fatal(err)
		}
		return string(output)
	}
	before := missing()
	if !strings.Contains(before, "\n?") {
		test.Fatal("partial clone fixture has no missing objects")
	}
	cc, err := commonContextForRepo(repo, true, "", resolution.ResolvedHead, 0, runner)
	if err != nil {
		test.Fatal(err)
	}
	if _, err := diff.NewCommitProvider(repo, resolution.ResolvedHead, cc.GitRunner).GetDiff(test.Context()); err == nil {
		test.Fatal("review obtained missing blobs")
	}
	if _, err := behaviortask.PrepareInput(test.Context(), behaviortask.Input{RepoDir: repo, Mode: "commit", Resolution: *resolution, GitRunner: cc.GitRunner}); err == nil {
		test.Fatal("analysis obtained missing blobs")
	}
	startFakeLLM(test, newFakeLLM())
	opts, err := parseReviewFlags([]string{"--repo", clone, "--analysis", "--commit", "HEAD", "--format", "json", "--output", filepath.Join(test.TempDir(), "out.json")})
	if err != nil {
		test.Fatal(err)
	}
	if err := executeReviewContext(test.Context(), opts); err == nil {
		test.Fatal("combined command did not report unavailable input")
	}
	if missing() != before {
		test.Fatal("local operations materialized promisor objects")
	}
}

func TestAnalysisWorkspaceComparisonLabel(test *testing.T) {
	for _, empty := range []bool{false, true} {
		comparison := behaviortask.Comparison{Mode: "workspace", Old: strings.Repeat("a", 40), BaseIsEmpty: empty}
		oldSource := "aaaaaaaa"
		if empty {
			comparison.Old = ""
			oldSource = "empty tree"
		}
		report := &workflow.Report{Comparison: comparison, Status: "completed", DeclaredCompletion: "partial", Markdown: "Observed workspace source."}
		var output bytes.Buffer
		outputAnalysisText(&output, &analysisOutput{Report: report, ReviewStatus: "complete"})
		if !strings.Contains(output.String(), "Comparison: "+oldSource+" -> workspace (live)") || report.Comparison != comparison {
			test.Fatalf("workspace label changed metadata or omitted source: %s", output.String())
		}
	}
}

func analysisFixture(observations []behaviortask.Observation, partial bool) behaviortask.Analysis {
	analysis := behaviortask.Analysis{
		Markdown:           fmt.Sprintf("## Behavior\nPrevious value [[%s]]; current value [[%s]].\n\n## Impact\nThe fixture consumer sees a different value.\n\n## Maintainer Decisions\nKeep compatibility? Retain the old value (defer migration), or change it (migrate consumers).\n\n## Verification\nSynthetic fixture; no tests executed.\n\n## Questions & Limitations\nThis is not a quality evaluation.\n", observations[0].ID, observations[1].ID),
		DeclaredCompletion: "complete",
	}
	if partial {
		analysis.DeclaredCompletion = "partial"
		analysis.Markdown += "Downstream investigation remains incomplete.\n"
	}
	return analysis
}

func analysisObservations(body []byte) []behaviortask.Observation {
	var request struct {
		Messages []struct {
			Content []struct {
				Type    string          `json:"type"`
				Content json.RawMessage `json:"content"`
			} `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &request)
	var observations []behaviortask.Observation
	for _, message := range request.Messages {
		for _, block := range message.Content {
			if block.Type != "tool_result" {
				continue
			}
			var content string
			if json.Unmarshal(block.Content, &content) != nil {
				var parts []struct {
					Text string `json:"text"`
				}
				_ = json.Unmarshal(block.Content, &parts)
				for _, part := range parts {
					content += part.Text
				}
			}
			var observation behaviortask.Observation
			if json.Unmarshal([]byte(content), &observation) == nil && len(observation.Locations) > 0 {
				observations = append(observations, observation)
			}
		}
	}
	return observations
}

func TestReviewWithAnalysisHTTP(test *testing.T) {
	for _, fixture := range []struct {
		name         string
		analysis     bool
		workspace    bool
		failReview   bool
		failAnalysis bool
		partial      bool
		budget       string
		status       string
		wantError    bool
	}{
		{name: "default", status: "complete"},
		{name: "complete", analysis: true, status: "completed"},
		{name: "workspace", analysis: true, workspace: true, status: "completed"},
		{name: "analysis partial", analysis: true, partial: true, status: "completed"},
		{name: "analysis failure", analysis: true, failAnalysis: true, status: "partial", wantError: true},
		{name: "review failure", analysis: true, failReview: true, status: "partial", wantError: true},
		{name: "both fail", analysis: true, failReview: true, failAnalysis: true, status: "failed", wantError: true},
		{name: "shared budget", analysis: true, budget: "1", status: "failed", wantError: true},
		{name: "final analysis submission", analysis: true, workspace: true, budget: "15", status: "partial", wantError: true},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			home := test.TempDir()
			test.Setenv("HOME", home)
			test.Setenv("USERPROFILE", home)
			test.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			repo := retryTestRepo(test)
			if fixture.workspace {
				file := filepath.Join(repo, "a.go")
				data, err := os.ReadFile(file)
				if err != nil {
					test.Fatal(err)
				}
				if err := os.WriteFile(file, bytes.ReplaceAll(data, []byte("return 2"), []byte("return 3")), 0600); err != nil {
					test.Fatal(err)
				}
			}
			reviewer := newFakeLLM()
			if fixture.failReview {
				reviewer.failAll()
			}
			var analysisCalls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, _ := io.ReadAll(request.Body)
				if !bytes.Contains(body, []byte("submit_analysis")) {
					request.Body = io.NopCloser(bytes.NewReader(body))
					reviewer.ServeHTTP(writer, request)
					return
				}
				analysisCalls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				if fixture.failAnalysis {
					writer.WriteHeader(http.StatusPaymentRequired)
					_, _ = writer.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"analysis unavailable"}}`))
					return
				}
				observations := analysisObservations(body)
				name := "file_read"
				var arguments any = map[string]any{"side": "old", "file_path": "a.go", "start_line": 1, "end_line": 3}
				if len(observations) == 1 {
					arguments = map[string]any{"side": "new", "file_path": "a.go", "start_line": 1, "end_line": 3}
					if fixture.workspace && fixture.budget != "" {
						var final struct{ Tools []struct{ Name string } }
						if err := json.Unmarshal(body, &final); err != nil || len(final.Tools) != 1 || final.Tools[0].Name != "submit_analysis" {
							test.Error("expected a submission-only request at the shared limit")
						}
						name = "submit_analysis"
						arguments = behaviortask.Analysis{Markdown: "Read the old version [[" + observations[0].ID + "]]; the new version remains unverified.", DeclaredCompletion: "partial"}
					}
				}
				if len(observations) >= 2 {
					name = "submit_analysis"
					arguments = analysisFixture(observations, fixture.partial)
				}
				_ = json.NewEncoder(writer).Encode(map[string]any{"id": "analysis-message", "type": "message", "role": "assistant", "model": "claude-test", "content": []any{map[string]any{"type": "tool_use", "id": fmt.Sprintf("analysis-%d", analysisCalls.Load()), "name": name, "input": arguments}}, "stop_reason": "tool_use", "usage": map[string]int{"input_tokens": 10, "output_tokens": 5}})
			}))
			defer server.Close()
			for key, value := range map[string]string{"OCR_LLM_URL": server.URL + "/v1/messages", "OCR_LLM_TOKEN": "test-token", "OCR_LLM_MODEL": "claude-test", "OCR_LLM_PROTOCOL": "anthropic", "OCR_LLM_AUTH_HEADER": "x-api-key", "OCR_LLM_TIMEOUT": "30"} {
				test.Setenv(key, value)
			}
			output := filepath.Join(test.TempDir(), "output.json")
			args := []string{"--repo", repo, "--format", "json", "--output", output, "--effort", "low", "--no-filter", "--background", "Synthetic engineering verification"}
			if !fixture.workspace {
				args = append(args, "--from", "HEAD~1", "--to", "HEAD")
			}
			if fixture.analysis {
				args = append(args, "--analysis")
			}
			if fixture.budget != "" {
				args = append(args, "--max-tokens-budget", fixture.budget, "--concurrency", "1")
			}
			opts, err := parseReviewFlags(args)
			if err != nil {
				test.Fatal(err)
			}
			err = executeReviewContext(test.Context(), opts)
			if (err != nil) != fixture.wantError {
				test.Fatalf("execution: %v", err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(data, &payload); err != nil {
				test.Fatalf("output %s: %v", data, err)
			}
			if payload.Status != fixture.status {
				test.Fatalf("output: %s", data)
			}
			if !fixture.analysis {
				if payload.Analysis != nil || payload.AnalysisMeta != nil || payload.SchemaVersion != "" || analysisCalls.Load() != 0 {
					test.Fatalf("default review changed: %s", data)
				}
				return
			}
			if payload.SchemaVersion != "ocr.review-with-analysis/v2" || payload.Analysis == nil || payload.AnalysisMeta == nil || payload.Manifest == nil || payload.ReviewStatus != string(payload.Manifest.TerminalState) {
				test.Fatalf("missing peer report: %s", data)
			}
			report := payload.AnalysisMeta
			if fixture.failAnalysis && (strings.Contains(string(data), "analysis unavailable") || len(report.Errors) != 1 || report.Errors[0] != "analysis provider returned HTTP 402") {
				test.Fatalf("unexpected public provider diagnostics: %v", report.Errors)
			}
			if !fixture.workspace && (payload.Manifest.Input.RequestedFrom != "HEAD~1" || payload.Manifest.Input.RequestedHead != "HEAD") {
				test.Fatalf("requested refs changed: %+v", payload.Manifest.Input)
			}
			if report.SessionID == payload.SessionID || report.Comparison.Head == report.Comparison.Old {
				test.Fatalf("identity: %s", data)
			}
			if fixture.workspace {
				if report.Comparison.Mode != "workspace" || report.Comparison.Head != "" || len(report.Comparison.Old) != 40 {
					test.Fatalf("workspace identity: %s", data)
				}
			} else if len(report.Comparison.Head) != 40 {
				test.Fatalf("immutable identity: %s", data)
			}
			if payload.Summary.TotalTokens != payload.Summary.InputTokens+payload.Summary.OutputTokens || payload.Summary.TotalTokens < report.Usage.InputTokens+report.Usage.OutputTokens {
				test.Fatalf("usage: %s", data)
			}
			if fixture.budget != "" && !payload.Summary.BudgetExceeded {
				test.Fatalf("budget unreported: %s", data)
			}
			if fixture.workspace && fixture.budget != "" {
				if analysisCalls.Load() != 2 || payload.Summary.TotalTokens != 30 || !report.Completed || report.StopReason != "token_budget" || report.Status != "partial" || len(report.CitedObservationIDs) != 1 {
					test.Fatalf("expected one accepted, metered final submission: %s", data)
				}
			}
			if !fixture.failAnalysis && fixture.budget == "" {
				completion := "complete"
				if fixture.partial {
					completion = "partial"
				}
				if report.Status != "completed" || report.DeclaredCompletion != completion {
					test.Fatalf("execution and self-assessment conflated: %s", data)
				}
				if len(report.CitedObservationIDs) != 2 || !strings.Contains(*payload.Analysis, "## Maintainer Decisions") || analysisCalls.Load() != 3 {
					test.Fatalf("analysis: %s", data)
				}
				for _, name := range []string{"result.json", "report.md", "observations.jsonl"} {
					if _, err := os.Stat(filepath.Join(report.ArtifactDir, name)); err != nil {
						test.Fatal(err)
					}
				}
				markdown, err := os.ReadFile(filepath.Join(report.ArtifactDir, "report.md"))
				if err != nil || string(markdown) != *payload.Analysis {
					test.Fatalf("Markdown changed: %q %v", markdown, err)
				}
			}
			if fixture.failAnalysis && report.RetryReport == nil {
				test.Fatalf("analysis transport failure report missing: %s", data)
			}
		})
	}
}

func TestAnalysisSkippedReviewStatus(test *testing.T) {
	for _, fixture := range []struct {
		name            string
		failPersistence bool
		wantReview      string
		wantStatus      string
	}{
		{name: "session_persistence_failure", failPersistence: true, wantReview: "partial", wantStatus: "partial"},
		{name: "successful_skip", wantReview: "skipped", wantStatus: "completed"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			freshOCRHome(test)
			repo := initTestGitRepo(test)
			gitCommitFile(test, repo, "main.go", "package main\n", "Synthetic clean workspace")
			if fixture.failPersistence {
				sessionsDirectory, err := session.SessionsDir(repo)
				if err != nil {
					test.Fatal(err)
				}
				blocked := filepath.Dir(sessionsDirectory)
				if err := os.MkdirAll(filepath.Dir(blocked), 0700); err != nil {
					test.Fatal(err)
				}
				if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
					test.Fatal(err)
				}
			}
			reviewer := agent.New(agent.Args{RepoDir: repo})
			comments, reviewErr := reviewer.Run(test.Context())
			if (reviewErr != nil) != fixture.failPersistence {
				test.Fatalf("unexpected review error: %v", reviewErr)
			}
			if reviewErr != nil && !strings.Contains(reviewErr.Error(), "finalize session:") {
				test.Fatalf("expected a session persistence failure: %v", reviewErr)
			}
			manifest := reviewer.RunManifest()
			if manifest == nil || manifest.TerminalState != session.StateSkipped || len(manifest.Coverage.Selected) != 0 || len(comments) != 0 {
				test.Fatalf("expected an empty skipped review: manifest=%+v comments=%+v", manifest, comments)
			}
			combined := &combinedResult{Agent: reviewer, reviewErr: reviewErr, budget: workflow.NewBudget(0, 1), report: &workflow.Report{Status: "completed", Markdown: "Analysis result."}}
			output := combined.AnalysisOutput()
			if output.ReviewStatus != fixture.wantReview || output.status() != fixture.wantStatus {
				test.Errorf("review=%s overall=%s, want review=%s overall=%s", output.ReviewStatus, output.status(), fixture.wantReview, fixture.wantStatus)
			}
			var human bytes.Buffer
			if err := emitRunResult(test.Context(), combined, comments, time.Now(), "text", "human", nil, nil, &human, nil); err != nil {
				test.Fatal(err)
			}
			if !strings.Contains(human.String(), "Analysis: completed") || strings.Contains(human.String(), "Review:") != fixture.failPersistence {
				test.Errorf("text status mismatch: %s", human.String())
			}
			if fixture.failPersistence && (strings.Count(human.String(), "Review: partial") != 1 || strings.Index(human.String(), "Review: partial") > strings.Index(human.String(), "Analysis result.")) {
				test.Errorf("Review delivery failure was hidden or buried: %s", human.String())
			}
			var machine bytes.Buffer
			if err := outputJSONWithWarnings(comments, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, reviewer.SessionID(), manifest, false, nil, &machine, nil, nil, output); err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
				test.Fatal(err)
			}
			if payload.Status != fixture.wantStatus || payload.ReviewStatus != fixture.wantReview {
				test.Errorf("JSON status mismatch: %s", machine.String())
			}
			if payload.Manifest == nil || payload.Manifest.TerminalState != session.StateSkipped || reviewer.RunManifest().TerminalState != session.StateSkipped {
				test.Fatalf("frozen skipped manifest changed: %s", machine.String())
			}
		})
	}
}

func TestAnalysisTextAndStatus(test *testing.T) {
	for _, fixture := range []struct{ review, analysis, want string }{
		{"complete", "completed", "completed"},
		{"skipped", "completed", "completed"},
		{"failed", "completed", "partial"},
		{"failed", "failed", "failed"},
		{"partial", "completed", "partial"},
		{"partial", "partial", "partial"},
		{"complete", "failed", "partial"},
		{"complete", "partial", "partial"},
	} {
		output := &analysisOutput{ReviewStatus: fixture.review, Report: &workflow.Report{Status: fixture.analysis, DeclaredCompletion: "complete"}}
		if output.status() != fixture.want {
			test.Fatalf("status %+v", fixture)
		}
		var buffer bytes.Buffer
		outputAnalysisText(&buffer, output)
		wantText := "Analysis: " + fixture.analysis
		if !strings.Contains(buffer.String(), wantText) || strings.Contains(buffer.String(), "Model-declared completeness:") {
			test.Fatalf("unexpected display labels: %s", buffer.String())
		}
		if fixture.review == "partial" || fixture.review == "failed" {
			if strings.Count(buffer.String(), "Review: "+fixture.review) != 1 {
				test.Fatalf("Review failure status was hidden or repeated: %s", buffer.String())
			}
		} else if strings.Contains(buffer.String(), "Review:") {
			test.Fatalf("normal Review status should remain hidden: %s", buffer.String())
		}
		if !strings.Contains(buffer.String(), "No analysis report was accepted.") {
			test.Fatal(buffer.String())
		}
		var machine bytes.Buffer
		if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", nil, false, nil, &machine, nil, nil, output); err != nil {
			test.Fatal(err)
		}
		var payload jsonOutput
		if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
			test.Fatal(err)
		}
		if output.ReviewStatus != fixture.review || payload.ReviewStatus != fixture.review || payload.Status != fixture.want || payload.AnalysisMeta == nil || payload.AnalysisMeta.Status != fixture.analysis || payload.AnalysisMeta.DeclaredCompletion != "complete" {
			test.Fatalf("text rendering changed machine-readable states: %s", machine.String())
		}
	}
	analysis := analysisFixture([]behaviortask.Observation{{ID: "old", Locations: []behaviortask.Location{{Side: "old", Path: "a.go", StartLine: 1, EndLine: 3}}}, {ID: "new", Locations: []behaviortask.Location{{Side: "new", Path: "a.go", StartLine: 1, EndLine: 3}}}}, true)
	analysis.Markdown += "\nUnusual heading and quotes: \"fixture\"\n\x1b[31m\n"
	report := workflow.NewReport(behaviortask.Result{Analysis: &analysis, Errors: []string{"bad\x1b[31m"}, Warnings: []llmloop.AgentWarning{{Message: "warning\x1b[31m"}}}, nil, "artifacts")
	var buffer bytes.Buffer
	outputAnalysisText(&buffer, &analysisOutput{ReviewStatus: "complete", Report: report})
	if strings.Contains(buffer.String(), "\x1b") || !strings.Contains(buffer.String(), "Unusual heading and quotes:") || !strings.Contains(buffer.String(), "Investigation incomplete (model self-assessment)") {
		test.Fatalf("text: %s", buffer.String())
	}
}

func TestAnalysisTextOmitsProcessWarnings(test *testing.T) {
	for _, status := range []string{"completed", "partial", "failed"} {
		test.Run(status, func(test *testing.T) {
			markdown := "Limitation: the external consumer's compatibility remains unverified."
			if status == "failed" {
				markdown = ""
			}
			warnings := []llmloop.AgentWarning{
				{Type: "observation_error", File: "obs-000039", Message: "unavailable"},
				{Type: "observation_truncated", File: "obs-000022", Message: "Narrow the line range or file_patterns and read again."},
				{Type: "observation_truncated", File: "obs-000027", Message: "Narrow the line range or file_patterns and read again."},
				{Type: "observation_replay_truncated", File: "obs-000040", Message: "Observation replay returned a page; continue with context_read and its cursor."},
			}
			report := &workflow.Report{Status: status, DeclaredCompletion: "partial", Markdown: markdown, Warnings: warnings}
			before, err := json.Marshal(report)
			if err != nil {
				test.Fatal(err)
			}
			combined := &analysisOutput{ReviewStatus: "complete", Report: report}
			var human bytes.Buffer
			outputAnalysisText(&human, combined)
			if strings.Contains(human.String(), "Warning:") {
				test.Fatalf("process diagnostics leaked into the report: %s", human.String())
			}
			if !strings.Contains(human.String(), "Analysis: "+status) || strings.Contains(human.String(), "Investigation incomplete (model self-assessment)") != (markdown != "") {
				test.Fatalf("execution or declared completeness was hidden: %s", human.String())
			}
			if status == "failed" {
				if !strings.Contains(human.String(), "No analysis report was accepted.") {
					test.Fatal(human.String())
				}
			} else if strings.Count(human.String(), markdown) != 1 {
				test.Fatalf("material limitation was lost or repeated: %s", human.String())
			}
			after, err := json.Marshal(report)
			if err != nil || !bytes.Equal(before, after) {
				test.Fatalf("text output mutated the report: %s %v", after, err)
			}
			var machine bytes.Buffer
			if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", nil, false, nil, &machine, nil, nil, combined); err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
				test.Fatal(err)
			}
			if payload.AnalysisMeta == nil || !reflect.DeepEqual(payload.AnalysisMeta.Warnings, warnings) || payload.Analysis == nil || *payload.Analysis != markdown {
				test.Fatalf("JSON diagnostics or original Markdown changed: %s", machine.String())
			}
		})
	}
}

func TestAnalysisTextKeepsDeliveryWarnings(test *testing.T) {
	for _, warning := range []llmloop.AgentWarning{
		{Type: "behavior_limit_reached", Message: "Analysis token budget exhausted."},
		{Type: "behavior_limit_reached", Message: "Analysis timed out."},
		{Type: "artifact_warning", Message: "Analysis artifacts are incomplete."},
		{Message: "Unclassified warning."},
	} {
		test.Run(warning.Message, func(test *testing.T) {
			report := &workflow.Report{Status: "failed", Warnings: []llmloop.AgentWarning{warning}, Errors: []string{"Analysis report delivery failed."}}
			var output bytes.Buffer
			outputAnalysisText(&output, &analysisOutput{ReviewStatus: "complete", Report: report})
			for _, want := range []string{"Warning: " + warning.Message, "Error: Analysis report delivery failed.", "No analysis report was accepted."} {
				if !strings.Contains(output.String(), want) {
					test.Fatalf("missing %q: %s", want, output.String())
				}
			}
		})
	}
}

func TestAnalysisTextBrief(test *testing.T) {
	for _, completion := range []string{"complete", "unknown", ""} {
		test.Run(completion, func(test *testing.T) {
			const markdown = "## Behavior\n\nThe default changes [[obs-000001]].\n"
			report := &workflow.Report{
				Status: "completed", Completed: true, DeclaredCompletion: completion,
				Comparison: behaviortask.Comparison{Mode: "range", Old: strings.Repeat("a", 40), Head: strings.Repeat("b", 40)},
				StopReason: "none", SubmissionStatus: "accepted", ArtifactDir: "evidence/run",
				Markdown: markdown,
				Usage:    behaviortask.Usage{InputTokens: 1895278, OutputTokens: 15599},
				RetryReport: &llm.RetryReport{TotalRequests: 27, CancelledRequests: 2, Requests: []llm.RequestReport{
					{LogicalRequestID: "request-id", TaskType: "MEMORY_COMPRESSION_TASK", Outcome: llm.OutcomeCancelled},
				}},
			}
			before, err := json.Marshal(report)
			if err != nil {
				test.Fatal(err)
			}
			combined := &analysisOutput{ReviewStatus: "complete", Report: report}
			var human bytes.Buffer
			outputAnalysisText(&human, combined)
			want := "\n──────── Analysis Report ────────\nComparison: aaaaaaaa -> bbbbbbbb\n\n## Behavior\n\nThe default changes.\n\nAnalysis: completed\nEvidence: evidence/run\n"
			if human.String() != want {
				test.Fatalf("unexpected brief:\ngot: %q\nwant: %q", human.String(), want)
			}
			var machine bytes.Buffer
			if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", nil, false, nil, &machine, nil, nil, combined); err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
				test.Fatal(err)
			}
			after, err := json.Marshal(report)
			if err != nil || !bytes.Equal(before, after) || report.Markdown != markdown || payload.Analysis == nil || *payload.Analysis != markdown || payload.AnalysisMeta == nil || !reflect.DeepEqual(payload.AnalysisMeta.RetryReport, report.RetryReport) || payload.AnalysisMeta.Usage != report.Usage || payload.AnalysisMeta.Comparison != report.Comparison {
				test.Fatalf("text output changed raw report or JSON: %s, %v", machine.String(), err)
			}
		})
	}
}

func TestAnalysisTextLimitPlacement(test *testing.T) {
	for _, stop := range []llmloop.MainLoopStop{llmloop.StopMaxRounds, llmloop.StopEmptyRounds, llmloop.StopCompression, llmloop.StopTokenBudget} {
		test.Run(stop.String(), func(test *testing.T) {
			for _, markdown := range []string{"", "Supported result [[obs-000001]]."} {
				report := &workflow.Report{
					Status: "partial", StopReason: stop.String(), Markdown: markdown, DeclaredCompletion: "partial",
					Errors: []string{"Artifact write failed."},
					Warnings: []llmloop.AgentWarning{{Type: "behavior_limit_reached", Message: stop.Reason()},
						{Type: "artifact_warning", Message: "Evidence is incomplete."}},
				}
				body := "Supported result."
				if markdown == "" {
					report.Status = "failed"
					body = "No analysis report was accepted."
				}
				var human bytes.Buffer
				outputAnalysisText(&human, &analysisOutput{Report: report})
				text := human.String()
				for _, message := range []string{"Analysis: " + report.Status, stop.Reason(), "Error: Artifact write failed.", "Warning: Evidence is incomplete."} {
					if strings.Count(text, message) != 1 || strings.Index(text, message) > strings.Index(text, body) {
						test.Fatalf("missing, repeated or buried diagnostic %q: %s", message, text)
					}
				}
				if !strings.Contains(text, body) || strings.Contains(text, "Analysis: completed") || strings.Contains(text, "Evidence:") {
					test.Fatalf("misleading status or artifact path: %s", text)
				}
			}
		})
	}
	var human bytes.Buffer
	outputAnalysisText(&human, &analysisOutput{Report: &workflow.Report{Status: "failed", StopReason: "future_stop\x1b[31m"}})
	if !strings.Contains(human.String(), "Reason: future_stop") || strings.Contains(human.String(), "\x1b") {
		test.Fatalf("unknown stop was hidden or unsafe: %s", human.String())
	}
}

func TestShortAnalysisRevision(test *testing.T) {
	for _, fixture := range []struct{ revision, want string }{
		{strings.Repeat("a", 40), "aaaaaaaa"},
		{strings.Repeat("B", 64), "BBBBBBBB"},
		{"main", "main"},
		{"", ""},
		{"abcdef12", "abcdef12"},
		{strings.Repeat("z", 40), strings.Repeat("z", 40)},
	} {
		if got := shortAnalysisRevision(fixture.revision); got != fixture.want {
			test.Errorf("shortAnalysisRevision(%q) = %q, want %q", fixture.revision, got, fixture.want)
		}
	}
}
