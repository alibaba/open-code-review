// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func sel(id string) CoverageItem {
	return CoverageItem{ItemID: id, Path: id + ".go", Fingerprint: "fp-" + id}
}

// newBuilderWith registers a set of selected items on a fresh builder and sets a
// valid input mode so Finalize passes the mandatory-mode check.
func newBuilderWith(ids ...string) *ManifestBuilder {
	b := NewManifestBuilder("run-1", "review")
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	for _, id := range ids {
		if err := b.RegisterSelected(sel(id)); err != nil {
			panic(err)
		}
	}
	return b
}

// mustFinalize finalizes and fails the test on an unexpected validation error.
func mustFinalize(t *testing.T, b *ManifestBuilder) RunManifest {
	t.Helper()
	m, err := b.Finalize(0)
	if err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	return m
}

func TestTerminalComplete(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkCompleted("a")
	b.MarkCompleted("b")
	m := mustFinalize(t, b)
	if m.TerminalState != StateComplete {
		t.Fatalf("terminal = %q, want complete", m.TerminalState)
	}
	if len(m.Coverage.Completed) != 2 || len(m.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v", m.Coverage)
	}
	if len(m.Coverage.Selected) != 2 {
		t.Fatalf("selected = %d, want 2", len(m.Coverage.Selected))
	}
}

// Zero findings is a coverage concept: all selected complete regardless of
// comments, so the terminal state is still complete.
func TestTerminalCompleteZeroFindings(t *testing.T) {
	b := newBuilderWith("a")
	b.MarkCompleted("a")
	if got := mustFinalize(t, b).TerminalState; got != StateComplete {
		t.Fatalf("terminal = %q, want complete", got)
	}
}

func TestTerminalPartial(t *testing.T) {
	b := newBuilderWith("a", "b", "c")
	b.MarkCompleted("a")
	b.MarkReused("b")
	b.MarkFailed("c", FailureProvider, "provider concurrency")
	m := mustFinalize(t, b)
	if m.TerminalState != StatePartial {
		t.Fatalf("terminal = %q, want partial", m.TerminalState)
	}
	if len(m.Coverage.Failed) != 1 || m.Coverage.Failed[0].Classification != FailureProvider {
		t.Fatalf("failed coverage = %+v", m.Coverage.Failed)
	}
}

func TestTerminalFailedAll(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkFailed("a", FailureProvider, "x")
	b.MarkFailed("b", FailureTimeout, "y")
	if got := mustFinalize(t, b).TerminalState; got != StateFailed {
		t.Fatalf("terminal = %q, want failed", got)
	}
}

func TestTerminalSkipped(t *testing.T) {
	b := NewManifestBuilder("run-1", "review")
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	if got := mustFinalize(t, b).TerminalState; got != StateSkipped {
		t.Fatalf("terminal = %q, want skipped", got)
	}
}

// A run-level failure forces the terminal state to failed regardless of
// coverage, while any recorded per-item outcomes are preserved.
func TestRunFailureForcesFailed(t *testing.T) {
	// No selected items, but a run-level error occurred -> failed, not skipped.
	b := NewManifestBuilder("run-1", "review")
	b.SetInput(ManifestInput{Mode: InputModeRange})
	if err := b.SetRunFailure(RunFailureInput, "unable to resolve range"); err != nil {
		t.Fatalf("SetRunFailure: %v", err)
	}
	m := mustFinalize(t, b)
	if m.TerminalState != StateFailed {
		t.Fatalf("terminal = %q, want failed", m.TerminalState)
	}
	if m.RunFailure == nil || m.RunFailure.Classification != RunFailureInput {
		t.Fatalf("run_failure = %+v, want input", m.RunFailure)
	}

	// A fully-completed set is still failed once a run-level error is flagged,
	// and the completed outcome is retained.
	b2 := newBuilderWith("a")
	b2.MarkCompleted("a")
	if err := b2.SetRunFailure(RunFailureInternal, "scheduler invariant violated"); err != nil {
		t.Fatalf("SetRunFailure: %v", err)
	}
	m2 := mustFinalize(t, b2)
	if m2.TerminalState != StateFailed {
		t.Fatalf("terminal = %q, want failed", m2.TerminalState)
	}
	if len(m2.Coverage.Completed) != 1 {
		t.Fatalf("completed outcome dropped on run failure: %+v", m2.Coverage)
	}
}

