// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConversationToolBindings(test *testing.T) {
	manifest := manifestConversation{
		Messages:       []manifestMessage{{Role: "user", PromptFile: "main_task_user.md"}},
		Tools:          []string{"file_find", "file_read"},
		ToolReferences: []string{"code_search"},
	}
	conversation, err := resolveConversation(manifest)
	if err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(conversation.Tools, manifest.Tools) || !reflect.DeepEqual(conversation.ToolReferences, manifest.ToolReferences) {
		test.Fatalf("tool bindings lost while resolving prompts: %+v", conversation)
	}
}

func TestValidateUnsupportedToolBindings(test *testing.T) {
	for _, mode := range []string{"review", "scan"} {
		test.Run(mode, func(test *testing.T) {
			var main *LlmConversation
			var tasks map[string]*LlmConversation
			var validate func() error
			if mode == "review" {
				tpl, err := LoadDefault()
				if err != nil {
					test.Fatal(err)
				}
				main = &tpl.MainTask
				tasks = map[string]*LlmConversation{
					"PLAN_TASK":               tpl.PlanTask,
					"MEMORY_COMPRESSION_TASK": &tpl.MemoryCompressionTask,
					"RE_LOCATION_TASK":        tpl.ReLocationTask,
					"REVIEW_FILTER_TASK":      tpl.ReviewFilterTask,
					"GROUPING_TASK":           tpl.GroupingTask,
				}
				validate = tpl.Validate
			} else {
				tpl, err := LoadScanDefault()
				if err != nil {
					test.Fatal(err)
				}
				main = &tpl.MainTask
				tasks = map[string]*LlmConversation{
					"PLAN_TASK":               tpl.PlanTask,
					"MEMORY_COMPRESSION_TASK": &tpl.MemoryCompressionTask,
					"RE_LOCATION_TASK":        tpl.ReLocationTask,
					"DEDUP_TASK":              tpl.DedupTask,
					"PROJECT_SUMMARY_TASK":    tpl.ProjectSummaryTask,
				}
				validate = tpl.Validate
			}
			main.ToolReferences = []string{"file_read"}
			if err := validate(); err == nil || !strings.Contains(err.Error(), "MAIN_TASK.tool_references") {
				test.Fatalf("unexpected reference validation: %v", err)
			}
			main.ToolReferences = nil
			for name, task := range tasks {
				task.Tools = []string{"file_read"}
				if err := validate(); err == nil || !strings.Contains(err.Error(), name+".tools") {
					test.Fatalf("unexpected tool validation: %v", err)
				}
				task.Tools = nil
				if name == "PLAN_TASK" && mode == "review" {
					continue
				}
				task.ToolReferences = []string{"file_read"}
				if err := validate(); err == nil || !strings.Contains(err.Error(), name+".tool_references") {
					test.Fatalf("unexpected reference validation: %v", err)
				}
				task.ToolReferences = nil
			}
			if err := validate(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestLoadScanDefault_BudgetParsed(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	if tpl.MaxToolRequestTimes < 60 {
		t.Errorf("scan MaxToolRequestTimes(%d) should be >= 60", tpl.MaxToolRequestTimes)
	}
	if len(tpl.MainTask.Messages) == 0 {
		t.Fatal("scan MainTask must be populated from the embedded scan_template.json")
	}
	if tpl.MaxFileSizeBytes <= 0 {
		t.Errorf("scan MaxFileSizeBytes(%d) should be > 0 (defaults to 2 MiB in JSON)", tpl.MaxFileSizeBytes)
	}
}

func TestLoadScanDefault_PlanThreshold(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	if tpl.PlanModeLineThreshold != 200 {
		t.Errorf("PlanModeLineThreshold = %d, want 200", tpl.PlanModeLineThreshold)
	}
	if tpl.PlanTask == nil || len(tpl.PlanTask.Messages) == 0 {
		t.Fatal("default scan template must define PLAN_TASK messages")
	}
}

func TestApplyLanguage_ScanTemplate(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	tpl.ApplyLanguage("Spanish")

	for _, m := range tpl.MainTask.Messages {
		if m.Role != "system" {
			continue
		}
		if !strings.Contains(m.Content, "Always respond in Spanish.") {
			t.Errorf("language directive missing from scan MainTask system message")
		}
	}
}

func TestLoadDefault_HasNoScanFields(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if len(tpl.MainTask.Messages) == 0 {
		t.Fatal("review MainTask must be populated")
	}
	if tpl.MaxToolRequestTimes <= 0 {
		t.Errorf("review MaxToolRequestTimes invalid: %d", tpl.MaxToolRequestTimes)
	}
}

func TestGroupingPlan(t *testing.T) {
	tests := []struct {
		name         string
		fileCount    int
		totalChanged int64
		minFiles     int
		bundleLines  int
		want         GroupingStrategy
	}{
		{"at file threshold defers to the LLM", 4, 10, 4, 200, GroupingViaLLM},
		{"above file threshold defers to the LLM", 9, 10, 4, 200, GroupingViaLLM},
		{"high churn does not override the file threshold", 4, 9999, 4, 200, GroupingViaLLM},
		{"few files, low churn bundles", 3, 10, 4, 200, GroupingBundleAll},
		{"one line below the churn threshold bundles", 3, 199, 4, 200, GroupingBundleAll},
		{"at the churn threshold goes per file", 3, 200, 4, 200, GroupingPerFile},
		{"above the churn threshold goes per file", 2, 5000, 4, 200, GroupingPerFile},
		{"zero min files keeps grouping unconditional", 3, 10, 0, 200, GroupingViaLLM},
		{"negative min files keeps grouping unconditional", 3, 10, -1, 200, GroupingViaLLM},
		{"zero bundle threshold never bundles", 3, 10, 4, 0, GroupingPerFile},
		{"negative bundle threshold never bundles", 3, 10, 4, -1, GroupingPerFile},
		{"zero churn bundles", 2, 0, 4, 200, GroupingBundleAll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl := Template{
				GroupingMinFiles:            tt.minFiles,
				GroupingBundleLineThreshold: tt.bundleLines,
			}
			if got := tpl.GroupingPlan(tt.fileCount, tt.totalChanged); got != tt.want {
				t.Errorf("GroupingPlan(%d, %d) = %v, want %v",
					tt.fileCount, tt.totalChanged, got, tt.want)
			}
		})
	}
}

func TestGroupingStrategyString(t *testing.T) {
	tests := []struct {
		strategy GroupingStrategy
		want     string
	}{
		{GroupingViaLLM, "llm"},
		{GroupingBundleAll, "bundle_all"},
		{GroupingPerFile, "per_file"},
		{GroupingStrategy(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.strategy.String(); got != tt.want {
			t.Errorf("GroupingStrategy(%d).String() = %q, want %q", int(tt.strategy), got, tt.want)
		}
	}
}

func TestCompletionTokenLimit(t *testing.T) {
	review := Template{MaxTokens: 200000}
	if got := review.CompletionTokenLimit(); got != 200000 {
		t.Fatalf("review fallback = %d, want 200000", got)
	}
	review.MaxCompletionTokens = 58888
	if got := review.CompletionTokenLimit(); got != 58888 {
		t.Fatalf("review runtime limit = %d, want 58888", got)
	}

	scan := ScanTemplate{MaxTokens: 128000, MaxCompletionTokens: 4096}
	if got := scan.CompletionTokenLimit(); got != 4096 {
		t.Fatalf("scan runtime limit = %d, want 4096", got)
	}
}

func TestLoadDefault_FieldsPopulated(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}

	if len(tpl.MainTask.Messages) != 2 {
		t.Errorf("MainTask.Messages length = %d, want 2", len(tpl.MainTask.Messages))
	}
	for i, msg := range tpl.MainTask.Messages {
		if msg.Content == "" {
			t.Errorf("MainTask.Messages[%d].Content is empty", i)
		}
	}
	if tpl.PlanTask == nil {
		t.Fatal("PlanTask is nil, expected non-nil")
	}
	if len(tpl.PlanTask.Messages) != 2 {
		t.Errorf("PlanTask.Messages length = %d, want 2", len(tpl.PlanTask.Messages))
	}
	if tpl.ReLocationTask == nil {
		t.Fatal("ReLocationTask is nil, expected non-nil")
	}
	if tpl.ReviewFilterTask == nil {
		t.Fatal("ReviewFilterTask is nil, expected non-nil")
	}
	if tpl.MaxTokens != 200000 {
		t.Errorf("MaxTokens = %d, want 200000", tpl.MaxTokens)
	}
	if tpl.MaxCompletionTokens != 16384 {
		t.Errorf("MaxCompletionTokens = %d, want 16384", tpl.MaxCompletionTokens)
	}
	if tpl.MaxToolRequestTimes != 100 {
		t.Errorf("MaxToolRequestTimes = %d, want 100", tpl.MaxToolRequestTimes)
	}
	if tpl.PlanModeLineThreshold != 50 {
		t.Errorf("PlanModeLineThreshold = %d, want 50", tpl.PlanModeLineThreshold)
	}
	if tpl.GroupingMinFiles != 4 {
		t.Errorf("GroupingMinFiles = %d, want 4", tpl.GroupingMinFiles)
	}
	if tpl.GroupingBundleLineThreshold != 200 {
		t.Errorf("GroupingBundleLineThreshold = %d, want 200", tpl.GroupingBundleLineThreshold)
	}
	if tpl.PlanModeGroupLineThreshold != 100 {
		t.Errorf("PlanModeGroupLineThreshold = %d, want 100", tpl.PlanModeGroupLineThreshold)
	}
}

func TestLoadDefault_PlaceholdersPresent(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}

	tests := []struct {
		name        string
		content     string
		placeholder string
	}{
		{"MainTask user has diffs", tpl.MainTask.Messages[1].Content, "{{diffs}}"},
		{"PlanTask system has plan_tools", tpl.PlanTask.Messages[0].Content, "{{plan_tools}}"},
		{"MemoryCompression user has context", tpl.MemoryCompressionTask.Messages[1].Content, "{{context}}"},
		{"ReviewFilter user has comments", tpl.ReviewFilterTask.Messages[1].Content, "{{comments}}"},
		{"ReLocation user has diff (single brace)", tpl.ReLocationTask.Messages[1].Content, "{diff}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.content, tt.placeholder) {
				t.Errorf("content does not contain %q", tt.placeholder)
			}
		})
	}
}

func TestValidate_PassesOnDefault(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}
	if err := tpl.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestApplyLanguage(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}

	tpl.ApplyLanguage("Chinese")
	suffix := "\n\nAlways respond in Chinese."
	if !strings.HasSuffix(tpl.MainTask.Messages[0].Content, suffix) {
		t.Errorf("MainTask system message does not end with %q", suffix)
	}
	if !strings.HasSuffix(tpl.PlanTask.Messages[0].Content, suffix) {
		t.Errorf("PlanTask system message does not end with %q", suffix)
	}
	if !strings.HasSuffix(tpl.MemoryCompressionTask.Messages[0].Content, suffix) {
		t.Errorf("MemoryCompressionTask system message does not end with %q", suffix)
	}
}

func TestApplyLanguage_DefaultEnglish(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}

	tpl.ApplyLanguage("")
	suffix := "\n\nAlways respond in English."
	if !strings.HasSuffix(tpl.MainTask.Messages[0].Content, suffix) {
		t.Errorf("MainTask system message does not end with %q", suffix)
	}
}

