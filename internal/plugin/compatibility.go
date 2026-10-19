// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// HostVersion is set once by the application before loading any plugins.
var HostVersion = "dev"

type Requirements struct {
	MinOCRVersion string   `json:"ocr_min_version,omitempty"`
	Features      []string `json:"features,omitempty"`
}

func normalizedVersion(version string) string {
	return "v" + strings.TrimPrefix(version, "v")
}

func (r Requirements) validate() error {
	if r.MinOCRVersion != "" {
		version := normalizedVersion(r.MinOCRVersion)
		core := strings.SplitN(strings.SplitN(version, "+", 2)[0], "-", 2)[0]
		if !semver.IsValid(version) || strings.Count(core, ".") != 2 {
			return fmt.Errorf("requires.ocr_min_version must be a full semantic version")
		}
	}
	for _, feature := range r.Features {
		if !validID(feature) {
			return fmt.Errorf("requires.features contains an invalid capability name")
		}
	}
	return nil
}

// CheckCompatibility is separate from parsing so incompatible installations can
// still be inspected, disabled, removed or updated after changing OCR versions.
func (m Manifest) CheckCompatibility(hostVersion string) error {
	if m.Requires == nil {
		return nil
	}
	if err := m.Requires.validate(); err != nil {
		return err
	}
	if minimum := m.Requires.MinOCRVersion; minimum != "" {
		host := normalizedVersion(hostVersion)
		if !semver.IsValid(host) {
			return fmt.Errorf("plugin %q requires OCR >= %s; host version %q cannot be compared; use a versioned OCR build", m.ID, minimum, hostVersion)
		}
		if semver.Compare(host, normalizedVersion(minimum)) < 0 {
			return fmt.Errorf("plugin %q requires OCR >= %s; host is %s", m.ID, minimum, hostVersion)
		}
	}
	for _, feature := range m.Requires.Features {
		switch feature {
		case "rules", "mcp", "stdio-v1":
		default:
			return fmt.Errorf("plugin %q requires unsupported capability %q", m.ID, feature)
		}
	}
	return nil
}
