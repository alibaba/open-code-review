// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package ghpost owns discovery and idempotent delivery of review findings to
// the GitHub pull request for the exact immutable range that was reviewed.
package ghpost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/github"
	"github.com/alibaba/open-code-review/internal/model"
)

const (
	reviewBatchSize          = 50
	summaryBatchSize         = 50
	summaryMaxPayloadBytes   = 60 * 1024
	reconciliationAttempts   = 6
	reconciliationPollPeriod = 500 * time.Millisecond
)

// Target identifies the exact reviewed input whose findings may be posted.
type Target struct {
	RepoDir      string
	BaseRef      string
	ResolvedBase string
	ResolvedHead string
}

// Options contains GitHub posting credentials.
type Options struct {
	Token string
}

// Result reports the pull request and finding counts delivered by Post.
type Result struct {
	PullRequestNumber int
	InlineComments    int
	SummaryComments   int
}

type repositoryCandidate struct {
	remote     string
	repository github.Repository
}

type postingTarget struct {
	repositoryCandidate
	prNumber int
}

type summaryFinding struct {
	comment model.LlmComment
	reason  string
}

type summaryBatch struct {
	marker string
	body   string
}

// Post discovers the unique matching pull request, proves its current merge
// base, and posts deterministic review and summary batches.
func Post(ctx context.Context, target Target, comments []model.LlmComment, options Options) (Result, error) {
	if err := validateTarget(target); err != nil {
		return Result{}, err
	}
	configuration, err := github.NewConfigurationFromEnvironment()
	if err != nil {
		return Result{}, err
	}
	repositories, remoteNames, err := repositoriesForGitDir(target.RepoDir, configuration)
	if err != nil {
		return Result{}, err
	}
	baseBranch, err := resolveBranch(target.RepoDir, target.BaseRef, remoteNames)
	if err != nil {
		return Result{}, fmt.Errorf("resolve GitHub base branch from %q: %w", target.BaseRef, err)
	}

	destination, err := discoverTarget(ctx, options.Token, repositories, baseBranch, target.ResolvedHead)
	if err != nil {
		return Result{}, err
	}
	client, err := github.NewClient(options.Token, destination.repository, destination.prNumber)
	if err != nil {
		return Result{}, fmt.Errorf("create GitHub client: %w", err)
	}
	result := Result{PullRequestNumber: destination.prNumber}
	if err := verifyTarget(ctx, client, target, baseBranch); err != nil {
		return result, err
	}
	files, err := client.ListChangedFiles(ctx)
	if err != nil {
		return result, fmt.Errorf("read PR diff: %w", err)
	}
	if err := verifyTarget(ctx, client, target, baseBranch); err != nil {
		return result, err
	}

	ordered := canonicalComments(comments)
	inventory := buildDiffInventory(files)
	inline, inlineSources, summary := classifyFindings(ordered, inventory)
	runID := reviewRunID(destination.repository.Owner(), destination.repository.Name(), destination.prNumber, baseBranch, target, ordered)

	if fallbackStart, found, err := existingFallbackStart(ctx, client, runID, len(inline)); err != nil {
		return result, fmt.Errorf("inspect fallback sequence: %w", err)
	} else if found {
		return resumeFallback(ctx, client, target, baseBranch, runID, ordered, inlineSources, summary, fallbackStart, result)
	}

	if len(inline) == 0 {
		if err := postSummary(ctx, client, target, baseBranch, runID, "summary", ordered, 0, summary); err != nil {
			return result, fmt.Errorf("post review summary: %w", err)
		}
		result.SummaryComments = len(summary)
		return result, nil
	}

	summaryBatches, err := buildSummaryBatches(runID, "summary", ordered, len(inline), summary)
	if err != nil {
		return result, fmt.Errorf("build review summary: %w", err)
	}
	overview := buildSummaryBody(ordered, len(inline), len(summary), nil)
	if len(summaryBatches) > 0 {
		overview = strings.TrimPrefix(summaryBatches[0].body, summaryBatches[0].marker+"\n")
	}
	for start := 0; start < len(inline); start += reviewBatchSize {
		end := min(start+reviewBatchSize, len(inline))
		batchIndex := start / reviewBatchSize
		marker := postingMarker(runID, "review", batchIndex)
		body := "OpenCodeReview inline findings (continued)."
		if start == 0 {
			body = overview
		}
		body = marker + "\n" + body

		landed, err := reviewExists(ctx, client, target.ResolvedHead, marker)
		if err != nil {
			return result, fmt.Errorf("check existing inline review batch: %w", err)
		}
		if landed {
			result.InlineComments += end - start
			continue
		}
		if err := verifyTarget(ctx, client, target, baseBranch); err != nil {
			return result, err
		}
		_, createErr := client.CreateReview(ctx, target.ResolvedHead, github.ReviewRequest{
			Body: body, Event: "COMMENT", Comments: inline[start:end],
		})
		if createErr == nil {
			result.InlineComments += end - start
			continue
		}
		if writeIsAmbiguous(createErr) {
			landed, reconcileErr := pollForReview(ctx, client, target.ResolvedHead, marker)
			if reconcileErr != nil {
				return result, errors.Join(fmt.Errorf("create inline review: %w", createErr), fmt.Errorf("reconcile ambiguous inline review write: %w", reconcileErr))
			}
			if !landed {
				return result, errors.Join(fmt.Errorf("create inline review: %w", createErr), fmt.Errorf("review batch outcome remains unknown after bounded reconciliation; refusing fallback"))
			}
			result.InlineComments += end - start
			continue
		}

		fallback := appendSummaryFindings(summary, inlineSources[start:], "inline review could not be created")
		kind := fmt.Sprintf("fallback-%d", batchIndex)
		if err := postSummary(ctx, client, target, baseBranch, runID, kind, ordered, start, fallback); err != nil {
			return result, errors.Join(fmt.Errorf("create inline review: %w", createErr), fmt.Errorf("post fallback summary: %w", err))
		}
		result.InlineComments = start
		result.SummaryComments = len(ordered) - start
		return result, nil
	}

	if err := postSummaryBatches(ctx, client, target, baseBranch, summaryBatches, 1); err != nil {
		return result, fmt.Errorf("post non-inline finding continuations: %w", err)
	}
	result.InlineComments = len(inline)
	result.SummaryComments = len(summary)
	return result, nil
}

