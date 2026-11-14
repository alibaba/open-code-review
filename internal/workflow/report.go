// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workflow

import (
	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
)

type Report struct {
	SchemaVersion       string                      `json:"schema_version"`
	Status              string                      `json:"execution_status"`
	Markdown            string                      `json:"-"`
	Comparison          behaviortask.Comparison     `json:"comparison"`
	Scope               []behaviortask.Change       `json:"scope"`
	Completed           bool                        `json:"submission_accepted"`
	DeclaredCompletion  string                      `json:"declared_completion"`
	StopReason          string                      `json:"stop_reason"`
	SubmissionStatus    string                      `json:"submission_status"`
	Errors              []string                    `json:"errors"`
	Warnings            []llmloop.AgentWarning      `json:"warnings"`
	ToolFailures        []llmloop.ToolFailureDetail `json:"tool_failures"`
	Usage               behaviortask.Usage          `json:"usage"`
	Model               string                      `json:"model"`
	TemplateVersion     string                      `json:"template_version"`
	ToolsVersion        string                      `json:"tools_version"`
	SessionID           string                      `json:"session_id"`
	ArtifactDir         string                      `json:"artifact_dir"`
	ObservationIDs      []string                    `json:"observation_ids"`
	CitedObservationIDs []string                    `json:"cited_observation_ids"`
	ContextIndex        []behaviortask.Context      `json:"context_index"`
	RetryReport         *llm.RetryReport            `json:"retry_report,omitempty"`
}

func NewReport(result behaviortask.Result, runErr error, directory string) *Report {
	report := &Report{SchemaVersion: "ocr.review-analysis/v2", Status: "failed", DeclaredCompletion: "unknown",
		Comparison: result.Comparison, Scope: result.Scope, Completed: result.Completed, StopReason: result.StopReason, SubmissionStatus: result.SubmissionStatus,
		Errors: append([]string{}, result.Errors...), Warnings: result.Warnings, ToolFailures: result.ToolFailures, Usage: result.Usage, Model: result.Model,
		TemplateVersion: result.TemplateVersion, ToolsVersion: result.ToolsVersion, SessionID: result.SessionID, ArtifactDir: directory, ObservationIDs: result.ObservationIDs,
		CitedObservationIDs: append([]string{}, result.CitedObservationIDs...), ContextIndex: result.ContextIndex}
	if runErr != nil && len(report.Errors) == 0 {
		report.Errors = append(report.Errors, runErr.Error())
	}
	if result.Analysis != nil {
		report.Markdown = result.Analysis.Markdown
		if result.Analysis.DeclaredCompletion != "" {
			report.DeclaredCompletion = result.Analysis.DeclaredCompletion
		}
		report.Status = "partial"
		if result.Completed && len(report.Errors) == 0 && result.StopReason == "none" {
			report.Status = "completed"
		}
	}
	return report
}
