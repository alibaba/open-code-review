// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/alibaba/open-code-review/internal/textenc"
)

// a commit that CHANGES the file's encoding
//
// A diff carries both sides of the change: "-" lines are OLD bytes, "+" and
// context lines are NEW bytes. When the commit itself moves the file between
// encodings the two sides are in different charsets, and no single charset
// serves both. The new-file bytes decide only the new side.

// docSimplifiedFirst is the first line of docSimplified, which every fixture
// below carries on BOTH sides of the change — in the old encoding on the "-"
// line and in the new one on the "+" line. Asserting on both prefixes is what
// separates "the payload decoded" from "the side the file agrees with decoded".
const docSimplifiedFirst = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002"

const (
	encChangeOldComment = "\t// \u6821\u9A8C\u901A\u8FC7"
	encChangeNewComment = "\t// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7"
)

// weakHead/weakTail wrap the ONE line that differs between the weak-evidence
// fixtures. Everything else is ASCII, so exactly one payload line is not valid
// UTF-8 and the detector sees nothing but that line.
const (
	weakHead = "package auth\n" +
		"\n" +
		"import \"strings\"\n" +
		"\n" +
		"func Validate(tok string) (string, error) {\n" +
		"\tif tok == \"\" {\n" +
		"\t\treturn \"\", errEmptyToken\n" +
		"\t}\n"
	weakTail   = "\treturn lookup(strings.TrimSpace(tok))\n}\n"
	weakLegacy = "\t// \u4EE4\u724C\u6821\u9A8C"
)

func TestDecodeEncodingChangedByTheCommit(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	for _, tc := range []struct {
		name string
		// build returns the committed bytes and the working-tree bytes.
		build func(*testing.T) (old, new []byte)
		// wantInDiff is decoded text, prefix byte included, that d.Diff must
		// carry. wantRawInDiff is source text whose LEGACY bytes must still be
		// there, for the cases we deliberately leave raw.
		wantInDiff    []string
		wantRawInDiff []string
		// wantNewFileContent is the new-file text the line resolver scans. It
		// is asserted on every row because the two sides of the decode are
		// decided separately: a file that is already UTF-8 must be left
		// exactly as it was read, and one that is not must arrive decoded.
		wantNewFileContent string
		wantMarked         bool
		// wantClean pins that the marshalled prompt payload carries no U+FFFD.
		wantClean bool
	}{
		{
			// The silent case: the new file is valid UTF-8, so a detector fed
			// the file alone answers UTF-8 and the legacy "-" lines reach
			// json.Marshal as U+FFFD with nothing marked.
			name: "gbk_to_utf8",
			build: func(t *testing.T) ([]byte, []byte) {
				return encodeFixture(t, simplifiedchinese.GBK, legacyFile(docSimplified, encChangeOldComment)),
					[]byte(legacyFile(docSimplified, encChangeNewComment))
			},
			wantInDiff: []string{
				"-" + docSimplifiedFirst, "+" + docSimplifiedFirst,
				"-" + encChangeOldComment, "+" + encChangeNewComment,
			},
			// The file is already UTF-8, so the bytes the parser read are the
			// bytes the resolver must scan.
			wantNewFileContent: legacyFile(docSimplified, encChangeNewComment),
			wantClean:          true,
		},
		{
			// The mirror: the new file is legacy, the "-" lines are already
			// UTF-8. Decoding them again is what makes the whole-payload
			// convert gain U+FFFD and throw the file away.
			name: "utf8_to_gbk",
			build: func(t *testing.T) ([]byte, []byte) {
				return []byte(legacyFile(docSimplified, encChangeOldComment)),
					encodeFixture(t, simplifiedchinese.GBK, legacyFile(docSimplified, encChangeNewComment))
			},
			wantInDiff: []string{
				"-" + docSimplifiedFirst, "+" + docSimplifiedFirst,
				"-" + encChangeOldComment, "+" + encChangeNewComment,
			},
			// The file is legacy, so the resolver gets the decoded text.
			wantNewFileContent: legacyFile(docSimplified, encChangeNewComment),
			wantClean:          true,
		},
		{
			// Weak evidence: one short legacy "-" line inside an otherwise
			// clean file scores 10, far below the confidence gate. The honest
			// answer is to mark it and keep reviewing, not to guess a charset
			// and not to stay silent.
			name: "weak_evidence_is_marked_but_still_reviewed",
			build: func(t *testing.T) ([]byte, []byte) {
				return encodeFixture(t, simplifiedchinese.GBK, weakHead+weakLegacy+"\n"+weakTail),
					[]byte(weakHead + "\t// token check\n" + weakTail)
			},
			wantInDiff:    []string{"+\t// token check"},
			wantRawInDiff: []string{weakLegacy},
			// Marked, but the new side was UTF-8 all along: nothing may
			// touch what the resolver scans.
			wantNewFileContent: weakHead + "\t// token check\n" + weakTail,
			wantMarked:         true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, new := tc.build(t)
			repo, diffText := realDiff(t, name, old, new)

			diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
			if err != nil {
				t.Fatalf("ParseDiffText: %v", err)
			}
			if len(diffs) != 1 {
				t.Fatalf("got %d diffs, want 1", len(diffs))
			}
			d := diffs[0]

			// The whole point of keeping a half-legacy file: it is still
			// reviewed. Excluding it would review less than HEAD does.
			if d.Unreviewable {
				t.Errorf("file was excluded from the review (charset %q)", d.UndecodedCharset)
			}
			if marked := d.UndecodedCharset != ""; marked != tc.wantMarked {
				t.Errorf("UndecodedCharset = %q, want marked=%v", d.UndecodedCharset, tc.wantMarked)
			}
			for _, want := range tc.wantInDiff {
				if !strings.Contains(d.Diff, want) {
					t.Errorf("d.Diff is missing %q:\n%s", want, d.Diff)
				}
			}
			if d.NewFileContent != tc.wantNewFileContent {
				t.Errorf("NewFileContent = %q, want %q", d.NewFileContent, tc.wantNewFileContent)
			}
			for _, want := range tc.wantRawInDiff {
				raw := string(encodeFixture(t, simplifiedchinese.GBK, want))
				if !strings.Contains(d.Diff, raw) {
					t.Errorf("d.Diff no longer carries the raw bytes of %q", want)
				}
			}
			if tc.wantClean {
				// A Go string holding raw legacy bytes carries no U+FFFD rune,
				// so the check has to run where the model actually reads it.
				payload, err := json.Marshal(d.Diff)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				if bytes.ContainsRune(payload, utf8.RuneError) || bytes.Contains(payload, []byte(`\ufffd`)) {
					t.Errorf("the marshalled prompt payload carries U+FFFD:\n%s", payload)
				}
			}
		})
	}
}

