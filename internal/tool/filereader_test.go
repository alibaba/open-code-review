package tool

import (
	"strings"
	"testing"
)

func TestParseReviewMode(t *testing.T) {
	tests := []struct {
		name           string
		from, to, comm string
		want           ReviewMode
	}{
		{"workspace default", "", "", "", ModeWorkspace},
		{"range mode", "HEAD~1", "HEAD", "", ModeRange},
		{"commit mode", "", "", "abc123", ModeCommit},
		{"commit takes precedence", "a", "b", "c", ModeCommit},
		{"from only is workspace", "HEAD~1", "", "", ModeWorkspace},
		{"to only is workspace", "", "HEAD", "", ModeWorkspace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseReviewMode(tt.from, tt.to, tt.comm)
			if got != tt.want {
				t.Errorf("ParseReviewMode(%q,%q,%q) = %v, want %v", tt.from, tt.to, tt.comm, got, tt.want)
			}
		})
	}
}

func TestReviewMode_RefValue(t *testing.T) {
	tests := []struct {
		name    string
		mode    ReviewMode
		toRef   string
		commit  string
		wantRef string
		wantOK  bool
	}{
		{"workspace returns empty", ModeWorkspace, "HEAD", "abc", "", false},
		{"range returns toRef", ModeRange, "HEAD", "", "HEAD", true},
		{"commit returns commit", ModeCommit, "HEAD", "abc123", "abc123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := tt.mode.RefValue(tt.toRef, tt.commit)
			if ref != tt.wantRef || ok != tt.wantOK {
				t.Errorf("RefValue(%q,%q) = (%q,%v), want (%q,%v)", tt.toRef, tt.commit, ref, ok, tt.wantRef, tt.wantOK)
			}
		})
	}
}

func TestScanLines(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		startLine int
		maxLines  int
		wantLines []string
		wantTotal int
	}{
		{
			name:      "full file",
			input:     "line1\nline2\nline3\n",
			startLine: 1,
			maxLines:  100,
			wantLines: []string{"line1", "line2", "line3", ""},
			wantTotal: 4,
		},
		{
			name:      "no trailing newline",
			input:     "line1\nline2",
			startLine: 1,
			maxLines:  100,
			wantLines: []string{"line1", "line2"},
			wantTotal: 2,
		},
		{
			name:      "start from line 2",
			input:     "a\nb\nc\n",
			startLine: 2,
			maxLines:  100,
			wantLines: []string{"b", "c", ""},
			wantTotal: 4,
		},
		{
			name:      "limit lines",
			input:     "a\nb\nc\nd\n",
			startLine: 1,
			maxLines:  2,
			wantLines: []string{"a", "b"},
			wantTotal: 5,
		},
		{
			name:      "start beyond end",
			input:     "a\nb\n",
			startLine: 10,
			maxLines:  100,
			wantLines: nil,
			wantTotal: 3,
		},
		{
			name:      "empty input",
			input:     "",
			startLine: 1,
			maxLines:  100,
			wantLines: nil,
			wantTotal: 0,
		},
		{
			name:      "crlf line endings",
			input:     "line1\r\nline2\r\n",
			startLine: 1,
			maxLines:  100,
			wantLines: []string{"line1", "line2", ""},
			wantTotal: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, total, err := scanLines(strings.NewReader(tt.input), tt.startLine, tt.maxLines)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if len(lines) != len(tt.wantLines) {
				t.Fatalf("lines count = %d, want %d; lines=%v", len(lines), len(tt.wantLines), lines)
			}
			for i, l := range lines {
				if l != tt.wantLines[i] {
					t.Errorf("line[%d] = %q, want %q", i, l, tt.wantLines[i])
				}
			}
		})
	}
}