func TestValidate_Template_Errors(t *testing.T) {
	cases := []struct {
		name    string
		tpl     Template
		wantErr string
	}{
		{
			name:    "zero MaxTokens",
			tpl:     Template{MaxTokens: 0, MaxToolRequestTimes: 1, MainTask: LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "x"}}}},
			wantErr: "max_tokens must be positive",
		},
		{
			name:    "negative MaxTokens",
			tpl:     Template{MaxTokens: -1, MaxToolRequestTimes: 1, MainTask: LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "x"}}}},
			wantErr: "max_tokens must be positive",
		},
		{
			name:    "zero MaxToolRequestTimes",
			tpl:     Template{MaxTokens: 100, MaxToolRequestTimes: 0, MainTask: LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "x"}}}},
			wantErr: "max_tool_request_times must be positive",
		},
		{
			name:    "empty MainTask messages",
			tpl:     Template{MaxTokens: 100, MaxToolRequestTimes: 1, MainTask: LlmConversation{Messages: nil}},
			wantErr: "main_task.messages must not be empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tpl.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidate_ScanTemplate(t *testing.T) {
	valid := ScanTemplate{
		MaxTokens:           100,
		MaxToolRequestTimes: 1,
		// A valid scan template needs a user message carrying the file
		// under review (see validateScanPlaceholders).
		MainTask: LlmConversation{Messages: []ChatMessage{
			{Role: "system", Content: "x"},
			{Role: "user", Content: "{{file_content}}"},
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid ScanTemplate.Validate() error: %v", err)
	}

	cases := []struct {
		name    string
		tpl     ScanTemplate
		wantErr string
	}{
		{
			name:    "zero MaxTokens",
			tpl:     ScanTemplate{MaxTokens: 0, MaxToolRequestTimes: 1, MainTask: LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "x"}}}},
			wantErr: "scan: max_tokens must be positive",
		},
		{
			name:    "zero MaxToolRequestTimes",
			tpl:     ScanTemplate{MaxTokens: 100, MaxToolRequestTimes: 0, MainTask: LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "x"}}}},
			wantErr: "scan: max_tool_request_times must be positive",
		},
		{
			name:    "empty MainTask messages",
			tpl:     ScanTemplate{MaxTokens: 100, MaxToolRequestTimes: 1, MainTask: LlmConversation{Messages: nil}},
			wantErr: "scan: main_task.messages must not be empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tpl.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadScanDefault_Validate(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	if err := tpl.Validate(); err != nil {
		t.Errorf("loaded scan template should be valid: %v", err)
	}
}

