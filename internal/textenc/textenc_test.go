// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package textenc

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// EncodeFixture encodes s with enc, failing the test if the encoder rejects a
// rune. Exported for the seam tests in internal/diff, internal/scan and
// internal/tool, which need the identical legacy bytes plus a UTF-8 control.
func EncodeFixture(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, _, err := transform.Bytes(enc.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return out
}

func encodeTo(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	return EncodeFixture(t, enc, s)
}

// countDetector swaps the package detector for one that counts invocations.
// Tests using it must not call t.Parallel(): make test runs -race.
func countDetector(t *testing.T) *int {
	t.Helper()
	calls, restore := CountDetectionsForTest()
	t.Cleanup(restore)
	return calls
}

// densityPct is the fraction of runes in raw that would render as U+FFFD when
// the bytes are treated as UTF-8 — the same measure Unreviewable applies.
func densityPct(raw []byte) float64 {
	bad, total := 0, 0
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		if r == utf8.RuneError && size == 1 {
			bad++
		}
		total++
		i += size
	}
	if total == 0 {
		return 0
	}
	return 100 * float64(bad) / float64(total)
}

// randomBytes returns n deterministic pseudo-random bytes in [lo, hi].
func randomBytes(n int, lo, hi byte) []byte {
	r := rand.New(rand.NewSource(1987))
	out := make([]byte, n)
	span := int(hi) - int(lo) + 1
	for i := range out {
		out[i] = byte(lo + byte(r.Intn(span)))
	}
	return out
}

// A: the UTF-8 fast path — the detector must never run

func TestDetectFastPath(t *testing.T) {
	oneMB := strings.Repeat("package auth // ok\n", 55000)

	cases := []struct {
		id  string
		raw []byte
	}{
		{"A1_empty", nil},
		{"A2_newline_only", []byte("\n")},
		{"A3_pure_ascii", []byte(srcSmartQuoteBase)},
		// srcGbk as UTF-8: its bytes are also a plausible GBK sequence, so a
		// detector-first ladder would mangle it.
		{"A4_utf8_cjk_also_valid_gbk", []byte(srcGbk)},
		{"A5_one_megabyte_utf8", []byte(oneMB)},
		{"A6_valid_utf8_with_nul", []byte("package auth\x00// still text\n")},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			calls := countDetector(t)
			charset, conf, ok := Detect(tc.raw)
			if !ok || charset != UTF8 || conf != 100 {
				t.Fatalf("Detect = (%q, %d, %v), want (UTF-8, 100, true)", charset, conf, ok)
			}
			if *calls != 0 {
				t.Errorf("detector called %d times, want 0", *calls)
			}
			got, ok := Convert(charset, tc.raw)
			if !ok || got != string(tc.raw) {
				t.Errorf("Convert changed the bytes (ok=%v, len %d -> %d)", ok, len(tc.raw), len(got))
			}
		})
	}
}

// B: BOM handling and guard ordering

