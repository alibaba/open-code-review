package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadLines_Disk_FullFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", "line1\nline2\nline3\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, total, err := fr.ReadLines(context.Background(), "a.txt", 1, 100)
	if err != nil {
		t.Fatal(err)
	}

	// strings.Split("line1\nline2\nline3\n", "\n") produces 4 elements (trailing empty)
	if total != 4 {
		t.Errorf("totalLines = %d, want 4", total)
	}
	want := []string{"line1", "line2", "line3", ""}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(lines), len(want))
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("lines[%d] = %q, want %q", i, lines[i], w)
		}
	}
}

func TestReadLines_Disk_Window(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "b.txt", "a\nb\nc\nd\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, total, err := fr.ReadLines(context.Background(), "b.txt", 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	if total != 5 {
		t.Errorf("totalLines = %d, want 5", total)
	}
	want := []string{"b", "c"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(lines), len(want))
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("lines[%d] = %q, want %q", i, lines[i], w)
		}
	}
}

func TestReadLines_Disk_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "empty.txt", "")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, total, err := fr.ReadLines(context.Background(), "empty.txt", 1, 100)
	if err != nil {
		t.Fatal(err)
	}

	if total != 0 {
		t.Errorf("totalLines = %d, want 0", total)
	}
	if len(lines) != 0 {
		t.Errorf("got %d lines, want 0", len(lines))
	}
}

func TestReadLines_Disk_StartBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "short.txt", "only\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, total, err := fr.ReadLines(context.Background(), "short.txt", 100, 10)
	if err != nil {
		t.Fatal(err)
	}

	if total != 2 {
		t.Errorf("totalLines = %d, want 2", total)
	}
	if len(lines) != 0 {
		t.Errorf("got %d lines, want 0", len(lines))
	}
}

func TestReadLines_Disk_TrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "trail.txt", "x\ny\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, total, err := fr.ReadLines(context.Background(), "trail.txt", 1, 100)
	if err != nil {
		t.Fatal(err)
	}

	// strings.Split("x\ny\n", "\n") = ["x","y",""] → 3 elements
	if total != 3 {
		t.Errorf("totalLines = %d, want 3", total)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[2] != "" {
		t.Errorf("lines[2] = %q, want empty", lines[2])
	}
}

func TestReadLines_Disk_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "notrail.txt", "x\ny")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, total, err := fr.ReadLines(context.Background(), "notrail.txt", 1, 100)
	if err != nil {
		t.Fatal(err)
	}

	// strings.Split("x\ny", "\n") = ["x","y"] → 2 elements
	if total != 2 {
		t.Errorf("totalLines = %d, want 2", total)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
}

func TestReadLines_GitShow_Window(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	lines, total, err := fr.ReadLines(context.Background(), "hello.go", 1, 100)
	if err != nil {
		t.Fatal(err)
	}

	// hello.go = "package main\n\nfunc Hello() {}\n" → 4 elements via strings.Split
	if total != 4 {
		t.Errorf("totalLines = %d, want 4", total)
	}
	if len(lines) < 1 || lines[0] != "package main" {
		t.Errorf("first line = %q, want %q", lines[0], "package main")
	}
}

func TestReadLines_Disk_RejectsParentTraversal(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside-secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	escapePath, err := filepath.Rel(repoDir, secretPath)
	if err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: repoDir, Mode: ModeWorkspace}
	if _, _, err := fr.ReadLines(context.Background(), escapePath, 1, 10); err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("ReadLines(%q) error = %v, want outside repository", escapePath, err)
	}
	if _, err := fr.Read(context.Background(), escapePath); err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("Read(%q) error = %v, want outside repository", escapePath, err)
	}
}

func TestReadLines_Disk_AllowsParentSegmentWithinRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "target.txt", "inside\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, _, err := fr.ReadLines(context.Background(), filepath.Join("pkg", "..", "target.txt"), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 || lines[0] != "inside" {
		t.Fatalf("ReadLines(pkg/../target.txt) = %q, want inside", lines)
	}
}

func TestReadLines_Disk_AbsolutePathStaysUnderRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("absolute path syntax varies on Windows")
	}

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, filepath.Join("etc", "passwd"), "repo-passwd\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, _, err := fr.ReadLines(context.Background(), "/etc/passwd", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 || lines[0] != "repo-passwd" {
		t.Fatalf("ReadLines(/etc/passwd) = %q, want repo-passwd", lines)
	}
}

func TestReadLines_Disk_MissingFilePreservesReadError(t *testing.T) {
	dir := t.TempDir()
	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}

	_, _, err := fr.ReadLines(context.Background(), "missing.txt", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.txt) error = nil, want not exist")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadLines(missing.txt) error = %v, want os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), `read file "missing.txt"`) || strings.Contains(err.Error(), "resolve file") {
		t.Fatalf("ReadLines(missing.txt) error = %v, want read file error", err)
	}

	_, err = fr.Read(context.Background(), "missing.txt")
	if err == nil {
		t.Fatal("Read(missing.txt) error = nil, want not exist")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read(missing.txt) error = %v, want os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), `read file "missing.txt"`) || strings.Contains(err.Error(), "resolve file") {
		t.Fatalf("Read(missing.txt) error = %v, want read file error", err)
	}
}

func TestReadLines_Disk_RejectsSymlinkOutsideRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}

	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside-secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(repoDir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: repoDir, Mode: ModeWorkspace}
	if _, _, err := fr.ReadLines(context.Background(), "link.txt", 1, 10); err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("ReadLines(link.txt) error = %v, want outside repository", err)
	}
}

func TestReadLines_Disk_AllowsSymlinkInsideRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "target.txt", "inside\n")
	if err := os.Symlink(filepath.Join(dir, "target.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	lines, _, err := fr.ReadLines(context.Background(), "link.txt", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 || lines[0] != "inside" {
		t.Fatalf("ReadLines(link.txt) = %q, want inside", lines)
	}
}

func TestExecute_Truncation(t *testing.T) {
	dir := t.TempDir()

	var sb strings.Builder
	for i := 1; i <= 600; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	writeTestFile(t, dir, "big.txt", sb.String())

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	p := NewFileRead(fr)

	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "big.txt",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "IS_TRUNCATED: true") {
		t.Error("expected IS_TRUNCATED: true")
	}
	if !strings.Contains(result, "LINE_RANGE: 1-500") {
		t.Error("expected LINE_RANGE: 1-500")
	}
	if !strings.Contains(result, "Results truncated to 500 lines") {
		t.Error("expected truncation note")
	}
	if strings.Contains(result, "501|") {
		t.Error("line 501 should not appear in output")
	}
}

func TestExecute_WithEndLine(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "c.txt", "a\nb\nc\nd\ne\n")

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	p := NewFileRead(fr)

	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  "c.txt",
		"start_line": float64(2),
		"end_line":   float64(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "IS_TRUNCATED: false") {
		t.Error("expected IS_TRUNCATED: false")
	}
	if !strings.Contains(result, "LINE_RANGE: 2-4") {
		t.Error("expected LINE_RANGE: 2-4")
	}
	if !strings.Contains(result, "2|b") {
		t.Error("expected line 2")
	}
	if !strings.Contains(result, "4|d") {
		t.Error("expected line 4")
	}
	if strings.Contains(result, "5|e") {
		t.Error("line 5 should not appear")
	}
}