func validateTarget(target Target) error {
	if strings.TrimSpace(target.RepoDir) == "" || strings.TrimSpace(target.BaseRef) == "" ||
		strings.TrimSpace(target.ResolvedBase) == "" || strings.TrimSpace(target.ResolvedHead) == "" {
		return fmt.Errorf("GitHub posting target requires repository, base ref, resolved base, and resolved head")
	}
	return nil
}

func classifyFindings(comments []model.LlmComment, inventory diffInventory) ([]github.Comment, []model.LlmComment, []summaryFinding) {
	inline := make([]github.Comment, 0, len(comments))
	inlineSources := make([]model.LlmComment, 0, len(comments))
	summary := make([]summaryFinding, 0)
	for _, comment := range comments {
		start, end, _ := commentLocation(comment)
		switch classifyLocation(comment, inventory) {
		case locationRightOnly:
			candidate := github.Comment{Path: comment.Path, Body: formatCommentBody(comment), Line: end, Side: "RIGHT"}
			if start != end {
				candidate.StartLine = start
				candidate.StartSide = "RIGHT"
			}
			inline = append(inline, candidate)
			inlineSources = append(inlineSources, comment)
		case locationAmbiguous:
			summary = append(summary, summaryFinding{comment: comment, reason: "line range is ambiguous across both sides of the PR diff"})
		case locationLeftOnly:
			summary = append(summary, summaryFinding{comment: comment, reason: "line range belongs to the old side of the PR diff"})
		case locationInvalid:
			summary = append(summary, summaryFinding{comment: comment, reason: "no valid line information"})
		default:
			summary = append(summary, summaryFinding{comment: comment, reason: "line range could not be verified in a complete right-side PR diff hunk"})
		}
	}
	return inline, inlineSources, summary
}

func appendSummaryFindings(existing []summaryFinding, comments []model.LlmComment, reason string) []summaryFinding {
	result := make([]summaryFinding, 0, len(existing)+len(comments))
	result = append(result, existing...)
	for _, comment := range comments {
		result = append(result, summaryFinding{comment: comment, reason: reason})
	}
	return result
}

