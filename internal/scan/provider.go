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

// maxScanFileBytes is the hard cap on how large a single file may be
// before the scanner skips it. Larger files almost always blow past the
// per-file token budget anyway and reading them just wastes memory.
const maxScanFileBytes = 5 << 20 // 5 MiB

// Provider enumerates source files in a repository for full-file review.
// Unlike diff.Provider it produces no unified diffs — each ScanItem carries
// the full file content via Content, and binaries are emitted as placeholder
// entries (Content empty, IsBinary=true) so callers can still surface them
// in previews without spending memory on their bytes.
type Provider struct {
	repoDir string
	paths   []string // empty = whole repo
	runner  *gitcmd.Runner
}

// NewProvider creates a Provider that enumerates the repository at repoDir.
// If paths is non-empty each element must be a repo-relative path (file or
// directory); only matching files are returned.
func NewProvider(repoDir string, paths []string, runner *gitcmd.Runner) *Provider {
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
	return &Provider{
		repoDir: repoDir,
		paths:   cleaned,
		runner:  runner,
	}
}

// Enumerate returns one ScanItem per reviewable file. Binaries are emitted
// with empty Content + IsBinary=true so previews can show them as excluded.
func (p *Provider) Enumerate(ctx context.Context) ([]model.ScanItem, error) {
	files, err := p.listFiles(ctx)
	if err != nil {
		return nil, err
	}

	if len(p.paths) > 0 {
		files = filterByPaths(files, p.paths)
	}

	gitignorePatterns := diff.LoadGitignorePatterns(p.repoDir)

	var out []model.ScanItem
	for _, rel := range files {
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
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxScanFileBytes {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: skipping %s (%d bytes exceeds %d-byte scan limit)\n",
				rel, info.Size(), maxScanFileBytes)
			continue
		}
		binary, err := isBinaryFile(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot sniff %s: %v\n", rel, err)
			continue
		}
		if binary {
			// Emit placeholder so preview can display [B], but do not
			// read the file body — saves memory on large binaries.
			out = append(out, model.ScanItem{
				Path:     rel,
				IsBinary: true,
			})
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read %s: %v\n", rel, err)
			continue
		}
		out = append(out, model.ScanItem{
			Path:      rel,
			Content:   string(content),
			IsBinary:  false,
			LineCount: countLines(content),
		})
	}
	return out, nil
}

// listFiles returns all tracked + untracked (non-ignored) files in the repo.
func (p *Provider) listFiles(ctx context.Context) ([]string, error) {
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

func (p *Provider) gitLs(ctx context.Context, args ...string) ([]string, error) {
	cmdArgs := append([]string{"-c", "core.quotepath=false", "ls-files"}, args...)
	var out string
	var err error
	if p.runner != nil {
		out, err = p.runner.Run(ctx, p.repoDir, cmdArgs...)
	} else {
		cmd := exec.CommandContext(ctx, "git", cmdArgs...)
		cmd.Dir = p.repoDir
		raw, runErr := cmd.CombinedOutput()
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

// isBinaryFile reads up to binarySniffWindow bytes from path and reports
// whether they contain a NUL byte (git's "binary" heuristic).
func isBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, binarySniffWindow)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}
