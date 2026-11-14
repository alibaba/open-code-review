// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package plugin manages installed OCR rule plugins without executing them.
package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
)

const ManifestFilename = "ocr-plugin.json"

// Manifest declares the contributions understood by version 1 of the plugin host.
type Manifest struct {
	SchemaVersion int                        `json:"schema_version"`
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	Version       string                     `json:"version"`
	Description   string                     `json:"description,omitempty"`
	Rules         []string                   `json:"rules,omitempty"`
	Requires      *Requirements              `json:"requires,omitempty"`
	Metadata      map[string]json.RawMessage `json:"metadata,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)

func validID(id string) bool {
	return idPattern.MatchString(id) && !reservedName(id)
}

func reservedName(name string) bool {
	stem := strings.ToUpper(strings.Split(name, ".")[0])
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "COM\u00b9", "COM\u00b2", "COM\u00b3", "LPT\u00b9", "LPT\u00b2", "LPT\u00b3":
		return true
	}
	return len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9'
}

// Validate checks the manifest contract independently of its on-disk package.
func (m Manifest) Validate() error {

	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported plugin schema_version %d (expected 1)", m.SchemaVersion)
	}
	if m.Requires != nil {
		if err := m.Requires.validate(); err != nil {
			return err
		}
	}
	if !validID(m.ID) {
		return fmt.Errorf("invalid plugin id %q: use a portable lowercase name of at most 64 characters", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("plugin name and version are required")
	}
	if len(m.Rules) == 0 {
		return fmt.Errorf("plugin must contribute rules")
	}
	seen := make(map[string]bool)
	for _, rule := range m.Rules {
		if err := safeRelativePath(rule); err != nil {
			return fmt.Errorf("invalid rule path %q: %w", rule, err)
		}
		if !strings.HasSuffix(rule, ".json") {
			return fmt.Errorf("rule path %q must name a JSON file", rule)
		}
		key := strings.ToLower(rule)
		if seen[key] {
			return fmt.Errorf("duplicate rule path %q", rule)
		}
		seen[key] = true
	}
	return nil
}

func ParseManifest(data []byte) (Manifest, error) {

	var m Manifest
	if err := decodeStrict(data, &m); err != nil {
		return m, fmt.Errorf("parse plugin manifest: %w", err)
	}
	return m, m.Validate()
}

func decodeStrict(data []byte, value any) error {
	if err := checkJSONKeys(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON document")
	}
	return nil
}

func checkJSONKeys(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting exceeds 64 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("JSON object key must be a string")
			}
			if seen[name] {
				return fmt.Errorf("duplicate JSON key %q", name)
			}
			seen[name] = true
		}
		if err := checkJSONKeys(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

// safeRelativePath uses forward slashes and rules that remain safe on Windows,
// even when a package is prepared on Unix.
func safeRelativePath(name string) error {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") {
		return fmt.Errorf("path must be relative, normalized and use forward slashes")
	}
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, ". ") != part || strings.ContainsAny(part, `<>"|?*`) || reservedName(part) {
			return fmt.Errorf("path contains a nonportable filename")
		}
		for _, char := range part {
			if char < 32 {
				return fmt.Errorf("path contains a control character")
			}
		}
	}
	return nil
}