// A run_failure sweeps still-selected items into failed with the item class that
// matches the run stop cause; the terminal state is failed and prior outcomes
// are retained.
func TestRunFailureSweepsPendingToMatchingClass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		runClass RunFailureClass
		wantItem FailureClass
	}{
		{"cancelled", RunFailureCancelled, FailureCancelled},
		{"budget", RunFailureBudget, FailureBudget},
		{"timeout", RunFailureTimeout, FailureTimeout},
		{"configuration", RunFailureConfiguration, FailureConfiguration},
		{"internal", RunFailureInternal, FailureUnknown}, // no item-level internal
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBuilderWith("a", "b")
			b.MarkCompleted("a")
			if err := b.SetRunFailure(tc.runClass, "stopped"); err != nil {
				t.Fatalf("SetRunFailure: %v", err)
			}
			// "b" never marked -> swept with the matching item class.
			m := mustFinalize(t, b)
			if m.TerminalState != StateFailed {
				t.Fatalf("terminal = %q, want failed", m.TerminalState)
			}
			if len(m.Coverage.Completed) != 1 {
				t.Fatalf("completed dropped: %+v", m.Coverage)
			}
			if len(m.Coverage.Failed) != 1 || m.Coverage.Failed[0].Classification != tc.wantItem {
				t.Fatalf("swept class = %+v, want %s", m.Coverage.Failed, tc.wantItem)
			}
		})
	}
}

// A pending failure cause attributes the undecided items without escalating the
// run: it is a controlled coverage truncation, so anything already covered keeps
// the terminal state at partial rather than forcing failed the way run_failure
// would. This is what makes `--max-tokens-budget` exit 0 while still reporting
// exactly which files went unreviewed and why.
func TestPendingFailureCauseSweepsWithoutForcingFailed(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkCompleted("a")
	if err := b.SetPendingFailureCause(FailureBudget, "aggregate token budget reached"); err != nil {
		t.Fatalf("SetPendingFailureCause: %v", err)
	}
	m := mustFinalize(t, b)
	if m.TerminalState != StatePartial {
		t.Fatalf("terminal = %q, want partial", m.TerminalState)
	}
	if m.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil", m.RunFailure)
	}
	if len(m.Coverage.Completed) != 1 {
		t.Fatalf("completed dropped: %+v", m.Coverage)
	}
	if len(m.Coverage.Failed) != 1 {
		t.Fatalf("failed = %d, want 1", len(m.Coverage.Failed))
	}
	if got := m.Coverage.Failed[0]; got.Classification != FailureBudget || got.Reason == "" {
		t.Fatalf("swept item = %+v, want budget with the truncation reason", got)
	}
}

// When the truncation leaves nothing covered, the same coverage rule that
// produced partial above produces failed here — no run_failure needed. This is
// the exit-code boundary: one covered item flips the process exit status.
func TestPendingFailureCauseWithNoCoverageIsFailed(t *testing.T) {
	b := newBuilderWith("a", "b")
	if err := b.SetPendingFailureCause(FailureBudget, "aggregate token budget reached"); err != nil {
		t.Fatalf("SetPendingFailureCause: %v", err)
	}
	m := mustFinalize(t, b)
	if m.TerminalState != StateFailed {
		t.Fatalf("terminal = %q, want failed", m.TerminalState)
	}
	if m.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil", m.RunFailure)
	}
	if len(m.Coverage.Failed) != 2 {
		t.Fatalf("failed = %d, want 2", len(m.Coverage.Failed))
	}
	for _, it := range m.Coverage.Failed {
		if it.Classification != FailureBudget {
			t.Fatalf("item %s class = %q, want budget", it.ItemID, it.Classification)
		}
	}
}

