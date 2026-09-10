// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/textenc"
)

// fixtures
//
// ASCII \uXXXX literals only, encoded at test time with the matching x/text
// encoder. Deriving the legacy bytes and the UTF-8 control from ONE constant
// makes "identical content in two encodings" structural instead of a property
// somebody has to maintain across checked-in files, and keeps
// make english-check green with no allow-non-english marker.

// gbkFileNew is the on-disk new-side content for the structure and frame cases
// (F1-F4, F7, F8): CJK-dense doc comments at the top, an ASCII-heavy function
// below. The whole file detects as GB-18030 at confidence 100, which is the
// only reason those synthesized control diffs decode at all. F16/F17 need a
// real git diff, so they carry their own gbkF16Old/gbkF16New pair.
const gbkFileNew = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
	"package auth\n" +
	"\n" +
	"func Validate(tok string) (string, error) {\n" +
	"\tif tok == \"\" {\n" +
	"\t\treturn \"\", errEmptyToken\n" +
	"\t}\n" +
	"\treturn lookup(strings.TrimSpace(tok))\n" +
	"}\n"

func encodeFixture(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, _, err := transform.Bytes(enc.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return out
}

// countDetections swaps in a counting detector for the duration of the test.
// Tests using it must not call t.Parallel(): make test runs -race.
func countDetections(t *testing.T) *int {
	t.Helper()
	calls, restore := textenc.CountDetectionsForTest()
	t.Cleanup(restore)
	return calls
}

// quietStdout silences the per-file "[ocr] decoded ..." info line so it does
// not pollute test output, and returns the buffer for tests that assert on it.
func quietStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	t.Cleanup(stdout.Swap(&buf))
	return &buf
}

// writeRepoFile writes raw bytes into dir under name, creating parents.
func writeRepoFile(t *testing.T, dir, name string, raw []byte) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// buildDiff renders a one-file unified diff for the given hunk body. The body
// lines already carry their "+", "-" or " " prefix.
func buildDiff(path, header string, body ...string) string {
	lines := []string{
		"diff --git a/" + path + " b/" + path,
		"index 1111111..2222222 100644",
		"--- a/" + path,
		"+++ b/" + path,
		header,
	}
	return strings.Join(append(lines, body...), "\n")
}

// realDiff creates a git repo holding old, commits it, replaces it with new,
// and returns the repo dir plus the exact unified diff git produced. Fixtures
// are written as raw bytes so git sees a genuinely legacy-encoded file.
func realDiff(t *testing.T, name string, old, new []byte) (string, string) {
	t.Helper()
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	writeRepoFile(t, repo, name, old)
	runGitTest(t, repo, "add", name)
	runGitTest(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, repo, name, new)

	out, err := gitCapture(t, repo, "diff", "--no-ext-diff", "--", name)
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	return repo, out
}

func gitCapture(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// stripPayload returns the payload line bodies of a section, prefix byte
// removed, in order — i.e. what the file content of the new side looks like.
func stripPayload(t *testing.T, section string) []string {
	t.Helper()
	var out []string
	inHunk := false
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		if c := line[0]; c != '+' && c != '-' && c != ' ' {
			continue
		}
		out = append(out, line[1:])
	}
	return out
}

