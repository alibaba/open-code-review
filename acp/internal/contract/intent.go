// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// ReviewType represents the type of review operation.
type ReviewType string

const (
	ReviewTypeWorkspace ReviewType = "workspace"
	ReviewTypeRange     ReviewType = "range"
	ReviewTypeCommit    ReviewType = "commit"
)

// ReviewIntent represents a structured review request.
type ReviewIntent struct {
	Type   ReviewType
	From   string // Required for range
	To     string // Required for range
	Commit string // Required for commit

	// Extra holds optional OCR CLI flags forwarded verbatim to the child
	// process. Every element is validated against reviewExtraFlags: unknown
	// flags, missing values, and enum values outside the allowed set are all
	// rejected. Only the flags listed there can be forwarded; integration
	// flags such as --format, --audience, --output and --repo are fixed by
	// the adapters above and cannot be overridden from here.
	Extra []string
}

// ScanIntent represents a structured scan request.
type ScanIntent struct {
	Paths []string // Relative paths to scan; empty means scan root

	// Extra mirrors ReviewIntent.Extra but is validated against
	// scanExtraFlags, which is a different set: the OCR CLI registers
	// different flags on `scan` than on `review`.
	Extra []string
}

// extraFlagSpec describes one flag that callers may forward through Extra.
type extraFlagSpec struct {
	takesValue bool
	// enum, when non-empty, lists the only values the flag accepts. The OCR
	// CLI silently falls back to a default for unknown values on some flags
	// (e.g. --batch), so the adapter rejects them instead of guessing.
	enum []string
}

// reviewExtraFlags is the whitelist for ReviewIntent.Extra. It matches the
// flags registered by registerReviewFlags in cmd/opencodereview/shared_flags.go.
var reviewExtraFlags = map[string]extraFlagSpec{
	"--effort":          {takesValue: true, enum: []string{"low", "medium", "high"}},
	"--no-filter":       {},
	"--background":      {takesValue: true},
	"--background-file": {takesValue: true},
}

// scanExtraFlags is the whitelist for ScanIntent.Extra. It matches the flags
// registered by registerScanFlags in cmd/opencodereview/shared_flags.go.
// Note that `scan` has no --effort and `review` has no --batch, so a flag
// valid on one command is rejected on the other.
var scanExtraFlags = map[string]extraFlagSpec{
	"--batch":      {takesValue: true, enum: []string{"none", "by-language", "by-directory"}},
	"--no-plan":    {},
	"--no-dedup":   {},
	"--no-summary": {},
	"--background": {takesValue: true},
}

// BuildReviewArgs constructs OCR CLI arguments from ReviewIntent.
// Returns error if intent is invalid, required fields are missing, or Extra
// contains a flag outside reviewExtraFlags.
func BuildReviewArgs(intent *ReviewIntent) ([]string, error) {
	args := []string{"review"}

	switch intent.Type {
	case ReviewTypeWorkspace:
		// No additional arguments for workspace review
	case ReviewTypeRange:
		if intent.From == "" && intent.To == "" {
			return nil, fmt.Errorf("range review requires both --from and --to")
		} else if intent.From == "" {
			return nil, fmt.Errorf("range review requires --from")
		} else if intent.To == "" {
			return nil, fmt.Errorf("range review requires --to")
		}
		args = append(args, "--from", intent.From, "--to", intent.To)
	case ReviewTypeCommit:
		if intent.Commit == "" {
			return nil, fmt.Errorf("commit review requires --commit")
		}
		args = append(args, "--commit", intent.Commit)
	default:
		return nil, fmt.Errorf("unknown review type: %s", intent.Type)
	}

	if err := validateExtra(intent.Extra, reviewExtraFlags); err != nil {
		return nil, err
	}
	args = append(args, intent.Extra...)

	// Fixed flags for ACP integration, appended last. Flag parsing is
	// last-wins, so even a flag that slipped past the whitelist could not
	// override these.
	args = append(args, "--format", "json", "--audience", "human", "--color", "never")

	return args, nil
}

// BuildScanArgs constructs OCR CLI arguments from ScanIntent.
// Returns error if a path escapes the scan root or Extra contains a flag
// outside scanExtraFlags.
func BuildScanArgs(intent *ScanIntent) ([]string, error) {
	args := []string{"scan"}

	// Add paths if specified
	for _, p := range intent.Paths {
		cleaned, err := normalizePath(p)
		if err != nil {
			return nil, err
		}
		args = append(args, "--path", cleaned)
	}

	if err := validateExtra(intent.Extra, scanExtraFlags); err != nil {
		return nil, err
	}
	args = append(args, intent.Extra...)

	// Fixed flags for ACP integration, appended last. Flag parsing is
	// last-wins, so even a flag that slipped past the whitelist could not
	// override these.
	args = append(args, "--format", "json", "--audience", "human", "--color", "never")

	return args, nil
}

// validateExtra rejects any element of extra that is not a whitelisted flag,
// is a whitelisted flag missing its value, or carries a value outside the
// flag's enum. Both "--flag value" and "--flag=value" forms are accepted.
//
// A separated value that itself looks like a flag is rejected rather than
// consumed, so ["--effort", "--no-filter"] reports a missing value instead of
// silently reading "--no-filter" as the effort. The same applies to an inline
// value: "--effort=--no-filter" is rejected.
//
// An empty inline value ("--background=") is accepted for flags without an
// enum, because the OCR CLI treats it the same as omitting the flag. Enum
// flags reject it, since "" is not one of their allowed values.
func validateExtra(extra []string, allowed map[string]extraFlagSpec) error {
	for i := 0; i < len(extra); i++ {
		arg := extra[i]

		name := arg
		value := ""
		hasInlineValue := false
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name, value, hasInlineValue = arg[:idx], arg[idx+1:], true
		}

		spec, ok := allowed[name]
		if !ok {
			return fmt.Errorf("flag not allowed: %s", arg)
		}

		if !spec.takesValue {
			// Boolean flags accept a bare form or an explicit --flag=true.
			continue
		}

		if !hasInlineValue {
			i++
			if i >= len(extra) {
				return fmt.Errorf("flag %s requires a value", name)
			}
			value = extra[i]
		}

		if strings.HasPrefix(value, "-") {
			return fmt.Errorf("flag %s requires a value, got %q which looks like a flag", name, value)
		}

		if len(spec.enum) > 0 && !slices.Contains(spec.enum, value) {
			return fmt.Errorf("flag %s: invalid value %q (allowed: %s)",
				name, value, strings.Join(spec.enum, ", "))
		}
	}
	return nil
}

// normalizePath cleans a user-supplied relative path and rejects anything
// that escapes the target root. filepath.Clean collapses "." and "..", but
// it only treats the host separator as a separator: on Unix, "..\\..\\file"
// survives Clean unchanged. We therefore treat both "/" and "\\" as
// separators before cleaning, so a path written for either platform is
// judged the same way on every platform.
func normalizePath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute path not allowed: %s", p)
	}
	// filepath.IsAbs does not recognise Windows drive letters on Unix.
	if len(p) >= 2 && p[1] == ':' && isASCIILetter(p[0]) {
		return "", fmt.Errorf("absolute path not allowed: %s", p)
	}

	unified := strings.ReplaceAll(p, `\`, "/")
	cleaned := filepath.Clean(filepath.FromSlash(unified))

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %s", p)
	}
	// Clean can return an absolute path on Windows after unification.
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute path not allowed: %s", p)
	}
	return cleaned, nil
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
