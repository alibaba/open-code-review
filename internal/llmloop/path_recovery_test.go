// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type flakyFileFindProvider struct {
	calls int
}

func (p *flakyFileFindProvider) Tool() tool.Tool { return tool.FileFind }

func (p *flakyFileFindProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	p.calls++
	if p.calls == 1 {
		return "", errors.New("temporary file_find failure")
	}
	return "config/project.yaml", nil
}

func setupPathRecoveryRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "project.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old", "legacy.c"), []byte("int legacy;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")
	if err := os.Remove(filepath.Join(dir, "old", "legacy.c")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "new", "location"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new", "location", "settings.go"), []byte("package location\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "project.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "-A")
	run("git", "commit", "-m", "delete legacy and move settings")
	return dir, run("git", "rev-parse", "HEAD")
}

func fileReadToolCallsResponse(paths ...string) *llm.ChatResponse {
	content := ""
	calls := make([]llm.ToolCall, 0, len(paths))
	for i, path := range paths {
		calls = append(calls, llm.ToolCall{
			ID:   "call_" + string(rune('1'+i)),
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "file_read",
				Arguments: `{"file_path":` + mustJSONQuote(path) + `}`,
			},
		})
	}
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content, ToolCalls: calls}}},
		Model:   "fake",
		Usage:   &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 10},
	}
}

func mustJSONQuote(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func newPathRecoveryRunner(t *testing.T, client *fakeClient, maxRounds int) (*Runner, *tool.CommentCollector) {
	t.Helper()
	dir, commit := setupPathRecoveryRepo(t)
	fr := &tool.FileReader{RepoDir: dir, Mode: tool.ModeCommit, Ref: commit}
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Register(tool.NewFileFind(fr))
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()
	return NewRunner(Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: maxRounds},
		Tools:            reg,
		MainToolDefs:     pathRecoveryToolDefs(),
		CommentCollector: collector,
		Session:          session.New(dir, commit, "fake", session.SessionOptions{}),
	}), collector
}

func pathRecoveryToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
		{Type: "function", Function: llm.FunctionDef{Name: "file_find"}},
		{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
		{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
	}
}

