package template

import (
	"strings"
	"testing"
)

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
	if tpl.MainTask.Timeout != 120 {
		t.Errorf("MainTask.Timeout = %d, want 120", tpl.MainTask.Timeout)
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
	if tpl.MaxTokens != 58888 {
		t.Errorf("MaxTokens = %d, want 58888", tpl.MaxTokens)
	}
	if tpl.MaxToolRequestTimes != 30 {
		t.Errorf("MaxToolRequestTimes = %d, want 30", tpl.MaxToolRequestTimes)
	}
	if tpl.PlanModeLineThreshold != 50 {
		t.Errorf("PlanModeLineThreshold = %d, want 50", tpl.PlanModeLineThreshold)
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
		{"MainTask user has current_file_path", tpl.MainTask.Messages[1].Content, "{{current_file_path}}"},
		{"MainTask user has diff", tpl.MainTask.Messages[1].Content, "{{diff}}"},
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
		MainTask:            LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "x"}}},
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
