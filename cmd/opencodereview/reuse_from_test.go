// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// writeReuseSource writes a previous run's JSON output file in the exact shape
// the producer (jsonOutput) publishes, so the loader is tested against the real
// round trip rather than a hand-rolled lookalike.
func writeReuseSource(t *testing.T, name string, out jsonOutput) string {
	t.Helper()
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal reuse source: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write reuse source: %v", err)
	}
	return path
}

// validReuseOutput builds a source manifest with the given repository digest
// and coverage entries, mirroring what a completed range run publishes.
func validReuseOutput(identitySHA string, completed, reused []session.CoverageItem, comments []model.LlmComment) jsonOutput {
	return jsonOutput{
		Status:   "complete",
		Comments: comments,
		Manifest: &session.RunManifest{
			SchemaVersion: session.ManifestSchemaVersion,
			RunID:         "run-42",
			Operation:     session.OperationReview,
			TerminalState: session.StateComplete,
			Repository:    session.ManifestRepository{IdentitySHA256: identitySHA},
			Input:         session.ManifestInput{Mode: session.InputModeRange},
			Execution:     session.ManifestExecution{Model: "claude-old"},
			Coverage: session.Coverage{
				Selected:  append(append([]session.CoverageItem{}, completed...), reused...),
				Completed: completed,
				Reused:    reused,
			},
		},
	}
}

func TestLoadReuseStateValidJoin(t *testing.T) {
	// A plain temp dir has no origin remote, and the manifest records no
	// repository identity: empty versus empty is a match.
	repoDir := t.TempDir()
	completed := []session.CoverageItem{
		{ItemID: "id-a", Path: "a.go", Fingerprint: "fp-a"},
		{ItemID: "id-renamed", Path: "moved.go", OldPath: "old.go", Fingerprint: "fp-renamed"},
		{ItemID: "id-empty-fp", Path: "no-fp.go", Fingerprint: ""},
	}
	reused := []session.CoverageItem{
		{ItemID: "id-b", Path: "b.go", Fingerprint: "fp-b"},
		// A settled item with zero comments: no comment list is not "no verdict".
		{ItemID: "id-c", Path: "c.go", Fingerprint: "fp-c"},
	}
	comments := []model.LlmComment{
		{Path: "a.go", Content: "first about a.go"},
		{Path: "b.go", Content: "about b.go"},
		{Path: "a.go", Content: "second about a.go"},
		// A comment for a path no coverage item settled is not a checkpoint;
		// the join is driven by coverage, not by the comment list.
		{Path: "uncovered.go", Content: "orphan"},
	}
	path := writeReuseSource(t, "ocr-result.json", validReuseOutput("", completed, reused, comments))

	state, warnings := loadReuseState(context.Background(), repoDir, path)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if state == nil {
		t.Fatal("expected a reuse state")
	}
	if state.SessionID != "reuse:run-42" {
		t.Errorf("SessionID = %q, want reuse:run-42", state.SessionID)
	}
	if !state.Closed || state.Model != "claude-old" || state.Manifest == nil || state.Manifest.RunID != "run-42" {
		t.Errorf("state = %+v manifest = %+v, want closed state carrying the source manifest", state, state.Manifest)
	}
	if got := len(state.Items); got != 4 {
		t.Fatalf("items = %d, want 4 (empty fingerprints are skipped)", got)
	}
	a, ok := state.Items["fp-a"]
	if !ok {
		t.Fatal("fp-a missing from items")
	}
	if a.FilePath != "a.go" || a.NewPath != "a.go" || a.OldPath != "" || a.Fingerprint != "fp-a" {
		t.Errorf("fp-a item = %+v", a)
	}
	if len(a.Comments) != 2 || a.Comments[0].Content != "first about a.go" || a.Comments[1].Content != "second about a.go" {
		t.Errorf("fp-a comments = %+v, want both a.go comments grouped by exact path", a.Comments)
	}
	renamed := state.Items["fp-renamed"]
	if renamed.OldPath != "old.go" || renamed.NewPath != "moved.go" {
		t.Errorf("fp-renamed item = %+v, want old/new paths carried from coverage", renamed)
	}
	// A settled item with zero comments is still a reusable checkpoint.
	c := state.Items["fp-c"]
	if len(c.Comments) != 0 {
		t.Errorf("fp-c comments = %+v, want none", c.Comments)
	}
	// The coverage the loader recorded must make each item reusable.
	if _, ok := state.ReusableItem("fp-a"); !ok {
		t.Error("fp-a must be reusable through the resume engine's coverage gate")
	}
	if _, ok := state.ReusableItem("fp-missing"); ok {
		t.Error("an unknown fingerprint must not be reusable")
	}
}

