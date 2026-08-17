// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"os"
	"strings"
	"testing"
)

// parentIdentity is the identity every fixture parent below was run with. Each
// test case mutates exactly one field of the child so the assertion pins which
// comparison did the rejecting.
var parentIdentity = RunIdentity{
	Mode:                 InputModeRange,
	SourceArtifactSHA256: "artifact-parent",
	RuleConfigSHA256:     "rules-parent",
	RepositorySHA256:     "repo-parent",
}

// parentState builds a resumable parent: a v1 review manifest whose identity is
// parentIdentity, run on anthropic/claude, with one selected item. mutate may
// adjust the manifest to model a parent that is older, interrupted or empty.
func parentState(mutate func(*RunManifest)) *ResumeState {
	m := &RunManifest{
		SchemaVersion: ManifestSchemaVersion,
		RunID:         "run-parent",
		Operation:     OperationReview,
		Repository:    ManifestRepository{IdentitySHA256: parentIdentity.RepositorySHA256},
		Input: ManifestInput{
			Mode:                 parentIdentity.Mode,
			SourceArtifactSHA256: parentIdentity.SourceArtifactSHA256,
		},
		Execution: ManifestExecution{
			Provider:         "anthropic",
			Model:            "claude",
			RuleConfigSHA256: parentIdentity.RuleConfigSHA256,
		},
		Coverage: Coverage{Selected: []CoverageItem{{ItemID: "item-1"}}},
	}
	if mutate != nil {
		mutate(m)
	}
	return &ResumeState{SessionID: "parent-session", Manifest: m, Closed: true}
}

// request is a resume of parentState with the same provider and model, which is
// the accepted baseline every case below deviates from.
func request(mutate func(*ResumeRequest)) ResumeRequest {
	req := ResumeRequest{
		Identity: parentIdentity,
		Provider: "anthropic",
		Model:    "claude",
	}
	if mutate != nil {
		mutate(&req)
	}
	return req
}

