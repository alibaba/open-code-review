// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStagedManifestIdentityAndSchema(t *testing.T) {
	for _, width := range []int{40, 64} {
		for _, unborn := range []bool{false, true} {
			b := newBuilderWith("a")
			in := ManifestInput{Mode: InputModeStaged, SnapshotTree: strings.Repeat("a", width)}
			if !unborn {
				in.ResolvedBase = strings.Repeat("b", width)
			}
			b.SetInput(in)
			b.MarkCompleted("a")
			m := mustFinalize(t, b)
			if m.SchemaVersion != StagedManifestSchemaVersion || !m.HasSupportedSchema() || m.Input != in {
				t.Fatalf("incorrect staged identity: %+v", m)
			}
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "resolved_head") || strings.Contains(string(data), "exact_range") {
				t.Fatalf("tree presented as commit: %s", data)
			}
		}
	}
	for _, mode := range []string{InputModeWorkspace, InputModeCommit, InputModeRange} {
		b := newBuilderWith()
		b.SetInput(ManifestInput{Mode: mode})
		if m := mustFinalize(t, b); m.SchemaVersion != ManifestSchemaVersion || !m.HasSupportedSchema() {
			t.Fatalf("changed existing manifest contract: %+v", m)
		}
	}
	for _, m := range []RunManifest{
		{SchemaVersion: "ocr.run-manifest/v3"},
		{SchemaVersion: StagedManifestSchemaVersion, Input: ManifestInput{Mode: InputModeRange}},
	} {
		if m.HasSupportedSchema() {
			t.Fatalf("accepted unknown contract: %+v", m)
		}
	}
}

func TestStagedManifestRejectsAmbiguousIdentity(t *testing.T) {
	valid := ManifestInput{Mode: InputModeStaged, SnapshotTree: strings.Repeat("a", 40)}
	for _, tc := range []struct {
		name   string
		change func(*ManifestInput)
	}{
		{"no tree", func(in *ManifestInput) { in.SnapshotTree = "" }},
		{"mutable tree", func(in *ManifestInput) { in.SnapshotTree = "HEAD" }},
		{"invalid hex", func(in *ManifestInput) { in.SnapshotTree = strings.Repeat("z", 40) }},
		{"mutable base", func(in *ManifestInput) { in.ResolvedBase = "main" }},
		{"commit head", func(in *ManifestInput) { in.ResolvedHead = strings.Repeat("b", 40) }},
		{"range", func(in *ManifestInput) { in.ExactRange = "base..head" }},
		{"requested base", func(in *ManifestInput) { in.RequestedFrom = "main" }},
		{"requested head", func(in *ManifestInput) { in.RequestedHead = "HEAD" }},
		{"wrong mode", func(in *ManifestInput) { in.Mode = InputModeWorkspace }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.change(&in)
			b := newBuilderWith()
			b.SetInput(in)
			if _, err := b.Finalize(0); err == nil {
				t.Fatal("ambiguous staged identity passed validation")
			}
		})
	}
	b := newBuilderWith()
	b.SetInput(valid)
	b.SetParentRunID("parent")
	if _, err := b.Finalize(0); err == nil {
		t.Fatal("staged resume lineage passed validation")
	}
}