func TestDetectBOM(t *testing.T) {
	const control = "// \u4EE4\u724C\u6821\u9A8C\npackage auth\n"
	utf16le := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM)
	utf16be := unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM)

	t.Run("B1_utf8_bom_passthrough", func(t *testing.T) {
		raw := append([]byte{0xEF, 0xBB, 0xBF}, control...)
		charset, _, ok := Detect(raw)
		if !ok || charset != UTF8 {
			t.Fatalf("Detect = (%q, %v), want (UTF-8, true)", charset, ok)
		}
		got, ok := Convert(charset, raw)
		if !ok || got != string(raw) {
			t.Fatalf("UTF-8 BOM must pass through unstripped")
		}
		if !strings.HasPrefix(got, "\xEF\xBB\xBF") {
			t.Errorf("BOM was stripped")
		}
	})

	t.Run("B2_utf16le_bom", func(t *testing.T) {
		raw := encodeTo(t, utf16le, control)
		charset, conf, ok := Detect(raw)
		if !ok || charset != UTF16LE || conf != 100 {
			t.Fatalf("Detect = (%q, %d, %v), want (UTF-16LE, 100, true)", charset, conf, ok)
		}
		got, ok := Convert(charset, raw)
		if !ok || got != control {
			t.Fatalf("Convert = %q (ok=%v), want the UTF-8 control", got, ok)
		}
	})

	t.Run("B3_utf16be_bom", func(t *testing.T) {
		raw := encodeTo(t, utf16be, control)
		charset, _, ok := Detect(raw)
		if !ok || charset != UTF16BE {
			t.Fatalf("Detect = (%q, %v), want (UTF-16BE, true)", charset, ok)
		}
		got, ok := Convert(charset, raw)
		if !ok || got != control {
			t.Fatalf("Convert = %q (ok=%v), want the UTF-8 control", got, ok)
		}
	})

	t.Run("B4_bom_only_no_content", func(t *testing.T) {
		raw := []byte{0xFF, 0xFE}
		charset, _, ok := Detect(raw)
		if !ok || charset != UTF16LE {
			t.Fatalf("Detect = (%q, %v), want (UTF-16LE, true)", charset, ok)
		}
		got, ok := Convert(charset, raw)
		if !ok || got != "" {
			t.Fatalf("Convert = %q (ok=%v), want empty string", got, ok)
		}
	})

	// B6 pins the ladder order: UTF-16 text is full of NUL bytes, so moving the
	// NUL guard ahead of the BOM branch makes every UTF-16 file undecodable.
	t.Run("B6_nul_guard_runs_after_bom_branch", func(t *testing.T) {
		raw := encodeTo(t, utf16le, control)
		if bytes.IndexByte(raw, 0) < 0 {
			t.Fatal("precondition: UTF-16LE fixture must contain NUL bytes")
		}
		charset, _, ok := Detect(raw)
		if !ok || charset == Binary {
			t.Fatalf("Detect = (%q, %v); the NUL guard was hoisted above the BOM sniff", charset, ok)
		}
	})

	t.Run("B7_truncated_utf16_reverts", func(t *testing.T) {
		raw := encodeTo(t, utf16le, control)
		raw = raw[:len(raw)-1] // odd length: the last unit is cut in half
		charset, _, ok := Detect(raw)
		if !ok || charset != UTF16LE {
			t.Fatalf("Detect = (%q, %v), want (UTF-16LE, true)", charset, ok)
		}
		got, ok := Convert(charset, raw)
		if ok {
			t.Fatalf("Convert accepted a truncated UTF-16 stream")
		}
		if got != string(raw) {
			t.Errorf("reverted output is not byte-identical to the input")
		}
	})
}

// C: the six allowlisted legacy encodings

// legacyFixtures is the shared C1-C7 table: each entry round-trips exactly and
// scores at or above minConfidence.
var legacyFixtures = []struct {
	id      string
	enc     encoding.Encoding
	charset string
	text    string
}{
	{"C1_gbk", simplifiedchinese.GBK, "GB-18030", srcGbk},
	{"C2_gb18030_four_byte", simplifiedchinese.GB18030, "GB-18030", srcGbk + srcGb18030WideTail},
	{"C3_big5", traditionalchinese.Big5, "Big5", srcBig5},
	{"C4_shift_jis", japanese.ShiftJIS, "Shift_JIS", srcSjis},
	{"C5_euc_jp", japanese.EUCJP, "EUC-JP", srcEucjp},
	{"C6_euc_kr", korean.EUCKR, "EUC-KR", srcEuckr},
}