func TestLoadReuseStateDuplicateFingerprintAcrossBucketsIsOneItem(t *testing.T) {
	repoDir := t.TempDir()
	item := session.CoverageItem{ItemID: "id-a", Path: "a.go", Fingerprint: "fp-a"}
	path := writeReuseSource(t, "ocr-result.json",
		validReuseOutput("", []session.CoverageItem{item}, []session.CoverageItem{item}, nil))

	state, warnings := loadReuseState(context.Background(), repoDir, path)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if got := len(state.Items); got != 1 {
		t.Fatalf("items = %d, want 1 for a duplicated fingerprint", got)
	}
}

func TestLoadReuseStateDegradations(t *testing.T) {
	repoDir := t.TempDir()
	ctx := context.Background()

	cases := []struct {
		name        string
		path        func(t *testing.T) string
		wantWarning string
	}{
		{
			name: "missing file",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist.json")
			},
			wantWarning: "cannot read reuse source",
		},
		{
			name: "invalid json",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "ocr-result.json")
				if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantWarning: "not a valid previous-run JSON output",
		},
		{
			name: "empty file is invalid json",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "ocr-result.json")
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantWarning: "not a valid previous-run JSON output",
		},
		{
			name: "manifest absent",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "ocr-result.json")
				if err := os.WriteFile(path, []byte(`{"status":"complete","comments":[]}`), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantWarning: "no manifest",
		},
		{
			name: "unsupported schema version",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "ocr-result.json")
				if err := os.WriteFile(path, []byte(`{"manifest":{"schema_version":"ocr.run-manifest/v0","run_id":"r","operation":"review"}}`), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantWarning: "unsupported manifest schema version",
		},
		{
			name: "repository identity mismatch",
			path: func(t *testing.T) string {
				completed := []session.CoverageItem{{ItemID: "id-a", Path: "a.go", Fingerprint: "fp-a"}}
				return writeReuseSource(t, "ocr-result.json",
					validReuseOutput(strings.Repeat("ab", 32), completed, nil, nil))
			},
			wantWarning: "different repository",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, warnings := loadReuseState(ctx, repoDir, tc.path(t))
			if state != nil {
				t.Errorf("state = %+v, want nil for a degraded source", state)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.wantWarning) {
				t.Errorf("warnings = %v, want one containing %q", warnings, tc.wantWarning)
			}
		})
	}
}

func TestLoadReuseStateIdentityDerivation(t *testing.T) {
	// The digest the loader compares is sha256-hex of the canonicalized origin
	// remote — the same value a run records in repository.identity_sha256.
	// Canonicalization drops the userinfo, lowercases the host and trims the
	// trailing ".git" while preserving path case.
	repoDir := initTestGitRepo(t)
	gitIn(t, repoDir, "remote", "add", "origin", "https://User@GitHub.com/acme/Widget.git")
	sum := sha256.Sum256([]byte("github.com/acme/Widget"))
	digest := hex.EncodeToString(sum[:])
	completed := []session.CoverageItem{{ItemID: "id-a", Path: "a.go", Fingerprint: "fp-a"}}

	t.Run("matching canonical identity is accepted", func(t *testing.T) {
		path := writeReuseSource(t, "ocr-result.json", validReuseOutput(digest, completed, nil, nil))
		state, warnings := loadReuseState(context.Background(), repoDir, path)
		if len(warnings) != 0 || state == nil {
			t.Fatalf("state = %v warnings = %v, want a loaded state with no warnings", state, warnings)
		}
	})

	t.Run("different remote is rejected", func(t *testing.T) {
		other := sha256.Sum256([]byte("github.com/acme/other"))
		path := writeReuseSource(t, "ocr-result.json",
			validReuseOutput(hex.EncodeToString(other[:]), completed, nil, nil))
		state, warnings := loadReuseState(context.Background(), repoDir, path)
		if state != nil || len(warnings) != 1 {
			t.Fatalf("state = %v warnings = %v, want nil state with one warning", state, warnings)
		}
	})

	t.Run("recorded identity without origin remote is rejected", func(t *testing.T) {
		// The current repo has an origin here, so an empty recorded digest must
		// not match; only empty-versus-empty is a match.
		path := writeReuseSource(t, "ocr-result.json", validReuseOutput("", completed, nil, nil))
		state, warnings := loadReuseState(context.Background(), repoDir, path)
		if state != nil || len(warnings) != 1 {
			t.Fatalf("state = %v warnings = %v, want nil state with one warning", state, warnings)
		}
	})
}

