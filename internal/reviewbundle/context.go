package reviewbundle

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// ContextResult is the stable envelope returned by all read-only context operations.
type ContextResult struct {
	SchemaVersion string `json:"schema_version"`
	BundleID      string `json:"bundle_id"`
	Operation     string `json:"operation"`
	Result        string `json:"result"`
}

// ContextService exposes target-aware read-only repository tools.
type ContextService struct {
	repoDir string
	bundle  *Bundle
	runner  *gitcmd.Runner
	reader  *tool.FileReader
	readyMu sync.Mutex
	readyOk bool
}

// NewContextService binds all subsequent operations to one bundle identity.
func NewContextService(
	repoDir string,
	bundle *Bundle,
	runner *gitcmd.Runner,
) *ContextService {
	mode := tool.ModeWorkspace
	ref := ""
	if bundle != nil {
		switch bundle.Target.Mode {
		case TargetRange:
			mode = tool.ModeRange
			ref = bundle.Target.HeadSHA
		case TargetCommit:
			mode = tool.ModeCommit
			ref = bundle.Target.HeadSHA
		}
	}
	return &ContextService{
		repoDir: repoDir,
		bundle:  bundle,
		runner:  runner,
		reader: &tool.FileReader{
			RepoDir: repoDir,
			Mode:    mode,
			Ref:     ref,
			Runner:  runner,
		},
	}
}

// Read returns at most 500 target-version lines through the native reader.
func (service *ContextService) Read(
	ctx context.Context,
	path string,
	startLine int,
	maxLines int,
) (ContextResult, error) {
	if err := service.ready(ctx); err != nil {
		return ContextResult{}, err
	}
	cleaned, safe := cleanProtocolPath(path)
	if !safe {
		return ContextResult{}, &ProtocolError{Code: "path_escape", Message: "path must stay inside the repository"}
	}
	if startLine <= 0 {
		startLine = 1
	}
	if maxLines <= 0 {
		maxLines = 200
	}
	if maxLines > 500 {
		maxLines = 500
	}
	if service.bundle.Target.Mode == TargetScan {
		result, err := service.readScanFile(cleaned, startLine, maxLines)
		if err != nil {
			return ContextResult{}, err
		}
		return service.result("read", result), nil
	}
	endLine := startLine + maxLines - 1
	result, err := tool.NewFileRead(service.reader).Execute(ctx, map[string]any{
		"file_path":  cleaned,
		"start_line": float64(startLine),
		"end_line":   float64(endLine),
	})
	if err != nil {
		return ContextResult{}, err
	}
	return service.result("read", result), nil
}

// Find lists target-version files whose base name contains query.
func (service *ContextService) Find(
	ctx context.Context,
	query string,
	caseSensitive bool,
) (ContextResult, error) {
	if err := service.ready(ctx); err != nil {
		return ContextResult{}, err
	}
	if service.bundle.Target.Mode == TargetScan {
		return service.result("find", service.findScanFiles(query, caseSensitive)), nil
	}
	result, err := tool.NewFileFind(service.reader).Execute(ctx, map[string]any{
		"query_name":     query,
		"case_sensitive": caseSensitive,
	})
	if err != nil {
		return ContextResult{}, err
	}
	return service.result("find", result), nil
}

// Diff returns exact patches stored in the immutable bundle.
func (service *ContextService) Diff(
	ctx context.Context,
	paths []string,
) (ContextResult, error) {
	if err := service.ready(ctx); err != nil {
		return ContextResult{}, err
	}
	diffMap := make(map[string]string, len(service.bundle.Files))
	for _, file := range service.bundle.Files {
		evidence := file.Patch
		if service.bundle.Target.Mode == TargetScan {
			evidence = file.Content
		}
		diffMap[file.Path] = evidence
	}
	pathArguments := make([]any, 0, len(paths))
	for _, path := range paths {
		cleaned, safe := cleanProtocolPath(path)
		if !safe {
			return ContextResult{}, &ProtocolError{Code: "path_escape", Message: "path must stay inside the repository"}
		}
		pathArguments = append(pathArguments, cleaned)
	}
	result, err := tool.NewFileReadDiff(tool.NewDiffMap(diffMap)).Execute(ctx, map[string]any{
		"path_array": pathArguments,
	})
	if err != nil {
		return ContextResult{}, err
	}
	return service.result("diff", result), nil
}

