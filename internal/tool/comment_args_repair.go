// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"encoding/json"
	"regexp"
	"strings"
)

// The code_comment schema declares `comments` as an array, but a model
// occasionally serializes that array into a string instead. The nested string
// then needs one more level of escaping than the model applied, and the level
// it drops is almost always a double quote inside prose — Chinese review text
// quotes a term with "..." where the JSON string needs \"...\". The batch then
// fails to parse and every comment in it is lost.
//
// The damage is mechanical and so is the fix: a bare quote inside a JSON string
// is either that string's terminator or content the model forgot to escape, and
// the two are told apart by what follows. Repairing it here keeps the findings
// instead of discarding a whole batch over a punctuation mark.

// contentFieldPattern counts the comment objects the original text described.
// It matches the field name only in key position, so a `"content":` appearing
// inside prose inflates the count and makes the repair check stricter — which
// errs toward rejecting a repair, never toward accepting a lossy one.
var contentFieldPattern = regexp.MustCompile(`"content"\s*:`)

// knownCommentFields is the set of field names the code_comment schema defines.
// A repaired batch carrying anything else means the scan re-read a stretch of
// prose as structure — see repairedCommentsAcceptable.
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
// every byte below 0x20 inside a string, so the ones without a short form get
// the \u00XX escape rather than being left to fail the parse.
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
// string invalid JSON — prose double quotes and bare control characters — and
// returns the repaired text with the number of characters it escaped.
//
// A bare '"' inside a JSON string is either the end of that string or content
// that should have been escaped. A real terminator is always followed by ',',
// '}', ']', ':' or the end of the text; anything else means the quote belongs
// to the content. In `the name suggests "a trusted proxy exists" but ...` the
// first quote is followed by a letter and the second by another word, so both
// fail that test and both get escaped. The reported failures were all Chinese
// review prose, where quoting a term this way is the norm rather than the
// exception — which is why one test fixture keeps that language.
//
// Scanning byte by byte is safe for UTF-8: continuation bytes are all >= 0x80
// and so can never be mistaken for the ASCII characters this looks at.
//
// A zero count means the text was already well-formed in these two respects,
// and the caller should treat the original parse error as final rather than
// re-parsing identical text.
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
				// A real escape pair; copy it verbatim.
				b.WriteByte(c)
				i++
				b.WriteByte(s[i])
				i++
				break
			}
			// The dropped escaping level hits backslashes too, not just quotes:
			// prose citing a regex (\d), a Windows path (C:\Users) or a literal
			// \n leaves a backslash that opens no valid sequence. Escaping it
			// recovers a batch that would otherwise be lost to the same root
			// cause as the quotes.
			//
			// A backslash followed by b/f/n/r/t/u is a legal escape and is left
			// alone above, so `C:\bin` still decodes to a backspace rather than
			// the path the prose meant. JSON gives no way to tell those apart,
			// and inventing one would corrupt genuine escapes — which are far
			// more common in this field than Windows paths.
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

// repairedCommentsAcceptable reports whether a repaired batch still carries
// every comment the original text described.
//
// Parsing again is not enough on its own: a misjudged terminator can yield JSON
// that is valid but wrong. Three things have to hold.
//
// Every entry keeps a non-empty content, and the entry count does not fall below
// the number of `"content":` fields in the original text — that rules out a
// repair that merged two comments by ending a string early, which would
// otherwise halve a batch and still look successful.
//
// No entry carries a field the schema does not define. When the scan ends a
// string too early, the prose after it is re-read as structure, and a stretch of
// review text almost never spells a real field name — so an unknown key is the
// signature of exactly that mistake.
//
// A window remains, and narrowing it further is not possible from local
// evidence: `"a word", "existing_code":"x"` inside prose is byte-for-byte
// indistinguishable from a genuine string terminator followed by the next
// field, so a repair can still truncate a value when the prose happens to spell
// a real field name right after a quoted term. The checks above shrink that to
// prose citing this schema's own field names in that exact shape; anything else
// either fails to parse or trips the unknown-field check. Callers get the
// original parse error in those cases, which is the safe direction.
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

// parseRepairedComments attempts the deterministic repair of a serialized
// `comments` string and returns the recovered entries plus the number of
// characters escaped. It returns (nil, 0) when the text needed no repair, the
// repair still did not parse, or the result failed the preservation check — in
// every one of those cases the caller must keep reporting the original parse
// error.
func parseRepairedComments(s string) ([]any, int) {
	repaired, escaped := repairSerializedComments(s)
	if escaped == 0 {
		return nil, 0
	}
	var entries []any
	if err := json.Unmarshal([]byte(repaired), &entries); err != nil {
		return nil, 0
	}
	if !repairedCommentsAcceptable(entries, s) {
		return nil, 0
	}
	return entries, escaped
}
