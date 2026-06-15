package gitflic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/model"
)

// ReviewResult mirrors the JSON emitted by `ocr review --format json`.
type ReviewResult struct {
	Status   string             `json:"status"`
	Message  string             `json:"message"`
	Comments []model.LlmComment `json:"comments"`
	Warnings []ReviewWarning    `json:"warnings"`
}

// ReviewWarning mirrors agent.AgentWarning in the review JSON output.
type ReviewWarning struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// LoadReviewResult reads a review JSON produced by `ocr review --format json`.
// Path "-" reads from stdin.
func LoadReviewResult(path string) (*ReviewResult, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read review result: %w", err)
	}
	var result ReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse review result: %w", err)
	}
	return &result, nil
}

// Options configures Publish.
type Options struct {
	Owner   string // project owner alias
	Project string // project alias
	MRID    string // merge request local id
	DryRun  bool   // print discussions instead of posting them
}

// Stats summarizes a Publish run.
type Stats struct {
	Inline   int // comments posted as inline discussions
	Fallback int // comments folded into the fallback note
}

// Publisher posts review comments onto a GitFlic merge request.
type Publisher struct {
	client *Client
	logf   func(format string, args ...any)
}

// NewPublisher wraps a Client. Progress and errors are logged to stderr.
func NewPublisher(client *Client) *Publisher {
	return &Publisher{
		client: client,
		logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		},
	}
}

// Publish posts the review result onto the merge request: an inline
// discussion per comment that maps onto the diff, a single fallback note
// collecting the comments that do not, and a final summary note.
// diffs must cover the same range the review was run on, so that comment
// line numbers (new file side) align with the hunks.
func (p *Publisher) Publish(ctx context.Context, opts Options, result *ReviewResult, diffs []model.Diff) (Stats, error) {
	var stats Stats

	if len(result.Comments) == 0 {
		message := result.Message
		if message == "" {
			message = "No comments generated. Looks good to me."
		}
		return stats, p.post(ctx, opts, Discussion{Message: "✅ **OpenCodeReview**: " + message})
	}

	byPath := make(map[string]model.Diff, len(diffs))
	for _, d := range diffs {
		byPath[d.NewPath] = d
	}

	// Parse each file's diff hunks at most once, even with several comments.
	hunksByPath := make(map[string][]diff.Hunk)

	var failed []model.LlmComment
	for _, c := range result.Comments {
		d, ok := byPath[c.Path]
		if !ok {
			p.logf("no diff for %s; folding comment into the summary note", c.Path)
			failed = append(failed, c)
			continue
		}
		if d.IsBinary || d.IsDeleted || c.EndLine <= 0 {
			failed = append(failed, c)
			continue
		}

		oldPath := d.OldPath
		oldLine := 1
		if d.IsNew || oldPath == "" || oldPath == "/dev/null" {
			// GitFlic has no old side for a new file; anchor to the new path.
			oldPath = d.NewPath
		} else {
			hunks, cached := hunksByPath[c.Path]
			if !cached {
				hunks = diff.ParseHunks(d.Diff)
				hunksByPath[c.Path] = hunks
			}
			oldLine = oldLineFor(hunks, c.EndLine)
		}

		disc := Discussion{
			Message: formatComment(c),
			NewLine: intPtr(c.EndLine),
			OldLine: intPtr(oldLine),
			NewPath: c.Path,
			OldPath: oldPath,
		}
		if err := p.post(ctx, opts, disc); err != nil {
			p.logf("inline comment failed for %s:%d: %v", c.Path, c.EndLine, err)
			failed = append(failed, c)
			continue
		}
		stats.Inline++
	}
	stats.Fallback = len(failed)

	if len(failed) > 0 {
		var sb strings.Builder
		sb.WriteString("🔍 **OpenCodeReview** found issues that could not be posted inline:\n\n---\n\n")
		for _, c := range failed {
			sb.WriteString(formatCommentFallback(c))
			sb.WriteString("\n\n---\n\n")
		}
		if err := p.post(ctx, opts, Discussion{Message: sb.String()}); err != nil {
			return stats, fmt.Errorf("post fallback note: %w", err)
		}
	}

	summary := fmt.Sprintf("🔍 **OpenCodeReview** found **%d** issue(s) in this MR.", len(result.Comments))
	summary += fmt.Sprintf("\n- ✅ %d posted as inline comment(s)", stats.Inline)
	summary += fmt.Sprintf("\n- 📝 %d posted as summary (could not be placed inline)", stats.Fallback)
	if len(result.Warnings) > 0 {
		summary += fmt.Sprintf("\n\n⚠️ %d warning(s) occurred during review.", len(result.Warnings))
	}
	if err := p.post(ctx, opts, Discussion{Message: summary}); err != nil {
		return stats, fmt.Errorf("post summary note: %w", err)
	}
	return stats, nil
}

func (p *Publisher) post(ctx context.Context, opts Options, d Discussion) error {
	if opts.DryRun {
		position := "general"
		if d.NewPath != "" && d.NewLine != nil && d.OldLine != nil {
			position = fmt.Sprintf("%s:%d (old %s:%d)", d.NewPath, *d.NewLine, d.OldPath, *d.OldLine)
		}
		fmt.Printf("--- dry-run discussion [%s] ---\n%s\n\n", position, d.Message)
		return nil
	}
	return p.client.CreateDiscussion(ctx, opts.Owner, opts.Project, opts.MRID, d)
}

// intPtr returns a pointer to i, for Discussion's nullable line fields.
func intPtr(i int) *int { return &i }

// formatComment renders an inline discussion body.
func formatComment(c model.LlmComment) string {
	body := c.Content
	if c.SuggestionCode != "" && c.ExistingCode != "" {
		body += "\n\n**Suggestion:**\n```\n" + c.SuggestionCode + "\n```"
	}
	return body
}

// formatCommentFallback renders a comment for the fallback (non-inline) note.
func formatCommentFallback(c model.LlmComment) string {
	md := fmt.Sprintf("### 📄 `%s`", c.Path)
	if c.StartLine > 0 && c.EndLine > 0 {
		md += fmt.Sprintf(" (L%d-L%d)", c.StartLine, c.EndLine)
	}
	md += "\n\n" + c.Content
	if c.SuggestionCode != "" && c.ExistingCode != "" {
		md += "\n\n**Before:**\n```\n" + c.ExistingCode + "\n```\n\n**After:**\n```\n" + c.SuggestionCode + "\n```"
	}
	return md
}
