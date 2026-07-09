package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/model"
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
}

func (m *mockResultProvider) Diffs() []model.Diff            { return m.diffs }
func (m *mockResultProvider) FilesReviewed() int64           { return m.filesReviewed }
func (m *mockResultProvider) TotalInputTokens() int64        { return m.inputTokens }
func (m *mockResultProvider) TotalOutputTokens() int64       { return m.outputTokens }
func (m *mockResultProvider) TotalTokensUsed() int64         { return m.totalTokens }
func (m *mockResultProvider) TotalCacheReadTokens() int64    { return m.cacheReadTokens }
func (m *mockResultProvider) TotalCacheWriteTokens() int64   { return m.cacheWriteTokens }
func (m *mockResultProvider) Warnings() []agent.AgentWarning { return m.warnings }
func (m *mockResultProvider) ProjectSummary() string         { return m.projectSummary }
func (m *mockResultProvider) ToolCalls() map[string]int64    { return m.toolCalls }
func (m *mockResultProvider) ResumeInfo() *agent.ResumeInfo  { return m.resumeInfo }
func (m *mockResultProvider) SessionID() string              { return m.sessionID }

func TestEmitRunResult_JSONNoFiles(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 0}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, comments, time.Now(), "json", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "Looks good to me") {
		t.Errorf("expected 'Looks good to me', got %q", got)
	}
}

func TestEmitRunResult_TextDoesNotPrintSuccessfulSessionHint(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2, sessionID: "session-123"}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, comments, time.Now(), "text", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "agent", q)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "agent", q)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "agent", nil)
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
		err := emitRunResult(ctx, ag, nil, time.Now(), "json", "developer", nil)
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
		err := emitRunResult(ctx, ag, nil, time.Now(), "json", "developer", nil)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil)
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
