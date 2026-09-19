// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package textenc detects the character encoding of source files and decodes
// legacy CJK encodings to UTF-8 in memory.
//
// Everything downstream of a review treats file bytes as UTF-8 — json.Marshal
// of the prompt above all — so a GBK/Big5/Shift-JIS file reaches the model as
// U+FFFD soup and its line numbers no longer resolve. This package turns those
// bytes into real text before they leave the seam that read them.
//
// Nothing here writes to disk: ocr stays a read-only reviewer.
package textenc

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// Stable charset labels returned by Detect. Legacy charsets keep chardet's own
// spelling so Convert can look them up without a second naming scheme.
const (
	UTF8    = "UTF-8"
	UTF16LE = "UTF-16LE"
	UTF16BE = "UTF-16BE"
	Binary  = "binary"
	Unknown = "unknown"
)

// minConfidence is the chardet score below which a detection is not trusted.
// Legacy decoders never error and cross-decode each other into plausible but
// wrong CJK (GBK/Big5/Shift-JIS all accept each other's bytes), so the score
// and the U+FFFD-gain check in Convert are the only usable failure signals.
const minConfidence = 90

// detectBest is a package var only so tests can count invocations: "the
// detector is never called for valid UTF-8" and "exactly one call per file
// inside ParseDiffText" are not observable any other way.
//
// It is a plain var rather than an interface because one swap point is all
// the tests need. Tests that swap it must not call t.Parallel() — make test
// runs -race.
var detectBest = deterministicBest

// Shared deliberately. chardet.Detector is read-only after construction, its
// recognizers hold no per-call state, and DetectAll builds a fresh
// recognizerInput each time, so concurrent Detect is safe. A detector per call
// would share the same package-level recognizer slice anyway.
var detector = chardet.NewTextDetector()

// deterministicBest is DetectAll plus a tie-break on charset name.
//
// chardet's own DetectBest cannot be used here. DetectAll fans its recognizers
// out across goroutines, collects them through a channel, and finishes with
// sort.Sort, which is not stable — so results that tie on confidence come back
// in goroutine completion order and DetectBest returns whichever won the race.
//
// Measured on the 800 deterministic garbage bytes in the scan preview test,
// where Shift_JIS, GB-18030 and Big5 all score 10: 200 calls in ONE process
// returned Shift_JIS 179 times, GB-18030 15 and Big5 6. The visible effect was
// `ocr scan --preview` reporting a different detected_charset for the same file
// between runs, and a test that pinned the label failing about one run in ten.
//
// The tie-break is the charset name because it is total and needs no table.
// Which name wins is arbitrary; that the same bytes always pick the same one is
// not. Ties at or above minConfidence are not known to occur — every correct
// detection measured scored 100 with the runner-up at 50 or below — so this
// decides only which label gets REPORTED for input that is skipped anyway.
func deterministicBest(raw []byte) (*chardet.Result, error) {
	all, err := detector.DetectAll(raw)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, chardet.NotDetectedError
	}
	best := all[0]
	for _, c := range all[1:] {
		if c.Confidence > best.Confidence ||
			(c.Confidence == best.Confidence && c.Charset < best.Charset) {
			best = c
		}
	}
	return &best, nil
}

// decoders is the explicit allowlist of charsets we will decode.
//
// It is deliberately not ianaindex: ianaindex.IANA.Encoding("GB-18030") fails
// with "invalid encoding name", and "GB-18030" is the exact label chardet emits
// for Simplified Chinese — the single most important encoding for this feature.
// An ianaindex-based mapping would work for Japanese and be inert for Chinese.
//
// Single-byte charsets (ISO-8859-1, windows-1252) are excluded on purpose.
// Their mojibake is mild, chardet's single-byte scores are weak, and under
// Unreviewable such files are marked and still reviewed — today's behaviour.
// If a Latin-1 issue is ever filed, add the entry together with a higher
// confidence gate for single-byte hits.
var decoders = map[string]encoding.Encoding{
	"GB-18030":  simplifiedchinese.GB18030, // chardet's spelling, for GBK and GB18030 alike
	"GB18030":   simplifiedchinese.GB18030,
	"Big5":      traditionalchinese.Big5,
	"Shift_JIS": japanese.ShiftJIS,
	"EUC-JP":    japanese.EUCJP,
	"EUC-KR":    korean.EUCKR,
}

