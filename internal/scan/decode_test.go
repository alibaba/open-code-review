// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/textenc"
)

// Fixtures are ASCII \uXXXX literals encoded at test time, so the legacy bytes
// and the UTF-8 control come from one constant and make english-check stays
// green with no allow-non-english marker.

const scanGBK = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
	"package auth\n\nfunc Validate(tok string) error { return nil }\n"

const scanBig5 = "// \u4F7F\u7528\u8005\u8A8D\u8B49\u6A21\u7D44\uFF0C\u8CA0\u8CAC\u6AA2\u9A57\u6B0A\u6756\u4E26\u66F4\u65B0\u5DE5\u4F5C\u968E\u6BB5\u72C0\u614B\u3002\n" +
	"// \u82E5\u6B0A\u6756\u5DF2\u7D93\u903E\u671F\uFF0C\u547C\u53EB\u7AEF\u5FC5\u9808\u91CD\u65B0\u767B\u5165\u53D6\u5F97\u65B0\u7684\u6191\u8B49\u3002\n" +
	"// \u672C\u6A94\u6848\u4F7F\u7528\u50B3\u7D71\u7DE8\u78BC\u5132\u5B58\uFF0C\u5BE9\u67E5\u5DE5\u5177\u5FC5\u9808\u5148\u89E3\u78BC\u518D\u95B1\u8B80\u3002\n" +
	"package auth\n\nfunc Validate(tok string) error { return nil }\n"

const scanSJIS = "// \u5229\u7528\u8005\u8A8D\u8A3C\u30E2\u30B8\u30E5\u30FC\u30EB\u3067\u3059\u3002\u30C8\u30FC\u30AF\u30F3\u3092\u691C\u8A3C\u3057\u3066\u3001\u30BB\u30C3\u30B7\u30E7\u30F3\u72B6\u614B\u3092\u66F4\u65B0\u3057\u307E\u3059\u3002\n" +
	"// \u30C8\u30FC\u30AF\u30F3\u306E\u6709\u52B9\u671F\u9650\u304C\u5207\u308C\u3066\u3044\u308B\u5834\u5408\u306F\u3001\u518D\u5EA6\u30ED\u30B0\u30A4\u30F3\u3057\u3066\u304F\u3060\u3055\u3044\u3002\n" +
	"// \u3053\u306E\u30D5\u30A1\u30A4\u30EB\u306F\u5F93\u6765\u306E\u6587\u5B57\u30B3\u30FC\u30C9\u3067\u4FDD\u5B58\u3055\u308C\u3066\u3044\u307E\u3059\u3002\n" +
	"package auth\n\nfunc Validate(tok string) error { return nil }\n"

// scanEUCKR is Korean, the one language in the allowlist with no other
// fixture here. The Japanese EUC-JP case reuses scanSJIS: same text, different
// bytes, so it also pins that detection separates the two Japanese encodings
// rather than merely recognising the language.
const scanEUCKR = "// \uC0AC\uC6A9\uC790 \uC778\uC99D \uBAA8\uB4C8\uC785\uB2C8\uB2E4. \uD1A0\uD070\uC744 \uAC80\uC99D\uD558\uACE0 \uC138\uC158 \uC0C1\uD0DC\uB97C \uAC31\uC2E0\uD569\uB2C8\uB2E4.\n" +
	"// \uD1A0\uD070\uC774 \uB9CC\uB8CC\uB41C \uACBD\uC6B0\uC5D0\uB294 \uB2E4\uC2DC \uB85C\uADF8\uC778\uD558\uC5EC \uC0C8\uB85C\uC6B4 \uC790\uACA9 \uC99D\uBA85\uC744 \uBC1B\uC544\uC57C \uD569\uB2C8\uB2E4.\n" +
	"// \uC774 \uD30C\uC77C\uC740 \uC608\uC804 \uBB38\uC790 \uC778\uCF54\uB529\uC73C\uB85C \uC800\uC7A5\uB418\uC5B4 \uC788\uC2B5\uB2C8\uB2E4.\n" +
	"package auth\n\nfunc Validate(tok string) error { return nil }\n"

const scanUTF8 = "package auth\n\n// Validate reports whether the token is usable.\nfunc Validate(tok string) error { return nil }\n"

