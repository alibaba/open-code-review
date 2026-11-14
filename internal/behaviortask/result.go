// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
)

const SchemaVersion = "ocr.behavior-impact/v2"
const ToolsVersion = "ocr.behavior-tools/v2"

type Input struct {
	RepoDir         string
	Mode            string
	Resolution      diff.InputResolution
	Diffs           []model.Diff
	ExcludePatterns []string
	Contexts        []Context
}

type Comparison struct {
	ID          string `json:"id"`
	Repository  string `json:"repository"`
	Mode        string `json:"mode"`
	Old         string `json:"old"`
	Head        string `json:"head"`
	BaseIsEmpty bool   `json:"base_is_empty"`
}

type Change struct {
	OldPath        string `json:"old_path"`
	NewPath        string `json:"new_path"`
	Deleted        bool   `json:"deleted"`
	Added          bool   `json:"added"`
	Renamed        bool   `json:"renamed"`
	ExcludedReason string `json:"excluded_reason,omitempty"`
}

type Location struct {
	Side      string `json:"side"`
	Revision  string `json:"revision"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type Analysis struct {
	Markdown           string `json:"markdown"`
	DeclaredCompletion string `json:"declared_completion,omitempty"`
}

type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

type Result struct {
	SchemaVersion       string                      `json:"schema_version"`
	Comparison          Comparison                  `json:"comparison"`
	Scope               []Change                    `json:"scope"`
	Analysis            *Analysis                   `json:"analysis"`
	SubmissionStatus    string                      `json:"submission_status"`
	Completed           bool                        `json:"completed"`
	StopReason          string                      `json:"stop_reason"`
	Errors              []string                    `json:"errors"`
	Warnings            []llmloop.AgentWarning      `json:"warnings"`
	ToolFailures        []llmloop.ToolFailureDetail `json:"tool_failures"`
	Usage               Usage                       `json:"usage"`
	Model               string                      `json:"model"`
	TemplateVersion     string                      `json:"template_version"`
	ToolsVersion        string                      `json:"tools_version"`
	SessionID           string                      `json:"session_id"`
	ObservationIDs      []string                    `json:"observation_ids"`
	CitedObservationIDs []string                    `json:"cited_observation_ids"`
	ContextIndex        []Context                   `json:"context_index"`
	MaxRounds           int                         `json:"max_rounds"`
	MaxTokensBudget     int64                       `json:"max_tokens_budget"`
}
