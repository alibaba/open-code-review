package reviewbackend

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
)

func TestBuildCursorReviewPrompt_IncludesTools(t *testing.T) {
	prompt := buildCursorReviewPrompt([]llm.Message{
		llm.NewTextMessage("user", "Review this diff."),
	}, "- **code_comment**: leave feedback\n")

	if !strings.Contains(prompt, "custom-user-tools") {
		t.Fatalf("missing MCP hint: %q", prompt)
	}
	if !strings.Contains(prompt, "code_comment") {
		t.Fatalf("missing tool guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Review this diff.") {
		t.Fatalf("missing user content: %q", prompt)
	}
}

func TestNormalizeCursorToolName(t *testing.T) {
	cases := map[string]string{
		"code_comment":                    "code_comment",
		"custom-user-tools/code_comment":  "code_comment",
		"custom-user-tools__code_comment": "code_comment",
	}
	for in, want := range cases {
		if got := normalizeCursorToolName(in); got != want {
			t.Fatalf("%q => %q, want %q", in, got, want)
		}
	}
}