func TestConvertLegacy(t *testing.T) {
	for _, tc := range legacyFixtures {
		t.Run(tc.id, func(t *testing.T) {
			raw := encodeTo(t, tc.enc, tc.text)
			charset, conf, ok := Detect(raw)
			if !ok {
				t.Fatalf("Detect refused the fixture: charset %q confidence %d", charset, conf)
			}
			if charset != tc.charset {
				t.Fatalf("charset = %q, want %q", charset, tc.charset)
			}
			if conf < minConfidence {
				t.Fatalf("confidence = %d, want >= %d", conf, minConfidence)
			}
			got, ok := Convert(charset, raw)
			if !ok {
				t.Fatalf("Convert refused a correctly detected fixture")
			}
			if got != tc.text {
				t.Errorf("round-trip mismatch:\n got %q\nwant %q", got, tc.text)
			}
		})
	}

	// C7 states the round-trip property once over the whole table, so a future
	// fixture cannot be added without it.
	t.Run("C7_round_trip_property", func(t *testing.T) {
		for _, tc := range legacyFixtures {
			raw := encodeTo(t, tc.enc, tc.text)
			if got, ok := Convert(tc.charset, raw); !ok || got != tc.text {
				t.Errorf("%s: Convert(charset, encode(s)) != s (ok=%v)", tc.id, ok)
			}
		}
	})

	// C8 pins the allowlist. A dependency bump that renames a charset — or an
	// "improvement" that swaps the map for ianaindex, which rejects the literal
	// string "GB-18030" — fails here rather than silently in the field.
	t.Run("C8_allowlist_key_set", func(t *testing.T) {
		want := map[string]bool{
			"GB-18030": true, "GB18030": true, "Big5": true,
			"Shift_JIS": true, "EUC-JP": true, "EUC-KR": true,
		}
		if len(decoders) != len(want) {
			t.Fatalf("decoders has %d keys, want %d", len(decoders), len(want))
		}
		for k, v := range decoders {
			if !want[k] {
				t.Errorf("unexpected charset %q in the allowlist", k)
			}
			if v == nil {
				t.Errorf("charset %q maps to a nil encoding", k)
			}
		}
	})

	t.Run("C9_unlisted_charset_is_inert", func(t *testing.T) {
		raw := encodeTo(t, charmap.Windows1252, srcFrenchSource)
		got, ok := Convert("windows-1252", raw)
		if ok {
			t.Fatalf("Convert accepted a charset outside the allowlist")
		}
		if got != string(raw) {
			t.Errorf("output is not byte-identical to the input")
		}
	})
}

// D: skips, marking, and the exclusion boundary

// A U+FFFD the source already carried must not raise the bar the decode has to
// clear. EF BF BD is a replacement character only in UTF-8; in Big5 it is three
// ordinary bytes, so counting it as one lets a genuinely broken decode through.
func TestConvertBaselineDoesNotMaskABrokenDecode(t *testing.T) {
	body, _, err := transform.Bytes(traditionalchinese.Big5.NewEncoder(),
		[]byte(strings.Repeat("\u7CFB\u7D71\u6E2C\u8A66\u8CC7\u6599\uFF0C\u9019\u662F\u4E00\u500B\u6A94\u6848\u3002\n", 30)))
	if err != nil {
		t.Fatalf("encoding the Big5 fixture failed: %v", err)
	}
	// 0x80 is not a valid Big5 lead byte, so the decode must fail on it.
	const undecodable = 0x80
	poison := []byte{0xEF, 0xBF, 0xBD}

	tests := []struct {
		name    string
		prefix  []byte
		wantOK  bool
		wantWhy string
	}{
		{
			name:    "one_bad_byte_is_rejected",
			prefix:  nil,
			wantOK:  false,
			wantWhy: "an undecodable byte gains a U+FFFD over a baseline of 0",
		},
		{
			name:    "one_bad_byte_is_still_rejected_behind_one_utf8_replacement",
			prefix:  poison,
			wantOK:  false,
			wantWhy: "the EF BF BD is Big5 data, not a replacement character, so it must not lift the baseline",
		},
		{
			name:    "one_bad_byte_is_still_rejected_behind_two",
			prefix:  append(append([]byte{}, poison...), poison...),
			wantOK:  false,
			wantWhy: "more padding must not buy more headroom either",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := append(append(append([]byte{}, body...), tt.prefix...), undecodable)
			charset, confidence, ok := Detect(raw)
			if !ok || charset != "Big5" {
				t.Fatalf("Detect = %q (confidence %d, ok %v), want a trusted Big5 so Convert is reached",
					charset, confidence, ok)
			}
			text, gotOK := Convert(charset, raw)
			if gotOK != tt.wantOK {
				t.Errorf("Convert ok = %v, want %v: %s", gotOK, tt.wantOK, tt.wantWhy)
			}
			if gotOK && countReplacement(text) > 0 {
				t.Errorf("Convert reported success while handing back %d U+FFFD; %s",
					countReplacement(text), tt.wantWhy)
			}
			if !gotOK && text != string(raw) {
				t.Error("a failed Convert must hand back the raw bytes unchanged")
			}
		})
	}
}

