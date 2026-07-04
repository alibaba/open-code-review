package reviewbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

// ValidationNotice is a stable machine-readable validation diagnostic.
type ValidationNotice struct {
	Code         string `json:"code"`
	Path         string `json:"path,omitempty"`
	CommentIndex *int   `json:"comment_index,omitempty"`
	Message      string `json:"message"`
}

// ValidationResult reports whether comments are safe to publish.
type ValidationResult struct {
	SchemaVersion  string             `json:"schema_version"`
	BundleID       string             `json:"bundle_id"`
	CommentsSHA256 string             `json:"comments_sha256"`
	Valid          bool               `json:"valid"`
	Errors         []ValidationNotice `json:"errors"`
	Warnings       []ValidationNotice `json:"warnings"`
}

// ValidateComments checks comments against the bundle and current target state.
// It is read-only and never rewrites or relocates a supplied comment.
func ValidateComments(
	ctx context.Context,
	bundle *Bundle,
	comments *Comments,
	repoDir string,
	runner *gitcmd.Runner,
) ValidationResult {
	result := ValidationResult{
		SchemaVersion: ValidationSchemaVersion,
		Errors:        make([]ValidationNotice, 0),
		Warnings:      make([]ValidationNotice, 0),
	}
	if bundle == nil || comments == nil {
		addValidationError(&result, "invalid_schema", "", nil, "bundle and comments are required")
		return result
	}
	result.BundleID = bundle.BundleID
	result.CommentsSHA256 = computeCommentsSHA256(comments)
	if bundle.SchemaVersion != BundleSchemaVersion ||
		comments.SchemaVersion != CommentsSchemaVersion {
		addValidationError(&result, "invalid_schema", "", nil, "unsupported protocol schema version")
	}
	if comments.BundleID != bundle.BundleID {
		addValidationError(
			&result,
			"bundle_id_mismatch",
			"",
			nil,
			"comments bundle_id does not match the review bundle",
		)
	}
	if repoDir != "" {
		validateFreshTarget(ctx, &result, bundle, repoDir, runner)
	}

	files := make(map[string]File, len(bundle.Files))
	for _, file := range bundle.Files {
		files[file.Path] = file
	}
	contentCache := make(map[string][]byte)
	for index := range comments.Comments {
		if err := ctx.Err(); err != nil {
			addValidationError(&result, "validation_canceled", "", nil, err.Error())
			break
		}
		validateOneComment(ctx, &result, bundle, files, contentCache, comments.Comments[index], index, repoDir, runner)
	}
	if comments.Summary.IssuesFound != len(comments.Comments) {
		addValidationError(
			&result,
			"invalid_summary",
			"",
			nil,
			"summary.issues_found must equal the number of comments",
		)
	}
	commentedPaths := make(map[string]struct{})
	for _, comment := range comments.Comments {
		commentedPaths[comment.Path] = struct{}{}
	}
	if comments.Summary.FilesReviewed > bundle.Summary.ReviewableFiles {
		addValidationError(
			&result,
			"invalid_summary",
			"",
			nil,
			"summary.files_reviewed exceeds reviewable files in the bundle",
		)
	}
	if comments.Summary.FilesReviewed < len(commentedPaths) {
		addValidationError(
			&result,
			"invalid_summary",
			"",
			nil,
			"summary.files_reviewed is less than the number of distinct commented paths",
		)
	}
	result.Valid = len(result.Errors) == 0
	return result
}

// ValidateScanManifestFreshness extends a selected scan-bundle validation with
// the sibling files that share the same manifest-level scan target.
func ValidateScanManifestFreshness(
	result *ValidationResult,
	manifest *ScanManifest,
	selectedBundleID string,
	repoDir string,
) {
	if result == nil || manifest == nil || repoDir == "" {
		return
	}
	for index := range manifest.Bundles {
		bundle := &manifest.Bundles[index]
		if bundle.BundleID == selectedBundleID {
			continue
		}
		validateFreshScanFiles(result, bundle.Files, repoDir)
	}
	result.Valid = len(result.Errors) == 0
}

