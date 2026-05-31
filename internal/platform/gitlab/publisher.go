package gitlab

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/platform"
)

var hunkHeaderRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// Publisher implements platform.PublishResult-oriented operations on a GitLab MR.
type Publisher struct {
	client *Client
	opts   platform.PublishOptions
}

// NewPublisher creates a GitLab MR publisher.
func NewPublisher(client *Client, opts platform.PublishOptions) *Publisher {
	return &Publisher{client: client, opts: opts}
}

// ClearInline deletes all OCR inline marker notes from the MR.
func (p *Publisher) ClearInline() (*platform.PublishResult, error) {
	return p.clearByMarkers(platform.InlineMarker)
}

// ClearSummary deletes all OCR summary and request-changes marker notes from the MR.
func (p *Publisher) ClearSummary() (*platform.PublishResult, error) {
	return p.clearByMarkers(platform.SummaryMarker, platform.RequestChangesMarker)
}

func (p *Publisher) clearByMarkers(markers ...string) (*platform.PublishResult, error) {
	ctx := context.Background()
	discs, err := p.client.ListDiscussions(ctx, p.opts.ProjectID, p.opts.MergeRequestIID)
	if err != nil {
		return nil, fmt.Errorf("list discussions: %w", err)
	}

	result := &platform.PublishResult{}
	var deleteErrors []string

	for _, disc := range discs {
		for _, note := range disc.Notes {
			if note.System {
				continue
			}
			if !containsAnyMarker(note.Body, markers) {
				continue
			}
			if err := p.client.DeleteNote(ctx, p.opts.ProjectID, p.opts.MergeRequestIID, note.ID); err != nil {
				deleteErrors = append(deleteErrors, fmt.Sprintf("note %d: %v", note.ID, err))
				continue
			}
			if containsMarker(note.Body, platform.InlineMarker) {
				result.InlineDeleted++
			}
			if containsMarker(note.Body, platform.SummaryMarker) || containsMarker(note.Body, platform.RequestChangesMarker) {
				result.SummaryDeleted++
			}
		}
	}

	if len(deleteErrors) > 0 {
		return result, fmt.Errorf("failed to delete %d note(s): %s", len(deleteErrors), strings.Join(deleteErrors, "; "))
	}
	return result, nil
}

func containsMarker(body, marker string) bool {
	return strings.Contains(body, marker)
}

func containsAnyMarker(body string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

// diffAddedLines parses a unified diff and returns the set of line numbers
// that were added (lines starting with "+"). This is used to validate that
// an inline comment targets a line that actually exists in the diff.
func diffAddedLines(diff string) map[int]bool {
	added := make(map[int]bool)
	currentLine := 0
	for _, line := range strings.Split(diff, "\n") {
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			currentLine = n
			continue
		}
		if currentLine > 0 {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				added[currentLine] = true
				currentLine++
			} else if !strings.HasPrefix(line, "-") {
				currentLine++
			}
		}
	}
	return added
}

// isAddedLine checks if a line number is an added line in the given diff.
// Returns true if the line is added, false otherwise.
// If the diff is empty or cannot be parsed, returns true (permissive fallback).
func isAddedLine(diff string, line int) bool {
	if line <= 0 {
		return false
	}
	if diff == "" {
		return true // No diff info available, be permissive.
	}
	added := diffAddedLines(diff)
	if len(added) == 0 {
		return true // Could not parse any added lines, be permissive.
	}
	return added[line]
}

