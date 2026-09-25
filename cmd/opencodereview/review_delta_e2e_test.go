// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runRangeReviewJSON runs a JSON range review and decodes its output.
func runRangeReviewJSON(t *testing.T, repoDir string, extra ...string) jsonOutput {
	t.Helper()
	args := append([]string{"--repo", repoDir, "--to", "HEAD", "--format", "json"}, extra...)
	var err error
	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() { err = runReview(args) })
	})
	if err != nil {
		t.Fatalf("review %v: %v\nstderr: %s", extra, err, errOut)
	}
	var got jsonOutput
	if e := json.Unmarshal([]byte(out), &got); e != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", e, out)
	}
	return got
}

// TestReviewE2E_DeltaFromReviewsOnlyChangedFiles is the motivating case for
// --delta-from: the author pushes a second version of a change that touches one
// of its four files. Only that file may reach the model; the other three are
// reused from the first review, and the manifest says so.
func TestReviewE2E_DeltaFromReviewsOnlyChangedFiles(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)

	// Both versions are reviewed against the same base, as a pull request would
	// be, so the unchanged files produce byte-identical per-file diffs.
	baseOut, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD~1").Output()
	if err != nil {
		t.Fatalf("rev-parse base: %v", err)
	}
	base := strings.TrimSpace(string(baseOut))

	first := runRangeReviewJSON(t, repoDir, "--from", base)
	if first.Manifest == nil || len(first.Manifest.Coverage.Completed) != len(markers) {
		t.Fatalf("first review must complete every file, manifest = %+v", first.Manifest)
	}
	before := srv.attemptCounts()

	body := "package p\n\n// changed again MARKER_BETA\nfunc b() int {\n\treturn 3\n}\n"
	if err := os.WriteFile(filepath.Join(repoDir, "b.go"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	retryTestGit(t, repoDir, "commit", "-q", "-am", "second version")

	second := runRangeReviewJSON(t, repoDir, "--from", base, "--delta-from", first.SessionID)

	after := srv.attemptCounts()
	for _, name := range markers {
		if name == "b.go" {
			if after[name] <= before[name] {
				t.Errorf("b.go changed and must be reviewed again; attempts %d -> %d", before[name], after[name])
			}
			continue
		}
		if after[name] != before[name] {
			t.Errorf("%s is unchanged and must not reach the model; attempts %d -> %d", name, before[name], after[name])
		}
	}

	m := second.Manifest
	if m == nil {
		t.Fatal("delta review emitted no manifest")
	}
	if len(m.Coverage.Reused) != len(markers)-1 || len(m.Coverage.Completed) != 1 || m.Coverage.Completed[0].Path != "b.go" {
		t.Errorf("coverage = reused %+v, completed %+v; want the three unchanged files reused and b.go completed",
			m.Coverage.Reused, m.Coverage.Completed)
	}
	if m.ParentRunID != first.SessionID {
		t.Errorf("parent_run_id = %q, want the first review %q", m.ParentRunID, first.SessionID)
	}
	if m.Input.SourceArtifactSHA256 == first.Manifest.Input.SourceArtifactSHA256 {
		t.Error("the second version must record its own input identity, not the parent's")
	}
	if second.Resume == nil || !second.Resume.Delta || second.Resume.ReusedFiles != int64(len(markers)-1) || second.Resume.RerunFiles != 1 {
		t.Errorf("resume = %+v, want a delta reusing 3 files and reviewing 1", second.Resume)
	}

	// The same change without --delta-from is exactly what --resume refuses.
	var resumeErr error
	captureStderr(t, func() {
		captureStdout(t, func() {
			resumeErr = runReview([]string{"--repo", repoDir, "--from", base, "--to", "HEAD", "--format", "json", "--resume", first.SessionID})
		})
	})
	if resumeErr == nil || !strings.Contains(resumeErr.Error(), "reviewed input changed") {
		t.Errorf("--resume across a changed input must still be rejected, got: %v", resumeErr)
	}
}

func TestValidateReviewOptionsDeltaFromConflicts(t *testing.T) {
	tests := []struct {
		name    string
		opts    reviewOptions
		wantErr string
	}{
		{"with resume", reviewOptions{from: "a", to: "b", deltaFrom: "s1", resume: "s2", outputFormat: "text"}, "--delta-from and --resume"},
		{"with preview", reviewOptions{from: "a", to: "b", deltaFrom: "s1", preview: true, outputFormat: "text"}, "--preview and --delta-from"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			err := validateReviewOptions(&opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error mentioning %q, got %v", tc.wantErr, err)
			}
		})
	}
}