// The regression guard for the case that already worked: when both sides are
// GBK, deciding per line must produce exactly what deciding per payload
// produced. The control is the identical repo committed in UTF-8, which the
// parser leaves byte for byte alone — so d.Diff matching it pins the decoded
// bytes, the frame and the line count in one comparison.
func TestDecodeSameEncodingBothSidesUnchanged(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	oldText := legacyFile(docSimplified, encChangeOldComment)
	newText := legacyFile(docSimplified, encChangeNewComment)

	repo, diffText := realDiff(t, name,
		encodeFixture(t, simplifiedchinese.GBK, oldText),
		encodeFixture(t, simplifiedchinese.GBK, newText))
	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	got := diffs[0]

	ctlRepo, ctlText := realDiff(t, name, []byte(oldText), []byte(newText))
	ctlDiffs, err := ParseDiffText(context.Background(), ctlText, ctlRepo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText (control): %v", err)
	}
	ctl := ctlDiffs[0]

	if got.Diff != ctl.Diff {
		t.Errorf("decoded diff is not byte-identical to the UTF-8 control:\n got %q\nwant %q", got.Diff, ctl.Diff)
	}
	if got.NewFileContent != newText {
		t.Errorf("NewFileContent = %q, want %q", got.NewFileContent, newText)
	}
	if got.UndecodedCharset != "" || got.Unreviewable {
		t.Errorf("a decodable GBK file was marked: %q/%v", got.UndecodedCharset, got.Unreviewable)
	}
}

// The preconditions the three rows above rest on. Without these a row could
// pass for the wrong reason — a fixture that quietly detects on its own, or a
// weak line that quietly clears the gate.
func TestEncodingChangeFixturePreconditions(t *testing.T) {
	t.Run("the_weak_line_alone_does_not_reach_the_confidence_gate", func(t *testing.T) {
		raw := encodeFixture(t, simplifiedchinese.GBK, weakLegacy)
		cs, conf, ok := textenc.Detect(raw)
		if ok {
			t.Fatalf("the weak line detects as (%q, %d), so it cannot pin the below-gate path", cs, conf)
		}
	})

	t.Run("utf8_lines_decoded_as_gb18030_gain_replacement_characters", func(t *testing.T) {
		// This is why a single charset applied to the whole payload throws the
		// utf8_to_gbk file away: the "-" lines are already UTF-8.
		if _, ok := textenc.Convert("GB-18030", []byte(docSimplified)); ok {
			t.Fatal("the U+FFFD-gain guard no longer fires on UTF-8 CJK read as GB-18030")
		}
	})
}

