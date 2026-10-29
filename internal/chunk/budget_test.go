// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import "testing"

func TestDeriveBudget_IsDerivedFromTheRequestLimit(t *testing.T) {
	// A 200k ceiling and a 32k ceiling must not produce the same chunk budget:
	// the whole point is that the number follows the provider configuration
	// rather than sitting in the code as a constant.
	big := DeriveBudget(160000, 4000, 16384)
	small := DeriveBudget(20800, 4000, 16384)

	if big.Chunk() <= small.Chunk() {
		t.Errorf("chunk budget must scale with the request limit: 200k gave %d, 32k gave %d", big.Chunk(), small.Chunk())
	}
	if want := 160000 - 4000 - 48000 - 16384; big.Chunk() != want {
		t.Errorf("Chunk() = %d, want %d (limit - overhead - conversation - output)", big.Chunk(), want)
	}
	if !big.Usable() || !small.Usable() {
		t.Errorf("both configurations must be usable: %d, %d", big.Chunk(), small.Chunk())
	}
}

func TestDeriveBudget_OutputReserveCannotEatTheContext(t *testing.T) {
	// A completion ceiling larger than the request limit can afford must not
	// leave nothing to review with.
	b := DeriveBudget(20000, 0, 500000)
	if !b.Usable() {
		t.Fatalf("chunk budget must survive an oversized completion ceiling, got %d", b.Chunk())
	}
	if b.OutputReserve != 4000 {
		t.Errorf("OutputReserve = %d, want it capped at a fifth of the limit (4000)", b.OutputReserve)
	}
}

func TestBudget_UnusableIsZeroNotInfinite(t *testing.T) {
	// The dangerous failure mode is a budget that collapses and silently means
	// "unbounded". It must read as zero so callers fail closed.
	b := DeriveBudget(1000, 900, 2000)
	if b.Usable() {
		t.Errorf("budget with no room should be unusable, Chunk() = %d", b.Chunk())
	}
	if b.Chunk() != 0 || b.Read() != 0 {
		t.Errorf("unusable budget must read as 0, got Chunk=%d Read=%d", b.Chunk(), b.Read())
	}
}

func TestBudget_NormalizesDegenerateInputs(t *testing.T) {
	b := DeriveBudget(0, -50, -10)
	if b.FixedOverhead != 0 {
		t.Errorf("negative overhead must clamp to 0, got %d", b.FixedOverhead)
	}
	if b.OutputReserve < 0 {
		t.Errorf("OutputReserve must not be negative, got %d", b.OutputReserve)
	}
}

func TestBudget_ManifestIsAShareOfTheChunkBudget(t *testing.T) {
	b := DeriveBudget(160000, 4000, 16384)
	if b.Manifest() >= b.Chunk() {
		t.Errorf("manifest budget %d must stay below the chunk budget %d", b.Manifest(), b.Chunk())
	}
	tiny := DeriveBudget(1000, 0, 100)
	if tiny.Manifest() < 1 {
		t.Errorf("manifest budget must stay positive, got %d", tiny.Manifest())
	}
}
