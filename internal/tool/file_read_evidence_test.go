// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func commitReadEvidenceFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, name, content)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "Add read evidence fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return getHeadCommit(t, dir)
}

func TestFileReadEvidence_VerifiedCandidates(t *testing.T) {
	dir := setupTestRepo(t)
	files := map[string]string{
		"src/Widget.tsx":   "export const Widget = () => <div />;",
		"reverse.ts":       "export const value = 1;",
		"both.ts":          "original",
		"both.tsx":         "alternate",
		"[literal].tsx":    "literal bracket",
		"-option.tsx":      "literal leading dash",
		"glob-one.tsx":     "not the literal query",
		"folder.tsx/child": "directory is not a file",
	}
	if runtime.GOOS != "windows" {
		files["a*.tsx"] = "literal star"
		files[":(glob)literal.tsx"] = "literal magic"
		files["tab\tline\n.tsx"] = "literal whitespace"
		if err := os.Symlink("hello.go", filepath.Join(dir, "link.tsx")); err != nil {
			t.Fatal(err)
		}
	}
	commit := commitReadEvidenceFiles(t, dir, files)
	writeTestFile(t, dir, "disk.tsx", "only on disk")
	cases := []struct {
		name     string
		filePath string
		want     string
	}{
		{"typescript to tsx", "src/Widget.ts", "src/Widget.tsx"},
		{"tsx to typescript", "reverse.tsx", "reverse.ts"},
		{"literal bracket", "[literal].ts", "[literal].tsx"},
		{"literal star", "a*.ts", "a*.tsx"},
		{"literal pathspec magic", ":(glob)literal.ts", ":(glob)literal.tsx"},
		{"literal whitespace", "tab\tline\n.ts", "tab\tline\n.tsx"},
		{"literal leading dash", "-option.ts", "-option.tsx"},
		{"glob does not expand", "glob-*.ts", ""},
		{"candidate only on disk", "disk.ts", ""},
		{"no alternate", "missing.ts", ""},
		{"non typescript", "src/Widget.js", ""},
		{"directory candidate", "folder.ts", ""},
		{"symlink candidate", "link.ts", ""},
		{"dot segment", "src/./Widget.ts", ""},
		{"parent segment", "src/../src/Widget.ts", ""},
		{"outside path", "../src/Widget.ts", ""},
		{"absolute path", "/src/Widget.ts", ""},
		{"backslash path", "src\\Widget.ts", ""},
		{"nul path", "bad\x00.ts", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.want != "" && strings.ContainsAny(tc.want, "*:\t\n") {
				t.Skip("filename is not supported on Windows")
			}
			for _, runner := range []*gitcmd.Runner{nil, gitcmd.New(1)} {
				provider := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: runner})
				if strings.ContainsAny(tc.filePath, "*[]") {
					// Keep literal tree matching independent of Git's revision glob parsing.
					if candidate := provider.FileReader.readCandidate(context.Background(), tc.filePath); candidate != tc.want {
						t.Fatalf("candidate = %q, want %q", candidate, tc.want)
					}
					continue
				}
				out, evidence, err := provider.ExecuteWithEvidence(context.Background(), map[string]any{"file_path": tc.filePath})
				if err == nil || out != "" {
					t.Fatalf("ExecuteWithEvidence() = %q, %v; want read failure", out, err)
				}
				if evidence == nil || evidence.TargetCommit != commit || evidence.CandidatePath != tc.want || evidence.Read != nil {
					t.Fatalf("evidence = %+v, want candidate %q at %s without read content", evidence, tc.want, commit)
				}
				if !strings.HasPrefix(err.Error(), "file ") {
					t.Fatalf("historical error was changed: %v", err)
				}
			}
		})
	}
	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	if candidate := fr.readCandidate(context.Background(), "both.ts"); candidate != "" {
		t.Fatalf("existing original produced candidate %q", candidate)
	}
}

