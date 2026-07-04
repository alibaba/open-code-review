// Package scan implements `ocr scan` — full-file code review. It owns the
// file-enumeration provider, the per-file orchestrator, and the FULL_SCAN
// prompt-template plumbing. Shared LLM tool-use loop / memory compression
// lives in internal/llmloop; this package only handles scan-specific
// concerns (enumeration, FULL_SCAN_TASK rendering, scan-specific filter).
package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
)

// binarySniffWindow is the number of leading bytes inspected to decide
// whether a file is binary. Matches common heuristics (git, less).
const binarySniffWindow = 8000

// DefaultMaxFileSizeBytes is the default hard cap on how large a single
// file may be before the scanner skips it. The real review-feasibility
// limit is the per-file token budget (filterLargeScans, ~188 KB at
// MaxTokens=58888) — this byte cap exists only to stop us from reading
// multi-MB dumps into memory. Callers can override via NewProvider.
const DefaultMaxFileSizeBytes int64 = 2 << 20 // 2 MiB

// Provider enumerates source files in a repository for full-file review.
// Unlike diff.Provider it produces no unified diffs — each ScanItem carries
// the full file content via Content, and binaries are emitted as placeholder
// entries (Content empty, IsBinary=true) so callers can still surface them
// in previews without spending memory on their bytes.
type Provider struct {
	repoDir          string
	paths            []string // empty = whole repo
	runner           *gitcmd.Runner
	maxFileSizeBytes int64
}

// SkippedItem records a discovered scan path that could not be enumerated.
type SkippedItem struct {
	Path   string
	Reason string
}

// NewProvider creates a Provider that enumerates the repository at repoDir.
// If paths is non-empty each element must be a repo-relative path (file or
// directory); only matching files are returned. maxFileSizeBytes <= 0 falls
// back to DefaultMaxFileSizeBytes.
func NewProvider(repoDir string, paths []string, runner *gitcmd.Runner, maxFileSizeBytes int64) *Provider {
	cleaned := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Normalize: strip leading "./" and trailing "/" so prefix matching
		// against `git ls-files` output (which never has leading "./") works.
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimSuffix(p, "/")
		cleaned = append(cleaned, filepath.ToSlash(p))
	}
	if maxFileSizeBytes <= 0 {
		maxFileSizeBytes = DefaultMaxFileSizeBytes
	}
	return &Provider{
		repoDir:          repoDir,
		paths:            cleaned,
		runner:           runner,
		maxFileSizeBytes: maxFileSizeBytes,
	}
}

// Enumerate returns one ScanItem per reviewable file. Binaries are emitted
// with empty Content + IsBinary=true so previews can show them as excluded.
func (p *Provider) Enumerate(ctx context.Context) ([]model.ScanItem, error) {
	items, _, err := p.EnumerateDetailed(ctx)
	return items, err
}

// EnumerateDetailed returns review candidates and explicit provider-level skips.
func (p *Provider) EnumerateDetailed(
	ctx context.Context,
) ([]model.ScanItem, []SkippedItem, error) {
	files, err := p.listFiles(ctx)
	if err != nil {
		return nil, nil, err
	}

	if len(p.paths) > 0 {
		files = filterByPaths(files, p.paths)
	}

	gitignorePatterns := diff.LoadGitignorePatterns(p.repoDir)

	var out []model.ScanItem
	var skipped []SkippedItem
	for _, rel := range files {
		// Per-iteration cancellation check: a large repo with thousands of
		// files may take seconds to walk, and downstream Lstat / ReadFile
		// each cost a syscall — abort early when ctx is cancelled.
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if rel == "" {
			continue
		}
		if diff.IsPathExcluded(p.repoDir, rel, gitignorePatterns) {
			continue
		}
		full := filepath.Join(p.repoDir, rel)
		info, err := os.Lstat(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot stat %s: %v\n", rel, err)
			skipped = append(skipped, SkippedItem{Path: rel, Reason: "unreadable"})
			continue
		}
		if !info.Mode().IsRegular() {
			skipped = append(skipped, SkippedItem{Path: rel, Reason: "non_regular"})
			continue
		}
		if info.Size() > p.maxFileSizeBytes {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: skipping %s (%d bytes exceeds %d-byte scan limit; raise MaxTokens if the real concern is token budget, not memory)\n",
				rel, info.Size(), p.maxFileSizeBytes)
			skipped = append(skipped, SkippedItem{Path: rel, Reason: "file_size"})
			continue
		}
		binary, content, lineCount, err := readRegularFile(full, p.maxFileSizeBytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read %s: %v\n", rel, err)
			skipped = append(skipped, SkippedItem{Path: rel, Reason: "unreadable"})
			continue
		}
		if binary {
			out = append(out, model.ScanItem{
				Path:     rel,
				IsBinary: true,
			})
			continue
		}
		out = append(out, model.ScanItem{
			Path:      rel,
			Content:   string(content),
			IsBinary:  false,
			LineCount: lineCount,
		})
	}
	return out, skipped, nil
}

