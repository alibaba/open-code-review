package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

const (
	backgroundSoftLimit    = 2000
	backgroundHardLimit    = 8000
	backgroundOpenTag      = "<ocr_user_background>"
	backgroundCloseTag     = "</ocr_user_background>"
	maxBackgroundFileBytes = 1 << 20 // 1 MB
)

var multiNewline = regexp.MustCompile(`\n{3,}`)

// mergeBackground combines the inline --background value (or an auto-populated
// commit message) with the content read from --background-file, separated by a
// blank line. The inline value is sanitised the same way as the file content so
// both portions are cleaned consistently. The file content is already wrapped
// and sanitised by loadBackgroundFile.
func mergeBackground(inline, fromFile string) string {
	inline = sanitizeMarkdown(inline)
	switch {
	case inline == "":
		return fromFile
	case fromFile == "":
		return inline
	default:
		return inline + "\n\n" + fromFile
	}
}

func loadBackgroundFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read background file %q: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("background file %q is a directory, not a file", path)
	}
	if info.Size() > maxBackgroundFileBytes {
		return "", fmt.Errorf(
			"background file %q is %d bytes, exceeding the maximum of %d bytes; please provide a smaller file",
			path, info.Size(), maxBackgroundFileBytes,
		)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read background file %q: %w", path, err)
	}

	cleaned := sanitizeMarkdown(string(raw))
	if cleaned == "" {
		return "", fmt.Errorf("background file %q is empty after sanitisation", path)
	}

	if strings.Contains(cleaned, backgroundOpenTag) || strings.Contains(cleaned, backgroundCloseTag) {
		return "", fmt.Errorf(
			"background file %q must not contain the reserved delimiters %q or %q",
			path, backgroundOpenTag, backgroundCloseTag,
		)
	}

	// Enforce the limits on the cleaned content only: the wrapper delimiters add
	// overhead the user cannot control, so counting them would make the reported
	// character count misleading.
	if n := len([]rune(cleaned)); n > backgroundHardLimit {
		return "", fmt.Errorf(
			"background content is %d characters, exceeding the hard limit of %d (aborting)",
			n, backgroundHardLimit,
		)
	} else if n > backgroundSoftLimit {
		fmt.Fprintf(os.Stderr,
			"[ocr] --background-file content is %d characters, exceeding the recommended %d (continuing but review quality might be impacted)\n",
			n, backgroundSoftLimit,
		)
	}

	return backgroundOpenTag + "\n" + cleaned + "\n" + backgroundCloseTag, nil
}

func sanitizeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\n', '\t':
			b.WriteRune(r)
			continue
		case '\r':
			continue
		}
		if isForbiddenChar(r) {
			continue
		}
		b.WriteRune(r)
	}

	collapsed := multiNewline.ReplaceAllString(b.String(), "\n\n")
	return strings.TrimSpace(collapsed)
}

func isForbiddenChar(r rune) bool {
	switch {
	case r <= 0x1F: // C0 control characters (includes NUL)
		return true
	case r >= 0x7F && r <= 0x9F: // DEL and C1 control characters
		return true
	}

	// The runes below all belong to Unicode category Cf and are therefore
	// already caught by the unicode.Is(unicode.Cf, r) check at the end. They are
	// listed explicitly only as documentation of the most common invisible
	// characters we strip; the switch is redundant, not a correctness necessity.
	switch r {
	case '\u200B', // zero-width space
		'\u200C', // zero-width non-joiner
		'\u200D', // zero-width joiner
		'\u200E', // left-to-right mark
		'\u200F', // right-to-left mark
		'\u2060', // word joiner
		'\u00AD', // soft hyphen
		'\uFEFF': // BOM / zero-width no-break space
		return true
	}

	return unicode.Is(unicode.Cf, r)
}
