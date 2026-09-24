// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestStripEmptyPlanBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "review plan wrapper is removed",
			input: "header\n### Review Plan\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "english template wrapper without trailing blank line is removed",
			input: "header\n### Review Plan (Optional)\n{{plan_guidance}}\ntail",
			want:  "header\ntail",
		},
		{
			name:  "no wrapper present is a no-op",
			input: "no plan block here\njust text",
			want:  "no plan block here\njust text",
		},
		{
			name:  "multiple wrappers all removed",
			input: "### Review Plan (Optional)\n{{plan_guidance}}\n\nmiddle\n### Review Plan\n{{plan_guidance}}\n\nend",
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
	template := "header\n### Review Plan\n{{plan_guidance}}\n\ntail"

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
	if len(cp) != len(orig)+1 {
		t.Errorf("appended copy length = %d, want %d", len(cp), len(orig)+1)
	}
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

// TestFlattenOneLine_CollapsesEveryLineBreak pins the contract the name and the
// doc comment promise: the result is one line. Only \r\n, \n and \r were mapped,
// so vertical tab, form feed, NEL and the Unicode line/paragraph separators
// passed straight through. A confirmed finding carrying any of them keeps a line
// break inside the <confirmed_findings> block that buildConfirmedCommentsBlock
// renders, which breaks the "code:" / "issue:" line structure the model reads.
func TestFlattenOneLine_CollapsesEveryLineBreak(t *testing.T) {
	breaks := []struct {
		name string
		in   string
		want string
	}{
		// \r and \n are each mapped to a space, so a CRLF pair becomes two
		// spaces. That is pre-existing behaviour and harmless: the point is that
		// no line break survives, not that the spacing is minimal.
		{"carriage return + newline", "a\r\nb", "a  b"},
		{"newline", "a\nb", "a b"},
		{"carriage return", "a\rb", "a b"},
		{"vertical tab", "a\x0bb", "a b"},
		{"form feed", "a\x0cb", "a b"},
		{"NEL", "a\u0085b", "a b"},
		{"line separator", "a\u2028b", "a b"},
		{"paragraph separator", "a\u2029b", "a b"},
	}

	for _, tt := range breaks {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenOneLine(tt.in)
			if strings.ContainsAny(got, "\r\n\x0b\x0c\u0085\u2028\u2029") {
				t.Errorf("flattenOneLine(%q) = %q, still contains a line break", tt.in, got)
			}
			if got != tt.want {
				t.Errorf("flattenOneLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFlattenOneLine_DropsControlCharacters pins the other half: C0 controls
// other than the whitespace ones, DEL and C1 controls must not survive. These
// reach the prompt from model-authored findings, where an escape sequence or a
// NUL would corrupt the rendered block. internal/session already applies this
// rule via stripUnsafeChars; this keeps the agent-side projection consistent
// with it.
func TestFlattenOneLine_DropsControlCharacters(t *testing.T) {
	dropped := []struct {
		name string
		in   string
		want string
	}{
		{"NUL", "a\x00b", "ab"},
		{"bell", "a\x07b", "ab"},
		{"backspace", "a\x08b", "ab"},
		{"escape", "a\x1bb", "ab"},
		// Only the ESC itself is stripped: "[31m" is printable text, and a
		// colourless sequence is inert without its introducer.
		{"ANSI CSI sequence", "a\x1b[31mb", "a[31mb"},
		{"DEL", "a\x7fb", "ab"},
		{"C1 control", "a\u009bb", "ab"},
	}

	for _, tt := range dropped {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenOneLine(tt.in)
			if got != tt.want {
				t.Errorf("flattenOneLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFlattenOneLine_KeepsPrintableText pins that the fix is a strip, not a
// mangle: ordinary text, multibyte runes and intentional spacing survive.
func TestFlattenOneLine_KeepsPrintableText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "no findings here", "no findings here"},
		{"multibyte runes", "处理越界写入", "处理越界写入"}, // allow-non-english: fixture proves multibyte runes survive the strip
		{"leading and trailing space", "  padded  ", "padded"},
		{"inner spaces collapse via trim only", "a  b", "a  b"},
		{"empty", "", ""},
		{"whitespace only", "  \n  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flattenOneLine(tt.in); got != tt.want {
				t.Errorf("flattenOneLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestBuildConfirmedCommentsBlock_StaysSingleLine pins the end-to-end effect: a
// prior-round finding whose text carries line breaks or control characters must
// still render as one "code:" / "issue:" line each. This is the block handed to
// the model, so a surviving break shifts every later line and the model can no
// longer tell which finding it is being told not to repeat.
func TestBuildConfirmedCommentsBlock_StaysSingleLine(t *testing.T) {
	comments := []model.LlmComment{
		{
			Path:         "internal/agent/agent.go",
			ExistingCode: "if err != nil {\n\treturn err\x1b[0m\n}",
			Content:      "Missing\x0bwrap; also\u2028a stray separator",
		},
	}

	block := buildConfirmedCommentsBlock(comments)

	var codeLines, issueLines int
	for _, line := range strings.Split(block, "\n") {
		switch {
		case strings.HasPrefix(line, "   code: "):
			codeLines++
		case strings.HasPrefix(line, "   issue: "):
			issueLines++
		}
	}
	if codeLines != 1 || issueLines != 1 {
		t.Fatalf("got %d code: line(s) and %d issue: line(s), want 1 of each\nblock:\n%s", codeLines, issueLines, block)
	}
	if strings.ContainsAny(block, "\x1b\x0b\u2028\x85") {
		t.Errorf("control characters survived into the prompt block:\n%q", block)
	}
}
