// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/vcs"
	vcsGit "github.com/alibaba/open-code-review/internal/vcs/git"
)

// binarySniffWindow is the number of leading bytes inspected to decide
// whether a file is binary. Matches common heuristics (git, less).
const binarySniffWindow = 8000

// DefaultMaxFileSizeBytes is the default hard cap on how large a single
// file may be before the scanner skips it. The real review-feasibility
// limit is the per-file token budget (selectScanItems, ~188 KB at
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
	files            vcs.FileSource
	maxFileSizeBytes int64
}

// NewProvider creates a Provider that enumerates the repository at repoDir.
// If paths is non-empty each element must be a repo-relative path (file or
// directory); only matching files are returned. maxFileSizeBytes <= 0 falls
// back to DefaultMaxFileSizeBytes.
func NewProvider(repoDir string, paths []string, runner *gitcmd.Runner, maxFileSizeBytes int64) *Provider {
	return NewProviderWithSource(repoDir, paths, vcsGit.New(repoDir, runner), maxFileSizeBytes)
}

// NewProviderWithSource enumerates workspace files through source. Local file
// metadata still determines whether an entry is regular and exceeds the cap.
func NewProviderWithSource(repoDir string, paths []string, source vcs.FileSource, maxFileSizeBytes int64) *Provider {
	if source == nil {
		source = vcsGit.New(repoDir, nil)
	}
	cleaned := make([]string, 0, len(paths))
	rootSelected := false
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Normalize: strip leading "./" and trailing "/" so prefix matching
		// against `git ls-files` output (which never has leading "./") works.
		// ToSlash runs first: on Windows `.\dir` only becomes `./dir` here, and
		// trimming the prefix before the conversion leaves it in place to match
		// nothing.
		p = filepath.ToSlash(p)
		for strings.HasPrefix(p, "./") {
			p = strings.TrimPrefix(p, "./")
		}
		p = strings.TrimSuffix(p, "/")
		// "." and "./" name the repository root, which is what an omitted --path
		// already means. Kept as a selector they match nothing, because
		// `git ls-files` prints `main.go` and never `./main.go`.
		if p == "" || p == "." {
			rootSelected = true
			continue
		}
		cleaned = append(cleaned, p)
	}
	// A root selector widens the scan to the whole repository, so any narrower
	// selector alongside it is already covered.
	if rootSelected {
		cleaned = nil
	}
	if maxFileSizeBytes <= 0 {
		maxFileSizeBytes = DefaultMaxFileSizeBytes
	}
	return &Provider{
		repoDir:          repoDir,
		paths:            cleaned,
		files:            source,
		maxFileSizeBytes: maxFileSizeBytes,
	}
}

