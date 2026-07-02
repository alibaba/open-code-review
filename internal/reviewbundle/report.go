package reviewbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ReportOptions controls deterministic report rendering.
type ReportOptions struct {
	Format     string
	Validation *ValidationResult
}

// RenderReport formats external findings without changing their meaning.
func RenderReport(bundle *Bundle, comments *Comments, options ReportOptions) ([]byte, error) {
	if bundle == nil || comments == nil {
		return nil, fmt.Errorf("bundle and comments are required")
	}
	if comments.BundleID != bundle.BundleID {
		return nil, fmt.Errorf("bundle_id mismatch")
	}
	if options.Validation != nil && options.Validation.BundleID != bundle.BundleID {
		return nil, fmt.Errorf("validation bundle_id mismatch")
	}
	sorted := sortedComments(comments)
	switch options.Format {
	case "json":
		encoded, err := json.MarshalIndent(sorted, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode JSON report: %w", err)
		}
		return append(encoded, '\n'), nil
	case "text":
		return renderTextReport(bundle, sorted, options.Validation), nil
	case "", "markdown":
		return renderMarkdownReport(bundle, sorted, options.Validation), nil
	default:
		return nil, fmt.Errorf("unsupported report format %q", options.Format)
	}
}

func sortedComments(comments *Comments) *Comments {
	result := *comments
	result.Comments = make([]ReviewComment, len(comments.Comments))
	copy(result.Comments, comments.Comments)
	result.Warnings = make([]ProtocolNotice, len(comments.Warnings))
	copy(result.Warnings, comments.Warnings)
	sort.SliceStable(result.Comments, func(i, j int) bool {
		left, right := result.Comments[i], result.Comments[j]
		if priorityRank(left.Priority) != priorityRank(right.Priority) {
			return priorityRank(left.Priority) < priorityRank(right.Priority)
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.StartLine != right.StartLine {
			return left.StartLine < right.StartLine
		}
		return left.Title < right.Title
	})
	return &result
}

func priorityRank(priority string) int {
	switch priority {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

func renderMarkdownReport(
	bundle *Bundle,
	comments *Comments,
	validation *ValidationResult,
) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "# Agent Code Review")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Bundle: %s\n", markdownCode(bundle.BundleID))
	fmt.Fprintf(
		&output,
		"- Scope: %d reviewable / %d total files\n",
		bundle.Summary.ReviewableFiles,
		bundle.Summary.TotalFiles,
	)
	fmt.Fprintf(&output, "- Findings: %d\n", len(comments.Comments))
	writeMarkdownValidation(&output, validation)
	if len(comments.Comments) == 0 {
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "No findings.")
	} else {
		for _, comment := range comments.Comments {
			fmt.Fprintln(&output)
			fmt.Fprintf(
				&output,
				"## [%s] %s\n\n",
				strings.ToUpper(comment.Priority),
				escapeMarkdownHeading(comment.Title),
			)
			fmt.Fprintf(
				&output,
				"%s · %s · confidence %.2f\n\n",
				markdownCode(commentLocation(comment)),
				comment.Category,
				comment.Confidence,
			)
			fmt.Fprintln(&output, comment.Content)
			if comment.Recommendation != "" {
				fmt.Fprintln(&output)
				fmt.Fprintf(&output, "Recommendation: %s\n", comment.Recommendation)
			}
			if comment.ExistingCode != "" {
				fmt.Fprintln(&output)
				fmt.Fprintln(&output, "Existing code:")
				fmt.Fprintln(&output)
				writeFencedCode(&output, comment.ExistingCode)
			}
			if comment.SuggestionCode != "" {
				fmt.Fprintln(&output)
				fmt.Fprintln(&output, "Suggested code:")
				fmt.Fprintln(&output)
				writeFencedCode(&output, comment.SuggestionCode)
			}
		}
	}
	writeMarkdownNotices(&output, "Warnings", comments.Warnings)
	return output.Bytes()
}

