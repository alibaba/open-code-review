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
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/workflow"
)

func TestAnalysisFlags(test *testing.T) {
	if !strings.Contains(reviewCmd.Flags().Lookup("analysis").Usage, "without changing the Review exit status") {
		test.Fatal("analysis help does not describe the Review exit policy")
	}
	for _, fixture := range []struct {
		args   []string
		valid  bool
		wantTo string
	}{
		{[]string{}, true, ""},
		{[]string{"--analysis"}, false, ""},
		{[]string{"--analysis", "--analysis-max-tokens-budget", "0"}, false, ""},
		{[]string{"--analysis-max-tokens-budget", "0"}, true, ""},
		{[]string{"--analysis", "--analysis-max-tokens-budget", "-1"}, false, ""},
		{[]string{"--analysis-max-tokens-budget", "-1"}, false, ""},
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
		if len(fixture.args) == 0 && opts.analysisMaxTokensBudget != 0 {
			test.Fatalf("expected unlimited analysis budget by default, got %d", opts.analysisMaxTokensBudget)
		}
	}
}

func TestWorkspaceAnalysisRejectedBeforeReview(test *testing.T) {
	output := filepath.Join(test.TempDir(), "review.json")
	err := runReview([]string{"--analysis", "--repo", filepath.Join(test.TempDir(), "missing"), "--output", output})
	if err == nil {
		test.Fatal("workspace analysis unexpectedly succeeded")
	}
	for _, hint := range []string{"workspace mode", "omit --analysis", "--commit", "--from"} {
		if !strings.Contains(err.Error(), hint) {
			test.Fatalf("missing %q in %v", hint, err)
		}
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		test.Fatalf("unexpected output: %v", err)
	}
}

func analysisTestContext(test *testing.T, repo string) *commonContext {
	test.Helper()
	cc, err := loadCommonContext(repo, "", "HEAD", 0, 1, true)
	if err != nil {
		test.Fatal(err)
	}
	return cc
}

func TestAnalysisSharesReviewGitLimit(test *testing.T) {
	repo := retryTestRepo(test)
	cc := analysisTestContext(test, repo)
	opts := reviewOptions{commit: "HEAD"}
	setup, err := prepareReviewCodeTools(test.Context(), repo, opts, cc.GitRunner, nil)
	if err != nil {
		test.Fatal(err)
	}
	input := analysisInputForReview(cc, opts, &llmRuntime{}, "review", *setup.resolution).input
	err = cc.GitRunner.Stream(test.Context(), repo, func(stdout io.Reader) error {
		if _, err := io.Copy(io.Discard, stdout); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(test.Context(), 250*time.Millisecond)
		defer cancel()
		if _, err := behaviortask.PrepareInput(ctx, input); !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("analysis did not wait for the occupied Git slot: %v", err)
		}
		return nil
	}, "rev-parse", "HEAD")
	if err != nil {
		test.Fatal(err)
	}
	if _, err := behaviortask.PrepareInput(test.Context(), input); err != nil {
		test.Fatalf("analysis after releasing the Git slot: %v", err)
	}
}

func TestAnalysisUsesResolvedReviewInput(test *testing.T) {
	repo := retryTestRepo(test)
	cc := analysisTestContext(test, repo)
	for _, opts := range []reviewOptions{{repoDir: repo, from: "HEAD~1", to: "HEAD"}, {repoDir: repo, commit: "HEAD"}} {
		setup, err := prepareReviewCodeTools(test.Context(), repo, opts, nil, nil)
		if err != nil {
			test.Fatal(err)
		}
		request := analysisInputForReview(cc, opts, &llmRuntime{}, "review", *setup.resolution)
		retryTestGit(test, repo, "commit", "--allow-empty", "-qm", "Advance branch")
		prepared, err := behaviortask.PrepareInput(test.Context(), request.input)
		if err != nil || prepared.Resolution != *setup.resolution {
			test.Fatalf("comparison changed: %+v, %v", prepared.Resolution, err)
		}
	}
}

