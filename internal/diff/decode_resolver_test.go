// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/alibaba/open-code-review/internal/model"
)

// Group G: a comment quoting decoded CJK must resolve to the same line number
// as the identical UTF-8 control. Every case starts from RAW LEGACY BYTES on
// disk and runs the real chain ParseDiffText -> ResolveComment, and every case
// asserts a specific expected line number rather than merely "the two runs
// agree" — two runs that both resolve to 0 agree perfectly and prove nothing.
//
// There is exactly one line resolver in this repo. internal/llm/resolver.go
// resolves LLM endpoint configuration and contains no line matching; scan mode
// reaches this same resolver through model.ScanItem.AsDiff().

// resolverFile is the fixture: 9 lines, CJK comments interleaved with ASCII.
const resolverFile = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" + // 1
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" + // 2
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" + // 3
	"package auth\n" + // 4
	"\n" + // 5
	"func Validate(tok string) error {\n" + // 6
	"\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A\n" + // 7
	"\treturn nil\n" + // 8
	"}\n" // 9

// newFileDiff renders the diff git synthesizes for a new, untracked file: one
// hunk whose payload is the whole file, every line an addition.
func newFileDiff(path, content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	out := []string{
		"diff --git a/" + path + " b/" + path,
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/" + path,
		"@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@",
	}
	for _, l := range lines {
		out = append(out, "+"+l)
	}
	return strings.Join(out, "\n")
}

