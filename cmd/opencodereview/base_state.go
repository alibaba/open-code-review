// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const baseStateRelativePath = ".scratch/base"

// baseState is the small, human-readable contract shared by the implement and
// code-review skills. The parser is deliberately strict so an agent cannot
// silently review a moving or ambiguous baseline.
type baseState struct {
	BaseSHA string
	Source  string
	Ref     string
	Summary string
}

func baseStatePath(repoDir string) string {
	return filepath.Join(repoDir, filepath.FromSlash(baseStateRelativePath))
}

func loadBaseState(repoDir string) (baseState, error) {
	path := baseStatePath(repoDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return baseState{}, fmt.Errorf("read %s: %w", baseStateRelativePath, err)
	}
	state, err := parseBaseState(string(raw))
	if err != nil {
		return baseState{}, fmt.Errorf("invalid %s: %w", baseStateRelativePath, err)
	}
	return state, nil
}

func parseBaseState(raw string) (baseState, error) {
	var state baseState
	seen := make(map[string]bool, 4)
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return baseState{}, fmt.Errorf("file is empty")
	}

	for lineNo, line := range lines {
		if strings.TrimSpace(line) == "" {
			return baseState{}, fmt.Errorf("line %d is empty", lineNo+1)
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			return baseState{}, fmt.Errorf("line %d must be key: value", lineNo+1)
		}
		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		if seen[key] {
			return baseState{}, fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		switch key {
		case "base_sha":
			state.BaseSHA = value
		case "source":
			state.Source = value
		case "ref":
			state.Ref = value
		case "summary":
			state.Summary = value
		default:
			return baseState{}, fmt.Errorf("unknown field %q", key)
		}
	}

	if err := validateBaseState(state); err != nil {
		return baseState{}, err
	}
	return state, nil
}

func validateBaseState(state baseState) error {
	if len(state.BaseSHA) != 40 {
		return fmt.Errorf("base_sha must be a full 40-character commit SHA")
	}
	if _, err := hex.DecodeString(state.BaseSHA); err != nil {
		return fmt.Errorf("base_sha must contain only hexadecimal characters")
	}
	if state.Source == "" {
		return fmt.Errorf("source is required")
	}
	if state.Source != strings.ToLower(state.Source) || strings.ContainsAny(state.Source, " \t\r\n") {
		return fmt.Errorf("source must be a lowercase provider key")
	}
	if (state.Ref == "") == (state.Summary == "") {
		return fmt.Errorf("exactly one of ref or summary is required")
	}
	if state.Source == "user" {
		if state.Summary == "" {
			return fmt.Errorf("source user requires summary")
		}
		if state.Ref != "" {
			return fmt.Errorf("source user cannot use ref")
		}
	} else if state.Ref == "" {
		return fmt.Errorf("source %q requires ref", state.Source)
	}
	if strings.ContainsAny(state.Ref, "\r\n") || strings.ContainsAny(state.Summary, "\r\n") {
		return fmt.Errorf("ref and summary must each occupy one line")
	}
	return nil
}
