// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"strings"
	"testing"
)

func TestEnclosingReviewFunctionsReturnsExactSpans(t *testing.T) {
	for _, tc := range []struct {
		name, path, prefix, body, suffix string
		start, end                       int
	}{
		{
			name: "Go method with physical lines and CRLF", path: "pricing.go",
			prefix: "package pricing\r\n\r\ntype price int\r\n//line old.go:900\r\n",
			body:   "func (p price) total(delta int) int {\r\n\tamount := int(p) + delta\r\n\treturn amount\r\n}",
			suffix: "\r\n\r\nvar other = 1\r\n", start: 5, end: 8,
		},
		{
			name: "Python decorated async method", path: "pricing.py",
			prefix: "class Price:\n",
			body:   "    @classmethod\n    async def total(cls, amount):\n        adjusted = amount + 1\n        return adjusted",
			suffix: "\n\n# Unrelated module setup\nready = True\n", start: 2, end: 5,
		},
		{
			name: "JavaScript function", path: "pricing.mjs",
			prefix: "const display = 'example';\n\n",
			body:   "export function total(amount) {\n  return amount + 1;\n}",
			suffix: "\n\nconst arrow = (x) => x;\n", start: 3, end: 5,
		},
		{
			name: "JavaScript method", path: "pricing.js",
			prefix: "class Price {\n",
			body:   "  total(amount) {\n    return amount + 1;\n  }",
			suffix: "\n}\n", start: 2, end: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := Citation{Kind: "code", Path: tc.path, StartLine: tc.start, EndLine: tc.end, Quote: tc.body}
			found := EnclosingReviewFunctions(tc.path, tc.prefix+tc.body+tc.suffix, tc.end, 1, tc.start, 0, 100)
			if len(found) != 5 || found[0] != want || found[2] != want ||
				found[1] != (Citation{}) || found[3] != (Citation{}) || found[4] != (Citation{}) {
				t.Fatalf("expected aligned exact function spans: %+v", found)
			}
		})
	}
}

func TestEnclosingReviewFunctionsKeepsLongCompleteCalculation(t *testing.T) {
	const path = "pricing.py"
	body := "def net_amount(ordered):\n    current = ordered\n" +
		strings.Repeat("    current = current + 1\n", 77) +
		"    return current"
	found := EnclosingReviewFunctions(path, body+"\n\nother = 3\n", 79, 2, 80)
	want := Citation{Kind: "code", Path: path, StartLine: 1, EndLine: 80, Quote: body}
	for _, c := range found {
		if c != want {
			t.Fatalf("long calculation omitted or clipped. Line range %d-%d, length %d", c.StartLine, c.EndLine, len(c.Quote))
		}
	}
}

func TestEnclosingReviewFunctionsWithholdsIneligibleOrIncompleteText(t *testing.T) {
	valid := "package pricing\n\nfunc total() int {\n\treturn 1\n}\n"
	for _, tc := range []struct {
		name, path, content string
		line                int
	}{
		{"credential path", "token=SensitiveValue.go", valid, 4},
		{"secret content outside function", "pricing.go", valid + "\nvar api_key = \"placeholderCredential\"\n", 4},
		{"traversal", "../pricing.go", valid, 4},
		{"invalid UTF8", "pricing.go", valid + "\xff", 4},
		{"binary", "pricing.go", valid + "\x00", 4},
		{"file too large", "pricing.go", valid + "/*" + strings.Repeat("x", maxEvidenceBytes) + "*/", 4},
		{"function too large", "pricing.go", "package pricing\nfunc total() string { return \"" + strings.Repeat("x", maxReviewFunctionBytes) + "\" }\n", 2},
		{"incomplete Go body", "pricing.go", "package pricing\nfunc total() {}\nfunc unfinished() {\n", 2},
		{"incomplete Python definition", "pricing.py", "def total(value):\n", 1},
		{"incomplete JavaScript body", "pricing.js", "function total(value) {\n  return value;\n", 2},
		{"Go statement sharing the edge", "pricing.go", "package pricing; func total() {}\n", 1},
		{"JavaScript two declarations on one line", "pricing.js", "function total() {} function other() {}\n", 1},
		{"JavaScript method shares class closing line", "pricing.js", "class Price {\n  total(value) { return value; }}\n", 2},
		{"JavaScript methods share a physical line", "pricing.js", "class Price {\n  total(value) { return value; } other() { return 0; }\n}\n", 2},
		{"JavaScript assignment", "pricing.js", "const total = () => {\n  return 1;\n};\n", 2},
		{"Python class itself", "pricing.py", "class Price:\n    rate = 3\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EnclosingReviewFunctions(tc.path, tc.content, tc.line)
			if len(got) != 1 || got[0] != (Citation{}) {
				t.Fatalf("unexpected evidence for incomplete or excluded input: %+v", got)
			}
		})
	}
}
