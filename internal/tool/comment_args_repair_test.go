// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
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