func TestRunPerFile_RecoversCandidatesAfterRepeatedMissingPath(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"file_path":"DOC/cloud/project.yct"}`),
		fileReadToolCallResponse("call_2", `{"file_path":"DOC/cloud/project.yct"}`),
		fileReadToolCallResponse("call_3", `{"file_path":"DOC/cloud/project.yct"}`),
		fileReadToolCallResponse("call_4", `{"file_path":"config/project.yaml"}`),
		codeCommentResponse("", ""),
		taskDoneResponse(),
	}}
	defs := pathRecoveryToolDefs()
	runner, collector := newPathRecoveryRunner(t, client, 3)

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this commit")},
		"config/project.yaml",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed || stop != StopNone {
		t.Fatalf("RunPerFile completed=%t stop=%v, want completed with StopNone", completed, stop)
	}
	if client.calls != 6 {
		t.Fatalf("LLM calls = %d, want 3 rejected reads, a valid read, a comment, and task_done", client.calls)
	}

	validReadRequest := client.requests[3]
	var history strings.Builder
	for i := range validReadRequest.Messages {
		history.WriteString(validReadRequest.Messages[i].ExtractText())
		history.WriteByte('\n')
	}
	if !strings.Contains(history.String(), "config/project.yaml") {
		t.Fatalf("recovery history does not contain candidate path:\n%s", history.String())
	}
	comments := collector.Comments()
	if len(comments) != 1 || comments[0].Path != "config/project.yaml" {
		t.Fatalf("comments = %+v, want one finding on the reviewed file", comments)
	}
	for i, req := range client.requests {
		if len(req.Tools) != len(defs) {
			t.Fatalf("request %d tools = %d, want unchanged %d", i, len(req.Tools), len(defs))
		}
		for j := range defs {
			if req.Tools[j].Function.Name != defs[j].Function.Name {
				t.Fatalf("request %d tool %d = %q, want %q", i, j, req.Tools[j].Function.Name, defs[j].Function.Name)
			}
		}
	}
}

func TestRunPerFile_RecoversInventedDeletedAndRenamedPaths(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallsResponse(
			`DOC\cloud\project.yct`,
			"old/legacy.c",
			"old/settings.go",
		),
		taskDoneResponse(),
	}}
	runner, _ := newPathRecoveryRunner(t, client, 1)

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"config/project.yaml",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed || stop != StopNone {
		t.Fatalf("RunPerFile completed=%t stop=%v, want completed with StopNone", completed, stop)
	}
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2", client.calls)
	}

	var history strings.Builder
	for i := range client.requests[1].Messages {
		history.WriteString(client.requests[1].Messages[i].ExtractText())
		history.WriteByte('\n')
	}
	got := history.String()
	for _, want := range []string{
		`Rejected "DOC/cloud/project.yct"`,
		"config/project.json",
		"config/project.yaml",
		`Rejected "old/legacy.c"`,
		"No candidate path was found",
		`Rejected "old/settings.go"`,
		"new/location/settings.go",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("recovery history missing %q:\n%s", want, got)
		}
	}
}

func TestRunPerFile_SuccessfulReadResetsMissingPathBatch(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"file_path":"missing/one.go"}`),
		fileReadToolCallResponse("call_2", `{"file_path":"missing/two.go"}`),
		fileReadToolCallResponse("call_3", `{"file_path":"config/project.yaml"}`),
		fileReadToolCallResponse("call_4", `{"file_path":"missing/three.go"}`),
		taskDoneResponse(),
	}}
	runner, _ := newPathRecoveryRunner(t, client, 5)

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"config/project.yaml",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed || stop != StopNone {
		t.Fatalf("RunPerFile completed=%t stop=%v, want completed with StopNone", completed, stop)
	}
	for i, req := range client.requests {
		for j := range req.Messages {
			if strings.Contains(req.Messages[j].ExtractText(), "OCR path recovery") {
				t.Fatalf("request %d unexpectedly contains recovery after a successful read reset", i)
			}
		}
	}
}

func TestRunPerFile_MissingPathRefundIsBounded(t *testing.T) {
	responses := make([]*llm.ChatResponse, 0, 6)
	for i := 0; i < 5; i++ {
		responses = append(responses, fileReadToolCallResponse(
			"call_"+string(rune('1'+i)),
			`{"file_path":"missing/forever.go"}`,
		))
	}
	responses = append(responses, taskDoneResponse())
	client := &fakeClient{responses: responses}
	runner, _ := newPathRecoveryRunner(t, client, 2)

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"config/project.yaml",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("persistent invalid reads must not be reported as complete")
	}
	if stop != StopMaxRounds {
		t.Fatalf("stop = %v, want StopMaxRounds", stop)
	}
	if client.calls != 6 {
		t.Fatalf("LLM calls = %d, want 5 main rounds plus 1 grace round", client.calls)
	}
}

func TestRecoverInvalidPathsDoesNotCacheFinderErrors(t *testing.T) {
	finder := &flakyFileFindProvider{}
	reg := tool.NewRegistry()
	reg.Register(finder)
	runner := NewRunner(Deps{Tools: reg})
	state := newInvalidPathRecovery()

	for range invalidPathRecoveryThreshold {
		state.observe("missing/project.yaml")
	}
	first := runner.recoverInvalidPaths(context.Background(), state)
	if !strings.Contains(first, "Candidate search failed") {
		t.Fatalf("first recovery = %q, want search failure guidance", first)
	}
	if _, cached := state.candidateCache["missing/project.yaml"]; cached {
		t.Fatal("failed candidate search must not be cached")
	}

	state.finishBatch()
	for range invalidPathRecoveryThreshold {
		state.observe("missing/project.yaml")
	}
	second := runner.recoverInvalidPaths(context.Background(), state)
	if !strings.Contains(second, "config/project.yaml") || finder.calls != 2 {
		t.Fatalf("second recovery = %q, calls = %d; want a fresh successful search", second, finder.calls)
	}
}
