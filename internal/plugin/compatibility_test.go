// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestHostCompatibility(t *testing.T) {
	for _, test := range []struct{ minimum, host, want string }{
		{"1.2.3", "v1.2.3", ""},
		{"v1.2.3", "1.10.0", ""},
		{"1.2.3", "v1.2.3+build", ""},
		{"1.2.3-rc.1", "v1.2.3", ""},
		{"1.2.3", "1.2.3-rc.1", "requires OCR"},
		{"1.2.3", "1.2.2", "requires OCR"},
		{"1.2.3", "dev", "cannot be compared"},
		{"1.2", "1.3.0", "full semantic version"},
		{"01.2.3", "1.3.0", "full semantic version"},
		{"latest", "1.3.0", "full semantic version"},
	} {
		t.Run(test.minimum+"/"+test.host, func(t *testing.T) {
			m := testManifest("example", "1")
			m.Requires = &Requirements{MinOCRVersion: test.minimum}
			err := m.CheckCompatibility(test.host)
			if test.want != "" {
				requireError(t, err, test.want)
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	m := testManifest("example", "1")
	if err := m.CheckCompatibility("dev"); err != nil {
		t.Fatalf("legacy manifest requires a version: %v", err)
	}
	m.Requires = &Requirements{Features: []string{"rules", "mcp", "stdio-v1"}}
	if err := m.CheckCompatibility("dev"); err != nil {
		t.Fatal(err)
	}
	m.Requires.Features = []string{"new-feature"}
	requireError(t, m.CheckCompatibility("1.2.3"), "unsupported capability")
	m.Requires.Features = []string{""}
	requireError(t, m.Validate(), "invalid capability")
}

func TestManifestMetadataAndRequiredFields(t *testing.T) {
	for _, test := range []struct{ extra, want string }{
		{`,"metadata":{"publisher":{"future":"value"},"setup":"not executed"}`, ""},
		{`,"requires":{"future_permission":true}`, "unknown field"},
		{`,"setup":"command"`, "unknown field"},
		{`,"metadata":{"publisher":1,"publisher":2}`, "duplicate JSON key"},
		{`,"requires":{"ocr_min_version":"1.2"}`, "full semantic version"},
	} {
		data := []byte(`{"schema_version":1,"id":"example","name":"Example","version":"1","rules":["rules.json"]` + test.extra + `}`)
		manifest, err := ParseManifest(data)
		if test.want != "" {
			requireError(t, err, test.want)
		} else if err != nil || len(manifest.Metadata) != 2 {
			t.Fatalf("metadata = %+v, %v", manifest.Metadata, err)
		}
	}
}

func TestCompatibilityCheckedBeforeInstallEnableAndExecution(t *testing.T) {
	previous := HostVersion
	HostVersion = "1.2.3"
	t.Cleanup(func() { HostVersion = previous })
	ctx := context.Background()
	store := NewStore(t.TempDir())
	source := testPackage(t, "example", "1")
	m := testManifest("example", "1")
	m.Requires = &Requirements{MinOCRVersion: "1.3.0"}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), m)
	_, err := store.Install(ctx, source, true)
	requireError(t, err, "requires OCR")
	m.Requires.MinOCRVersion = "1.2.0"
	m.Metadata = map[string]json.RawMessage{"publisher": json.RawMessage(`"Example"`)}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), m)
	installed, err := store.Install(ctx, source, false)
	if err != nil {
		t.Fatal(err)
	}
	HostVersion = "1.1.0"
	if _, err := store.List(); err != nil {
		t.Fatalf("cannot inspect incompatible package: %v", err)
	}
	requireError(t, store.SetEnabled("example", true), "requires OCR")
	_, err = StartReview(ctx, installed, t.TempDir())
	requireError(t, err, "requires OCR")
	if err := store.Remove("example"); err != nil {
		t.Fatalf("cannot remove incompatible package: %v", err)
	}
}
