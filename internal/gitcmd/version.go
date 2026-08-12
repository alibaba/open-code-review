// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type GitVersion struct {
	Major int
	Minor int
	Patch int
}

var gitVersionMin = GitVersion{Major: 2, Minor: 41, Patch: 0}

var ErrGitVersionTooOld = errors.New("git version below minimum supported")

func (v GitVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v GitVersion) AtLeast(o GitVersion) bool {
	switch {
	case v.Major != o.Major:
		return v.Major > o.Major
	case v.Minor != o.Minor:
		return v.Minor > o.Minor
	default:
		return v.Patch >= o.Patch
	}
}

// ParseGitVersion extracts a MAJOR.MINOR.PATCH version from a git --version
func ParseGitVersion(s string) (GitVersion, error) {
	i := 0
	for i < len(s) && (s[i] < '0' || s[i] > '9') {
		i++
	}
	if i == len(s) {
		return GitVersion{}, fmt.Errorf("unrecognized git version %q", strings.TrimSpace(s))
	}
	var v GitVersion
	n, err := fmt.Sscanf(s[i:], "%d.%d.%d", &v.Major, &v.Minor, &v.Patch)
	if err != nil || n < 3 {
		return GitVersion{}, fmt.Errorf("unrecognized git version %q", strings.TrimSpace(s))
	}
	return v, nil
}

func versionWarning(current GitVersion) (string, bool) {
	if current.AtLeast(gitVersionMin) {
		return "", false
	}
	return fmt.Sprintf(
		"warning: git %s is older than the minimum supported version %s. "+
			"Some commands may not work correctly; consider upgrading git.\n",
		current, gitVersionMin,
	), true
}

func CheckGitVersion() (string, error) {
	return checkGitVersion(os.Stderr, func() ([]byte, error) {
		return exec.Command("git", "--version").Output()
	})
}

func checkGitVersion(w io.Writer, getVersion func() ([]byte, error)) (string, error) {
	out, err := getVersion()
	if err != nil {
		return "", fmt.Errorf("running git --version: %w", err)
	}
	v, err := ParseGitVersion(string(out))
	if err != nil {
		return "", err
	}
	if msg, ok := versionWarning(v); ok {
		fmt.Fprint(w, msg)
		return v.String(), fmt.Errorf("%w: %s", ErrGitVersionTooOld, v.String())
	}
	return v.String(), nil
}
