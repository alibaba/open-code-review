// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

// These fixtures are the observed shape of the failure: `comments` arrives as a
// serialized string whose prose quotes a term with an unescaped double quote.
// Raw string literals express that directly — the quote needs no Go-level
// escaping, so what appears here is exactly what reached the parser.

const proseQuoteEnglish = `[{"content":"the name suggests "a trusted proxy exists" but the call returns the opposite","existing_code":"boolean ok = cfg.isEmpty();","path":"Auth.java"}]`

// The real failures were all Chinese review prose, and the byte-wise scan has to
// stay correct across multi-byte runes, so one fixture keeps that language.
const proseQuoteChinese = `[{"content":"变量名暗示的是"存在可信代理"，但调用返回的是相反的结果。","existing_code":"boolean ok = cfg.isEmpty();","path":"Auth.java"}]` // allow-non-english: encoding fixture — the reported failures occur only in Chinese prose, and multi-byte runes are what make the byte-wise scan worth testing

const twoCommentsWithProseQuotes = `[{"content":"the counter changed from "files finished" to "files started"","path":"a.go"},{"content":"the flag reads as "proxy present" but means the opposite","path":"b.go"}]`

func TestParseComments_RepairsUnescapedProseQuote(t *testing.T) {
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": proseQuoteEnglish}, "fallback.go")
	if errMsg != "" {
		t.Fatalf("expected the repair to recover the batch, got error: %s", errMsg)
	}
	if repaired != 2 {
		t.Errorf("escaped character count = %d, want 2 (both prose quotes)", repaired)
	}
	if len(comments) != 1 {
		t.Fatalf("recovered %d comments, want 1", len(comments))
	}
	// The quoted term must survive verbatim: the repair escapes the quote for
	// JSON, it does not delete it from the text.
	if !strings.Contains(comments[0].Content, `"a trusted proxy exists"`) {
		t.Errorf("quoted term lost from content: %q", comments[0].Content)
	}
	if comments[0].Path != "Auth.java" {
		t.Errorf("path = %q, want Auth.java", comments[0].Path)
	}
	if comments[0].ExistingCode != "boolean ok = cfg.isEmpty();" {
		t.Errorf("existing_code = %q", comments[0].ExistingCode)
	}
}

func TestParseComments_RepairsProseQuoteAcrossMultiByteRunes(t *testing.T) {
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": proseQuoteChinese}, "fallback.go")
	if errMsg != "" {
		t.Fatalf("expected the repair to recover the batch, got error: %s", errMsg)
	}
	if repaired != 2 {
		t.Errorf("escaped character count = %d, want 2", repaired)
	}
	if len(comments) != 1 {
		t.Fatalf("recovered %d comments, want 1", len(comments))
	}
	// A byte-wise scan that mishandled UTF-8 would corrupt or truncate this.
	if !strings.HasSuffix(comments[0].Content, "相反的结果。") { // allow-non-english: asserts the multi-byte fixture above round-trips intact
		t.Errorf("multi-byte content did not survive the repair: %q", comments[0].Content)
	}
}

func TestParseComments_RepairsBareControlCharacter(t *testing.T) {
	// A literal newline inside a JSON string is illegal; the model meant \n.
	serialized := "[{\"content\":\"first line\nsecond line\",\"path\":\"a.go\"}]"
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": serialized}, "fallback.go")
	if errMsg != "" {
		t.Fatalf("expected the repair to recover the batch, got error: %s", errMsg)
	}
	if repaired != 1 {
		t.Errorf("escaped character count = %d, want 1", repaired)
	}
	if len(comments) != 1 || comments[0].Content != "first line\nsecond line" {
		t.Fatalf("content = %q, want the newline preserved", comments[0].Content)
	}
}

func TestParseComments_RepairKeepsEveryCommentInTheBatch(t *testing.T) {
	// The count check exists for exactly this case: a repair that merged these
	// two comments into one would still produce valid JSON.
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": twoCommentsWithProseQuotes}, "fallback.go")
	if errMsg != "" {
		t.Fatalf("expected the repair to recover the batch, got error: %s", errMsg)
	}
	if repaired != 6 {
		t.Errorf("escaped character count = %d, want 6", repaired)
	}
	if len(comments) != 2 {
		t.Fatalf("recovered %d comments, want 2 (a merge must be rejected)", len(comments))
	}
	if comments[0].Path != "a.go" || comments[1].Path != "b.go" {
		t.Errorf("paths = %q, %q; want a.go, b.go", comments[0].Path, comments[1].Path)
	}
}

