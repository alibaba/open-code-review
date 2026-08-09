// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/config/toolsconfig"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type fakeAgentClient struct {
	responses []*llm.ChatResponse
	calls     int
}

func (f *fakeAgentClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if f.calls >= len(f.responses) {
		content := ""
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
			Model:   "fake",
		}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func agentTaskDoneResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_done",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: `{}`,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}
}

func codeCommentResponse(path string) *llm.ChatResponse {
	content := ""
	args := map[string]any{
		"path": path,
		"comments": []any{
			map[string]any{
				"content":       "potential null pointer",
				"existing_code": "foo := bar.Baz()",
			},
		},
	}
	argsJSON, _ := json.Marshal(args)
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_comment",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: string(argsJSON),
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 50, CompletionTokens: 20},
	}
}

func TestBuildFilterCommentsJSON(t *testing.T) {
	tests := []struct {
		name     string
		comments []model.LlmComment
		wantIDs  []string
	}{
		{
			name:     "empty slice",
			comments: nil,
			wantIDs:  nil,
		},
		{
			name: "single comment",
			comments: []model.LlmComment{
				{Content: "fix this", ExistingCode: "old code"},
			},
			wantIDs: []string{"c-0"},
		},
		{
			name: "multiple comments sequential IDs",
			comments: []model.LlmComment{
				{Content: "issue A"},
				{Content: "issue B", ExistingCode: "existing"},
				{Content: "issue C"},
			},
			wantIDs: []string{"c-0", "c-1", "c-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFilterCommentsJSON(tt.comments)

			var items []struct {
				ID           string `json:"id"`
				Content      string `json:"content"`
				ExistingCode string `json:"existing_code,omitempty"`
			}
			if err := json.Unmarshal([]byte(got), &items); err != nil {
				t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
			}

			if len(items) != len(tt.comments) {
				t.Fatalf("len = %d, want %d", len(items), len(tt.comments))
			}

			for i, item := range items {
				if tt.wantIDs != nil && item.ID != tt.wantIDs[i] {
					t.Errorf("items[%d].ID = %q, want %q", i, item.ID, tt.wantIDs[i])
				}
				if item.Content != tt.comments[i].Content {
					t.Errorf("items[%d].Content = %q, want %q", i, item.Content, tt.comments[i].Content)
				}
				if item.ExistingCode != tt.comments[i].ExistingCode {
					t.Errorf("items[%d].ExistingCode = %q, want %q", i, item.ExistingCode, tt.comments[i].ExistingCode)
				}
			}
		})
	}
}

func TestParseFilterResponse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		total   int
		wantSet map[int]struct{}
	}{
		{
			name:    "valid JSON array",
			raw:     `["c-0","c-2","c-4"]`,
			total:   5,
			wantSet: map[int]struct{}{0: {}, 2: {}, 4: {}},
		},
		{
			name:    "markdown fenced JSON",
			raw:     "```json\n[\"c-1\"]\n```",
			total:   3,
			wantSet: map[int]struct{}{1: {}},
		},
		{
			name:    "out-of-range indices ignored",
			raw:     `["c-0","c-10","c-99"]`,
			total:   5,
			wantSet: map[int]struct{}{0: {}},
		},
		{
			name:    "negative index ignored",
			raw:     `["c--1","c-0"]`,
			total:   2,
			wantSet: map[int]struct{}{0: {}},
		},
		{
			name:    "invalid ID format ignored",
			raw:     `["x-0","c-abc","c-1"]`,
			total:   3,
			wantSet: map[int]struct{}{1: {}},
		},
		{
			name:    "invalid JSON returns nil",
			raw:     `not json`,
			total:   5,
			wantSet: nil,
		},
		{
			name:    "empty array",
			raw:     `[]`,
			total:   5,
			wantSet: map[int]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFilterResponse(tt.raw, tt.total)
			if tt.wantSet == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != len(tt.wantSet) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tt.wantSet), got)
			}
			for idx := range tt.wantSet {
				if _, ok := got[idx]; !ok {
					t.Errorf("missing index %d in result", idx)
				}
			}
		})
	}
}