func TestValidateResume(t *testing.T) {
	tests := []struct {
		name string
		// parent mutates the fixture parent manifest; nil leaves it resumable.
		parent func(*RunManifest)
		// req mutates the matching request; nil leaves it accepted.
		req func(*ResumeRequest)
		// state mutates the parent state itself, for the two unverifiable-parent
		// cases that cannot be expressed by mutating a manifest.
		state func(*ResumeState)
		// wantErr is a substring of the required error, or "" to require acceptance.
		wantErr string
	}{
		{
			name: "identical input, provider and model is accepted",
		},
		{
			name: "parent that completed nothing is still resumable",
			// The parent's own coverage is irrelevant here: admission depends on
			// whether the input is verifiable, not on how much work got done. A
			// fully-failed parent is the case resume exists for.
			parent: func(m *RunManifest) {
				m.Coverage.Failed = []CoverageItem{{ItemID: "item-1"}}
			},
		},
		{
			name: "differing ref text with identical resolved input is accepted",
			// Nothing in the request carries ref text any more; this pins that a
			// parent recording one spelling accepts a child that resolved the same
			// input from another.
			parent: func(m *RunManifest) {
				m.Input.RequestedFrom = "main"
				m.Input.RequestedHead = "abc1234"
			},
		},
		{
			name:    "interrupted parent with no manifest is rejected as unverifiable",
			state:   func(s *ResumeState) { s.Manifest = nil; s.Closed = false },
			wantErr: "was interrupted before it closed",
		},
		{
			// Same rejection, different cause: this one did close, so blaming an
			// interruption would send the user hunting a crash that never happened.
			name:    "parent that closed without a manifest is rejected as unverifiable",
			state:   func(s *ResumeState) { s.Manifest = nil },
			wantErr: "closed without a run manifest",
		},
		{
			name:    "unknown manifest schema is rejected",
			parent:  func(m *RunManifest) { m.SchemaVersion = "ocr.run-manifest/v99" },
			wantErr: "manifest schema",
		},
		{
			name:    "non-review parent is rejected",
			parent:  func(m *RunManifest) { m.Operation = "scan" },
			wantErr: "operation",
		},
		{
			name:    "parent that selected nothing is rejected",
			parent:  func(m *RunManifest) { m.Coverage.Selected = nil },
			wantErr: "selected no input",
		},
		{
			name:    "changed input mode is rejected",
			req:     func(r *ResumeRequest) { r.Identity.Mode = InputModeCommit },
			wantErr: "input mode changed",
		},
		{
			name:    "changed repository identity is rejected",
			req:     func(r *ResumeRequest) { r.Identity.RepositorySHA256 = "repo-other" },
			wantErr: "repository identity changed",
		},
		{
			name: "repository with no remote on both sides is unchanged",
			parent: func(m *RunManifest) {
				m.Repository.IdentitySHA256 = ""
			},
			req: func(r *ResumeRequest) { r.Identity.RepositorySHA256 = "" },
		},
		{
			name:    "changed source artifact is rejected",
			req:     func(r *ResumeRequest) { r.Identity.SourceArtifactSHA256 = "artifact-moved" },
			wantErr: "reviewed input changed",
		},
		{
			name:    "parent without rule identity is rejected as unverifiable",
			parent:  func(m *RunManifest) { m.Execution.RuleConfigSHA256 = "" },
			wantErr: "no rule identity",
		},
		{
			name:    "changed rule config is rejected",
			req:     func(r *ResumeRequest) { r.Identity.RuleConfigSHA256 = "rules-other" },
			wantErr: "rule identity changed",
		},
		{
			// A filter change moves both digests, and source artifact is compared
			// first so the user is told what actually changed about the input
			// rather than being pointed at an unattributable rule digest.
			name: "filter change is reported as an input change, not a rule change",
			req: func(r *ResumeRequest) {
				r.Identity.SourceArtifactSHA256 = "artifact-fewer-files"
				r.Identity.RuleConfigSHA256 = "rules-with-exclude"
			},
			wantErr: "reviewed input changed",
		},
		{
			name:    "implicit provider change is rejected",
			req:     func(r *ResumeRequest) { r.Provider = "openai" },
			wantErr: "provider changed",
		},
		{
			name: "explicit provider change is accepted",
			req: func(r *ResumeRequest) {
				r.Provider = "openai"
				r.Model = "gpt-5"
				r.ProviderExplicit = true
			},
		},
		{
			name:    "implicit model change is rejected",
			req:     func(r *ResumeRequest) { r.Model = "claude-next" },
			wantErr: "model changed",
		},
		{
			name: "explicit model change is accepted",
			req: func(r *ResumeRequest) {
				r.Model = "claude-next"
				r.ModelExplicit = true
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := parentState(tc.parent)
			if tc.state != nil {
				tc.state(state)
			}

			err := state.ValidateResume(request(tc.req))

			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("want accepted, got error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("want rejection mentioning %q, got accepted", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// A nil parent is a non-resume run and must not be second-guessed here.
func TestValidateResumeNilStateAccepts(t *testing.T) {
	var s *ResumeState
	if err := s.ValidateResume(request(nil)); err != nil {
		t.Errorf("nil state must accept, got: %v", err)
	}
}

// An endpoint configured straight from environment variables resolves no
// provider name, so the rejection must not tell the user to run `--provider `
// with nothing after it. The same applies to a model that resolved empty.
func TestTransitionRejectionStaysActionableWithNoName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		req     func(*ResumeRequest)
		wantSub string
	}{
		{
			name:    "provider resolved to no name",
			req:     func(r *ResumeRequest) { r.Provider = "" },
			wantSub: "pass --provider <name> explicitly",
		},
		{
			name:    "model resolved to no name",
			req:     func(r *ResumeRequest) { r.Model = "" },
			wantSub: "pass --model <name> explicitly",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := parentState(nil).ValidateResume(request(tc.req))
			if err == nil {
				t.Fatal("an unasked-for transition must still be rejected")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("message must name the flag rather than echo an empty value, got: %v", err)
			}
		})
	}
}

func TestNewResumeLineage(t *testing.T) {
	t.Run("nil parent records nothing", func(t *testing.T) {
		if l := NewResumeLineage(nil, "run-child", "anthropic", "claude"); l != nil {
			t.Errorf("want nil for a non-resume run, got %+v", l)
		}
	})

	t.Run("parent without manifest records nothing", func(t *testing.T) {
		s := parentState(nil)
		s.Manifest = nil
		if l := NewResumeLineage(s, "run-child", "anthropic", "claude"); l != nil {
			t.Errorf("want nil when there is no parent run id, got %+v", l)
		}
	})

	t.Run("transition carries both endpoints", func(t *testing.T) {
		l := NewResumeLineage(parentState(nil), "run-child", "openai", "gpt-5")
		if l == nil {
			t.Fatal("want a lineage")
		}
		want := ResumeLineage{
			Type:           "resume_lineage",
			SchemaVersion:  ResumeLineageSchemaVersion,
			RunID:          "run-child",
			ParentRunID:    "run-parent",
			SourceProvider: "anthropic",
			SourceModel:    "claude",
			TargetProvider: "openai",
			TargetModel:    "gpt-5",
		}
		if *l != want {
			t.Errorf("lineage mismatch:\n got %+v\nwant %+v", *l, want)
		}
		if !l.IsTransition() {
			t.Error("a provider and model change is a transition")
		}
	})

	t.Run("same target is a lineage but not a transition", func(t *testing.T) {
		l := NewResumeLineage(parentState(nil), "run-child", "anthropic", "claude")
		if l == nil {
			t.Fatal("a same-target resume still records its parent")
		}
		if l.IsTransition() {
			t.Error("identical source and target is not a transition")
		}
	})

	t.Run("nil lineage is not a transition", func(t *testing.T) {
		var l *ResumeLineage
		if l.IsTransition() {
			t.Error("nil must not report a transition")
		}
	})
}

// End-to-end for the event itself: a lineage written by a live session must come
// back out of the session file through the same reader `ocr session show` uses.
func TestResumeLineageRoundTripsThroughSessionFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()

	sh := New(repoDir, "feature", "gpt-5", SessionOptions{
		ReviewMode:  ReviewModeRange,
		DiffFrom:    "main",
		DiffTo:      "feature",
		ResumedFrom: "parent-session",
		Operation:   OperationReview,
	})
	want := NewResumeLineage(parentState(nil), sh.SessionID, "openai", "gpt-5")
	sh.RecordResumeLineage(want)
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}

	summary, err := LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadSummary: %v", err)
	}
	got := summary.ResumeLineage
	if got == nil {
		t.Fatal("session file carries no resume_lineage")
	}
	if *got != *want {
		t.Errorf("lineage did not round-trip:\n got %+v\nwant %+v", *got, *want)
	}
}

