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

func getCommitMessage(repoDir, commit string) (string, error) {
	out, err := runGitCmd(repoDir, "log", "-1", "--format=%B", "--end-of-options", commit)
	if err != nil {
		return "", fmt.Errorf("git log failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
