// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// scanPreviewGBK is a legacy-encoded fixture that makes the provider emit its
// per-file decode notice on stdout — the write that must not reach a JSON
// document. ASCII \uXXXX literals, encoded at test time.
const scanPreviewGBK = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
	"package auth\n\nfunc Validate(tok string) error { return nil }\n"

// A file whose bytes decode to nothing usable, so the preview has an
// undecodable_encoding entry to report.
func scanPreviewGarbage() []byte {
	out := make([]byte, 800)
	for i := range out {
		out[i] = byte(0x80 + (i*37+11)%0x7F)
	}
	return out
}

// TestScanPreviewJSONStaysParseableWithLegacyFiles is a regression test for a
// real defect found end-to-end: the preview path never installed the quiet
// handle, so the charset-decode notice was printed to stdout ahead of the JSON
// document and `ocr scan --preview --format json` no longer parsed.
//
// The non-preview path silences stdout at scan_cmd.go's newQuietHandle; the
// preview path now does the same.
func TestScanPreviewJSONStaysParseableWithLegacyFiles(t *testing.T) {
	dir := initTestGitRepo(t)

	raw, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(scanPreviewGBK))
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	writeAndCommit(t, dir, "legacy.go", raw)
	writeAndCommit(t, dir, "blob.go", scanPreviewGarbage())
	writeAndCommit(t, dir, "clean.go", []byte("package auth\n\nfunc Clean() {}\n"))

	cc, err := loadCommonContext(dir, "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}

	// The decode notice and the rendered preview travel on two different
	// handles: the notice goes through stdout.Writer(), the preview through
	// os.Stdout. In a real run both are the same file descriptor, which is
	// exactly why one can corrupt the other.
	t.Run("text_format_still_reports_the_decode", func(t *testing.T) {
		var notice bytes.Buffer
		restore := stdout.Swap(&notice)
		rendered := captureStdout(t, func() {
			if err := runScanPreview(cc, scanTpl, nil, "text"); err != nil {
				t.Fatalf("runScanPreview: %v", err)
			}
		})
		restore()

		if !strings.Contains(notice.String(), "[ocr] decoded legacy.go from GB-18030") {
			t.Errorf("text mode lost the decode notice:\n%s", notice.String())
		}
		if !strings.Contains(rendered, "undecodable_encoding") {
			t.Errorf("text mode did not report the excluded file:\n%s", rendered)
		}
	})

	t.Run("json_format_is_a_single_parseable_document", func(t *testing.T) {
		// Reproduce the real command faithfully: point stdout.Writer() at the
		// SAME pipe os.Stdout is captured on, so the decode notice and the JSON
		// document share one stream exactly as they share one fd in a real run.
		// Without the quiet handle the notice lands in front of the JSON and
		// json.Unmarshal fails — that is what makes this case falsifiable.
		out := captureStdout(t, func() {
			restoreWriter := stdout.Swap(os.Stdout)
			defer restoreWriter()
			q := newQuietHandle("json", "")
			defer q.Restore()

			if err := runScanPreview(cc, scanTpl, nil, "json"); err != nil {
				t.Fatalf("runScanPreview: %v", err)
			}
		})

		if strings.Contains(out, "[ocr]") {
			t.Fatalf("human-readable output leaked into the JSON document:\n%s", out)
		}
		var p model.Preview
		if err := json.Unmarshal([]byte(out), &p); err != nil {
			t.Fatalf("preview JSON does not parse (%v):\n%s", err, out)
		}

		byPath := map[string]model.PreviewEntry{}
		for _, e := range p.Entries {
			byPath[e.Path] = e
		}
		blob, ok := byPath["blob.go"]
		if !ok {
			t.Fatal("blob.go missing from the preview")
		}
		if blob.WillReview {
			t.Error("an undecodable file must not be reviewed")
		}
		if blob.ExcludeReason != model.ExcludeUndecodable {
			t.Errorf("exclude_reason = %q, want %q", blob.ExcludeReason, model.ExcludeUndecodable)
		}
		// Pinned to the exact label, not merely non-empty: the failure this
		// guards against is the field carrying the WRONG charset (the previous
		// file's, or a hard-coded constant), which a non-empty check waves
		// through. "Big5" is what the detector reports for these bytes at
		// confidence 10 — nonsense as a charset, and that low score is exactly
		// why the file was never decoded. The user is told what was detected,
		// which is the only actionable thing there is to say.
		//
		// Pinning a label is only legitimate because textenc.Detect is
		// deterministic. Shift_JIS, GB-18030 and Big5 all score 10 on these
		// bytes, and chardet's own DetectBest returns a different one of the
		// three from run to run; this assertion failed about one run in ten
		// until textenc broke the tie by charset name. See deterministicBest.
		if blob.DetectedCharset != "Big5" {
			t.Errorf("detected_charset = %q, want %q", blob.DetectedCharset, "Big5")
		}
		if !byPath["legacy.go"].WillReview {
			t.Error("a decodable legacy file must still be reviewed")
		}
	})
}

// writeAndCommit is the []byte twin of gitCommitFile: the fixtures here are
// legacy bytes, which cannot round-trip through a Go string parameter.
func writeAndCommit(t *testing.T, dir, name string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "add " + name}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
