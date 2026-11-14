// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdown(test *testing.T) {
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

func TestRenderMarkdownCodeSpanEntities(test *testing.T) {
	for _, fixture := range []struct {
		name, markdown, want string
	}{
		{"named", "Keep `&lt;script&gt;` escaped.", "Keep &lt;script&gt; escaped."},
		{"numeric", "Keep `&#60;&#x3e;&#38;` literal.", "Keep &#60;&#x3e;&#38; literal."},
		{"nested", "Keep `&amp;lt;` literal.", "Keep &amp;lt; literal."},
		{"plain ampersand", "Use `first && second`.", "Use first && second."},
		{"backticks", "Keep `` `&lt;tag&gt;` `` literal.", "Keep `&lt;tag&gt;` literal."},
		{"multiline", "Keep `&lt;\nscript&gt;` escaped.", "Keep &lt; script&gt; escaped."},
		{"emphasis", "**Keep `&lt;script&gt;` escaped.**", "Keep &lt;script&gt; escaped."},
		{"link", "[`&lt;tag&gt;`](https://example.invalid)", "&lt;tag&gt; https://example.invalid"},
		{"table", "| Example |\n| --- |\n| `&lt;tag&gt;` |\n", "&lt;tag&gt;"},
		{"encoded CSI", "Keep `&#27;[2J` literal.", "Keep &#27;[2J literal."},
		{"encoded OSC", "Keep `&#27;]52;c;c2VjcmV0&#7;` literal.", "Keep &#27;]52;c;c2VjcmV0&#7; literal."},
		{"prose", "Decode &lt;tag&gt; but keep `&lt;tag&gt;` literal.", "Decode <tag> but keep &lt;tag&gt; literal."},
		{"fenced code", "```html\n&lt;tag&gt;\n```\n", "&lt;tag&gt;"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			rendered := renderMarkdown(fixture.markdown, 100)
			plain := strings.Join(strings.Fields(ansi.Strip(rendered)), " ")
			if !strings.Contains(plain, fixture.want) {
				test.Fatalf("literal code changed: got %q, want %q", plain, fixture.want)
			}
			if sanitized := sanitizeRenderedMarkdown(rendered); sanitized != rendered {
				test.Fatalf("unsafe terminal output: %q", rendered)
			}
		})
	}
}