func TestParseComments_UnrepairableKeepsOriginalParserWording(t *testing.T) {
	// Structurally truncated: escaping the prose quote cannot make this parse.
	serialized := `[{"content":"say "hi""},{"content"`
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": serialized}, "fallback.go")
	if len(comments) != 0 || repaired != 0 {
		t.Fatalf("comments=%d repaired=%d; want the repair to decline", len(comments), repaired)
	}
	// The phrasing is load-bearing: "invalid character" is what leads the model
	// to regenerate the batch instead of resending the same broken string.
	if !strings.Contains(errMsg, "failed to parse 'comments' JSON string") {
		t.Errorf("error message lost its original wording: %q", errMsg)
	}
	if !strings.Contains(errMsg, "invalid character") {
		t.Errorf("error message must keep the parser's %q phrasing, got %q", "invalid character", errMsg)
	}
}

func TestParseComments_RepairRejectedWhenABatchEntryEndsUpEmpty(t *testing.T) {
	// The repair recovers the first comment and the text parses, but the second
	// entry carries no content. Parsing again is therefore not sufficient on its
	// own: a repair is only accepted when every entry still says something, so
	// this whole batch falls back to the original error rather than emitting a
	// blank finding.
	serialized := `[{"content":"say "hi" loudly"},{"content":""}]`
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": serialized}, "fallback.go")
	if errMsg == "" {
		t.Fatalf("repair was accepted (%d comments, %d escaped); want it declined "+
			"because one entry lost its content", len(comments), repaired)
	}
	if repaired != 0 {
		t.Errorf("repaired = %d, want 0 when the repair is rejected", repaired)
	}
	if !strings.Contains(errMsg, "invalid character") {
		t.Errorf("rejected repair must fall back to the parser wording, got %q", errMsg)
	}
}

func TestParseComments_RepairsIllegalBackslashEscape(t *testing.T) {
	// The dropped escaping level hits backslashes as well as quotes. Prose that
	// cites a regex or a Windows path leaves a backslash opening no valid
	// sequence, which failed the whole batch before.
	for _, tc := range []struct {
		name, serialized, want string
	}{
		{
			name:       "regex in prose",
			serialized: `[{"content":"the pattern \d+ only matches digits","path":"a.go"}]`,
			want:       `the pattern \d+ only matches digits`,
		},
		{
			// \U and \s open no valid sequence. A segment starting with one of
			// b/f/n/r/t/u would be a legal escape and is left alone by design —
			// see the note in repairSerializedComments.
			name:       "windows path in prose",
			serialized: `[{"content":"the path C:\Users\src is invalid","path":"a.go"}]`,
			want:       `the path C:\Users\src is invalid`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comments, repaired, errMsg := ParseCommentsWithPath(
				map[string]any{"comments": tc.serialized}, "fallback.go")
			if errMsg != "" {
				t.Fatalf("expected the repair to recover the batch, got error: %s", errMsg)
			}
			if repaired == 0 {
				t.Error("repaired = 0, want the illegal escape counted")
			}
			if len(comments) != 1 || comments[0].Content != tc.want {
				t.Fatalf("content = %q, want %q", comments[0].Content, tc.want)
			}
		})
	}
}

func TestParseComments_RepairsAllControlCharacters(t *testing.T) {
	// JSON forbids every byte below 0x20, not just the three with short escapes.
	serialized := "[{\"content\":\"bell\x07 and vtab\x0b here\",\"path\":\"a.go\"}]"
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": serialized}, "fallback.go")
	if errMsg != "" {
		t.Fatalf("expected the repair to recover the batch, got error: %s", errMsg)
	}
	if repaired != 2 {
		t.Errorf("repaired = %d, want 2", repaired)
	}
	if len(comments) != 1 || comments[0].Content != "bell\x07 and vtab\x0b here" {
		t.Fatalf("content = %q, want the control characters preserved", comments[0].Content)
	}
}

