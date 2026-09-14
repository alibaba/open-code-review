// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitVersion holds a parsed MAJOR.MINOR.PATCH git version.
type GitVersion struct {
	Major int
	Minor int
	Patch int
}

// gitVersionMin is the minimum git version open-code-review supports.
var gitVersionMin = GitVersion{Major: 2, Minor: 41, Patch: 0}

func (v GitVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast reports whether v is greater than or equal to o.
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

type VersionTooOldError struct {
	Current GitVersion
	Minimum GitVersion
}

func (e *VersionTooOldError) Error() string {
	return fmt.Sprintf(
		"git %s is older than the minimum supported version %s; consider upgrading git",
		e.Current, e.Minimum,
	)
}

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

func CheckGitVersion() error {
	return checkGitVersion(func() ([]byte, error) {
		return exec.Command("git", "--version").Output()
	})
}

func checkGitVersion(getVersion func() ([]byte, error)) error {
	out, err := getVersion()
	if err != nil {
		return fmt.Errorf("running git --version: %w", err)
	}
	v, err := ParseGitVersion(string(out))
	if err != nil {
		return err
	}
	if !v.AtLeast(gitVersionMin) {
		return &VersionTooOldError{Current: v, Minimum: gitVersionMin}
	}
	return nil
}
