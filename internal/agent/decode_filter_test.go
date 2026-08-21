// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// captureStderr runs fn with os.Stderr replaced by a pipe and returns what was
// written. Same os.Pipe swap pattern internal/session/persist_test.go uses for
// os.Stdout, applied to os.Stderr — the warning this feature emits goes there,
// because it must survive --audience agent, which silences stdout.
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

// TestFilterDiffsEncodingReporting pins the mark-versus-exclude split at the
// filter, which is where both decisions become visible to a user.
//
// No t.Parallel() anywhere below: these swap package-level stdout and process
// stderr, and make test runs -race.
func TestFilterDiffsEncodingReporting(t *testing.T) {
	// D8: a file that is merely imperfect keeps the review it gets at HEAD.
	// A design that excludes every non-UTF-8 file fails right here.
	t.Run("D8_marked_but_still_reviewed", func(t *testing.T) {
		a := New(Args{})
		diffs := []model.Diff{
			{NewPath: "auth/token.go", UndecodedCharset: "windows-1252", Unreviewable: false},
			{NewPath: "auth/clean.go"},
		}

		var out bytes.Buffer
		var warn string
		restore := stdout.Swap(&out)
		warn = captureStderr(t, func() { diffs = a.filterDiffs(diffs) })
		restore()

		if len(diffs) != 2 {
			t.Fatalf("kept %d file(s), want both", len(diffs))
		}
		var kept *model.Diff
		for i := range diffs {
			if diffs[i].NewPath == "auth/token.go" {
				kept = &diffs[i]
			}
		}
		if kept == nil {
			t.Fatal("the imperfect file was dropped; HEAD reviews it")
		}
		if kept.UndecodedCharset == "" || kept.Unreviewable {
			t.Errorf("marker lost: charset=%q unreviewable=%v", kept.UndecodedCharset, kept.Unreviewable)
		}

		// I1: exactly one stderr warning, naming the path and the charset, and
		// nothing at all on stdout for that file.
		if n := strings.Count(warn, "[ocr] WARNING:"); n != 1 {
			t.Errorf("got %d stderr warnings, want exactly 1:\n%s", n, warn)
		}
		for _, want := range []string{"auth/token.go", "windows-1252"} {
			if !strings.Contains(warn, want) {
				t.Errorf("warning does not mention %q:\n%s", want, warn)
			}
		}
		if strings.Contains(out.String(), "auth/token.go") {
			t.Errorf("a reviewed file must not appear as skipped on stdout:\n%s", out.String())
		}
		if strings.Contains(warn, "auth/clean.go") {
			t.Errorf("a clean file must not be warned about:\n%s", warn)
		}
	})

	// D10: a genuinely unreviewable file is dropped, and the message names the
	// real cause. Reporting it as "filtered by path/extension rules" would send
	// the user hunting through their exclude globs for a problem that is not
	// there.
	t.Run("D10_excluded_message_names_the_encoding", func(t *testing.T) {
		a := New(Args{})
		diffs := []model.Diff{
			{NewPath: "auth/blob.go", UndecodedCharset: "GB-18030", Unreviewable: true},
			{NewPath: "auth/clean.go"},
		}

		var out bytes.Buffer
		restore := stdout.Swap(&out)
		kept := a.filterDiffs(diffs)
		restore()

		if len(kept) != 1 || kept[0].NewPath != "auth/clean.go" {
			t.Fatalf("kept %+v, want only auth/clean.go", kept)
		}
		got := out.String()
		if !strings.Contains(got, "Skipping auth/blob.go") {
			t.Fatalf("no skip line for the unreviewable file:\n%s", got)
		}
		if !strings.Contains(got, "undecodable encoding (detected GB-18030)") {
			t.Errorf("skip line does not name the encoding or the charset:\n%s", got)
		}
		if strings.Contains(got, "auth/blob.go — filtered by path/extension rules") {
			t.Errorf("the encoding exclusion was misreported as a path/extension filter:\n%s", got)
		}
		// The pre-existing summary line must survive the rewrite.
		if !strings.Contains(got, "Filtered 1 file(s) by include/exclude rules") {
			t.Errorf("the trailing summary line was lost:\n%s", got)
		}
	})

	// The other reasons must keep their existing messages.
	t.Run("existing_skip_messages_unchanged", func(t *testing.T) {
		a := New(Args{})
		var out bytes.Buffer
		restore := stdout.Swap(&out)
		a.filterDiffs([]model.Diff{
			{NewPath: "assets/logo.png", IsBinary: true},
			{NewPath: "notes.txt"},
		})
		restore()

		got := out.String()
		if !strings.Contains(got, "Skipping assets/logo.png — binary file") {
			t.Errorf("binary message changed:\n%s", got)
		}
		if !strings.Contains(got, "Skipping notes.txt — filtered by path/extension rules") {
			t.Errorf("path/extension message changed:\n%s", got)
		}
	})

	// I3: --audience agent silences stdout; the warning must still get out.
	t.Run("I3_quiet_stdout_still_warns_on_stderr", func(t *testing.T) {
		a := New(Args{})
		var warn string
		restoreQuiet := stdout.Quiet()
		warn = captureStderr(t, func() {
			a.filterDiffs([]model.Diff{
				{NewPath: "auth/token.go", UndecodedCharset: "ISO-8859-1"},
			})
		})
		restoreQuiet()

		if !strings.Contains(warn, "auth/token.go") || !strings.Contains(warn, "ISO-8859-1") {
			t.Errorf("the warning was suppressed along with stdout:\n%s", warn)
		}
	})
}