func TestParseComments_RepairRejectedWhenProseBecameAField(t *testing.T) {
	// Ending a string early re-reads the prose after it as structure. Review
	// text almost never spells a real field name, so the stray key is the
	// signature of that mistake and the batch is refused rather than emitted
	// with a truncated value.
	serialized := `[{"content":"the flag is named "verbose", "and it defaults to true":"yes"}]`
	comments, repaired, errMsg := ParseCommentsWithPath(
		map[string]any{"comments": serialized}, "fallback.go")
	if errMsg == "" {
		t.Fatalf("repair was accepted (%d comments, %d escaped); want it declined "+
			"because prose was re-read as a field name", len(comments), repaired)
	}
	if !strings.Contains(errMsg, "invalid character") {
		t.Errorf("rejected repair must fall back to the parser wording, got %q", errMsg)
	}
}

func TestIsLegalEscape(t *testing.T) {
	// Getting this wrong in either direction is costly: too strict corrupts
	// genuine escapes, too loose leaves the batch unparseable.
	for _, tc := range []struct {
		in   string // the text after the backslash
		want bool
	}{
		{`"`, true}, {`\`, true}, {`/`, true},
		{"b", true}, {"f", true}, {"n", true}, {"r", true}, {"t", true},
		{"u0041", true}, {"uFFFF", true}, {"uabcd", true},
		{"u00", false},   // too few hex digits
		{"u00zz", false}, // not hex
		{"uD8", false},   // truncated
		{"d", false},     // regex \d
		{"U0041", false}, // uppercase U is not the escape
		{"x41", false},   // \x is not JSON
		{"", false},      // nothing follows the backslash
		{" ", false},     // space
	} {
		if got := isLegalEscape(tc.in, 0); got != tc.want {
			t.Errorf("isLegalEscape(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// Index past the end must not panic.
	if isLegalEscape("n", 5) {
		t.Error("out-of-range index must report false")
	}
}

func TestEscapeControl(t *testing.T) {
	for _, tc := range []struct {
		in   byte
		want string
	}{
		{'\n', `\n`}, {'\r', `\r`}, {'\t', `\t`}, {'\b', `\b`}, {'\f', `\f`},
		{0x00, `\u0000`}, {0x07, `\u0007`}, {0x0b, `\u000b`}, {0x1f, `\u001f`},
	} {
		if got := escapeControl(tc.in); got != tc.want {
			t.Errorf("escapeControl(%#x) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Every escape it produces has to be one the JSON parser accepts.
	for c := byte(0); c < 0x20; c++ {
		var target string
		blob := `["` + escapeControl(c) + `"]`
		if err := json.Unmarshal([]byte(blob), &[]*string{&target}); err != nil {
			t.Errorf("escapeControl(%#x) produced unparseable JSON %q: %v", c, blob, err)
		}
	}
}

func TestRepairSerializedComments_PreservesUnicodeEscape(t *testing.T) {
	// \u sequences are legal and must survive untouched, or a repair would
	// mangle content it was supposed to rescue.
	in := `[{"content":"snowman ☃ and a quote "here"","path":"a.go"}]`
	out, escaped := repairSerializedComments(in)
	if escaped != 2 {
		t.Fatalf("escaped = %d, want 2 (only the prose quotes)", escaped)
	}
	if !strings.Contains(out, `☃`) {
		t.Errorf("unicode escape was rewritten: %q", out)
	}
}

func TestRepairedCommentsAcceptable_RejectsUnknownField(t *testing.T) {
	original := `[{"content":"first"}]`
	withStray := []any{map[string]any{
		"content":        "first",
		"not a real key": "swallowed prose",
	}}
	if repairedCommentsAcceptable(withStray, original) {
		t.Error("an entry carrying a field the schema does not define must be rejected")
	}

	// Every documented field must stay acceptable, or the guard would reject
	// legitimate batches.
	full := []any{map[string]any{
		"content": "first", "existing_code": "x", "suggestion_code": "y",
		"category": "bug", "severity": "high", "path": "a.go", "thinking": "why",
	}}
	if !repairedCommentsAcceptable(full, original) {
		t.Error("a batch using only schema fields must be accepted")
	}
}

func TestParseComments_WellFormedInputReportsNoRepair(t *testing.T) {
	t.Run("native array", func(t *testing.T) {
		args := map[string]any{"comments": []any{
			map[string]any{"content": "issue", "path": "a.go"},
		}}
		comments, repaired, errMsg := ParseCommentsWithPath(args, "fallback.go")
		if errMsg != "" || len(comments) != 1 {
			t.Fatalf("comments=%d errMsg=%q", len(comments), errMsg)
		}
		if repaired != 0 {
			t.Errorf("repaired = %d, want 0 for a schema-conformant array", repaired)
		}
	})

	t.Run("correctly escaped string", func(t *testing.T) {
		// Still a schema violation, but it parses on its own — nothing to repair.
		serialized := `[{"content":"the name suggests \"ok\"","path":"a.go"}]`
		comments, repaired, errMsg := ParseCommentsWithPath(
			map[string]any{"comments": serialized}, "fallback.go")
		if errMsg != "" || len(comments) != 1 {
			t.Fatalf("comments=%d errMsg=%q", len(comments), errMsg)
		}
		if repaired != 0 {
			t.Errorf("repaired = %d, want 0 when the string already parses", repaired)
		}
		if comments[0].Content != `the name suggests "ok"` {
			t.Errorf("content = %q", comments[0].Content)
		}
	})
}

func TestRepairSerializedComments_LeavesWellFormedJSONByteIdentical(t *testing.T) {
	// Anything that already parses must come back untouched, so enabling the
	// repair cannot change the result of a batch that was fine.
	cases := []string{
		`[{"content":"plain","path":"a.go"}]`,
		`[{"content":"escaped \"term\" here","path":"a.go"}]`,
		`[{"content":"newline \n tab \t done","path":"a.go"}]`,
		`[ { "content" : "spaced out" } , { "content" : "second" } ]`,
		`[{"content":"trailing colon in prose: see","suggestion_code":"x := 1"}]`,
		`[{"content":"brace } and bracket ] inside prose","path":"a.go"}]`,
		`[]`,
	}
	for _, in := range cases {
		out, escaped := repairSerializedComments(in)
		if escaped != 0 {
			t.Errorf("escaped %d characters in already-valid input %q", escaped, in)
		}
		if out != in {
			t.Errorf("repair rewrote valid input\n  in:  %q\n  out: %q", in, out)
		}
	}
}

func TestRepairedCommentsAcceptable(t *testing.T) {
	original := `[{"content":"first"},{"content":"second"}]`

	t.Run("rejects a merged batch", func(t *testing.T) {
		// One entry recovered where the original described two: the repair
		// swallowed a terminator and fused them.
		merged := []any{map[string]any{"content": "first second"}}
		if repairedCommentsAcceptable(merged, original) {
			t.Error("a batch that lost a comment must be rejected")
		}
	})

	t.Run("accepts a complete batch", func(t *testing.T) {
		full := []any{
			map[string]any{"content": "first"},
			map[string]any{"content": "second"},
		}
		if !repairedCommentsAcceptable(full, original) {
			t.Error("a batch preserving both comments must be accepted")
		}
	})

	t.Run("rejects empty content", func(t *testing.T) {
		blank := []any{
			map[string]any{"content": "first"},
			map[string]any{"content": "   "},
		}
		if repairedCommentsAcceptable(blank, original) {
			t.Error("a blank content means the repair mangled a value")
		}
	})

	t.Run("rejects non-object entries", func(t *testing.T) {
		if repairedCommentsAcceptable([]any{"first", "second"}, original) {
			t.Error("string entries are not comments")
		}
	})

	t.Run("rejects an empty batch", func(t *testing.T) {
		if repairedCommentsAcceptable(nil, original) {
			t.Error("an empty batch preserves nothing")
		}
	})
}

func TestParseRepairedComments_DeclinesWhenNothingToEscape(t *testing.T) {
	// Valid JSON reaches this helper only when the caller already failed to
	// parse it, so "nothing to escape" means the failure is something else and
	// re-parsing identical text would be pointless.
	entries, escaped := parseRepairedComments(`[{"content":"fine"}]`)
	if entries != nil || escaped != 0 {
		t.Errorf("entries=%v escaped=%d; want a decline", entries, escaped)
	}
}