// Enumerate returns one ScanItem per reviewable file. Binaries are emitted
// with empty Content + IsBinary=true so previews can show them as excluded.
func (p *Provider) Enumerate(ctx context.Context) ([]model.ScanItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files, err := p.files.ListFiles(ctx, vcs.ContentRef{})
	if err != nil {
		return nil, err
	}

	if len(p.paths) > 0 {
		files, err = p.filterByPaths(files)
		if err != nil {
			return nil, err
		}
	}

	gitignorePatterns := diff.LoadGitignorePatterns(p.repoDir)

	var out []model.ScanItem
	for _, rel := range files {
		// Per-iteration cancellation check: a large repo with thousands of
		// files may take seconds to walk, and downstream Lstat / ReadFile
		// each cost a syscall — abort early when ctx is cancelled.
		if err := ctx.Err(); err != nil {
			return nil, err
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
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > p.maxFileSizeBytes {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: skipping %s (%d bytes exceeds %d-byte scan limit; raise MaxTokens if the real concern is token budget, not memory)\n",
				rel, info.Size(), p.maxFileSizeBytes)
			continue
		}
		var content []byte
		var binary bool
		err = p.files.ReadFile(ctx, vcs.ContentRef{}, rel, func(r io.Reader) error {
			var readErr error
			content, binary, readErr = readScanContent(r, p.maxFileSizeBytes)
			return readErr
		})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err != nil {
			if errors.Is(err, errScanFileTooLarge) {
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: skipping %s (content exceeds %d-byte scan limit; raise MaxTokens if the real concern is token budget, not memory)\n", rel, p.maxFileSizeBytes)
				continue
			}
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read %s: %v\n", rel, err)
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
		out = append(out, model.ScanItem{
			Path:      rel,
			Content:   string(content),
			IsBinary:  false,
			LineCount: countLines(content),
		})
	}
	return out, nil
}

var errScanFileTooLarge = errors.New("file exceeds scan size limit")

// readScanContent stops after the binary sniff for binary placeholders and
// bounds text reads even if a workspace file grows after its metadata check.
func readScanContent(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	limited := &io.LimitedReader{R: r, N: maxBytes}
	prefix := make([]byte, binarySniffWindow)
	n, err := io.ReadFull(limited, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, false, err
	}
	prefix = prefix[:n]
	if bytes.IndexByte(prefix, 0) >= 0 {
		return nil, true, nil
	}
	rest, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if limited.N == 0 {
		var extra [1]byte
		n, err := io.ReadFull(r, extra[:])
		if n != 0 {
			return nil, false, errScanFileTooLarge
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, false, err
		}
	}
	return append(prefix, rest...), false, nil
}

// filterByPaths keeps only entries named by a --path selector: the file
// itself, or everything under it for a directory.
//
// The selector's own spelling is tried first, so on a case-sensitive
// filesystem `--path src` names `src/` alone even when a sibling `SRC/`
// exists. A selector that names nothing in its own spelling is then resolved
// against the working tree, which decides what the spelling names: symlinks
// are followed and, on a case-insensitive filesystem, the answer carries the
// on-disk case, so `--path SRC` names the tracked `src/` on Windows and
// macOS. Only the resolved path is matched, and it is matched exactly, so a
// selector can never widen to a case-variant sibling that the filesystem did
// not name -- an empty `src/docs/` selects nothing even while `src/DOCS/`
// holds tracked files.
//
// A selector that resolves but matches no enumerated entry (an ignored path,
// say) stays a clean, empty selection. One the working tree does not resolve
// is an error -- `--path` names a path, not a search pattern, so a mistyped or
// wrongly-cased selector must not pass for a clean scan of zero files.
func (p *Provider) filterByPaths(all []string) ([]string, error) {
	keep := make([]bool, len(all))
	for _, want := range p.paths {
		exact := false
		for i, f := range all {
			if pathMatches(f, want) {
				keep[i] = true
				exact = true
			}
		}
		if exact {
			continue
		}
		resolved, err := p.resolveSelector(want)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("--path %q does not exist in the working tree", want)
			}
			return nil, fmt.Errorf("--path %q: %w", want, err)
		}
		for i, f := range all {
			if pathMatches(f, resolved) {
				keep[i] = true
			}
		}
	}
	var out []string
	for i, f := range all {
		if keep[i] {
			out = append(out, f)
		}
	}
	return out, nil
}

// resolveSelector returns the repo-relative path the working tree holds at
// the selector's spelling, or an error wrapping fs.ErrNotExist when nothing
// is there. The filesystem decides what resolves: symlinks are followed, and
// a case-insensitive filesystem answers with the on-disk spelling, so `SRC`
// names `src/` on Windows and macOS while a case-sensitive filesystem keeps
// the two apart. The result feeds exact matching, never folding, so a
// resolved selector cannot also drag in a case-variant sibling.
func (p *Provider) resolveSelector(want string) (string, error) {
	root, err := filepath.EvalSymlinks(p.repoDir)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(want)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	// A selector can leave the repository through `..` or an absolute path;
	// nothing tracked can match, and the clean skip stays.
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return rel, nil
	}
	// EvalSymlinks normalises case on Windows but not on macOS, where the
	// on-disk spelling of every component still has to come from the
	// directory listing.
	resolved := ""
	for _, part := range strings.Split(rel, "/") {
		name, err := onDiskName(filepath.Join(root, filepath.FromSlash(resolved)), part)
		if err != nil {
			return "", err
		}
		if resolved == "" {
			resolved = name
			continue
		}
		resolved += "/" + name
	}
	return resolved, nil
}

// onDiskName returns the name of the directory entry under dir that the
// filesystem resolves as name, preferring an entry spelled exactly. The
// listing is consulted because on macOS EvalSymlinks keeps the caller's case
// even where the filesystem matched a differently-cased entry.
func onDiskName(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Name() == name {
			return name, nil
		}
	}
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		ei, err := e.Info()
		if err != nil {
			continue
		}
		if os.SameFile(ei, info) {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("%s: no directory entry resolves %q", dir, name)
}

// pathMatches reports whether f is want itself or lies under it as a
// directory, so `src` also matches `src/loader.go` but never `srcx.go`.
func pathMatches(f, want string) bool {
	return f == want || strings.HasPrefix(f, want+"/")
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
