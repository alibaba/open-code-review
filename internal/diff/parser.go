// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package diff parses unified git diff output into structured Diff objects.
package diff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
)

var (
	// Anchored: git emits the marker at column 0 ("Binary files a/x and b/y
	// differ"). Content lines inside hunks always carry a leading "+", "-"
	// or " " prefix, so an anchored match can never misfire on file content.
	binaryRe = regexp.MustCompile(`^Binary files `)
)

func parseDiffHeader(line string) (string, string, bool) {
	const prefix = "diff --git "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(line, prefix)
	var oldToken, newToken string
	if strings.HasPrefix(rest, `"`) {
		var remaining string
		var ok bool
		oldToken, remaining, ok = consumeQuotedPath(rest)
		if !ok || !strings.HasPrefix(remaining, " ") {
			return "", "", false
		}
		newToken = strings.TrimPrefix(remaining, " ")
	} else {
		separator := strings.Index(rest, ` "b/`)
		if separator < 0 {
			separator = strings.Index(rest, " b/")
		}
		if separator < 0 {
			return "", "", false
		}
		oldToken = rest[:separator]
		newToken = rest[separator+1:]
	}
	oldPath, oldOK := decodeGitPath(oldToken, "a/")
	newPath, newOK := decodeGitPath(newToken, "b/")
	return oldPath, newPath, oldOK && newOK
}

func consumeQuotedPath(value string) (string, string, bool) {
	escaped := false
	for i := 1; i < len(value); i++ {
		switch {
		case escaped:
			escaped = false
		case value[i] == '\\':
			escaped = true
		case value[i] == '"':
			return value[:i+1], value[i+1:], true
		}
	}
	return "", "", false
}

func decodeGitPath(value, prefix string) (string, bool) {
	if strings.HasPrefix(value, `"`) {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", false
		}
		value = decoded
	}
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	return strings.TrimPrefix(value, prefix), true
}

func decodeExtendedPath(value string) string {
	if !strings.HasPrefix(value, `"`) {
		return value
	}
	decoded, err := strconv.Unquote(value)
	if err != nil {
		return value
	}
	return decoded
}

// ParseDiffText splits the unified diff text into per-file Diff structs.
// ref, if non-empty, is a git ref used to read new-file content via
// git show instead of reading from the working tree.
// runner, if non-nil, is used to execute git subprocesses through a
// shared concurrency limiter.
func ParseDiffText(ctx context.Context, diffText string, repoDir string, ref string, runner *gitcmd.Runner) ([]model.Diff, error) {
	lines := strings.Split(diffText, "\n")
	var diffs []model.Diff
	var current *model.Diff
	var buf strings.Builder
	// inHunk tracks whether the current line sits inside a "@@" hunk of the
	// current file's section. Only hunk content lines carry a leading
	// "+"/"-"/" " marker, so insertion/deletion counting and the binary
	// marker must look at hunk state: outside a hunk, "+++ b/file" and
	// "--- a/file" are headers, not content; inside a hunk, an added line
	// like "++i" renders as "+++i" and still counts as an insertion.
	inHunk := false

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	for _, line := range lines {
		if oldPath, newPath, ok := parseDiffHeader(line); ok {
			// Flush previous diff
			if current != nil {
				current.Diff = strings.TrimSuffix(buf.String(), "\n")
				finalizeDiff(ctx, current, repoDir, ref, runner)
				diffs = append(diffs, *current)
				buf.Reset()
			}
			current = &model.Diff{
				OldPath: oldPath,
				NewPath: newPath,
			}
			inHunk = false
		}
		if current == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		// The object IDs and mode in Git's extended "index" header are not
		// useful review context. Keep index text in hunks, where it is file
		// content and therefore carries a diff prefix.
		case !inHunk && strings.HasPrefix(line, "index "):
			continue
		case !inHunk && binaryRe.MatchString(line):
			current.IsBinary = true
		// Extended header lines (unambiguous: content lines always carry a
		// leading "+", "-" or " " prefix, so a bare prefix match is safe).
		case strings.HasPrefix(line, "new file mode "):
			current.IsNew = true
		case strings.HasPrefix(line, "deleted file mode "):
			current.IsDeleted = true
		case strings.HasPrefix(line, "rename from "):
			// Authoritative old path for renames; more reliable than the
			// "diff --git" header when paths contain spaces.
			current.OldPath = decodeExtendedPath(strings.TrimPrefix(line, "rename from "))
			current.IsRenamed = true
		case strings.HasPrefix(line, "rename to "):
			current.NewPath = decodeExtendedPath(strings.TrimPrefix(line, "rename to "))
			current.IsRenamed = true
		// git emits "--- /dev/null" / "+++ /dev/null" without a/ b/ prefixes.
		// Guarded by inHunk: inside a hunk the same strings can be content
		// (e.g. an added line "++ /dev/null").
		case !inHunk && line == "--- /dev/null":
			current.IsNew = true
		case !inHunk && line == "+++ /dev/null":
			current.IsDeleted = true
		case inHunk && strings.HasPrefix(line, "+"):
			current.Insertions++
		case inHunk && strings.HasPrefix(line, "-"):
			current.Deletions++
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}

	// Flush last diff
	if current != nil {
		current.Diff = strings.TrimSuffix(buf.String(), "\n")
		finalizeDiff(ctx, current, repoDir, ref, runner)
		diffs = append(diffs, *current)
	}

	return diffs, nil
}

// finalizeDiff reads the new file content. When ref is non-empty it uses
// git show to read the file at that ref; otherwise it reads from disk.
func finalizeDiff(ctx context.Context, d *model.Diff, repoDir string, ref string, runner *gitcmd.Runner) {
	if d.IsDeleted || d.NewPath == "/dev/null" {
		d.NewPath = "/dev/null"
		return
	}
	if ref != "" {
		args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", ref + ":" + d.NewPath}
		var output []byte
		var err error
		if runner != nil {
			output, err = runner.Output(ctx, repoDir, args...)
		} else {
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = repoDir
			output, err = cmd.Output()
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read file %s at ref %s: %v\n",
				d.NewPath, ref, err)
			return
		}
		d.NewFileContent = string(output)
		return
	}
	content, err := readWorkspaceFileForDiff(repoDir, d.NewPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read file %s for review: %v\n", d.NewPath, err)
		return
	}
	d.NewFileContent = string(content)
}