// Search runs the native bounded code search against the target version.
func (service *ContextService) Search(
	ctx context.Context,
	query string,
	caseSensitive bool,
	usePerlRegexp bool,
	patterns []string,
) (ContextResult, error) {
	if err := service.ready(ctx); err != nil {
		return ContextResult{}, err
	}
	patternArguments := make([]any, 0, len(patterns))
	for _, pattern := range patterns {
		if !safeSearchPattern(pattern) {
			return ContextResult{}, &ProtocolError{Code: "path_escape", Message: "file pattern must stay inside the repository"}
		}
		patternArguments = append(patternArguments, pattern)
	}
	if service.bundle.Target.Mode == TargetScan {
		if usePerlRegexp {
			return ContextResult{}, &ProtocolError{
				Code:    "unsupported_search_regex",
				Message: "--perl-regexp is not supported for scan bundle context search",
			}
		}
		result, err := service.searchScanFiles(query, caseSensitive, usePerlRegexp, patterns)
		if err != nil {
			return ContextResult{}, err
		}
		return service.result("search", result), nil
	}
	result, err := tool.NewCodeSearch(service.reader).Execute(ctx, map[string]any{
		"search_text":     query,
		"case_sensitive":  caseSensitive,
		"use_perl_regexp": usePerlRegexp,
		"file_patterns":   patternArguments,
	})
	if err != nil {
		return ContextResult{}, err
	}
	return service.result("search", result), nil
}