func TestApplyLanguage_ScanTemplate_AllOptionalTasks(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	if tpl.DedupTask == nil {
		t.Fatal("DedupTask should be present in default scan template")
	}
	if tpl.ProjectSummaryTask == nil {
		t.Fatal("ProjectSummaryTask should be present in default scan template")
	}

	tpl.ApplyLanguage("Japanese")
	suffix := "Always respond in Japanese."

	check := func(name string, conv *LlmConversation) {
		t.Helper()
		if conv == nil {
			return
		}
		for _, m := range conv.Messages {
			if m.Role == "system" && !strings.Contains(m.Content, suffix) {
				t.Errorf("%s system message missing language directive", name)
			}
		}
	}
	check("MainTask", &tpl.MainTask)
	check("PlanTask", tpl.PlanTask)
	check("DedupTask", tpl.DedupTask)
	check("ProjectSummaryTask", tpl.ProjectSummaryTask)
	check("MemoryCompressionTask", &tpl.MemoryCompressionTask)
}

func TestApplyLanguage_ScanTemplate_NilOptionalTasks(t *testing.T) {
	tpl := &ScanTemplate{
		MainTask:              LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "base"}}},
		MemoryCompressionTask: LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "compress"}}},
	}
	tpl.ApplyLanguage("Korean")
	suffix := "Always respond in Korean."
	if !strings.Contains(tpl.MainTask.Messages[0].Content, suffix) {
		t.Error("MainTask should contain language directive")
	}
	if !strings.Contains(tpl.MemoryCompressionTask.Messages[0].Content, suffix) {
		t.Error("MemoryCompressionTask should contain language directive")
	}
}