func writeMarkdownValidation(output *bytes.Buffer, validation *ValidationResult) {
	if validation == nil {
		fmt.Fprintln(output, "- Validation: not supplied")
		return
	}
	state := "INVALID"
	if validation.Valid {
		state = "valid"
	}
	fmt.Fprintf(output, "- Validation: %s\n", state)
	if len(validation.Errors) > 0 {
		fmt.Fprintln(output)
		fmt.Fprintln(output, "## Validation errors")
		for _, notice := range validation.Errors {
			fmt.Fprintf(output, "\n- `%s`: %s\n", notice.Code, notice.Message)
		}
	}
	if len(validation.Warnings) > 0 {
		fmt.Fprintln(output)
		fmt.Fprintln(output, "## Validation warnings")
		for _, notice := range validation.Warnings {
			fmt.Fprintf(output, "\n- `%s`: %s\n", notice.Code, notice.Message)
		}
	}
}

func writeMarkdownNotices(output *bytes.Buffer, title string, notices []ProtocolNotice) {
	if len(notices) == 0 {
		return
	}
	fmt.Fprintln(output)
	fmt.Fprintf(output, "## %s\n", title)
	for _, notice := range notices {
		fmt.Fprintf(output, "\n- `%s`: %s\n", notice.Code, notice.Message)
	}
}

func renderTextReport(
	bundle *Bundle,
	comments *Comments,
	validation *ValidationResult,
) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "Agent Code Review\nBundle: %s\nFindings: %d\n", bundle.BundleID, len(comments.Comments))
	if validation == nil {
		fmt.Fprintln(&output, "Validation: not supplied")
	} else if validation.Valid {
		fmt.Fprintln(&output, "Validation: valid")
	} else {
		fmt.Fprintln(&output, "Validation: INVALID")
		for _, notice := range validation.Errors {
			fmt.Fprintf(&output, "ERROR %s: %s\n", notice.Code, notice.Message)
		}
	}
	if validation != nil && len(validation.Warnings) > 0 {
		for _, notice := range validation.Warnings {
			fmt.Fprintf(&output, "WARNING %s: %s\n", notice.Code, notice.Message)
		}
	}
	for _, comment := range comments.Comments {
		fmt.Fprintf(
			&output,
			"\n[%s] %s (%s, %s, confidence %.2f)\n%s\n",
			strings.ToUpper(comment.Priority),
			comment.Title,
			commentLocation(comment),
			comment.Category,
			comment.Confidence,
			comment.Content,
		)
		if comment.Recommendation != "" {
			fmt.Fprintf(&output, "Recommendation: %s\n", comment.Recommendation)
		}
	}
	if len(comments.Warnings) > 0 {
		fmt.Fprintln(&output, "\nWarnings:")
		for _, notice := range comments.Warnings {
			fmt.Fprintf(&output, "WARNING %s: %s\n", notice.Code, notice.Message)
		}
	}
	return output.Bytes()
}

func commentLocation(comment ReviewComment) string {
	if comment.FileLevelComment {
		return comment.Path
	}
	if comment.StartLine == comment.EndLine {
		return fmt.Sprintf("%s:%d", comment.Path, comment.StartLine)
	}
	return fmt.Sprintf("%s:%d-%d", comment.Path, comment.StartLine, comment.EndLine)
}

func markdownCode(value string) string {
	if !strings.Contains(value, "`") {
		return "`" + value + "`"
	}
	fence := "``"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	return fence + " " + value + " " + fence
}

func escapeMarkdownHeading(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "#", "\\#")
}

func writeFencedCode(output *bytes.Buffer, code string) {
	fence := "```"
	for strings.Contains(code, fence) {
		fence += "`"
	}
	fmt.Fprintln(output, fence)
	fmt.Fprintln(output, code)
	fmt.Fprintln(output, fence)
}
