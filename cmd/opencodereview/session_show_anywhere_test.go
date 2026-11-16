// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
)

func TestRunSessionShow_FromAnotherDirectory(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	sh := session.New(repo, "feature", "test-model", session.SessionOptions{ReviewMode: session.ReviewModeWorkspace})
	sh.RecordReviewItemDone("done.go", "done.go", "done.go", "fp-done", nil)
	sh.RecordReviewItemFailed("failed.go", "failed.go", "failed.go", "fp-failed", "request timed out")
	sh.Finalize()
	path, err := session.SessionFilePath(repo, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	got := captureStdout(t, func() {
		if err := runSessionShowCompat([]string{sh.SessionID, "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var payload struct {
		Summary session.Summary      `json:"summary"`
		Items   []session.ItemDetail `json:"items"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Summary.SessionID != sh.SessionID || filepath.Clean(payload.Summary.RepoDir) != filepath.Clean(repo) || payload.Summary.FilePath != path {
		t.Fatalf("wrong session: %+v", payload.Summary)
	}
	if len(payload.Items) != 2 || payload.Items[0].FilePath != "done.go" || payload.Items[1].Error != "request timed out" {
		t.Fatalf("items = %+v", payload.Items)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("saved session changed: %v", err)
	}
}

func TestRunSessionShow_MissingCheckoutAndMetadata(t *testing.T) {
	for _, recorded := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing checkout", false: "missing repository metadata"}[recorded], func(t *testing.T) {
			setTestHome(t, t.TempDir())
			repo := filepath.Join(t.TempDir(), "removed-checkout")
			const id = "saved-session"
			var path string
			if recorded {
				path = seedRmSessionRecorded(t, repo, id, "feature")
			} else {
				path = seedRmSession(t, "", repo, id)
			}
			got := captureStdout(t, func() {
				if err := runSessionShowCompat([]string{id}); err != nil {
					t.Fatal(err)
				}
			})
			for _, want := range []string{id, path} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q in %s", want, got)
				}
			}
			if recorded && !strings.Contains(got, repo) {
				t.Fatalf("missing saved repository in %s", got)
			}
		})
	}
}

func TestRunSessionShow_AmbiguousIDRequiresRepo(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoA, repoB := t.TempDir(), t.TempDir()
	const id = "shared-session"
	seedRmSessionRecorded(t, repoA, id, "branch-a")
	seedRmSessionRecorded(t, repoB, id, "branch-b")
	t.Chdir(repoA)
	err := runSessionShowCompat([]string{id})
	if err == nil {
		t.Fatal("an ambiguous id must not select the current repository implicitly")
	}
	for _, want := range []string{repoA, repoB, "--repo"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not identify %q", err, want)
		}
	}
	got := captureStdout(t, func() {
		if err := runSessionShowCompat([]string{id, "--repo", repoB}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(got, "branch-b") || strings.Contains(got, "branch-a") {
		t.Fatalf("--repo selected the wrong session: %s", got)
	}
}

func TestRunSessionShow_RepoDoesNotFallBackToOtherRepositories(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoA, repoB := t.TempDir(), t.TempDir()
	seedRmSessionRecorded(t, repoA, "elsewhere", "main")
	if err := runSessionShowCompat([]string{"elsewhere", "--repo", repoB}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit lookup should fail instead of returning another repository: %v", err)
	}
}

func TestRunSessionShow_MissingAndInvalidIDs(t *testing.T) {
	setTestHome(t, t.TempDir())
	if err := runSessionShowCompat([]string{"missing"}); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("missing id: %v", err)
	}
	for _, id := range []string{"../escape", `a\b`, ".", "..", " "} {
		for _, args := range [][]string{{id}, {id, "--repo", t.TempDir()}} {
			if err := runSessionShowCompat(args); err == nil || errors.Is(err, os.ErrNotExist) || errors.Is(err, session.ErrSessionNotFound) {
				t.Fatalf("invalid id %q was treated as a filesystem lookup: %v", id, err)
			}
		}
	}
}

func TestSessionShow_CompletesIDsAcrossRepositories(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoA, repoB := t.TempDir(), t.TempDir()
	seedRmSessionRecorded(t, repoA, "saved-a", "main")
	seedRmSessionRecorded(t, repoB, "saved-b", "feature")
	for _, tc := range []struct {
		repo string
		want int
	}{{"", 2}, {repoB, 1}} {
		cmd := &cobra.Command{}
		cmd.Flags().String("repo", tc.repo, "")
		got, directive := sessionShowCmd.ValidArgsFunction(cmd, nil, "saved-")
		if len(got) != tc.want || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Fatalf("completion for --repo %q = %v (%v)", tc.repo, got, directive)
		}
		if tc.repo != "" && !strings.HasPrefix(got[0], "saved-b\t") {
			t.Fatalf("completion ignored --repo: %v", got)
		}
	}
}