func TestExtFromPath(t *testing.T) {
	a := New(Args{})

	tests := []struct {
		path string
		want string
	}{
		{"main.go", ".go"},
		{"src/app.tsx", ".tsx"},
		{"path/to/FILE.JSON", ".json"},
		{"Makefile", ""},
		{".gitignore", ""},
		{"dir/.hidden", ""},
		{"archive.tar.gz", ".gz"},
		{"no-ext", ""},
		{"path/to/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := a.extFromPath(tt.path)
			if got != tt.want {
				t.Errorf("extFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFormatToolDefs(t *testing.T) {
	t.Run("empty defs returns empty string", func(t *testing.T) {
		got := formatToolDefs(nil)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("single tool with parameters", func(t *testing.T) {
		defs := []llm.ToolDef{
			{
				Type: "function",
				Function: llm.FunctionDef{
					Name:        "file_read",
					Description: "Read a file from the repository",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{
								"type":        "string",
								"description": "File path to read",
							},
							"start_line": map[string]any{
								"type":        "integer",
								"description": "Starting line number",
							},
						},
						"required": []any{"path"},
					},
				},
			},
		}
		got := formatToolDefs(defs)
		if !strings.Contains(got, "### Available Tools") {
			t.Error("missing header")
		}
		if !strings.Contains(got, "**file_read**") {
			t.Error("missing tool name")
		}
		if !strings.Contains(got, "Read a file from the repository") {
			t.Error("missing description")
		}
		if !strings.Contains(got, "path") {
			t.Error("missing parameter name")
		}
		if !strings.Contains(got, "(required)") {
			t.Error("missing required marker")
		}
	})

	t.Run("parameters preserve raw JSON order", func(t *testing.T) {
		raw := json.RawMessage(`{
			"name":"code_search",
			"description":"Search code",
			"parameters":{
				"type":"object",
				"properties":{
					"query":{"description":"Query string"},
					"path_glob":{"description":"Path glob"},
					"case_sensitive":{"description":"Match case"},
					"max_results":{"description":"Maximum results"}
				},
				"required":["query"]
			}
		}`)
		defs := []llm.ToolDef{
			{
				Type: "function",
				Function: llm.FunctionDef{
					Name:          "code_search",
					Description:   "Search code",
					RawDefinition: raw,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{
								"description": "Query string",
							},
							"path_glob": map[string]any{
								"description": "Path glob",
							},
							"case_sensitive": map[string]any{
								"description": "Match case",
							},
							"max_results": map[string]any{
								"description": "Maximum results",
							},
						},
						"required": []any{"query"},
					},
				},
			},
		}

		got := formatToolDefs(defs)
		wantLines := []string{
			"  - query: Query string (required)",
			"  - path_glob: Path glob",
			"  - case_sensitive: Match case",
			"  - max_results: Maximum results",
		}
		last := -1
		for _, line := range wantLines {
			idx := strings.Index(got, line)
			if idx == -1 {
				t.Fatalf("missing parameter line %q in:\n%s", line, got)
			}
			if idx <= last {
				t.Fatalf("parameter line %q is out of order in:\n%s", line, got)
			}
			last = idx
		}
	})

	t.Run("fallback parameters are sorted when raw order is unavailable", func(t *testing.T) {
		defs := []llm.ToolDef{
			{
				Type: "function",
				Function: llm.FunctionDef{
					Name:        "code_search",
					Description: "Search code",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{
								"description": "Query string",
							},
							"case_sensitive": map[string]any{
								"description": "Match case",
							},
							"path_glob": map[string]any{
								"description": "Path glob",
							},
							"max_results": map[string]any{
								"description": "Maximum results",
							},
						},
						"required": []any{"query"},
					},
				},
			},
		}

		got := formatToolDefs(defs)
		wantLines := []string{
			"  - case_sensitive: Match case",
			"  - max_results: Maximum results",
			"  - path_glob: Path glob",
			"  - query: Query string (required)",
		}
		last := -1
		for _, line := range wantLines {
			idx := strings.Index(got, line)
			if idx == -1 {
				t.Fatalf("missing parameter line %q in:\n%s", line, got)
			}
			if idx <= last {
				t.Fatalf("parameter line %q is out of order in:\n%s", line, got)
			}
			last = idx
		}
	})

	t.Run("tool without parameters", func(t *testing.T) {
		defs := []llm.ToolDef{
			{
				Type: "function",
				Function: llm.FunctionDef{
					Name:        "task_done",
					Description: "Signal task completion",
					Parameters:  map[string]any{},
				},
			},
		}
		got := formatToolDefs(defs)
		if !strings.Contains(got, "**task_done**") {
			t.Error("missing tool name")
		}
		if strings.Contains(got, "Parameters:") {
			t.Error("should not show Parameters section for empty params")
		}
	})

	t.Run("multiple tools", func(t *testing.T) {
		defs := []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "tool_a", Description: "desc a"}},
			{Type: "function", Function: llm.FunctionDef{Name: "tool_b", Description: "desc b"}},
		}
		got := formatToolDefs(defs)
		if !strings.Contains(got, "tool_a") || !strings.Contains(got, "tool_b") {
			t.Errorf("missing tools in output: %s", got)
		}
	})
}

