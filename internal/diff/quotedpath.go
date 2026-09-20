// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import "strings"

// Git writes a pathname in C-style quotes whenever it holds a byte it cannot
// print literally on one line: a control character such as a tab or a newline,
// a double quote, or a backslash. core.quotepath=false removes non-ASCII bytes
// from that set and nothing else, so these three survive it.
//
// The two sides of a header are quoted independently, so all four combinations
// occur:
//
//	diff --git a/plain.go b/plain.go
//	diff --git "a/tab\tname.go" "b/tab\tname.go"
//	diff --git a/plain.go "b/tab\tname.go"
//	diff --git "a/tab\tname.go" b/plain.go
//
// "rename from" and "rename to" are quoted by the same rule.

// unquoteGitPath decodes one C-style quoted pathname. s must begin with the
// opening quote. It returns the decoded path, the remainder of the line after
// the closing quote, and whether the token was well formed.
//
// Decoding is byte-oriented: a multi-byte character arrives as a run of octal
// escapes, one per byte, and only reassembles into a rune once those bytes sit
// next to each other again.
func unquoteGitPath(s string) (path string, rest string, ok bool) {
	if len(s) == 0 || s[0] != '"' {
		return "", s, false
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			return b.String(), s[i+1:], true
		case c != '\\':
			b.WriteByte(c)
		default:
			i++
			if i >= len(s) {
				return "", s, false
			}
			switch e := s[i]; e {
			case 'a':
				b.WriteByte('\a')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'v':
				b.WriteByte('\v')
			case '"', '\\':
				b.WriteByte(e)
			case '0', '1', '2', '3', '4', '5', '6', '7':
				// Exactly three octal digits, which is what git emits.
				if i+2 >= len(s) {
					return "", s, false
				}
				v := 0
				for d := 0; d < 3; d++ {
					digit := s[i+d]
					if digit < '0' || digit > '7' {
						return "", s, false
					}
					v = v*8 + int(digit-'0')
				}
				if v > 0xff {
					// \400 and above do not fit in a byte. Git never emits
					// them, and truncating to the low bits would hand back a
					// path that is quietly not the one on disk.
					return "", s, false
				}
				i += 2
				b.WriteByte(byte(v))
			default:
				// Not an escape git produces. Refusing here is better than
				// inventing a pathname: the caller falls back and the header
				// is left alone rather than silently mis-decoded.
				return "", s, false
			}
		}
	}
	return "", s, false
}

// splitHeaderPaths splits the text after "diff --git " into its two pathnames,
// each still carrying its "a/" or "b/" prefix.
//
// A bare pathname can hold neither a double quote nor a space-quote sequence,
// because either would have forced git to quote the whole name. So the first
// quote in the text unambiguously opens the second path whenever the first one
// is bare, and no ambiguity is introduced that the caller has to guess about.
func splitHeaderPaths(rest string) (string, string, bool) {
	if strings.HasPrefix(rest, `"`) {
		first, after, ok := unquoteGitPath(rest)
		if !ok {
			return "", "", false
		}
		after, ok = strings.CutPrefix(after, " ")
		if !ok {
			return "", "", false
		}
		if strings.HasPrefix(after, `"`) {
			second, tail, ok := unquoteGitPath(after)
			if !ok || tail != "" {
				return "", "", false
			}
			return first, second, true
		}
		return first, after, true
	}

	q := strings.Index(rest, `"`)
	if q <= 0 || rest[q-1] != ' ' {
		// No quoted side; the caller keeps using the plain regex, which also
		// preserves its long-standing handling of unquoted spaces.
		return "", "", false
	}
	second, tail, ok := unquoteGitPath(rest[q:])
	if !ok || tail != "" {
		return "", "", false
	}
	return rest[:q-1], second, true
}

// parseQuotedDiffHeader extracts the old and new paths from a "diff --git"
// line in which at least one side is quoted, with the "a/" and "b/" prefixes
// removed. It reports false for a fully unquoted header, which the plain
// regex still handles.
func parseQuotedDiffHeader(line string) (oldPath string, newPath string, ok bool) {
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return "", "", false
	}
	first, second, ok := splitHeaderPaths(rest)
	if !ok {
		return "", "", false
	}
	oldPath, okOld := strings.CutPrefix(first, "a/")
	newPath, okNew := strings.CutPrefix(second, "b/")
	if !okOld || !okNew {
		return "", "", false
	}
	return oldPath, newPath, true
}

// unquoteRenamePath decodes a "rename from"/"rename to" operand, which git
// quotes by the same rule and without any a/ or b/ prefix. A path git left
// bare is returned unchanged.
func unquoteRenamePath(s string) string {
	if !strings.HasPrefix(s, `"`) {
		return s
	}
	path, rest, ok := unquoteGitPath(s)
	if !ok || rest != "" {
		return s
	}
	return path
}