func safeSearchPattern(pattern string) bool {
	if pattern == "" || filepath.IsAbs(pattern) || strings.ContainsRune(pattern, '\x00') {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(pattern), "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func (service *ContextService) scanFile(path string) (File, bool) {
	for _, file := range service.bundle.Files {
		if file.Path == path {
			return file, true
		}
	}
	return File{}, false
}

func (service *ContextService) readScanFile(path string, startLine int, maxLines int) (string, error) {
	file, ok := service.scanFile(path)
	if !ok {
		return "", fmt.Errorf("file %q is not present in the scan bundle", path)
	}
	lines := splitSourceLines([]byte(file.Content))
	totalLines := len(lines)
	if totalLines == 0 {
		return "", fmt.Errorf("file %q is empty", path)
	}
	if startLine > totalLines {
		return "", fmt.Errorf("file %q has only %d lines, requested range starting at %d", path, totalLines, startLine)
	}
	end := startLine - 1 + maxLines
	if end > totalLines {
		end = totalLines
	}
	truncated := end < totalLines
	displayEnd := startLine - 1
	if end > startLine-1 {
		displayEnd = end
	}
	var output strings.Builder
	output.WriteString(fmt.Sprintf("File: %s (Total lines: %d)\n", path, totalLines))
	output.WriteString(fmt.Sprintf("IS_TRUNCATED: %t\n", truncated))
	output.WriteString(fmt.Sprintf("LINE_RANGE: %d-%d\n", startLine, displayEnd))
	for index, line := range lines[startLine-1 : end] {
		output.WriteString(fmt.Sprintf("%d|%s\n", startLine+index, line))
	}
	if truncated {
		output.WriteString("\nNote: Results truncated to 500 lines. Please narrow your line range.\n")
	}
	return output.String(), nil
}

func (service *ContextService) findScanFiles(query string, caseSensitive bool) string {
	if strings.TrimSpace(query) == "" {
		return "// The file was not found"
	}
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	var matched []string
	for _, file := range service.bundle.Files {
		base := file.Path
		if index := strings.LastIndex(base, "/"); index >= 0 {
			base = base[index+1:]
		}
		haystack := base
		if !caseSensitive {
			haystack = strings.ToLower(haystack)
		}
		if strings.Contains(haystack, needle) {
			matched = append(matched, file.Path)
		}
	}
	sort.Strings(matched)
	if len(matched) == 0 {
		return "// The file was not found"
	}
	if len(matched) > 100 {
		matched = matched[:100]
	}
	return strings.Join(matched, "\n")
}

func (service *ContextService) searchScanFiles(
	query string,
	caseSensitive bool,
	useRegexp bool,
	patterns []string,
) (string, error) {
	if err := validateScanSearchPatterns(patterns); err != nil {
		return "", err
	}
	matcher, err := scanSearchMatcher(query, caseSensitive, useRegexp)
	if err != nil {
		return "", err
	}
	type match struct {
		line int
		text string
	}
	fileMatches := make(map[string][]match)
	var paths []string
	count := 0
	for _, file := range service.bundle.Files {
		if !scanPatternMatches(file.Path, patterns) {
			continue
		}
		for index, line := range splitSourceLines([]byte(file.Content)) {
			if matcher(line) {
				if _, ok := fileMatches[file.Path]; !ok {
					paths = append(paths, file.Path)
				}
				fileMatches[file.Path] = append(fileMatches[file.Path], match{line: index + 1, text: line})
				count++
				if count >= 100 {
					break
				}
			}
		}
		if count >= 100 {
			break
		}
	}
	if count == 0 {
		return "No matches found", nil
	}
	sort.Strings(paths)
	var output strings.Builder
	if count >= 100 {
		output.WriteString("Note: The results have been truncated. Only showing first 100 results.\n")
	}
	for _, path := range paths {
		matches := fileMatches[path]
		output.WriteString(fmt.Sprintf("File: %s\nMatch lines: %d\n", path, len(matches)))
		for _, match := range matches {
			output.WriteString(fmt.Sprintf("%d|%s\n", match.line, match.text))
		}
		output.WriteString("\n")
	}
	return output.String(), nil
}

func scanSearchMatcher(query string, caseSensitive bool, useRegexp bool) (func(string) bool, error) {
	if strings.TrimSpace(query) == "" {
		return func(string) bool { return false }, nil
	}
	if useRegexp {
		expression := query
		if !caseSensitive {
			expression = "(?i)" + expression
		}
		compiled, err := regexp.Compile(expression)
		if err != nil {
			return nil, err
		}
		return compiled.MatchString, nil
	}
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	return func(line string) bool {
		if !caseSensitive {
			line = strings.ToLower(line)
		}
		return strings.Contains(line, needle)
	}, nil
}

func scanPatternMatches(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		if strings.HasPrefix(path, strings.TrimSuffix(pattern, "/")+"/") {
			return true
		}
		// ponytail: minimal git-pathspec compatibility; expand if scan search needs full pathspec semantics.
		if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(path, strings.TrimPrefix(pattern, "*")) {
			return true
		}
	}
	return false
}

func validateScanSearchPatterns(patterns []string) error {
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, ":(exclude)") ||
			strings.HasPrefix(pattern, ":!") ||
			strings.HasPrefix(pattern, ":^") {
			return fmt.Errorf("unsupported scan search pathspec %q: exclude patterns are not supported for scan bundles", pattern)
		}
	}
	return nil
}

func (service *ContextService) ready(ctx context.Context) error {
	service.readyMu.Lock()
	defer service.readyMu.Unlock()
	if service.readyOk {
		return nil
	}
	if service.bundle == nil {
		return fmt.Errorf("bundle is required")
	}
	if service.repoDir == "" {
		return fmt.Errorf("repository directory is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result := ValidationResult{Errors: make([]ValidationNotice, 0)}
	validateFreshTarget(ctx, &result, service.bundle, service.repoDir, service.runner)
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return &ProtocolError{Code: "stale_bundle", Message: result.Errors[0].Message}
	}
	service.readyOk = true
	return nil
}

func (service *ContextService) result(operation, result string) ContextResult {
	return ContextResult{
		SchemaVersion: "agent-review-context/v1",
		BundleID:      service.bundle.BundleID,
		Operation:     operation,
		Result:        result,
	}
}
