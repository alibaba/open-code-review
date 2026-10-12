// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The code_comment schema declares `comments` as an array, but a model
// occasionally serializes it into a string, which then needs one more level of
// escaping than the model applied. The level it drops is almost always a double
// quote inside prose, so the batch fails to parse and every comment is lost.
//
// A bare quote inside a JSON string is either that string's terminator or content
// the model forgot to escape, and what follows tells the two apart.

// contentFieldPattern counts the comment objects the original text described. A
// `"content":` inside prose inflates the count and so makes the check stricter,
// which is the safe direction.
var contentFieldPattern = regexp.MustCompile(`"content"\s*:`)

// knownCommentFields holds every field the code_comment schema defines, plus
// thinking, which parseCommentsInner also copies through. Anything else in a
// repaired batch means the scan re-read prose as structure — see
// repairedCommentsAcceptable.
var knownCommentFields = map[string]struct{}{
	"content":         {},
	"existing_code":   {},
	"suggestion_code": {},
	"category":        {},
	"severity":        {},
	"path":            {},
	"thinking":        {},
}

// escapeControl returns the JSON escape for a control character. JSON forbids
// every byte below 0x20 in a string, so those without a short form get \u00XX.
func escapeControl(c byte) string {
	switch c {
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	}
	const hex = "0123456789abcdef"
	return `\u00` + string([]byte{hex[c>>4], hex[c&0x0f]})
}

// isLegalEscape reports whether s[i] opens a valid JSON escape sequence, given
// that s[i-1] is a backslash. \u additionally requires four hex digits.
func isLegalEscape(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	switch s[i] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return true
	case 'u':
		if i+5 > len(s) {
			return false
		}
		for _, c := range []byte(s[i+1 : i+5]) {
			if !isHexDigit(c) {
				return false
			}
		}
		return true
	}
	return false
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// repairSerializedComments escapes what makes a model-serialized `comments`
// string invalid JSON — prose quotes, bare control characters, illegal
// backslashes — and returns the repaired text plus the number of characters it
// escaped. A zero count means nothing needed escaping, so the caller must treat
// the original parse error as final rather than re-parse identical text.
//
// A real terminator is always followed by ',', '}', ']', ':' or the end of the
// text; a quote followed by anything else is content and gets escaped. So the
// scan never misses a genuine terminator, and its only possible error is ending a
// string early — which the acceptance checks are built to catch.
//
// Scanning byte by byte is safe for UTF-8: continuation bytes are all >= 0x80.
func repairSerializedComments(s string) (string, int) {
	var b strings.Builder
	b.Grow(len(s) + 16)

	escaped := 0
	inString := false
	for i := 0; i < len(s); {
		c := s[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			b.WriteByte(c)
			i++
			continue
		}

		switch {
		case c == '\\':
			if isLegalEscape(s, i+1) {
				b.WriteByte(c)
				i++
				b.WriteByte(s[i])
				i++
				break
			}
			// The dropped escaping level hits backslashes too: prose citing \d or
			// C:\Users leaves one that opens no valid sequence. A backslash before
			// b/f/n/r/t/u is legal and is left alone above, so `C:\bin` still
			// decodes to a backspace — JSON cannot tell those apart, and guessing
			// would corrupt genuine escapes.
			b.WriteString(`\\`)
			escaped++
			i++
		case c == '"':
			if next := nextSignificantByte(s, i+1); next < 0 || isJSONStructural(s[next]) {
				inString = false
				b.WriteByte(c)
			} else {
				b.WriteString(`\"`)
				escaped++
			}
			i++
		case c < 0x20:
			b.WriteString(escapeControl(c))
			escaped++
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), escaped
}

// nextSignificantByte returns the index of the first non-whitespace byte at or
// after i, or -1 when nothing but whitespace remains.
func nextSignificantByte(s string, i int) int {
	for ; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
		default:
			return i
		}
	}
	return -1
}

// isJSONStructural reports whether b can legally follow a string's closing
// quote in JSON.
func isJSONStructural(b byte) bool {
	switch b {
	case ',', '}', ']', ':':
		return true
	}
	return false
}