func TestRenderMarkdownWrapAndRetention(test *testing.T) {
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

func TestRenderMarkdownTableLinkRetention(test *testing.T) {
	longURL := "https://example.invalid/repository/blob/0123456789abcdef0123456789abcdef01234567/internal/scan/agent.go#L900"
	shortURL := "https://example.invalid"
	longLabel := strings.Repeat("Evidence for the compatibility change ", 4)
	for _, fixture := range []struct {
		name, cell, url string
	}{
		{"long URL", "[Evidence](" + longURL + ")", longURL},
		{"long label", "[" + longLabel + "](" + shortURL + ")", shortURL},
		{"image", "![Diagram](" + longURL + ")", longURL},
		{"autolink", longURL, longURL},
	} {
		for _, width := range []int{24, 38, 80, 0} {
			test.Run(fmt.Sprintf("%s/width_%d", fixture.name, width), func(test *testing.T) {
				markdown := "| Reference |\n| --- |\n| " + fixture.cell + " |\n"
				rendered := renderMarkdown(markdown, width)
				plain := strings.Join(strings.Fields(ansi.Strip(rendered)), "")
				if !strings.Contains(plain, fixture.url) {
					test.Errorf("table lost URL %q: %q", fixture.url, rendered)
				}
				if strings.Contains(rendered, "\x1b]") || sanitizeRenderedMarkdown(rendered) != rendered {
					test.Errorf("unsafe terminal output: %q", rendered)
				}
			})
		}
	}
}

func TestRenderMarkdownTerminalSafety(test *testing.T) {
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

func TestRenderMarkdownRejectsEntityEncodedStyles(test *testing.T) {
	for _, fixture := range []struct {
		name, markdown string
	}{
		{"decimal", "&#27;[8mCRITICAL FINDING&#27;[0m"},
		{"hexadecimal", "&#x1b;[8mCRITICAL FINDING&#x1B;[0m"},
		{"without semicolon", "&#27[8mCRITICAL FINDING&#27[0m"},
		{"encoded parameters", "&#27;&#91;&#56;mCRITICAL FINDING&#27;[0m"},
		{"without reset", "&#27;[8mCRITICAL FINDING"},
		{"color", "&#27;[31mCRITICAL FINDING&#27;[0m"},
		{"bold", "**&#27;[8mCRITICAL FINDING&#27;[0m**"},
		{"strikethrough", "~~&#27;[8mCRITICAL FINDING&#27;[0m~~"},
		{"split strikethrough", "~~&#2**7;[8mCRITICAL FINDING**&#27;[0m~~"},
		{"link", "[&#27;[8mCRITICAL FINDING&#27;[0m](https://example.invalid)"},
		{"table", "| Finding |\n| --- |\n| &#27;[8mCRITICAL FINDING&#27;[0m |"},
		{"HTML block", "<div>&#27;[8mCRITICAL FINDING&#27;[0m</div>"},
		{"HTML closure", "<div>\n&#27;[8mCRITICAL FINDING&#27;[0m\n</div>"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			rendered := renderMarkdown(fixture.markdown, 100)
			for _, injected := range []string{"\x1b[8m", "\x1b[31m", "\x1b[0m"} {
				if strings.Contains(rendered, injected) {
					test.Errorf("input-owned style survived: %q", rendered)
				}
			}
			if !strings.Contains(ansi.Strip(rendered), "CRITICAL FINDING") {
				test.Fatalf("finding text lost: %q", rendered)
			}
		})
	}
}

func TestRenderMarkdownEntityStyleLiteralRetention(test *testing.T) {
	for _, fixture := range []struct {
		name, markdown, want string
	}{
		{"inline code", "`&#27;[8mCRITICAL&#27;[0m`", "&#27;[8mCRITICAL&#27;[0m"},
		{"fenced code", "```text\n&#27;[8mCRITICAL&#27;[0m\n```", "&#27;[8mCRITICAL&#27;[0m"},
		{"indented code", "    &#27;[8mCRITICAL&#27;[0m", "&#27;[8mCRITICAL&#27;[0m"},
		{"escaped entity", "&amp;#27;[8mCRITICAL", "&#27;[8mCRITICAL"},
		{"adjacent entity", "&#27;[8mCRITICAL &lt;tag&gt; &amp;#27;[8m&#27;[0m", "CRITICAL <tag> &#27;[8m"},
		{"HTML entity", "<div>&#27;[8mCRITICAL &lt;tag&gt; &amp;#27;[8m&#27;[0m</div>", "CRITICAL <tag> &#27;[8m"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			rendered := renderMarkdown(fixture.markdown, 100)
			if strings.Contains(rendered, "\x1b[8m") || !strings.Contains(ansi.Strip(rendered), fixture.want) {
				test.Fatalf("unsafe or changed literal: %q; want %q", rendered, fixture.want)
			}
		})
	}
}

func TestRenderMarkdownRetainsStylesAfterSanitizingEntities(test *testing.T) {
	markdown := "**Visible** &#27;[8mCRITICAL FINDING&#27;[0m with *emphasis* and `&lt;code&gt;`."
	rendered := renderMarkdown(markdown, 100)
	for _, style := range []string{"\x1b[1m", "\x1b[3m", "\x1b[36m"} {
		if !strings.Contains(rendered, style) {
			test.Errorf("renderer style lost: %q", rendered)
		}
	}
	want := "Visible CRITICAL FINDING with emphasis and &lt;code&gt;."
	if strings.Contains(rendered, "\x1b[8m") || strings.TrimSpace(ansi.Strip(rendered)) != want {
		test.Fatalf("unsafe or changed content: %q; want %q", rendered, want)
	}
}