func TestBuildToolDefs(t *testing.T) {
	funcDef := json.RawMessage(`{"name":"test_tool","description":"a tool","parameters":{}}`)

	entries := []toolsconfig.ToolConfigEntry{
		{Name: "plan_only", PlanTask: true, MainTask: false, Definition: funcDef},
		{Name: "main_only", PlanTask: false, MainTask: true, Definition: funcDef},
		{Name: "both", PlanTask: true, MainTask: true, Definition: funcDef},
		{Name: "neither", PlanTask: false, MainTask: false, Definition: funcDef},
	}

	t.Run("planOnly=true returns plan_task tools", func(t *testing.T) {
		defs := BuildToolDefs(entries, true)
		if len(defs) != 2 {
			t.Fatalf("expected 2 defs, got %d", len(defs))
		}
		names := make(map[string]bool)
		for _, d := range defs {
			names[d.Function.Name] = true
		}
		if !names["test_tool"] {
			t.Error("expected test_tool in plan defs")
		}
	})

	t.Run("planOnly=false returns main_task tools", func(t *testing.T) {
		defs := BuildToolDefs(entries, false)
		if len(defs) != 2 {
			t.Fatalf("expected 2 defs, got %d", len(defs))
		}
	})

	t.Run("invalid definition JSON is skipped", func(t *testing.T) {
		bad := []toolsconfig.ToolConfigEntry{
			{Name: "bad", PlanTask: true, MainTask: true, Definition: json.RawMessage(`{invalid}`)},
			{Name: "good", PlanTask: true, MainTask: true, Definition: funcDef},
		}
		defs := BuildToolDefs(bad, true)
		if len(defs) != 1 {
			t.Fatalf("expected 1 def (bad skipped), got %d", len(defs))
		}
	})

	t.Run("empty entries returns nil", func(t *testing.T) {
		defs := BuildToolDefs(nil, true)
		if defs != nil {
			t.Errorf("expected nil, got %v", defs)
		}
	})
}

// selectionReasons indexes a selection pass by path so the tests can assert on
// the decision the run and `--preview` now share.
func selectionReasons(decisions []fileDecision) map[string]ExcludeReason {
	reasons := make(map[string]ExcludeReason, len(decisions))
	for _, dec := range decisions {
		reasons[effectivePath(dec.Diff)] = dec.Reason
	}
	return reasons
}

func TestSelectFilesTooLarge(t *testing.T) {
	a := New(Args{
		Template: template.Template{MaxTokens: 100},
	})

	diffs := []model.Diff{
		{NewPath: "small.go", Diff: "short diff"},
		{NewPath: "large.go", Diff: strings.Repeat("word ", 500)},
	}

	reasons := selectionReasons(a.selectFiles(diffs))
	if reasons["small.go"] != ExcludeNone {
		t.Errorf("small.go reason = %q, want selected", reasons["small.go"])
	}
	if reasons["large.go"] != ExcludeTooLarge {
		t.Errorf("large.go reason = %q, want %q", reasons["large.go"], ExcludeTooLarge)
	}
}

// exactNTokens builds a string that llm.CountTokens reports as exactly n
// tokens, failing loudly if the tokenizer disagrees so fixture drift cannot
// silently weaken the boundary assertions below.
func exactNTokens(t *testing.T, n int) string {
	t.Helper()
	s := strings.TrimSpace(strings.Repeat("a ", n))
	if got := llm.CountTokens(s); got != n {
		t.Fatalf("fixture drift: llm.CountTokens(<%d-token string>) = %d, want %d", n, got, n)
	}
	return s
}

