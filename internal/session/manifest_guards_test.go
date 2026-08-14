// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"errors"
	"testing"
)

// TestManifestBuilderNilReceiver covers the `if b == nil` guard on every
// exported ManifestBuilder method: a nil builder must never panic, the mutating
// error-returning methods return errNilBuilder, and the boolean/void methods
// degrade quietly.
func TestManifestBuilderNilReceiver(t *testing.T) {
	var b *ManifestBuilder

	// Void setters: must not panic on a nil receiver.
	b.SetParentRunID("parent")
	b.SetRepository(ManifestRepository{})
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	b.SetExecution(ManifestExecution{})

	// Error-returning methods: must report errNilBuilder.
	errReturners := map[string]error{
		"SetRunFailure":          b.SetRunFailure(RunFailureBudget, "r"),
		"SetPendingFailureCause": b.SetPendingFailureCause(FailureBudget, "r"),
		"RegisterSelected":       b.RegisterSelected(CoverageItem{ItemID: "a"}),
		"SealSelected":           b.SealSelected(),
		"MarkCompleted":          b.MarkCompleted("a"),
		"MarkReused":             b.MarkReused("a"),
		"MarkFailed":             b.MarkFailed("a", FailureProvider, "r"),
		"MarkWaived":             b.MarkWaived("a", "r"),
	}
	for name, err := range errReturners {
		if !errors.Is(err, errNilBuilder) {
			t.Errorf("%s on nil builder: got %v, want errNilBuilder", name, err)
		}
	}

	if _, err := b.Finalize(0); !errors.Is(err, errNilBuilder) {
		t.Errorf("Finalize on nil builder: got %v, want errNilBuilder", err)
	}

	// Boolean predicates: must report false on a nil receiver.
	if b.Sealed() {
		t.Error("Sealed on nil builder should be false")
	}
	if b.Frozen() {
		t.Error("Frozen on nil builder should be false")
	}
}

// TestManifestBuilderFrozenNoOp covers the frozen-branch no-ops of the void
// setters and the errFrozen paths of the mutating methods after Finalize has
// frozen the builder.
func TestManifestBuilderFrozenNoOp(t *testing.T) {
	b := NewManifestBuilder("run-frozen", "review")
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	b.SetParentRunID("orig-parent")
	if _, err := b.Finalize(0); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !b.Frozen() {
		t.Fatal("builder should be frozen after Finalize")
	}

	// Void setters must silently no-op once frozen (no panic, no mutation).
	b.SetParentRunID("changed")
	b.SetRepository(ManifestRepository{})
	b.SetInput(ManifestInput{Mode: InputModeCommit})
	b.SetExecution(ManifestExecution{})

	// Mutating methods must report errFrozen.
	frozenReturners := map[string]error{
		"SetRunFailure":          b.SetRunFailure(RunFailureBudget, "r"),
		"SetPendingFailureCause": b.SetPendingFailureCause(FailureBudget, "r"),
		"RegisterSelected":       b.RegisterSelected(CoverageItem{ItemID: "z"}),
		"SealSelected":           b.SealSelected(),
		"MarkCompleted":          b.MarkCompleted("a"),
	}
	for name, err := range frozenReturners {
		if !errors.Is(err, errFrozen) {
			t.Errorf("%s after freeze: got %v, want errFrozen", name, err)
		}
	}

	// The frozen manifest must still report the original parent, proving the
	// post-freeze SetParentRunID was a no-op.
	m, err := b.Finalize(0)
	if err != nil {
		t.Fatalf("idempotent Finalize: %v", err)
	}
	if m.ParentRunID != "orig-parent" {
		t.Errorf("parent_run_id = %q, want unchanged %q", m.ParentRunID, "orig-parent")
	}
}