// A real run-level failure outranks a pending cause in the sweep: the run did
// fail, so its class colors the undecided items and forces terminal failed even
// though a truncation cause was also recorded.
func TestRunFailureOutranksPendingFailureCause(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkCompleted("a")
	if err := b.SetPendingFailureCause(FailureBudget, "aggregate token budget reached"); err != nil {
		t.Fatalf("SetPendingFailureCause: %v", err)
	}
	if err := b.SetRunFailure(RunFailureInternal, "scheduler invariant violated"); err != nil {
		t.Fatalf("SetRunFailure: %v", err)
	}
	m := mustFinalize(t, b)
	if m.TerminalState != StateFailed {
		t.Fatalf("terminal = %q, want failed", m.TerminalState)
	}
	// internal has no item-level equivalent, so the swept item degrades to unknown
	// rather than silently reporting the (now irrelevant) budget cause.
	if len(m.Coverage.Failed) != 1 || m.Coverage.Failed[0].Classification != FailureUnknown {
		t.Fatalf("swept item = %+v, want unknown from the run_failure mapping", m.Coverage.Failed)
	}
}

func TestSetPendingFailureCauseValidation(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.SetPendingFailureCause("not-a-class", "x"); err == nil {
		t.Fatal("expected an invalid pending failure class to be rejected")
	}
	if err := b.SetPendingFailureCause(FailureBudget, "first"); err != nil {
		t.Fatalf("first set: %v", err)
	}
	// Re-recording the same class is idempotent; changing it is an error, so a
	// second, conflicting producer surfaces instead of silently winning or losing.
	if err := b.SetPendingFailureCause(FailureBudget, "again"); err != nil {
		t.Fatalf("same class must be idempotent: %v", err)
	}
	if err := b.SetPendingFailureCause(FailureTimeout, "different"); err == nil {
		t.Fatal("expected a class change to be rejected")
	}
	mustFinalize(t, b)
	if err := b.SetPendingFailureCause(FailureBudget, "after freeze"); err == nil {
		t.Fatal("expected a post-freeze set to be rejected")
	}
}

// Waived items count as resolved (non-failed): a run with completed + waived
// and no failed is complete.
func TestWaivedResolvesToComplete(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkReused("a")
	b.MarkWaived("b", "user waived on resume")
	m := mustFinalize(t, b)
	if m.TerminalState != StateComplete {
		t.Fatalf("terminal = %q, want complete", m.TerminalState)
	}
	if len(m.Coverage.Waived) != 1 || m.Coverage.Waived[0].Reason == "" {
		t.Fatalf("waived coverage = %+v", m.Coverage.Waived)
	}
}

// A failed + waived + completed mix still has an unresolved failure -> partial.
func TestFailedPlusWaivedIsPartial(t *testing.T) {
	b := newBuilderWith("a", "b", "c")
	b.MarkCompleted("a")
	b.MarkWaived("b", "waived")
	b.MarkFailed("c", FailureProvider, "boom")
	if got := mustFinalize(t, b).TerminalState; got != StatePartial {
		t.Fatalf("terminal = %q, want partial", got)
	}
}

// Finalize must sweep any item left without a terminal outcome into
// failed/unknown, so no selected item is ever silently dropped.
func TestFinalizeSweepsUndecidedToUnknown(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkCompleted("a")
	// "b" never marked — simulates a goroutine that exited early.
	m := mustFinalize(t, b)
	if m.TerminalState != StatePartial {
		t.Fatalf("terminal = %q, want partial", m.TerminalState)
	}
	if len(m.Coverage.Failed) != 1 {
		t.Fatalf("failed = %d, want 1", len(m.Coverage.Failed))
	}
	if got := m.Coverage.Failed[0]; got.Classification != FailureUnknown || got.Reason == "" {
		t.Fatalf("swept item = %+v, want unknown with reason", got)
	}
}

// The first terminal state wins and a conflicting later transition returns an
// error: a late error must not demote a completed item.
func TestConflictingTransitionErrorsAndKeepsFirst(t *testing.T) {
	b := newBuilderWith("a")
	b.MarkCompleted("a")
	if err := b.MarkFailed("a", FailureProvider, "late error"); err == nil {
		t.Fatal("expected error demoting completed item to failed")
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Completed) != 1 || len(m.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v, want a still completed", m.Coverage)
	}
}