// selectAddedLineInRange finds the first added line in the diff within [startLine, endLine].
// Returns the line number and true if found, or 0 and false if no added line exists in the range.
// If startLine <= 0, returns false.
// If endLine <= 0 or endLine < startLine, treats as a single-line range (endLine = startLine).
// If the diff is empty or cannot be parsed (no hunk headers), falls back to startLine (permissive).
// If the diff has hunk headers but no added lines in the range, returns false.
func selectAddedLineInRange(diff string, startLine, endLine int) (int, bool) {
	if startLine <= 0 {
		return 0, false
	}
	if endLine <= 0 || endLine < startLine {
		endLine = startLine
	}
	if diff == "" {
		return startLine, true // No diff info available, be permissive.
	}
	added := diffAddedLines(diff)
	if len(added) == 0 {
		// Distinguish "valid diff with no additions" from "unparseable diff".
		// If the diff has hunk headers, it was parsed successfully but has no added lines.
		if hunkHeaderRe.MatchString(diff) {
			return 0, false
		}
		return startLine, true // Could not parse any hunk headers, be permissive.
	}
	for n := startLine; n <= endLine; n++ {
		if added[n] {
			return n, true
		}
	}
	return 0, false
}

// Publish creates inline discussions and a summary note for the given review comments.
func (p *Publisher) Publish(comments []model.LlmComment) (*platform.PublishResult, error) {
	ctx := context.Background()
	result := &platform.PublishResult{}

	// Step 1: Clear existing if requested.
	if p.opts.ClearExisting {
		clearResult, err := p.ClearInline()
		if err != nil {
			return result, fmt.Errorf("clear inline: %w", err)
		}
		result.InlineDeleted = clearResult.InlineDeleted
		result.Warnings = append(result.Warnings, clearResult.Warnings...)

		clearResult, err = p.ClearSummary()
		if err != nil {
			return result, fmt.Errorf("clear summary: %w", err)
		}
		result.SummaryDeleted = clearResult.SummaryDeleted
		result.Warnings = append(result.Warnings, clearResult.Warnings...)
	}

	// Step 2: Load diff version for inline positions.
	var diffVersion *DiffVersion
	if !p.opts.NoInline && len(comments) > 0 {
		versions, err := p.client.ListDiffVersions(ctx, p.opts.ProjectID, p.opts.MergeRequestIID)
		if err != nil {
			return result, fmt.Errorf("list diff versions: %w", err)
		}
		if len(versions) > 0 {
			diffVersion = &versions[0]
		}
	}

	// Step 3: Load changed files for position validation.
	fileDiffs := make(map[string]string)
	if !p.opts.NoInline && diffVersion != nil && len(comments) > 0 {
		changedFiles, err := p.client.ListChangedFiles(ctx, p.opts.ProjectID, p.opts.MergeRequestIID)
		if err == nil {
			for _, f := range changedFiles {
				fileDiffs[f.NewPath] = f.Diff
			}
		}
		// If ListChangedFiles fails, we still proceed (permissive fallback).
	}

	// Step 4: Create inline discussions.
	if !p.opts.NoInline && diffVersion == nil && len(comments) > 0 {
		// No diff version available - count all comments as failed.
		for _, comment := range comments {
			result.InlineFailed++
			result.Warnings = append(result.Warnings, platform.PublishWarning{
				Type:    "inline_failed",
				Path:    comment.Path,
				Message: fmt.Sprintf("line %d: no GitLab diff version available", comment.StartLine),
			})
		}
	}
	if !p.opts.NoInline && diffVersion != nil {
		for _, comment := range comments {
			// Validate: StartLine must be positive.
			if comment.StartLine <= 0 {
				result.InlineSkipped++
				result.Warnings = append(result.Warnings, platform.PublishWarning{
					Type:    "inline_skipped",
					Path:    comment.Path,
					Message: fmt.Sprintf("line %d: start line must be positive", comment.StartLine),
				})
				continue
			}

			// Validate: range must contain at least one added line in the diff.
			diff := fileDiffs[comment.Path]
			selectedLine, ok := selectAddedLineInRange(diff, comment.StartLine, comment.EndLine)
			if !ok {
				result.InlineSkipped++
				result.Warnings = append(result.Warnings, platform.PublishWarning{
					Type:    "inline_skipped",
					Path:    comment.Path,
					Message: fmt.Sprintf("line %d-%d: no added line in range", comment.StartLine, comment.EndLine),
				})
				continue
			}

			pos := Position{
				PositionType: "text",
				BaseSHA:      diffVersion.BaseCommitSHA,
				StartSHA:     diffVersion.StartCommitSHA,
				HeadSHA:      diffVersion.HeadCommitSHA,
				NewPath:      comment.Path,
				NewLine:      selectedLine,
			}
			_, err := p.client.CreateInlineDiscussion(ctx, p.opts.ProjectID, p.opts.MergeRequestIID, platform.RenderInlineComment(comment), pos)
			if err != nil {
				result.InlineFailed++
				result.Warnings = append(result.Warnings, platform.PublishWarning{
					Type:    "inline_failed",
					Path:    comment.Path,
					Message: fmt.Sprintf("line %d: %v", comment.StartLine, err),
				})
				continue
			}
			result.InlineCreated++
		}
	}

	// Step 4: Create or update summary note.
	if !p.opts.NoSummaryComment {
		summaryBody := platform.RenderSummaryComment(comments, result.Warnings)

		existingNote, err := p.findExistingSummaryNote(ctx)
		if err != nil {
			return result, fmt.Errorf("find summary note: %w", err)
		}

		if existingNote != nil {
			if err := p.client.UpdateNote(ctx, p.opts.ProjectID, p.opts.MergeRequestIID, existingNote.ID, summaryBody); err != nil {
				return result, fmt.Errorf("update summary note: %w", err)
			}
			result.SummaryUpdated = true
		} else {
			if _, err := p.client.CreateNote(ctx, p.opts.ProjectID, p.opts.MergeRequestIID, summaryBody); err != nil {
				return result, fmt.Errorf("create summary note: %w", err)
			}
			result.SummaryCreated = true
		}
	}

	// Step 5: Update MR description with managed summary block.
	if p.opts.PRSummary {
		mr, err := p.client.GetMergeRequest(ctx, p.opts.ProjectID, p.opts.MergeRequestIID)
		if err != nil {
			return result, fmt.Errorf("get merge request: %w", err)
		}
		summaryBody := platform.RenderSummaryComment(comments, result.Warnings)
		newDesc := platform.AppendOrReplaceManagedBlock(mr.Description, summaryBody)
		if err := p.client.UpdateDescription(ctx, p.opts.ProjectID, p.opts.MergeRequestIID, newDesc); err != nil {
			return result, fmt.Errorf("update description: %w", err)
		}
		result.DescriptionUpdated = true
	}

	return result, nil
}