// The round trip above finalizes the session, which flushes on its way out and
// so cannot tell a buffered record from a persisted one. Lineage exists to
// survive a run that dies before it finalizes, so assert it is on disk while the
// session is still open.
func TestResumeLineageReachesDiskBeforeFinalize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()

	sh := New(repoDir, "feature", "gpt-5", SessionOptions{
		ReviewMode: ReviewModeRange,
		Operation:  OperationReview,
	})
	sh.RecordResumeLineage(NewResumeLineage(parentState(nil), sh.SessionID, "openai", "gpt-5"))

	path, err := SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	if !strings.Contains(string(raw), ResumeLineageSchemaVersion) {
		t.Error("resume_lineage is still buffered; a run that dies here loses its lineage")
	}
}

// An unknown lineage schema must be ignored rather than half-read: its fields may
// not mean what this build assumes.
func TestUnknownLineageSchemaIsIgnored(t *testing.T) {
	var s Summary
	applyRecordToSummary(&s, summaryRecord{
		Type:          "resume_lineage",
		SchemaVersion: "ocr.resume-lineage/v99",
		ParentRunID:   "run-parent",
	})
	if s.ResumeLineage != nil {
		t.Errorf("want an unknown schema ignored, got %+v", s.ResumeLineage)
	}
}
