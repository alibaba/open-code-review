// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const mcpScratchExclude = ".scratch/**"

// prepareMCPRange resolves a range to immutable commit SHAs and performs the
// same merge-base diff preflight used by the range provider. The generic MCP
// server still accepts commit/workspace requests for compatibility; the
// code-review skill always uses this range path.
func prepareMCPRange(repoDir string, input ocrReviewInput) (ocrReviewInput, bool, error) {
	if input.From == "" {
		return input, false, nil
	}
	baseState, err := loadBaseState(repoDir)
	if err != nil {
		return ocrReviewInput{}, false, err
	}
	to := input.To
	if to == "" {
		to = "HEAD"
	}
	fromSHA, err := resolveMCPCommit(repoDir, input.From)
	if err != nil {
		return ocrReviewInput{}, false, fmt.Errorf("resolve range from: %w", err)
	}
	if !strings.EqualFold(baseState.BaseSHA, fromSHA) {
		return ocrReviewInput{}, false, fmt.Errorf("review range from %q does not match .scratch/base base_sha %q", fromSHA, baseState.BaseSHA)
	}
	toSHA, err := resolveMCPCommit(repoDir, to)
	if err != nil {
		return ocrReviewInput{}, false, fmt.Errorf("resolve range to: %w", err)
	}
	if out, err := runGitCmdStdout(repoDir, "merge-base", "--is-ancestor", "--end-of-options", fromSHA, toSHA); err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = "from is not an ancestor of to"
		}
		return ocrReviewInput{}, false, fmt.Errorf("invalid review range %s..%s: %s", fromSHA, toSHA, message)
	}
	mergeBase, err := runGitCmdStdout(repoDir, "merge-base", "--end-of-options", fromSHA, toSHA)
	if err != nil {
		return ocrReviewInput{}, false, fmt.Errorf("compute review merge-base: %w", err)
	}
	mergeBaseSHA := strings.TrimSpace(string(mergeBase))
	if mergeBaseSHA == "" {
		return ocrReviewInput{}, false, errors.New("compute review merge-base: git returned an empty SHA")
	}
	changed, err := mcpRangeHasChanges(repoDir, mergeBaseSHA, toSHA)
	if err != nil {
		return ocrReviewInput{}, false, err
	}
	input.From = fromSHA
	input.To = toSHA
	return input, !changed, nil
}

func resolveMCPCommit(repoDir, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" || strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("invalid commit ref %q", ref)
	}
	out, err := runGitCmdStdout(repoDir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%q is not a valid commit ref: %w", ref, err)
	}
	sha := strings.TrimSpace(string(out))
	if len(sha) != 40 {
		return "", fmt.Errorf("%q resolved to invalid SHA %q", ref, sha)
	}
	return sha, nil
}

func mcpRangeHasChanges(repoDir, from, to string) (bool, error) {
	args := []string{
		"-C", repoDir,
		"-c", "core.quotepath=false",
		"diff", "--quiet", "--no-ext-diff", "--no-textconv",
		"--end-of-options", from, to, "--", ".", ":(exclude)" + mcpScratchExclude,
	}
	cmd := exec.Command("git", args...)
	if err := cmd.Run(); err == nil {
		return false, nil
	} else {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("check review range diff: %w", err)
	}
}

func emptyRangeReviewResult() []byte {
	return []byte(`{"status":"skipped","message":"No supported files changed.","comments":[],"tool_calls":{"total":0,"by_tool":{}}}`)
}