// Re-applying the same terminal state is idempotent (no error).
func TestIdempotentSameOutcome(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.MarkCompleted("a"); err != nil {
		t.Fatalf("first MarkCompleted: %v", err)
	}
	if err := b.MarkCompleted("a"); err != nil {
		t.Fatalf("idempotent MarkCompleted should not error: %v", err)
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Completed) != 1 {
		t.Fatalf("completed = %d, want 1", len(m.Coverage.Completed))
	}
}

// Re-marking a failed item with the same classification is idempotent; reason is
// free text and does not participate, so the first reason is kept.
func TestFailedSameClassIsIdempotent(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.MarkFailed("a", FailureProvider, "first reason"); err != nil {
		t.Fatalf("first MarkFailed: %v", err)
	}
	if err := b.MarkFailed("a", FailureProvider, "second reason"); err != nil {
		t.Fatalf("idempotent MarkFailed with same class should not error: %v", err)
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Failed) != 1 {
		t.Fatalf("failed = %d, want 1", len(m.Coverage.Failed))
	}
	if got := m.Coverage.Failed[0].Classification; got != FailureProvider {
		t.Fatalf("classification = %s, want provider", got)
	}
	if got := m.Coverage.Failed[0].Reason; got != "first reason" {
		t.Fatalf("reason = %q, want first reason kept", got)
	}
}

// Re-marking a failed item with a different classification is a conflicting
// transition: it errors and the first classification is kept (never silently
// overwritten or dropped).
func TestFailedDifferentClassErrorsAndKeepsFirst(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.MarkFailed("a", FailureTimeout, "timed out"); err != nil {
		t.Fatalf("first MarkFailed: %v", err)
	}
	if err := b.MarkFailed("a", FailureProvider, "provider error"); err == nil {
		t.Fatal("expected error re-marking failed item with a different class")
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Failed) != 1 {
		t.Fatalf("failed = %d, want 1", len(m.Coverage.Failed))
	}
	if got := m.Coverage.Failed[0].Classification; got != FailureTimeout {
		t.Fatalf("classification = %s, want timeout kept", got)
	}
}

// An invalid/empty failure class is rejected (never downgraded to unknown); the
// item stays selected and is swept at Finalize.
func TestInvalidFailureClassRejected(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.MarkFailed("a", FailureClass("bogus"), "r"); err == nil {
		t.Fatal("expected error for invalid failure class")
	}
	if err := b.MarkFailed("a", "", "r"); err == nil {
		t.Fatal("expected error for empty failure class")
	}
}

// Marking an item that was never selected returns an error (not a silent no-op).
func TestMarkUnknownItemErrors(t *testing.T) {
	b := newBuilderWith("a")
	b.MarkCompleted("a")
	if err := b.MarkCompleted("ghost"); err == nil {
		t.Fatal("expected error marking unknown item")
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Selected) != 1 {
		t.Fatalf("selected = %d, want 1", len(m.Coverage.Selected))
	}
}

// A waived item requires a non-empty reason.
func TestWaiveEmptyReasonRejected(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.MarkWaived("a", ""); err == nil {
		t.Fatal("expected error waiving without a reason")
	}
	if err := b.MarkWaived("a", "   \n  "); err == nil {
		t.Fatal("expected error waiving with a blank reason")
	}
}

