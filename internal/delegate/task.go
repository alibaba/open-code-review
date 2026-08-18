// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package delegate

import (
	"bytes"
	"encoding/json"
)

// TaskSchemaVersion distinguishes the aggregated task spec (v2) from the
// standalone preview/rule documents (v1). A host agent keys on this to pick the
// correct parser; v2 groups scope + excludes + rules + background + diffs into a
// single document.
const TaskSchemaVersion = "2"

// defaultAcceptanceCriteria are OCR's built-in constraints handed to the host
// agent so its behaviour is deterministic and verifiable: it must only comment
// on reviewable files, justify each comment against a rule or convention, and
// never touch excluded files.
var defaultAcceptanceCriteria = []string{
	"Host agent must only comment on reviewable_files",
	"Each comment must cite a matching rule or convention",
	"Never modify excluded_files",
}

// DefaultAcceptanceCriteria returns a copy of OCR's built-in acceptance criteria
// so callers cannot mutate the shared package default.
func DefaultAcceptanceCriteria() []string {
	out := make([]string, len(defaultAcceptanceCriteria))
	copy(out, defaultAcceptanceCriteria)
	return out
}

// TaskFile is one file entry in a task scope (reviewable or excluded).
type TaskFile struct {
	Path          string `json:"path"`
	Status        string `json:"status"`
	Insertions    int64  `json:"insertions"`
	Deletions     int64  `json:"deletions"`
	ExcludeReason string `json:"exclude_reason,omitempty"`
}

// TaskScope is the resolved review scope. It reuses the shape of preview's
// scope but lifts `repository` to the TaskSpec top level (see TaskSpec.Repository)
// so the aggregated document does not duplicate it.
type TaskScope struct {
	Mode            string     `json:"mode"`
	From            string     `json:"from,omitempty"`
	To              string     `json:"to,omitempty"`
	Commit          string     `json:"commit,omitempty"`
	MergeBase       string     `json:"merge_base,omitempty"`
	TotalFiles      int        `json:"total_files"`
	ReviewableCount int        `json:"reviewable_count"`
	ExcludedCount   int        `json:"excluded_count"`
	TotalInsertions int64      `json:"total_insertions"`
	TotalDeletions  int64      `json:"total_deletions"`
	ReviewableFiles []TaskFile `json:"reviewable_files"`
	ExcludedFiles   []TaskFile `json:"excluded_files"`
}

// TaskRuleGroup mirrors ruleGroupsJSON: a cluster of files sharing one resolved
// rule (identified by source + pattern + text).
type TaskRuleGroup struct {
	GroupID int      `json:"group_id"`
	Source  string   `json:"source"`
	Pattern string   `json:"pattern"`
	Files   []string `json:"files"`
	Rule    string   `json:"rule"`
}

// TaskDiff is the real change content handed to the host agent - the piece the
// standalone preview output omits. `Hunk` is the unified diff for one file.
type TaskDiff struct {
	Path string `json:"path"`
	Hunk string `json:"hunk"`
}

// TaskSpec is the single structured review task produced by `ocr delegate task`.
// It aggregates the scope, excludes, rules, background, and the collected diffs
// so a host coding agent can take over the review deterministically. No field in
// this document carries secrets; it is safe to share with an external agent.
type TaskSpec struct {
	SchemaVersion      string          `json:"schema_version"`
	Repository         string          `json:"repository,omitempty"`
	Scope              TaskScope       `json:"scope"`
	Excludes           []string        `json:"excludes,omitempty"`
	Rules              []TaskRuleGroup `json:"rules"`
	Background         string          `json:"background,omitempty"`
	Diffs              []TaskDiff      `json:"diffs"`
	AcceptanceCriteria []string        `json:"acceptance_criteria"`
}

// AsJSON marshals the spec with stable two-space indentation and HTML escaping
// disabled, matching the CLI output path (writeDelegateJSON) so library
// consumers produce byte-identical JSON to the CLI.
func (s TaskSpec) AsJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// AsMarkdown renders a field-faithful Markdown view (see TaskMarkdown).
func (s TaskSpec) AsMarkdown() string {
	return TaskMarkdown(s)
}