func TestApplyLanguage_SkipsNonSystemMessages(t *testing.T) {
	tpl := &Template{
		MainTask: LlmConversation{Messages: []ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "usr"},
		}},
		MemoryCompressionTask: LlmConversation{Messages: []ChatMessage{
			{Role: "system", Content: "sys"},
		}},
	}
	tpl.ApplyLanguage("French")
	if strings.Contains(tpl.MainTask.Messages[1].Content, "French") {
		t.Error("user-role message should not get language directive")
	}
}

func TestResolveLang(t *testing.T) {
	if got := resolveLang(""); got != "English" {
		t.Errorf("resolveLang(\"\") = %q, want \"English\"", got)
	}
	if got := resolveLang("German"); got != "German" {
		t.Errorf("resolveLang(\"German\") = %q, want \"German\"", got)
	}
}

func TestPlanRequired(t *testing.T) {
	tests := []struct {
		name      string
		tpl       Template
		fileCount int
		total     int64
		maxFile   int64
		want      bool
	}{
		{
			name:      "single file exceeds per-file threshold",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 120},
			fileCount: 1, total: 60, maxFile: 60,
			want: true,
		},
		{
			name:      "single file below per-file threshold",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 120},
			fileCount: 1, total: 40, maxFile: 40,
			want: false,
		},
		{
			name:      "single file cannot trigger group threshold",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 120},
			fileCount: 1, total: 200, maxFile: 200,
			want: true, // triggers per-file, not group
		},
		{
			name:      "multi-file group exceeds group threshold",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 120},
			fileCount: 5, total: 200, maxFile: 40,
			want: true,
		},
		{
			name:      "multi-file group below both thresholds",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 120},
			fileCount: 3, total: 90, maxFile: 30,
			want: false,
		},
		{
			name:      "per-file threshold zero means always plan",
			tpl:       Template{PlanModeLineThreshold: 0, PlanModeGroupLineThreshold: 120},
			fileCount: 1, total: 5, maxFile: 5,
			want: true,
		},
		{
			name:      "group threshold zero disables group gate",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 0},
			fileCount: 5, total: 200, maxFile: 40,
			want: false,
		},
		{
			name:      "multi-file group at exact group threshold boundary",
			tpl:       Template{PlanModeLineThreshold: 50, PlanModeGroupLineThreshold: 120},
			fileCount: 3, total: 120, maxFile: 40,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tpl.PlanRequired(tt.fileCount, tt.total, tt.maxFile)
			if got != tt.want {
				t.Errorf("PlanRequired(%d, %d, %d) = %v, want %v",
					tt.fileCount, tt.total, tt.maxFile, got, tt.want)
			}
		})
	}
}