func TestLoadReuseStateSessionIDFallback(t *testing.T) {
	repoDir := t.TempDir()
	out := validReuseOutput("", []session.CoverageItem{{ItemID: "id-a", Path: "a.go", Fingerprint: "fp-a"}}, nil, nil)
	out.Manifest.RunID = "" // hand-edited file with no run_id
	path := writeReuseSource(t, "prev-run.json", out)

	state, warnings := loadReuseState(context.Background(), repoDir, path)
	if len(warnings) != 0 || state == nil {
		t.Fatalf("state = %v warnings = %v, want a loaded state with no warnings", state, warnings)
	}
	if want := "reuse:prev-run.json"; state.SessionID != want {
		t.Errorf("SessionID = %q, want %q (file base name fallback)", state.SessionID, want)
	}
}

// TestReuseFromNeverTriggersResumeGates pins the wiring invariant: a reuse-only
// run keeps opts.resume empty, so loadReviewResumeState (the sole caller of
// ResumeState.ValidateOptions) returns before loading anything and
// validateResumeIdentity (the sole caller of ValidateResume) sees a nil state.
// The synthetic reuse state therefore cannot be rejected by the strict gates
// that exist to reject the cross-push case reuse is designed for.
func TestReuseFromNeverTriggersResumeGates(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := initResumeRepo(t)
	// A deliberately bogus reuse source: even when the file is unusable the
	// strict gates must not fire, because the wiring never hands it to them.
	opts := reviewOptions{from: "HEAD~1", to: "HEAD", reuseFrom: filepath.Join(t.TempDir(), "prev.json")}

	state, err := loadReviewResumeState(repoDir, opts)
	if err != nil {
		t.Fatalf("loadReviewResumeState: %v (a reuse-only run must not enter the resume path)", err)
	}
	if state != nil {
		t.Fatal("a reuse-only run must not load a resume state")
	}

	cc := resumeTestContext(repoDir)
	rt := &llmRuntime{Provider: "anthropic", Model: "claude"}
	sealed, err := validateResumeIdentity(context.Background(), cc, opts, rt, state)
	if err != nil {
		t.Fatalf("validateResumeIdentity: %v (a nil resume state must short-circuit)", err)
	}
	if sealed != nil {
		t.Error("a reuse-only run must not seal its input to a resume admission")
	}
}

// TestLoadReuseStateCommentsReachAgentOutput drives the loader's state through
// the agent's reuse engine end to end at the unit level: the comments the join
// grouped must be exactly the comments applyResume carries into the collector.
func TestLoadReuseStateCommentsReachAgentOutput(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	completed := []session.CoverageItem{{ItemID: "id-a", Path: "a.go", Fingerprint: "fp-agent"}}
	comments := []model.LlmComment{{Path: "a.go", Content: "carried", StartLine: 3, EndLine: 3}}
	path := writeReuseSource(t, "ocr-result.json", validReuseOutput("", completed, nil, comments))

	state, warnings := loadReuseState(context.Background(), repoDir, path)
	if len(warnings) != 0 || state == nil {
		t.Fatalf("state = %v warnings = %v, want a loaded state", state, warnings)
	}
	if _, ok := state.Items["fp-agent"]; !ok {
		t.Fatal("fp-agent missing from items")
	}
	// The collector carry itself is covered by the agent-side reuse tests; here
	// the contract is that ReusableItem (what applyResume calls) returns the
	// joined comments unchanged.
	item, ok := state.ReusableItem("fp-agent")
	if !ok || len(item.Comments) != 1 || item.Comments[0].Content != "carried" {
		t.Fatalf("ReusableItem = %+v ok=%v, want the joined comment", item, ok)
	}
}