// repairedCommentsAcceptable reports whether a repaired batch still carries every
// comment the original text described. Parsing again is not enough on its own: a
// misjudged terminator can yield JSON that is valid but wrong.
//
// Every entry must keep a non-empty content, and the entry count must not fall
// below the number of `"content":` fields in the original — that rules out a
// repair that fused two comments by ending a string early. No entry may carry a
// field the schema does not define, since prose re-read as structure almost never
// spells a real field name.
//
// Passing here is necessary but not sufficient: hasSuspectTruncation runs after
// this and rejects cut values that these checks let through.
func repairedCommentsAcceptable(entries []any, original string) bool {
	if len(entries) == 0 {
		return false
	}
	for _, raw := range entries {
		obj, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		content, _ := obj["content"].(string)
		if strings.TrimSpace(content) == "" {
			return false
		}
		for field := range obj {
			if _, known := knownCommentFields[field]; !known {
				return false
			}
		}
	}
	return len(entries) >= len(contentFieldPattern.FindAllStringIndex(original, -1))
}

// commentTextFields are the string-valued fields checked for truncation.
//
// thinking is deliberately absent: a cut thinking reaches the JSON output only,
// with no terminal or viewer rendering, so suspecting it would forfeit a sound
// recovery over a diagnostic string.
var commentTextFields = []string{"content", "existing_code", "suggestion_code", "path"}

// hasOddQuotes reports whether v carries an odd number of double quotes — the
// signature of a value cut short at a misjudged terminator.
//
// Number a value's quotes q1..qN as the scan meets them: it escapes qi unless qi
// is followed by ',' '}' ']' or ':', where it reads qi as the terminator. A value
// cut at qi keeps i-1 quotes, so the cut is visible precisely when i is even.
// Prose puts it there — a term is quoted with a pair, and only a closing quote is
// followed by punctuation. Odd i slips through, but needs a closing quote with no
// opening quote before it in the same value, which prose does not produce.
//
// Code can legitimately carry an odd count, and the check is batch-wide: such a
// batch is declined even when the repair only escaped control characters and made
// no quote judgement at all. Nothing regresses — that is the outcome it had before
// this repair existed — but a lossless recovery is forgone.
func hasOddQuotes(v string) bool {
	return strings.Count(v, `"`)%2 == 1
}

// hasSuspectTruncation reports whether any entry carries a value a misjudged
// terminator may have cut short. Such a batch is refused outright rather than
// partially recovered: matchConsecutive compares whole lines, so a cut
// existing_code matches nothing, every deterministic resolver declines, and the
// LLM re-location then spends a call guessing line numbers from a mutilated
// excerpt. The original parse error instead makes the model resend the batch with
// its anchor, its suggestion_code and a deterministic position intact.
func hasSuspectTruncation(entries []any) bool {
	for _, raw := range entries {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range commentTextFields {
			if v, ok := obj[field].(string); ok && hasOddQuotes(v) {
				return true
			}
		}
	}
	return false
}

// CommentRepair records that the deterministic repair ran on a serialized
// `comments` argument; nil means it did not. Callers report it so a schema
// violation the framework papers over still leaves a trace. A count is all it
// needs, because an accepted repair is an intact one — see hasSuspectTruncation.
type CommentRepair struct {
	EscapedChars int
}

// Message renders the repair as a single warning line.
func (r *CommentRepair) Message() string {
	return fmt.Sprintf("comments arrived as a serialized string instead of an array; "+
		"repaired %d unescaped character(s)", r.EscapedChars)
}

// parseRepairedComments attempts the deterministic repair of a serialized
// `comments` string. It returns (nil, nil) when the text needed no repair, the
// repair still did not parse, or the result failed a check — in every one of those
// cases the caller must keep reporting the original parse error.
func parseRepairedComments(s string) ([]any, *CommentRepair) {
	repaired, escaped := repairSerializedComments(s)
	if escaped == 0 {
		return nil, nil
	}
	var entries []any
	if err := json.Unmarshal([]byte(repaired), &entries); err != nil {
		return nil, nil
	}
	if !repairedCommentsAcceptable(entries, s) {
		return nil, nil
	}
	if hasSuspectTruncation(entries) {
		return nil, nil
	}
	return entries, &CommentRepair{EscapedChars: escaped}
}
