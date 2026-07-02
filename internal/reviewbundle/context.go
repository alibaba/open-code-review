package reviewbundle

import (
	"context"
	"fmt"
	"path/filepath"
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
	repoDir   string
	bundle    *Bundle
	runner    *gitcmd.Runner
	reader    *tool.FileReader
	readyOnce sync.Once
	readyErr  error
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

func (service *ContextService) ready(ctx context.Context) error {
	service.readyOnce.Do(func() {
		if service.bundle == nil {
			service.readyErr = fmt.Errorf("bundle is required")
			return
		}
		if service.repoDir == "" {
			service.readyErr = fmt.Errorf("repository directory is required")
			return
		}
		result := ValidationResult{Errors: make([]ValidationNotice, 0)}
		validateFreshTarget(ctx, &result, service.bundle, service.repoDir, service.runner)
		if len(result.Errors) > 0 {
			service.readyErr = &ProtocolError{Code: "stale_bundle", Message: result.Errors[0].Message}
		}
	})
	return service.readyErr
}

func (service *ContextService) result(operation, result string) ContextResult {
	return ContextResult{
		SchemaVersion: "codex-review-context/v1",
		BundleID:      service.bundle.BundleID,
		Operation:     operation,
		Result:        result,
	}
}
