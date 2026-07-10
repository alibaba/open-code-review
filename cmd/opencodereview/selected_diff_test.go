package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// --- CLI flag validation for --diff-file ---

func TestParseReviewFlags_DiffFile(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "valid with range",
			args: []string{"--from", "main", "--to", "dev", "--diff-file", "sel.patch"},
		},
		{
			name: "valid with commit",
			args: []string{"--commit", "abc123", "--diff-file", "sel.patch"},
		},
		{
			name:    "rejects workspace mode",
			args:    []string{"--diff-file", "sel.patch"},
			wantErr: "requires --from/--to or --commit",
		},
		{
			name:    "rejects with only from",
			args:    []string{"--from", "main", "--diff-file", "sel.patch"},
			wantErr: "--to is required",
		},
		{
			name:    "rejects with resume",
			args:    []string{"--from", "main", "--to", "dev", "--diff-file", "sel.patch", "--resume", "sess1"},
			wantErr: "--diff-file and --resume cannot be used together",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseReviewFlags(tt.args)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadSelectedDiff(t *testing.T) {
	if s, err := loadSelectedDiff(""); err != nil || s != "" {
		t.Fatalf("empty path: got (%q,%v), want (\"\",nil)", s, err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "sel.patch")
	if err := os.WriteFile(p, []byte("PATCH-BODY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err := loadSelectedDiff(p); err != nil || s != "PATCH-BODY" {
		t.Fatalf("got (%q,%v)", s, err)
	}
	if _, err := loadSelectedDiff(filepath.Join(dir, "nope.patch")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- Integration test: real git repo, subset selection ---

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// buildMultiHunkRepo creates a repo whose HEAD changes svc.go in two separate
// regions (two hunks) and rewrites other.go. Returns (dir, baseSHA, headSHA).
func buildMultiHunkRepo(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	// Base version of svc.go: 20 numbered lines.
	var base strings.Builder
	for i := 1; i <= 20; i++ {
		base.WriteString("line")
		base.WriteByte(byte('0' + i%10))
		base.WriteString(" original\n")
	}
	writeFile(t, dir, "svc.go", base.String())
	writeFile(t, dir, "other.go", "package other\n\nfunc Old() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "base")
	baseSHA := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))

	// HEAD version: insert a line near the top (hunk 1) and near the bottom
	// (hunk 2); keep the middle unchanged so the two edits form separate hunks.
	lines := strings.Split(strings.TrimRight(base.String(), "\n"), "\n")
	var head strings.Builder
	for i, l := range lines {
		head.WriteString(l)
		head.WriteString("\n")
		if i == 1 { // after line index 1 → top hunk
			head.WriteString("INSERTED-TOP\n")
		}
		if i == 17 { // near the bottom → separate hunk
			head.WriteString("INSERTED-BOTTOM\n")
		}
	}
	writeFile(t, dir, "svc.go", head.String())
	writeFile(t, dir, "other.go", "package other\n\nfunc New() { println(\"changed\") }\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "head")
	headSHA := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))

	return dir, baseSHA, headSHA
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

var hunkStartRe = regexp.MustCompile(`(?m)^@@ `)

// firstHunkOnly reconstructs a single-file patch containing only the first hunk
// of the given canonical per-file diff (preamble + hunk 1), byte-for-byte.
func firstHunkOnly(t *testing.T, fileDiff string) string {
	t.Helper()
	locs := hunkStartRe.FindAllStringIndex(fileDiff, -1)
	if len(locs) < 2 {
		t.Fatalf("expected >=2 hunks in svc.go diff, got %d\n%s", len(locs), fileDiff)
	}
	// Keep everything from start up to (not including) the second hunk header.
	return strings.TrimRight(fileDiff[:locs[1][0]], "\n")
}

func TestIntegration_SelectedDiffScopeAndTrueHead(t *testing.T) {
	dir, baseSHA, headSHA := buildMultiHunkRepo(t)
	ctx := context.Background()

	// 1. Canonical BASE..HEAD diff via the real provider.
	provider := diff.NewProvider(dir, baseSHA, headSHA, nil)
	canonical, err := provider.GetDiff(ctx)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	var svcDiff string
	seenOther := false
	for _, d := range canonical {
		if d.NewPath == "svc.go" {
			svcDiff = d.Diff
		}
		if d.NewPath == "other.go" {
			seenOther = true
		}
	}
	if svcDiff == "" || !seenOther {
		t.Fatalf("canonical missing files; got %d diffs", len(canonical))
	}

	// 2. Build a selection patch = only svc.go's first hunk.
	selected := firstHunkOnly(t, svcDiff)

	// 3. Apply selection.
	narrowed, err := diff.ApplySelection(canonical, selected)
	if err != nil {
		t.Fatalf("ApplySelection: %v", err)
	}

	// Scope: only svc.go, only the top hunk.
	if len(narrowed) != 1 || narrowed[0].NewPath != "svc.go" {
		var paths []string
		for _, d := range narrowed {
			paths = append(paths, d.NewPath)
		}
		t.Fatalf("expected only svc.go, got %v", paths)
	}
	if !strings.Contains(narrowed[0].Diff, "INSERTED-TOP") {
		t.Errorf("narrowed diff should contain top hunk")
	}
	if strings.Contains(narrowed[0].Diff, "INSERTED-BOTTOM") {
		t.Errorf("narrowed diff must NOT contain the unselected bottom hunk:\n%s", narrowed[0].Diff)
	}

	// 4. Full file reads stay anchored at true HEAD: the FileReader returns the
	// COMPLETE head file, including the region of the *unselected* bottom hunk.
	fr := &tool.FileReader{RepoDir: dir, Mode: tool.ModeRange, Ref: headSHA}
	content, err := fr.Read(ctx, "svc.go")
	if err != nil {
		t.Fatalf("FileReader.Read: %v", err)
	}
	if !strings.Contains(content, "INSERTED-TOP") || !strings.Contains(content, "INSERTED-BOTTOM") {
		t.Errorf("full HEAD read must include both edits (true-head context); got:\n%s", content)
	}
	// And NewFileContent carried on the narrowed diff is the same true-HEAD file.
	if !strings.Contains(narrowed[0].NewFileContent, "INSERTED-BOTTOM") {
		t.Errorf("narrowed NewFileContent must remain the full true-HEAD file")
	}

	// 5. Preview scope reflects the narrowed selection (only svc.go reviewable).
	ag := agent.New(agent.Args{RepoDir: dir, From: baseSHA, To: headSHA, SelectedDiff: selected})
	preview, err := ag.Preview(ctx)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.TotalFiles != 1 {
		t.Fatalf("preview TotalFiles=%d, want 1 (other.go dropped by selection)", preview.TotalFiles)
	}
	if preview.Entries[0].Path != "svc.go" || !preview.Entries[0].WillReview {
		t.Errorf("preview entry unexpected: %+v", preview.Entries[0])
	}
}

func TestIntegration_MutatedSelectionRejected(t *testing.T) {
	dir, baseSHA, headSHA := buildMultiHunkRepo(t)
	ctx := context.Background()
	provider := diff.NewProvider(dir, baseSHA, headSHA, nil)
	canonical, err := provider.GetDiff(ctx)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	var svcDiff string
	for _, d := range canonical {
		if d.NewPath == "svc.go" {
			svcDiff = d.Diff
		}
	}
	selected := firstHunkOnly(t, svcDiff)
	// Mutate an added line body — must fail closed.
	mutated := strings.Replace(selected, "INSERTED-TOP", "INSERTED-TOP-EVIL", 1)
	if _, err := diff.ApplySelection(canonical, mutated); err == nil {
		t.Fatal("expected rejection of mutated selection patch")
	}
}