// srcFrenchLatin1 is the MARKED-but-still-reviewed fixture: ASCII \uXXXX
// literals here, encoded to ISO-8859-1 at test time with the matching x/text
// encoder, so no non-UTF-8 byte is ever checked in. Read as UTF-8 its raw bytes
// are a few percent U+FFFD — far under textenc's 20% reviewability bar — so the
// detector marks it and the file keeps the review it gets at HEAD.
const srcFrenchLatin1 = "// V\u00E9rifie le jeton et r\u00E9g\u00E9n\u00E8re la cl\u00E9.\npackage auth\n\n" +
	"func Valider(jeton string) error {\n" +
	"\t// La r\u00E9f\u00E9rence doit \u00EAtre d\u00E9j\u00E0 d\u00E9cod\u00E9e c\u00F4t\u00E9 appelant.\n" +
	"\tif jeton == \"\" {\n\t\treturn errJetonVide\n\t}\n\treturn nil\n}\n"

// previewGarbage is the EXCLUDED fixture: high-half noise with no NUL byte, so
// git still calls the file text and it reaches the encoding filter instead of
// being dropped earlier as binary. Read as UTF-8 it is ~76% U+FFFD.
func previewGarbage() []byte {
	out := make([]byte, 800)
	for i := range out {
		out[i] = byte(0x80 + (i*37+11)%0x7F)
	}
	return out
}