func TestRenderMarkdownDelimiterBoundaries(test *testing.T) {
	for _, fixture := range []struct {
		markdown string
		literal  bool
	}{
		{"**\u6807\u7b7e\uff1a** \u6b63\u6587", false},
		{"**\u6807\u7b7e\uff1a**\u6b63\u6587", true},
		{"**\u6807\u7b7e**\uff1a\u6b63\u6587", false},
	} {
		plain := ansi.Strip(renderMarkdown(fixture.markdown, 80))
		if strings.Contains(plain, "**") != fixture.literal {
			test.Errorf("unexpected CommonMark delimiter handling: %q -> %q", fixture.markdown, plain)
		}
	}
}

func TestRenderedMarkdownRejectsPrivateCSI(test *testing.T) {
	for _, sequence := range []string{"\x1b[>4;2m", "\x1b[?1m", "\x1b[1 m"} {
		if got := sanitizeRenderedMarkdown(sequence + "safe"); got != "safe" {
			test.Errorf("private/intermediate CSI survived: %q", got)
		}
		encoded := strings.Replace(sequence, "\x1b", "&#27;", 1)
		rendered := renderMarkdown("**Safe** "+encoded+" text", 80)
		if strings.Contains(rendered, sequence) || !strings.Contains(ansi.Strip(rendered), "Safe") {
			test.Errorf("entity-decoded terminal command survived: %q", rendered)
		}
	}
	for _, sequence := range []string{"\x1b[m", "\x1b[1;36m", "\x1b[38:2::10:20:30m"} {
		if got := sanitizeRenderedMarkdown(sequence); got != sequence {
			test.Errorf("valid SGR lost: %q -> %q", sequence, got)
		}
	}
}

func TestMarkdownOutputDestinations(test *testing.T) {
	markdown := "## Project Summary\n\n**Default changes** to `true`.\n\n&#27;[8mImportant finding&#27;[0m\n"
	result := &mockResultProvider{filesReviewed: 1, projectSummary: markdown}
	defer stdout.Quiet()()
	for _, styled := range []bool{false, true} {
		setColor(test, styled)
		stdout := captureStdout(test, func() {
			if err := emitRunResult(test.Context(), result, nil, time.Now(), "text", "human", nil, nil, os.Stdout, nil); err != nil {
				test.Fatal(err)
			}
		})
		if styled {
			if strings.Contains(stdout, "**Default changes**") || !strings.Contains(stdout, "\x1b[1m") {
				test.Fatalf("stdout was not rendered: %q", stdout)
			}
			if strings.Contains(stdout, "\x1b[8m") || !strings.Contains(stdout, "Important finding") {
				test.Fatalf("unsafe or lost summary: %q", stdout)
			}
		} else if !strings.Contains(stdout, markdown) || strings.Contains(stdout, "\x1b") {
			test.Fatalf("plain stdout was changed: %q", stdout)
		}
		var buffer bytes.Buffer
		if err := emitRunResult(test.Context(), result, nil, time.Now(), "text", "human", nil, nil, &buffer, nil); err != nil {
			test.Fatal(err)
		}
		if !strings.Contains(buffer.String(), markdown) || strings.Contains(buffer.String(), "\x1b") {
			test.Fatalf("non-stdout writer was styled: %q", buffer.String())
		}
	}
	setColor(test, true)
	path := filepath.Join(test.TempDir(), "summary.txt")
	writer, closeWriter, err := resolveOutputWriter(path, "text")
	if err != nil {
		test.Fatal(err)
	}
	if err := emitRunResult(test.Context(), result, nil, time.Now(), "text", "human", nil, nil, writer, nil); err != nil {
		test.Fatal(err)
	}
	if err := closeWriter(); err != nil {
		test.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), markdown) || bytes.Contains(content, []byte("\x1b")) {
		test.Fatalf("file output changed: %q, %v", content, err)
	}
	jsonText := captureStdout(test, func() {
		if err := emitRunResult(test.Context(), result, nil, time.Now(), "json", "human", nil, nil, os.Stdout, nil); err != nil {
			test.Fatal(err)
		}
	})
	var document jsonOutput
	if err := json.Unmarshal([]byte(jsonText), &document); err != nil || document.ProjectSummary != markdown {
		test.Fatalf("JSON summary changed: %q, %v", jsonText, err)
	}
	if strings.Contains(jsonText, "\x1b") || result.projectSummary != markdown {
		test.Fatal("rendering mutated the original report")
	}
}