func encodeScanFixture(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, _, err := transform.Bytes(enc.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return out
}

func writeScanFile(t *testing.T, dir, name string, raw []byte) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func itemsByPath(items []model.ScanItem) map[string]model.ScanItem {
	out := make(map[string]model.ScanItem, len(items))
	for _, it := range items {
		out[it.Path] = it
	}
	return out
}

// H3: a mixed repo. Every file is detected on its own whole-file evidence and
// decoded with its own charset: one repo, five answers.
func TestEnumerateDecodesEachFileWithItsOwnCharset(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "gbk.go", encodeScanFixture(t, simplifiedchinese.GBK, scanGBK))
	writeScanFile(t, dir, "big5.go", encodeScanFixture(t, traditionalchinese.Big5, scanBig5))
	writeScanFile(t, dir, "sjis.go", encodeScanFixture(t, japanese.ShiftJIS, scanSJIS))
	writeScanFile(t, dir, "eucjp.go", encodeScanFixture(t, japanese.EUCJP, scanSJIS))
	writeScanFile(t, dir, "euckr.go", encodeScanFixture(t, korean.EUCKR, scanEUCKR))
	writeScanFile(t, dir, "clean.go", []byte(scanUTF8))

	var out bytes.Buffer
	restore := stdout.Swap(&out)
	items, err := NewProvider(dir, nil, nil, 0).Enumerate(context.Background())
	restore()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}

	byPath := itemsByPath(items)
	for _, tc := range []struct {
		file    string
		control string
	}{
		{"gbk.go", scanGBK},
		{"big5.go", scanBig5},
		{"sjis.go", scanSJIS},
		// EUC-JP and EUC-KR are in the decoder allowlist but were exercised
		// only by textenc's own unit tests; nothing proved a file in either
		// charset survives enumeration.
		{"eucjp.go", scanSJIS},
		{"euckr.go", scanEUCKR},
		{"clean.go", scanUTF8},
	} {
		t.Run("H3_"+tc.file, func(t *testing.T) {
			it, ok := byPath[tc.file]
			if !ok {
				t.Fatalf("%s was not enumerated", tc.file)
			}
			if it.Content != tc.control {
				t.Errorf("Content is not the UTF-8 control:\n got %q\nwant %q", it.Content, tc.control)
			}
			if want := strings.Count(tc.control, "\n"); it.LineCount != want {
				t.Errorf("LineCount = %d, want %d (counted on the decoded text)", it.LineCount, want)
			}
			if strings.ContainsRune(it.Content, utf8.RuneError) {
				t.Errorf("decoded content still holds U+FFFD")
			}
		})
	}

	// I2: one info line per decoded file, naming path, charset and confidence.
	t.Run("I2_one_info_line_per_decoded_file", func(t *testing.T) {
		got := out.String()
		for _, want := range []string{
			"gbk.go", "big5.go", "sjis.go", "eucjp.go", "euckr.go",
			"GB-18030", "Big5", "Shift_JIS", "EUC-JP", "EUC-KR", "confidence",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("info output does not mention %q:\n%s", want, got)
			}
		}
		if n := strings.Count(got, "[ocr] decoded "); n != 5 {
			t.Errorf("got %d decode info lines, want 5 (one per legacy file):\n%s", n, got)
		}
		if strings.Contains(got, "clean.go") {
			t.Errorf("the UTF-8 fast path must print nothing:\n%s", got)
		}
	})
}

// A file we cannot decode keeps its raw bytes and is marked. Whether it is also
// excluded is the filter's decision, not the provider's.
func TestEnumerateMarksUndecodableFiles(t *testing.T) {
	dir := t.TempDir()
	garbage := make([]byte, 600)
	for i := range garbage {
		garbage[i] = byte(0x80 + (i*37+11)%0x7F)
	}
	writeScanFile(t, dir, "blob.go", garbage)

	restore := stdout.Swap(io.Discard)
	items, err := NewProvider(dir, nil, nil, 0).Enumerate(context.Background())
	restore()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}

	it := itemsByPath(items)["blob.go"]
	if it.Content != string(garbage) {
		t.Error("an undecodable file must keep its raw bytes")
	}
	if it.UndecodedCharset == "" {
		t.Error("an undecodable file must be marked with the detector's answer")
	}
	if !it.Unreviewable {
		t.Error("high-byte garbage must be judged past reviewing")
	}
}

