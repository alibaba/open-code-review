package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/config/toolsconfig"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/tool"
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

func TestFilterLargeDiffs(t *testing.T) {
	a := New(Args{
		Template: template.Template{MaxTokens: 100},
	})

	diffs := []model.Diff{
		{NewPath: "small.go", Diff: "short diff"},
		{NewPath: "large.go", Diff: strings.Repeat("word ", 500)},
	}

	kept := a.filterLargeDiffs(diffs)
	if len(kept) != 1 {
		t.Fatalf("expected 1 kept diff, got %d", len(kept))
	}
	if kept[0].NewPath != "small.go" {
		t.Errorf("kept wrong file: %s", kept[0].NewPath)
	}
}

func TestFilterLargeDiffs_ZeroMaxTokens(t *testing.T) {
	a := New(Args{
		Template: template.Template{MaxTokens: 0},
	})

	diffs := []model.Diff{{NewPath: "a.go", Diff: "some diff"}}
	kept := a.filterLargeDiffs(diffs)
	if len(kept) != 1 {
		t.Errorf("expected all kept when MaxTokens=0, got %d", len(kept))
	}
}

func TestCountReviewable(t *testing.T) {
	a := New(Args{})
	diffs := []model.Diff{
		{NewPath: "main.go", Insertions: 10, Deletions: 2},
		{NewPath: "deleted.go", IsDeleted: true, Deletions: 20},
		{NewPath: "binary.bin", IsBinary: true},
		{NewPath: "helper.go", Insertions: 5},
	}

	count := a.countReviewable(diffs)
	if count != 2 {
		t.Errorf("countReviewable = %d, want 2", count)
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