func TestFileReadEvidence_TargetIsolation(t *testing.T) {
	dir := setupTestRepo(t)
	oldCommit := getHeadCommit(t, dir)
	newCommit := commitReadEvidenceFiles(t, dir, map[string]string{"Widget.tsx": "new commit only"})
	cases := []struct {
		name          string
		mode          ReviewMode
		ref           string
		wantEvidence  bool
		wantCandidate string
	}{
		{"old commit", ModeCommit, oldCommit, true, ""},
		{"range target", ModeRange, newCommit, true, "Widget.tsx"},
		{"workspace", ModeWorkspace, newCommit, false, ""},
		{"unknown mode", ReviewMode(99), newCommit, false, ""},
		{"symbolic ref", ModeCommit, "HEAD", false, ""},
		{"abbreviated ref", ModeCommit, newCommit[:12], false, ""},
		{"empty ref", ModeCommit, "", false, ""},
		{"invalid ref", ModeCommit, strings.Repeat("z", 40), false, ""},
		{"missing object", ModeCommit, strings.Repeat("a", 40), true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewFileRead(&FileReader{RepoDir: dir, Mode: tc.mode, Ref: tc.ref})
			_, evidence, err := provider.ExecuteWithEvidence(context.Background(), map[string]any{"file_path": "Widget.ts"})
			if err == nil {
				t.Fatal("expected a read failure")
			}
			if (evidence != nil) != tc.wantEvidence {
				t.Fatalf("evidence = %+v, want present %v", evidence, tc.wantEvidence)
			}
			if evidence != nil && (evidence.CandidatePath != tc.wantCandidate || evidence.Read != nil) {
				t.Fatalf("evidence = %+v, want candidate %q without content", evidence, tc.wantCandidate)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, runner := range []*gitcmd.Runner{nil, gitcmd.New(1)} {
		provider := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeCommit, Ref: newCommit, Runner: runner})
		_, evidence, err := provider.ExecuteWithEvidence(ctx, map[string]any{"file_path": "Widget.ts"})
		if err == nil || evidence == nil || evidence.CandidatePath != "" || evidence.Read != nil {
			t.Fatalf("cancelled read produced %v, %+v", err, evidence)
		}
	}
}

func TestFileReadEvidence_ContentMetadata(t *testing.T) {
	dir := setupTestRepo(t)
	commit := commitReadEvidenceFiles(t, dir, map[string]string{
		"Widget.tsx": "one\ntwo\nthree",
		"empty.tsx":  "",
		"big.tsx":    strings.Repeat("line\n", 600),
	})
	provider := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit})
	cases := []struct {
		name string
		args map[string]any
		want FileReadContent
	}{
		{"full", map[string]any{"file_path": "Widget.tsx"}, FileReadContent{"Widget.tsx", 1, 3, 3, false}},
		{"partial", map[string]any{"file_path": "Widget.tsx", "start_line": float64(2), "end_line": float64(2)}, FileReadContent{"Widget.tsx", 2, 2, 3, false}},
		{"empty", map[string]any{"file_path": "empty.tsx"}, FileReadContent{"empty.tsx", 1, 0, 0, false}},
		{"truncated", map[string]any{"file_path": "big.tsx"}, FileReadContent{"big.tsx", 1, 500, 601, true}},
		{"bounded truncated", map[string]any{"file_path": "big.tsx", "start_line": float64(2), "end_line": float64(550)}, FileReadContent{"big.tsx", 2, 501, 601, true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, evidence, err := provider.ExecuteWithEvidence(context.Background(), tc.args)
			if err != nil || evidence == nil || evidence.TargetCommit != commit || evidence.CandidatePath != "" || !reflect.DeepEqual(evidence.Read, &tc.want) {
				t.Fatalf("ExecuteWithEvidence() = %q, %+v, %v; want %+v", out, evidence, err, tc.want)
			}
		})
	}
	for _, args := range []map[string]any{
		{},
		{"file_path": "Widget.tsx", "start_line": float64(5), "end_line": float64(2)},
		{"file_path": "Widget.tsx", "start_line": float64(5)},
	} {
		_, evidence, _ := provider.ExecuteWithEvidence(context.Background(), args)
		if evidence != nil {
			t.Fatalf("invalid arguments produced evidence %+v", evidence)
		}
	}
	provider.FileReader.Ref = "HEAD"
	_, evidence, err := provider.ExecuteWithEvidence(context.Background(), map[string]any{"file_path": "Widget.tsx"})
	if err != nil || evidence != nil {
		t.Fatalf("symbolic target successful read = %+v, %v; want no evidence", evidence, err)
	}
}
