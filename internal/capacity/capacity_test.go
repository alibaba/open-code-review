// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package capacity

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCheck_NoLimitsNeverFire(t *testing.T) {
	if err := Check(StageReviewChangeset, 1_000_000, 0, 1<<40, 0); err != nil {
		t.Fatalf("unlimited limits must never fire, got %v", err)
	}
}

func TestCheck_WithinLimitsPasses(t *testing.T) {
	if err := Check(StageReviewChangeset, 10, 100, 1024, 1<<20); err != nil {
		t.Fatalf("within limits must pass, got %v", err)
	}
}

func TestCheck_LimitIsInclusiveUpperBound(t *testing.T) {
	if err := Check(StageReviewChangeset, 100, 100, 1<<20, 1<<20); err != nil {
		t.Fatalf("exactly-at-limit must pass, got %v", err)
	}
}

func TestCheck_FileLimitFires(t *testing.T) {
	err := Check(StageReviewChangeset, 101, 100, 10, 0)
	if err == nil {
		t.Fatal("expected rejection over the file limit")
	}
	var capErr *Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if capErr.Stage != StageReviewChangeset || capErr.RequestedFiles != 101 || capErr.LimitFiles != 100 {
		t.Fatalf("unexpected fields: %+v", capErr)
	}
	if capErr.LimitBytes != 0 || capErr.RequestedBytes != 10 {
		t.Fatalf("non-fired class must stay present with observed values: %+v", capErr)
	}
	if capErr.Code != Code {
		t.Fatalf("Code = %q, want %q", capErr.Code, Code)
	}
}

func TestCheck_ByteLimitFires(t *testing.T) {
	err := Check(StageScanChangeset, 1, 0, 1<<20+1, 1<<20)
	if err == nil {
		t.Fatal("expected rejection over the byte limit")
	}
	var capErr *Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if capErr.Stage != StageScanChangeset || capErr.RequestedBytes != 1<<20+1 || capErr.LimitBytes != 1<<20 {
		t.Fatalf("unexpected fields: %+v", capErr)
	}
}

func TestError_JSONShapeAlwaysComplete(t *testing.T) {
	data, err := json.Marshal(&Error{Stage: StageReviewChangeset, RequestedFiles: 120, LimitFiles: 100})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"code", "stage", "requested_files", "limit_files", "requested_bytes", "limit_bytes"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("field %q missing from JSON payload %s", key, data)
		}
	}
}

func TestError_MessageCarriesActualVersusLimit(t *testing.T) {
	e := &Error{Code: Code, Stage: StageReviewChangeset, RequestedFiles: 120, LimitFiles: 100, RequestedBytes: 5 << 20, LimitBytes: 1 << 20}
	msg := e.Error()
	for _, want := range []string{Code, StageReviewChangeset, "120", "100"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}
