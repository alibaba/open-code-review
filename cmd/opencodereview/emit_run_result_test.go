// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

type mockResultProvider struct {
	diffs            []model.Diff
	filesReviewed    int64
	inputTokens      int64
	outputTokens     int64
	totalTokens      int64
	cacheReadTokens  int64
	cacheWriteTokens int64
	warnings         []agent.AgentWarning
	projectSummary   string
	toolCalls        map[string]int64
	resumeInfo       *agent.ResumeInfo
	sessionID        string
	budgetExceeded   bool
	manifest         *session.RunManifest
}

func (m *mockResultProvider) Diffs() []model.Diff               { return m.diffs }
func (m *mockResultProvider) FilesReviewed() int64              { return m.filesReviewed }
func (m *mockResultProvider) TotalInputTokens() int64           { return m.inputTokens }
func (m *mockResultProvider) TotalOutputTokens() int64          { return m.outputTokens }
func (m *mockResultProvider) TotalTokensUsed() int64            { return m.totalTokens }
func (m *mockResultProvider) TotalCacheReadTokens() int64       { return m.cacheReadTokens }
func (m *mockResultProvider) TotalCacheWriteTokens() int64      { return m.cacheWriteTokens }
func (m *mockResultProvider) Warnings() []agent.AgentWarning    { return m.warnings }
func (m *mockResultProvider) ProjectSummary() string            { return m.projectSummary }
func (m *mockResultProvider) ToolCalls() map[string]int64       { return m.toolCalls }
func (m *mockResultProvider) ResumeInfo() *agent.ResumeInfo     { return m.resumeInfo }
func (m *mockResultProvider) SessionID() string                 { return m.sessionID }
func (m *mockResultProvider) BudgetExceeded() bool              { return m.budgetExceeded }
func (m *mockResultProvider) RunManifest() *session.RunManifest { return m.manifest }

func mockManifest(state session.TerminalState) *session.RunManifest {
	a := session.CoverageItem{ItemID: "a", Path: "a.go", Fingerprint: "fp-a"}
	b := session.CoverageItem{ItemID: "b", Path: "b.go", Fingerprint: "fp-b"}
	m := &session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		RunID:         "run-1",
		Operation:     session.OperationReview,
		TerminalState: state,
		Input:         session.ManifestInput{Mode: session.InputModeWorkspace},
		Coverage: session.Coverage{
			Selected:  []session.CoverageItem{},
			Completed: []session.CoverageItem{},
			Reused:    []session.CoverageItem{},
			Failed:    []session.CoverageItem{},
			Waived:    []session.CoverageItem{},
		},
	}
	switch state {
	case session.StateComplete:
		m.Coverage.Selected = []session.CoverageItem{a, b}
		m.Coverage.Completed = []session.CoverageItem{a, b}
	case session.StatePartial:
		b.Classification = session.FailureProvider
		m.Coverage.Selected = []session.CoverageItem{a, b}
		m.Coverage.Completed = []session.CoverageItem{a}
		m.Coverage.Failed = []session.CoverageItem{b}
	case session.StateFailed:
		a.Classification = session.FailureProvider
		b.Classification = session.FailureTimeout
		m.Coverage.Selected = []session.CoverageItem{a, b}
		m.Coverage.Failed = []session.CoverageItem{a, b}
	}
	return m
}
func TestEmitRunResult_JSONNoFiles(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 0}
	identity := &jsonLLMIdentity{Provider: "anthropic", Model: "claude-opus-4-6"}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, identity, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "skipped" {
		t.Errorf("status = %q, want skipped", out.Status)
	}
	if out.LLM == nil || out.LLM.Provider != "anthropic" || out.LLM.Model != "claude-opus-4-6" {
		t.Fatalf("llm = %+v", out.LLM)
	}
}

func TestEmitRunResult_JSONLLMIdentityNamedProvider(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	identity := &jsonLLMIdentity{Provider: "anthropic", Model: "claude-opus-4-6"}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, identity, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.LLM == nil || out.LLM.Provider != "anthropic" || out.LLM.Model != "claude-opus-4-6" {
		t.Fatalf("llm = %+v", out.LLM)
	}
}

func TestEmitRunResult_JSONLLMIdentityOmitsUnknownProvider(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	identity := &jsonLLMIdentity{Model: "gpt-5-codex"}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, identity, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.LLM == nil || out.LLM.Model != "gpt-5-codex" || out.LLM.Provider != "" {
		t.Fatalf("llm = %+v", out.LLM)
	}
	if strings.Contains(got, `"provider"`) {
		t.Fatalf("provider key must be omitted: %s", got)
	}
}

