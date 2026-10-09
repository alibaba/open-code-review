// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
)

func TestProgressLogPreservesLinesAndBounds(t *testing.T) {
	var log progressLog
	log.append("")
	log.append("[ocr] first\r\n")
	log.append("[ocr] second")
	if log.text != "[ocr] first\n[ocr] second\n" {
		t.Fatalf("lines joined: %q", log.text)
	}
	data, _ := json.Marshal(log.content())
	if !strings.Contains(string(data), `[ocr] first\n[ocr] second\n`) {
		t.Fatalf("logs not rendered as code: %s", data)
	}
	log.append(strings.Repeat("\u20ac", progressLogLimit))
	if len(log.text) > progressLogLimit || !utf8.ValidString(log.text) || !log.truncated {
		t.Fatal("log tail is not bounded valid UTF-8")
	}
	log.append("invalid\xff")
	data, _ = json.Marshal(log.content())
	if !strings.Contains(string(data), "truncated") || !strings.Contains(log.text, "invalid?") {
		t.Fatalf("missing truncation or encoding repair: %s", data)
	}
}

func TestExecutionCommandQuotesArguments(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"ocr", "ocr"}, {"--format=json", "--format=json"}, {"", "''"},
		{"two words", "'two words'"}, {"a'b", "'a'\"'\"'b'"},
		{"$(echo unsafe);*", "'$(echo unsafe);*'"}, {"a\nb", "'a\nb'"},
	} {
		if got := shellArgument(tc.value); got != tc.want {
			t.Errorf("quote %q = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestProgressTitleIsSingleLineBoundedAndEscaped(t *testing.T) {
	if got := progressTitle("OCR review", "", 0); got != "OCR review · Running · 0s" {
		t.Fatalf("unexpected empty title: %q", got)
	}
	if got := progressTitle("OCR scan", "previous line\n[ocr] reading [file](url)", 2*time.Second); got != "OCR scan · "+markdownLabel("reading [file](url)")+" · 2s" {
		t.Fatalf("latest activity not escaped: %q", got)
	}
	for _, message := range []string{
		"[ocr] a\tb\r c\u0085d\u202ee\u2028f\u2029",
		strings.Repeat("\u754c", 200), strings.Repeat("*", 200), "invalid\xff",
	} {
		title := progressTitle("OCR review", message, time.Minute)
		if !utf8.ValidString(title) || strings.ContainsAny(title, "\r\n\t\u0085\u202e\u2028\u2029") || len(title) > 370 {
			t.Fatalf("unsafe or unbounded title: %q", title)
		}
	}
}

func TestExecutionStatsFromCLIJSON(t *testing.T) {
	for _, operation := range []string{"review", "scan"} {
		t.Run(operation, func(t *testing.T) {
			const payload = `{"summary":{"total_tokens":14,"input_tokens":10,"output_tokens":0,"cache_read_tokens":3,"cache_write_tokens":1},"tool_calls":{"total":3,"by_tool":{"read_file":2,"code_search":1}}}`
			result := &orchestrator.Result{}
			if operation == "review" {
				result.Review = &contract.ReviewResult{}
				if err := json.Unmarshal([]byte(payload), result.Review); err != nil {
					t.Fatal(err)
				}
			} else {
				result.Scan = &contract.ScanResult{}
				if err := json.Unmarshal([]byte(payload), result.Scan); err != nil {
					t.Fatal(err)
				}
			}
			got := executionStats(result)
			for _, want := range []string{"3 calls", "code\\_search × 1", "read\\_file × 2", "14 total", "10 input", "0 output", "3 cache read", "1 cache write"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q: %s", want, got)
				}
			}
			if strings.Index(got, "code") > strings.Index(got, "read") {
				t.Errorf("tool counts are not deterministic: %s", got)
			}
		})
	}
	for _, result := range []*orchestrator.Result{nil, {}, {Review: &contract.ReviewResult{Summary: &contract.Summary{}}}} {
		if got := executionStats(result); got != "" {
			t.Errorf("missing statistics invented: %s", got)
		}
	}
}