const validScanOverrideJSON = `{
  "MAIN_TASK": {
    "tools": ["task_done", "code_comment"],
    "messages": [
      {"role": "system", "content": "bounded scope system {{system_rule}}"},
      {"role": "user", "content": "file {{current_file_path}}:\n{{file_content}}\n{{plan_guidance}}"}
    ]
  },
  "NO_PLAN_GUIDANCE": "(bounded scope; no pre-scan plan)",
  "MAX_TOKENS": 1000,
  "MAX_TOOL_REQUEST_TIMES": 5
}`

func writeScanOverrideFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadScanFile_ValidOverride(t *testing.T) {
	tpl, err := LoadScanFile(writeScanOverrideFile(t, validScanOverrideJSON))
	if err != nil {
		t.Fatalf("LoadScanFile(valid): %v", err)
	}
	if !reflect.DeepEqual(tpl.MainTask.Tools, []string{"task_done", "code_comment"}) {
		t.Errorf("MainTask.Tools = %v, want 2-tool subset", tpl.MainTask.Tools)
	}
	if tpl.NoPlanGuidance != "(bounded scope; no pre-scan plan)" {
		t.Errorf("NoPlanGuidance = %q", tpl.NoPlanGuidance)
	}
	if tpl.MaxTokens != 1000 || tpl.MaxToolRequestTimes != 5 {
		t.Errorf("budgets = (%d, %d), want lowered (1000, 5)", tpl.MaxTokens, tpl.MaxToolRequestTimes)
	}
}

