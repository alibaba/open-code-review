package diff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
)

// DiffContextLines defines the number of context lines around each changed hunk.
const DiffContextLines = 3

// providerDirIgnoreDirs: directory prefixes to always exclude from diff results.
var providerDirIgnoreDirs = []string{
	".idea/",
	".vscode/",
	".svn/",
	".git/",
	"vendor/",
	"node_modules/",
	"target/",
	".happypack/",
	".cachefile/",
	"_packages/",
	"rpm/",
	"pkgs/",
}

// Mode defines how the diff is retrieved.
type Mode int

const (
	ModeWorkspace Mode = iota // current workspace (staged + unstaged + untracked)
	ModeCommit                // single commit vs its parent
	ModeRange                 // merge-base(from,to)..to
)

// Provider retrieves and parse git diffs from a repository.
type Provider struct {
	repoDir string
	mode    Mode
	runner  *gitcmd.Runner

	// Range mode parameters
	from, to string // from/to refs for range comparison

	// Commit mode parameter
	commit string // single commit hash/ref

	mergeBase string // cached common ancestor for range mode
}

// NewProvider creates a Provider for range mode: from..to (via merge-base).
func NewProvider(repoDir, from, to string, runner *gitcmd.Runner) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeRange,
		from:    from,
		to:      to,
		runner:  runner,
	}
}

// NewCommitProvider creates a Provider for commit mode: show changes introduced by a single commit.
func NewCommitProvider(repoDir, commit string, runner *gitcmd.Runner) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeCommit,
		commit:  commit,
		runner:  runner,
	}
}

// NewWorkspaceProvider creates a Provider for workspace mode (current uncommitted changes).
func NewWorkspaceProvider(repoDir string, runner *gitcmd.Runner) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeWorkspace,
		runner:  runner,
	}
}

// IsRangeMode returns true when comparing two refs.
func (p *Provider) IsRangeMode() bool {
	return p.mode == ModeRange
}

// IsCommitMode returns true when analyzing a single commit.
func (p *Provider) IsCommitMode() bool {
	return p.mode == ModeCommit
}

// MergeBase returns the computed merge-base commit hash for range mode.
func (p *Provider) MergeBase(ctx context.Context) string {
	if p.mode != ModeRange || p.mergeBase != "" {
		return p.mergeBase
	}
	p.mergeBase = p.computeMergeBase(ctx, p.from, p.to)
	return p.mergeBase
}

