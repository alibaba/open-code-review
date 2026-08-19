// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// End-to-end coverage of #928: `--audience human --format json` used to run
// completely silent because the json format discarded stdout outright, ignoring
// the audience. Progress now goes to stderr so the human sees the run live
// while stdout stays a single parseable document.

// runReviewWithAudience drives a full review through runReview and returns
// (stdout, stderr, error) so both halves of the contract can be asserted at
// once: what a pipe consumer receives, and what the terminal shows.
func runReviewWithAudience(t *testing.T, repoDir, format, audience string) (string, string, error) {
	t.Helper()
	var err error
	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() {
			err = runReview([]string{
				"--repo", repoDir, "--from", "HEAD~1", "--to", "HEAD",
				"--format", format, "--audience", audience,
			})
		})
	})
	return out, errOut, err
}

func TestReviewE2E_JSONHumanStreamsProgressToStderr(t *testing.T) {
	repoDir := retryTestRepo(t)
	startFakeLLM(t, newFakeLLM())

	out, errOut, err := runReviewWithAudience(t, repoDir, "json", "human")
	if err != nil {
		t.Fatalf("review must succeed: %v\nstderr: %s", err, errOut)
	}

	// The regression itself: the run must not be silent.
	if !strings.Contains(errOut, "[ocr]") {
		t.Errorf("--audience human must stream [ocr] progress to stderr, got:\n%s", errOut)
	}

	// stdout must remain exactly one JSON document. Unmarshal is the real
	// assertion a consumer like jq cares about; a single leaked progress line
	// would break it.
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must be parseable JSON, got error %v for:\n%s", err, out)
	}
	if strings.Contains(out, "[ocr]") {
		t.Errorf("no progress line may reach stdout:\n%s", out)
	}
}

// TestReviewE2E_JSONAgentStaysSilent is the counterpart: audience=agent asked
// for no progress, so the stderr redirect must not resurrect it.
func TestReviewE2E_JSONAgentStaysSilent(t *testing.T) {
	repoDir := retryTestRepo(t)
	startFakeLLM(t, newFakeLLM())

	out, errOut, err := runReviewWithAudience(t, repoDir, "json", "agent")
	if err != nil {
		t.Fatalf("review must succeed: %v\nstderr: %s", err, errOut)
	}

	if strings.Contains(errOut, "[ocr]   ▶") {
		t.Errorf("audience=agent must not emit tool progress, got:\n%s", errOut)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must be parseable JSON, got error %v for:\n%s", err, out)
	}
}