// TestSelectFilesTooLarge_Boundary pins the 80% threshold exactly: with
// MaxTokens=100 the limit is 80, so an 80-token diff is kept and an 81-token
// one is dropped. TestSelectFilesTooLarge above uses margins wide enough to
// pass at any threshold, so it does not pin the value.
func TestSelectFilesTooLarge_Boundary(t *testing.T) {
	a := New(Args{
		Template: template.Template{MaxTokens: 100},
	})

	diffs := []model.Diff{
		{NewPath: "at-limit.go", Diff: exactNTokens(t, 80)},
		{NewPath: "over-limit.go", Diff: exactNTokens(t, 81)},
	}

	reasons := selectionReasons(a.selectFiles(diffs))
	if reasons["at-limit.go"] != ExcludeNone {
		t.Errorf("at-limit.go reason = %q, want selected", reasons["at-limit.go"])
	}
	if reasons["over-limit.go"] != ExcludeTooLarge {
		t.Errorf("over-limit.go reason = %q, want %q", reasons["over-limit.go"], ExcludeTooLarge)
	}
}

func TestSelectFilesTooLarge_ZeroMaxTokens(t *testing.T) {
	a := New(Args{
		Template: template.Template{MaxTokens: 0},
	})

	diffs := []model.Diff{{NewPath: "a.go", Diff: strings.Repeat("word ", 500)}}
	if reason := a.selectFiles(diffs)[0].Reason; reason != ExcludeNone {
		t.Errorf("reason = %q, want selected when MaxTokens=0 disables the size gate", reason)
	}
}

func TestApplyResumeReusesCompletedItemsAcrossModels(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "a.go", NewPath: "a.go", Diff: "+a", Insertions: 1},
		{OldPath: "b.go", NewPath: "b.go", Diff: "+b", Insertions: 1},
	}
	fp := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "old-session",
		Model:      "anthropic-model",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fp: {
				FilePath:    "a.go",
				OldPath:     "a.go",
				NewPath:     "a.go",
				Fingerprint: fp,
				Comments: []model.LlmComment{{
					Path:    "a.go",
					Content: "cached comment",
				}},
			},
		},
	}
	collector := tool.NewCommentCollector()
	sess := session.New(t.TempDir(), "feature", "openai-model", session.SessionOptions{
		ReviewMode:  session.ReviewModeRange,
		DiffFrom:    "main",
		DiffTo:      "feature",
		ResumedFrom: "old-session",
	})
	defer sess.Finalize()
	a := New(Args{
		From:             "main",
		To:               "feature",
		Model:            "openai-model",
		CommentCollector: collector,
		Resume:           resume,
		Session:          sess,
	})

	toDispatch := a.applyResume(diffs)
	if len(toDispatch) != 1 || toDispatch[0].NewPath != "b.go" {
		t.Fatalf("toDispatch = %+v, want only b.go", toDispatch)
	}
	comments := collector.Comments()
	if len(comments) != 1 || comments[0].Content != "cached comment" {
		t.Fatalf("comments = %+v", comments)
	}
	info := a.ResumeInfo()
	if info == nil || info.ReusedFiles != 1 || info.RerunFiles != 1 || info.PreviousModel != "anthropic-model" || info.CurrentModel != "openai-model" {
		t.Fatalf("ResumeInfo = %+v", info)
	}
}

// TestAgentGettersNil covers the defensive early returns in the accessor
// methods when the agent (or its session) was never fully constructed, so
// callers never advertise a resume target that does not exist.
func TestAgentGettersNil(t *testing.T) {
	var nilAgent *Agent
	if got := nilAgent.SessionID(); got != "" {
		t.Errorf("nil agent SessionID = %q, want empty", got)
	}
	if got := nilAgent.RunManifest(); got != nil {
		t.Errorf("nil agent RunManifest = %v, want nil", got)
	}

	// An agent with no session must also return the empty/nil sentinels.
	empty := &Agent{}
	if got := empty.SessionID(); got != "" {
		t.Errorf("sessionless SessionID = %q, want empty", got)
	}
	if got := empty.RunManifest(); got != nil {
		t.Errorf("sessionless RunManifest = %v, want nil", got)
	}
	if got := empty.ResumeInfo(); got != nil {
		t.Errorf("resumeless ResumeInfo = %v, want nil", got)
	}
}