func TestDetectSkip(t *testing.T) {
	mixed := append(encodeTo(t, simplifiedchinese.GBK, srcGbk),
		encodeTo(t, traditionalchinese.Big5, srcBig5)...)

	cases := []struct {
		id  string
		raw []byte
	}{
		// Small evidence: a hunk-sized GBK snippet legitimately scores under
		// the gate. Pins that skipping is a real outcome, not a bug.
		{"D1_short_gbk_snippet", encodeTo(t, simplifiedchinese.GBK, srcShortGBK)},
		// Single-byte charsets are outside the allowlist whatever the score.
		{"D2_iso8859_1_accents", encodeTo(t, charmap.ISO8859_1, srcFrenchComment)},
		{"D3_high_byte_binary_no_nul", randomBytes(600, 0x80, 0xFF)},
		{"D6_repetitive_big5", encodeTo(t, traditionalchinese.Big5,
			strings.Repeat(srcRepeatBig5Phrase, 12))},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			charset, conf, ok := Detect(tc.raw)
			if ok {
				t.Fatalf("Detect accepted %s: charset %q confidence %d", tc.id, charset, conf)
			}
			// A rejected charset is never handed to Convert; the wrapper is
			// what callers use, and it must leave the bytes alone.
			got, _, dok := DecodeSource("fixture.go", tc.raw)
			if dok {
				t.Errorf("DecodeSource decoded a fixture Detect rejected")
			}
			if got != string(tc.raw) {
				t.Errorf("a skipped fixture must be left byte-identical")
			}
		})
	}

	// D4: two charsets in one file. Either the detector refuses it, or the
	// decode gains U+FFFD and Convert reverts. Both outcomes leave the bytes
	// alone; what must never happen is a silent half-correct decode.
	t.Run("D4_mixed_gbk_and_big5", func(t *testing.T) {
		charset, _, ok := Detect(mixed)
		if !ok {
			return
		}
		got, cok := Convert(charset, mixed)
		if cok && strings.ContainsRune(got, utf8.RuneError) {
			t.Fatalf("mixed-charset file decoded with a U+FFFD gain")
		}
	})

	// D5: the measured false positive of the U+FFFD-gain guard. A GB18030 file
	// that legitimately encodes U+FFFD is refused. Accepted as a safe skip.
	// Must use the GB18030 encoder — the GBK encoder rejects U+FFFD outright.
	t.Run("D5_gb18030_containing_replacement_char", func(t *testing.T) {
		text := srcGbk + "// \uFFFD marker\n"
		raw := encodeTo(t, simplifiedchinese.GB18030, text)
		charset, _, ok := Detect(raw)
		if !ok {
			t.Skipf("detector refused the fixture (charset %q); the guard is not reached", charset)
		}
		if _, cok := Convert(charset, raw); cok {
			t.Fatalf("expected the gain guard to refuse a file that legitimately holds U+FFFD")
		}
	})
}