func resumeFallback(ctx context.Context, client *github.Client, target Target, baseBranch, runID string, all, inline []model.LlmComment, summary []summaryFinding, start int, result Result) (Result, error) {
	if start < 0 || start > len(inline) {
		return result, fmt.Errorf("existing fallback sequence has invalid inline offset %d", start)
	}
	fallback := appendSummaryFindings(summary, inline[start:], "inline review could not be created")
	kind := fmt.Sprintf("fallback-%d", start/reviewBatchSize)
	if err := postSummary(ctx, client, target, baseBranch, runID, kind, all, start, fallback); err != nil {
		return result, fmt.Errorf("resume fallback summary: %w", err)
	}
	result.InlineComments = start
	result.SummaryComments = len(all) - start
	return result, nil
}

func existingFallbackStart(ctx context.Context, client *github.Client, runID string, inlineCount int) (int, bool, error) {
	comments, err := client.ListIssueComments(ctx)
	if err != nil {
		return 0, false, err
	}
	for start := 0; start < inlineCount; start += reviewBatchSize {
		prefix := fmt.Sprintf("<!-- ocr-review-%s-fallback-%d-", runID, start/reviewBatchSize)
		for _, comment := range comments {
			if strings.Contains(comment.Body, prefix) {
				return start, true, nil
			}
		}
	}
	return 0, false, nil
}

func discoverTarget(ctx context.Context, token string, repositories []repositoryCandidate, baseBranch, headSHA string) (postingTarget, error) {
	var found *postingTarget
	for _, repository := range repositories {
		prNumber, err := github.FindPRByHead(ctx, token, repository.repository, baseBranch, headSHA)
		if errors.Is(err, github.ErrNoMatchingPullRequest) {
			continue
		}
		if err != nil {
			return postingTarget{}, fmt.Errorf("discover PR in %s/%s: %w", repository.repository.Owner(), repository.repository.Name(), err)
		}
		if found != nil {
			return postingTarget{}, fmt.Errorf("multiple configured GitHub remotes have open PRs for base=%s and reviewed commit %s", baseBranch, headSHA)
		}
		candidate := postingTarget{repositoryCandidate: repository, prNumber: prNumber}
		found = &candidate
	}
	if found == nil {
		return postingTarget{}, fmt.Errorf("no open PR found for base=%s and reviewed commit %s in configured GitHub remotes", baseBranch, headSHA)
	}
	return *found, nil
}

func verifyTarget(ctx context.Context, client *github.Client, target Target, expectedBase string) error {
	pull, err := client.GetPRInfo(ctx)
	if err != nil {
		return fmt.Errorf("read PR target: %w", err)
	}
	if pull.State != "open" {
		return fmt.Errorf("PR is %s; rerun discovery before posting", pull.State)
	}
	if pull.Head.SHA != target.ResolvedHead {
		return fmt.Errorf("PR head changed from reviewed commit %s to %s; rerun the review before posting", target.ResolvedHead, pull.Head.SHA)
	}
	if pull.Base.Ref != expectedBase {
		return fmt.Errorf("PR base changed from %s to %s; rerun the review before posting", expectedBase, pull.Base.Ref)
	}
	if pull.Base.SHA == "" {
		return fmt.Errorf("PR base response omitted its commit SHA; refusing to post")
	}
	mergeBase, err := client.MergeBase(ctx, pull.Base.SHA, pull.Head.SHA)
	if err != nil {
		return fmt.Errorf("resolve PR merge-base: %w", err)
	}
	if mergeBase != target.ResolvedBase {
		return fmt.Errorf("PR merge-base changed from reviewed commit %s to %s; rerun the review before posting", target.ResolvedBase, mergeBase)
	}
	return nil
}

func repositoriesForGitDir(repoDir string, configuration github.Configuration) ([]repositoryCandidate, []string, error) {
	output, err := gitOutput(repoDir, "remote")
	if err != nil {
		return nil, nil, fmt.Errorf("list git remotes: %w", err)
	}
	remotes := strings.Fields(output)
	if len(remotes) == 0 {
		return nil, nil, fmt.Errorf("repository has no Git remotes")
	}
	seen := make(map[string]struct{})
	repositories := make([]repositoryCandidate, 0, len(remotes))
	for _, remote := range remotes {
		remoteURL, err := gitOutput(repoDir, "remote", "get-url", remote)
		if err != nil {
			return nil, nil, fmt.Errorf("read URL for git remote %q: %w", remote, err)
		}
		repository, err := configuration.ResolveRepository(remoteURL)
		if err != nil {
			continue
		}
		key := strings.ToLower(repository.Owner() + "/" + repository.Name())
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		repositories = append(repositories, repositoryCandidate{remote: remote, repository: repository})
	}
	if len(repositories) == 0 {
		return nil, nil, fmt.Errorf("no parseable GitHub repository found in configured remotes")
	}
	return repositories, remotes, nil
}