// Duplicate registration keeps the first entry and is not an error.
func TestDuplicateRegistrationIgnored(t *testing.T) {
	b := NewManifestBuilder("run-1", "review")
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	if err := b.RegisterSelected(CoverageItem{ItemID: "a", Path: "first.go"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := b.RegisterSelected(CoverageItem{ItemID: "a", Path: "second.go"}); err != nil {
		t.Fatalf("duplicate register should be idempotent, got: %v", err)
	}
	b.MarkCompleted("a")
	m := mustFinalize(t, b)
	if len(m.Coverage.Selected) != 1 || m.Coverage.Selected[0].Path != "first.go" {
		t.Fatalf("selected = %+v, want single first.go", m.Coverage.Selected)
	}
}

// SealSelected closes the denominator: RegisterSelected fails afterwards, while
// Mark* against the sealed set still works. sealed is distinct from frozen.
func TestSealSelectedClosesDenominator(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.SealSelected(); err != nil {
		t.Fatalf("SealSelected: %v", err)
	}
	if !b.Sealed() {
		t.Fatal("builder should report sealed")
	}
	if b.Frozen() {
		t.Fatal("sealed must not imply frozen")
	}
	if err := b.RegisterSelected(sel("b")); err == nil {
		t.Fatal("RegisterSelected after seal should error")
	}
	// Mark* on the sealed set still works.
	if err := b.MarkCompleted("a"); err != nil {
		t.Fatalf("MarkCompleted after seal: %v", err)
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Selected) != 1 || len(m.Coverage.Completed) != 1 {
		t.Fatalf("coverage = %+v, want single completed", m.Coverage)
	}
}

// After Finalize the builder is frozen: further mutation returns an error and
// Finalize is idempotent.
func TestFrozenAfterFinalize(t *testing.T) {
	b := newBuilderWith("a", "b")
	b.MarkCompleted("a")
	b.MarkCompleted("b")
	first, err := b.Finalize(5 * time.Second)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !b.Frozen() {
		t.Fatal("builder should be frozen")
	}
	if err := b.RegisterSelected(sel("c")); err == nil {
		t.Fatal("RegisterSelected after freeze should error")
	}
	if err := b.MarkFailed("a", FailureProvider, "x"); err == nil {
		t.Fatal("MarkFailed after freeze should error")
	}
	second, err := b.Finalize(99 * time.Second)
	if err != nil {
		t.Fatalf("re-Finalize: %v", err)
	}
	if first.TerminalState != second.TerminalState || len(second.Coverage.Selected) != 2 {
		t.Fatalf("frozen manifest changed: %+v vs %+v", first, second)
	}
	if first.ElapsedMS != second.ElapsedMS {
		t.Fatalf("elapsed changed on re-finalize: %d vs %d", first.ElapsedMS, second.ElapsedMS)
	}
}

// Identity/execution setters populate the manifest and stop mattering after freeze.
func TestIdentityAndExecutionFields(t *testing.T) {
	b := newBuilderWith("a")
	b.SetParentRunID("run-parent")
	b.SetRepository(ManifestRepository{IdentitySHA256: "sha256:repo"})
	b.SetInput(ManifestInput{Mode: InputModeRange, ResolvedBase: "8f6c", ResolvedHead: "c2d1", ExactRange: "8f6c..c2d1"})
	b.SetExecution(ManifestExecution{Provider: "anthropic", Model: "claude", ConfiguredConcurrency: 16})
	b.MarkCompleted("a")
	m := mustFinalize(t, b)
	if m.ParentRunID != "run-parent" || m.Repository.IdentitySHA256 != "sha256:repo" {
		t.Fatalf("identity not set: %+v", m)
	}
	if m.Input.Mode != InputModeRange || m.Input.ExactRange != "8f6c..c2d1" || m.Execution.ConfiguredConcurrency != 16 {
		t.Fatalf("input/execution not set: %+v", m)
	}
	if m.SchemaVersion != ManifestSchemaVersion || m.RunID != "run-1" || m.Operation != "review" {
		t.Fatalf("header wrong: %+v", m)
	}
}

// Coverage arrays are sorted by item_id and marshal as [] (never null).
func TestCoverageSortedAndNonNilJSON(t *testing.T) {
	b := newBuilderWith("c", "a", "b")
	b.MarkCompleted("c")
	b.MarkCompleted("a")
	b.MarkCompleted("b")
	m := mustFinalize(t, b)
	ids := []string{m.Coverage.Completed[0].ItemID, m.Coverage.Completed[1].ItemID, m.Coverage.Completed[2].ItemID}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Fatalf("not sorted: %v", ids)
	}
	data, err := json.Marshal(m.Coverage)
	if err != nil {
		t.Fatal(err)
	}
	// reused/failed/waived are empty and must render as [].
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"selected", "completed", "reused", "failed", "waived"} {
		if string(raw[k]) == "null" || raw[k] == nil {
			t.Fatalf("%s serialized as null, want []", k)
		}
	}
}