// B5 pins a deliberate review/scan asymmetry: scan's NUL sniff runs before the
// file is read, so a UTF-16 file never reaches the decoder here even though
// review mode decodes it. Changing that would mean reading every binary.
func TestEnumerateUTF16IsBinaryBeforeDecoding(t *testing.T) {
	dir := t.TempDir()
	raw := encodeScanFixture(t, unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM), scanUTF8)
	writeScanFile(t, dir, "wide.go", raw)

	// Precondition: the decoder itself would handle this file happily.
	if _, _, ok := textenc.Detect(raw); !ok {
		t.Fatal("precondition: textenc must be able to decode this fixture")
	}

	restore := stdout.Swap(io.Discard)
	items, err := NewProvider(dir, nil, nil, 0).Enumerate(context.Background())
	restore()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}

	it := itemsByPath(items)["wide.go"]
	if !it.IsBinary {
		t.Errorf("scan's NUL sniff no longer classifies UTF-16 as binary")
	}
	if it.Content != "" {
		t.Errorf("a binary item must carry no content, got %q", it.Content)
	}
}

// J1 is a REGRESSION GUARD, not decode evidence: ocr is a read-only reviewer
// and must stay one. The sha256 half passes at HEAD; the second half is what
// stops it passing vacuously on a build where nothing was decoded at all.
func TestEnumerateNeverWritesToTheWorkingTree(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "gbk.go", encodeScanFixture(t, simplifiedchinese.GBK, scanGBK))
	writeScanFile(t, dir, "big5.go", encodeScanFixture(t, traditionalchinese.Big5, scanBig5))
	writeScanFile(t, dir, "clean.go", []byte(scanUTF8))

	before := hashTree(t, dir)

	var out bytes.Buffer
	restore := stdout.Swap(&out)
	if _, err := NewProvider(dir, nil, nil, 0).Enumerate(context.Background()); err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	restore()

	after := hashTree(t, dir)
	if len(before) != len(after) {
		t.Fatalf("file count changed: %d -> %d", len(before), len(after))
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Errorf("%s was modified on disk", name)
		}
	}
	if !strings.Contains(out.String(), "[ocr] decoded ") {
		t.Fatal("nothing was decoded, so the sha256 comparison proves nothing")
	}
}

// hashTree maps each file under dir to the sha256 of its bytes. It hashes
// rather than keeping the bodies: an earlier version called sha256.New().Sum(b),
// which APPENDS the digest of the (empty) stream to b and so held every file
// body in the map under the name of a hash.
func hashTree(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[rel] = sha256.Sum256(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}

// gitInScanTest runs git in dir and fails the test on a non-zero exit.
func gitInScanTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitDiffBytes returns the raw diff git prints for HEAD, headers and all. Bytes
// and not a string: the payload is legacy-encoded, so anything that normalises
// or trims here would decode the fixture before the code under test sees it.
func gitDiffBytes(t *testing.T, dir string) []byte {
	t.Helper()
	cmd := exec.Command("git", "--no-pager", "show", "--no-color", "--format=", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	return out
}

// The issue's "diff and full-file scan modes behave consistently" criterion,
// tested for real. ONE repo, one legacy file, two INDEPENDENT readers: the
// review path (diff.ParseDiffText, which detects and decodes in
// finalizeDiff/decodeDiffFile) and the scan path (Provider.Enumerate, which
// detects and decodes at its own seam). Neither side is derived from the other.
//
// That independence is the entire point. The version of this test that lived in
// internal/diff built its "scan" item out of the review path's own
// NewFileContent, so it never touched scan.Provider and stayed green with scan
// decoding switched off completely.
//
// It lives in internal/scan because internal/scan already imports internal/diff
// (provider.go uses diff.LoadGitignorePatterns); the reverse import would be a
// cycle, so internal/diff can never test the scan seam.
//
// Agreement alone is not the assertion: two seams that both fail to decode
// agree perfectly. Both sides are also checked against the UTF-8 control.
func TestReviewAndScanDecodeOneFileIdentically(t *testing.T) {
	const path = "auth/token.go"
	dir := t.TempDir()
	raw := encodeScanFixture(t, simplifiedchinese.GBK, scanGBK)

	gitInScanTest(t, dir, "init", "-q")
	gitInScanTest(t, dir, "config", "user.email", "test@example.com")
	gitInScanTest(t, dir, "config", "user.name", "Test User")
	gitInScanTest(t, dir, "config", "commit.gpgsign", "false")
	writeScanFile(t, dir, path, raw)
	gitInScanTest(t, dir, "add", path)
	gitInScanTest(t, dir, "commit", "-q", "-m", "initial")

	restore := stdout.Swap(io.Discard)
	diffs, diffErr := diff.ParseDiffText(context.Background(), string(gitDiffBytes(t, dir)), dir, "", nil)
	items, scanErr := NewProvider(dir, nil, nil, 0).Enumerate(context.Background())
	restore()

	if diffErr != nil {
		t.Fatalf("ParseDiffText: %v", diffErr)
	}
	if scanErr != nil {
		t.Fatalf("Enumerate: %v", scanErr)
	}
	if len(diffs) != 1 || diffs[0].NewPath != path {
		t.Fatalf("review path produced %d diff(s), want 1 for %s", len(diffs), path)
	}
	review := diffs[0].NewFileContent
	item, ok := itemsByPath(items)[path]
	if !ok {
		t.Fatalf("scan path did not enumerate %s", path)
	}
	scanned := item.Content

	if review != scanned {
		t.Errorf("the two modes decoded the same file differently:\n review %q\n   scan %q", review, scanned)
	}
	if review != scanGBK {
		t.Errorf("review mode is not the UTF-8 control:\n got %q\nwant %q", review, scanGBK)
	}
	if scanned != scanGBK {
		t.Errorf("scan mode is not the UTF-8 control:\n got %q\nwant %q", scanned, scanGBK)
	}
}

// captureStderr runs fn with os.Stderr replaced by a pipe and returns what was
// written. Same helper internal/agent's decode_filter_test.go uses, for the
// same reason: the undecoded warning goes to stderr so it survives
// --audience agent, which silences stdout.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = w
	fn()
	os.Stderr = old
	if err := w.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	out, readErr := io.ReadAll(r)
	if closeErr := r.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		t.Fatalf("read captured stderr: %v", readErr)
	}
	return string(out)
}