func TestEmitRunResult_JSONUsesManifestTerminalState(t *testing.T) {
	manifest := mockManifest(session.StatePartial)
	ag := &mockResultProvider{
		filesReviewed: 2,
		manifest:      manifest,
		warnings: []agent.AgentWarning{
			{Type: "subtask_error", File: "b.go", Message: "provider rejected api_key=LEAKED at /Users/example/private"},
			{Type: "info", File: "a.go", Message: "retry used"},
		},
	}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != string(session.StatePartial) {
		t.Fatalf("status = %q, want partial", out.Status)
	}
	if out.Manifest == nil || out.Manifest.RunID != manifest.RunID {
		t.Fatalf("manifest = %+v", out.Manifest)
	}
	if strings.Contains(out.Message, "Looks good") {
		t.Fatalf("partial message must not claim success: %q", out.Message)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Type != "info" {
		t.Fatalf("published warnings = %+v, want only non-coverage warning", out.Warnings)
	}
	if strings.Contains(got, "LEAKED") || strings.Contains(got, "/Users/example/private") {
		t.Fatalf("manifest JSON exposed raw subtask warning: %q", got)
	}
}

func TestEmitRunResult_JSONSkippedIncludesManifest(t *testing.T) {
	ag := &mockResultProvider{manifest: mockManifest(session.StateSkipped)}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != string(session.StateSkipped) || out.Manifest == nil {
		t.Fatalf("output = %+v", out)
	}
	if !strings.Contains(out.Message, "no items were selected") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestEmitRunResult_JSONManifestMatchesPersistedSessionEnd(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Operation:  session.OperationReview,
	})
	builder := sh.Manifest()
	builder.SetInput(session.ManifestInput{Mode: session.InputModeRange, RequestedFrom: "main", RequestedHead: "feature"})
	a := session.CoverageItem{ItemID: "a", Path: "a.go", Fingerprint: "fp-a"}
	b := session.CoverageItem{ItemID: "b", Path: "b.go", Fingerprint: "fp-b"}
	if err := builder.RegisterSelected(a); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := builder.RegisterSelected(b); err != nil {
		t.Fatalf("register b: %v", err)
	}
	if err := builder.SealSelected(); err != nil {
		t.Fatalf("seal selected: %v", err)
	}
	if err := builder.MarkCompleted(a.ItemID); err != nil {
		t.Fatalf("complete a: %v", err)
	}
	if err := builder.MarkFailed(b.ItemID, session.FailureProvider, "provider or subtask request failed"); err != nil {
		t.Fatalf("fail b: %v", err)
	}
	manifest, err := builder.Finalize(0)
	if err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	sh.SetFinalManifest(&manifest)
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}

	ag := &mockResultProvider{filesReviewed: 2, sessionID: sh.SessionID, manifest: sh.FinalManifest()}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	// Capture each exit's manifest as the raw bytes it actually emitted, not a
	// struct round-trip: re-marshaling both sides through session.RunManifest would
	// normalize away any real serialization divergence between the two exits. The
	// contract is "same source, canonicalized equal" — compact only the
	// insignificant whitespace, then compare bytes.
	var cliOut struct {
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(got), &cliOut); err != nil {
		t.Fatalf("unmarshal CLI output: %v", err)
	}
	if len(cliOut.Manifest) == 0 {
		t.Fatal("CLI output carried no manifest")
	}

	path, err := session.SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var persisted struct {
		Type        string          `json:"type"`
		RunManifest json.RawMessage `json:"run_manifest"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &persisted); err != nil {
		t.Fatalf("unmarshal session_end: %v", err)
	}
	if persisted.Type != "session_end" || len(persisted.RunManifest) == 0 {
		t.Fatalf("last record has no run_manifest: type=%q", persisted.Type)
	}

	var cliCompact, persistedCompact bytes.Buffer
	if err := json.Compact(&cliCompact, cliOut.Manifest); err != nil {
		t.Fatalf("compact CLI manifest: %v", err)
	}
	if err := json.Compact(&persistedCompact, persisted.RunManifest); err != nil {
		t.Fatalf("compact persisted manifest: %v", err)
	}
	if !bytes.Equal(cliCompact.Bytes(), persistedCompact.Bytes()) {
		t.Fatalf("manifest bytes differ after canonicalization\nCLI:       %s\npersisted: %s",
			cliCompact.Bytes(), persistedCompact.Bytes())
	}
}

func TestEmitRunResult_JSONWithComments(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 3,
		inputTokens:   100,
		outputTokens:  50,
		totalTokens:   150,
		warnings:      []agent.AgentWarning{{Type: "info", Message: "note"}},
		toolCalls:     map[string]int64{"file_read": 2},
	}
	comments := []model.LlmComment{{Path: "main.go", Content: "fix", StartLine: 1, EndLine: 2}}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, comments, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(out.Comments))
	}
	if out.Summary == nil || out.Summary.FilesReviewed != 3 {
		t.Errorf("summary.FilesReviewed = %v", out.Summary)
	}
}

func TestEmitRunResult_JSONWithResumeInfo(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 2,
		resumeInfo: &agent.ResumeInfo{
			ResumedFrom:   "old-session",
			ReusedFiles:   1,
			RerunFiles:    1,
			PreviousModel: "anthropic-model",
			CurrentModel:  "openai-model",
		},
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Resume == nil || out.Resume.ResumedFrom != "old-session" || out.Resume.ReusedFiles != 1 || out.Resume.RerunFiles != 1 {
		t.Fatalf("resume = %+v", out.Resume)
	}
}

func TestEmitRunResult_TextNoComments(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "Looks good to me") {
		t.Errorf("expected 'Looks good to me', got %q", got)
	}
}

func TestEmitRunResult_TextPartialNeverLooksGood(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, manifest: mockManifest(session.StatePartial)}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	if !strings.Contains(got, "partially complete") || strings.Contains(got, "Looks good") {
		t.Fatalf("partial output = %q", got)
	}
}

func TestEmitRunResult_TextCompleteReportsFindingsAndWaived(t *testing.T) {
	manifest := mockManifest(session.StateComplete)
	b := manifest.Coverage.Completed[1]
	manifest.Coverage.Completed = manifest.Coverage.Completed[:1]
	b.Reason = "accepted"
	manifest.Coverage.Waived = []session.CoverageItem{b}
	ag := &mockResultProvider{filesReviewed: 2, manifest: manifest}
	comments := []model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1, EndLine: 1}}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, comments, time.Now(), "text", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	for _, want := range []string{"Review complete: 1 finding(s)", "including 1 waived", "a.go"} {
		if !strings.Contains(got, want) {
			t.Fatalf("complete output missing %q: %q", want, got)
		}
	}
}

func TestEmitRunResult_TextDoesNotPrintSuccessfulSessionHint(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, sessionID: "session-123"}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if strings.Contains(got, "session-123") || strings.Contains(got, "--resume") {
		t.Fatalf("successful text output should not print session ID or resume hint, got %q", got)
	}
}

func TestEmitRunResult_TextWithComments(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	comments := []model.LlmComment{{Path: "a.go", Content: "rename", StartLine: 5, EndLine: 10}}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, comments, time.Now(), "text", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "a.go") {
		t.Errorf("expected path, got %q", got)
	}
	if !strings.Contains(got, "rename") {
		t.Errorf("expected comment content, got %q", got)
	}
}

func TestEmitRunResult_TextWithProjectSummary(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed:  5,
		projectSummary: "All tests pass, code quality is good.",
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "Project Summary") {
		t.Errorf("expected 'Project Summary', got %q", got)
	}
	if !strings.Contains(got, "All tests pass") {
		t.Errorf("expected summary content, got %q", got)
	}
}

func TestEmitRunResult_AgentTextRestoresQuiet(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	q := newQuietHandle("text", "agent")
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "agent", q, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if q.fn != nil {
		t.Error("expected quiet handle to be restored")
	}
	_ = got
}

func TestEmitRunResult_AgentJSONDoesNotRestore(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 1,
		inputTokens:   10,
		outputTokens:  5,
		totalTokens:   15,
	}
	q := newQuietHandle("json", "agent")
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "agent", q, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	q.Restore()
}

func TestEmitRunResult_NilQuietHandle(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "agent", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	_ = got
}

func TestEmitRunResult_JSONTraceIDFromContext(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-root")
	wantTraceID := span.SpanContext().TraceID().String()
	defer span.End()

	ag := &mockResultProvider{
		filesReviewed: 2,
		inputTokens:   10,
		outputTokens:  5,
		totalTokens:   15,
	}
	got := captureStdout(t, func() {
		err := emitRunResult(ctx, ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TraceID != wantTraceID {
		t.Errorf("trace_id = %q, want %q", out.TraceID, wantTraceID)
	}
}

func TestEmitRunResult_JSONNoFilesTraceID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-root")
	wantTraceID := span.SpanContext().TraceID().String()
	defer span.End()

	ag := &mockResultProvider{filesReviewed: 0}
	got := captureStdout(t, func() {
		err := emitRunResult(ctx, ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "skipped" {
		t.Errorf("status = %q, want skipped", out.Status)
	}
	if out.TraceID != wantTraceID {
		t.Errorf("trace_id = %q, want %q", out.TraceID, wantTraceID)
	}
}

func TestEmitRunResult_JSONIncludesSessionID(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1, sessionID: "session-99"}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SessionID != "session-99" {
		t.Errorf("session_id = %q, want session-99", out.SessionID)
	}
}

// --- retry report at the emit boundary (#368 P5) ---
// These cases pin how a frozen retry report reaches the two run exits,
// emitRunResult and emitFailureUsage. The report's own rendering is covered by
// retry_report_output_test.go; retryReportFixture lives there too.

func TestEmitRunResult_JSONCarriesRetryReport(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, manifest: mockManifest(session.StateComplete)}
	rep := retryReportFixture()
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, rep); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RetryReport == nil {
		t.Fatalf("retry_report missing from JSON output: %s", got)
	}
	if out.RetryReport.SchemaVersion != llm.RetryReportSchemaVersion {
		t.Errorf("schema_version = %q", out.RetryReport.SchemaVersion)
	}
	if out.RetryReport.TotalRequests != 12 || out.RetryReport.FailedRequests != 1 {
		t.Errorf("aggregates not carried through: %+v", out.RetryReport)
	}
}

// A run with nothing to report must emit exactly the pre-#368 JSON shape.
func TestEmitRunResult_JSONOmitsRetryReportWhenNil(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, manifest: mockManifest(session.StateComplete)}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	if strings.Contains(got, "retry_report") {
		t.Errorf("nil report must not appear in JSON, got %s", got)
	}
}

// Order is part of the terminal contract: comments/manifest first, then the
// retry report, then the project summary.
func TestEmitRunResult_TextReportOrder(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed:  2,
		manifest:       mockManifest(session.StateComplete),
		projectSummary: "PROJECT-SUMMARY-MARKER",
	}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, retryReportFixture()); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	report := strings.Index(got, "LLM retry report summary:")
	summary := strings.Index(got, "PROJECT-SUMMARY-MARKER")
	if report < 0 {
		t.Fatalf("report missing from text output: %s", got)
	}
	if summary < 0 || report > summary {
		t.Errorf("report must precede the project summary\n%s", got)
	}
	if !strings.Contains(got, "rejected by provider (HTTP 402) -> failed") {
		t.Errorf("per-request lines missing: %s", got)
	}
}

func TestEmitRunResult_TextOmitsReportWhenNil(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, manifest: mockManifest(session.StateComplete)}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	if strings.Contains(got, "LLM retry report summary") {
		t.Errorf("nil report must print nothing, got %s", got)
	}
}

// JSON mode must keep stdout a single JSON document, so the text renderer never
// runs there.
func TestEmitRunResult_JSONHasNoReportText(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, manifest: mockManifest(session.StateComplete)}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, retryReportFixture()); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	if strings.Contains(got, "LLM retry report summary:") {
		t.Errorf("JSON mode must not emit the terminal summary: %s", got)
	}
	dec := json.NewDecoder(strings.NewReader(got))
	var first jsonOutput
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.More() {
		t.Error("stdout must carry exactly one JSON document")
	}
}

func TestEmitFailureUsage_JSONCarriesRetryReport(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1, sessionID: "sess-1"}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, time.Second, "json", nil, retryReportFixture())
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, got)
	}
	if out.Status != "failed" {
		t.Errorf("status = %q, want failed", out.Status)
	}
	if out.RetryReport == nil || out.RetryReport.FailedRequests != 1 {
		t.Fatalf("retry_report missing or wrong on the failure exit: %s", got)
	}
}

func TestEmitFailureUsage_TextCarriesRetryReport(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, time.Second, "text", nil, retryReportFixture())
	})
	usage := strings.Index(got, "[ocr] usage on failure:")
	report := strings.Index(got, "LLM retry report summary:")
	if usage < 0 || report < 0 || usage > report {
		t.Errorf("report must follow the usage line on stderr:\n%s", got)
	}
}

func TestEmitFailureUsage_NilReportUnchanged(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 1}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, time.Second, "text", nil, nil)
	})
	if strings.Contains(got, "LLM retry report summary") {
		t.Errorf("nil report must print nothing, got %q", got)
	}
}

// warningsForOutput is unrelated to the report, but the text exit now writes
// between the warnings block and the project summary; keep a case where both
// warnings and a report are present so the two cannot interleave.
func TestEmitRunResult_TextReportWithWarnings(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 1,
		warnings:      []agent.AgentWarning{{Type: "subtask_error", File: "b.go", Message: "boom"}},
	}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, os.Stdout, retryReportFixture()); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	if !strings.Contains(got, "LLM retry report summary:") {
		t.Errorf("report missing: %s", got)
	}
	if strings.Count(got, "LLM retry report summary:") != 1 {
		t.Errorf("report emitted more than once: %s", got)
	}
}