// One payload line that cannot be converted holds the whole file back: the
// file is marked and every OTHER line keeps its raw bytes. A partial decode
// would put two encodings inside one diff, with no marker saying so — the one
// shape the model has no way to read and nothing downstream can detect.
//
// The file itself is clean GBK, so detection succeeds; only a deleted line,
// which lives in the commit and not in the file, carries the broken byte.
func TestDecodeOneUnconvertibleLineLeavesTheWholeFileRaw(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	newRaw := encodeFixture(t, simplifiedchinese.GBK, legacyFile(docSimplified, encChangeNewComment))
	// 0xFF is a dangling GB-18030 lead byte: not a valid lead, so a line ending
	// in it gains U+FFFD when decoded and textenc.Convert refuses it. It cannot
	// come out of the GBK encoder, which only emits valid sequences, so it is
	// appended as a raw byte.
	brokenLine := append([]byte("\t// "), 0xFF)
	oldRaw := append(
		encodeFixture(t, simplifiedchinese.GBK, legacyFile(docSimplified, encChangeOldComment)),
		append(brokenLine, '\n')...)

	// Both halves of the shape this test needs. Without them the case could
	// pass for the wrong reason: a file that never detects, or a line that
	// converts cleanly after all.
	if cs, _, ok := textenc.Detect(newRaw); !ok || cs != "GB-18030" {
		t.Fatalf("the new file detects as (%q, ok=%v), so the convert loop is never reached", cs, ok)
	}
	if _, ok := textenc.Convert("GB-18030", brokenLine); ok {
		t.Fatal("the broken line converts cleanly, so no line fails and the case is vacuous")
	}

	repo, diffText := realDiff(t, name, oldRaw, newRaw)
	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	d := diffs[0]

	if d.UndecodedCharset != "GB-18030" {
		t.Errorf("UndecodedCharset = %q, want the file marked as GB-18030", d.UndecodedCharset)
	}
	// The two comment lines sit before the broken one in payload order, so
	// they are exactly the lines a partial decode would have converted.
	for _, want := range []string{encChangeOldComment, encChangeNewComment} {
		if !strings.Contains(d.Diff, string(encodeFixture(t, simplifiedchinese.GBK, want))) {
			t.Errorf("d.Diff no longer carries the raw bytes of %q", want)
		}
		if strings.Contains(d.Diff, want) {
			t.Errorf("%q was committed decoded beside a line that failed to convert", want)
		}
	}
}

// A deleted file has no new-file bytes, so fileIsUTF8 is false however clean
// its payload is. When that payload is all UTF-8 there is still nothing to do,
// and the run must cost nothing: no detection, no info line, no rewrite. The
// "a UTF-8 repo pays nothing" property has to hold on this path too, and it is
// the only fast-path case where the file bytes cannot carry it.
func TestDecodeDeletedUTF8FileTakesTheFastPath(t *testing.T) {
	out := quietStdout(t)
	calls := countDetections(t)

	raw := strings.Join([]string{
		"diff --git a/auth/token.go b/auth/token.go",
		"deleted file mode 100644",
		"--- a/auth/token.go",
		"+++ /dev/null",
		"@@ -1,4 +0,0 @@",
		"-package auth",
		"-",
		"-// Validate reports whether tok is still usable.",
		"-func Validate(tok string) error { return nil }",
	}, "\n")

	diffs, err := ParseDiffText(context.Background(), raw, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	d := diffs[0]
	if !d.IsDeleted || d.NewPath != "/dev/null" {
		t.Fatalf("not the deleted-file case: deleted=%v newPath=%q", d.IsDeleted, d.NewPath)
	}
	if *calls != 0 {
		t.Errorf("detector invoked %d times for an all-UTF-8 file, want 0", *calls)
	}
	if out.Len() != 0 {
		t.Errorf("a file that needed no decoding reported one: %s", out)
	}
	if d.UndecodedCharset != "" || d.Unreviewable {
		t.Errorf("an all-UTF-8 file was marked: %q/%v", d.UndecodedCharset, d.Unreviewable)
	}
}

// The info line has to name the file the reader would go and open. A rename is
// the only case where the two paths differ, so it is the only case that can
// name the wrong one; a deletion is why the branch exists at all, since there
// the new path is "/dev/null" and the old path is the only real name.
func TestDecodeInfoLineNamesTheFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		// setup returns the repo dir and the raw diff text to parse.
		setup func(*testing.T) (string, string)
		want  string
	}{
		{
			name: "rename_reports_the_new_path",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeRepoFile(t, dir, "auth/new.go", encodeFixture(t, simplifiedchinese.GBK, gbkFileNew))
				control := strings.Join([]string{
					"diff --git a/auth/old.go b/auth/new.go",
					"similarity index 90%",
					"rename from auth/old.go",
					"rename to auth/new.go",
					"--- a/auth/old.go",
					"+++ b/auth/new.go",
					"@@ -1,2 +1,2 @@",
					"-// \u65E7\u7684\u6CE8\u91CA\u3002",
					"+// \u65B0\u7684\u6CE8\u91CA\u3002",
				}, "\n")
				return dir, string(encodeFixture(t, simplifiedchinese.GBK, control))
			},
			want: "auth/new.go",
		},
		{
			name: "deletion_reports_the_old_path",
			setup: func(t *testing.T) (string, string) {
				control := strings.Join([]string{
					"diff --git a/auth/token.go b/auth/token.go",
					"deleted file mode 100644",
					"--- a/auth/token.go",
					"+++ /dev/null",
					"@@ -1,4 +0,0 @@",
					"-" + docSimplifiedFirst,
					"-// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002",
					"-// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002",
					"-package auth",
				}, "\n")
				return t.TempDir(), string(encodeFixture(t, simplifiedchinese.GBK, control))
			},
			want: "auth/token.go",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := quietStdout(t)
			dir, raw := tc.setup(t)

			diffs, err := ParseDiffText(context.Background(), raw, dir, "", nil)
			if err != nil {
				t.Fatalf("ParseDiffText: %v", err)
			}
			if diffs[0].UndecodedCharset != "" {
				t.Fatalf("the fixture did not decode (%q), so no info line is emitted",
					diffs[0].UndecodedCharset)
			}
			if want := "[ocr] decoded " + tc.want + " from "; !strings.Contains(out.String(), want) {
				t.Errorf("info line does not name %s:\n%s", tc.want, out)
			}
		})
	}
}