// Detect classifies raw. ok=false means "no supported charset: leave the bytes
// alone"; charset is still returned for reporting (Binary, Unknown, or the
// detector's own label).
//
// There is no detection window. utf8.Valid short-circuits first, so the
// detector cost is paid per legacy file rather than per file, and truncating to
// a head window is measurably harmful — a file whose first non-ASCII line sits
// past the window collapses from confidence 100 to 55 and turns into a false
// skip. If a repo of huge legacy files ever makes this hurt, the fix is a
// window that keeps reading until it has seen N non-ASCII bytes.
func Detect(raw []byte) (charset string, confidence int, ok bool) {
	// Empty and valid-UTF-8 input take a byte-identical fast path and never
	// reach the detector. This also covers UTF-8-with-BOM: the BOM is left in
	// place, because stripping it would change today's output for files that
	// are already fine.
	if len(raw) == 0 || utf8.Valid(raw) {
		return UTF8, 100, true
	}

	// UTF-16 BOM sniff. Reached only for non-UTF-8 bytes, and both BOMs are
	// invalid UTF-8, so the fast path above can never swallow a BOM-ed file.
	switch {
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return UTF16LE, 100, true
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return UTF16BE, 100, true
	}

	// NUL guard. Deliberately after the UTF-8 fast path, so a valid-UTF-8 file
	// containing a NUL still passes through unchanged, and after the BOM sniff,
	// because UTF-16 text is full of NULs and hoisting this guard would make
	// UTF-16 undecodable.
	if bytes.IndexByte(raw, 0) >= 0 {
		return Binary, 0, false
	}

	res, err := detectBest(raw)
	if err != nil || res == nil {
		return Unknown, 0, false
	}
	if res.Confidence < minConfidence {
		return res.Charset, res.Confidence, false
	}
	if _, supported := decoders[res.Charset]; !supported {
		return res.Charset, res.Confidence, false
	}
	return res.Charset, res.Confidence, true
}

// Convert decodes raw using a charset label previously returned by Detect.
// ok=false means the decode gained U+FFFD (or was not possible at all) and the
// result must not be used; text is then byte-identical to raw.
func Convert(charset string, raw []byte) (text string, ok bool) {
	if charset == UTF8 {
		return string(raw), true
	}

	var enc encoding.Encoding
	switch charset {
	case UTF16LE:
		enc = unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM)
	case UTF16BE:
		enc = unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM)
	default:
		enc = decoders[charset]
	}
	if enc == nil {
		return string(raw), false
	}

	// No allowlisted decoder is known to return an error: they substitute
	// U+FFFD instead, which is what the gain check below is for. The check
	// stays because transform.Bytes is allowed to error by contract, and a
	// dependency bump that starts exercising that would otherwise hand back a
	// partial decode as if it were the whole file. It is unreachable today, so
	// no test covers it.
	out, _, err := transform.Bytes(enc.NewDecoder(), raw)
	if err != nil {
		return string(raw), false
	}

	// The only decode-failure signal that exists. x/text legacy decoders never
	// return an error — they substitute U+FFFD — so a branch keyed on err alone
	// is dead. Gaining a replacement character means we picked the wrong
	// charset (or the input is truncated, which is how odd-length UTF-16 is
	// caught).
	decoded := string(out)
	if countReplacement(decoded) > baselineReplacement(raw) {
		return string(raw), false
	}
	return decoded, true
}

// maxReplacementRatio is the fraction of a file's runes that may turn into
// U+FFFD, when its raw bytes are treated as UTF-8, before the file is judged
// past reviewing and excluded from the run.
//
// Measured on both sides of the boundary: legitimate source that HEAD reviews
// correctly tops out at 11.04% (a comment-only Latin-1 French .go file); a
// single stray smart quote is 0.57%, a Latin-1 French source file 2.61%.
// Genuinely unreviewable input starts at 37.41% (an undetected Big5 file) and
// runs to 76% (random high bytes). 0.20 sits in that gap: 1.8x above the worst
// legitimate file and 1.9x below the best garbage one.
//
// It is one constant with no config knob: a dependency bump that moves either
// cluster across it fails the density table test rather than the field.
const maxReplacementRatio = 0.20

