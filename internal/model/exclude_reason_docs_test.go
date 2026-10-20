// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// filterDocsLocales are the maintained translations of the pages documentation.
// The filter is described in three places that a reader may land on, so the
// reason inventory has to agree in every locale: review-rules.md lists the
// gates, and faq.md and architecture.md explain the reasons the CLI reports.
var filterDocsLocales = []string{"en", "zh", "ja", "ko", "ru"}

// excludeReasonDeclRE matches a reason constant declaration in preview.go, for
// example `ExcludeSecret ExcludeReason = "secret_exclude"`. The declarations
// themselves are the inventory: Go cannot enumerate constants, so the test
// reads the file that declares them instead of repeating the list here, where
// a new reason could be forgotten twice.
var excludeReasonDeclRE = regexp.MustCompile(`^\s*(Exclude\w+)\s+ExcludeReason\s*=\s*"([^"]*)"`)

// whyExcludedReturnRE matches a reason returned by the filter's gate function.
var whyExcludedReturnRE = regexp.MustCompile(`return (Exclude\w+)`)

// faqReasonRowRE matches a row of the FAQ's exclusion-reason table, capturing
// the reason token in the first column ("| `default_path` | ... |").
var faqReasonRowRE = regexp.MustCompile("^\\|\\s*`([a-z_]+)`\\s*\\|")

// gateCountLabelRE matches the pipeline diagram's filter node label, which is
// the same literal in every locale ("6-gate filter (selection.go) ...").
var gateCountLabelRE = regexp.MustCompile(`(\d+)-gate filter`)

// orderedGateStepRE matches a numbered step of the architecture page's gate
// list. Continuation lines are indented, so they never match.
var orderedGateStepRE = regexp.MustCompile(`^(\d+)\. `)

// declaredExcludeReasons returns every reason constant declared in preview.go,
// keyed by constant name. ExcludeNone is included (its value is the empty string
// the filter returns for a kept file) so that a `return ExcludeNone` in the gate
// function resolves; callers skip empty values when they want the reported
// reasons only.
func declaredExcludeReasons(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile("preview.go")
	if err != nil {
		t.Fatalf("read preview.go: %v", err)
	}
	reasons := make(map[string]string)
	for _, line := range strings.Split(string(body), "\n") {
		m := excludeReasonDeclRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		reasons[m[1]] = m[2]
	}
	if len(reasons) == 0 {
		t.Fatal("preview.go declares no ExcludeReason constants")
	}
	return reasons
}

// whyExcludedReasons returns the reason values the file filter's gate function
// in internal/agent/selection.go can return, in the order the function tests
// them. They are the reasons a kept-but-not-reviewed file reports, which is
// exactly what the architecture page enumerates.
func whyExcludedReasons(t *testing.T, declared map[string]string) []string {
	t.Helper()
	path := filepath.Join("..", "agent", "selection.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(body), "\n")

	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "func (a *Agent) whyExcluded(") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s no longer declares whyExcluded; update this guard", path)
	}

	var reasons []string
	seen := make(map[string]bool)
	for _, line := range lines[start+1:] {
		if line == "}" { // end of the function's body at column zero
			break
		}
		m := whyExcludedReturnRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value, ok := declared[m[1]]
		if !ok {
			t.Fatalf("whyExcluded returns %s, which preview.go does not declare", m[1])
		}
		if value == "" { // ExcludeNone: the file is kept, not excluded
			continue
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		reasons = append(reasons, value)
	}
	if len(reasons) == 0 {
		t.Fatalf("%s: found no reason returned by whyExcluded", path)
	}
	return reasons
}

