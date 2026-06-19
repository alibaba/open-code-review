package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/reviewstore"
)

func saveReviewResult(repoDir string, opts reviewOptions, ag *agent.Agent, comments []model.LlmComment, warnings []agent.AgentWarning, duration time.Duration) (string, error) {
	sourceBranch := firstNonEmpty(opts.resultSource, os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"))
	targetBranch := firstNonEmpty(opts.resultTarget, os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"))
	projectName := firstNonEmpty(opts.resultProject, os.Getenv("CI_PROJECT_PATH"), filepath.Base(repoDir))
	projectID := os.Getenv("CI_PROJECT_ID")

	reviewMode := "workspace"
	if opts.commit != "" {
		reviewMode = "commit"
	} else if opts.from != "" && opts.to != "" {
		reviewMode = "range"
	}

	result := reviewstore.Result{
		Project: reviewstore.ProjectInfo{
			ID:      projectID,
			Name:    projectName,
			RepoDir: repoDir,
			WebURL:  os.Getenv("CI_PROJECT_URL"),
		},
		GitLab: reviewstore.GitLabInfo{
			ServerURL:       os.Getenv("CI_SERVER_URL"),
			ProjectID:       projectID,
			MergeRequestIID: os.Getenv("CI_MERGE_REQUEST_IID"),
			PipelineID:      os.Getenv("CI_PIPELINE_ID"),
			JobID:           os.Getenv("CI_JOB_ID"),
		},
		Review: reviewstore.ReviewInfo{
			Mode:             reviewMode,
			SourceBranch:     sourceBranch,
			TargetBranch:     targetBranch,
			From:             opts.from,
			To:               opts.to,
			Commit:           opts.commit,
			Model:            ag.Session().Model,
			FilesReviewed:    ag.FilesReviewed(),
			CommentCount:     int64(len(comments)),
			TotalTokens:      ag.TotalTokensUsed(),
			InputTokens:      ag.TotalInputTokens(),
			OutputTokens:     ag.TotalOutputTokens(),
			CacheReadTokens:  ag.TotalCacheReadTokens(),
			CacheWriteTokens: ag.TotalCacheWriteTokens(),
			Duration:         duration.String(),
			DurationSeconds:  int64(duration.Seconds()),
			SessionID:        ag.Session().SessionID,
		},
		Comments: comments,
		Warnings: mapReviewWarnings(warnings),
	}

	path, err := reviewstore.Save(opts.resultDir, result)
	if err != nil {
		return "", err
	}
	return path, nil
}

func mapReviewWarnings(warnings []agent.AgentWarning) []reviewstore.Warning {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]reviewstore.Warning, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, reviewstore.Warning{
			File:    warning.File,
			Message: warning.Message,
			Type:    warning.Type,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func warnReviewResultSaved(path string) {
	fmt.Fprintf(os.Stderr, "[ocr] review result saved: %s\n", path)
}
