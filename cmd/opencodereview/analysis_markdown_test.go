// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/workflow"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderAnalysisMarkdown(test *testing.T) {
	markdown := "# Takeaway\n\n## Behavior\n\n- **Default changes** from `false` to `true` [[obs-000001]].\n- *Compatibility* is ~~unchanged~~ affected.\n\n```go\nif enabled {\n    return true\n}\n```\n"
	rendered := renderMarkdown(markdown, 80)
	plain := ansi.Strip(rendered)
	for _, want := range []string{"Takeaway", "Behavior", "Default changes", "false", "true", "[[obs-000001]]", "Compatibility", "unchanged", "affected", "if enabled {", "return true"} {
		if !strings.Contains(plain, want) {
			test.Errorf("missing %q in %q", want, plain)
		}
	}
	for _, delimiter := range []string{"# Takeaway", "## Behavior", "**", "`", "~~", "*Compatibility*"} {
		if strings.Contains(plain, delimiter) {
			test.Errorf("unrendered delimiter %q in %q", delimiter, plain)
		}
	}
	if !strings.Contains(rendered, "\x1b[1m") || !strings.Contains(rendered, "\x1b[36m") {
		test.Errorf("missing heading/emphasis or code styling: %q", rendered)
	}
}

func TestRenderAnalysisMarkdownWrapAndRetention(test *testing.T) {
	for _, width := range []int{24, 80, 0} {
		markdown := strings.Repeat("Before \u884c\u4e3a\u53d8\u66f4 after compatibility e\u0301 \U0001f600. ", 160)
		rendered := ansi.Strip(renderMarkdown(markdown, width))
		if strings.Join(strings.Fields(rendered), " ") != strings.Join(strings.Fields(markdown), " ") {
			test.Fatalf("report content changed at width %d", width)
		}
		expectedWidth := width
		if expectedWidth == 0 {
			expectedWidth = 80
		}
		for _, line := range strings.Split(rendered, "\n") {
			if ansi.StringWidth(line) > expectedWidth {
				test.Fatalf("line exceeds width %d: %q", expectedWidth, line)
			}
		}
	}
	table := "| Trigger | Impact |\n| --- | --- |\n| enabled | Retain every important compatibility boundary and condition [[obs-000002]] |\n"
	plain := strings.Join(strings.Fields(ansi.Strip(renderMarkdown(table, 38))), " ")
	for _, word := range []string{"Trigger", "Impact", "enabled", "Retain", "every", "important", "compatibility", "boundary", "condition", "[[obs-000002]]"} {
		if !strings.Contains(plain, word) {
			test.Errorf("table lost %q: %s", word, plain)
		}
	}
}

func TestRenderAnalysisMarkdownTerminalSafety(test *testing.T) {
	markdown := "**Safe**\x1b[2J\x1b]52;c;c2VjcmV0\a\n\n[Reference](https://example.invalid/evidence)\n\n![Diagram](https://example.invalid/image.png)\n\n&#27;]52;c;c2VjcmV0&#7;\n"
	rendered := renderMarkdown(markdown, 100)
	for _, unsafe := range []string{"\x1b]", "\x1b[2J", "\a", "c2VjcmV0"} {
		if strings.Contains(rendered, unsafe) {
			test.Errorf("unsafe terminal content %q: %q", unsafe, rendered)
		}
	}
	for _, want := range []string{"Safe", "Reference", "https://example.invalid/evidence", "Diagram", "https://example.invalid/image.png"} {
		if !strings.Contains(ansi.Strip(rendered), want) {
			test.Errorf("missing link/image text %q: %q", want, rendered)
		}
	}
	input := "\x1b[1mBold\x1b[0m\n\t\x1b[2J\x1b]0;title\a\x1b]8;;https://example.invalid\x1b\\link\x1b]8;;\x1b\\\r\x00"
	if got := sanitizeRenderedMarkdown(input); got != "\x1b[1mBold\x1b[0m\n\tlink" {
		test.Fatalf("unsafe or lost output: %q", got)
	}
}

func TestAnalysisMarkdownOutputDestinations(test *testing.T) {
	markdown := "## Behavior\n\n**Default changes** to `true` [[obs-000001]].\n"
	output := &analysisOutput{ReviewStatus: "complete", Report: &workflow.Report{Status: "completed", Markdown: markdown}}
	for _, styled := range []bool{false, true} {
		setColor(test, styled)
		stdout := captureStdout(test, func() { outputAnalysisText(os.Stdout, output) })
		if styled {
			if strings.Contains(stdout, "**Default changes**") || !strings.Contains(stdout, "\x1b[1m") {
				test.Fatalf("stdout was not rendered: %q", stdout)
			}
		} else if !strings.Contains(stdout, markdown) || strings.Contains(stdout, "\x1b") {
			test.Fatalf("plain stdout was changed: %q", stdout)
		}
		var buffer bytes.Buffer
		outputAnalysisText(&buffer, output)
		if !strings.Contains(buffer.String(), markdown) || strings.Contains(buffer.String(), "\x1b") {
			test.Fatalf("non-stdout writer was styled: %q", buffer.String())
		}
	}
	setColor(test, true)
	path := filepath.Join(test.TempDir(), "analysis.txt")
	writer, closeWriter, err := resolveOutputWriter(path, "text")
	if err != nil {
		test.Fatal(err)
	}
	outputAnalysisText(writer, output)
	if err := closeWriter(); err != nil {
		test.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), markdown) || bytes.Contains(content, []byte("\x1b")) {
		test.Fatalf("file output changed: %q, %v", content, err)
	}
	if output.Report.Markdown != markdown {
		test.Fatal("rendering mutated the original report")
	}
}