// utf8SafeLegacyLine is a GBK comment whose ENCODED bytes are ALSO valid
// UTF-8: every one of its characters encodes to a lead byte in 0xC2-0xDF and a
// trail byte in 0x80-0xBF, which is exactly the shape of a two-byte UTF-8
// sequence. 1920 of the CJK characters GBK can encode have that property, so
// this is an ordinary legacy line and not a curiosity.
const utf8SafeLegacyLine = "\t// \u72B6\u6001\u7EDF\u4E00"

// A "+" line is new-side bytes, and when the new file is legacy the line is
// legacy too — whatever its own bytes happen to look like. Deciding from the
// bytes carries this line through raw, and it then reaches the model as
// well-formed UTF-8 spelling something else entirely, with nothing marked and
// no U+FFFD for any downstream check to catch.
//
// The same hazard is irreducible for "-" lines: they live in the commit, not
// in the file, so with no base ref their own bytes are the only evidence there
// is. The ceiling is documented on decodeDiffFile.
func TestDecodeAddedLegacyLineThatIsAlsoValidUTF8(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	oldText := legacyFile(docSimplified, encChangeOldComment)
	newText := legacyFile(docSimplified, utf8SafeLegacyLine)
	lineRaw := encodeFixture(t, simplifiedchinese.GBK, utf8SafeLegacyLine)

	// The preconditions. Without the first, a bytes-only rule decodes the line
	// anyway and the test passes on any build; without the second, the raw and
	// the decoded assertions below would be the same string.
	if !utf8.Valid(lineRaw) {
		t.Fatalf("precondition: %q encodes to % X, which is not valid UTF-8", utf8SafeLegacyLine, lineRaw)
	}
	if string(lineRaw) == utf8SafeLegacyLine {
		t.Fatal("precondition: the encoded bytes must differ from the text they encode")
	}

	repo, diffText := realDiff(t, name,
		encodeFixture(t, simplifiedchinese.GBK, oldText),
		encodeFixture(t, simplifiedchinese.GBK, newText))
	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	d := diffs[0]

	if d.UndecodedCharset != "" || d.Unreviewable {
		t.Fatalf("a decodable GBK file was marked: %q/%v", d.UndecodedCharset, d.Unreviewable)
	}
	if !strings.Contains(d.Diff, "+"+utf8SafeLegacyLine) {
		t.Errorf("the added line was not decoded:\n%q", d.Diff)
	}
	if strings.Contains(d.Diff, string(lineRaw)) {
		t.Errorf("d.Diff still carries the legacy bytes of the added line, which read as UTF-8 as %q:\n%q",
			string(lineRaw), d.Diff)
	}
	if d.NewFileContent != newText {
		t.Errorf("NewFileContent = %q, want %q", d.NewFileContent, newText)
	}
}
