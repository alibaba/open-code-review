package diff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
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
		out, err := p.runGit(ctx, "diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--end-of-options", base, p.to, "--")
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
		combined.WriteString(out)

	case ModeCommit:
		out, err := p.runGit(ctx, "show", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--end-of-options", p.commit)
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
	for _, line := range strings.Split(string(data), "\n") {
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
func (p *Provider) isPathExcluded(relPath string, gitignorePatterns []string) bool {
	// Hardcoded directory prefix checks
	for _, prefix := range providerDirIgnoreDirs {
		dirPart := strings.TrimSuffix(prefix, "/")
		if relPath == dirPart || strings.HasPrefix(relPath, prefix) {
			return true
		}
	}

	// .gitignore pattern matching
	for _, pat := range gitignorePatterns {
		if matchGitignorePattern(relPath, pat) {
			return true
		}
	}
	return false
}

// matchGitignorePattern checks if relPath matches a single .gitignore pattern.
func matchGitignorePattern(relPath, pat string) bool {
	// Directory-only patterns (trailing /)
	if strings.HasSuffix(pat, "/") {
		dirName := strings.TrimSuffix(pat, "/")
		// Match if any path segment equals the dir name
		segments := strings.Split(relPath, "/")
		for _, seg := range segments {
			if seg == dirName {
				return true
			}
		}
		return false
	}

	// Negation patterns are not needed for exclusion purposes
	if strings.HasPrefix(pat, "!") {
		return false
	}

	// Patterns without / match basename
	if !strings.Contains(pat, "/") {
		base := filepath.Base(relPath)
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
		return false
	}

	// Patterns with / match against the full relative path
	if matched, _ := filepath.Match(pat, relPath); matched {
		return true
	}
	// Also try matching against suffix of path
	if strings.HasSuffix(relPath, pat) {
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
	out, err := p.runGit(ctx, "diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "HEAD", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--")
	if err == nil && out != "" {
		return out, nil
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return p.runGit(ctx, "diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", "--staged", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--")
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
	out, err := p.runGit(ctx, "ls-files", "--others", "--exclude-standard")
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
