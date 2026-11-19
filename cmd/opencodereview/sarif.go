// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// --- SARIF v2.1.0 structure definitions (OASIS standard) ---
//
// Only the subset required by OpenCodeReview is modeled here. The structures
// conform to the SARIF v2.1.0 schema (json.schemastore.org/sarif-2.1.0.json):
// result.locations is an array (not location), replacement.deletedRegion is
// required, and run.invocations carries execution status + notifications.

const (
	sarifSchema         = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion        = "2.1.0"
	sarifToolName       = "OpenCodeReview"
	sarifInformationURI = "https://github.com/alibaba/open-code-review"
	sarifFingerprintKey = "ocrFinding/v1"
)

type sarifReport struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Results     []sarifResult     `json:"results"`
	Invocations []sarifInvocation `json:"invocations,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	Fixes               []sarifFix        `json:"fixes,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

type sarifFix struct {
	ArtifactChanges []sarifArtifactChange `json:"artifactChanges"`
}

type sarifArtifactChange struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Replacements     []sarifReplacement    `json:"replacements"`
}

// sarifReplacement models a SARIF replacement. deletedRegion is required by
// the SARIF schema, so it is NOT omitempty — callers must only build a
// replacement when a valid region exists.
type sarifReplacement struct {
	DeletedRegion   sarifRegion           `json:"deletedRegion"`
	InsertedContent *sarifInsertedContent `json:"insertedContent,omitempty"`
}

type sarifInsertedContent struct {
	Text string `json:"text"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifInvocation struct {
	ExecutionSuccessful        bool                `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications,omitempty"`
}

type sarifNotification struct {
	Level   string       `json:"level"`
	Message sarifMessage `json:"message"`
}

// outputSARIF writes a SARIF v2.1.0 document to the provided writer. The document always
// contains a single run with the 8 built-in category rules, one result per
// LlmComment, and an invocation block carrying execution status and any
// warnings as tool execution notifications. When comments is empty or nil,
// results is an empty array (not null), so the document remains structurally
// valid for SARIF consumers.
func outputSARIF(comments []model.LlmComment, version string, warnings []agent.AgentWarning, manifest *session.RunManifest, out io.Writer) error {
	report := sarifReport{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           sarifToolName,
					Version:        version,
					InformationURI: sarifInformationURI,
					Rules:          sarifRules(),
				},
			},
			Results:     sarifResults(comments),
			Invocations: []sarifInvocation{sarifInvocationFromRun(warnings, manifest, len(comments))},
		}},
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// sarifRules returns the 8 built-in category rule definitions. Every result's
// ruleId must resolve to one of these rules.
func sarifRules() []sarifRule {
	return []sarifRule{
		{ID: "bug", Name: "Bug", ShortDescription: sarifMessage{Text: "Defect or logic error"}},
		{ID: "security", Name: "Security", ShortDescription: sarifMessage{Text: "Security vulnerability"}},
		{ID: "performance", Name: "Performance", ShortDescription: sarifMessage{Text: "Performance issue"}},
		{ID: "maintainability", Name: "Maintainability", ShortDescription: sarifMessage{Text: "Maintainability concern"}},
		{ID: "test", Name: "Test", ShortDescription: sarifMessage{Text: "Test coverage or quality issue"}},
		{ID: "style", Name: "Style", ShortDescription: sarifMessage{Text: "Code style issue"}},
		{ID: "documentation", Name: "Documentation", ShortDescription: sarifMessage{Text: "Documentation issue"}},
		{ID: "other", Name: "Other", ShortDescription: sarifMessage{Text: "Other review finding"}},
	}
}

// sarifSeverityLevel maps an OCR severity string to a SARIF result level.
// critical/high → error, medium → warning, low → note.
// Empty/unknown falls back to "note", matching normalizeCodeCommentSeverity
// in internal/tool/code_comment.go which defaults unknown severities to "low"
// (→ "note" at the SARIF level). This prevents an empty severity from being
// bumped to "warning", which would overstate the finding's importance.
func sarifSeverityLevel(severity string) string {
	switch severity {
	case "critical", "high":
		return "error"
	case "medium":
		return "warning"
	case "low":
		return "note"
	default:
		return "note"
	}
}

// sarifResults converts a slice of LlmComment to SARIF result objects. Returns
// a non-nil empty slice when comments is empty or nil so that JSON serialises
// "results": [] rather than null.
//
// When multiple findings produce the same base fingerprint (e.g. two identical
// SQL injection patterns in the same file), an occurrence index is appended to
// each duplicate so GitHub Code Scanning tracks them as separate alerts rather
// than folding them into one.
func sarifResults(comments []model.LlmComment) []sarifResult {
	results := make([]sarifResult, 0, len(comments))
	// Track occurrence counts per base fingerprint to disambiguate duplicates.
	seen := make(map[string]int, len(comments))
	for _, c := range comments {
		r := sarifResultFromComment(c)
		// Disambiguate duplicate fingerprints by appending an occurrence index.
		baseFP := r.PartialFingerprints[sarifFingerprintKey]
		count := seen[baseFP]
		seen[baseFP] = count + 1
		if count > 0 {
			r.PartialFingerprints[sarifFingerprintKey] = baseFP + "#" + strconv.Itoa(count)
		}
		results = append(results, r)
	}
	return results
}

