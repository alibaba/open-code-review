// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package diff parses unified git diff output into structured Diff objects.
package diff

import (
	"bytes"
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
	diffHeaderRe = regexp.MustCompile(`^diff --git a/(.+?) b/(.+)$`)
	// Anchored: git emits the marker at column 0 ("Binary files a/x and b/y
	// differ"). Content lines inside hunks always carry a leading "+", "-"
	// or " " prefix, so an anchored match can never misfire on file content.
	binaryRe = regexp.MustCompile(`^Binary files `)
)

// splitDiffLines splits unified diff text on "\n" and drops the carriage
// return that CRLF-terminated diff text leaves at the end of every line.
//
// Git writes its own structural lines with "\n", but the diff text can reach
// us already converted: a .patch checked out under core.autocrlf=true, or
// output captured through a Windows shell. Left in place, that "\r" is not
// cosmetic. It rides into the "diff --git a/(.+?) b/(.+)$" capture, so NewPath
// becomes "file.go\r" and the file can no longer be opened for review; and it
// defeats the exact comparisons against "--- /dev/null" and "+++ /dev/null",
// which silently costs a diff its IsNew or IsDeleted flag.
//
// Stripping it matches how the rest of the package already treats a trailing
// carriage return: resolver.go trims it before matching lines, and filereader
// and viewer do the same when reading file content.
func splitDiffLines(text string) []string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}

// parseDiffHeaderLine extracts the two pathnames from a "diff --git" line,
// preferring the split a bare header names twice and falling back to
// parseQuotedDiffHeader when git has quoted a side (see there).
//
// The two sides are separated by the same " b/" a pathname can contain, and git
// does not quote a space, so the delimiter diffHeaderRe takes -- the first one --
// is not always the separator. "diff --git a/x b/y.go b/x b/y.go" was read as a
// rename from "x" to "y.go b/x b/y.go", and the file was then reviewed under a
// path that does not exist. A same-path header is unambiguous when its two
// operands agree. Renames and copies can also admit an equal split, so their
// extended headers remain authoritative over these provisional paths.
func parseDiffHeaderLine(line string) (oldPath string, newPath string, ok bool) {
	if rest, isHeader := strings.CutPrefix(line, "diff --git "); isHeader {
		if sameOld, sameNew, ok := splitSamePathHeader(rest); ok {
			return sameOld, sameNew, true
		}
	}
	if m := diffHeaderRe.FindStringSubmatch(line); m != nil {
		return m[1], m[2], true
	}
	return parseQuotedDiffHeader(line)
}

// splitSamePathHeader reports the split of a bare operand pair whose two sides
// are the same pathname, which is the shape git writes for every header that is
// not a rename or a copy. rest is the line after "diff --git ".
//
// An equal split can only be the separator: at a " b/" inside the pathname of
// such a header the two sides differ by an odd number of bytes, so they never
// match, and the separator is the only split that can.
func splitSamePathHeader(rest string) (oldPath string, newPath string, ok bool) {
	for i := 0; i+3 <= len(rest); i++ {
		if rest[i] != ' ' || rest[i+1] != 'b' || rest[i+2] != '/' {
			continue
		}
		stripped, isOld := strings.CutPrefix(rest[:i], "a/")
		if newPath = rest[i+3:]; isOld && stripped == newPath {
			return stripped, newPath, true
		}
	}
	return "", "", false
}