func resolveBranch(repoDir, ref string, remotes []string) (string, error) {
	fullName, err := gitOutput(repoDir, "rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", ref)
	if err != nil {
		return "", err
	}
	if local, ok := strings.CutPrefix(fullName, "refs/heads/"); ok {
		return local, nil
	}
	remoteRef, ok := strings.CutPrefix(fullName, "refs/remotes/")
	if !ok {
		return "", fmt.Errorf("ref resolved to %q instead of a local or remote-tracking branch", fullName)
	}
	for _, remote := range remotes {
		if branch, ok := strings.CutPrefix(remoteRef, remote+"/"); ok {
			return branch, nil
		}
	}
	return remoteRef, nil
}

func gitOutput(repoDir string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func reviewExists(ctx context.Context, client *github.Client, commitSHA, marker string) (bool, error) {
	reviews, err := client.ListReviews(ctx)
	if err != nil {
		return false, err
	}
	for _, review := range reviews {
		if review.CommitID == commitSHA && strings.Contains(review.Body, marker) {
			return true, nil
		}
	}
	return false, nil
}

func issueCommentExists(ctx context.Context, client *github.Client, marker string) (string, bool, error) {
	comments, err := client.ListIssueComments(ctx)
	if err != nil {
		return "", false, err
	}
	for _, comment := range comments {
		if strings.Contains(comment.Body, marker) {
			return comment.HTMLURL, true, nil
		}
	}
	return "", false, nil
}

func writeIsAmbiguous(err error) bool {
	var apiErr *github.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusRequestTimeout || apiErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

func pollForReview(ctx context.Context, client *github.Client, commitSHA, marker string) (bool, error) {
	for attempt := 0; attempt < reconciliationAttempts; attempt++ {
		landed, err := reviewExists(ctx, client, commitSHA, marker)
		if err != nil || landed {
			return landed, err
		}
		if attempt+1 < reconciliationAttempts {
			if err := waitForReconciliation(ctx); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func pollForIssueComment(ctx context.Context, client *github.Client, marker string) (string, bool, error) {
	for attempt := 0; attempt < reconciliationAttempts; attempt++ {
		url, landed, err := issueCommentExists(ctx, client, marker)
		if err != nil || landed {
			return url, landed, err
		}
		if attempt+1 < reconciliationAttempts {
			if err := waitForReconciliation(ctx); err != nil {
				return "", false, err
			}
		}
	}
	return "", false, nil
}

func waitForReconciliation(ctx context.Context) error {
	timer := time.NewTimer(reconciliationPollPeriod)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func postSummary(ctx context.Context, client *github.Client, target Target, baseBranch, runID, kind string, all []model.LlmComment, inlineCount int, findings []summaryFinding) error {
	batches, err := buildSummaryBatches(runID, kind, all, inlineCount, findings)
	if err != nil {
		return err
	}
	return postSummaryBatches(ctx, client, target, baseBranch, batches, 0)
}

func postSummaryBatches(ctx context.Context, client *github.Client, target Target, baseBranch string, batches []summaryBatch, start int) error {
	for i := start; i < len(batches); i++ {
		batch := batches[i]
		_, landed, err := issueCommentExists(ctx, client, batch.marker)
		if err != nil {
			return fmt.Errorf("check existing summary batch %d: %w", i+1, err)
		}
		if landed {
			continue
		}
		if err := verifyTarget(ctx, client, target, baseBranch); err != nil {
			return err
		}
		_, createErr := client.CreateIssueComment(ctx, batch.body)
		if createErr == nil {
			continue
		}
		if !writeIsAmbiguous(createErr) {
			return fmt.Errorf("create summary batch %d: %w", i+1, createErr)
		}
		_, landed, reconcileErr := pollForIssueComment(ctx, client, batch.marker)
		if reconcileErr != nil {
			return errors.Join(fmt.Errorf("create summary batch %d: %w", i+1, createErr), fmt.Errorf("reconcile ambiguous summary write: %w", reconcileErr))
		}
		if !landed {
			return errors.Join(fmt.Errorf("create summary batch %d: %w", i+1, createErr), fmt.Errorf("summary batch outcome remains unknown after bounded reconciliation; refusing another write"))
		}
	}
	return nil
}

func buildSummaryBatches(runID, kind string, all []model.LlmComment, inlineCount int, findings []summaryFinding) ([]summaryBatch, error) {
	fragments := make([]string, 0, len(findings))
	for _, finding := range findings {
		fragments = append(fragments, splitSummaryText(formatSummaryFinding(finding))...)
	}
	if len(fragments) == 0 {
		return nil, nil
	}
	batches := make([]summaryBatch, 0, (len(fragments)+summaryBatchSize-1)/summaryBatchSize)
	current := make([]string, 0, summaryBatchSize)
	flush := func() {
		marker := postingMarker(runID, kind, len(batches))
		body := marker + "\n" + buildSummaryBody(all, inlineCount, len(findings), current)
		batches = append(batches, summaryBatch{marker: marker, body: body})
		current = current[:0]
	}
	for _, fragment := range fragments {
		candidate := append(append([]string(nil), current...), fragment)
		marker := postingMarker(runID, kind, len(batches))
		payloadSize, err := issueCommentPayloadSize(marker + "\n" + buildSummaryBody(all, inlineCount, len(findings), candidate))
		if err != nil {
			return nil, err
		}
		if len(current) > 0 && (len(candidate) > summaryBatchSize || payloadSize > summaryMaxPayloadBytes) {
			flush()
			candidate = []string{fragment}
			marker = postingMarker(runID, kind, len(batches))
			payloadSize, err = issueCommentPayloadSize(marker + "\n" + buildSummaryBody(all, inlineCount, len(findings), candidate))
			if err != nil {
				return nil, err
			}
		}
		if payloadSize > summaryMaxPayloadBytes {
			return nil, fmt.Errorf("summary fragment requires %d encoded bytes, exceeding the %d-byte batch limit", payloadSize, summaryMaxPayloadBytes)
		}
		current = candidate
	}
	if len(current) > 0 {
		flush()
	}
	return batches, nil
}

func issueCommentPayloadSize(body string) (int, error) {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return 0, fmt.Errorf("measure GitHub summary payload: %w", err)
	}
	return len(payload), nil
}

func formatSummaryFinding(finding summaryFinding) string {
	location := finding.comment.Path
	if start, end, ok := commentLocation(finding.comment); ok {
		if start == end {
			location = fmt.Sprintf("%s (line %d)", location, end)
		} else {
			location = fmt.Sprintf("%s (lines %d-%d)", location, start, end)
		}
	}
	return fmt.Sprintf("#### `%s`\n\n_Summary reason: %s._\n\n%s", location, finding.reason, formatCommentBody(finding.comment))
}

func buildSummaryBody(all []model.LlmComment, inlineCount, summaryCount int, fragments []string) string {
	var body strings.Builder
	body.WriteString("## OpenCodeReview Summary\n\n")
	body.WriteString(fmt.Sprintf("%d comment(s) generated.\n\n", len(all)))
	body.WriteString(fmt.Sprintf("- Inline findings: %d\n", inlineCount))
	body.WriteString(fmt.Sprintf("- Summary-only findings: %d\n\n", summaryCount))
	if len(fragments) > 0 {
		body.WriteString("### Findings without a verified inline location\n\n")
		for _, fragment := range fragments {
			body.WriteString(fragment)
			body.WriteString("\n\n---\n\n")
		}
	}
	severityCount := make(map[string]int)
	for _, comment := range all {
		if comment.Severity != "" {
			severityCount[comment.Severity]++
		}
	}
	if len(severityCount) > 0 {
		body.WriteString("### Severity breakdown:\n\n")
		severities := make([]string, 0, len(severityCount))
		for severity := range severityCount {
			severities = append(severities, severity)
		}
		sort.Strings(severities)
		for _, severity := range severities {
			body.WriteString(fmt.Sprintf("- **%s**: %d\n", severity, severityCount[severity]))
		}
		body.WriteByte('\n')
	}
	body.WriteString("---\n*Generated by OpenCodeReview*")
	return body.String()
}