func TestRejectedArtifactDirectoryIsNotPublished(test *testing.T) {
	repo := retryTestRepo(test)
	test.Setenv("HOME", repo)
	test.Setenv("USERPROFILE", repo)
	opts := reviewOptions{repoDir: repo, commit: "HEAD"}
	setup, err := prepareReviewCodeTools(test.Context(), repo, opts, nil, nil)
	if err != nil {
		test.Fatal(err)
	}
	execution := newAnalysisExecution(analysisInputForReview(analysisTestContext(test, repo), opts, &llmRuntime{}, "never-created", *setup.resolution))
	result, err := execution.run(test.Context())
	report := execution.report(result, err)
	if !errors.Is(err, behaviortask.ErrArtifactLocation) || report.ArtifactDir != "" || report.SessionID != "" || !reflect.DeepEqual(report.Errors, []string{behaviortask.ErrArtifactLocation.Error()}) {
		test.Fatalf("preparation: directory=%q err=%v", execution.directory, err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".opencodereview", "analysis")); !errors.Is(err, os.ErrNotExist) {
		test.Fatalf("analysis directory: %v", err)
	}
}

func TestAnalysisPartialCloneDoesNotFetchMissingBlobs(test *testing.T) {
	source := retryTestRepo(test)
	retryTestGit(test, source, "config", "uploadpack.allowFilter", "true")
	clone := filepath.Join(test.TempDir(), "partial")
	retryTestGit(test, source, "clone", "--quiet", "--no-local", "--no-checkout", "--filter=blob:none", source, clone)
	runner := gitcmd.New(1)
	missing := func() string {
		output, err := runner.Output(test.Context(), clone, "rev-list", "--objects", "--missing=print", "HEAD")
		if err != nil {
			test.Fatal(err)
		}
		return string(output)
	}
	before := missing()
	if !strings.Contains(before, "\n?") {
		test.Fatal("partial clone fixture has no missing objects")
	}
	for _, opts := range []reviewOptions{{commit: "HEAD"}, {from: "HEAD~1", to: "HEAD"}} {
		setup, err := prepareReviewCodeTools(test.Context(), clone, opts, runner, nil)
		if err != nil {
			test.Fatal(err)
		}
		if _, err := behaviortask.PrepareInput(test.Context(), behaviortask.Input{RepoDir: clone, Mode: reviewModeFromOptions(opts), Resolution: *setup.resolution}); err == nil {
			test.Fatal("analysis obtained missing blobs")
		}
	}
	if missing() != before {
		test.Fatal("local operations materialized promisor objects")
	}
}

