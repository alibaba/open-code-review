package diff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	ModeWorkspace   Mode = iota // current workspace (staged + unstaged + untracked)
	ModeCommit                  // single commit vs its parent
	ModeRange                   // merge-base(from,to)..to
	ModeMultiCommit             // multiple commits combined
)

// Provider retrieves and parse git diffs from a repository.
type Provider struct {
	repoDir string
	mode    Mode

	// Range mode parameters
	from, to string // from/to refs for range comparison

	// Commit mode parameter
	commit string // single commit hash/ref

	// Multi-commit mode parameters
	commits []string // multiple commit hashes for combined review

	mergeBase string // cached common ancestor for range mode
}

// NewProvider creates a Provider for range mode: from..to (via merge-base).
func NewProvider(repoDir, from, to string) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeRange,
		from:    from,
		to:      to,
	}
}

// NewCommitProvider creates a Provider for commit mode: show changes introduced by a single commit.
func NewCommitProvider(repoDir, commit string) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeCommit,
		commit:  commit,
	}
}

// NewMultiCommitProvider creates a Provider for multi-commit mode: combined changes from multiple commits.
func NewMultiCommitProvider(repoDir string, commits []string) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeMultiCommit,
		commits: commits,
	}
}

// NewWorkspaceProvider creates a Provider for workspace mode (current uncommitted changes).
func NewWorkspaceProvider(repoDir string) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeWorkspace,
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
func (p *Provider) MergeBase() string {
	if p.mode != ModeRange || p.mergeBase != "" {
		return p.mergeBase
	}
	p.mergeBase = p.computeMergeBase(p.from, p.to)
	return p.mergeBase
}

// GetDiff returns all changes as parsed model.Diff structs.
func (p *Provider) GetDiff() ([]model.Diff, error) {
	var combined strings.Builder

	switch p.mode {
	case ModeRange:
		base := p.MergeBase()
		if base == "" {
			return nil, fmt.Errorf("cannot find merge-base between %s and %s", p.from, p.to)
		}
		out, err := p.runGit("diff", "--no-color", "-U"+fmt.Sprint(DiffContextLines), base, p.to, "--")
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
		combined.WriteString(out)

	case ModeCommit:
		out, err := p.runGit("show", "--no-color", "-U"+fmt.Sprint(DiffContextLines), p.commit)
		if err != nil {
			return nil, fmt.Errorf("git show failed: %w", err)
		}
		combined.WriteString(out)

	case ModeMultiCommit:
		return p.getMultiCommitDiff()

	case ModeWorkspace:
		tracked, err := p.workspaceTrackedDiff()
		if err != nil {
			return nil, fmt.Errorf("workspace tracked diff failed: %w", err)
		}
		combined.WriteString(tracked)

		untracked, err := p.untrackedFileDiffs()
		if err != nil {
			return nil, fmt.Errorf("untracked file diff failed: %w", err)
		}
		for _, ud := range untracked {
			combined.WriteString(ud)
			combined.WriteString("\n\n")
		}
	}

	diffs, err := ParseDiffText(combined.String(), p.repoDir)
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

// getMultiCommitDiff generates a combined diff from multiple commits by running
// git show for each commit individually, parsing the results, and merging them.
func (p *Provider) getMultiCommitDiff() ([]model.Diff, error) {
	diffSets := make([][]model.Diff, 0, len(p.commits))

	for _, c := range p.commits {
		out, err := p.runGit("show", "--no-color", "-U"+fmt.Sprint(DiffContextLines), c)
		if err != nil {
			return nil, fmt.Errorf("git show %s failed: %w", c, err)
		}
		diffs, err := ParseDiffText(out, p.repoDir)
		if err != nil {
			return nil, fmt.Errorf("parse diff for commit %s: %w", c, err)
		}
		diffSets = append(diffSets, diffs)
	}

	merged := MergeDiffs(diffSets...)
	return p.filterDiffs(merged), nil
}

func (p *Provider) computeMergeBase(from, to string) string {
	out, err := p.runGit("merge-base", from, to)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (p *Provider) workspaceTrackedDiff() (string, error) {
	out, err := p.runGit("diff", "HEAD", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--")
	if err == nil && out != "" {
		return out, nil
	}
	return p.runGit("diff", "--staged", "--no-color", "-U"+fmt.Sprint(DiffContextLines), "--")
}

func (p *Provider) untrackedFileDiffs() ([]string, error) {
	files, err := p.untrackedFilesList()
	if err != nil {
		return nil, err
	}

	var results []string
	for _, f := range files {
		fullPath := filepath.Join(p.repoDir, f)
		stat, serr := os.Stat(fullPath)
		if serr != nil || stat.IsDir() {
			continue
		}
		content, rerr := os.ReadFile(fullPath)
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

func (p *Provider) untrackedFilesList() ([]string, error) {
	out, err := p.runGit("ls-files", "--others", "--exclude-standard")
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

func (p *Provider) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = p.repoDir
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err != nil {
		return outStr, err
	}
	return outStr, nil
}
