package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runGitCmd(repoDir string, args ...string) ([]byte, error) {
	fullArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.Command("git", fullArgs...)
	return cmd.CombinedOutput()
}

// runGitCmdStdout is like runGitCmd but returns stdout only. Use it when the
// output is consumed as data (e.g. a resolved path) so git's stderr warnings
// (permissions, deprecations, config notices) can't pollute the result.
func runGitCmdStdout(repoDir string, args ...string) ([]byte, error) {
	fullArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.Command("git", fullArgs...)
	return cmd.Output()
}

func getCommitMessage(repoDir, commit string) (string, error) {
	out, err := runGitCmd(repoDir, "log", "-1", "--format=%B", "--end-of-options", commit)
	if err != nil {
		return "", fmt.Errorf("git log failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
