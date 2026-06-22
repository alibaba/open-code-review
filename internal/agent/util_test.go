package agent

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
)

func TestStripEmptyPlanBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "english template wrapper is removed",
			input: "header\n### Review Plan (Optional)\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "english template wrapper without trailing blank line is removed",
			input: "header\n### Review Plan (Optional)\n{{plan_guidance}}\ntail",
			want:  "header\ntail",
		},
		{
			name:  "chinese template wrapper is removed",
			input: "header\n### 审查计划\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "chinese optional wrapper is removed",
			input: "header\n### 审查计划（可选）\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "no wrapper present is a no-op",
			input: "no plan block here\njust text",
			want:  "no plan block here\njust text",
		},
		{
			name:  "multiple wrappers all removed",
			input: "### Review Plan (Optional)\n{{plan_guidance}}\n\nmiddle\n### 审查计划\n{{plan_guidance}}\n\nend",
			want:  "middle\nend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripEmptyPlanBlock(tt.input)
			if got != tt.want {
				t.Errorf("stripEmptyPlanBlock() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "{{plan_guidance}}") {
				t.Errorf("stripEmptyPlanBlock() left literal {{plan_guidance}} in output: %q", got)
			}
		})
	}
}

func TestStripEmptyPlanBlock_IntegrationWithReplaceAll(t *testing.T) {
	template := "header\n### Review Plan (Optional)\n{{plan_guidance}}\n\ntail"

	stripped := stripEmptyPlanBlock(template)
	final := strings.ReplaceAll(stripped, "{{plan_guidance}}", "")

	want := "header\ntail"
	if final != want {
		t.Errorf("stripEmptyPlanBlock integration:\n  got:  %q\n  want: %q", final, want)
	}
	if strings.Contains(final, "{{plan_guidance}}") {
		t.Errorf("literal {{plan_guidance}} leaked: %q", final)
	}
	if strings.Contains(final, "Review Plan") {
		t.Errorf("dangling Review Plan header retained: %q", final)
	}
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no fences",
			input: `["c-0","c-2"]`,
			want:  `["c-0","c-2"]`,
		},
		{
			name:  "json fenced block",
			input: "```json\n[\"c-0\"]\n```",
			want:  `["c-0"]`,
		},
		{
			name:  "plain fenced block",
			input: "```\nhello\n```",
			want:  "hello",
		},
		{
			name:  "surrounding whitespace",
			input: "  \n```json\ncontent\n```\n  ",
			want:  "content",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only opening fence no newline",
			input: "```json{}```",
			want:  "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripMarkdownFences(tt.input)
			if got != tt.want {
				t.Errorf("stripMarkdownFences() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildMessageXML(t *testing.T) {
	msgs := []llm.Message{
		llm.NewTextMessage("user", "hello"),
		llm.NewTextMessage("assistant", "world"),
	}

	got := buildMessageXML(msgs)

	if !strings.Contains(got, `<message id="0" role="user">`) {
		t.Errorf("missing user message tag in output:\n%s", got)
	}
	if !strings.Contains(got, `<message id="1" role="assistant">`) {
		t.Errorf("missing assistant message tag in output:\n%s", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("missing message content in output:\n%s", got)
	}
}

func TestCopyMessages(t *testing.T) {
	orig := []llm.Message{
		llm.NewTextMessage("user", "a"),
		llm.NewTextMessage("assistant", "b"),
	}

	cp := copyMessages(orig)

	if len(cp) != len(orig) {
		t.Fatalf("copyMessages length = %d, want %d", len(cp), len(orig))
	}

	cp = append(cp, llm.NewTextMessage("user", "c"))
	if len(orig) != 2 {
		t.Error("copyMessages: appending to copy modified original slice")
	}
}

func TestCountMessagesTokens(t *testing.T) {
	msgs := []llm.Message{
		llm.NewTextMessage("user", "hello world"),
	}

	count := countMessagesTokens(msgs)
	if count <= 0 {
		t.Errorf("countMessagesTokens() = %d, want > 0", count)
	}

	empty := countMessagesTokens(nil)
	if empty != 0 {
		t.Errorf("countMessagesTokens(nil) = %d, want 0", empty)
	}
}

func TestReviewModeString(t *testing.T) {
	tests := []struct {
		from, to, commit string
		want             string
	}{
		{"", "", "abc123", session.ReviewModeCommit},
		{"main", "feature", "", session.ReviewModeRange},
		{"", "", "", session.ReviewModeWorkspace},
		{"main", "feature", "abc123", session.ReviewModeCommit},
	}

	for _, tt := range tests {
		got := reviewModeString(tt.from, tt.to, tt.commit)
		if got != tt.want {
			t.Errorf("reviewModeString(%q, %q, %q) = %q, want %q", tt.from, tt.to, tt.commit, got, tt.want)
		}
	}
}