func validateFreshTarget(
	ctx context.Context,
	result *ValidationResult,
	bundle *Bundle,
	repoDir string,
	runner *gitcmd.Runner,
) {
	if bundle.Target.Mode == TargetScan {
		validateFreshScanFiles(result, bundle.Files, repoDir)
		return
	}
	if runner == nil {
		addValidationError(result, "stale_bundle", "", nil, "git runner is required to verify target state")
		return
	}
	spec := TargetSpec{
		From:   bundle.Target.From,
		To:     bundle.Target.To,
		Commit: bundle.Target.Commit,
	}
	current, state, err := ResolveTarget(ctx, repoDir, spec, runner)
	if err != nil {
		addValidationError(result, "stale_bundle", "", nil, fmt.Sprintf("cannot verify target: %v", err))
		return
	}
	stale := current.Mode != bundle.Target.Mode ||
		current.BaseSHA != bundle.Target.BaseSHA ||
		current.HeadSHA != bundle.Target.HeadSHA ||
		current.MergeBaseSHA != bundle.Target.MergeBaseSHA
	if bundle.Target.Mode == TargetWorkspace {
		stale = stale || state == nil || bundle.WorkspaceState == nil || *state != *bundle.WorkspaceState
	}
	if stale {
		addValidationError(result, "stale_bundle", "", nil, "review target changed after bundle creation")
	}
}

func validateFreshScanFiles(result *ValidationResult, files []File, repoDir string) {
	for _, file := range files {
		digest, err := hashScanTargetFileAtPath(repoDir, file.Path)
		if err != nil || digest != file.ContentSHA256 {
			addValidationError(
				result,
				"stale_bundle",
				file.Path,
				nil,
				"scan file changed after bundle creation",
			)
		}
	}
}

func validateOneComment(
	ctx context.Context,
	result *ValidationResult,
	bundle *Bundle,
	files map[string]File,
	contentCache map[string][]byte,
	comment ReviewComment,
	index int,
	repoDir string,
	runner *gitcmd.Runner,
) {
	commentIndex := index
	cleanPath, safe := cleanProtocolPath(comment.Path)
	if !safe {
		addValidationError(result, "path_escape", comment.Path, &commentIndex, "path must stay inside the repository")
		return
	}
	if cleanPath != comment.Path {
		addValidationError(result, "non_canonical_path", comment.Path, &commentIndex, "path must be canonical")
		return
	}
	file, exists := files[cleanPath]
	if !exists {
		addValidationError(result, "unknown_path", cleanPath, &commentIndex, "path is not present in the bundle")
		return
	}
	if !file.Reviewable {
		addValidationError(result, "excluded_path", cleanPath, &commentIndex, "path was excluded from review")
		return
	}
	if !slices.Contains(bundle.Contract.AllowedPriorities, comment.Priority) {
		addValidationError(result, "invalid_priority", cleanPath, &commentIndex, "priority is not allowed by the bundle contract")
	}
	if !slices.Contains(bundle.Contract.AllowedCategories, comment.Category) {
		addValidationError(result, "invalid_category", cleanPath, &commentIndex, "category is not allowed by the bundle contract")
	}
	if comment.Confidence < 0 || comment.Confidence > 1 {
		addValidationError(result, "invalid_confidence", cleanPath, &commentIndex, "confidence must be between 0 and 1")
	}
	if comment.Title == "" || comment.Content == "" || comment.Recommendation == "" {
		addValidationError(result, "invalid_comment", cleanPath, &commentIndex, "title, content, and recommendation are required")
	}
	if !comment.FileLevelComment && (comment.StartLine < 1 || comment.EndLine < comment.StartLine) {
		addValidationError(result, "invalid_line_range", cleanPath, &commentIndex, "line range must be one-based and ordered")
		return
	}
	if bundle.Target.Mode != TargetScan &&
		!comment.FileLevelComment &&
		!rangeTouchesHunk(comment.StartLine, comment.EndLine, file.Hunks) {
		addValidationWarning(result, "outside_changed_hunk", cleanPath, &commentIndex, "comment points outside a changed hunk")
	}
	if repoDir != "" {
		validateCommentContent(ctx, result, bundle, files, contentCache, comment, cleanPath, commentIndex, repoDir, runner)
	}
}

