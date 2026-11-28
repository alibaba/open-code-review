// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/capacity"
	"github.com/alibaba/open-code-review/internal/session"
)

type capacityMockProvider struct {
	*mockResultProvider
	capErr *capacity.Error
}

func (m *capacityMockProvider) CapacityError() *capacity.Error { return m.capErr }

func TestResolveChangesetLimits(t *testing.T) {
	tests := []struct {
		name              string
		tplFiles, cliFile int
		tplBytes, cliByte int64
		wantFiles         int
		wantBytes         int64
	}{
		{name: "defaults stay unlimited", tplFiles: 0, cliFile: 0, tplBytes: 0, cliByte: 0, wantFiles: 0, wantBytes: 0},
		{name: "cli overrides template files", tplFiles: 10, cliFile: 5, tplBytes: 0, cliByte: 0, wantFiles: 5, wantBytes: 0},
		{name: "cli overrides template bytes", tplFiles: 0, cliFile: 0, tplBytes: 10, cliByte: 5, wantFiles: 0, wantBytes: 5},
		{name: "template applies without cli", tplFiles: 10, cliFile: 0, tplBytes: 10, cliByte: 0, wantFiles: 10, wantBytes: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFiles, gotBytes := resolveChangesetLimits(tt.tplFiles, tt.cliFile, tt.tplBytes, tt.cliByte)
			if gotFiles != tt.wantFiles || gotBytes != tt.wantBytes {
				t.Errorf("resolveChangesetLimits = (%d, %d), want (%d, %d)", gotFiles, gotBytes, tt.wantFiles, tt.wantBytes)
			}
		})
	}
}

func TestValidateReviewOptions_RejectsNegativeChangesetLimits(t *testing.T) {
	opts := reviewOptions{maxChangesetFiles: -1, outputFormat: "text", audience: "human"}
	err := validateReviewOptions(&opts)
	if err == nil || !strings.Contains(err.Error(), "--max-changeset-files") {
		t.Fatalf("expected --max-changeset-files validation error, got %v", err)
	}
	opts = reviewOptions{maxChangesetBytes: -1, outputFormat: "text", audience: "human"}
	err = validateReviewOptions(&opts)
	if err == nil || !strings.Contains(err.Error(), "--max-changeset-bytes") {
		t.Fatalf("expected --max-changeset-bytes validation error, got %v", err)
	}
}

func TestValidateScanOptions_RejectsNegativeChangesetLimits(t *testing.T) {
	opts := scanOptions{maxChangesetFiles: -1, outputFormat: "text", audience: "human"}
	err := validateScanOptions(&opts)
	if err == nil || !strings.Contains(err.Error(), "--max-changeset-files") {
		t.Fatalf("expected --max-changeset-files validation error, got %v", err)
	}
	opts = scanOptions{maxChangesetBytes: -1, outputFormat: "text", audience: "human"}
	err = validateScanOptions(&opts)
	if err == nil || !strings.Contains(err.Error(), "--max-changeset-bytes") {
		t.Fatalf("expected --max-changeset-bytes validation error, got %v", err)
	}
}

func TestEmitRunResult_JSONWithCapacityError(t *testing.T) {
	capErr := &capacity.Error{
		Code:           capacity.Code,
		Stage:          capacity.StageReviewChangeset,
		RequestedFiles: 120,
		LimitFiles:     100,
		RequestedBytes: 5 << 20,
		LimitBytes:     1 << 20,
	}
	ag := &capacityMockProvider{
		mockResultProvider: &mockResultProvider{
			filesReviewed: 0,
			manifest:      mockManifest(session.StateFailed),
			sessionID:     "sess-capacity",
		},
		capErr: capErr,
	}

	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v (payload: %s)", err, got)
	}
	if out.Status != string(session.StateFailed) {
		t.Errorf("status = %q, want %q", out.Status, session.StateFailed)
	}
	if out.Error == nil {
		t.Fatal("error object missing from JSON output")
	}
	if out.Error.Code != capacity.Code || out.Error.Stage != capacity.StageReviewChangeset {
		t.Errorf("unexpected error payload: %+v", out.Error)
	}
	if out.Error.RequestedFiles != 120 || out.Error.LimitFiles != 100 {
		t.Errorf("unexpected error counts: %+v", out.Error)
	}
	if out.Error.LimitBytes != 1<<20 || out.Error.RequestedBytes != 5<<20 {
		t.Errorf("byte fields must stay present even when only one class fired: %+v", out.Error)
	}
	if out.SessionID != "sess-capacity" {
		t.Errorf("session_id = %q, want sess-capacity", out.SessionID)
	}
}

func TestEmitRunResult_JSONWithoutCapacityErrorOmitsField(t *testing.T) {
	ag := &capacityMockProvider{
		mockResultProvider: &mockResultProvider{filesReviewed: 2},
		capErr:             nil,
	}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v (payload: %s)", err, got)
	}
	if out.Error != nil {
		t.Errorf("clean run must not carry an error object: %+v", out.Error)
	}
}

func TestEmitFailureUsage_JSONCarriesCapacityError(t *testing.T) {
	capErr := &capacity.Error{
		Stage:          capacity.StageScanChangeset,
		RequestedFiles: 2,
		LimitFiles:     1,
	}
	ag := &capacityMockProvider{
		mockResultProvider: &mockResultProvider{sessionID: "sess-scan-capacity"},
		capErr:             capErr,
	}

	got := captureStderr(t, func() {
		emitFailureUsage(ag, time.Second, "json", nil, nil)
	})

	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal stderr JSON: %v (payload: %s)", err, got)
	}
	if out.Status != "failed" {
		t.Errorf("status = %q, want failed", out.Status)
	}
	if out.Error == nil || out.Error.Stage != capacity.StageScanChangeset {
		t.Fatalf("structured rejection missing on the failure-usage record: %+v", out.Error)
	}
	if out.SessionID != "sess-scan-capacity" {
		t.Errorf("session_id = %q", out.SessionID)
	}
}