// Finalize on a nil receiver must not panic and returns a well-formed, empty
// manifest alongside an error (mirrors the other nil guards).
func TestFinalizeNilReceiver(t *testing.T) {
	var b *ManifestBuilder
	m, err := b.Finalize(0)
	if err == nil {
		t.Fatal("nil-receiver Finalize should return an error")
	}
	if m.TerminalState != StateSkipped || m.SchemaVersion != ManifestSchemaVersion {
		t.Fatalf("nil finalize = %+v", m)
	}
	data, err := json.Marshal(m.Coverage)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"selected", "completed", "reused", "failed", "waived"} {
		if string(raw[k]) == "null" || raw[k] == nil {
			t.Fatalf("%s serialized as null on nil finalize", k)
		}
	}
}

// Within a single run, waiving an item that has already failed is a conflicting
// transition: it returns an error and the first terminal state wins.
func TestWaiveAfterFailedErrorsAndKeepsFailed(t *testing.T) {
	b := newBuilderWith("a")
	b.MarkFailed("a", FailureProvider, "boom")
	if err := b.MarkWaived("a", "too late"); err == nil {
		t.Fatal("expected error waiving an already-failed item")
	}
	m := mustFinalize(t, b)
	if len(m.Coverage.Failed) != 1 || len(m.Coverage.Waived) != 0 {
		t.Fatalf("coverage = %+v, want a still failed", m.Coverage)
	}
	if m.TerminalState != StateFailed {
		t.Fatalf("terminal = %q, want failed", m.TerminalState)
	}
}

// Finalize rejects a missing/invalid input mode rather than emitting an invalid
// v1 manifest.
func TestFinalizeRejectsMissingMode(t *testing.T) {
	b := NewManifestBuilder("run-1", "review")
	b.RegisterSelected(sel("a"))
	b.MarkCompleted("a")
	if _, err := b.Finalize(0); err == nil {
		t.Fatal("expected error finalizing without input.mode")
	}
	b2 := NewManifestBuilder("run-1", "review")
	b2.SetInput(ManifestInput{Mode: "bogus"})
	b2.RegisterSelected(sel("a"))
	b2.MarkCompleted("a")
	if _, err := b2.Finalize(0); err == nil {
		t.Fatal("expected error finalizing with an invalid input.mode")
	}
}

func TestFinalizeValidationFailureDoesNotMutateSelectedItems(t *testing.T) {
	b := NewManifestBuilder("run-1", "review")
	if err := b.RegisterSelected(sel("a")); err != nil {
		t.Fatalf("register selected: %v", err)
	}
	if err := b.SealSelected(); err != nil {
		t.Fatalf("seal selected: %v", err)
	}

	if _, err := b.Finalize(0); err == nil {
		t.Fatal("expected missing input.mode to fail validation")
	}
	if b.Frozen() {
		t.Fatal("validation failure must not freeze the builder")
	}

	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	if err := b.MarkCompleted("a"); err != nil {
		t.Fatalf("mark completed after failed Finalize: %v", err)
	}
	m, err := b.Finalize(0)
	if err != nil {
		t.Fatalf("retry Finalize: %v", err)
	}
	if m.TerminalState != StateComplete || len(m.Coverage.Completed) != 1 || len(m.Coverage.Failed) != 0 {
		t.Fatalf("retried manifest = %+v, want one completed item", m)
	}
}

// Finalize rejects an empty run_id.
func TestFinalizeRejectsEmptyRunID(t *testing.T) {
	b := NewManifestBuilder("", "review")
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	if _, err := b.Finalize(0); err == nil {
		t.Fatal("expected error finalizing with empty run_id")
	}
}

// SetRunFailure rejects an invalid class and a conflicting re-classification,
// but accepts an idempotent repeat.
func TestSetRunFailureValidation(t *testing.T) {
	b := newBuilderWith("a")
	if err := b.SetRunFailure(RunFailureClass("bogus"), "x"); err == nil {
		t.Fatal("expected error for invalid run_failure class")
	}
	if err := b.SetRunFailure(RunFailureTimeout, "deadline"); err != nil {
		t.Fatalf("first SetRunFailure: %v", err)
	}
	if err := b.SetRunFailure(RunFailureTimeout, "again"); err != nil {
		t.Fatalf("idempotent SetRunFailure should not error: %v", err)
	}
	if err := b.SetRunFailure(RunFailureCancelled, "conflict"); err == nil {
		t.Fatal("expected error re-classifying an already-set run_failure")
	}
}

