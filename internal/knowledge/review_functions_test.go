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

func TestReviewEffectEvidenceReturnsCompleteModuleDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, path, prefix, declaration, between, body string
		start, end, changed                            int
	}{
		{
			name: "Go grouped constant", path: "pricing.go", prefix: "package pricing\r\n",
			declaration: "const (\r\n\tfirst = iota + 4\r\n\tallowed\r\n)", between: "\r\n\r\n//line alternate.go:40\r\n",
			body: "func total(quantity int) int {\r\n\treturn quantity + allowed\r\n}", start: 4, end: 4, changed: 9,
		},
		{
			name: "Python annotated assignment", path: "pricing.py", prefix: "# Dispatch fees\n",
			declaration: "ALLOWED: int = (\n    4 + 1\n)", between: "\n\n# Dispatch entry point\n",
			body: "def total(quantity):\n    return quantity + ALLOWED", start: 2, end: 4, changed: 8,
		},
		{
			name: "Go map literal key", path: "pricing.go", prefix: "package pricing\n",
			declaration: "const index = 2", between: "\n\n",
			body: "func total() int {\n\treturn map[int]int{index: 7}[2]\n}", start: 2, end: 2, changed: 5,
		},
		{
			name: "Go indexed array literal key", path: "pricing.go", prefix: "package pricing\n",
			declaration: "const index = 2", between: "\n\n",
			body: "func total() int {\n\treturn [...]int{index: 7}[2]\n}", start: 2, end: 2, changed: 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := tc.prefix + tc.declaration + tc.between + tc.body + "\n"
			function, premise, declaration := ReviewEffectEvidence(tc.path, content, tc.changed, tc.start, tc.end)
			if function.Quote != tc.body || function.EndLine < tc.changed || !declaration || premise.Quote != tc.declaration ||
				premise.StartLine != 2 || premise.EndLine != 2+strings.Count(tc.declaration, "\n") {
				t.Fatalf("changed function = %+v, premise = %+v, declaration = %v", function, premise, declaration)
			}
		})
	}
}

func TestReviewEffectEvidenceRejectsUnboundOrIncompleteDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, path, content string
		changed, start, end int
	}{
		{"Go local shadow", "pricing.go", "package pricing\nconst allowed = 4\nfunc total(allowed int) int { return allowed }\n", 3, 2, 2},
		{"Go struct field", "pricing.go", "package pricing\nconst allowed = 4\nfunc total() int { return struct{ allowed int }{allowed: 2}.allowed }\n", 3, 2, 2},
		{"Go variable", "pricing.go", "package pricing\nvar allowed = 4\nfunc total() int { return allowed }\n", 3, 2, 2},
		{"Go change outside function", "pricing.go", "package pricing\nconst allowed = 4\nfunc total() int { return allowed }\n", 2, 2, 2},
		{"Go shared line", "pricing.go", "package pricing\nconst allowed = 4; const price = 3\nfunc total() int { return allowed }\n", 3, 2, 2},
		{"Python parameter shadow", "pricing.py", "ALLOWED = 4\ndef total(ALLOWED):\n    return ALLOWED\n", 3, 1, 1},
		{"Python without reference", "pricing.py", "ALLOWED = 4\ndef total(x):\n    return x.ALLOWED\n", 3, 1, 1},
		{"Python reassignment", "pricing.py", "ALLOWED = 4\nALLOWED += 1\ndef total():\n    return ALLOWED\n", 4, 1, 1},
		{"Python global write", "pricing.py", "ALLOWED = 4\ndef total():\n    return ALLOWED\ndef change():\n    global ALLOWED\n    ALLOWED = 0\n", 3, 1, 1},
		{"Python compound assignment", "pricing.py", "ALLOWED, OTHER = 4, 8\ndef total():\n    return ALLOWED\n", 3, 1, 1},
		{"Python shared line", "pricing.py", "ALLOWED = 4; OTHER = 8\ndef total():\n    return ALLOWED\n", 3, 1, 1},
		{"Python quote crosses declarations", "pricing.py", "ALLOWED = 4\ndef total():\n    return ALLOWED\n", 3, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, premise, declaration := ReviewEffectEvidence(tc.path, tc.content, tc.changed, tc.start, tc.end)
			if declaration || premise != (Citation{}) {
				t.Fatalf("unavailable declaration furnished evidence: %+v", premise)
			}
		})
	}
}

func TestJavaScriptRegexMaskPreservesDivisionAndCoordinates(t *testing.T) {
	const function = "function total(n, other) {\r\n" +
		"  if (n > 0) /[(]/.test(other);\r\n" +
		"  return other.return / (n + 1) / n++ / n;\r\n}"
	const source = `const regexp = /[()\/\]]+/gi;` + "\r\n" +
		"const ratio = {} / 2 / 3;\r\n" + function + "\r\n"
	masked, ok := maskJavaScript(source)
	if !ok || len(masked) != len(source) {
		t.Fatalf("invalid source mask: %q", masked)
	}
	for _, pattern := range []string{`/[()\/\]]+/gi`, `/[(]/`} {
		start := strings.Index(source, pattern)
		if string(masked[start:start+len(pattern)]) != "_"+strings.Repeat(" ", len(pattern)-1) {
			t.Fatalf("regex was not masked at its offsets: %q", masked)
		}
	}
	if !strings.Contains(string(masked), "other.return / (n + 1) / n++ / n;") || !strings.Contains(string(masked), "{} / 2 / 3;") {
		t.Fatalf("division changed in masked code: %q", masked)
	}
	got := EnclosingReviewFunctions("pricing.js", source, 4)[0]
	if got.Quote != function || got.StartLine != 3 || got.EndLine != 6 {
		t.Fatalf("unexpected JavaScript function span: %+v", got)
	}
}

func TestJavaScriptRegexMaskRejectsUnterminatedAndAmbiguousLiterals(t *testing.T) {
	for _, source := range []string{
		"const re = /[(]/\nconst bad = /[)\n",
		"const re = /[(]/gg;\n",
		"if (ok) {} /[)]/.test(value);\n",
	} {
		if masked, ok := maskJavaScript(source); ok {
			t.Fatalf("ambiguous or unfinished regex was accepted: %q", masked)
		}
	}
}