// GetDiff returns all changes as parsed model.Diff structs.
func (p *Provider) GetDiff(ctx context.Context) ([]model.Diff, error) {
	var combined strings.Builder

	switch p.mode {
	case ModeRange:
		base := p.MergeBase(ctx)
		if base == "" {
			return nil, fmt.Errorf("cannot find merge-base between %s and %s", p.from, p.to)
		}
		out, err := p.runGit(ctx, "-c", "core.quotepath=false", "diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--end-of-options", base, p.to, "--")
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
		combined.WriteString(out)

	case ModeCommit:
		// --diff-merges=first-parent: for merge commits, plain `git show`
		// emits a combined diff ("diff --cc"), which ParseDiffText cannot
		// parse — the commit would silently yield zero reviewable diffs.
		// Diffs against the first parent instead, in regular unified format.
		out, err := p.runGit(ctx, "-c", "core.quotepath=false", "show", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--no-color", "--diff-merges=first-parent", "-U"+fmt.Sprint(DiffContextLines), "--end-of-options", p.commit)
		if err != nil {
			return nil, fmt.Errorf("git show failed: %w", err)
		}
		combined.WriteString(out)

	case ModeWorkspace:
		tracked, err := p.workspaceTrackedDiff(ctx)
		if err != nil {
			return nil, fmt.Errorf("workspace tracked diff failed: %w", err)
		}
		combined.WriteString(tracked)

		untracked, err := p.untrackedFileDiffs(ctx)
		if err != nil {
			return nil, fmt.Errorf("untracked file diff failed: %w", err)
		}
		for _, ud := range untracked {
			combined.WriteString(ud)
			combined.WriteString("\n\n")
		}
	}

	var ref string
	switch p.mode {
	case ModeRange:
		ref = p.to
	case ModeCommit:
		ref = p.commit
	}

	diffs, err := ParseDiffText(ctx, combined.String(), p.repoDir, ref, p.runner)
	if err != nil {
		return nil, err
	}
	return p.filterDiffs(diffs), nil
}

// loadGitignorePatterns reads and parses .gitignore patterns from the repo root.
func (p *Provider) loadGitignorePatterns() []string {
	data, err := os.ReadFile(filepath.Join(p.repoDir, ".gitignore"))
	if err != nil {
		return nil
	}
	var patterns []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// isPathExcluded returns true when the given relative file path should be skipped
// based on hardcoded dir rules or .gitignore patterns.
//
// Patterns are resolved the way git resolves them: in file order, with the LAST
// matching pattern deciding, and a leading "!" inverting that pattern's verdict.
// Order matters because the "allow list" idiom (ignore everything with `*`, then
// re-include with `!` lines — github/gitignore ships one per language) is only
// correct under last-match-wins. Treating negations as unmatchable made every
// file in such a repository look excluded, so a review silently covered nothing.
func (p *Provider) isPathExcluded(relPath string, gitignorePatterns []string) bool {
	// Hardcoded directory prefix checks. These are an unconditional blocklist:
	// a .gitignore negation cannot re-admit .git/ or node_modules/.
	for _, prefix := range providerDirIgnoreDirs {
		dirPart := strings.TrimSuffix(prefix, "/")
		if relPath == dirPart || strings.HasPrefix(relPath, prefix) {
			return true
		}
	}

	excluded := false
	for _, pat := range gitignorePatterns {
		body, negated := strings.CutPrefix(pat, "!")
		if body == "" {
			continue
		}

		// Directory-only patterns (trailing "/") apply to directories, never to
		// files. Git uses a negated one such as `!*/` to keep descending into
		// subdirectories, not to re-admit the files inside them — honouring it
		// here would readmit everything below the root.
		if negated && strings.HasSuffix(body, "/") {
			continue
		}

		if matchGitignoreBody(relPath, body) {
			excluded = !negated
		}
	}
	return excluded
}

// matchGitignorePattern checks if relPath matches a single .gitignore pattern.
//
// Polarity is not this function's concern: a negated pattern reports false, so
// callers testing one pattern in isolation still read it as "does this exclude
// the path". Ordered resolution across a whole pattern list, where negations do
// carry meaning, lives in isPathExcluded.
func matchGitignorePattern(relPath, pat string) bool {
	if strings.HasPrefix(pat, "!") {
		return false
	}
	return matchGitignoreBody(relPath, pat)
}

// matchGitignoreBody reports whether relPath matches a single pattern body —
// the pattern with any leading "!" already stripped.
func matchGitignoreBody(relPath, body string) bool {
	// Directory-only patterns (trailing /)
	if before, ok := strings.CutSuffix(body, "/"); ok {
		// Only a real directory component can match, so the final segment (the
		// file's own name) is excluded from consideration: `vendor/` must not
		// match a *file* named "vendor", and `*/` must not match every path.
		segments := strings.Split(relPath, "/")
		return slices.Contains(segments[:max(len(segments)-1, 0)], before)
	}

	// A leading "/" anchors the pattern to the repository root rather than
	// making it a path pattern; "/.golangci.yml" addresses the root file.
	anchored := false
	if trimmed, ok := strings.CutPrefix(body, "/"); ok {
		body, anchored = trimmed, true
	}

	// "**" is not expressible with filepath.Match, so patterns containing it go
	// through doublestar, which implements gitignore's globstar semantics.
	if strings.Contains(body, "**") {
		matched, err := doublestar.Match(body, relPath)
		return err == nil && matched
	}

	// Patterns without / match basename — unless anchored, where the pattern
	// addresses that name at the root only.
	if !strings.Contains(body, "/") {
		target := filepath.Base(relPath)
		if anchored {
			target = relPath
		}
		matched, _ := filepath.Match(body, target)
		return matched
	}

	// Patterns with / match against the full relative path
	if matched, _ := filepath.Match(body, relPath); matched {
		return true
	}
	// Also try matching against suffix of path, but not for anchored patterns:
	// "/docs/api.md" names one file, not any path ending that way.
	if !anchored && strings.HasSuffix(relPath, body) {
		return true
	}

	return false
}

// filterDiffs removes diffs whose file paths are excluded.
func (p *Provider) filterDiffs(diffs []model.Diff) []model.Diff {
	patterns := p.loadGitignorePatterns()
	var result []model.Diff
	for _, d := range diffs {
		path := d.NewPath
		if path == "/dev/null" {
			path = d.OldPath
		}
		if !p.isPathExcluded(path, patterns) {
			result = append(result, d)
		}
	}
	return result
}

// ---- Internal helpers ----

func (p *Provider) computeMergeBase(ctx context.Context, from, to string) string {
	out, err := p.runGit(ctx, "merge-base", "--end-of-options", from, to)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (p *Provider) workspaceTrackedDiff(ctx context.Context) (string, error) {
	out, err := p.runGit(ctx, "-c", "core.quotepath=false", "diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--end-of-options", "HEAD", "--")
	if err == nil && out != "" {
		return out, nil
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	// Fall back to the staged diff when `git diff HEAD` errored or was empty. This is
	// not redundant with the call above: in a repository with no commits yet there is no
	// HEAD, so `git diff HEAD` fails with "bad revision 'HEAD'", but `git diff --staged`
	// still surfaces staged changes by diffing the index against the empty tree — the only
	// way to review a workspace before its first commit.
	return p.runGit(ctx, "-c", "core.quotepath=false", "diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--staged", "--")
}

func (p *Provider) untrackedFileDiffs(ctx context.Context) ([]string, error) {
	files, err := p.untrackedFilesList(ctx)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, f := range files {
		content, rerr := readWorkspaceFileForDiff(p.repoDir, f)
		if rerr != nil {
			continue
		}

		lineCount := bytes.Count(content, []byte{'\n'})
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lineCount++
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", f, f))
		sb.WriteString("--- /dev/null\n")
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", f))
		sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", lineCount))

		lines := bytes.Split(content, []byte{'\n'})
		if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
			lines = lines[:len(lines)-1]
		}
		for _, line := range lines {
			sb.WriteByte('+')
			sb.Write(line)
			sb.WriteByte('\n')
		}
		results = append(results, sb.String())
	}
	return results, nil
}

func (p *Provider) untrackedFilesList(ctx context.Context) ([]string, error) {
	out, err := p.runGit(ctx, "-c", "core.quotepath=false", "ls-files", "--others", "--exclude-standard")
	if err != nil || out == "" {
		return nil, nil
	}
	patterns := p.loadGitignorePatterns()
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !p.isPathExcluded(line, patterns) {
			files = append(files, line)
		}
	}
	return files, nil
}

func (p *Provider) runGit(ctx context.Context, args ...string) (string, error) {
	if p.runner != nil {
		return p.runner.Run(ctx, p.repoDir, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = p.repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