// frameLines returns everything that is NOT payload, in order.
func frameLines(section string) []string {
	var out []string
	inHunk := false
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			out = append(out, line)
			continue
		}
		if inHunk && line != "" {
			if c := line[0]; c == '+' || c == '-' || c == ' ' {
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

// parseGBK writes the GBK file to a temp dir, encodes the control diff to GBK,
// and runs the real ParseDiffText over it. It returns the decoded result and
// the UTF-8 control diff text they must agree with.
func parseGBK(t *testing.T, path, controlDiff string, fileText string) (model.Diff, string) {
	t.Helper()
	dir := t.TempDir()
	writeRepoFile(t, dir, path, encodeFixture(t, simplifiedchinese.GBK, fileText))
	raw := encodeFixture(t, simplifiedchinese.GBK, controlDiff)

	diffs, err := ParseDiffText(context.Background(), string(raw), dir, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	return diffs[0], controlDiff
}

// F: unified diff structure

func TestDecodeDiffStructure(t *testing.T) {
	quietStdout(t)

	const path = "internal/auth/token.go"
	controlDiff := buildDiff(path, "@@ -1,4 +1,4 @@",
		"// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u3002",
		"-// \u65E7\u7684\u6CE8\u91CA\uFF1A\u5DF2\u7ECF\u8FC7\u671F\u4E86\u3002",
		"+// \u65B0\u7684\u6CE8\u91CA\uFF1A\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u51ED\u8BC1\u3002",
		" package auth")
	// The context line above deliberately starts with a space in the control;
	// rebuild it so the fixture is a well-formed diff.
	controlDiff = strings.Replace(controlDiff,
		"\n// \u7528\u6237", "\n // \u7528\u6237", 1)

	got, control := parseGBK(t, path, controlDiff, gbkFileNew)

	t.Run("F1_file_headers_byte_identical", func(t *testing.T) {
		// No index assertion: parser.go drops "index " lines before writing,
		// so they never reach d.Diff.
		for _, want := range []string{
			"diff --git a/" + path + " b/" + path,
			"--- a/" + path,
			"+++ b/" + path,
		} {
			if !strings.Contains(got.Diff, want) {
				t.Errorf("header %q missing from decoded diff", want)
			}
		}
	})

	t.Run("F2_hunk_headers_byte_identical", func(t *testing.T) {
		if !strings.Contains(got.Diff, "@@ -1,4 +1,4 @@") {
			t.Errorf("hunk header was rewritten:\n%s", got.Diff)
		}
	})

	t.Run("F1_F2_whole_frame_byte_identical_to_the_control", func(t *testing.T) {
		// F1/F2 spot-check single header lines with Contains, which a decoder
		// that reordered the frame, dropped a hunk header or appended a stray
		// line would still pass. This pins the ENTIRE non-payload frame, in
		// order, against the same diff parsed from UTF-8 bytes.
		ctl := parseUTF8Control(t, path, control, gbkFileNew)
		gotFrame := strings.Join(frameLines(got.Diff), "\n")
		wantFrame := strings.Join(frameLines(ctl.Diff), "\n")
		if gotFrame != wantFrame {
			t.Errorf("decoded frame:\n%q\nwant:\n%q", gotFrame, wantFrame)
		}
	})

	t.Run("F3_prefix_bytes_preserved_in_order", func(t *testing.T) {
		wantPrefixes := prefixesOf(control)
		gotPrefixes := prefixesOf(got.Diff)
		if len(gotPrefixes) != len(wantPrefixes) {
			t.Fatalf("payload line count %d, want %d", len(gotPrefixes), len(wantPrefixes))
		}
		for i := range wantPrefixes {
			if gotPrefixes[i] != wantPrefixes[i] {
				t.Errorf("payload line %d prefix %q, want %q", i, gotPrefixes[i], wantPrefixes[i])
			}
		}
	})

	t.Run("F4_section_line_count_unchanged", func(t *testing.T) {
		raw := string(encodeFixture(t, simplifiedchinese.GBK, control))
		// The parser drops "index " lines, so compare against the control put
		// through the same parser rather than against the raw text.
		if a, b := strings.Count(got.Diff, "\n"), strings.Count(parseUTF8Control(t, path, control, gbkFileNew).Diff, "\n"); a != b {
			t.Errorf("line count %d, want %d (raw input had %d)", a, b, strings.Count(raw, "\n"))
		}
	})

	t.Run("F12_insertions_and_deletions_match_control", func(t *testing.T) {
		ctl := parseUTF8Control(t, path, control, gbkFileNew)
		if got.Insertions != ctl.Insertions || got.Deletions != ctl.Deletions {
			t.Errorf("+%d/-%d, want +%d/-%d",
				got.Insertions, got.Deletions, ctl.Insertions, ctl.Deletions)
		}
	})

	t.Run("F13_hunk_geometry_survives_a_length_change", func(t *testing.T) {
		// GBK CJK is 2 bytes, UTF-8 CJK is 3: decoding grows the payload.
		if len(got.Diff) <= len(string(encodeFixture(t, simplifiedchinese.GBK, control))) {
			t.Fatal("precondition: decoding must have changed the byte length")
		}
		ctl := parseUTF8Control(t, path, control, gbkFileNew)
		gotHunks, wantHunks := ParseHunks(got.Diff), ParseHunks(ctl.Diff)
		if len(gotHunks) != len(wantHunks) {
			t.Fatalf("got %d hunks, want %d", len(gotHunks), len(wantHunks))
		}
		for i := range wantHunks {
			g, w := gotHunks[i], wantHunks[i]
			if g.OldStart != w.OldStart || g.OldCount != w.OldCount ||
				g.NewStart != w.NewStart || g.NewCount != w.NewCount {
				t.Errorf("hunk %d geometry %v, want %v", i, g, w)
			}
		}
	})

	t.Run("payload_equals_the_utf8_control", func(t *testing.T) {
		ctl := parseUTF8Control(t, path, control, gbkFileNew)
		gotBodies := stripPayload(t, got.Diff)
		wantBodies := stripPayload(t, ctl.Diff)
		if strings.Join(gotBodies, "\n") != strings.Join(wantBodies, "\n") {
			t.Errorf("decoded payload:\n%q\nwant:\n%q", gotBodies, wantBodies)
		}
	})
}

// parseUTF8Control runs the identical chain over UTF-8 bytes.
func parseUTF8Control(t *testing.T, path, controlDiff, fileText string) model.Diff {
	t.Helper()
	dir := t.TempDir()
	writeRepoFile(t, dir, path, []byte(fileText))
	diffs, err := ParseDiffText(context.Background(), controlDiff, dir, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText (control): %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("control produced %d diffs, want 1", len(diffs))
	}
	return diffs[0]
}

func prefixesOf(section string) []string {
	var out []string
	inHunk := false
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		if c := line[0]; c == '+' || c == '-' || c == ' ' {
			out = append(out, line[:1])
		}
	}
	return out
}

// gbkF16 is the one-detection-per-file fixture: CJK-dense doc comments at the
// top, and a change deep enough in the file that a -U3 hunk carries only one
// short CJK comment. The whole file detects; the hunk on its own does not.
const gbkF16Old = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
	"package auth\n" +
	"\n" +
	"import \"strings\"\n" +
	"\n" +
	"func Validate(tok string) (string, error) {\n" +
	"\tif tok == \"\" {\n" +
	"\t\treturn \"\", errEmptyToken\n" +
	"\t}\n" +
	"\t// \u6821\u9A8C\u901A\u8FC7\n" +
	"\treturn lookup(strings.TrimSpace(tok))\n" +
	"}\n"

const gbkF16New = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
	"package auth\n" +
	"\n" +
	"import \"strings\"\n" +
	"\n" +
	"func Validate(tok string) (string, error) {\n" +
	"\tif tok == \"\" {\n" +
	"\t\treturn \"\", errEmptyToken\n" +
	"\t}\n" +
	"\t// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7\n" +
	"\treturn lookup(strings.TrimSpace(tok))\n" +
	"}\n"

// F16 is the regression test for the whole design: detection must happen on
// whole-file evidence, not per stream. It verifies its own preconditions first,
// because a fixture whose hunk payload happens to detect on its own would make
// the real assertion pass vacuously on a per-stream build.
func TestDecodeDetectsOnWholeFileNotPerStream(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	repo, diffText := realDiff(t, name,
		encodeFixture(t, simplifiedchinese.GBK, gbkF16Old),
		encodeFixture(t, simplifiedchinese.GBK, gbkF16New))

	t.Run("F16_precondition_hunk_payload_alone_does_not_detect", func(t *testing.T) {
		payload, _ := splitHunkPayload(diffText)
		if payload == "" {
			t.Fatal("fixture produced no hunk payload")
		}
		if _, _, ok := textenc.Detect([]byte(payload)); ok {
			t.Fatalf("the hunk payload detects on its own, so this fixture cannot "+
				"tell a whole-file design from a per-stream one:\n%q", payload)
		}
	})

	t.Run("F16_precondition_whole_file_detects_as_gb18030", func(t *testing.T) {
		raw := encodeFixture(t, simplifiedchinese.GBK, gbkF16New)
		cs, conf, ok := textenc.Detect(raw)
		if !ok || cs != "GB-18030" {
			t.Fatalf("whole file detected as (%q, %d, %v), want (GB-18030, >=90, true)", cs, conf, ok)
		}
	})

	t.Run("F16_small_hunk_big_file_decodes", func(t *testing.T) {
		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		if len(diffs) != 1 {
			t.Fatalf("got %d diffs, want 1", len(diffs))
		}
		got := strings.Join(stripPayload(t, diffs[0].Diff), "\n")
		if !strings.Contains(got, "// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7") {
			t.Errorf("hunk payload was not decoded:\n%q", got)
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Errorf("decoded payload still contains U+FFFD:\n%q", got)
		}
	})

	// F17: both streams commit together. Never one decoded and its sibling raw.
	t.Run("F17_both_streams_agree", func(t *testing.T) {
		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if d.NewFileContent != gbkF16New {
			t.Errorf("NewFileContent is not the UTF-8 control:\n got %q\nwant %q",
				d.NewFileContent, gbkF16New)
		}
		if !strings.Contains(strings.Join(stripPayload(t, d.Diff), "\n"), "// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7") {
			t.Error("d.Diff payload stayed raw while NewFileContent was decoded")
		}
	})

	// A file we cannot decode leaves BOTH streams raw and marked.
	t.Run("F17_skip_leaves_both_streams_raw", func(t *testing.T) {
		garbage := randomHighBytes(600)
		gRepo, gDiff := realDiff(t, "auth/blob.go", []byte("package auth\n"), garbage)
		diffs, err := ParseDiffText(context.Background(), gDiff, gRepo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if d.UndecodedCharset == "" {
			t.Error("an undecodable file must still be marked")
		}
		if d.NewFileContent != string(garbage) {
			t.Error("NewFileContent was rewritten for a file we could not decode")
		}
	})

	// I4: exactly one detection inside one ParseDiffText call. Fails on any
	// per-stream design, which would detect once for the hunk and once for the
	// file. Scope is the function, not the process: file_read and code_search
	// detect again at their own seams by design.
	t.Run("I4_one_detection_per_file_inside_ParseDiffText", func(t *testing.T) {
		calls := countDetections(t)
		if _, err := ParseDiffText(context.Background(), diffText, repo, "", nil); err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		if *calls != 1 {
			t.Errorf("detector invoked %d times for one GBK file, want exactly 1", *calls)
		}
	})
}

func TestDecodeUTF8RepoNeverDetects(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"
	repo, diffText := realDiff(t, name,
		[]byte("package auth\n\nfunc a() {}\n"),
		[]byte("package auth\n\nfunc b() {}\n"))

	// I5: a normal repo pays nothing. utf8.Valid short-circuits first.
	t.Run("I5_pure_utf8_repo_zero_detections", func(t *testing.T) {
		calls := countDetections(t)
		if _, err := ParseDiffText(context.Background(), diffText, repo, "", nil); err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		if *calls != 0 {
			t.Errorf("detector invoked %d times on a UTF-8 repo, want 0", *calls)
		}
	})
}

// F18: ModeRange and ModeCommit set a non-empty ref, which sends finalizeDiff
// down its `git show` branch. No other case reaches it.
func TestDecodeRefMode(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")
	writeRepoFile(t, repo, name, encodeFixture(t, simplifiedchinese.GBK, gbkF16Old))
	runGitTest(t, repo, "add", name)
	runGitTest(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, repo, name, encodeFixture(t, simplifiedchinese.GBK, gbkF16New))
	runGitTest(t, repo, "add", name)
	runGitTest(t, repo, "commit", "-q", "-m", "second")

	sha, err := gitCapture(t, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha = strings.TrimSpace(sha)

	diffText, err := gitCapture(t, repo, "diff", "--no-ext-diff", "HEAD~1", "HEAD", "--", name)
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}

	t.Run("F18_git_show_branch_decodes", func(t *testing.T) {
		calls := countDetections(t)
		// Point the working tree at something else so a decode here can only
		// have come from `git show`, not from the disk branch.
		writeRepoFile(t, repo, name, []byte("package auth // replaced\n"))

		diffs, err := ParseDiffText(context.Background(), diffText, repo, sha, nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if d.NewFileContent != gbkF16New {
			t.Errorf("git show branch did not decode:\n got %q\nwant %q", d.NewFileContent, gbkF16New)
		}
		if !strings.Contains(strings.Join(stripPayload(t, d.Diff), "\n"), "// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7") {
			t.Errorf("d.Diff stayed raw in ref mode:\n%s", d.Diff)
		}
		if *calls != 1 {
			t.Errorf("detector invoked %d times in ref mode, want exactly 1", *calls)
		}
	})
}

// F14 and F15 pin that classification comes from the parser's hunk state plus
// the first byte, never from matching header text. Both fixtures come out of
// real git, so the hazard is documented as fact rather than asserted about an
// invented diff.
func TestDecodePayloadLinesThatLookLikeHeaders(t *testing.T) {
	quietStdout(t)

	t.Run("F14_deleted_sql_comment_renders_as_triple_dash", func(t *testing.T) {
		const name = "db/schema.sql"
		base := "-- \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\u7684\u6570\u636E\u8868\u5B9A\u4E49\uFF0C\u8D1F\u8D23\u4FDD\u5B58\u4EE4\u724C\u4E0E\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
			"-- \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
			"-- \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
			"CREATE TABLE tokens (\n" +
			"\tid BIGINT PRIMARY KEY,\n" +
			"\tvalue TEXT NOT NULL\n" +
			");\n"
		old := base + "-- \u65E7\u7684\u6CE8\u91CA\u8BF4\u660E\u3002\nSELECT 1;\n"
		new := base + "-- \u65B0\u7684\u6CE8\u91CA\u8BF4\u660E\u3002\nSELECT 1;\n"
		repo, diffText := realDiff(t, name,
			encodeFixture(t, simplifiedchinese.GBK, old),
			encodeFixture(t, simplifiedchinese.GBK, new))

		// Precondition: git really emits the deleted "-- ..." line as "--- ..."
		// inside the hunk, where a prefix-list classifier reads it as a header.
		if !strings.Contains(diffText, "\n--- ") || !strings.Contains(diffText, "\n+++ ") {
			t.Fatalf("precondition: git did not emit the ---/+++ collision:\n%s", diffText)
		}

		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		body := strings.Join(stripPayload(t, diffs[0].Diff), "\n")
		if !strings.Contains(body, "-- \u65B0\u7684\u6CE8\u91CA\u8BF4\u660E\u3002") {
			t.Errorf("the added SQL comment was not decoded:\n%q", body)
		}
		if !strings.Contains(body, "-- \u65E7\u7684\u6CE8\u91CA\u8BF4\u660E\u3002") {
			t.Errorf("the deleted SQL comment (emitted as \"--- ...\") was treated as a header:\n%q", body)
		}
		// The real file headers must still be byte-identical.
		if !strings.Contains(diffs[0].Diff, "--- a/"+name) ||
			!strings.Contains(diffs[0].Diff, "+++ b/"+name) {
			t.Errorf("file headers were rewritten:\n%s", diffs[0].Diff)
		}
	})

	t.Run("F15_added_line_starting_with_plus_plus", func(t *testing.T) {
		const name = "auth/count.go"
		base := "// \u8BA1\u6570\u5668\u6A21\u5757\uFF0C\u8D1F\u8D23\u7EDF\u8BA1\u4EE4\u724C\u6821\u9A8C\u7684\u8C03\u7528\u6B21\u6570\u3002\n" +
			"// \u5982\u679C\u8BA1\u6570\u6EA2\u51FA\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u521D\u59CB\u5316\u7EDF\u8BA1\u72B6\u6001\u3002\n" +
			"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
			"package auth\n" +
			"\n" +
			"func f(i int) {\n" +
			"\tj := i\n" +
			"\t_ = j\n" +
			"\t// \u8BA1\u6570\u5668\u81EA\u589E\n"
		old := base + "}\n"
		new := base + "\t++i\n}\n"
		repo, diffText := realDiff(t, name,
			encodeFixture(t, simplifiedchinese.GBK, old),
			encodeFixture(t, simplifiedchinese.GBK, new))

		if !strings.Contains(diffText, "+\t++i") {
			t.Fatalf("precondition: git did not emit the ++ collision:\n%s", diffText)
		}

		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if !strings.Contains(d.Diff, "+\t++i") {
			t.Errorf("the \"++i\" line lost its prefix or its text:\n%s", d.Diff)
		}
		if !strings.Contains(strings.Join(stripPayload(t, d.Diff), "\n"), "// \u8BA1\u6570\u5668\u81EA\u589E") {
			t.Errorf("its CJK neighbour was not decoded:\n%s", d.Diff)
		}
	})
}

func randomHighBytes(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(0x80 + (i*37+11)%0x7F)
	}
	return out
}

// F5-F11: frame shapes that must survive untouched

func TestDecodeFrameShapes(t *testing.T) {
	quietStdout(t)

	t.Run("F5_no_newline_at_end_of_file_is_frame", func(t *testing.T) {
		const name = "auth/token.go"
		old := gbkF16New + "// \u4EE4\u724C\u6821\u9A8C\n"
		new := gbkF16New + "// \u4EE4\u724C\u5DF2\u7ECF\u6821\u9A8C"
		repo, diffText := realDiff(t, name,
			encodeFixture(t, simplifiedchinese.GBK, old),
			encodeFixture(t, simplifiedchinese.GBK, new))
		if !strings.Contains(diffText, `\ No newline at end of file`) {
			t.Fatalf("precondition: git did not emit the no-newline marker:\n%s", diffText)
		}

		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		if !strings.Contains(diffs[0].Diff, `\ No newline at end of file`) {
			t.Errorf("the no-newline marker was decoded or dropped:\n%s", diffs[0].Diff)
		}
		if !strings.Contains(strings.Join(stripPayload(t, diffs[0].Diff), "\n"), "// \u4EE4\u724C\u5DF2\u7ECF\u6821\u9A8C") {
			t.Errorf("payload beside the marker was not decoded:\n%s", diffs[0].Diff)
		}
	})

	t.Run("F6_gb18030_four_byte_sequence_with_digit_bytes", func(t *testing.T) {
		// GB18030's four-byte form uses trail bytes in 0x30-0x39. None of them
		// is 0x2B/0x2D/0x20/0x0A, so the one-prefix-byte split still holds.
		const name = "auth/wide.go"
		text := "package auth\n\n// \u6269\u5C55\u5B57\u7B26 \U00020000 \u7684\u56DB\u5B57\u8282\u7F16\u7801\u3002\nfunc f() {}\n"
		raw := encodeFixture(t, simplifiedchinese.GB18030, text)
		if !bytes.ContainsAny(raw, "0123456789") {
			t.Fatal("precondition: fixture must contain bytes in 0x30-0x39")
		}
		repo, diffText := realDiff(t, name, []byte("package auth\n"), raw)

		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		if diffs[0].NewFileContent != text {
			t.Errorf("four-byte round-trip failed:\n got %q\nwant %q", diffs[0].NewFileContent, text)
		}
	})

	t.Run("F7_non_ascii_filename_headers_stay_raw", func(t *testing.T) {
		// The path is legacy bytes on disk. Header lines carry those bytes and
		// must come back byte-identical: we decode content, never filenames.
		rawName := string(encodeFixture(t, simplifiedchinese.GBK, "auth/\u4EE4\u724C.go"))
		dir := t.TempDir()
		writeRepoFile(t, dir, rawName, encodeFixture(t, simplifiedchinese.GBK, gbkFileNew))

		// rawName already holds legacy BYTES, so it cannot be run through the
		// encoder again; splice it into the frame instead.
		body := encodeFixture(t, simplifiedchinese.GBK, strings.Join([]string{
			"@@ -1,2 +1,2 @@",
			"-// \u65E7\u7684\u6CE8\u91CA\u3002",
			"+// \u65B0\u7684\u6CE8\u91CA\u3002",
		}, "\n"))
		var bb bytes.Buffer
		bb.WriteString("diff --git a/" + rawName + " b/" + rawName + "\n")
		bb.WriteString("--- a/" + rawName + "\n")
		bb.WriteString("+++ b/" + rawName + "\n")
		bb.Write(body)
		raw := bb.String()

		diffs, err := ParseDiffText(context.Background(), raw, dir, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		for _, want := range []string{
			"diff --git a/" + rawName + " b/" + rawName,
			"--- a/" + rawName,
			"+++ b/" + rawName,
		} {
			if !strings.Contains(diffs[0].Diff, want) {
				t.Errorf("header line was decoded; filenames must stay raw:\n%s", diffs[0].Diff)
			}
		}
	})

	t.Run("F8_rename_headers_untouched", func(t *testing.T) {
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
		raw := string(encodeFixture(t, simplifiedchinese.GBK, control))

		diffs, err := ParseDiffText(context.Background(), raw, dir, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if !d.IsRenamed || d.OldPath != "auth/old.go" || d.NewPath != "auth/new.go" {
			t.Errorf("rename metadata changed: renamed=%v %q -> %q", d.IsRenamed, d.OldPath, d.NewPath)
		}
		for _, want := range []string{"rename from auth/old.go", "rename to auth/new.go", "similarity index 90%"} {
			if !strings.Contains(d.Diff, want) {
				t.Errorf("%q was rewritten:\n%s", want, d.Diff)
			}
		}
	})

	t.Run("F9_deletion_decodes_from_its_own_payload", func(t *testing.T) {
		// No new-file bytes exist, so the hunk payload is the only evidence.
		dir := t.TempDir()
		control := strings.Join([]string{
			"diff --git a/auth/token.go b/auth/token.go",
			"deleted file mode 100644",
			"--- a/auth/token.go",
			"+++ /dev/null",
			"@@ -1,4 +1,0 @@",
			"-// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002",
			"-// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002",
			"-// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002",
			"-package auth",
		}, "\n")
		raw := string(encodeFixture(t, simplifiedchinese.GBK, control))

		diffs, err := ParseDiffText(context.Background(), raw, dir, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if !d.IsDeleted || d.NewPath != "/dev/null" {
			t.Fatalf("deletion metadata changed: deleted=%v newPath=%q", d.IsDeleted, d.NewPath)
		}
		if !strings.Contains(d.Diff, "-// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002") {
			t.Errorf("a deleted file's hunk was not decoded from its own payload:\n%s", d.Diff)
		}
	})

	t.Run("F10_empty_diff_text", func(t *testing.T) {
		diffs, err := ParseDiffText(context.Background(), "", t.TempDir(), "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		if len(diffs) != 0 {
			t.Errorf("got %d diffs for empty input, want 0", len(diffs))
		}
	})

	t.Run("F11_crlf_preserved", func(t *testing.T) {
		const name = "auth/token.go"
		text := "package auth\r\n// \u4EE4\u724C\u6821\u9A8C\u901A\u8FC7\u3002\r\nfunc f() {}\r\n"
		dir := t.TempDir()
		writeRepoFile(t, dir, name, encodeFixture(t, simplifiedchinese.GBK, text))
		control := buildDiff(name, "@@ -1,3 +1,3 @@",
			" package auth\r",
			"-// \u65E7\u7684\u6CE8\u91CA\u3002\r",
			"+// \u4EE4\u724C\u6821\u9A8C\u901A\u8FC7\u3002\r")
		raw := string(encodeFixture(t, simplifiedchinese.GBK, control))

		diffs, err := ParseDiffText(context.Background(), raw, dir, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if strings.Count(d.Diff, "\r") != strings.Count(control, "\r") {
			t.Errorf("CR count %d, want %d:\n%q", strings.Count(d.Diff, "\r"), strings.Count(control, "\r"), d.Diff)
		}
		if strings.Count(d.Diff, "\n") != strings.Count(control, "\n")-1 {
			// -1: the parser drops the "index " line this fixture does carry.
			t.Errorf("line count changed: %d vs %d", strings.Count(d.Diff, "\n"), strings.Count(control, "\n"))
		}
	})
}

// E3: a binary section is left completely alone and never reaches the detector.
func TestDecodeBinarySection(t *testing.T) {
	quietStdout(t)
	calls := countDetections(t)

	control := strings.Join([]string{
		"diff --git a/assets/logo.png b/assets/logo.png",
		"Binary files a/assets/logo.png and b/assets/logo.png differ",
	}, "\n")

	dir := t.TempDir()
	writeRepoFile(t, dir, "assets/logo.png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"))

	diffs, err := ParseDiffText(context.Background(), control, dir, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	d := diffs[0]
	if !d.IsBinary {
		t.Fatal("IsBinary was not set")
	}
	if d.Diff != control {
		t.Errorf("binary section was rewritten:\n got %q\nwant %q", d.Diff, control)
	}
	if d.UndecodedCharset != "" || d.Unreviewable {
		t.Errorf("binary must not be marked by the decoder (that is ExcludeBinary's job): %q/%v",
			d.UndecodedCharset, d.Unreviewable)
	}
	if *calls != 0 {
		t.Errorf("detector invoked %d times for a binary section, want 0", *calls)
	}
}

// D9 pins BOTH directions of the mixed-encoding limitation as executable fact.
// A legacy file containing one UTF-8 CJK line is decoded with the FILE's
// charset, because there is only ever one detection per file.
func TestDecodeMixedEncodingLimitation(t *testing.T) {
	quietStdout(t)

	// D9a: a realistic long UTF-8 CJK line inside a GBK file. Decoding it as
	// GB18030 gains U+FFFD, so the whole-file guard fires and both streams
	// revert to raw, marked. This is the common case and the guard catches it.
	t.Run("D9a_utf8_line_that_gains_replacement_reverts_the_file", func(t *testing.T) {
		const name = "auth/token.go"
		mixed := append(encodeFixture(t, simplifiedchinese.GBK, gbkF16New),
			[]byte("// \u3053\u306E\u30E2\u30B8\u30E5\u30FC\u30EB\u306F\u5229\u7528\u8005\u306E\u8A8D\u8A3C\u60C5\u5831\u3092\u691C\u8A3C\u3057\u3001\u30BB\u30C3\u30B7\u30E7\u30F3\u3092\u66F4\u65B0\u3059\u308B\u8CAC\u52D9\u3092\u6301\u3061\u307E\u3059\u3002\n")...)
		repo, diffText := realDiff(t, name,
			encodeFixture(t, simplifiedchinese.GBK, gbkF16Old), mixed)

		diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
		if err != nil {
			t.Fatalf("ParseDiffText: %v", err)
		}
		d := diffs[0]
		if d.NewFileContent != string(mixed) {
			t.Errorf("the U+FFFD-gain guard did not revert the file")
		}
		if d.UndecodedCharset == "" {
			t.Error("a reverted file must still be marked")
		}
		// Its raw bytes are ~30% U+FFFD once treated as UTF-8, so the file is
		// genuinely past reviewing and is excluded with a named reason rather
		// than sent to the model as soup. That is the point of the threshold:
		// a mostly-ASCII file with a few stray bytes stays reviewed (D8/D11a),
		// a CJK-dense file we could not decode does not.
		if !d.Unreviewable {
			t.Errorf("a CJK-dense file we failed to decode must be excluded, not reviewed as U+FFFD soup")
		}
	})

	// D9b: the dangerous mirror. This specific Japanese comment decodes from
	// GB18030 with a U+FFFD gain of ZERO, so the guard cannot see it and the
	// line becomes wrong-but-plausible CJK. Asserted verbatim so the limitation
	// is a recorded fact rather than prose in a commit message.
	t.Run("D9b_zero_gain_utf8_line_becomes_plausible_wrong_cjk", func(t *testing.T) {
		const line = "// \u65E5\u672C\u8A9E\u306E\u30B3\u30E1\u30F3\u30C8\u3067\u3059"
		decoded, ok := textenc.Convert("GB-18030", []byte(line))
		if !ok {
			t.Fatalf("precondition: this line is the measured ZERO-gain case; "+
				"if the guard now catches it, D9b has nothing left to pin (got %q)", decoded)
		}
		if decoded == line {
			t.Fatal("precondition: the line must actually change under a GB18030 decode")
		}
		if strings.ContainsRune(decoded, utf8.RuneError) {
			t.Fatalf("precondition: gain must be zero, got %q", decoded)
		}
		if decoded != "// \u93C3\u30E6\u6E70\u747E\u70AA\u4F04\u9288\u70BD\u512D\u9289\u70BD\u5113\u9287\u0441\u4EDA" {
			t.Errorf("the measured wrong-but-plausible decode changed: got %q", decoded)
		}
	})
}

// C1-C6 at the review seam: every charset in the allowlist
//
// textenc's own C-table proves Detect+Convert round-trip all six allowlisted
// charsets, but that is a unit test of two functions. Before this table, only
// GBK and GB18030 ever reached ParseDiffText: Big5, Shift_JIS, EUC-JP and
// EUC-KR were decoded nowhere near the diff seam, so anything charset-specific
// on this path — a trail byte the one-prefix-byte split cuts in half, a hunk
// that scores below the gate for one language and not another — would have
// shipped unnoticed for four of the six.

// docSimplified, docTraditional, docJapanese and docKorean are the CJK-dense
// header comments each row's fixture carries. Density is the point: the
// detector scores the whole file, and a fixture with one short comment scores
// around 50 and is left raw, which would make its row pass vacuously.
const (
	docSimplified = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
		"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
		"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n"

	docTraditional = "// \u4F7F\u7528\u8005\u8A8D\u8B49\u6A21\u7D44\uFF0C\u8CA0\u8CAC\u6AA2\u9A57\u6B0A\u6756\u4E26\u66F4\u65B0\u5DE5\u4F5C\u968E\u6BB5\u72C0\u614B\u3002\n" +
		"// \u82E5\u6B0A\u6756\u5DF2\u7D93\u903E\u671F\uFF0C\u547C\u53EB\u7AEF\u5FC5\u9808\u91CD\u65B0\u767B\u5165\u53D6\u5F97\u65B0\u7684\u6191\u8B49\u3002\n" +
		"// \u672C\u6A94\u6848\u4F7F\u7528\u50B3\u7D71\u7DE8\u78BC\u5132\u5B58\uFF0C\u5BE9\u67E5\u5DE5\u5177\u5FC5\u9808\u5148\u89E3\u78BC\u518D\u95B1\u8B80\u3002\n"

	docJapanese = "// \u5229\u7528\u8005\u8A8D\u8A3C\u30E2\u30B8\u30E5\u30FC\u30EB\u3067\u3059\u3002\u30C8\u30FC\u30AF\u30F3\u3092\u691C\u8A3C\u3057\u3066\u3001\u30BB\u30C3\u30B7\u30E7\u30F3\u72B6\u614B\u3092\u66F4\u65B0\u3057\u307E\u3059\u3002\n" +
		"// \u30C8\u30FC\u30AF\u30F3\u306E\u6709\u52B9\u671F\u9650\u304C\u5207\u308C\u3066\u3044\u308B\u5834\u5408\u306F\u3001\u518D\u5EA6\u30ED\u30B0\u30A4\u30F3\u3057\u3066\u65B0\u3057\u3044\u8CC7\u683C\u60C5\u5831\u3092\u53D6\u5F97\u3057\u3066\u304F\u3060\u3055\u3044\u3002\n" +
		"// \u3053\u306E\u30D5\u30A1\u30A4\u30EB\u306F\u5F93\u6765\u306E\u6587\u5B57\u30B3\u30FC\u30C9\u3067\u4FDD\u5B58\u3055\u308C\u3066\u3044\u307E\u3059\u3002\u30EC\u30D3\u30E5\u30FC\u524D\u306B\u5FC5\u305A\u5909\u63DB\u3057\u3066\u304F\u3060\u3055\u3044\u3002\n"

	docKorean = "// \uC0AC\uC6A9\uC790 \uC778\uC99D \uBAA8\uB4C8\uC785\uB2C8\uB2E4. \uD1A0\uD070\uC744 \uAC80\uC99D\uD558\uACE0 \uC138\uC158 \uC0C1\uD0DC\uB97C \uAC31\uC2E0\uD569\uB2C8\uB2E4.\n" +
		"// \uD1A0\uD070\uC774 \uB9CC\uB8CC\uB41C \uACBD\uC6B0\uC5D0\uB294 \uB2E4\uC2DC \uB85C\uADF8\uC778\uD558\uC5EC \uC0C8\uB85C\uC6B4 \uC790\uACA9 \uC99D\uBA85\uC744 \uBC1B\uC544\uC57C \uD569\uB2C8\uB2E4.\n" +
		"// \uC774 \uD30C\uC77C\uC740 \uC804\uD1B5 \uBB38\uC790 \uC778\uCF54\uB529\uC73C\uB85C \uC800\uC7A5\uB418\uC5C8\uC73C\uBBC0\uB85C \uAC80\uD1A0 \uB3C4\uAD6C\uAC00 \uBA3C\uC800 \uBCC0\uD658\uD574\uC57C \uD569\uB2C8\uB2E4.\n"
)

// legacyFile wraps CJK doc comments around a realistic ASCII function body,
// with one comment line inside the body that the fixture pair changes. The
// ASCII is deliberate: a file that is nothing but CJK would detect trivially
// and would not resemble the source this tool reviews.
func legacyFile(doc, comment string) string {
	return doc +
		"package auth\n" +
		"\n" +
		"import \"strings\"\n" +
		"\n" +
		"func Validate(tok string) (string, error) {\n" +
		"\tif tok == \"\" {\n" +
		"\t\treturn \"\", errEmptyToken\n" +
		"\t}\n" +
		comment + "\n" +
		"\treturn lookup(strings.TrimSpace(tok))\n" +
		"}\n"
}

// allowlistedCharsets mirrors textenc's decoders map, one row per entry, in
// the language each charset is actually used for. The charset field is the
// label textenc.Detect must return, not the encoder's name: GBK and GB18030
// both come back as chardet's "GB-18030".
var allowlistedCharsets = []struct {
	name       string
	enc        encoding.Encoding
	charset    string
	doc        string
	oldC, newC string
}{
	{"C1_gbk", simplifiedchinese.GBK, "GB-18030", docSimplified,
		"\t// \u6821\u9A8C\u901A\u8FC7", "\t// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7"},
	// The four-byte form: trail bytes land in 0x30-0x39, none of which is a
	// diff prefix byte, so the payload split must still hold.
	{"C2_gb18030_four_byte", simplifiedchinese.GB18030, "GB-18030",
		docSimplified + "// \u6269\u5C55\u5B57\u7B26 \U00020000 \u5F80\u8FD4\u3002\n",
		"\t// \u6821\u9A8C\u901A\u8FC7", "\t// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7"},
	{"C3_big5", traditionalchinese.Big5, "Big5", docTraditional,
		"\t// \u6AA2\u9A57\u901A\u904E", "\t// \u5DF2\u7D93\u6AA2\u9A57\u901A\u904E"},
	{"C4_shift_jis", japanese.ShiftJIS, "Shift_JIS", docJapanese,
		"\t// \u30C8\u30FC\u30AF\u30F3\u691C\u8A3C", "\t// \u30C8\u30FC\u30AF\u30F3\u691C\u8A3C\u6E08\u307F"},
	{"C5_euc_jp", japanese.EUCJP, "EUC-JP", docJapanese,
		"\t// \u30C8\u30FC\u30AF\u30F3\u691C\u8A3C", "\t// \u30C8\u30FC\u30AF\u30F3\u691C\u8A3C\u6E08\u307F"},
	{"C6_euc_kr", korean.EUCKR, "EUC-KR", docKorean,
		"\t// \uD1A0\uD070 \uAC80\uC99D", "\t// \uD1A0\uD070 \uAC80\uC99D \uC644\uB8CC"},
}

// H6 used to be a GBK-only case. It is the only place where raw bytes actually
// become U+FFFD — json.Marshal of the prompt payload. A Go string holding raw
// legacy bytes contains no U+FFFD *rune*, so "no U+FFFD in d.Diff" passes at
// HEAD and proves nothing; the marshalled bytes are what the model receives.
// Running it per charset makes one table carry both properties: every charset
// in the allowlist survives ParseDiffText, and it survives as far as the wire.
func TestDecodeEveryAllowlistedCharsetSurvivesToThePromptPayload(t *testing.T) {
	quietStdout(t)
	const name = "auth/token.go"

	for _, tc := range allowlistedCharsets {
		t.Run(tc.name, func(t *testing.T) {
			oldText := legacyFile(tc.doc, tc.oldC)
			newText := legacyFile(tc.doc, tc.newC)
			rawNew := encodeFixture(t, tc.enc, newText)

			// Precondition: the row exercises the charset it claims. A fixture
			// that drifted below the confidence gate would be left raw, and
			// every assertion below would then be comparing raw bytes to raw
			// bytes — green, and vacuous.
			cs, conf, ok := textenc.Detect(rawNew)
			if !ok || cs != tc.charset {
				t.Fatalf("fixture detected as (%q, %d, %v), want (%q, >=90, true)",
					cs, conf, ok, tc.charset)
			}

			repo, diffText := realDiff(t, name, encodeFixture(t, tc.enc, oldText), rawNew)
			diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
			if err != nil {
				t.Fatalf("ParseDiffText: %v", err)
			}
			if len(diffs) != 1 {
				t.Fatalf("got %d diffs, want 1", len(diffs))
			}
			d := diffs[0]

			if d.UndecodedCharset != "" || d.Unreviewable {
				t.Fatalf("the file was skipped, not decoded: charset %q unreviewable %v",
					d.UndecodedCharset, d.Unreviewable)
			}
			if d.NewFileContent != newText {
				t.Errorf("NewFileContent is not the UTF-8 control:\n got %q\nwant %q",
					d.NewFileContent, newText)
			}
			body := strings.Join(stripPayload(t, d.Diff), "\n")
			for _, want := range []string{tc.oldC, tc.newC} {
				if !strings.Contains(body, want) {
					t.Errorf("hunk payload is missing %q:\n%q", want, body)
				}
			}

			decoded, err := json.Marshal(d.Diff)
			if err != nil {
				t.Fatalf("marshal decoded: %v", err)
			}
			if bytes.ContainsRune(decoded, utf8.RuneError) || bytes.Contains(decoded, []byte(`\ufffd`)) {
				t.Errorf("the marshalled prompt payload still carries U+FFFD:\n%s", decoded)
			}

			// Falsifiability: the same diff without decoding must fail the
			// assertion above, otherwise this row cannot fail.
			raw, err := json.Marshal(diffText)
			if err != nil {
				t.Fatalf("marshal raw: %v", err)
			}
			if !bytes.ContainsRune(raw, utf8.RuneError) && !bytes.Contains(raw, []byte(`\ufffd`)) {
				t.Fatal("the undecoded control marshalled cleanly, so this row cannot fail")
			}
		})
	}
}

// splitHunkPayload / rebuild are the two halves of the guarantee that decoding
// cannot reshape a diff. These drive them directly, including the guard that
// has no natural fixture: a decode that changes the line count.
func TestSplitHunkPayloadAndRebuild(t *testing.T) {
	section := strings.Join([]string{
		"diff --git a/a.go b/a.go",
		"--- a/a.go",
		"+++ b/a.go",
		"@@ -1,3 +1,3 @@",
		" ctx",
		"-old",
		"+new",
		`\ No newline at end of file`,
	}, "\n")

	t.Run("payload_excludes_every_frame_line", func(t *testing.T) {
		payload, f := splitHunkPayload(section)
		if payload != "ctx\nold\nnew" {
			t.Errorf("payload = %q, want %q", payload, "ctx\nold\nnew")
		}
		if got := string(f.prefix); got != " -+" {
			t.Errorf("prefixes = %q, want %q", got, " -+")
		}
		// The no-newline marker starts with 0x5C, so the first-byte rule drops
		// it into the frame without any text match.
		if len(f.idx) != 3 {
			t.Errorf("got %d payload lines, want 3", len(f.idx))
		}
	})

	t.Run("rebuild_round_trips_unchanged_payload", func(t *testing.T) {
		payload, f := splitHunkPayload(section)
		got, ok := f.rebuild(payload)
		if !ok {
			t.Fatal("rebuild refused an unchanged payload")
		}
		if got != section {
			t.Errorf("round trip changed the section:\n got %q\nwant %q", got, section)
		}
	})

	// The guard that matters: a decode that added or dropped a line would make
	// the diff describe a file it no longer matches. Refuse and keep it raw.
	t.Run("rebuild_refuses_a_line_count_change", func(t *testing.T) {
		_, f := splitHunkPayload(section)
		if _, ok := f.rebuild("ctx\nold\nnew\nextra"); ok {
			t.Error("rebuild accepted a payload with an extra line")
		}
		if _, ok := f.rebuild("ctx\nold"); ok {
			t.Error("rebuild accepted a payload with a missing line")
		}
	})

	t.Run("section_with_no_hunk_is_all_frame", func(t *testing.T) {
		bare := "diff --git a/a.go b/a.go\nBinary files a/a.go and b/a.go differ"
		payload, f := splitHunkPayload(bare)
		if payload != "" {
			t.Errorf("payload = %q, want empty", payload)
		}
		got, ok := f.rebuild("")
		if !ok || got != bare {
			t.Errorf("rebuild = (%q, %v), want the section unchanged", got, ok)
		}
	})
}

// multi-file: one diff, one charset per file

// repoFile is one file's before/after bytes for realMultiDiff.
type repoFile struct {
	name     string
	old, new []byte
}

// realMultiDiff is realDiff for several files at once: one repo, one commit,
// one `git diff`, so the sections arrive concatenated exactly as git emits
// them — each in its own encoding, inside a single byte stream.
func realMultiDiff(t *testing.T, files ...repoFile) (string, string) {
	t.Helper()
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	for _, f := range files {
		writeRepoFile(t, repo, f.name, f.old)
	}
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-q", "-m", "initial")
	for _, f := range files {
		writeRepoFile(t, repo, f.name, f.new)
	}

	out, err := gitCapture(t, repo, "diff", "--no-ext-diff")
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	return repo, out
}

// The issue requires that one repository may hold UTF-8 and legacy files at the
// same time. Every other case here is a single-file diff, which cannot see the
// per-file boundary: a bug that carried the first file's charset over to the
// next, or detected once for the whole diff text, would pass all of them. This
// drives three files with three different encodings through ONE ParseDiffText
// call and pins each result against its own control.
func TestDecodeMultiFileMixedEncodings(t *testing.T) {
	quietStdout(t)
	calls := countDetections(t)

	utf8Text := legacyFile(docJapanese, "\t// \u30C8\u30FC\u30AF\u30F3\u691C\u8A3C\u6E08\u307F")
	gbkText := legacyFile(docSimplified, "\t// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7")
	big5Text := legacyFile(docTraditional, "\t// \u5DF2\u7D93\u6AA2\u9A57\u901A\u904E")

	repo, diffText := realMultiDiff(t,
		repoFile{"auth/clean.go", []byte("package auth\n"), []byte(utf8Text)},
		repoFile{"auth/gbk.go", []byte("package auth\n"),
			encodeFixture(t, simplifiedchinese.GBK, gbkText)},
		repoFile{"auth/big5.go", []byte("package auth\n"),
			encodeFixture(t, traditionalchinese.Big5, big5Text)})

	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 3 {
		t.Fatalf("got %d diffs, want 3", len(diffs))
	}
	byPath := map[string]model.Diff{}
	for _, d := range diffs {
		byPath[d.NewPath] = d
	}

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"utf8_file_is_untouched", "auth/clean.go", utf8Text},
		{"gbk_file_decodes_with_its_own_charset", "auth/gbk.go", gbkText},
		{"big5_file_decodes_with_its_own_charset", "auth/big5.go", big5Text},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := byPath[tc.path]
			if !ok {
				t.Fatalf("%s is missing from the parsed diffs", tc.path)
			}
			if d.UndecodedCharset != "" || d.Unreviewable {
				t.Fatalf("file was skipped, not decoded: charset %q unreviewable %v",
					d.UndecodedCharset, d.Unreviewable)
			}
			// Equality, not containment: a neighbour's charset applied to these
			// bytes still produces CJK, and only an exact match against this
			// file's own control rules that out.
			if d.NewFileContent != tc.want {
				t.Errorf("NewFileContent is not this file's control:\n got %q\nwant %q",
					d.NewFileContent, tc.want)
			}
			body := strings.Join(stripPayload(t, d.Diff), "\n")
			if !strings.Contains(body, strings.TrimSuffix(tc.want, "\n")) {
				t.Errorf("hunk payload is not the whole decoded file:\n%q", body)
			}
		})
	}

	// The other half of the boundary: detection is per file, and the UTF-8 file
	// pays nothing. A design that detected once per diff text would score 1
	// here, and one that re-detected per stream would score more than 2.
	t.Run("one_detection_per_legacy_file_and_none_for_utf8", func(t *testing.T) {
		if *calls != 2 {
			t.Errorf("detector invoked %d times for 2 legacy files + 1 UTF-8 file, want 2", *calls)
		}
	})
}

// working-tree safety

// "Decoding must not modify files in the working tree" is an issue-level
// requirement, and decoding in memory is the whole design. internal/scan pins
// it for the enumerate seam; ParseDiffText is the other reader and it touches
// disk on two different branches — readWorkspaceFileForDiff when ref is empty,
// `git show` when it is not — so both are driven over the same legacy repo and
// every file is hashed before and after.
func TestParseDiffTextNeverWritesToTheWorkingTree(t *testing.T) {
	out := quietStdout(t)

	gbkOld := legacyFile(docSimplified, "\t// \u6821\u9A8C\u901A\u8FC7")
	gbkNew := legacyFile(docSimplified, "\t// \u5DF2\u7ECF\u6821\u9A8C\u901A\u8FC7")
	big5Old := legacyFile(docTraditional, "\t// \u6AA2\u9A57\u901A\u904E")
	big5New := legacyFile(docTraditional, "\t// \u5DF2\u7D93\u6AA2\u9A57\u901A\u904E")

	repo, _ := realMultiDiff(t,
		repoFile{"auth/gbk.go", encodeFixture(t, simplifiedchinese.GBK, gbkOld),
			encodeFixture(t, simplifiedchinese.GBK, gbkNew)},
		repoFile{"auth/big5.go", encodeFixture(t, traditionalchinese.Big5, big5Old),
			encodeFixture(t, traditionalchinese.Big5, big5New)},
		repoFile{"auth/clean.go", []byte("package auth\n"), []byte("package auth\n\nfunc a() {}\n")})
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-q", "-m", "second")

	sha, err := gitCapture(t, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha = strings.TrimSpace(sha)
	diffText, err := gitCapture(t, repo, "diff", "--no-ext-diff", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}

	before := hashWorkTree(t, repo)
	if len(before) != 3 {
		t.Fatalf("hashed %d working-tree files, want 3", len(before))
	}

	for _, mode := range []struct {
		name string
		ref  string
	}{
		{"workspace_mode_reads_from_disk", ""},
		{"ref_mode_reads_through_git_show", sha},
	} {
		t.Run(mode.name, func(t *testing.T) {
			mark := out.Len()
			if _, err := ParseDiffText(context.Background(), diffText, repo, mode.ref, nil); err != nil {
				t.Fatalf("ParseDiffText: %v", err)
			}
			// Without this the sha256 comparison below would prove nothing: a
			// run that decoded nothing also writes nothing.
			if !strings.Contains(out.String()[mark:], "[ocr] decoded ") {
				t.Fatal("this mode decoded nothing, so it cannot show that decoding leaves the tree alone")
			}
		})
	}

	after := hashWorkTree(t, repo)
	if len(before) != len(after) {
		t.Fatalf("working-tree file count changed: %d -> %d", len(before), len(after))
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Errorf("%s was modified on disk by ParseDiffText", name)
		}
	}
}

// hashWorkTree sha256s every file in the working tree, keyed by relative path.
// .git is skipped on purpose: git's own metadata is not the working tree, and
// running git at all can touch the index and its caches.
func hashWorkTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}