// parseFixture writes content to a temp dir in the given encoding and runs the
// real parser over the matching diff text. gbk=false produces the UTF-8 control
// through the identical call chain.
func parseFixture(t *testing.T, path, content string, gbk bool) model.Diff {
	t.Helper()
	dir := t.TempDir()
	diffText := newFileDiff(path, content)
	if gbk {
		writeRepoFile(t, dir, path, encodeFixture(t, simplifiedchinese.GBK, content))
		diffText = string(encodeFixture(t, simplifiedchinese.GBK, diffText))
	} else {
		writeRepoFile(t, dir, path, []byte(content))
	}
	diffs, err := ParseDiffText(context.Background(), diffText, dir, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	return diffs[0]
}

func TestResolveCommentsAgainstDecodedCJK(t *testing.T) {
	quietStdout(t)
	const path = "auth/token.go"

	// G2: a line that appears twice resolves to the first occurrence.
	t.Run("G2_repeated_line_resolves_to_the_first_occurrence", func(t *testing.T) {
		dup := resolverFile + "\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A\n"
		d := parseFixture(t, path, dup, true)
		cm := &model.LlmComment{Path: path, ExistingCode: "\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A"}
		if !ResolveComment(cm, &d) {
			t.Fatal("comment did not resolve at all")
		}
		if cm.StartLine != 7 {
			t.Errorf("StartLine = %d, want the first occurrence at 7", cm.StartLine)
		}
	})

	// G3: a multi-line CJK excerpt resolves start and end.
	t.Run("G3_multiline_cjk_resolves_start_and_end", func(t *testing.T) {
		d := parseFixture(t, path, resolverFile, true)
		cm := &model.LlmComment{Path: path, ExistingCode: strings.Join([]string{
			"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002",
			"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002",
		}, "\n")}
		if !ResolveComment(cm, &d) {
			t.Fatal("comment did not resolve at all")
		}
		if cm.StartLine != 2 || cm.EndLine != 3 {
			t.Errorf("resolved to %d-%d, want 2-3", cm.StartLine, cm.EndLine)
		}
	})

	// G4 is the case that fails without the decode: the GBK twin and the UTF-8
	// control hold byte-identical CONTENT and must land on the same non-zero
	// line. At HEAD the GBK run resolves to 0, because the resolver compares a
	// UTF-8 excerpt against mojibake.
	t.Run("G4_gbk_and_utf8_control_agree_on_a_nonzero_line", func(t *testing.T) {
		const want = 7
		excerpt := "\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A"

		control := parseFixture(t, path, resolverFile, false)
		ctlComment := &model.LlmComment{Path: path, ExistingCode: excerpt}
		if !ResolveComment(ctlComment, &control) || ctlComment.StartLine != want {
			t.Fatalf("the UTF-8 control resolved to %d, want %d", ctlComment.StartLine, want)
		}

		gbk := parseFixture(t, path, resolverFile, true)
		gbkComment := &model.LlmComment{Path: path, ExistingCode: excerpt}
		if !ResolveComment(gbkComment, &gbk) {
			t.Fatal("the GBK twin did not resolve; this is the #987 failure")
		}
		if gbkComment.StartLine != want {
			t.Errorf("the GBK twin resolved to %d, want %d", gbkComment.StartLine, want)
		}
	})

	// G5: a line padded with the fullwidth space U+3000. Normalisation is
	// TrimSpace-based, so a fullwidth pad is part of the text, not whitespace —
	// it must survive the decode intact for the match to land.
	t.Run("G5_fullwidth_space_padding_matches", func(t *testing.T) {
		padded := strings.Replace(resolverFile,
			"\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A", "\t//\u3000\u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A", 1)
		d := parseFixture(t, path, padded, true)
		cm := &model.LlmComment{Path: path, ExistingCode: "\t//\u3000\u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A"}
		if !ResolveComment(cm, &d) {
			t.Fatal("comment did not resolve at all")
		}
		if cm.StartLine != 7 {
			t.Errorf("StartLine = %d, want 7", cm.StartLine)
		}
	})

	// ResolveLineNumbers is the batch entry point the review loop actually
	// calls; it must reach the same answer.
	t.Run("G1_batch_entry_point_agrees", func(t *testing.T) {
		d := parseFixture(t, path, resolverFile, true)
		got := ResolveLineNumbers([]model.LlmComment{
			{Path: path, ExistingCode: "\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A"},
		}, []model.Diff{d})
		if len(got) != 1 || got[0].StartLine != 7 {
			t.Errorf("ResolveLineNumbers gave %+v, want StartLine 7", got)
		}
	})
}

// H1 and G6: the two entry points into the ONE line resolver — a model.Diff
// straight out of ParseDiffText, and a model.ScanItem through AsDiff() — must
// land on the same line for the same decoded file.
//
// Both sides here are fed by the review path's decode: the ScanItem is built
// from d.NewFileContent, not from scan's own enumerate seam. That is
// deliberate, and it is the limit of what this test proves — it pins the
// resolver and the AsDiff() shim, not that the two modes decode alike.
// internal/scan owns the genuine cross-mode case, where each mode decodes the
// same file through its own seam.
func TestResolverAgreesAcrossDiffAndScanItemEntryPoints(t *testing.T) {
	quietStdout(t)
	const path = "auth/token.go"
	const wantLine = 7
	excerpt := "\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A"

	d := parseFixture(t, path, resolverFile, true)

	// H1(a): stripping one prefix byte per payload line of d.Diff and rejoining
	// EQUALS the UTF-8 control file. Not "is a substring of" — raw bytes are a
	// substring of raw bytes, so containment passes at HEAD and proves nothing.
	t.Run("H1a_stripped_diff_payload_equals_the_control_file", func(t *testing.T) {
		got := strings.Join(stripPayload(t, d.Diff), "\n") + "\n"
		if got != resolverFile {
			t.Errorf("stripped payload is not the control file:\n got %q\nwant %q", got, resolverFile)
		}
	})

	// H1(b): the new-file text the ScanItem entry point is built from carries
	// the same control. Asserting it on a ScanItem literal would only prove the
	// literal copied the field, so it is asserted on d.NewFileContent itself.
	t.Run("H1b_new_file_content_equals_the_same_control", func(t *testing.T) {
		if d.NewFileContent != resolverFile {
			t.Errorf("scan content is not the control file:\n got %q\nwant %q", d.NewFileContent, resolverFile)
		}
	})

	// H1(c) / G6: both entry points resolve the same comment to the same
	// non-zero line. "0 == 0" must not pass.
	t.Run("H1c_G6_both_entry_points_resolve_to_the_same_nonzero_line", func(t *testing.T) {
		review := &model.LlmComment{Path: path, ExistingCode: excerpt}
		if !ResolveComment(review, &d) {
			t.Fatal("the model.Diff entry point did not resolve")
		}

		it := &model.ScanItem{Path: path, Content: d.NewFileContent, LineCount: 9}
		scanDiff := it.AsDiff()
		scan := &model.LlmComment{Path: path, ExistingCode: excerpt}
		if !ResolveComment(scan, scanDiff) {
			t.Fatal("the ScanItem entry point did not resolve")
		}

		if review.StartLine == 0 || scan.StartLine == 0 {
			t.Fatalf("a zero line number is not agreement: diff %d, scan item %d",
				review.StartLine, scan.StartLine)
		}
		if review.StartLine != scan.StartLine {
			t.Errorf("the model.Diff resolved to %d, the ScanItem to %d", review.StartLine, scan.StartLine)
		}
		if review.StartLine != wantLine {
			t.Errorf("resolved to %d, want %d", review.StartLine, wantLine)
		}
	})
}