// H4: the machine-readable channel, driven through Agent.preview itself rather
// than through a hand-rolled copy of its loop — a copy asserts on the test's own
// arithmetic and stays green even if preview.go stops filling the field.
//
// Same shape as the scan-mode twin in cmd/opencodereview
// (TestScanPreviewJSONStaysParseableWithLegacyFiles): a real repo, real fixture
// bytes, and the charset the detector actually returned. Both seams are then
// tested the same way, which is the point — scan and review must not drift.
//
// Both encoding shapes appear, because they take different paths through
// whyExcluded: a marked file (UndecodedCharset set, Unreviewable false) is still
// reviewed and carries no exclude reason, while an unreviewable one is excluded
// and must report what its bytes looked like.
//
// No t.Parallel(): New reads git state from a shared temp repo and the fixtures
// run the package-level detector.
func TestPreviewReportsUndecodableEncoding(t *testing.T) {
	dir := initPreviewRepo(t)

	raw, _, err := transform.Bytes(charmap.ISO8859_1.NewEncoder(), []byte(srcFrenchLatin1))
	if err != nil {
		t.Fatalf("encode Latin-1 fixture: %v", err)
	}
	for name, content := range map[string][]byte{
		"legacy.go": raw,
		"blob.go":   previewGarbage(),
		"clean.go":  []byte("package auth\n\nfunc Clean() {}\n"),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	a := New(Args{RepoDir: dir})
	preview, err := a.preview(context.Background())
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	// What the detector said about each file, read off the parsed diffs. The
	// table below compares the preview entry against these because the property
	// under test is "the entry reports THE detected charset", not "the entry
	// says ISO-8859-1". Detection is deterministic (textenc breaks chardet ties
	// by charset name), so this is not a softer check: the fixture guard below
	// pins legacy.go to the exact label, and blob.go's is pinned to "Big5" by
	// the scan-mode twin over the same bytes.
	charsetOf := map[string]string{}
	unreviewable := map[string]bool{}
	for _, d := range a.diffs {
		charsetOf[d.NewPath] = d.UndecodedCharset
		unreviewable[d.NewPath] = d.Unreviewable
	}

	// Fixture sanity. Without this the table could pass by asserting "" == ""
	// everywhere, on a repo where the encoding feature never fired at all.
	if charsetOf["legacy.go"] != "ISO-8859-1" || unreviewable["legacy.go"] {
		t.Fatalf("Latin-1 fixture is not the marked-but-reviewed shape: charset=%q unreviewable=%v",
			charsetOf["legacy.go"], unreviewable["legacy.go"])
	}
	if charsetOf["blob.go"] == "" || !unreviewable["blob.go"] {
		t.Fatalf("garbage fixture is not the excluded shape: charset=%q unreviewable=%v",
			charsetOf["blob.go"], unreviewable["blob.go"])
	}

	byPath := map[string]DiffPreviewEntry{}
	for _, e := range preview.Entries {
		byPath[e.Path] = e
	}

	tests := []struct {
		name       string
		path       string
		wantReview bool
		wantReason ExcludeReason
		// wantCharset is the charset for every file the decode step could not
		// turn into UTF-8, whether that excluded the file or left it marked and
		// still reviewed: preview's job is to say what will happen, and "this
		// file will be reviewed but its text is mojibake" is part of that. It is
		// "" only for a file that decoded cleanly.
		wantCharset string
	}{
		{
			name:        "excluded_file_reports_the_detected_charset",
			path:        "blob.go",
			wantReason:  ExcludeUndecodable,
			wantCharset: charsetOf["blob.go"],
		},
		{
			name:        "marked_file_is_still_reviewed_and_reports_its_charset",
			path:        "legacy.go",
			wantReview:  true,
			wantReason:  ExcludeNone,
			wantCharset: charsetOf["legacy.go"],
		},
		{
			name:       "clean_file_is_untouched",
			path:       "clean.go",
			wantReview: true,
			wantReason: ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := byPath[tt.path]
			if !ok {
				t.Fatalf("%s missing from the preview", tt.path)
			}
			if e.WillReview != tt.wantReview {
				t.Errorf("will_review = %v, want %v", e.WillReview, tt.wantReview)
			}
			if e.ExcludeReason != tt.wantReason {
				t.Errorf("exclude_reason = %q, want %q", e.ExcludeReason, tt.wantReason)
			}
			if e.DetectedCharset != tt.wantCharset {
				t.Errorf("detected_charset = %q, want %q", e.DetectedCharset, tt.wantCharset)
			}
		})
	}
}

// whyExcluded must report the encoding before the extension allowlist, or a
// fixture with an unusual extension reports "unsupported_ext" for an encoding
// problem and the user edits the wrong config.
func TestWhyExcludedEncodingBeatsExtension(t *testing.T) {
	a := &Agent{args: Args{}}
	got := a.whyExcluded(model.Diff{
		NewPath:          "data/dump.weirdext",
		UndecodedCharset: "GB-18030",
		Unreviewable:     true,
	})
	if got != ExcludeUndecodable {
		t.Errorf("reason = %q, want %q", got, ExcludeUndecodable)
	}
}