// readDoc returns one locale's page body.
func readDoc(t *testing.T, locale, page string) string {
	t.Helper()
	path := filepath.Join("..", "..", "pages", "src", "content", "docs", locale, page)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// TestFAQDocumentsEveryExcludeReason keeps the FAQ's exclusion-reason table in
// step with the reasons the model declares.
//
// The table is the reference a user reaches for after seeing an
// `(excluded: ...)` reason in `--preview` or `--format json` output, so a
// reason that is missing from it sends that user looking for a fix that is
// never written down. secret_exclude was missing from all five locales while
// the review-rules page already documented the gate that produces it.
//
// The table is parsed from its `binary` row to the end of the contiguous table
// block, so both directions are asserted without depending on translated
// headings: every declared reason is listed, and nothing else is.
func TestFAQDocumentsEveryExcludeReason(t *testing.T) {
	declared := declaredExcludeReasons(t)
	for _, locale := range filterDocsLocales {
		t.Run(locale, func(t *testing.T) {
			rows := faqReasonRows(t, locale)
			if len(rows) == 0 {
				t.Fatalf("%s/faq.md: found no exclusion-reason table", locale)
			}
			for name, value := range declared {
				if value == "" { // ExcludeNone is the kept-file value, not a reason
					continue
				}
				if !rows[value] {
					t.Errorf("%s/faq.md: exclusion reason %q (%s) has no table row", locale, value, name)
				}
			}
			for value := range rows {
				if !declaresValue(declared, value) {
					t.Errorf("%s/faq.md: table row %q is not an ExcludeReason declared in preview.go", locale, value)
				}
			}
		})
	}
}

// faqReasonRows returns the reason values listed by one locale's FAQ table,
// keyed by the token in the row's first column.
func faqReasonRows(t *testing.T, locale string) map[string]bool {
	t.Helper()
	body := readDoc(t, locale, "faq.md")
	lines := strings.Split(body, "\n")

	start := -1
	for i, line := range lines {
		if m := faqReasonRowRE.FindStringSubmatch(line); m != nil && m[1] == "binary" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}

	rows := make(map[string]bool)
	for _, line := range lines[start:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		if m := faqReasonRowRE.FindStringSubmatch(line); m != nil {
			rows[m[1]] = true
		}
	}
	return rows
}

func declaresValue(declared map[string]string, value string) bool {
	for _, v := range declared {
		if v == value {
			return true
		}
	}
	return false
}

// TestArchitectureGateListNamesEveryFilterReason keeps the architecture page's
// description of the filter consistent with the function it describes.
//
// The page enumerates the reasons `whyExcluded` returns and states how many
// gates the filter has, and the pipeline diagram repeats that count in its node
// label. Both drifted when the built-in secret-path gate landed (#1262): the
// page still said five gates and omitted secret_exclude, while the review-rules
// page and the code both had six. The count and the enumeration are asserted,
// not the wording, so a translation stays free.
func TestArchitectureGateListNamesEveryFilterReason(t *testing.T) {
	declared := declaredExcludeReasons(t)
	reasons := whyExcludedReasons(t, declared)
	for _, locale := range filterDocsLocales {
		t.Run(locale, func(t *testing.T) {
			body := readDoc(t, locale, "architecture.md")

			for _, reason := range reasons {
				if !strings.Contains(body, "`"+reason+"`") {
					t.Errorf("%s/architecture.md: whyExcluded can return %q, but the page never names it", locale, reason)
				}
			}

			label := gateCountLabelRE.FindStringSubmatch(body)
			if label == nil {
				t.Fatalf("%s/architecture.md: no \"N-gate filter\" label in the pipeline diagram", locale)
			}
			want, err := strconv.Atoi(label[1])
			if err != nil {
				t.Fatalf("%s/architecture.md: unparsable gate count %q", locale, label[1])
			}
			steps := orderedGateSteps(t, locale, body)
			if len(steps) != want {
				t.Errorf("%s/architecture.md: diagram label says %d gates, the ordered list has %d steps", locale, want, len(steps))
			}
			for i, step := range steps {
				if step != i+1 {
					t.Errorf("%s/architecture.md: gate list numbers %v, want 1..%d", locale, steps, len(steps))
					break
				}
			}
		})
	}
}

// orderedGateSteps returns the step numbers of the filter's ordered gate list,
// found from the step that drops `binary` and read until the list ends.
func orderedGateSteps(t *testing.T, locale, body string) []int {
	t.Helper()
	lines := strings.Split(body, "\n")

	start := -1
	for i, line := range lines {
		if m := orderedGateStepRE.FindStringSubmatch(line); m != nil && m[1] == "1" &&
			strings.Contains(line, "`binary`") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s/architecture.md: no numbered gate list starting at `binary`", locale)
	}

	var steps []int
	for _, line := range lines[start:] {
		m := orderedGateStepRE.FindStringSubmatch(line)
		if m == nil {
			// Continuation lines of a step are indented or blank; anything
			// else ends the list.
			if strings.HasPrefix(line, "## ") {
				break
			}
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break
			}
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("%s/architecture.md: unparsable gate step %q", locale, m[1])
		}
		steps = append(steps, n)
	}
	if len(steps) == 0 {
		t.Fatalf("%s/architecture.md: gate list has no numbered steps", locale)
	}
	return steps
}