// densityRows is the mark-versus-exclude table. Every row is a file the
// detector REFUSES — the only files that ever reach Unreviewable. Each row
// records the measured U+FFFD density of its raw bytes and which side of
// maxReplacementRatio it must land on: `reviewed` files are ones HEAD reviews
// correctly today and must keep that review; the rest are past reviewing.
//
// Files the detector accepts are deliberately absent: they are decoded, never
// marked, so the threshold does not apply to them. Their densities are high
// (a GBK source file measures ~30%) precisely because decoding is what saves
// them, and folding them into this table would imply the opposite.
func densityRows(t *testing.T) []struct {
	id       string
	raw      []byte
	wantPct  float64
	reviewed bool
} {
	t.Helper()
	smartQuote := []byte(strings.Replace(srcSmartQuoteBase, "caller's", "caller\x92s", 1))
	truncated := []byte(strings.Replace(srcGbk, "\u4EE4\u724C", "\u4EE4\xE7\x89", 1))
	asciiPlusGBK := append([]byte(srcSmartQuoteBase),
		encodeTo(t, simplifiedchinese.GBK, "// \u4EE4\u724C\u6821\u9A8C\n")...)
	magicPlusEntropy := append([]byte("GIF89a\x01\x00"), randomBytes(400, 0x80, 0xFF)...)
	pngNoNul := bytes.ReplaceAll(
		append([]byte("\x89PNG\r\n\x1a\n"), randomBytes(500, 0x01, 0xFF)...),
		[]byte{0}, []byte{0x41})
	// Every other byte of a GBK file overwritten: high-byte soup that no
	// frequency model recognises, standing in for a corrupted source file.
	shredded := encodeTo(t, simplifiedchinese.GBK, srcGbk+srcGbk)
	for i := 1; i < len(shredded); i += 2 {
		shredded[i] = byte(0x80 + (i*37)%0x7F)
	}

	return []struct {
		id       string
		raw      []byte
		wantPct  float64
		reviewed bool
	}{
		// the legitimate cluster: marked, still reviewed
		{"D11a_utf8_one_smart_quote", smartQuote, 0.53, true},
		{"D11b_latin1_french_source", encodeTo(t, charmap.ISO8859_1, srcFrenchSource), 6.97, true},
		{"D11c_utf8_truncated_sequence", truncated, 0.90, true},
		{"D11d_ascii_plus_small_gbk_comment", asciiPlusGBK, 2.65, true},
		{"D11e_windows1252_curly_quotes", encodeTo(t, charmap.Windows1252,
			strings.Replace(srcSmartQuoteBase, "caller's", "caller\u2019s", 1)), 0.53, true},
		{"D11f_latin1_french_doc_comment", encodeTo(t, charmap.ISO8859_1, srcFrenchComment), 10.97, true},
		{"D11g_latin1_german_umlauts", encodeTo(t, charmap.ISO8859_1,
			"// Pr\u00FCft, ob der \u00FCbergebene Schl\u00FCssel g\u00FCltig ist.\n"+
				"// Andernfalls muss der Aufrufer die Sitzung zur\u00FCcksetzen.\n"), 4.59, true},
		{"D11h_comment_only_latin1_french", encodeTo(t, charmap.ISO8859_1,
			strings.Repeat(srcFrenchComment, 2)), 10.97, true},
		{"D11i_short_gbk_snippet", encodeTo(t, simplifiedchinese.GBK, srcShortGBK), 8.82, true},

		// the unreviewable cluster: marked and excluded
		{"D11j_shredded_gbk_source", shredded, 60.80, false},
		{"D11k_random_bytes_01_to_ff", randomBytes(600, 0x01, 0xFF), 47.49, false},
		{"D11l_magic_bytes_plus_entropy", magicPlusEntropy, 74.92, false},
		{"D11m_random_bytes_80_to_ff", randomBytes(600, 0x80, 0xFF), 79.79, false},
		{"D11n_png_body_nuls_stripped", pngNoNul, 45.88, false},
	}
}

