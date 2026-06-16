package main

import "testing"

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