// The scan-side mirror of the filter split. Same two outcomes, same messages.
func TestFilterScanItemsEncodingReporting(t *testing.T) {
	a := NewAgent(Args{
		Template: makeTemplateWithFullScan(),
	})

	t.Run("D8_marked_but_still_reviewed", func(t *testing.T) {
		var out bytes.Buffer
		var kept []model.ScanItem
		restore := stdout.Swap(&out)
		warn := captureStderr(t, func() {
			kept = a.filterScanItems([]model.ScanItem{
				{Path: "token.go", Content: "package auth\n", UndecodedCharset: "windows-1252"},
				{Path: "clean.go", Content: "package auth\n"},
			})
		})
		restore()
		if len(kept) != 2 {
			t.Fatalf("kept %d item(s), want both", len(kept))
		}
		if strings.Contains(out.String(), "Skipping token.go") {
			t.Errorf("an imperfect file was skipped:\n%s", out.String())
		}

		// I1, the same assertion internal/agent's D8 makes on the review seam:
		// exactly one stderr warning, naming the path and the charset. "It was
		// not skipped" passes with the warning gutted to a bare newline, which
		// is how a user loses the only notice that their review text may be
		// mojibake.
		if n := strings.Count(warn, "[ocr] WARNING:"); n != 1 {
			t.Errorf("got %d stderr warnings, want exactly 1:\n%s", n, warn)
		}
		for _, want := range []string{"token.go", "windows-1252"} {
			if !strings.Contains(warn, want) {
				t.Errorf("warning does not mention %q:\n%s", want, warn)
			}
		}
		if strings.Contains(warn, "clean.go") {
			t.Errorf("a clean file must not be warned about:\n%s", warn)
		}
	})

	t.Run("D10_excluded_message_names_the_encoding", func(t *testing.T) {
		var out bytes.Buffer
		restore := stdout.Swap(&out)
		kept := a.filterScanItems([]model.ScanItem{
			{Path: "blob.go", Content: "x", UndecodedCharset: "Big5", Unreviewable: true},
			{Path: "clean.go", Content: "package auth\n"},
		})
		restore()
		if len(kept) != 1 || kept[0].Path != "clean.go" {
			t.Fatalf("kept %+v, want only clean.go", kept)
		}
		got := out.String()
		if !strings.Contains(got, "Skipping blob.go — undecodable encoding (detected Big5)") {
			t.Errorf("skip line does not name the real reason:\n%s", got)
		}
		if !strings.Contains(got, "Filtered 1 file(s) by include/exclude rules") {
			t.Errorf("the trailing summary line was lost:\n%s", got)
		}
	})
}