func TestAnalysisCommitComparisonLabel(test *testing.T) {
	for _, empty := range []bool{false, true} {
		comparison := behaviortask.Comparison{Mode: "commit", Old: strings.Repeat("a", 40), Head: strings.Repeat("b", 40), BaseIsEmpty: empty}
		oldSource := "aaaaaaaa"
		if empty {
			comparison.Old = ""
			oldSource = "empty tree"
		}
		report := &workflow.Report{Comparison: comparison, Status: "completed", DeclaredCompletion: "partial", Markdown: "Observed committed source."}
		var output bytes.Buffer
		outputAnalysisText(&output, &analysisOutput{Report: report, ReviewStatus: "complete"})
		if !strings.Contains(output.String(), "Comparison: "+oldSource+" -> bbbbbbbb") || report.Comparison != comparison {
			test.Fatalf("commit label changed metadata or omitted source: %s", output.String())
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
		name            string
		analysis        bool
		rawLogging      bool
		emptyCommit     bool
		failReview      bool
		failAnalysis    bool
		partial         bool
		budget          string
		analysisBudget  string
		analysisTokens  int
		largeBackground bool
		maxTokensFlag   string
		mainTokens      int
		blockOutput     bool
		status          string
		wantError       bool
	}{
		{name: "default", status: "complete"},
		{name: "saved context limit pages analysis", analysis: true, emptyCommit: true, largeBackground: true, status: "skipped"},
		{name: "CLI context limit overrides saved limit", analysis: true, emptyCommit: true, largeBackground: true, maxTokensFlag: "160000", status: "skipped"},
		{name: "unlimited analysis", analysis: true, analysisTokens: 100_001, rawLogging: true, status: "complete"},
		{name: "analysis partial self-assessment", analysis: true, partial: true, status: "complete"},
		{name: "analysis failure", analysis: true, failAnalysis: true, status: "complete"},
		{name: "output failure", analysis: true, failAnalysis: true, blockOutput: true, wantError: true},
		{name: "review failure", analysis: true, failReview: true, status: "failed", wantError: true},
		{name: "both fail", analysis: true, failReview: true, failAnalysis: true, status: "failed", wantError: true},
		{name: "review dispatch budget", analysis: true, budget: "1", status: "failed", wantError: true},
		{name: "budget-only partial review with analysis failure", analysis: true, failAnalysis: true, budget: "100000", mainTokens: 95000, status: "partial"},
		{name: "final analysis submission", analysis: true, analysisBudget: "15", status: "complete"},
		{name: "skipped review with limited analysis", analysis: true, emptyCommit: true, analysisBudget: "15", status: "skipped"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			home := test.TempDir()
			if fixture.rawLogging {
				test.Setenv("OCR_RAW_LOGGING", "1")
			}
			test.Setenv("HOME", home)
			test.Setenv("USERPROFILE", home)
			test.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			repo := retryTestRepo(test)
			if fixture.emptyCommit {
				retryTestGit(test, repo, "commit", "--allow-empty", "-qm", "Empty change")
			}
			reviewer := newFakeLLM()
			if fixture.mainTokens > 0 {
				reviewer.mainInputTokens = fixture.mainTokens
			}
			if fixture.failReview {
				reviewer.failAll()
			}
			var analysisCalls atomic.Int64
			output := filepath.Join(test.TempDir(), "output.json")
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, _ := io.ReadAll(request.Body)
				if !bytes.Contains(body, []byte("submit_analysis")) {
					request.Body = io.NopCloser(bytes.NewReader(body))
					reviewer.ServeHTTP(writer, request)
					return
				}
				if analysisCalls.Add(1) == 1 && fixture.largeBackground {
					paged := bytes.Contains(body, []byte("follow the continuation cursor"))
					inline := bytes.Contains(body, []byte("long-background-marker"))
					if paged != (fixture.maxTokensFlag == "") || inline == paged {
						test.Errorf("first analysis prompt: paged=%t inline=%t with --max-tokens=%q", paged, inline, fixture.maxTokensFlag)
					}
				}
				writer.Header().Set("Content-Type", "application/json")
				if fixture.failAnalysis {
					if fixture.blockOutput {
						if err := os.Mkdir(output, 0700); err != nil {
							test.Error(err)
						}
					}
					writer.WriteHeader(http.StatusPaymentRequired)
					_, _ = writer.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"analysis unavailable"}}`))
					return
				}
				observations := analysisObservations(body)
				name := "file_read"
				var arguments any = map[string]any{"side": "old", "file_path": "a.go", "start_line": 1, "end_line": 3}
				if len(observations) == 1 {
					arguments = map[string]any{"side": "new", "file_path": "a.go", "start_line": 1, "end_line": 3}
					if fixture.analysisBudget != "" {
						var final struct{ Tools []struct{ Name string } }
						if err := json.Unmarshal(body, &final); err != nil || len(final.Tools) != 1 || final.Tools[0].Name != "submit_analysis" {
							test.Error("expected a submission-only request at the analysis limit")
						}
						name = "submit_analysis"
						arguments = behaviortask.Analysis{Markdown: "Read the old version [[" + observations[0].ID + "]]; the new version remains unverified.", DeclaredCompletion: "partial"}
					}
				}
				if len(observations) >= 2 {
					name = "submit_analysis"
					arguments = analysisFixture(observations, fixture.partial)
				}
				_ = json.NewEncoder(writer).Encode(map[string]any{"id": "analysis-message", "type": "message", "role": "assistant", "model": "claude-test", "content": []any{map[string]any{"type": "tool_use", "id": fmt.Sprintf("analysis-%d", analysisCalls.Load()), "name": name, "input": arguments}}, "stop_reason": "tool_use", "usage": map[string]int{"input_tokens": max(10, fixture.analysisTokens), "output_tokens": 5}})
			}))
			defer server.Close()
			for key, value := range map[string]string{"OCR_LLM_URL": server.URL + "/v1/messages", "OCR_LLM_TOKEN": "test-token", "OCR_LLM_MODEL": "claude-test", "OCR_LLM_PROTOCOL": "anthropic", "OCR_LLM_AUTH_HEADER": "x-api-key", "OCR_LLM_TIMEOUT": "30"} {
				test.Setenv(key, value)
			}
			background := "Synthetic engineering verification"
			if fixture.largeBackground {
				if err := runConfigSet("max_tokens", "16000"); err != nil {
					test.Fatal(err)
				}
				background = "long-background-marker" + strings.Repeat(" background", 18_000)
			}
			args := []string{"--repo", repo, "--from", "HEAD~1", "--to", "HEAD", "--format", "json", "--output", output, "--effort", "low", "--no-filter", "--background", background}
			if fixture.maxTokensFlag != "" {
				args = append(args, "--max-tokens", fixture.maxTokensFlag)
			}
			if fixture.analysis {
				args = append(args, "--analysis")
			}
			if fixture.analysisBudget != "" {
				args = append(args, "--analysis-max-tokens-budget", fixture.analysisBudget)
			}
			if fixture.budget != "" {
				args = append(args, "--max-tokens-budget", fixture.budget, "--concurrency", "1")
			}
			opts, err := parseReviewFlags(args)
			if err != nil {
				test.Fatal(err)
			}
			stderr := captureStderr(test, func() { err = executeReviewContext(test.Context(), opts) })
			if (err != nil) != fixture.wantError {
				test.Fatalf("execution: %v", err)
			}
			if fixture.blockOutput {
				if !strings.Contains(err.Error(), "close output file") {
					test.Fatalf("missing output delivery error: %v", err)
				}
				return
			}
			data, err := os.ReadFile(output)
			if err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(data, &payload); err != nil {
				test.Fatalf("output %s: %v", data, err)
			}
			if payload.Status != fixture.status || payload.Message != manifestMessage(payload.Manifest, len(payload.Comments)) {
				test.Fatalf("output: %s", data)
			}
			if !fixture.analysis {
				if bytes.Contains(data, []byte(`"analysis_raw":`)) || payload.Analysis != nil || payload.AnalysisMeta != nil || payload.ReviewStatus != "" || payload.SchemaVersion != "" || analysisCalls.Load() != 0 {
					test.Fatalf("default review changed: %s", data)
				}
				return
			}
			if payload.SchemaVersion != "ocr.review-with-analysis/v2" || payload.Analysis == nil || payload.AnalysisRaw == nil || payload.AnalysisMeta == nil || payload.Manifest == nil || payload.ReviewStatus != string(payload.Manifest.TerminalState) {
				test.Fatalf("missing peer report: %s", data)
			}
			report := payload.AnalysisMeta
			const warning = "[ocr] warning: analysis execution "
			if (report.Status == "completed" && strings.Contains(stderr, warning)) || (report.Status != "completed" && !strings.Contains(stderr, warning+report.Status)) {
				test.Fatalf("unexpected warning for %s analysis: %s", report.Status, stderr)
			}
			if fixture.failAnalysis && (report.Status != "failed" || *payload.Analysis != "" || *payload.AnalysisRaw != "" || strings.Contains(string(data), "analysis unavailable") || len(report.Errors) != 1 || report.Errors[0] != "analysis provider returned HTTP 402") {
				test.Fatalf("unexpected public provider diagnostics: %v", report.Errors)
			}
			if payload.Manifest.Input.RequestedFrom != "HEAD~1" || payload.Manifest.Input.RequestedHead != "HEAD" {
				test.Fatalf("requested refs changed: %+v", payload.Manifest.Input)
			}
			if report.SessionID == "" || report.SessionID != payload.SessionID || report.Comparison.Head == report.Comparison.Old {
				test.Fatalf("identity: %s", data)
			}
			sessionRepo, err := resolveRepoDir(repo)
			if err != nil {
				test.Fatal(err)
			}
			summaries, err := session.ListSessions(sessionRepo)
			if err != nil || len(summaries) != 1 || summaries[0].Analysis == nil || !summaries[0].Analysis.Enabled || summaries[0].SessionID != report.SessionID || report.ArtifactDir != filepath.Join(strings.TrimSuffix(summaries[0].FilePath, ".jsonl"), "analysis") {
				test.Fatalf("single report: %+v, %v; artifacts=%s", summaries, err, report.ArtifactDir)
			}
			if report.Comparison.Mode != "range" || len(report.Comparison.Head) != 40 {
				test.Fatalf("immutable identity: %s", data)
			}
			if fixture.rawLogging {
				raw, err := os.ReadFile(filepath.Join(report.ArtifactDir, "raw.jsonl"))
				if err != nil || !bytes.Contains(raw, []byte(report.SessionID)) {
					test.Fatalf("missing Analysis raw capture: %v", err)
				}
			}
			if payload.Summary.TotalTokens != payload.Summary.InputTokens+payload.Summary.OutputTokens {
				test.Fatalf("usage: %s", data)
			}
			if fixture.budget != "" && !payload.Summary.BudgetExceeded {
				test.Fatalf("budget unreported: %s", data)
			}
			if fixture.status == "partial" && len(payload.Manifest.Coverage.Completed) == 0 {
				test.Fatalf("budget-only partial review lost usable coverage: %s", data)
			}
			if fixture.analysisBudget != "" {
				if analysisCalls.Load() != 2 || report.Usage.InputTokens+report.Usage.OutputTokens != 30 || payload.Summary.BudgetExceeded || !report.Completed || report.StopReason != "token_budget" || report.Status != "partial" || len(report.CitedObservationIDs) != 1 {
					test.Fatalf("expected one accepted, metered final submission: %s", data)
				}
			}
			if !fixture.failAnalysis && fixture.analysisBudget == "" {
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
				for _, name := range []string{"result.json", "report.md", "observations.jsonl", "trace.jsonl"} {
					if _, err := os.Stat(filepath.Join(report.ArtifactDir, name)); err != nil {
						test.Fatal(err)
					}
				}
				markdown, err := os.ReadFile(filepath.Join(report.ArtifactDir, "report.md"))
				if err != nil || string(markdown) != *payload.AnalysisRaw {
					test.Fatalf("report.md does not match analysis_raw: %q %v", markdown, err)
				}
			}
			if fixture.failAnalysis && report.RetryReport == nil {
				test.Fatalf("analysis transport failure report missing: %s", data)
			}
		})
	}
}

func TestAnalysisPreservesCommitReview(test *testing.T) {
	for _, scenario := range []string{"completed", "budget", "provider failure", "git config", "alternate index"} {
		test.Run(scenario, func(test *testing.T) {
			repo := retryTestRepo(test)
			if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package p\nfunc a() int { return 3 }\n"), 0600); err != nil {
				test.Fatal(err)
			}
			retryTestGit(test, repo, "commit", "-qm", "Change a", "--", "a.go")
			if scenario == "git config" {
				test.Setenv("GIT_CONFIG_COUNT", "1")
				test.Setenv("GIT_CONFIG_KEY_0", "core.quotepath")
				test.Setenv("GIT_CONFIG_VALUE_0", "false")
			}
			if scenario == "alternate index" {
				index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
				if err != nil {
					test.Fatal(err)
				}
				alternate := filepath.Join(test.TempDir(), "index")
				if err := os.WriteFile(alternate, index, 0600); err != nil {
					test.Fatal(err)
				}
				test.Setenv("GIT_INDEX_FILE", alternate)
			}
			reviewer := newFakeLLM()
			var calls atomic.Int32
			var waitForAnalysis atomic.Bool
			reviewStarted, analysisStarted := make(chan struct{}), make(chan struct{})
			waitFor := func(peer <-chan struct{}) {
				select {
				case <-peer:
				case <-time.After(5 * time.Second):
					test.Error("model requests did not overlap at review concurrency one")
				}
			}
			startFakeLLM(test, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					test.Error(err)
					return
				}
				if !bytes.Contains(body, []byte("submit_analysis")) {
					if waitForAnalysis.CompareAndSwap(true, false) {
						close(reviewStarted)
						waitFor(analysisStarted)
					}
					request.Body = io.NopCloser(bytes.NewReader(body))
					reviewer.ServeHTTP(writer, request)
					return
				}
				count := calls.Add(1)
				if scenario == "completed" && count == 1 {
					close(analysisStarted)
					waitFor(reviewStarted)
				}
				writer.Header().Set("Content-Type", "application/json")
				if scenario == "provider failure" {
					writer.WriteHeader(http.StatusPaymentRequired)
					_, _ = io.WriteString(writer, `{"type":"error","error":{"type":"invalid_request_error","message":"fixture"}}`)
					return
				}
				name, args := "submit_analysis", `{"markdown":"The commit changes a.go."}`
				if scenario == "budget" && count == 1 {
					name, args = "file_read", `{"file_path":"a.go"}`
				}
				fmt.Fprintf(writer, `{"id":"msg_a","type":"message","role":"assistant","model":"claude-test","content":[{"type":"tool_use","id":"call","name":%q,"input":%s}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`, name, args)
			}))
			run := func(enabled bool) jsonOutput {
				output := filepath.Join(test.TempDir(), "review.json")
				args := []string{"--repo", repo, "--commit", "HEAD", "--output", output, "--format", "json", "--effort", "low", "--concurrency", "1", "--no-filter"}
				if enabled {
					args = append(args, "--analysis")
					if scenario == "budget" {
						args = append(args, "--analysis-max-tokens-budget", "15")
					}
				}
				if err := runReview(args); err != nil {
					test.Fatal(err)
				}
				data, err := os.ReadFile(output)
				if err != nil {
					test.Fatal(err)
				}
				var result jsonOutput
				if err := json.Unmarshal(data, &result); err != nil {
					test.Fatal(err)
				}
				return result
			}
			plain := run(false)
			if plain.AnalysisMeta != nil || calls.Load() != 0 || plain.Manifest == nil || len(plain.Manifest.Coverage.Selected) != 1 || plain.Manifest.Coverage.Selected[0].Path != "a.go" {
				test.Fatalf("plain review: %+v", plain)
			}
			sessions, err := session.SessionsDir(repo)
			if err != nil {
				test.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(sessions, plain.SessionID)); !errors.Is(err, os.ErrNotExist) {
				test.Fatalf("unexpected analysis artifacts: %v", err)
			}
			waitForAnalysis.Store(scenario == "completed")
			combined := run(true)
			report := combined.AnalysisMeta
			if report == nil || report.Comparison.Old != plain.Manifest.Input.ResolvedBase {
				test.Fatalf("analysis comparison: %+v", report)
			}
			want, cost := "completed", int64(15)
			switch scenario {
			case "provider failure":
				want, cost = "failed", 0
			case "budget":
				want, cost = "partial", 30
			case "git config", "alternate index":
				want, cost = "skipped", 0
				if calls.Load() != 0 || len(report.Errors) != 1 || report.SessionID != combined.SessionID || report.ArtifactDir == "" {
					test.Fatalf("unsupported view: %+v", report)
				}
			}
			if report.Status != want || report.Usage.TotalTokens != cost {
				test.Fatalf("analysis status or usage: %+v", report)
			}
			var stored behaviortask.SavedResult
			data, err := os.ReadFile(filepath.Join(report.ArtifactDir, "result.json"))
			if err != nil || json.Unmarshal(data, &stored) != nil || stored.Status != want || stored.SessionID != combined.SessionID {
				test.Fatalf("persisted result: %s, %v", data, err)
			}
			for _, result := range []*jsonOutput{&plain, &combined} {
				result.SchemaVersion, result.ReviewStatus, result.SessionID, result.TraceID = "", "", "", ""
				result.Analysis, result.AnalysisRaw, result.AnalysisMeta = nil, nil, nil
				result.Manifest.RunID = ""
				result.Manifest.ElapsedMS = 0
				result.Summary.Elapsed = ""
			}
			if !reflect.DeepEqual(plain, combined) {
				test.Fatalf("Review changed with Analysis:\nplain=%+v\ncombined=%+v", plain, combined)
			}
		})
	}
}

func TestReviewWithAnalysisCancellationAfterReview(test *testing.T) {
	reviewDir := filepath.Join(retryTestRepo(test), "nested")
	if err := os.Mkdir(reviewDir, 0o700); err != nil {
		test.Fatal(err)
	}
	sessionRepo, err := resolveRepoDir(reviewDir)
	if err != nil {
		test.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(test.Context(), 10*time.Second)
	defer cancel()
	startFakeLLM(test, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for ctx.Err() == nil {
			summaries, err := session.ListSessions(sessionRepo)
			if err != nil {
				test.Error(err)
				break
			}
			for _, summary := range summaries {
				if summary.RunManifest != nil && summary.RunManifest.TerminalState == session.StateSkipped {
					cancel()
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	opts, err := parseReviewFlags([]string{"--repo", reviewDir, "--from", "HEAD", "--analysis", "--format", "json", "--audience", "agent"})
	if err != nil {
		test.Fatal(err)
	}
	stdout := captureStdout(test, func() { err = executeReviewContext(ctx, opts) })
	if !errors.Is(err, context.Canceled) {
		test.Fatalf("command cancellation after Review finished: %v", err)
	}
	var payload jsonOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil || payload.Status != "skipped" || payload.AnalysisMeta == nil || payload.AnalysisMeta.Status != "failed" {
		test.Fatalf("expected skipped Review and interrupted Analysis on stdout: %s (%v)", stdout, err)
	}
}

func TestAnalysisSkippedReviewStatus(test *testing.T) {
	for _, fixture := range []struct {
		name            string
		failPersistence bool
		wantReview      string
	}{
		{name: "session_persistence_failure", failPersistence: true, wantReview: "partial"},
		{name: "successful_skip", wantReview: "skipped"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			freshOCRHome(test)
			repo := initTestGitRepo(test)
			gitCommitFile(test, repo, "main.go", "package main\n", "Seed empty comparison")
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
			reviewer := agent.New(agent.Args{RepoDir: repo, From: "HEAD", To: "HEAD"})
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
			combined := &combinedResult{Agent: reviewer, reviewErr: reviewErr, report: &workflow.Report{Status: "completed", Markdown: "Analysis result."}}
			output := combined.AnalysisOutput()
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
			if err := outputJSONWithWarnings(comments, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, reviewer.SessionID(), manifest, false, nil, &machine, nil, nil, nil, output); err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
				test.Fatal(err)
			}
			if payload.Status != "skipped" || payload.ReviewStatus != fixture.wantReview {
				test.Errorf("JSON status mismatch: %s", machine.String())
			}
			if payload.Manifest == nil || payload.Manifest.TerminalState != session.StateSkipped || reviewer.RunManifest().TerminalState != session.StateSkipped {
				test.Fatalf("frozen skipped manifest changed: %s", machine.String())
			}
		})
	}
}

func TestAnalysisTextAndStatus(test *testing.T) {
	for _, fixture := range []struct{ review, analysis string }{
		{"complete", "completed"},
		{"skipped", "failed"},
		{"failed", "completed"},
		{"failed", "failed"},
		{"partial", "completed"},
		{"partial", "partial"},
		{"complete", "failed"},
		{"complete", "partial"},
	} {
		output := &analysisOutput{ReviewStatus: fixture.review, Report: &workflow.Report{Status: fixture.analysis, DeclaredCompletion: "complete"}}
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
		manifest := &session.RunManifest{TerminalState: session.TerminalState(fixture.review)}
		if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", manifest, false, nil, &machine, nil, nil, nil, output); err != nil {
			test.Fatal(err)
		}
		var payload jsonOutput
		if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
			test.Fatal(err)
		}
		if payload.ReviewStatus != fixture.review || payload.Status != fixture.review || payload.AnalysisMeta == nil || payload.AnalysisMeta.Status != fixture.analysis || payload.AnalysisMeta.DeclaredCompletion != "complete" {
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

func TestAnalysisWithoutReviewManifest(test *testing.T) {
	output := &analysisOutput{ReviewStatus: "failed", Report: &workflow.Report{Status: "completed", Markdown: "Analysis result."}}
	var machine bytes.Buffer
	if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", nil, false, nil, &machine, nil, nil, nil, output); err != nil {
		test.Fatal(err)
	}
	var payload jsonOutput
	if err := json.Unmarshal(machine.Bytes(), &payload); err != nil || payload.Status != "failed" || payload.Analysis == nil || *payload.Analysis != output.Report.Markdown {
		test.Fatalf("expected failed Review and available Analysis: %s (%v)", machine.String(), err)
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
			if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", nil, false, nil, &machine, nil, nil, nil, combined); err != nil {
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
			const display = "## Behavior\n\nThe default changes.\n"
			report := &workflow.Report{
				Status: "completed", Completed: true, DeclaredCompletion: completion,
				Comparison: behaviortask.Comparison{Mode: "range", Old: strings.Repeat("a", 40), Head: strings.Repeat("b", 40)},
				StopReason: "none", SubmissionStatus: "accepted", ArtifactDir: "evidence/run",
				Markdown: markdown,
				Usage:    behaviortask.Usage{TotalTokens: 1910877, InputTokens: 1895278, OutputTokens: 15599, CacheReadTokens: 987654},
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
			want := "\n──────── Analysis Report ────────\nComparison: aaaaaaaa -> bbbbbbbb\n\n" + display + "\nAnalysis: completed\nAnalysis tokens: ~1910877 (input: ~1895278, output: ~15599), cache(read: ~987654, write: ~0)\n"
			if human.String() != want {
				test.Fatalf("unexpected brief:\ngot: %q\nwant: %q", human.String(), want)
			}
			var machine bytes.Buffer
			if err := outputJSONWithWarnings(nil, nil, 0, 0, 0, 0, 0, 0, 0, "", nil, nil, "", nil, "", nil, false, nil, &machine, nil, nil, nil, combined); err != nil {
				test.Fatal(err)
			}
			var payload jsonOutput
			if err := json.Unmarshal(machine.Bytes(), &payload); err != nil {
				test.Fatal(err)
			}
			after, err := json.Marshal(report)
			if payload.Analysis == nil || *payload.Analysis != display || payload.AnalysisRaw == nil || *payload.AnalysisRaw != markdown {
				test.Fatalf("unexpected display or raw Markdown in JSON: %s", machine.String())
			}
			if payload.AnalysisMeta == nil || payload.AnalysisMeta.ArtifactDir != report.ArtifactDir {
				test.Fatalf("missing evidence directory in JSON: %s", machine.String())
			}
			if err != nil || !bytes.Equal(before, after) || report.Markdown != markdown || !reflect.DeepEqual(payload.AnalysisMeta.RetryReport, report.RetryReport) || payload.AnalysisMeta.Usage != report.Usage || payload.AnalysisMeta.Comparison != report.Comparison {
				test.Fatalf("text output changed raw report or JSON: %s, %v", machine.String(), err)
			}
		})
	}
}

func TestAnalysisTextEvidence(test *testing.T) {
	for _, fixture := range []struct {
		status, completion string
		wantEvidence       bool
	}{
		{"completed", "partial", true},
		{"partial", "complete", true},
		{"failed", "unknown", true},
		{"skipped", "unknown", false},
	} {
		test.Run(fixture.status, func(test *testing.T) {
			report := &workflow.Report{Status: fixture.status, DeclaredCompletion: fixture.completion, ArtifactDir: "evidence/run"}
			var human bytes.Buffer
			outputAnalysisText(&human, &analysisOutput{Report: report})
			if strings.Contains(human.String(), "\nEvidence: evidence/run\n") != fixture.wantEvidence {
				test.Fatalf("unexpected evidence path visibility: %s", human.String())
			}
		})
	}
}

func TestAnalysisTextUsage(test *testing.T) {
	for _, fixture := range []struct {
		status string
		usage  behaviortask.Usage
		want   string
	}{
		{"partial", behaviortask.Usage{TotalTokens: 110, InputTokens: 80, OutputTokens: 25}, "Analysis tokens: ~110 (input: ~80, output: ~25)"},
		{"failed", behaviortask.Usage{TotalTokens: 19}, "Analysis tokens: ~19"},
		{"skipped", behaviortask.Usage{}, ""},
	} {
		test.Run(fixture.status, func(test *testing.T) {
			var human bytes.Buffer
			outputAnalysisText(&human, &analysisOutput{Report: &workflow.Report{Status: fixture.status, Usage: fixture.usage}})
			var got string
			for line := range strings.SplitSeq(human.String(), "\n") {
				if strings.HasPrefix(line, "Analysis tokens:") {
					got = line
					break
				}
			}
			if got != fixture.want {
				test.Fatalf("got %q, want %q", got, fixture.want)
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