// sanitizeReason must strip high-confidence secret shapes and never let the
// secret substring survive into the stored reason.
func TestSanitizeReasonStripsSecrets(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{"bearer", "provider error: Authorization: Bearer sk-abc123XYZ rejected", "sk-abc123XYZ"},
		{"basic", "auth failed Basic dXNlcjpwYXNz here", "dXNlcjpwYXNz"},
		{"api_key assignment", "config has api_key=SUPERSECRET123 value", "SUPERSECRET123"},
		{"token assignment", "token: ghp_TOKENVALUE99 expired", "ghp_TOKENVALUE99"},
		{"url userinfo", "clone https://alice:hunter2@github.com/x/y.git failed", "hunter2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeReason(tc.in)
			if strings.Contains(got, tc.secret) {
				t.Fatalf("secret %q leaked in %q", tc.secret, got)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("expected [REDACTED] marker in %q", got)
			}
		})
	}
}

// Reasons are capped to maxReasonLen runes (rune-safe) and collapsed to one line.
func TestSanitizeReasonTruncatesAndSingleLine(t *testing.T) {
	long := strings.Repeat("a", maxReasonLen+200)
	got := sanitizeReason(long)
	if utf8.RuneCountInString(got) > maxReasonLen+1 { // +1 for the ellipsis
		t.Fatalf("not truncated: %d runes", utf8.RuneCountInString(got))
	}
	if strings.ContainsAny(sanitizeReason("line1\nline2\rline3"), "\n\r") {
		t.Fatal("newlines not collapsed")
	}
	// Multibyte input must not be cut mid-rune.
	multibyte := strings.Repeat("世", maxReasonLen+50)
	if !utf8.ValidString(sanitizeReason(multibyte)) {
		t.Fatal("truncation produced invalid UTF-8")
	}
}

// The redaction floor is enforced through the builder, not just the helper:
// a secret passed via MarkFailed must not reach the manifest.
func TestMarkFailedEnforcesRedaction(t *testing.T) {
	b := newBuilderWith("a")
	b.MarkFailed("a", FailureProvider, "died: api_key=LEAKED_TOKEN_42")
	m := mustFinalize(t, b)
	if got := m.Coverage.Failed[0].Reason; strings.Contains(got, "LEAKED_TOKEN_42") {
		t.Fatalf("secret leaked through MarkFailed: %q", got)
	}
}

// ItemID is content-independent: it is derived from operation, mode and the
// normalized old/new path, so a diff-content change does not change it, but a
// path or operation change does.
func TestItemIDDerivation(t *testing.T) {
	id := ItemID("review", InputModeWorkspace, "", "payment.go")
	if len(id) != 64 {
		t.Fatalf("item_id length = %d, want 64 hex chars", len(id))
	}
	// Deterministic for the same identity inputs.
	if id != ItemID("review", InputModeWorkspace, "", "payment.go") {
		t.Fatal("item_id not deterministic")
	}
	// Content-independent: the fingerprint (diff content) is not an input.
	// Different paths, operations or modes produce different ids.
	if id == ItemID("review", InputModeWorkspace, "", "ledger.go") {
		t.Fatal("distinct paths must yield distinct item_ids")
	}
	if id == ItemID("scan", InputModeWorkspace, "", "payment.go") {
		t.Fatal("distinct operations must yield distinct item_ids")
	}
	if id == ItemID("review", InputModeRange, "", "payment.go") {
		t.Fatal("distinct input modes must yield distinct item_ids")
	}
	// Cosmetically different spellings of the same path normalize to one id.
	if ItemID("review", InputModeWorkspace, "", "./a/../payment.go") != id {
		t.Fatal("normalized paths must yield the same item_id")
	}
	// A rename (old_path set) is a distinct identity from a plain add.
	if ItemID("review", InputModeWorkspace, "old.go", "payment.go") == id {
		t.Fatal("rename identity must differ from a plain add")
	}
}

