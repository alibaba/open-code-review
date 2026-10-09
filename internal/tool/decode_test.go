// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// Fixture text is ASCII \uXXXX literals encoded at test time, so the legacy
// bytes and the UTF-8 control come from one constant.
const toolGBKFile = "// \u7528\u6237\u8BA4\u8BC1\u6A21\u5757\uFF0C\u8D1F\u8D23\u6821\u9A8C\u4EE4\u724C\u5E76\u5237\u65B0\u4F1A\u8BDD\u72B6\u6001\u3002\n" +
	"// \u5982\u679C\u4EE4\u724C\u5DF2\u7ECF\u8FC7\u671F\uFF0C\u8C03\u7528\u65B9\u9700\u8981\u91CD\u65B0\u767B\u5F55\u83B7\u53D6\u65B0\u7684\u51ED\u8BC1\u3002\n" +
	"// \u672C\u6587\u4EF6\u4F7F\u7528\u4F20\u7EDF\u7F16\u7801\u4FDD\u5B58\uFF0C\u8BC4\u5BA1\u5DE5\u5177\u5FC5\u987B\u5148\u89E3\u7801\u518D\u9605\u8BFB\u3002\n" +
	"package auth\n" +
	"\n" +
	"func ValidateToken(tok string) error { // \u6821\u9A8C\u4EE4\u724C\n" +
	"\t// \u6821\u9A8C\u4EE4\u724C\u662F\u5426\u4E3A\u7A7A\n" +
	"\tif tok == \"\" {\n" +
	"\t\treturn errEmptyToken\n" +
	"\t}\n" +
	"\treturn nil\n" +
	"}\n"