// listFiles returns all source files under repoDir. In a git repo it uses
// `git ls-files` for full .gitignore semantics (nested + global excludes +
// negation rules). In a non-git directory it falls back to filepath.WalkDir
// with the simpler in-process gitignore handling (root .gitignore + the
// internal ExcludedDirs blocklist).
func (p *Provider) listFiles(ctx context.Context) ([]string, error) {
	if p.isGitRepo(ctx) {
		return p.listFilesViaGit(ctx)
	}
	return p.listFilesViaWalk(ctx)
}

// isGitRepo reports whether p.repoDir is inside a git working tree.
func (p *Provider) isGitRepo(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", p.repoDir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func (p *Provider) listFilesViaGit(ctx context.Context) ([]string, error) {
	tracked, err := p.gitLs(ctx, "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files (tracked): %w", err)
	}
	untracked, err := p.gitLs(ctx, "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files (untracked): %w", err)
	}

	seen := make(map[string]struct{}, len(tracked)+len(untracked))
	all := make([]string, 0, len(tracked)+len(untracked))
	for _, f := range append(tracked, untracked...) {
		if f == "" {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		all = append(all, f)
	}
	return all, nil
}

// listFilesViaWalk recursively walks p.repoDir collecting regular files.
// Honors:
//   - the internal ExcludedDirs blocklist (.git, node_modules, vendor, ...)
//   - the root .gitignore (simplified semantics; nested .gitignore is NOT
//     supported in this mode)
//
// Skips entire subtrees via filepath.SkipDir for performance.
func (p *Provider) listFilesViaWalk(ctx context.Context) ([]string, error) {
	gitignorePatterns := diff.LoadGitignorePatterns(p.repoDir)
	var files []string

	err := filepath.WalkDir(p.repoDir, func(path string, d os.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			// Abort the walk; filepath.WalkDir propagates this back as the
			// returned error so the caller sees ctx.Err().
			return cerr
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: walk error at %s: %v\n", path, err)
			return nil // continue walking; skip this entry
		}
		if path == p.repoDir {
			return nil
		}
		rel, relErr := filepath.Rel(p.repoDir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			// Skip the whole subtree if the dir itself is excluded.
			if diff.IsPathExcluded(p.repoDir, rel, gitignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}
		// Regular files only; skip symlinks / sockets / etc.
		if !d.Type().IsRegular() {
			return nil
		}
		if diff.IsPathExcluded(p.repoDir, rel, gitignorePatterns) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", p.repoDir, err)
	}
	return files, nil
}

func (p *Provider) gitLs(ctx context.Context, args ...string) ([]string, error) {
	cmdArgs := append([]string{"-c", "core.quotepath=false", "ls-files"}, args...)
	var out string
	var err error
	if p.runner != nil {
		out, err = p.runner.Run(ctx, p.repoDir, cmdArgs...)
	} else {
		cmd := exec.CommandContext(ctx, "git", cmdArgs...)
		cmd.Dir = p.repoDir
		// Use Output (stdout only), not CombinedOutput: with -z, git emits
		// NUL-delimited paths on stdout, and merging stderr in would corrupt
		// the filename parsing below.
		raw, runErr := cmd.Output()
		out, err = string(raw), runErr
	}
	if err != nil {
		return nil, err
	}
	raw := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	files := make([]string, 0, len(raw))
	for _, f := range raw {
		f = strings.TrimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// filterByPaths keeps only entries whose path equals a user-supplied path
// (for exact files) or lies under it (for directories).
func filterByPaths(all []string, paths []string) []string {
	var out []string
	for _, f := range all {
		for _, want := range paths {
			if f == want || strings.HasPrefix(f, want+"/") {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// countLines returns the number of lines in content. A file without a
// trailing newline still counts its final line. Empty input → 0.
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	n := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		n++
	}
	return n
}

// readRegularFile opens path once, sniffs for binary content, and reads the body.
func readRegularFile(path string, maxBytes int64) (binary bool, content []byte, lineCount int, err error) {
	file, err := os.Open(path)
	if err != nil {
		return false, nil, 0, err
	}
	defer file.Close()
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileSizeBytes
	}
	sniff := make([]byte, binarySniffWindow)
	read, err := io.ReadFull(file, sniff)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, nil, 0, err
	}
	if bytes.IndexByte(sniff[:read], 0) >= 0 {
		return true, nil, 0, nil
	}
	if int64(read) > maxBytes {
		return false, nil, 0, fmt.Errorf("file exceeds %d-byte scan limit", maxBytes)
	}
	var body bytes.Buffer
	if read > 0 {
		if _, err := body.Write(sniff[:read]); err != nil {
			return false, nil, 0, err
		}
	}
	remaining := maxBytes - int64(read)
	if _, err := io.Copy(&body, io.LimitReader(file, remaining+1)); err != nil {
		return false, nil, 0, err
	}
	content = body.Bytes()
	if int64(len(content)) > maxBytes {
		return false, nil, 0, fmt.Errorf("file exceeds %d-byte scan limit", maxBytes)
	}
	return false, content, countLines(content), nil
}