func TestUnreviewableBoundary(t *testing.T) {
	// D11: every row's density is pinned, every row is genuinely refused by the
	// detector, and every row lands on its recorded side of the threshold. A
	// dependency bump that moves either cluster across 0.20 fails here.
	for _, row := range densityRows(t) {
		t.Run(row.id, func(t *testing.T) {
			got := densityPct(row.raw)
			if diff := got - row.wantPct; diff > 0.5 || diff < -0.5 {
				t.Errorf("density = %.2f%%, want %.2f%% (+/-0.5pp)", got, row.wantPct)
			}
			charset, _, ok := Detect(row.raw)
			if ok {
				t.Fatalf("precondition: this table holds only files the detector refuses, "+
					"but %s was accepted as %q", row.id, charset)
			}
			if unreviewable := Unreviewable(charset, row.raw); unreviewable == row.reviewed {
				t.Errorf("Unreviewable = %v at density %.2f%% (threshold %.0f%%), want %v",
					unreviewable, got, maxReplacementRatio*100, !row.reviewed)
			}
		})
	}

	// D8: the "mark but keep reviewing" side. Every one of these files is
	// reviewed at HEAD, so a design that excludes every non-UTF-8 file — which
	// is the regression this threshold exists to prevent — fails right here.
	t.Run("D8_imperfect_files_stay_reviewable", func(t *testing.T) {
		n := 0
		for _, row := range densityRows(t) {
			if !row.reviewed {
				continue
			}
			n++
			charset, _, _ := Detect(row.raw)
			if Unreviewable(charset, row.raw) {
				t.Errorf("%s: excluded at %.2f%%, but HEAD reviews it", row.id, densityPct(row.raw))
			}
			if charset == "" {
				t.Errorf("%s: no charset label to mark the model with", row.id)
			}
		}
		if n == 0 {
			t.Fatal("the reviewed cluster is empty; this case would pass vacuously")
		}
	})

	// D10: the "genuinely past reviewing" side.
	t.Run("D10_garbage_is_unreviewable", func(t *testing.T) {
		n := 0
		for _, row := range densityRows(t) {
			if row.reviewed {
				continue
			}
			n++
			charset, _, _ := Detect(row.raw)
			if !Unreviewable(charset, row.raw) {
				t.Errorf("%s: still reviewable at density %.2f%%", row.id, densityPct(row.raw))
			}
		}
		if n == 0 {
			t.Fatal("the garbage cluster is empty; this case would pass vacuously")
		}
	})

	// The two clusters must stay separated by a real gap, not merely sorted
	// around the constant. Without this, a threshold moved to 0.11 or 0.45
	// would still pass every row above.
	t.Run("D11_clusters_are_separated", func(t *testing.T) {
		worstLegit, bestGarbage := 0.0, 100.0
		for _, row := range densityRows(t) {
			d := densityPct(row.raw)
			if row.reviewed && d > worstLegit {
				worstLegit = d
			}
			if !row.reviewed && d < bestGarbage {
				bestGarbage = d
			}
		}
		if worstLegit >= maxReplacementRatio*100 || bestGarbage <= maxReplacementRatio*100 {
			t.Fatalf("threshold %.0f%% does not separate the clusters "+
				"(worst legitimate %.2f%%, best garbage %.2f%%)",
				maxReplacementRatio*100, worstLegit, bestGarbage)
		}
		if bestGarbage/worstLegit < 2 {
			t.Errorf("clusters are only %.1fx apart (%.2f%% vs %.2f%%); the threshold is a guess, not a gap",
				bestGarbage/worstLegit, worstLegit, bestGarbage)
		}
	})

	t.Run("D11_empty_input_is_reviewable", func(t *testing.T) {
		if Unreviewable(UTF8, nil) {
			t.Error("empty input must never be judged unreviewable")
		}
	})
}

// E: binary and NUL

func TestBinary(t *testing.T) {
	t.Run("E1_embedded_nul_not_valid_utf8", func(t *testing.T) {
		calls := countDetector(t)
		raw := append(encodeTo(t, simplifiedchinese.GBK, srcGbk), 0x00, 0x41)
		charset, conf, ok := Detect(raw)
		if ok || charset != Binary || conf != 0 {
			t.Fatalf("Detect = (%q, %d, %v), want (binary, 0, false)", charset, conf, ok)
		}
		if !Unreviewable(charset, raw) {
			t.Error("binary must always be unreviewable")
		}
		if *calls != 0 {
			t.Errorf("detector called %d times for binary, want 0", *calls)
		}
	})

	t.Run("E3_leading_nul_is_still_binary", func(t *testing.T) {
		// The NUL is at index 0 and is the only one in the file. A guard
		// written as "> 0" instead of ">= 0" misses exactly this file, and the
		// miss is not cosmetic: utf8.DecodeRune reports NUL as a valid rune,
		// so Unreviewable counts it as good text and the file goes on to be
		// reviewed as source.
		raw := append([]byte{0x00}, encodeTo(t, simplifiedchinese.GBK, srcGbk)...)
		charset, conf, ok := Detect(raw)
		if ok || charset != Binary || conf != 0 {
			t.Fatalf("Detect = (%q, %d, %v), want (binary, 0, false)", charset, conf, ok)
		}
		if !Unreviewable(charset, raw) {
			t.Error("binary must always be unreviewable")
		}
	})

	t.Run("E2_png_magic_bytes", func(t *testing.T) {
		raw := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), randomBytes(200, 0x00, 0xFF)...)
		if _, _, ok := Detect(raw); ok {
			t.Fatal("Detect accepted a PNG")
		}
	})
}

// DecodeSource: the single-stream wrapper and its one info line

