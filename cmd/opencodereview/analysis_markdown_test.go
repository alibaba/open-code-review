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

func TestAnalysisMarkdownOutputDestinations(test *testing.T) {
	markdown := "## [[obs-000002]]Behavior\n\n- [[obs-000003]]**Default changes** to `true` [[obs-000001]].\n\n| Scenario | Before | After |\n| --- | --- | --- |\n| Standard | Off [[obs-000001]] | On [[obs-000003]] |\n| Explicit opt-out | Off | Off |\n\n### Compatibility\n\nUse `--analysis` for the optional report.\n"
	displayMarkdown := "## Behavior\n\n- **Default changes** to `true`.\n\n| Scenario | Before | After |\n| --- | --- | --- |\n| Standard | Off | On |\n| Explicit opt-out | Off | Off |\n\n### Compatibility\n\nUse `--analysis` for the optional report.\n"
	output := &analysisOutput{ReviewStatus: "partial", Report: &workflow.Report{Status: "completed", Markdown: markdown}}
	for _, styled := range []bool{false, true} {
		setColor(test, styled)
		stdout := captureStdout(test, func() { outputAnalysisText(os.Stdout, output) })
		if styled {
			if strings.Contains(stdout, "**Default changes**") || !strings.Contains(stdout, "\x1b[1m") {
				test.Fatalf("stdout was not rendered: %q", stdout)
			}
			for _, content := range []string{"Scenario", "Before", "After", "Standard", "Explicit opt-out", "Off", "On"} {
				if !strings.Contains(ansi.Strip(stdout), content) {
					test.Fatalf("rendered comparison table lost %q: %q", content, stdout)
				}
			}
			if !strings.Contains(stdout, "\x1b[36m--analysis") {
				test.Fatalf("inline code was not cyan: %q", stdout)
			}
		} else if !strings.Contains(stdout, displayMarkdown) || strings.Contains(stdout, "\x1b") {
			test.Fatalf("plain stdout was changed: %q", stdout)
		}
		if strings.Contains(stdout, "[[obs-") {
			test.Fatalf("stdout leaked internal citations: %q", stdout)
		}
		if strings.Count(stdout, "Review: partial") != 1 {
			test.Fatalf("stdout omitted or repeated the Review failure: %q", stdout)
		}
		var buffer bytes.Buffer
		outputAnalysisText(&buffer, output)
		if !strings.Contains(buffer.String(), displayMarkdown) || strings.Contains(buffer.String(), "\x1b") || strings.Contains(buffer.String(), "[[obs-") {
			test.Fatalf("non-stdout writer was styled: %q", buffer.String())
		}
		if strings.Count(buffer.String(), "Review: partial") != 1 {
			test.Fatalf("non-stdout writer omitted or repeated the Review failure: %q", buffer.String())
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
	if err != nil || !strings.Contains(string(content), displayMarkdown) || bytes.Contains(content, []byte("\x1b")) || bytes.Contains(content, []byte("[[obs-")) {
		test.Fatalf("file output changed: %q, %v", content, err)
	}
	if bytes.Count(content, []byte("Review: partial")) != 1 {
		test.Fatalf("file output omitted or repeated the Review failure: %q", content)
	}
	if output.Report.Markdown != markdown {
		test.Fatal("rendering mutated the original report")
	}
}

func TestAnalysisMarkdownForDisplay(test *testing.T) {
	for _, fixture := range []struct{ name, markdown, want string }{
		{"no_citations", "## Heading\n\n  - Keep  spacing.  \n\n```go\n    return true\n```\n", "## Heading\n\n  - Keep  spacing.  \n\n```go\n    return true\n```\n"},
		{"adjacent_citations", "Supported. [[obs-000001]] [[obs-000002]][[obs-3]]\n\nNext paragraph.", "Supported.\n\nNext paragraph."},
		{"inline", "Before [[obs-1]] and after [[obs-2]]; affected [[obs-3]].", "Before and after; affected."},
		{"word_boundary", "Before [[obs-1]]after", "Before after"},
		{"no_added_space", "before[[obs-1]]after", "beforeafter"},
		{"unicode", "\u884c\u4e3a\u53d8\u5316 [[obs-1]]\u3002\u517c\u5bb9\u6027 [[obs-2]]\uff1a", "\u884c\u4e3a\u53d8\u5316\u3002\u517c\u5bb9\u6027\uff1a"},
		{"lists", "- First [[obs-1]]\n  - Nested [[obs-2]]\n\n1. Next [[obs-3]]\n", "- First\n  - Nested\n\n1. Next\n"},
		{"table", "| Before | After |\n| --- | --- |\n| Old [[obs-1]] | New [[obs-2]] |\n", "| Before | After |\n| --- | --- |\n| Old | New |\n"},
		{"hard_line_break", "First [[obs-1]]  \nSecond.\n", "First  \nSecond.\n"},
		{"citation_line", "First\n[[obs-1]] [[obs-2]]\n\nSecond", "First\n\n\nSecond"},
		{"only_citations", "[[obs-1]][[obs-2]]", ""},
		{"reserved_inside_code", "```text\n  item [[obs-1]]\n```\n\n`value [[obs-2]]`", "```text\n  item\n```\n\n`value`"},
		{"formatting", "**Bold [[obs-1]]** and *emphasis [[obs-2]]*.", "**Bold** and *emphasis*."},
		{"inside_opening_formatting", "**[[obs-1]] Bold** and *[[obs-2]] emphasis* and ~~[[obs-3]] obsolete~~", "**Bold** and *emphasis* and ~~obsolete~~"},
		{"list_opening_emphasis", "- [[obs-1]]**Default changes**", "- **Default changes**"},
		{"ordered_list_opening_code", "1. [[obs-1]]`--analysis` is optional", "1. `--analysis` is optional"},
		{"heading_opening_emphasis", "## [[obs-1]]*Behavior*", "## *Behavior*"},
		{"quoted_nested_list", "> - [[obs-1]]**Change**\n>   - [[obs-2]]_Impact_", "> - **Change**\n>   - _Impact_"},
		{"nested_list_content_spacing", "- [[obs-1]] **Parent**\n  - [[obs-2]] `child`", "- **Parent**\n  - `child`"},
		{"heading_content_spacing", "## [[obs-1]] [[obs-2]] *Behavior*", "## *Behavior*"},
		{"structural_space_before_punctuation", "- [[obs-1]]!\n\n## [[obs-2]]?", "- !\n\n## ?"},
		{"empty_list", "- [[obs-1]]", "- "},
		{"opening_inline_formatting", "Before [[obs-1]]_after_ and [[obs-2]]`code` and [[obs-3]]~~obsolete~~", "Before _after_ and `code` and ~~obsolete~~"},
		{"closing_inline_formatting", "_before [[obs-1]]_ and ~~obsolete [[obs-2]]~~ and `value [[obs-3]]`suffix", "_before_ and ~~obsolete~~ and `value`suffix"},
		{"formatting_after_unicode", "\u884c\u4e3a [[obs-1]]\uff1a\n\n- [[obs-2]]**Change**", "\u884c\u4e3a\uff1a\n\n- **Change**"},
		{"leading_indentation", "    [[obs-1]]content", "    content"},
		{"non_markers", "obs-000001, [obs-000002], [[obs-invalid]], [[other-1]]", "obs-000001, [obs-000002], [[obs-invalid]], [[other-1]]"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			if got := analysisMarkdownForDisplay(fixture.markdown); got != fixture.want {
				test.Fatalf("got %q, want %q", got, fixture.want)
			}
		})
	}
}

func TestAnalysisCitationAtEmphasisStart(test *testing.T) {
	for _, delimiter := range []string{"**", "*", "__", "_", "~~"} {
		test.Run(delimiter, func(test *testing.T) {
			markdown := delimiter + "[[obs-1]] Emphasized" + delimiter
			rendered := renderMarkdown(analysisMarkdownForDisplay(markdown), 80)
			if got := strings.TrimSpace(ansi.Strip(rendered)); got != "Emphasized" {
				test.Fatalf("citation removal broke emphasis: %q", rendered)
			}
			if !strings.Contains(rendered, "\x1b[") {
				test.Fatalf("emphasis was not styled: %q", rendered)
			}
		})
	}
}
