package reviewbundle

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

// TargetSpec is the user-selected review target. An empty spec is workspace mode.
type TargetSpec struct {
	From   string
	To     string
	Commit string
}

// ResolveTarget validates refs and resolves a target to immutable Git identities.
func ResolveTarget(
	ctx context.Context,
	repoDir string,
	spec TargetSpec,
	runner *gitcmd.Runner,
) (Target, *WorkspaceState, error) {
	if err := validateTargetSpec(spec); err != nil {
		return Target{}, nil, err
	}

	switch {
	case spec.Commit != "":
		return resolveCommitTarget(ctx, repoDir, spec.Commit, runner)
	case spec.From != "":
		return resolveRangeTarget(ctx, repoDir, spec, runner)
	default:
		return resolveWorkspaceTarget(ctx, repoDir, runner)
	}
}

func validateTargetSpec(spec TargetSpec) error {
	if (spec.From == "") != (spec.To == "") {
		if spec.From == "" {
			return fmt.Errorf("--from is required when --to is specified")
		}
		return fmt.Errorf("--to is required when --from is specified")
	}
	if spec.Commit != "" && spec.From != "" {
		return fmt.Errorf("only one review mode allowed (--from/--to or --commit)")
	}
	for _, ref := range []struct {
		flag      string
		reference string
	}{
		{"--from", spec.From},
		{"--to", spec.To},
		{"--commit", spec.Commit},
	} {
		flag, reference := ref.flag, ref.reference
		if strings.HasPrefix(reference, "-") {
			return fmt.Errorf("%s value %q is not a valid git ref: refs must not start with '-'", flag, reference)
		}
	}
	return nil
}

func resolveWorkspaceTarget(
	ctx context.Context,
	repoDir string,
	runner *gitcmd.Runner,
) (Target, *WorkspaceState, error) {
	head, err := resolveCommit(ctx, repoDir, "HEAD", runner)
	if err != nil {
		return Target{}, nil, fmt.Errorf("resolve workspace HEAD: %w", err)
	}
	staged, err := gitOutput(ctx, runner, repoDir,
		"diff", "--cached", "--no-ext-diff", "--no-textconv", "--binary", "--no-color", "--")
	if err != nil {
		return Target{}, nil, fmt.Errorf("read staged diff: %w", err)
	}
	unstaged, err := gitOutput(ctx, runner, repoDir,
		"diff", "--no-ext-diff", "--no-textconv", "--binary", "--no-color", "--")
	if err != nil {
		return Target{}, nil, fmt.Errorf("read unstaged diff: %w", err)
	}
	untrackedHash, err := hashUntracked(ctx, repoDir, runner)
	if err != nil {
		return Target{}, nil, err
	}

	state := &WorkspaceState{
		HeadSHA:         head,
		StagedSHA256:    hashFields(staged),
		UnstagedSHA256:  hashFields(unstaged),
		UntrackedSHA256: untrackedHash,
	}
	return Target{
		Mode:    TargetWorkspace,
		BaseSHA: head,
		HeadSHA: head,
	}, state, nil
}

func resolveRangeTarget(
	ctx context.Context,
	repoDir string,
	spec TargetSpec,
	runner *gitcmd.Runner,
) (Target, *WorkspaceState, error) {
	from, err := resolveCommit(ctx, repoDir, spec.From, runner)
	if err != nil {
		return Target{}, nil, fmt.Errorf("--from value %q is not a valid commit ref: %w", spec.From, err)
	}
	head, err := resolveCommit(ctx, repoDir, spec.To, runner)
	if err != nil {
		return Target{}, nil, fmt.Errorf("--to value %q is not a valid commit ref: %w", spec.To, err)
	}
	mergeBaseBytes, err := gitOutput(
		ctx, runner, repoDir, "merge-base", "--end-of-options", from, head,
	)
	if err != nil {
		return Target{}, nil, fmt.Errorf("resolve merge-base between %s and %s: %w", spec.From, spec.To, err)
	}
	mergeBase := strings.TrimSpace(string(mergeBaseBytes))
	if mergeBase == "" {
		return Target{}, nil, fmt.Errorf("cannot find merge-base between %s and %s", spec.From, spec.To)
	}
	return Target{
		Mode:         TargetRange,
		From:         spec.From,
		To:           spec.To,
		BaseSHA:      mergeBase,
		HeadSHA:      head,
		MergeBaseSHA: mergeBase,
	}, nil, nil
}