func encodeToolFixture(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, _, err := transform.Bytes(enc.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return out
}

func writeToolFile(t *testing.T, dir, name string, raw []byte) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func gitInTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// bodiesByLine parses a file_read / code_search "N|body" block into a map.
func bodiesByLine(t *testing.T, out string) map[int]string {
	t.Helper()
	got := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		bar := strings.Index(line, "|")
		if bar <= 0 {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(line[:bar], "%d", &n); err != nil {
			continue
		}
		got[n] = line[bar+1:]
	}
	return got
}

// assertBodiesMatchControl checks every emitted "N|body" against line N of the
// UTF-8 control. Equality, not "contains": raw bytes are a substring of raw
// bytes, so a containment check would pass without any decoding at all.
func assertBodiesMatchControl(t *testing.T, out, control string) {
	t.Helper()
	want := strings.Split(control, "\n")
	got := bodiesByLine(t, out)
	if len(got) == 0 {
		t.Fatalf("no \"N|body\" lines in output:\n%s", out)
	}
	for n, body := range got {
		if n < 1 || n > len(want) {
			t.Errorf("line %d is outside the control (%d lines)", n, len(want))
			continue
		}
		if body != want[n-1] {
			t.Errorf("line %d body = %q, want %q", n, body, want[n-1])
		}
	}
}

// H2: the live file_read path in workspace mode.
func TestFileReadDecodesWorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	writeToolFile(t, dir, "auth/token.go", encodeToolFixture(t, simplifiedchinese.GBK, toolGBKFile))

	restore := stdout.Swap(io.Discard)
	defer restore()

	p := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeWorkspace})
	out, err := p.Execute(context.Background(), map[string]any{"file_path": "auth/token.go"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	assertBodiesMatchControl(t, out, toolGBKFile)
	wantTotal := strings.Count(toolGBKFile, "\n") + 1
	if !strings.Contains(out, fmt.Sprintf("Total lines: %d", wantTotal)) {
		t.Errorf("Total lines is not the control's %d:\n%s", wantTotal, out)
	}
}

// H2b: ref mode. ModeRange and ModeCommit send ReadLines down
// readLinesFromGitShow, which has two branches — a Runner.Stream callback and a
// plain exec + StdoutPipe. Neither is reached by any other case, so both run.
func TestFileReadDecodesAtRef(t *testing.T) {
	dir := t.TempDir()
	gitInTest(t, dir, "init", "-q")
	gitInTest(t, dir, "config", "user.email", "test@example.com")
	gitInTest(t, dir, "config", "user.name", "Test User")
	gitInTest(t, dir, "config", "commit.gpgsign", "false")
	writeToolFile(t, dir, "auth/token.go", encodeToolFixture(t, simplifiedchinese.GBK, toolGBKFile))
	gitInTest(t, dir, "add", "auth/token.go")
	gitInTest(t, dir, "commit", "-q", "-m", "initial")
	sha := gitInTest(t, dir, "rev-parse", "HEAD")

	// Replace the working copy so anything read from disk would be obviously
	// wrong: only `git show` can produce the fixture now.
	writeToolFile(t, dir, "auth/token.go", []byte("package auth // replaced\n"))

	restore := stdout.Swap(io.Discard)
	defer restore()

	for _, tc := range []struct {
		id     string
		runner *gitcmd.Runner
	}{
		{"H2b_runner_stream_branch", gitcmd.New(2)},
		{"H2b_exec_stdout_pipe_branch", nil},
	} {
		t.Run(tc.id, func(t *testing.T) {
			p := NewFileRead(&FileReader{
				RepoDir: dir, Mode: ModeCommit, Ref: sha, Runner: tc.runner,
			})
			out, err := p.Execute(context.Background(), map[string]any{"file_path": "auth/token.go"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			assertBodiesMatchControl(t, out, toolGBKFile)
		})
	}
}

// H2c: a file above decodeMaxBytes keeps the streaming path. It must still
// return the requested window and the correct total, with its bytes raw.
func TestFileReadAboveByteCapStillStreams(t *testing.T) {
	dir := t.TempDir()
	line := "// padding line to push this file past the decode byte cap\n"
	big := strings.Repeat(line, (decodeMaxBytes/len(line))+64)
	if len(big) <= decodeMaxBytes {
		t.Fatalf("precondition: fixture is %d bytes, needs > %d", len(big), decodeMaxBytes)
	}
	writeToolFile(t, dir, "big.go", []byte(big))

	restore := stdout.Swap(io.Discard)
	defer restore()

	p := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeWorkspace})
	out, err := p.Execute(context.Background(), map[string]any{
		"file_path": "big.go", "start_line": float64(2), "end_line": float64(4),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantTotal := strings.Count(big, "\n") + 1
	if !strings.Contains(out, fmt.Sprintf("Total lines: %d", wantTotal)) {
		t.Errorf("Total lines wrong for a file past the cap:\n%s", firstLines(out, 4))
	}
	got := bodiesByLine(t, out)
	if len(got) != 3 {
		t.Errorf("got %d lines, want the 3 requested", len(got))
	}
	for n, body := range got {
		if body != strings.TrimSuffix(line, "\n") {
			t.Errorf("line %d = %q, want the padding line", n, body)
		}
	}
}

func firstLines(s string, n int) string {
	parts := strings.SplitN(s, "\n", n+1)
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}

// H5: code_search. A CJK search term can never match a legacy-encoded file:
// git grep matches raw bytes and the term arrives as UTF-8. So this searches
// the ASCII identifier on line 6 of the fixture, which is also the one line
// that carries a trailing GBK comment. That trailing comment is what lets this
// test fail at all. git grep prints the matched line raw, so the body arrives
// non-UTF-8, decodeMatches sees needsDecode and re-reads the file. Move the
// identifier to a pure-ASCII line and decodeMatches early-returns instead,
// which makes the whole test pass with the decode stubbed out.
//
// The match count is asserted FIRST: a zero-match run makes every later
// assertion vacuous.
func TestCodeSearchDecodesMatchBodies(t *testing.T) {
	dir := t.TempDir()
	gitInTest(t, dir, "init", "-q")
	gitInTest(t, dir, "config", "user.email", "test@example.com")
	gitInTest(t, dir, "config", "user.name", "Test User")
	gitInTest(t, dir, "config", "commit.gpgsign", "false")
	writeToolFile(t, dir, "auth/token.go", encodeToolFixture(t, simplifiedchinese.GBK, toolGBKFile))
	gitInTest(t, dir, "add", "auth/token.go")
	gitInTest(t, dir, "commit", "-q", "-m", "initial")

	restore := stdout.Swap(io.Discard)
	defer restore()

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	out, err := NewCodeSearch(fr).Execute(context.Background(), map[string]any{
		"search_text": "ValidateToken",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "Match lines: 1") {
		t.Fatalf("expected exactly one match; a zero-match run proves nothing:\n%s", out)
	}
	assertBodiesMatchControl(t, out, toolGBKFile)
}

// H5b: the same live path with the match on LINE 1. git grep numbers lines
// from 1, and the substitution loop indexes lines[n-1], so line 1 sits exactly
// on the lower bound of that bounds check — an off-by-one there leaves the
// first line of every legacy file as mojibake while every later line decodes.
// Line 1 is the case that matters in practice: these files carry their CJK
// header comment there.
//
// The search term is the ASCII comment marker, because no CJK term can match:
// git grep compares raw bytes and the term arrives as UTF-8. Line 1 of the
// fixture is comment marker plus CJK, so its body arrives non-UTF-8 and
// decodeMatches has real work to do.
func TestCodeSearchDecodesMatchOnFirstLine(t *testing.T) {
	dir := t.TempDir()
	gitInTest(t, dir, "init", "-q")
	gitInTest(t, dir, "config", "user.email", "test@example.com")
	gitInTest(t, dir, "config", "user.name", "Test User")
	gitInTest(t, dir, "config", "commit.gpgsign", "false")
	writeToolFile(t, dir, "auth/token.go", encodeToolFixture(t, simplifiedchinese.GBK, toolGBKFile))
	gitInTest(t, dir, "add", "auth/token.go")
	gitInTest(t, dir, "commit", "-q", "-m", "initial")

	restore := stdout.Swap(io.Discard)
	defer restore()

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	out, err := NewCodeSearch(fr).Execute(context.Background(), map[string]any{
		"search_text": "//",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Asserted first: without a hit on line 1 the rest of this test is vacuous.
	got := bodiesByLine(t, out)
	body, ok := got[1]
	if !ok {
		t.Fatalf("no match reported on line 1:\n%s", out)
	}
	if want := strings.Split(toolGBKFile, "\n")[0]; body != want {
		t.Errorf("line 1 body = %q, want the decoded header %q", body, want)
	}
	assertBodiesMatchControl(t, out, toolGBKFile)
}

// When every match body is already valid UTF-8 the provider must not re-read
// the file at all — the common case pays one utf8.ValidString per match line.
//
// The file on disk deliberately holds DIFFERENT text from the match body, so a
// re-read succeeds and visibly overwrites it. Pointing the FileReader at an
// empty directory would not discriminate: decodeMatches bails out on a failed
// re-read and leaves the raw bytes in place, which is exactly what this asserts.
func TestCodeSearchSkipsRereadForUTF8Matches(t *testing.T) {
	const matched = "func ValidateToken() {}"
	const onDisk = "func SomethingElse() {}"

	dir := t.TempDir()
	writeToolFile(t, dir, "auth/token.go", []byte(onDisk+"\n"))
	p := &CodeSearchProvider{FileReader: &FileReader{RepoDir: dir, Mode: ModeWorkspace}}

	// Precondition: the re-read this test forbids would really change the body.
	lines, _, err := p.FileReader.ReadLines(context.Background(), "auth/token.go", 1, 1)
	if err != nil || len(lines) != 1 || lines[0] != onDisk {
		t.Fatalf("precondition: a re-read must return %q, got %q (err %v)", onDisk, lines, err)
	}

	matches := []grepMatch{{lineNum: 1, content: matched}}
	p.decodeMatches(context.Background(), "auth/token.go", matches)
	if matches[0].content != matched {
		t.Errorf("the body was replaced by a re-read: got %q, want %q", matches[0].content, matched)
	}
}

// A re-read that cannot be trusted must leave EVERY match for that file raw: a
// search result is never worth an error, and raw bytes are what ships today.
//
// The two rows are the two independent halves of that guard. A read error
// returns no lines at all. A re-read that succeeds but comes back SHORTER than
// the highest matched line number means the file changed since git grep ran, so
// every line number in the batch now names different text — including the
// in-range ones, which is why the bail is per file and not per match.
func TestDecodeMatchesBailsOutOnUntrustedReread(t *testing.T) {
	dir := t.TempDir()
	writeToolFile(t, dir, "auth/token.go", encodeToolFixture(t, simplifiedchinese.GBK, toolGBKFile))
	raw := string(encodeToolFixture(t, simplifiedchinese.GBK, "// \u4EE4\u724C\u6821\u9A8C\u5931\u8D25"))

	restore := stdout.Swap(io.Discard)
	defer restore()

	for _, tc := range []struct {
		id       string
		path     string
		lineNums []int
	}{
		{"read_error_returns_no_lines", "missing.go", []int{9999}},
		// Line 1 exists in the file on disk and would decode cleanly on its
		// own, so only the whole-file bail keeps it raw.
		{"short_read_leaves_in_range_matches_raw", "auth/token.go", []int{1, 9999}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			matches := make([]grepMatch, len(tc.lineNums))
			for i, n := range tc.lineNums {
				matches[i] = grepMatch{lineNum: n, content: raw}
			}

			p := &CodeSearchProvider{FileReader: &FileReader{RepoDir: dir, Mode: ModeWorkspace}}
			p.decodeMatches(context.Background(), tc.path, matches)

			for i, m := range matches {
				if m.content != raw {
					t.Errorf("match %d (line %d) = %q, want the raw bytes left in place",
						i, tc.lineNums[i], m.content)
				}
			}
		})
	}
}

// A nil FileReader must not panic.
func TestDecodeMatchesWithoutFileReader(t *testing.T) {
	matches := []grepMatch{{lineNum: 1, content: "\x81\x40"}}
	(&CodeSearchProvider{}).decodeMatches(context.Background(), "x.go", matches)
	if matches[0].content != "\x81\x40" {
		t.Error("content changed with no FileReader available")
	}
}

// A malformed line number arrives from a subprocess, so it must never be used
// as an index without a bounds check.
func TestDecodeMatchesIgnoresOutOfRangeLineNumbers(t *testing.T) {
	dir := t.TempDir()
	writeToolFile(t, dir, "auth/token.go", encodeToolFixture(t, simplifiedchinese.GBK, toolGBKFile))

	restore := stdout.Swap(io.Discard)
	defer restore()

	bad := string(encodeToolFixture(t, simplifiedchinese.GBK, "// \u4EE4\u724C\u6821\u9A8C\u5931\u8D25"))
	matches := []grepMatch{
		{lineNum: 0, content: bad},
		{lineNum: 7, content: bad},
	}
	p := &CodeSearchProvider{FileReader: &FileReader{RepoDir: dir, Mode: ModeWorkspace}}
	p.decodeMatches(context.Background(), "auth/token.go", matches)

	if matches[0].content != bad {
		t.Error("a line number of 0 must leave the body untouched, not index out of range")
	}
	if want := strings.Split(toolGBKFile, "\n")[6]; matches[1].content != want {
		t.Errorf("valid match body = %q, want %q", matches[1].content, want)
	}
}
