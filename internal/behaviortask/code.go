// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/config/allowlist"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var numberedLine = regexp.MustCompile(`^(\d+)\|`)

type codeTools struct {
	runner     *gitcmd.Runner
	registry   *tool.Registry
	comparison Comparison
	allowed    map[string]map[string]bool
	diffs      map[string]model.Diff
	scope      []Change
}

func prepareCodeTools(ctx context.Context, input Input) (*codeTools, error) {
	resolution := input.Resolution
	if (input.Mode != "range" && input.Mode != "commit") || !revisionPattern.MatchString(resolution.ResolvedHead) ||
		(!resolution.BaseIsEmpty && !revisionPattern.MatchString(resolution.ResolvedBase)) ||
		(resolution.BaseIsEmpty && (input.Mode != "commit" || resolution.ResolvedBase != "")) {
		return nil, fmt.Errorf("behavior requires a resolved commit or range comparison")
	}
	gitRunner, err := input.GitRunner.LocalForRepository(ctx, input.RepoDir)
	if err != nil {
		return nil, err
	}
	if input.Mode == "commit" {
		actual, err := diff.NewCommitProvider(input.RepoDir, resolution.ResolvedHead, gitRunner).ResolveCommitComparison(ctx)
		if err != nil {
			return nil, err
		}
		if actual.ResolvedBase != resolution.ResolvedBase || actual.BaseIsEmpty != resolution.BaseIsEmpty {
			return nil, fmt.Errorf("commit comparison does not match the resolved parent or empty tree")
		}
	}
	comparison := Comparison{Repository: input.RepoDir, Mode: input.Mode, Old: resolution.ResolvedBase,
		Head: resolution.ResolvedHead, BaseIsEmpty: resolution.BaseIsEmpty}
	comparison.ID = contentHash(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%t", comparison.Repository, comparison.Mode, comparison.Old, comparison.Head, comparison.BaseIsEmpty))
	current := tool.FileReader{RepoDir: input.RepoDir, Mode: tool.ModeCommit, Ref: comparison.Head, Runner: gitRunner.WithAttributeSource(comparison.Head)}
	previous := tool.FileReader{RepoDir: input.RepoDir, Mode: tool.ModeEmpty, Runner: gitRunner}
	if !comparison.BaseIsEmpty {
		previous.Mode = tool.ModeCommit
		previous.Ref = comparison.Old
		previous.Runner = gitRunner.WithAttributeSource(comparison.Old)
	}
	access, err := tool.NewComparisonCodeAccess(current, previous)
	if err != nil {
		return nil, err
	}
	result := &codeTools{runner: gitRunner, registry: tool.NewRegistry(), comparison: comparison,
		allowed: map[string]map[string]bool{}, diffs: map[string]model.Diff{}, scope: []Change{}}
	for side, reader := range map[string]tool.FileReader{"old": previous, "new": current} {
		result.allowed[side] = map[string]bool{}
		if reader.Mode == tool.ModeEmpty {
			continue
		}
		if _, err := gitRunner.Output(ctx, input.RepoDir, "cat-file", "-e", reader.Ref+"^{commit}"); err != nil {
			return nil, fmt.Errorf("%s revision is not an available commit: %w", side, err)
		}
		data, err := gitRunner.Output(ctx, input.RepoDir, "ls-tree", "-r", "-z", "--full-tree", "--end-of-options", reader.Ref)
		if err != nil {
			return nil, fmt.Errorf("list %s revision: %w", side, err)
		}
		for entry := range strings.SplitSeq(string(data), "\x00") {
			if entry == "" {
				continue
			}
			metadata, filePath, ok := strings.Cut(entry, "\t")
			if !ok {
				return nil, fmt.Errorf("invalid tree entry")
			}
			fields := strings.Fields(metadata)
			if len(fields) != 3 {
				return nil, fmt.Errorf("invalid tree metadata")
			}
			if fields[1] == "blob" && fields[0] != "120000" && safePath(filePath) && diff.ProviderDirPrefix(filePath) == "" && !(&rules.FileFilter{Exclude: input.ExcludePatterns}).IsUserExcluded(filePath) {
				result.allowed[side][filePath] = true
			}
		}
	}
	diffMap := map[string]string{}
	for _, changed := range input.Diffs {
		entry := Change{OldPath: changed.OldPath, NewPath: changed.NewPath, Deleted: changed.IsDeleted, Added: changed.IsNew, Renamed: changed.IsRenamed}
		if (!changed.IsNew && !result.allowed["old"][changed.OldPath]) || (!changed.IsDeleted && !result.allowed["new"][changed.NewPath]) {
			entry.ExcludedReason = "path is outside the authorized code scope"
		} else if changed.IsBinary {
			entry.ExcludedReason = "binary diff is unavailable to this task version"
		} else {
			key := changed.NewPath
			if changed.IsDeleted {
				key = changed.OldPath
			}
			if _, exists := diffMap[key]; exists {
				return nil, fmt.Errorf("duplicate diff path %q", key)
			}
			diffMap[key] = changed.Diff
			result.diffs[key] = changed
		}
		result.scope = append(result.scope, entry)
	}
	result.registry.Register(&tool.FileReadProvider{Access: access})
	result.registry.Register(&tool.CodeSearchProvider{Access: access})
	result.registry.Register(tool.NewFileReadDiff(tool.NewDiffMap(diffMap)))
	result.registry.Freeze()
	return result, nil
}

func safePath(filePath string) bool {
	lower := strings.ToLower(filePath)
	if allowedext.IsSecretPath(filePath) || path.Base(lower) == ".git-credentials" || strings.HasSuffix("/"+lower, "/.docker/config.json") {
		return false
	}
	if !fs.ValidPath(filePath) || !filepath.IsLocal(filePath) || filePath == "." || strings.ContainsAny(filePath, "\\:\x00\r\n\t") {
		return false
	}
	for component := range strings.SplitSeq(lower, "/") {
		if component == ".." || component == ".git" || component == ".ssh" || component == ".aws" || component == ".gnupg" {
			return false
		}
	}
	base := path.Base(lower)
	return base != ".env" && !strings.HasPrefix(base, ".env.") && base != ".netrc" && base != ".npmrc" &&
		base != ".pypirc" && base != "credentials" && !strings.HasPrefix(base, "id_rsa") && !strings.HasPrefix(base, "id_ed25519") &&
		!strings.HasSuffix(base, ".pem") && !strings.HasSuffix(base, ".key") && !strings.HasSuffix(base, ".p12") && !strings.HasSuffix(base, ".pfx")
}

func (code *codeTools) execute(ctx context.Context, name string, args map[string]any) (Observation, error) {
	side, _ := args["side"].(string)
	if side == "" {
		side = "new"
	}
	observation := Observation{ToolName: name, Arguments: args, SourceKind: "code", Side: side, Revision: code.comparison.Head, Locations: []Location{}}
	if side == "old" {
		observation.Revision = code.comparison.Old
	}
	if name == "file_read_diff" {
		observation.Side = ""
		observation.Revision = ""
	}
	if name == "file_read" {
		filePath, _ := args["file_path"].(string)
		if !code.allowed[side][filePath] {
			observation.ErrorClass = "unavailable"
			return observation, fmt.Errorf("file is unavailable in the authorized %s scope", side)
		}
	}
	if name == "file_find" {
		if err := ctx.Err(); err != nil {
			return observation, err
		}
		paths := slices.Sorted(maps.Keys(code.allowed[side]))
		query, _ := args["query_name"].(string)
		sensitive, _ := args["case_sensitive"].(bool)
		matches, err := tool.MatchFileNames(ctx, paths, query, sensitive, 101, func(filePath string) bool { return code.allowed[side][filePath] })
		if err != nil {
			return observation, err
		}
		if len(matches) > 100 {
			observation.Truncated = true
			observation.Continuation = "Narrow query_name to see more authorized files."
			matches = matches[:100]
		}
		observation.ReturnedContent = strings.Join(matches, "\n")
		if len(matches) == 0 {
			observation.ReturnedContent = "No authorized files returned."
		}
		return observation, nil
	}
	provider, ok := code.registry.Get(name)
	if !ok {
		return observation, fmt.Errorf("tool is unavailable")
	}
	var content string
	var err error
	if name == "code_search" {
		content, err = code.search(ctx, provider, args, side)
	} else {
		content, err = provider.Execute(ctx, args)
	}
	observation.ReturnedContent = content
	if err != nil {
		return observation, err
	}
	if !utf8.ValidString(content) {
		observation.ErrorClass = "unsupported_encoding"
		return observation, fmt.Errorf("non-UTF-8 code output is unavailable")
	}
	if strings.HasPrefix(content, "Error:") || strings.Contains(content, "\nError:") ||
		(name != "code_search" && (strings.HasPrefix(content, "Warning:") || strings.Contains(content, "\nWarning:"))) {
		observation.ErrorClass = "tool_error"
	}
	if name == "file_read" {
		headers := strings.SplitN(stripSide(content), "\n", 4)
		observation.Truncated = len(headers) > 1 && headers[1] == "IS_TRUNCATED: true"
	} else if name == "code_search" {
		observation.Truncated = strings.HasPrefix(stripSide(content), "Note: Showing the first ")
	}
	if observation.Truncated {
		observation.Continuation = "Narrow the line range or file_patterns and read again."
	}
	if name == "file_read" || name == "code_search" {
		filePath, _ := args["file_path"].(string)
		for line := range strings.SplitSeq(stripSide(content), "\n") {
			if name == "code_search" && strings.HasPrefix(line, "File: ") {
				filePath = strings.TrimPrefix(line, "File: ")
			}
			match := numberedLine.FindStringSubmatch(line)
			if match == nil || !code.allowed[side][filePath] {
				continue
			}
			number, _ := strconv.Atoi(match[1])
			observation.Locations = append(observation.Locations, Location{Side: side, Revision: observation.Revision, Path: filePath, StartLine: number, EndLine: number})
		}
	}
	if name == "code_search" && (strings.HasPrefix(content, tool.CodeSearchIncompletePrefix) ||
		strings.Contains(content, "\n"+tool.CodeSearchIncompletePrefix)) {
		observation.Truncated = true
		observation.Continuation = "Search incomplete: Git reported a read or command failure. Cite only returned matches; do not infer absence. Narrow file_patterns and retry."
		if len(observation.Locations) == 0 {
			observation.ErrorClass = "tool_error"
		}
	}
	if name == "file_read_diff" {
		paths, _ := args["path_array"].([]any)
		for _, rawPath := range paths {
			filePath, _ := rawPath.(string)
			changed, exists := code.diffs[filePath]
			if !exists {
				continue
			}
			for _, hunk := range diff.HunkRanges(changed.Diff) {
				if hunk.OldCount > 0 {
					observation.Locations = append(observation.Locations, Location{"old", code.comparison.Old, changed.OldPath, hunk.OldStart, hunk.OldStart + hunk.OldCount - 1})
				}
				if hunk.NewCount > 0 {
					observation.Locations = append(observation.Locations, Location{"new", code.comparison.Head, changed.NewPath, hunk.NewStart, hunk.NewStart + hunk.NewCount - 1})
				}
			}
		}
	}
	return observation, nil
}

func stripSide(content string) string {
	if strings.HasPrefix(content, "SIDE: old\n") {
		parts := strings.SplitN(content, "\n", 3)
		if len(parts) == 3 {
			return parts[2]
		}
	}
	return content
}