// TestSummarizeSelection pins what Run reports and keeps: only files that pass
// every gate are counted as selected, while deletions stay in the working diff
// set (they are never dispatched but still describe the change).
func TestSummarizeSelection(t *testing.T) {
	a := New(Args{Template: template.Template{MaxTokens: 100}})
	diffs := []model.Diff{
		{NewPath: "main.go", Insertions: 10, Deletions: 2},
		{NewPath: "deleted.go", IsDeleted: true, Deletions: 20},
		{NewPath: "binary.bin", IsBinary: true},
		{NewPath: "helper.go", Insertions: 5},
		{NewPath: "huge.go", Diff: strings.Repeat("word ", 500)},
	}

	kept, counts := summarizeSelection(a.selectFiles(diffs))
	if counts.Selected != 2 {
		t.Errorf("selected = %d, want 2 (main.go, helper.go)", counts.Selected)
	}
	if counts.TooLarge != 1 {
		t.Errorf("tooLarge = %d, want 1 (huge.go)", counts.TooLarge)
	}
	var keptPaths []string
	for _, d := range kept {
		keptPaths = append(keptPaths, d.NewPath)
	}
	want := []string{"main.go", "deleted.go", "helper.go"}
	if !slices.Equal(keptPaths, want) {
		t.Errorf("kept = %v, want %v", keptPaths, want)
	}
}

func TestBuildChangeFilesExcept(t *testing.T) {
	a := New(Args{})
	a.diffs = []model.Diff{
		{NewPath: "main.go", OldPath: "main.go"},
		{NewPath: "helper.go", OldPath: "helper.go", IsNew: true},
		{NewPath: "removed.go", OldPath: "removed.go", IsDeleted: true},
		{NewPath: "renamed.go", OldPath: "old_name.go"},
		{NewPath: "bin.dat", OldPath: "bin.dat", IsBinary: true},
	}

	got := a.buildChangeFilesExcept("main.go")
	if strings.Contains(got, "main.go") {
		t.Error("excluded file should not appear")
	}
	if !strings.Contains(got, "ADDED") {
		t.Error("expected ADDED status for new file")
	}
	if !strings.Contains(got, "DELETED") {
		t.Error("expected DELETED status")
	}
	if !strings.Contains(got, "RENAMED") {
		t.Error("expected RENAMED status")
	}
	if strings.Contains(got, "bin.dat") {
		t.Error("binary files should be skipped")
	}
}

func TestDispatchSubtasks_WithFakeLLM(t *testing.T) {
	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		codeCommentResponse("main.go"),
		agentTaskDoneResponse(),
	}}

	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})

	a := New(Args{
		LLMClient:        client,
		Model:            "fake",
		CommentCollector: collector,
		Tools:            reg,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 10,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{diff}} for {{current_file_path}}"},
				},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})

	a.diffs = []model.Diff{
		{NewPath: "main.go", OldPath: "main.go", Diff: "+new line", Insertions: 1},
	}
	a.currentDate = "2025-06-26 10:00"

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Path != "main.go" {
		t.Errorf("Path = %q, want main.go", comments[0].Path)
	}
	if !strings.Contains(comments[0].Content, "null pointer") {
		t.Errorf("Content = %q", comments[0].Content)
	}
}

