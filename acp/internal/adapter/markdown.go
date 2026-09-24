// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"fmt"
	"path/filepath"
	"strings"
)

func markdownLabel(text string) string {
	return strings.NewReplacer("&", "&amp;", "\\", "\\\\", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "`", "\\`", "*", "\\*", "_", "\\_", "#", "\\#", "!", "\\!", "<", "&lt;", ">", "&gt;", "\r", " ", "\n", " ").Replace(text)
}

func writeCodeBlock(b *strings.Builder, path, code string) {
	language := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".mjs": "javascript", ".jsx": "jsx",
		".ts": "typescript", ".tsx": "tsx", ".rs": "rust", ".java": "java", ".c": "c", ".h": "c",
		".cpp": "cpp", ".cs": "csharp", ".sh": "bash", ".json": "json", ".yaml": "yaml", ".yml": "yaml",
		".toml": "toml", ".html": "html", ".css": "css", ".sql": "sql", ".md": "markdown",
	}[strings.ToLower(filepath.Ext(path))]
	writeFencedBlock(b, language, code)
}

func writeFencedBlock(b *strings.Builder, language, text string) {
	// The delimiter must be longer than any run in the quoted source.
	longest, run := 2, 0
	for _, char := range text {
		if char == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	fmt.Fprintf(b, "%s%s\n%s", fence, language, text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s\n", fence)
}