// sarifResultFromComment maps a single LlmComment to a SARIF result. Field
// mapping follows FR-3 in the requirements spec:
//   - Path          → locations[].physicalLocation.artifactLocation.uri
//   - StartLine/EndLine (valid range) → locations[].physicalLocation.region
//   - Content       → message.text
//   - Category (or "other") → ruleId
//   - Severity      → level
//   - SuggestionCode + ExistingCode + valid region → fixes
//   - Path + Category + ExistingCode → partialFingerprints (stable fingerprint)
//
// Fixes are only emitted when a valid region exists (StartLine > 0 &&
// EndLine >= StartLine), because replacement.deletedRegion is required by
// the SARIF schema and cannot be omitted. When the region is invalid (zero
// or inverted), the suggestion is still conveyed in message.text but no
// machine-readable fix is emitted.
func sarifResultFromComment(c model.LlmComment) sarifResult {
	category := c.Category
	if category == "" {
		category = "other"
	}

	result := sarifResult{
		RuleID:              category,
		Level:               sarifSeverityLevel(c.Severity),
		Message:             sarifMessage{Text: c.Content},
		PartialFingerprints: sarifFingerprints(c, category),
	}

	hasRegion := c.StartLine > 0 && c.EndLine >= c.StartLine

	if c.Path != "" {
		loc := sarifLocation{
			PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: c.Path},
			},
		}
		if hasRegion {
			loc.PhysicalLocation.Region = &sarifRegion{
				StartLine: c.StartLine,
				EndLine:   c.EndLine,
			}
		}
		result.Locations = []sarifLocation{loc}
	}

	// Fixes require: non-empty SuggestionCode, non-empty ExistingCode, non-empty
	// Path, AND a valid region. The region is needed because deletedRegion is
	// required by the SARIF schema — omitting it invalidates the entire document.
	if c.SuggestionCode != "" && c.ExistingCode != "" && c.Path != "" && hasRegion {
		rep := sarifReplacement{
			DeletedRegion: sarifRegion{
				StartLine: c.StartLine,
				EndLine:   c.EndLine,
			},
			InsertedContent: &sarifInsertedContent{Text: c.SuggestionCode},
		}
		result.Fixes = []sarifFix{{
			ArtifactChanges: []sarifArtifactChange{{
				ArtifactLocation: sarifArtifactLocation{URI: c.Path},
				Replacements:     []sarifReplacement{rep},
			}},
		}}
	}

	return result
}

// sarifFingerprints builds a stable partialFingerprints entry for a finding.
// The fingerprint is based on Path + Category + ExistingCode (not message.text,
// which is LLM prose that varies across runs). This allows GitHub Code Scanning
// to track the same alert across runs instead of closing and reopening it.
//
// When ExistingCode is empty (common for findings without a fix suggestion),
// falling back to StartLine prevents fingerprint collisions between same-file,
// same-category findings that would otherwise produce identical hashes.
func sarifFingerprints(c model.LlmComment, category string) map[string]string {
	var fingerprintSource string
	if code := strings.TrimSpace(c.ExistingCode); code != "" {
		fingerprintSource = c.Path + "|" + category + "|" + code
	} else {
		fingerprintSource = c.Path + "|" + category + "|" + strconv.Itoa(c.StartLine)
	}
	h := sha256.Sum256([]byte(fingerprintSource))
	return map[string]string{sarifFingerprintKey: hex.EncodeToString(h[:])}
}

// sarifInvocationFromRun builds the run.invocations entry from the manifest's
// terminal state and any warnings collected during the review.
//
// executionSuccessful mapping:
//   - StateFailed → false
//   - StatePartial with an operational failure → false
//   - StatePartial with only budget failures → true
//   - StateSkipped, StateComplete, nil → true
//
// StateSkipped is a fully successful empty run (e.g. a PR that only touches
// excluded paths). Publishability is independent of execution success: even an
// operationally failed partial run is emitted before the CLI returns non-zero.
//
// When the terminal state is not Complete, a notification carrying the
// manifest message is added so consumers can see why the run was non-complete.
func sarifInvocationFromRun(warnings []agent.AgentWarning, manifest *session.RunManifest, findings int) sarifInvocation {
	inv := sarifInvocation{
		ExecutionSuccessful: !reviewManifestRequiresNonZeroExit(manifest),
	}

	// When the run is non-complete, add a notification with the manifest
	// message so consumers can distinguish "clean scan" from "partial/skipped".
	if manifest != nil && manifest.TerminalState != session.StateComplete {
		inv.ToolExecutionNotifications = append(inv.ToolExecutionNotifications,
			sarifNotification{
				Level:   "warning",
				Message: sarifMessage{Text: manifestMessage(manifest, findings)},
			})
	}

	for _, w := range warnings {
		// Subtask diagnostics are only redundant when a manifest froze them
		// into coverage.failed; without a manifest they are the only record
		// of the failure and must reach the consumer.
		if manifest != nil && isSubtaskErrorType(w.Type) {
			continue
		}
		notif := sarifNotification{
			Level:   "warning",
			Message: sarifMessage{Text: w.Message},
		}
		inv.ToolExecutionNotifications = append(inv.ToolExecutionNotifications, notif)
	}
	return inv
}