func TestDecodeSource(t *testing.T) {
	t.Run("emits_one_info_line_on_decode", func(t *testing.T) {
		var buf bytes.Buffer
		defer stdout.Swap(&buf)()

		raw := encodeTo(t, simplifiedchinese.GBK, srcGbk)
		text, charset, ok := DecodeSource("internal/auth/token.go", raw)
		if !ok || text != srcGbk {
			t.Fatalf("DecodeSource did not decode the fixture (ok=%v, charset=%q)", ok, charset)
		}
		got := buf.String()
		if strings.Count(got, "\n") != 1 {
			t.Fatalf("want exactly one info line, got %q", got)
		}
		for _, want := range []string{"internal/auth/token.go", "GB-18030", "confidence 100"} {
			if !strings.Contains(got, want) {
				t.Errorf("info line %q does not mention %q", got, want)
			}
		}
	})

	t.Run("silent_on_the_utf8_fast_path", func(t *testing.T) {
		var buf bytes.Buffer
		defer stdout.Swap(&buf)()

		text, charset, ok := DecodeSource("a.go", []byte(srcSmartQuoteBase))
		if !ok || charset != UTF8 || text != srcSmartQuoteBase {
			t.Fatalf("DecodeSource = (%q, %v) on valid UTF-8", charset, ok)
		}
		if buf.Len() != 0 {
			t.Errorf("the fast path must print nothing, got %q", buf.String())
		}
	})

	t.Run("silent_on_skip_and_returns_raw", func(t *testing.T) {
		var buf bytes.Buffer
		defer stdout.Swap(&buf)()

		raw := randomBytes(600, 0x80, 0xFF)
		text, charset, ok := DecodeSource("blob.go", raw)
		if ok {
			t.Fatal("DecodeSource claimed to decode garbage")
		}
		if text != string(raw) {
			t.Error("a skip must return the raw bytes unchanged")
		}
		if charset == "" {
			t.Error("a skip must still report a charset label for marking")
		}
		// The caller owns the warning: only it knows whether the file survives
		// the reviewability and extension filters.
		if buf.Len() != 0 {
			t.Errorf("DecodeSource must not warn, got %q", buf.String())
		}
	})

	// The one path where detection succeeds and the DECODE does not: a
	// confident but WRONG charset. That is the failure mode the U+FFFD-gain
	// guard in Convert exists for — GBK, Big5 and Shift-JIS all accept each
	// other's bytes — and DecodeSource has to pass the refusal on. A caller
	// told ok=true here would feed mojibake to the model as if it were text.
	//
	// No natural fixture reaches it: a real Big5 file is detected as Big5, so
	// the wrong label can only arrive through the detector stub.
	t.Run("failed_decode_is_reported_as_a_skip", func(t *testing.T) {
		raw := encodeTo(t, traditionalchinese.Big5, srcBig5)

		orig := detectBest
		detectBest = func([]byte) (*chardet.Result, error) {
			return &chardet.Result{Charset: "GB-18030", Confidence: 100}, nil
		}
		t.Cleanup(func() { detectBest = orig })

		// Both preconditions, so a dependency bump that moves either one fails
		// here — naming the fixture — instead of looking like a DecodeSource
		// regression: detection must be trusted, and the decode must fail.
		if charset, conf, ok := Detect(raw); !ok || charset != "GB-18030" || conf < minConfidence {
			t.Fatalf("Detect = (%q, %d, %v), want a trusted GB-18030 hit", charset, conf, ok)
		}
		if _, ok := Convert("GB-18030", raw); ok {
			t.Fatalf("fixture no longer fails: Big5 bytes decode cleanly as GB-18030")
		}

		var buf bytes.Buffer
		defer stdout.Swap(&buf)()

		text, charset, ok := DecodeSource("auth/token.go", raw)
		if ok {
			t.Error("DecodeSource reported ok for a decode that failed")
		}
		if text != string(raw) {
			t.Error("a failed decode must return the raw bytes byte-identical")
		}
		if charset != "GB-18030" {
			t.Errorf("charset = %q, want the detected label so the caller can mark the file", charset)
		}
		if buf.Len() != 0 {
			t.Errorf("a file that was not decoded must not print the decode notice, got %q", buf.String())
		}
	})
}

// detector failure modes, driven through the swappable package var

