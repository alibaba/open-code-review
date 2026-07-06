package main

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestBuildBadge(t *testing.T) {
	tests := []struct {
		name    string
		comment model.LlmComment
		want    string
	}{
		{"both fields", model.LlmComment{Category: "security", Severity: "high"}, "[security · high]"},
		{"category only", model.LlmComment{Category: "bug"}, "[bug]"},
		{"severity only", model.LlmComment{Severity: "low"}, "[low]"},
		{"neither", model.LlmComment{}, ""},
		{"strips control chars", model.LlmComment{Category: "bug\x1b[0m", Severity: "high"}, "[bug[0m · high]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildBadge(tt.comment); got != tt.want {
				t.Errorf("buildBadge() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSeverityColor(t *testing.T) {
	// Each known severity must map to a distinct color; unknown falls back to dim.
	seen := map[string]string{}
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		c := severityColor(sev)
		if c == "" {
			t.Errorf("severityColor(%q) is empty", sev)
		}
		if prev, ok := seen[c]; ok {
			t.Errorf("severityColor(%q) shares color with %q", sev, prev)
		}
		seen[c] = sev
	}
	if got := severityColor("bogus"); got != "\033[2m" {
		t.Errorf("severityColor(unknown) = %q, want dim", got)
	}
	if got := severityColor(""); got != "\033[2m" {
		t.Errorf("severityColor(empty) = %q, want dim", got)
	}
}

// TestRenderComment_BadgeInline verifies the badge is colorized and rendered inline
// with the first line of the comment content.
func TestRenderComment_BadgeInline(t *testing.T) {
	out := captureStdout(t, func() {
		renderComment(model.LlmComment{
			Path:      "internal/mcp/client.go",
			StartLine: 27,
			EndLine:   27,
			Content:   "Potential environment variable leak.",
			Category:  "security",
			Severity:  "high",
		})
	})
	if !strings.Contains(out, "[security · high]") {
		t.Errorf("expected badge in output, got:\n%s", out)
	}
	// severity high → bright red; the badge must be wrapped in the color + reset.
	if !strings.Contains(out, "\033[91m[security · high]\033[0m") {
		t.Errorf("expected colorized badge, got:\n%q", out)
	}
	if !strings.Contains(out, "Potential environment variable leak.") {
		t.Errorf("expected content in output, got:\n%s", out)
	}
}

func TestSanitizeTerminal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text unchanged", "hello world", "hello world"},
		{"preserves tab", "col1\tcol2", "col1\tcol2"},
		{"preserves newline", "line1\nline2", "line1\nline2"},
		{"strips ESC", "before\x1b[2Jafter", "before[2Jafter"},
		{"strips OSC 52", "\x1b]52;c;dGVzdA==\x07", "]52;c;dGVzdA=="},
		{"strips BEL alone", "beep\x07done", "beepdone"},
		{"strips null byte", "a\x00b", "ab"},
		{"strips DEL", "a\x7fb", "ab"},
		{"strips carriage return", "fake\rreal", "fakereal"},
		{"empty string", "", ""},
		{"only control chars", "\x1b\x07\x00\x7f", ""},
		{"unicode preserved", "代码审查 レビュー 🔍", "代码审查 レビュー 🔍"},
		{"mixed safe and unsafe", "path\x1b[0m/file.go", "path[0m/file.go"},
		{"strips C1 CSI (U+009B)", "beforeafter", "beforeafter"},
		{"strips C1 OSC (U+009D)", "beforeafter", "beforeafter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTerminal(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeTerminal(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