func resolveCommitTarget(
	ctx context.Context,
	repoDir string,
	reference string,
	runner *gitcmd.Runner,
) (Target, *WorkspaceState, error) {
	head, err := resolveCommit(ctx, repoDir, reference, runner)
	if err != nil {
		return Target{}, nil, fmt.Errorf("--commit value %q is not a valid commit ref: %w", reference, err)
	}
	parentsBytes, err := gitOutput(ctx, runner, repoDir, "rev-list", "--parents", "-n", "1", head)
	if err != nil {
		return Target{}, nil, fmt.Errorf("resolve parent for commit %s: %w", reference, err)
	}
	fields := strings.Fields(string(parentsBytes))
	base := ""
	if len(fields) > 1 {
		base = fields[1]
	}
	return Target{
		Mode:    TargetCommit,
		Commit:  reference,
		BaseSHA: base,
		HeadSHA: head,
	}, nil, nil
}

func resolveCommit(
	ctx context.Context,
	repoDir string,
	reference string,
	runner *gitcmd.Runner,
) (string, error) {
	output, err := gitOutput(
		ctx, runner, repoDir, "rev-parse", "--verify", "--end-of-options", reference+"^{commit}",
	)
	if err != nil {
		return "", err
	}
	resolved := strings.TrimSpace(string(output))
	if resolved == "" {
		return "", fmt.Errorf("empty commit identity")
	}
	return resolved, nil
}

func hashUntracked(ctx context.Context, repoDir string, runner *gitcmd.Runner) (string, error) {
	output, err := gitOutput(
		ctx, runner, repoDir, "ls-files", "--others", "--exclude-standard", "-z",
	)
	if err != nil {
		return "", fmt.Errorf("list untracked files: %w", err)
	}
	paths := splitNUL(output)
	sort.Strings(paths)
	hasher := sha256.New()
	var length [8]byte
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		fullPath := filepath.Join(repoDir, filepath.FromSlash(path))
		info, statErr := os.Lstat(fullPath)
		if statErr != nil {
			return "", fmt.Errorf("stat untracked file %s: %w", path, statErr)
		}
		fileType := "regular"
		if info.Mode()&os.ModeSymlink != 0 {
			fileType = "symlink"
			target, readErr := os.Readlink(fullPath)
			if readErr != nil {
				return "", fmt.Errorf("read untracked symlink %s: %w", path, readErr)
			}
			writeHashFields(hasher, length, []byte(path), []byte(fileType), []byte(target))
		} else if info.Mode().IsRegular() {
			writeHashFields(hasher, length, []byte(path), []byte(fileType))
			binary.BigEndian.PutUint64(length[:], uint64(info.Size()))
			_, _ = hasher.Write(length[:])
			file, openErr := os.Open(fullPath)
			if openErr != nil {
				return "", fmt.Errorf("read untracked file %s: %w", path, openErr)
			}
			if _, copyErr := io.Copy(hasher, file); copyErr != nil {
				_ = file.Close()
				return "", fmt.Errorf("read untracked file %s: %w", path, copyErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				return "", fmt.Errorf("close untracked file %s: %w", path, closeErr)
			}
		} else {
			fileType = info.Mode().Type().String()
			writeHashFields(hasher, length, []byte(path), []byte(fileType), nil)
		}
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func splitNUL(content []byte) []string {
	raw := strings.Split(string(content), "\x00")
	paths := make([]string, 0, len(raw))
	for _, path := range raw {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func gitOutput(
	ctx context.Context,
	runner *gitcmd.Runner,
	repoDir string,
	arguments ...string,
) ([]byte, error) {
	if runner == nil {
		return nil, fmt.Errorf("git runner is required")
	}
	return runner.Output(ctx, repoDir, arguments...)
}

func hashFields(fields ...[]byte) string {
	hasher := sha256.New()
	var length [8]byte
	writeHashFields(hasher, length, fields...)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

func writeHashFields(hasher hash.Hash, length [8]byte, fields ...[]byte) {
	for _, field := range fields {
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write(field)
	}
}
