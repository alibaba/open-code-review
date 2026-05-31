package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/open-code-review/open-code-review/internal/platform"
	"github.com/open-code-review/open-code-review/internal/platform/gitlab"
)

// hasGitLabOperation returns true if any GitLab publish/clear flag is set.
func hasGitLabOperation(opts reviewOptions) bool {
	return opts.publish || opts.clearInline || opts.clearSummary || opts.clearExisting
}

// resolveGitLabReviewOptions builds platform.PublishOptions from CLI flags and environment.
func resolveGitLabReviewOptions(opts reviewOptions, token string) (platform.PublishOptions, error) {
	pubOpts := platform.PublishOptions{
		Platform:         platform.PlatformGitLab,
		ProjectID:        opts.projectID,
		MergeRequestIID:  opts.mrIID,
		BaseURL:          opts.baseURL,
		Token:            token,
		Publish:          opts.publish,
		PRSummary:        opts.prSummary,
		ClearExisting:    opts.clearExisting,
		ClearInline:      opts.clearInline,
		ClearSummary:     opts.clearSummary,
		NoInline:         opts.noInline,
		NoSummaryComment: opts.noSummaryComment,
	}

	// CI/env inference for project ID.
	if pubOpts.ProjectID == "" {
		pubOpts.ProjectID = os.Getenv("CI_PROJECT_ID")
	}

	// CI/env inference for MR IID.
	if pubOpts.MergeRequestIID == 0 {
		if v := os.Getenv("CI_MERGE_REQUEST_IID"); v != "" {
			iid, err := strconv.Atoi(v)
			if err != nil {
				return pubOpts, fmt.Errorf("invalid CI_MERGE_REQUEST_IID %q: must be an integer", v)
			}
			pubOpts.MergeRequestIID = iid
		}
	}

	// Base URL inference: flag > OCR_GITLAB_BASE_URL > CI_SERVER_URL > default.
	if pubOpts.BaseURL == "" {
		pubOpts.BaseURL = os.Getenv("OCR_GITLAB_BASE_URL")
	}
	if pubOpts.BaseURL == "" {
		pubOpts.BaseURL = os.Getenv("CI_SERVER_URL")
	}
	if pubOpts.BaseURL == "" {
		pubOpts.BaseURL = "https://gitlab.com"
	}

	// Token inference: env fallback.
	if pubOpts.Token == "" {
		pubOpts.Token = os.Getenv("GITLAB_TOKEN")
	}
	if pubOpts.Token == "" {
		pubOpts.Token = os.Getenv("OCR_GITLAB_TOKEN")
	}

	// Validate --clear-existing requires --publish.
	if opts.clearExisting && !opts.publish {
		return pubOpts, fmt.Errorf("--clear-existing requires --publish: it deletes OCR comments before publishing")
	}

	// Validate required fields when a GitLab operation is requested.
	needsValidation := opts.publish || opts.clearInline || opts.clearSummary || opts.clearExisting
	if needsValidation {
		if opts.platform != "gitlab" {
			return pubOpts, fmt.Errorf("--publish/--clear flags require --platform gitlab")
		}
		if pubOpts.Token == "" {
			return pubOpts, fmt.Errorf("GitLab token required: set GITLAB_TOKEN or OCR_GITLAB_TOKEN")
		}
		if pubOpts.ProjectID == "" {
			return pubOpts, fmt.Errorf("GitLab project ID required: set --project-id or CI_PROJECT_ID")
		}
		if pubOpts.MergeRequestIID <= 0 {
			return pubOpts, fmt.Errorf("GitLab MR IID required: set --mr or CI_MERGE_REQUEST_IID")
		}
	}

	return pubOpts, nil
}

// printPublishResult prints human-readable publish status unless in JSON or agent mode.
func printPublishResult(opts reviewOptions, result *platform.PublishResult) {
	if opts.outputFormat == "json" || opts.audience == "agent" {
		return
	}
	fmt.Printf("Published %d inline comment(s), %d failed\n", result.InlineCreated, result.InlineFailed)
	if result.SummaryCreated {
		fmt.Println("Created summary comment")
	} else if result.SummaryUpdated {
		fmt.Println("Updated summary comment")
	}
	if result.DescriptionUpdated {
		fmt.Println("Updated MR description")
	}
}

// newGitLabPublisher creates a GitLab Publisher from publish options.
func newGitLabPublisher(opts platform.PublishOptions) *gitlab.Publisher {
	client := gitlab.NewClient(opts.BaseURL, opts.Token)
	return gitlab.NewPublisher(client, opts)
}

// runGitLabClear executes clear-inline and/or clear-summary operations.
func runGitLabClear(opts platform.PublishOptions) error {
	pub := newGitLabPublisher(opts)

	if opts.ClearInline {
		result, err := pub.ClearInline()
		if err != nil {
			return fmt.Errorf("clear inline: %w", err)
		}
		fmt.Printf("Deleted %d inline comment(s)\n", result.InlineDeleted)
	}

	if opts.ClearSummary {
		result, err := pub.ClearSummary()
		if err != nil {
			return fmt.Errorf("clear summary: %w", err)
		}
		fmt.Printf("Deleted %d summary comment(s)\n", result.SummaryDeleted)
	}

	return nil
}
