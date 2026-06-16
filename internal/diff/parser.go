// Package diff parses unified git diff output into structured Diff objects.
package diff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
)

var (
	diffHeaderRe = regexp.MustCompile(`^diff --git a/(.+?) b/(.+)$`)
	binaryRe     = regexp.MustCompile(`Binary files `)
)

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

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	for _, line := range lines {
		if m := diffHeaderRe.FindStringSubmatch(line); m != nil {
			// Flush previous diff
			if current != nil {
				current.Diff = strings.TrimSuffix(buf.String(), "\n")
				finalizeDiff(ctx, current, repoDir, ref, runner)
				diffs = append(diffs, *current)
				buf.Reset()
			}
			current = &model.Diff{
				OldPath: m[1],
				NewPath: m[2],
			}
		}
		if current == nil {
			continue
		}

		switch {
		case binaryRe.MatchString(line):
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
			current.OldPath = strings.TrimPrefix(line, "rename from ")
			current.IsRenamed = true
		case strings.HasPrefix(line, "rename to "):
			current.NewPath = strings.TrimPrefix(line, "rename to ")
			current.IsRenamed = true
		// git emits "--- /dev/null" / "+++ /dev/null" without a/ b/ prefixes.
		case line == "--- /dev/null":
			current.IsNew = true
		case line == "+++ /dev/null":
			current.IsDeleted = true
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.Insertions++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
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