// RequestChanges unapproves the MR (best-effort) and posts a request-changes note.
func (p *Publisher) RequestChanges(comments []model.LlmComment) (*platform.PublishResult, error) {
	ctx := context.Background()
	result := &platform.PublishResult{}

	// Best-effort unapprove.
	if err := p.client.Unapprove(ctx, p.opts.ProjectID, p.opts.MergeRequestIID); err != nil {
		msg := fmt.Sprintf("unapprove: %v", err)
		result.Warnings = append(result.Warnings, platform.PublishWarning{
			Type:    "unapprove_failed",
			Message: msg,
		})
	}

	// Always post the request-changes note.
	body := platform.RenderRequestChangesComment(comments)
	if _, err := p.client.CreateNote(ctx, p.opts.ProjectID, p.opts.MergeRequestIID, body); err != nil {
		return result, fmt.Errorf("request-changes note: %w", err)
	}

	return result, nil
}

func (p *Publisher) findExistingSummaryNote(ctx context.Context) (*Note, error) {
	discs, err := p.client.ListDiscussions(ctx, p.opts.ProjectID, p.opts.MergeRequestIID)
	if err != nil {
		return nil, err
	}
	for _, disc := range discs {
		for _, note := range disc.Notes {
			if note.System {
				continue
			}
			if strings.Contains(note.Body, platform.SummaryMarker) {
				return &note, nil
			}
		}
	}
	return nil, nil
}
