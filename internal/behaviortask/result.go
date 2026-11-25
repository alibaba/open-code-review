// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/pathutil"
)

const SchemaVersion = "ocr.behavior-impact/v2"
const ToolsVersion = "ocr.behavior-tools/v2"

type Input struct {
	GitRunner       *gitcmd.Runner
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
	TotalTokens      int64 `json:"total_tokens"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

type Result struct {
	SchemaVersion       string                      `json:"schema_version"`
	Comparison          Comparison                  `json:"comparison"`
	Scope               []Change                    `json:"scope"`
	Analysis            *Analysis                   `json:"-"`
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

type Artifacts struct {
	directory string
}

func CreateArtifacts(repo, directory string) (*Artifacts, error) {
	parent, err := pathutil.CanonicalPath(filepath.Dir(directory))
	if err != nil {
		return nil, err
	}
	repo, err = pathutil.CanonicalPath(repo)
	if err != nil {
		return nil, err
	}
	directory = filepath.Join(parent, filepath.Base(directory))
	if pathutil.WithinBase(repo, directory) {
		return nil, ErrArtifactLocation
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return nil, err
	}
	return &Artifacts{directory: directory}, nil
}

type SavedResult struct {
	Result
	Status             string `json:"execution_status"`
	DeclaredCompletion string `json:"declared_completion,omitempty"`
}

func (result Result) ExecutionStatus(runErr error) string {
	if result.Analysis != nil {
		if result.Completed && runErr == nil && len(result.Errors) == 0 && result.StopReason == "none" {
			return "completed"
		}
		return "partial"
	}
	var view *gitcmd.UnsupportedViewError
	if errors.As(runErr, &view) {
		return "skipped"
	}
	return "failed"
}

func (artifacts *Artifacts) Save(result Result, status string) error {
	saved := SavedResult{Result: result, Status: status}
	if result.Analysis != nil {
		saved.DeclaredCompletion = result.Analysis.DeclaredCompletion
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(artifacts.directory, ".result-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(append(data, '\n'))
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(artifacts.directory, "result.json"))
}
