// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"strings"
	"testing"
)

func TestCodeAndLogMarkdownPreserveFencesAndNewlines(t *testing.T) {
	for _, tc := range []struct {
		name, text, code, log string
	}{
		{"empty", "", "```go\n\n```\n", "```text\n\n```\n"},
		{"no final newline", "value", "```go\nvalue\n```\n", "```text\nvalue\n```\n"},
		{"final newline", "value\n", "```go\nvalue\n```\n", "```text\nvalue\n```\n"},
		{"blank final line", "value\n\n", "```go\nvalue\n\n```\n", "```text\nvalue\n\n```\n"},
		{"multiple backtick runs", "`one`\n```two`````three``", "``````go\n`one`\n```two`````three``\n``````\n", "``````text\n`one`\n```two`````three``\n``````\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			writeCodeBlock(&b, "source.GO", tc.text)
			if got := b.String(); got != tc.code {
				t.Errorf("code block = %q, want %q", got, tc.code)
			}
			log := progressLog{text: tc.text}
			if got := log.markdown(); got != tc.log {
				t.Errorf("log block = %q, want %q", got, tc.log)
			}
		})
	}
}