// Unreviewable reports whether raw is past reviewing as-is: binary, or so much
// of it would turn into U+FFFD that a review of the raw bytes is worthless.
// Only meaningful after Detect returned ok=false.
//
// The measure is the density of the raw bytes, not of any decode attempt: raw
// bytes are what ship today, and json.Marshal is the point where each invalid
// byte becomes a literal U+FFFD in the prompt.
func Unreviewable(charset string, raw []byte) bool {
	if charset == Binary {
		return true
	}
	bad, total := 0, 0
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		if r == utf8.RuneError && size == 1 {
			bad++
		}
		total++
		i += size
	}
	return total > 0 && float64(bad) > maxReplacementRatio*float64(total)
}

// DecodeSource is the single-stream helper: Detect, then Convert, then at most
// one info line. On skip it returns string(raw) — today's behaviour — with
// ok=false and no warning. The caller owns the warning, because only the caller
// knows whether the file survives the reviewability and extension filters; a
// warning printed here would fire for every .png and vendored file that was
// going to be dropped anyway.
func DecodeSource(path string, raw []byte) (text string, charset string, ok bool) {
	charset, confidence, ok := Detect(raw)
	if !ok {
		return string(raw), charset, false
	}
	if charset == UTF8 {
		return string(raw), UTF8, true
	}
	text, ok = Convert(charset, raw)
	if !ok {
		return string(raw), charset, false
	}
	Info(path, charset, confidence)
	return text, charset, true
}

// Info emits the one-line "this file was decoded" notice on stdout.Writer().
// Because it lands on stdout, a caller that emits a machine-readable document
// there must silence stdout around the whole load or enumeration phase, not
// only around the final emit: the notice is printed while diffs are being
// parsed or files enumerated, long before anything is marshalled. See
// newQuietHandle in cmd/opencodereview, which review --preview and scan
// --preview install for exactly this reason — without it the JSON document is
// preceded by human-readable lines and no longer parses.
func Info(path, charset string, confidence int) {
	fmt.Fprintf(stdout.Writer(), "[ocr] decoded %s from %s (confidence %d)\n", path, charset, confidence)
}

// countReplacement counts U+FFFD runes present as such in s.
func countReplacement(s string) int {
	return strings.Count(s, string(utf8.RuneError))
}

// baselineReplacement is how many U+FFFD the decode is allowed to hand back
// without that counting as a failure: the ones raw already carried, so a source
// that genuinely contains replacement characters is not read as a bad decode.
//
// It answers 0 for anything that is not already UTF-8. In legacy bytes EF BF BD
// is not a replacement character, it is three ordinary bytes the source encoding
// gives its own meaning, and counting them raises the bar the decode has to
// clear. Measured on a Big5 file with one undecodable byte: it is correctly
// rejected on its own, and adding a single EF BF BD to it lifts the baseline to
// 1 so the very same broken decode is accepted with the U+FFFD still in it.
func baselineReplacement(raw []byte) int {
	if !utf8.Valid(raw) {
		return 0
	}
	return countReplacement(string(raw))
}

// CountDetectionsForTest installs a counting wrapper around the charset
// detector and returns the counter plus a restore function.
//
// It exists because two of this feature's load-bearing properties are not
// observable any other way: "the detector never runs on valid UTF-8" (so a
// normal repo pays nothing) and "exactly one detection per file inside
// ParseDiffText" (so the hunk the model reads and the file content the
// resolver scans can never disagree). Both are asserted from other packages —
// internal/diff, internal/scan — which is why this is exported.
//
// Not safe for concurrent use: callers must not call t.Parallel().
func CountDetectionsForTest() (calls *int, restore func()) {
	n := 0
	orig := detectBest
	detectBest = func(b []byte) (*chardet.Result, error) {
		n++
		return orig(b)
	}
	return &n, func() { detectBest = orig }
}