// TestReuseFromWiringEndToEnd is the only test that traverses
// executeReviewContext's --reuse-from wiring: the flag must reach
// loadReuseState, its warnings must reach stderr, and the synthesized state
// must reach agent.Args.Reuse. Every other test in this change exercises the
// loader or the agent directly, so deleting the wiring block in review_cmd.go
// would leave the flag parsing into a silent no-op with the whole suite green.
//
// A first run publishes a real result file over the fake Anthropic server; a
// second run over the same range with --reuse-from must reuse every item —
// the parent manifest's fingerprints match what the child recomputes — without
// a single new LLM request (not even the grouping call: dispatch never starts).
func TestReuseFromWiringEndToEnd(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)

	run := func(extra ...string) (string, string, error) {
		t.Helper()
		args := append([]string{"--repo", repoDir, "--from", "HEAD~1", "--to", "HEAD", "--format", "json"}, extra...)
		var out string
		var err error
		errOut := captureStderr(t, func() {
			out = captureStdout(t, func() {
				err = runReview(args)
			})
		})
		return out, errOut, err
	}

	// Run 1: a real completed range review; its published JSON is the artifact
	// a gate loop feeds back through --reuse-from.
	first, _, err := run()
	if err != nil {
		t.Fatalf("first review: %v", err)
	}
	prevPath := filepath.Join(t.TempDir(), "ocr-result.json")
	if err := os.WriteFile(prevPath, []byte(first), 0o644); err != nil {
		t.Fatalf("write reuse source: %v", err)
	}
	attemptsAfterFirst := srv.attemptCounts()
	srv.mu.Lock()
	groupingAfterFirst := srv.groupingCalls
	srv.mu.Unlock()

	// Run 2: same range, everything reusable. In json mode progress lines are
	// redirected to stderr, so stdout stays one parseable document.
	second, secondErrOut, err := run("--reuse-from", prevPath)
	if err != nil {
		t.Fatalf("reuse review: %v\nstderr: %s", err, secondErrOut)
	}
	var reused jsonOutput
	if err := json.Unmarshal([]byte(second), &reused); err != nil {
		t.Fatalf("unmarshal reuse run output: %v\n%s", err, second)
	}
	if reused.Manifest == nil || reused.Resume == nil {
		t.Fatalf("output must carry manifest and resume: %s", second)
	}
	if len(reused.Manifest.Coverage.Reused) != len(markers) || len(reused.Manifest.Coverage.Completed) != 0 {
		t.Fatalf("coverage reused=%d completed=%d, want %d reused and 0 completed",
			len(reused.Manifest.Coverage.Reused), len(reused.Manifest.Coverage.Completed), len(markers))
	}
	if !strings.HasPrefix(reused.Resume.ResumedFrom, "reuse:") ||
		reused.Resume.ReusedFiles != int64(len(markers)) || reused.Resume.RerunFiles != 0 {
		t.Fatalf("resume info = %+v, want a reuse: source with everything reused", reused.Resume)
	}
	if want := fmt.Sprintf("reusing %d file(s), reviewing 0 file(s)", len(markers)); !strings.Contains(secondErrOut, want) {
		t.Errorf("stderr must report the reuse line %q:\n%s", want, secondErrOut)
	}
	if !strings.Contains(secondErrOut, "[ocr] Reuse reuse:") {
		t.Errorf("stderr must label the source as Reuse, not Resume:\n%s", secondErrOut)
	}
	// A fully reused run must not touch the model at all.
	if got := srv.attemptCounts(); !reflect.DeepEqual(got, attemptsAfterFirst) {
		t.Errorf("LLM attempts changed on an all-reused run: got %v, want %v", got, attemptsAfterFirst)
	}
	srv.mu.Lock()
	groupingCalls := srv.groupingCalls
	srv.mu.Unlock()
	if groupingCalls != groupingAfterFirst {
		t.Errorf("grouping calls = %d after the reuse run, want unchanged %d", groupingCalls, groupingAfterFirst)
	}

	// Degraded source: the CI artifact vanished between pushes. The wiring must
	// warn on stderr and the run must degrade to today's full review.
	missing := filepath.Join(t.TempDir(), "vanished.json")
	third, thirdErrOut, err := run("--reuse-from", missing)
	if err != nil {
		t.Fatalf("a degraded reuse source must not fail the run: %v\nstderr: %s", err, thirdErrOut)
	}
	if !strings.Contains(thirdErrOut, "[ocr] WARNING [reuse_from]") {
		t.Errorf("stderr must carry the reuse_from warning:\n%s", thirdErrOut)
	}
	var degraded jsonOutput
	if err := json.Unmarshal([]byte(third), &degraded); err != nil {
		t.Fatalf("unmarshal degraded run output: %v\n%s", err, third)
	}
	if degraded.Manifest == nil || len(degraded.Manifest.Coverage.Reused) != 0 || len(degraded.Manifest.Coverage.Completed) != len(markers) {
		t.Fatalf("degraded coverage reused=%d completed=%d, want 0/%d (full fresh review)",
			len(degraded.Manifest.Coverage.Reused), len(degraded.Manifest.Coverage.Completed), len(markers))
	}
	// And "full review" means the model actually saw the files again.
	if got := srv.attemptCounts(); len(got) == 0 || !moreAttempts(got, attemptsAfterFirst) {
		t.Errorf("LLM attempts = %v after the degraded run, want more than %v", got, attemptsAfterFirst)
	}
}

// moreAttempts reports whether every count in got is at least the count in want
// and the total grew.
func moreAttempts(got, want map[string]int) bool {
	total := 0
	for file, n := range got {
		if n < want[file] {
			return false
		}
		total += n - want[file]
	}
	return total > 0
}