func TestDispatchSubtasks_TokenThresholdSkipIsNotReusableCheckpoint(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()
	sess := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	})

	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		agentTaskDoneResponse(),
	}}
	a := New(Args{
		From:      "main",
		To:        "feature",
		LLMClient: client,
		Model:     "fake",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           100,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: strings.Repeat("context ", 200) + "{{diff}}"},
				},
			},
		},
	})
	diff := model.Diff{NewPath: "large-prompt.go", OldPath: "large-prompt.go", Diff: "+x", Insertions: 1}
	a.diffs = []model.Diff{diff}
	a.currentDate = "2025-06-26 10:00"

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected no comments, got %d", len(comments))
	}
	if client.calls != 0 {
		t.Fatalf("threshold skip should not call LLM, got %d calls", client.calls)
	}
	sess.Finalize()

	state, err := session.LoadResumeState(repoDir, sess.SessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	if state.CompletedCount() != 0 {
		t.Fatalf("CompletedCount = %d, want 0", state.CompletedCount())
	}
	fp := reviewItemFingerprint(session.ReviewModeRange, diff)
	if _, ok := state.Item(fp); ok {
		t.Fatal("token-threshold skip was recorded as a reusable checkpoint")
	}
	summary, items, err := session.LoadDetail(repoDir, sess.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.CompletedFiles != 0 || summary.FailedFiles != 1 {
		t.Fatalf("summary counts = completed %d failed %d, want completed 0 failed 1", summary.CompletedFiles, summary.FailedFiles)
	}
	if len(items) != 1 || items[0].Type != "failed" || !strings.Contains(items[0].Error, "prompt tokens") {
		t.Fatalf("items = %+v, want one token-threshold failed item", items)
	}
}

func TestDispatchSubtasks_MainTaskWithoutTaskDoneIsNotReusableCheckpoint(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()
	sess := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	})

	emptyContent := ""
	client := &fakeAgentClient{responses: []*llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &emptyContent}}},
		Model:   "fake",
		Usage:   &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 1},
	}}}
	a := New(Args{
		From:      "main",
		To:        "feature",
		LLMClient: client,
		Model:     "fake",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 1,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diff}}"}},
			},
		},
	})
	diff := model.Diff{NewPath: "needs-review.go", OldPath: "needs-review.go", Diff: "+x", Insertions: 1}
	a.diffs = []model.Diff{diff}
	a.currentDate = "2025-06-26 10:00"

	_, err := a.dispatchSubtasks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "all 1 file review(s) failed") {
		t.Fatalf("expected all-failed error, got %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", client.calls)
	}
	warnings := a.Warnings()
	if len(warnings) != 1 || warnings[0].Type != "subtask_error" || warnings[0].File != diff.NewPath {
		t.Fatalf("warnings = %+v, want one subtask_error for %s", warnings, diff.NewPath)
	}
	sess.Finalize()

	state, err := session.LoadResumeState(repoDir, sess.SessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	if state.CompletedCount() != 0 {
		t.Fatalf("CompletedCount = %d, want 0", state.CompletedCount())
	}
	fp := reviewItemFingerprint(session.ReviewModeRange, diff)
	if _, ok := state.Item(fp); ok {
		t.Fatal("incomplete main task was recorded as a reusable checkpoint")
	}
	summary, items, err := session.LoadDetail(repoDir, sess.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.CompletedFiles != 0 || summary.FailedFiles != 1 {
		t.Fatalf("summary counts = completed %d failed %d, want completed 0 failed 1", summary.CompletedFiles, summary.FailedFiles)
	}
	if len(items) != 1 || items[0].Type != "failed" || !strings.Contains(items[0].Error, "main_task did not complete") {
		t.Fatalf("items = %+v, want one incomplete-main failed item", items)
	}
}

func TestDispatchSubtasks_IncompleteMainTaskMarksPartialFailure(t *testing.T) {
	emptyContent := ""
	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		agentTaskDoneResponse(),
		{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &emptyContent}}},
			Model:   "fake",
			Usage:   &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 1},
		},
	}}
	a := New(Args{
		From:           "main",
		To:             "feature",
		LLMClient:      client,
		Model:          "fake",
		MaxConcurrency: 1,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 1,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diff}}"}},
			},
		},
	})
	a.diffs = []model.Diff{
		{NewPath: "complete.go", OldPath: "complete.go", Diff: "+x", Insertions: 1},
		{NewPath: "incomplete.go", OldPath: "incomplete.go", Diff: "+y", Insertions: 1},
	}
	a.currentDate = "2025-06-26 10:00"

	_, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("one completed file should keep the review partial, got %v", err)
	}
	warnings := a.Warnings()
	if len(warnings) != 1 || warnings[0].Type != "subtask_error" || warnings[0].File != "incomplete.go" {
		t.Fatalf("warnings = %+v, want one subtask_error for incomplete.go", warnings)
	}
}

func TestDispatchSubtasks_AllDeleted(t *testing.T) {
	client := &fakeAgentClient{}
	a := New(Args{
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{diff}}"},
				},
			},
		},
	})

	a.diffs = []model.Diff{
		{NewPath: "removed.go", IsDeleted: true},
	}
	a.currentDate = "2025-06-26 10:00"

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments for deleted file, got %d", len(comments))
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestAgent_TokenAccumulation(t *testing.T) {
	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		agentTaskDoneResponse(),
	}}

	a := New(Args{
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 10,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{diff}}"},
				},
			},
		},
	})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "+x", Insertions: 1},
	}
	a.currentDate = "2025-06-26 10:00"

	_, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.TotalInputTokens() != 10 {
		t.Errorf("TotalInputTokens = %d, want 10", a.TotalInputTokens())
	}
	if a.TotalOutputTokens() != 5 {
		t.Errorf("TotalOutputTokens = %d, want 5", a.TotalOutputTokens())
	}
}
