// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestCompleteSessionIDs_WithSession drives the success loop of the shell
// completion helper against a real fixture session, covering the prefix-match
// and no-match branches that the fresh-repo test cannot reach.
func TestCompleteSessionIDs_WithSession(t *testing.T) {
	newCmd := func(repo string) *cobra.Command {
		c := &cobra.Command{}
		c.Flags().String("repo", repo, "")
		return c
	}

	t.Run("lists matching session IDs", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir, "a.go")

		got, _ := completeSessionIDs(newCmd(repoDir), nil, id[:4])
		if len(got) == 0 {
			t.Fatalf("expected a completion for session %s, got none", id)
		}
	})

	t.Run("prefix that matches nothing yields empty list", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		writeRangeResumeSession(t, repoDir, "a.go")

		got, _ := completeSessionIDs(newCmd(repoDir), nil, "zzzz-no-match")
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}