// ParseDiffText splits the unified diff text into per-file Diff structs.
// ref, if non-empty, is a git ref used to read new-file content via
// git show instead of reading from the working tree.
// runner, if non-nil, is used to execute git subprocesses through a
// shared concurrency limiter.
func ParseDiffText(ctx context.Context, diffText string, repoDir string, ref string, runner *gitcmd.Runner) ([]model.Diff, error) {
	lines := splitDiffLines(diffText)
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
		if oldPath, newPath, ok := parseDiffHeaderLine(line); ok {
			// Flush previous diff
			if current != nil {
				current.Diff = strings.TrimSuffix(buf.String(), "\n")
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
			current.OldPath = unquoteRenamePath(strings.TrimPrefix(line, "rename from "))
			current.IsRenamed = true
		case strings.HasPrefix(line, "rename to "):
			current.NewPath = unquoteRenamePath(strings.TrimPrefix(line, "rename to "))
			current.IsRenamed = true
		case !inHunk && strings.HasPrefix(line, "copy from "):
			current.OldPath = unquoteRenamePath(strings.TrimPrefix(line, "copy from "))
		case !inHunk && strings.HasPrefix(line, "copy to "):
			current.NewPath = unquoteRenamePath(strings.TrimPrefix(line, "copy to "))
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
		diffs = append(diffs, *current)
	}

	finalizeDiffs(ctx, diffs, repoDir, ref, runner)
	return diffs, nil
}

// finalizeDiffs runs finalizeDiff over diffs. When ref is set, the new-file
// contents are first read through one `git cat-file --batch` process instead
// of a `git show` per file, since starting git costs tens of milliseconds per
// file on Windows. A file the batch does not return still goes through
// finalizeDiff, so its content or warning is unchanged.
func finalizeDiffs(ctx context.Context, diffs []model.Diff, repoDir string, ref string, runner *gitcmd.Runner) {
	var contents map[string]string
	if ref != "" {
		var paths []string
		for _, d := range diffs {
			if needsNewFileContent(d) {
				paths = append(paths, d.NewPath)
			}
		}
		contents = readBlobsAtRef(ctx, repoDir, ref, paths, runner)
	}
	for i := range diffs {
		if content, ok := contents[diffs[i].NewPath]; ok && needsNewFileContent(diffs[i]) {
			diffs[i].NewFileContent = content
			continue
		}
		finalizeDiff(ctx, &diffs[i], repoDir, ref, runner)
	}
}

func needsNewFileContent(d model.Diff) bool {
	return !d.IsDeleted && d.NewPath != "/dev/null" && !d.IsBinary
}

// readBlobsAtRef returns the content at ref of each path that resolves to a
// blob. Paths it cannot return are left out of the map: missing paths,
// non-blob entries such as submodules, and paths containing a newline, which
// the line-based batch input cannot carry.
func readBlobsAtRef(ctx context.Context, repoDir string, ref string, paths []string, runner *gitcmd.Runner) map[string]string {
	var input strings.Builder
	var batched []string
	for _, path := range paths {
		if strings.Contains(path, "\n") {
			continue
		}
		input.WriteString(ref + ":" + path + "\n")
		batched = append(batched, path)
	}
	if len(batched) == 0 {
		return nil
	}

	args := []string{"cat-file", "--batch"}
	var output []byte
	if runner != nil {
		output, _ = runner.OutputWithInput(ctx, repoDir, strings.NewReader(input.String()), args...)
	} else {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		cmd.Stdin = strings.NewReader(input.String())
		output, _ = cmd.Output()
	}

	// Each answer is "<oid> <type> <size>\n<content>\n", or one line such as
	// "<name> missing". If git failed or stopped early, the paths without an
	// answer are left to finalizeDiff.
	contents := make(map[string]string, len(batched))
	rest := output
	for _, path := range batched {
		header, after, ok := bytes.Cut(rest, []byte("\n"))
		if !ok {
			break
		}
		rest = after
		if bytes.HasSuffix(header, []byte(" missing")) || bytes.HasSuffix(header, []byte(" ambiguous")) {
			continue
		}
		fields := strings.Fields(string(header))
		if len(fields) != 3 {
			break
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size >= len(rest) {
			break
		}
		if fields[1] == "blob" {
			contents[path] = string(rest[:size])
		}
		rest = rest[size+1:]
	}
	return contents
}

// finalizeDiff reads the new file content. When ref is non-empty it uses
// git show to read the file at that ref; otherwise it reads from disk.
func finalizeDiff(ctx context.Context, d *model.Diff, repoDir string, ref string, runner *gitcmd.Runner) {
	if d.IsDeleted || d.NewPath == "/dev/null" {
		d.NewPath = "/dev/null"
		return
	}
	if d.IsBinary {
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