func validateCommentContent(
	ctx context.Context,
	result *ValidationResult,
	bundle *Bundle,
	files map[string]File,
	contentCache map[string][]byte,
	comment ReviewComment,
	path string,
	index int,
	repoDir string,
	runner *gitcmd.Runner,
) {
	content, ok := contentCache[path]
	if !ok {
		var err error
		content, err = readTargetFile(ctx, bundle, repoDir, path, runner)
		if err != nil {
			addValidationError(result, "unknown_path", path, &index, fmt.Sprintf("cannot read target file: %v", err))
			return
		}
		contentCache[path] = content
	}
	if hashFields(content) != files[path].ContentSHA256 {
		delete(contentCache, path)
		addValidationError(result, "stale_bundle", path, &index, "file content changed after bundle creation")
		return
	}
	lines := splitSourceLines(content)
	if !comment.FileLevelComment && comment.EndLine > len(lines) {
		addValidationError(result, "invalid_line_range", path, &index, "line range exceeds target file")
		return
	}
	if comment.ExistingCode == "" {
		return
	}
	if comment.FileLevelComment {
		switch count := countStandaloneSnippet(content, comment.ExistingCode); {
		case count == 1:
			return
		case count > 1:
			addValidationError(result, "ambiguous_existing_code", path, &index, "existing_code occurs more than once")
			return
		default:
			addValidationError(result, "existing_code_mismatch", path, &index, "existing_code does not match the target file")
			return
		}
	}
	if comment.StartLine >= 1 && comment.EndLine <= len(lines) {
		selected := strings.Join(lines[comment.StartLine-1:comment.EndLine], "\n")
		if selected == comment.ExistingCode {
			return
		}
	}
	if countStandaloneSnippet(content, comment.ExistingCode) > 1 {
		addValidationError(result, "ambiguous_existing_code", path, &index, "existing_code occurs more than once")
		return
	}
	addValidationError(result, "existing_code_mismatch", path, &index, "existing_code does not match the supplied line range")
}

func countStandaloneSnippet(content []byte, snippet string) int {
	if snippet == "" {
		return 0
	}
	normalizedContent := "\n" + strings.TrimRight(string(content), "\n") + "\n"
	normalizedSnippet := "\n" + strings.Trim(snippet, "\n") + "\n"
	return strings.Count(normalizedContent, normalizedSnippet)
}

func cleanProtocolPath(path string) (string, bool) {
	if path == "" || filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return "", false
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func computeCommentsSHA256(comments *Comments) string {
	if comments == nil {
		return ""
	}
	if comments.sourceSHA256 != "" {
		return comments.sourceSHA256
	}
	encoded, err := json.Marshal(comments)
	if err != nil {
		return ""
	}
	return hashFields(encoded)
}

func readTargetFile(
	ctx context.Context,
	bundle *Bundle,
	repoDir string,
	path string,
	runner *gitcmd.Runner,
) ([]byte, error) {
	if bundle.Target.Mode != TargetWorkspace && bundle.Target.Mode != TargetScan {
		if runner == nil {
			return nil, fmt.Errorf("git runner is required")
		}
		return runner.Output(
			ctx,
			repoDir,
			"-c",
			"core.quotepath=false",
			"show",
			"--end-of-options",
			bundle.Target.HeadSHA+":"+path,
		)
	}
	resolved, err := resolveScanTargetPath(repoDir, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func splitSourceLines(content []byte) []string {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func rangeTouchesHunk(start, end int, hunks []Hunk) bool {
	for _, hunk := range hunks {
		hunkEnd := hunk.NewStart + max(hunk.NewCount, 1) - 1
		if start <= hunkEnd && end >= hunk.NewStart {
			return true
		}
	}
	return false
}

func addValidationError(
	result *ValidationResult,
	code string,
	path string,
	index *int,
	message string,
) {
	result.Errors = append(result.Errors, ValidationNotice{
		Code: code, Path: path, CommentIndex: index, Message: message,
	})
}

func addValidationWarning(
	result *ValidationResult,
	code string,
	path string,
	index *int,
	message string,
) {
	result.Warnings = append(result.Warnings, ValidationNotice{
		Code: code, Path: path, CommentIndex: index, Message: message,
	})
}
