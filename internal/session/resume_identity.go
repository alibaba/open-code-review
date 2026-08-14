// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import "fmt"

// RunIdentity is the resolved input identity of a candidate run. Every field
// mirrors the manifest field a finished run records, so a parent manifest and a
// child candidate are directly comparable. It is produced by agent.ResolveIdentity
// before any session exists.
type RunIdentity struct {
	Mode                 string // manifest input.mode
	SourceArtifactSHA256 string // manifest input.source_artifact_sha256
	RuleConfigSHA256     string // manifest execution.rule_config_sha256
	RepositorySHA256     string // manifest repository.identity_sha256; empty when the repo has no remote
}

// ResumeRequest is what the resuming command is asking to do: run this input
// identity with this provider and model, reusing a parent's checkpoints.
//
// ProviderExplicit and ModelExplicit report whether the value came from a flag
// on this very command line. A provider or model that changed because a config
// file, environment variable or shell RC changed is an implicit transition, and
// an implicit transition is exactly what this check exists to catch.
type ResumeRequest struct {
	Identity         RunIdentity
	Provider         string
	Model            string
	ProviderExplicit bool
	ModelExplicit    bool
}

const resumeHint = "start a new review instead of resuming"

// ValidateResume reports whether req may reuse this parent's checkpoints.
//
// A mismatch on any input field rejects the whole resume rather than degrading to
// partial reuse: mixing results computed from one input with results computed
// from another produces a report in which no field distinguishes the two. The
// caller must run this before session.New — session.New writes session_start the
// moment it is called, so a later check would leave an orphan session behind
// every rejection.
//
// Checks run top to bottom and the first mismatch decides. Order matters at
// source_artifact vs rule_config: rule_config_sha256 is a single aggregate over
// the rule-text layers and the file filter and cannot be decomposed, so a filter
// change is reported as the input mismatch it actually caused rather than as an
// unattributable rule change.
func (s *ResumeState) ValidateResume(req ResumeRequest) error {
	if s == nil {
		return nil
	}
	if err := s.validateInputIdentity(req.Identity); err != nil {
		return err
	}

	m := s.Manifest
	providerChanged := m.Execution.Provider != req.Provider
	if providerChanged && !req.ProviderExplicit {
		return fmt.Errorf("resume rejected: provider changed from %q to %q without being asked for; %s to resume across providers on purpose", m.Execution.Provider, req.Provider, explicitFlagHint("--provider", req.Provider))
	}
	// Model is only compared within the same provider: switching provider on
	// purpose necessarily brings that provider's own model with it.
	if !providerChanged && m.Execution.Model != req.Model && !req.ModelExplicit {
		return fmt.Errorf("resume rejected: model changed from %q to %q without being asked for; %s to resume across models on purpose", m.Execution.Model, req.Model, explicitFlagHint("--model", req.Model))
	}
	return nil
}