// sanitizeReason strips control chars, ANSI escapes and Unicode line separators,
// and coerces invalid UTF-8 — so nothing survives to inject into a terminal.
func TestSanitizeReasonStripsControlChars(t *testing.T) {
	in := "err\x1b[2J\x1b[H boom\x00\x07\x7f end next\nline"
	got := sanitizeReason(in)
	for _, bad := range []string{"\x1b", "\x00", "\x07", "\x7f", " ", "\n"} {
		if strings.Contains(got, bad) {
			t.Fatalf("control char %q survived in %q", bad, got)
		}
	}
	if !utf8.ValidString(got) {
		t.Fatal("output not valid UTF-8")
	}
	if strings.ContainsAny(got, "\n\r\v\f") {
		t.Fatalf("not single line: %q", got)
	}
	// invalid UTF-8 bytes are replaced, not preserved
	if got2 := sanitizeReason("bad\xff\xfebytes"); strings.ContainsRune(got2, 0xff) {
		t.Fatalf("invalid UTF-8 survived: %q", got2)
	}
}

// A control byte embedded inside a secret must not let the tail of the token
// survive: UTF-8 coercion + control-char stripping run before the redaction
// regexes, so the whole token is seen and redacted rather than split.
func TestSanitizeReasonControlByteInTokenDoesNotLeakTail(t *testing.T) {
	got := sanitizeReason("Authorization: Bearer AAA\x00BBB rejected")
	if strings.Contains(got, "BBB") {
		t.Fatalf("secret tail leaked past control byte: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", got)
	}
}

// A quoted secret value with spaces must be fully redacted (no partial leak).
func TestSanitizeReasonQuotedValue(t *testing.T) {
	got := sanitizeReason(`token="a b c" trailing`)
	if strings.Contains(got, "a b c") {
		t.Fatalf("quoted secret leaked: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", got)
	}
}

// Finalize returns snapshots that own their coverage slices and run_failure:
// mutating one caller's manifest must not affect another's.
func TestFinalizeReturnsOwnedSlices(t *testing.T) {
	b := newBuilderWith("a")
	b.MarkFailed("a", FailureProvider, "boom")
	b.SetRunFailure(RunFailureInternal, "sched")
	m1 := mustFinalize(t, b)
	m2 := mustFinalize(t, b)
	m1.Coverage.Failed[0].Reason = "MUTATED"
	m1.Coverage.Selected[0].Path = "MUTATED"
	m1.RunFailure.Reason = "MUTATED"
	if m2.Coverage.Failed[0].Reason == "MUTATED" || m2.Coverage.Selected[0].Path == "MUTATED" {
		t.Fatal("returned manifests share backing arrays")
	}
	if m2.RunFailure.Reason == "MUTATED" {
		t.Fatal("returned manifests share run_failure pointer")
	}
}

// A zero-value builder (not via NewManifestBuilder) must not panic on use.
func TestZeroValueBuilderSafe(t *testing.T) {
	b := &ManifestBuilder{}
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	b.RegisterSelected(CoverageItem{ItemID: "a", Path: "a.go"})
	b.MarkCompleted("a")
	m, err := b.Finalize(0)
	// A zero-value builder has an empty run_id, which is rejected by validation.
	if err == nil {
		t.Fatal("zero-value builder (empty run_id) should fail validation")
	}
	_ = m
}

// Concurrent registration and transitions on distinct items must be race-free
// (run with -race) and produce exactly-once coverage.
func TestConcurrentTransitions(t *testing.T) {
	const n = 200
	b := NewManifestBuilder("run-1", "review")
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("item-%03d", i)
		b.RegisterSelected(sel(id))
	}
	if err := b.SealSelected(); err != nil {
		t.Fatalf("SealSelected: %v", err)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("item-%03d", i)
			switch i % 3 {
			case 0:
				b.MarkCompleted(id)
			case 1:
				b.MarkReused(id)
			case 2:
				b.MarkFailed(id, FailureProvider, "e")
			}
		}(i)
	}
	wg.Wait()
	m := mustFinalize(t, b)
	total := len(m.Coverage.Completed) + len(m.Coverage.Reused) + len(m.Coverage.Failed) + len(m.Coverage.Waived)
	if len(m.Coverage.Selected) != n || total != n {
		t.Fatalf("selected=%d total=%d, want %d each", len(m.Coverage.Selected), total, n)
	}
}