func TestLoadScanFile_Invalid(t *testing.T) {
	withDedupTools := strings.Replace(validScanOverrideJSON, `"NO_PLAN_GUIDANCE"`,
		`"DEDUP_TASK": {"tools": ["file_read"], "messages": [{"role": "user", "content": "{{batch_comments}}"}]}, "NO_PLAN_GUIDANCE"`, 1)
	withUnknownToken := strings.Replace(validScanOverrideJSON, `{{plan_guidance}}`, `{{frobnicate}}`, 1)
	withoutFileContent := strings.Replace(validScanOverrideJSON, `\n{{file_content}}`, ``, 1)
	zeroMaxTokens := strings.Replace(validScanOverrideJSON, `"MAX_TOKENS": 1000`, `"MAX_TOKENS": 0`, 1)
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"malformed JSON", `{oops`, "unmarshal"},
		{"missing MAIN messages", `{"MAX_TOKENS": 1000, "MAX_TOOL_REQUEST_TIMES": 5}`, "main_task.messages must not be empty"},
		{"non-positive MAX_TOKENS", zeroMaxTokens, "max_tokens must be positive"},
		{"tools on DEDUP", withDedupTools, "DEDUP_TASK.tools"},
		{"unknown token in MAIN", withUnknownToken, "unknown placeholder {{frobnicate}}"},
		{"MAIN user without file_content", withoutFileContent, "{{file_content}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadScanFile(writeScanOverrideFile(t, tt.content))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("LoadScanFile error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadScanFile_PathHygiene(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.json")
		_, err := LoadScanFile(missing)
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("LoadScanFile error = %v, want path %q named", err, missing)
		}
	})
	t.Run("directory path", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadScanFile(dir)
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("LoadScanFile error = %v, want directory rejection", err)
		}
	})
	t.Run("oversize file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "huge.json")
		if err := os.WriteFile(path, make([]byte, (1<<20)+1), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadScanFile(path)
		if err == nil || !strings.Contains(err.Error(), "exceed") {
			t.Errorf("LoadScanFile error = %v, want size-cap rejection", err)
		}
	})
	t.Run("exactly 1 MiB file loads", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "capped.json")
		padded := validScanOverrideJSON + strings.Repeat(" ", (1<<20)-len(validScanOverrideJSON))
		if err := os.WriteFile(path, []byte(padded), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadScanFile(path); err != nil {
			t.Errorf("LoadScanFile(at cap) error = %v, want success", err)
		}
	})
}

func TestLoadScanDefault_PassesExtendedValidate(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	if err := tpl.Validate(); err != nil {
		t.Errorf("embedded default must pass extended Validate: %v", err)
	}
}

func TestScanTemplateExampleLoads(t *testing.T) {
	tpl, err := LoadScanFile("../../../examples/scan-template/bounded-function-review.json")
	if err != nil {
		t.Fatalf("shipped example must load and validate: %v", err)
	}
	if !reflect.DeepEqual(tpl.MainTask.Tools, []string{"task_done", "code_comment"}) {
		t.Errorf("example MainTask.Tools = %v, want minimal 2-tool set", tpl.MainTask.Tools)
	}
	if tpl.PlanTask != nil {
		t.Error("example must omit PLAN_TASK to exercise plan-less runs")
	}
	if tpl.NoPlanGuidance == "" {
		t.Error("example must set scope-coherent NO_PLAN_GUIDANCE")
	}
}

func TestApplyLanguage_ScanTemplateOverride(t *testing.T) {
	tpl, err := LoadScanFile(writeScanOverrideFile(t, validScanOverrideJSON))
	if err != nil {
		t.Fatalf("LoadScanFile(valid): %v", err)
	}
	tpl.ApplyLanguage("French")
	suffix := "\n\nAlways respond in French."
	if !strings.HasSuffix(tpl.MainTask.Messages[0].Content, suffix) {
		t.Errorf("override system message does not end with %q", suffix)
	}
}