// validateInputIdentity compares only the input half of the resume contract: the
// parent manifest must be verifiable, and every input field must match. Provider
// and model are deliberately left out: those are command-line intent, not
// something derived from the input.
//
// This runs once, at admission, and is never repeated during the run: the caller
// pins the run to the commit endpoints this comparison was made against (see
// agent.SealedInput), so a second comparison could only ever confirm the first.
func (s *ResumeState) validateInputIdentity(id RunIdentity) error {
	if s == nil {
		return nil
	}

	m := s.Manifest
	switch {
	case m == nil && !s.Closed:
		// Distinguishing all of these from "manifest present, zero completed
		// items" is the whole point: that one is resumable, none of these are.
		return fmt.Errorf("resume session %q was interrupted before it closed, so it never recorded a run manifest and its input identity cannot be verified; %s", s.SessionID, resumeHint)
	case m == nil:
		// It closed cleanly, so blaming an interruption would send the user
		// looking for a crash that never happened. A session_end with no manifest
		// is a session older than run manifests, or a run that failed before
		// freezing one.
		return fmt.Errorf("resume session %q closed without a run manifest, so its input identity cannot be verified — it either predates run manifests or failed before recording one; %s", s.SessionID, resumeHint)
	case m.SchemaVersion != ManifestSchemaVersion:
		return fmt.Errorf("resume session %q carries manifest schema %q, but this build can only verify %q; %s", s.SessionID, m.SchemaVersion, ManifestSchemaVersion, resumeHint)
	case m.Operation != OperationReview:
		return fmt.Errorf("resume session %q recorded operation %q, not %q; %s", s.SessionID, m.Operation, OperationReview, resumeHint)
	case len(m.Coverage.Selected) == 0:
		// Without this, an empty parent and an empty child would both hash to the
		// canonical empty digest, pass every comparison, and produce a run that
		// reuses nothing and dispatches nothing.
		return fmt.Errorf("resume session %q selected no input, so it has nothing to resume; %s", s.SessionID, resumeHint)
	}

	if m.Input.Mode != id.Mode {
		// Mode feeds item_id derivation, so parent and child items cannot even be
		// put side by side.
		return fmt.Errorf("resume rejected: input mode changed from %q to %q; %s", m.Input.Mode, id.Mode, resumeHint)
	}
	// Both sides empty means a repository with no remote, which is unchanged.
	if m.Repository.IdentitySHA256 != id.RepositorySHA256 {
		return fmt.Errorf("resume rejected: repository identity changed, so this is not the repository the parent run reviewed; %s", resumeHint)
	}
	if m.Input.SourceArtifactSHA256 != id.SourceArtifactSHA256 {
		return fmt.Errorf("resume rejected: the reviewed input changed since session %q — a ref may now point at a different commit, or the selected file set changed; %s", s.SessionID, resumeHint)
	}
	if m.Execution.RuleConfigSHA256 == "" {
		return fmt.Errorf("resume session %q recorded no rule identity, so it cannot be verified against the current rules; %s", s.SessionID, resumeHint)
	}
	if m.Execution.RuleConfigSHA256 != id.RuleConfigSHA256 {
		// The digest is one aggregate, so it can only be attributed to a layer,
		// never to a specific rule or pattern.
		return fmt.Errorf("resume rejected: review rule identity changed — either a rule text layer (custom, project, global or system) or the include/exclude file filter differs from session %q; %s", s.SessionID, resumeHint)
	}
	return nil
}

// explicitFlagHint renders the actionable half of a transition rejection. value
// is empty whenever the endpoint has no provider name — one configured straight
// from environment variables has none — and `pass --provider ` is not a command
// anyone can run, so name the flag rather than echoing the empty value.
func explicitFlagHint(flag, value string) string {
	if value == "" {
		return "pass " + flag + " <name> explicitly"
	}
	return "pass " + flag + " " + value
}

// ResumeLineageSchemaVersion versions the resume_lineage event independently of
// the run manifest: lineage records a transition between runs, not a run's
// coverage, so the two evolve separately.
const ResumeLineageSchemaVersion = "ocr.resume-lineage/v1"

// ResumeLineage records which run this one continued and which provider and
// model it moved between. Source and target being equal means a same-target
// resume; there is no separate transition-kind field because the kind is exactly
// what comparing them tells you.
//
// Every field is a non-secret label. No API key, authorization header, endpoint,
// absolute path, diff, prompt or model response ever belongs here.
type ResumeLineage struct {
	Type           string `json:"type"`
	SchemaVersion  string `json:"schema_version"`
	RunID          string `json:"run_id"`
	ParentRunID    string `json:"parent_run_id"`
	SourceProvider string `json:"source_provider"`
	SourceModel    string `json:"source_model"`
	TargetProvider string `json:"target_provider"`
	TargetModel    string `json:"target_model"`
}

// NewResumeLineage builds the lineage for an accepted resume of parent. It
// returns nil when there is nothing to record — no parent, or a parent with no
// verifiable manifest — so callers can hand the result straight to
// RecordResumeLineage.
func NewResumeLineage(parent *ResumeState, runID, targetProvider, targetModel string) *ResumeLineage {
	if parent == nil || parent.Manifest == nil {
		return nil
	}
	return &ResumeLineage{
		Type:           "resume_lineage",
		SchemaVersion:  ResumeLineageSchemaVersion,
		RunID:          runID,
		ParentRunID:    parent.Manifest.RunID,
		SourceProvider: parent.Manifest.Execution.Provider,
		SourceModel:    parent.Manifest.Execution.Model,
		TargetProvider: targetProvider,
		TargetModel:    targetModel,
	}
}

// IsTransition reports whether the resume changed provider or model, which is
// the condition that required an explicit flag to be accepted.
func (l *ResumeLineage) IsTransition() bool {
	if l == nil {
		return false
	}
	return l.SourceProvider != l.TargetProvider || l.SourceModel != l.TargetModel
}