func TestDetectDetectorFailures(t *testing.T) {
	raw := []byte("\x81\x40\x81\x41 not utf-8 \x82\x60\x82\x61")

	cases := []struct {
		id          string
		result      *chardet.Result
		err         error
		wantCharset string
	}{
		{"detector_error", nil, errStubDetector, Unknown},
		{"detector_nil_result", nil, nil, Unknown},
		{
			// A confident hit on a charset we deliberately do not decode.
			// Single-byte charsets land here: marked, still reviewed.
			id:          "confident_but_unlisted_charset",
			result:      &chardet.Result{Charset: "KOI8-R", Confidence: 99},
			wantCharset: "KOI8-R",
		},
		{
			// Right charset, not enough evidence to trust it.
			id:          "listed_charset_below_confidence_gate",
			result:      &chardet.Result{Charset: "Big5", Confidence: minConfidence - 1},
			wantCharset: "Big5",
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			orig := detectBest
			detectBest = func([]byte) (*chardet.Result, error) { return tc.result, tc.err }
			t.Cleanup(func() { detectBest = orig })

			charset, _, ok := Detect(raw)
			if ok {
				t.Fatalf("Detect accepted the stub result")
			}
			if charset != tc.wantCharset {
				t.Errorf("charset = %q, want %q", charset, tc.wantCharset)
			}
			// Whatever the label, the bytes must survive untouched.
			if got, _, dok := DecodeSource("f.go", raw); dok || got != string(raw) {
				t.Errorf("DecodeSource mangled a skipped file (ok=%v)", dok)
			}
		})
	}
}

// TestDetectConfidenceGateBoundary pins both sides of minConfidence. The gate
// is written "< minConfidence", so a score of exactly 90 is TRUSTED; written
// "<= minConfidence" it would be rejected, and a real GBK file that scores
// exactly at the gate would silently stop being decoded. No natural fixture
// lands on 90, so the boundary is only reachable through the stub.
func TestDetectConfidenceGateBoundary(t *testing.T) {
	raw := []byte("\x81\x40\x81\x41 not utf-8 \x82\x60\x82\x61")

	for _, tc := range []struct {
		id     string
		score  int
		wantOK bool
	}{
		{"exactly_at_the_gate_is_trusted", minConfidence, true},
		{"one_below_the_gate_is_not", minConfidence - 1, false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			orig := detectBest
			detectBest = func([]byte) (*chardet.Result, error) {
				return &chardet.Result{Charset: "Big5", Confidence: tc.score}, nil
			}
			t.Cleanup(func() { detectBest = orig })

			charset, conf, ok := Detect(raw)
			if ok != tc.wantOK {
				t.Errorf("Detect(score %d) ok = %v, want %v", tc.score, ok, tc.wantOK)
			}
			if charset != "Big5" || conf != tc.score {
				t.Errorf("Detect = (%q, %d), want (Big5, %d)", charset, conf, tc.score)
			}
		})
	}
}

var errStubDetector = errStub("detector unavailable")

type errStub string

func (e errStub) Error() string { return string(e) }

// TestDetectIsDeterministicOnTies pins the tie-break in deterministicBest.
//
// chardet's DetectAll races its recognizers across goroutines and finishes with
// an unstable sort, so equal-confidence results arrive in completion order and
// its DetectBest returns whichever won. These 800 bytes score 10 for Shift_JIS,
// GB-18030 and Big5 alike; before the tie-break, 200 calls in one process
// returned Shift_JIS 179 times, GB-18030 15 and Big5 6, and the scan preview
// test that pins detected_charset failed about one run in ten.
//
// Loops rather than asserting the label once: a single call passes just as well
// against the racy implementation. Only repetition can see the flap.
func TestDetectIsDeterministicOnTies(t *testing.T) {
	garbage := make([]byte, 800)
	for i := range garbage {
		garbage[i] = byte(0x80 + (i*37+11)%0x7F)
	}

	first, conf, ok := Detect(garbage)
	if ok {
		t.Fatalf("these bytes must stay undecodable; got charset %q at confidence %d", first, conf)
	}
	for i := 0; i < 200; i++ {
		got, _, _ := Detect(garbage)
		if got != first {
			t.Fatalf("call %d returned %q, first call returned %q: detection is not deterministic", i, got, first)
		}
	}
}
