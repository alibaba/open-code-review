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

// controlEscapes maps the bare control characters that are illegal inside a
// JSON string to their escaped form.
var controlEscapes = map[byte]string{
	'\n': `\n`,
	'\r': `\r`,
	'\t': `\t`,
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
			// Copy an existing escape pair verbatim. A trailing lone backslash
			// is left alone for the parser to reject.
			b.WriteByte(c)
			i++
			if i < len(s) {
				b.WriteByte(s[i])
				i++
			}
		case c == '"':
			if next := nextSignificantByte(s, i+1); next < 0 || isJSONStructural(s[next]) {
				inString = false
				b.WriteByte(c)
			} else {
				b.WriteString(`\"`)
				escaped++
			}
			i++
		case controlEscapes[c] != "":
			b.WriteString(controlEscapes[c])
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
// that is valid but wrong, most plausibly by merging two comments into one. So
// each entry must keep a non-empty content, and the entry count must not fall
// below the number of `"content":` fields in the original text. The count is
// what rules out a silent merge; without it a repair could halve a batch and
// still look successful.
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
