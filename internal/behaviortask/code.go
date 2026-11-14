// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var numberedLine = regexp.MustCompile(`^(\d+)\|`)

type codeTools struct {
	registry   *tool.Registry
	comparison Comparison
	allowed    map[string]map[string]bool
	diffs      map[string]model.Diff
	scope      []Change
}

func prepareCodeTools(ctx context.Context, input Input) (*codeTools, error) {
	resolution := input.Resolution
	workspace := input.Mode == "workspace"
	if (!workspace && input.Mode != "range" && input.Mode != "commit") ||
		(!workspace && !revisionPattern.MatchString(resolution.ResolvedHead)) ||
		(workspace && (resolution.ResolvedHead != "" || resolution.ExactRange != "")) ||
		(!resolution.BaseIsEmpty && !revisionPattern.MatchString(resolution.ResolvedBase)) ||
		(resolution.BaseIsEmpty && ((!workspace && input.Mode != "commit") || resolution.ResolvedBase != "")) {
		return nil, fmt.Errorf("behavior requires a resolved range, commit, or workspace comparison")
	}
	gitRunner := gitcmd.New(1)
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
	current := tool.FileReader{RepoDir: input.RepoDir, Mode: tool.ModeCommit, Ref: comparison.Head, Runner: gitRunner}
	previous := current
	previous.Ref = comparison.Old
	if comparison.BaseIsEmpty {
		previous.Mode = tool.ModeEmpty
	}
	var access *tool.CodeAccess
	var err error
	if workspace {
		current.Mode = tool.ModeWorkspace
		access, err = tool.NewWorkspaceCodeAccess(current, previous)
	} else {
		access, err = tool.NewComparisonCodeAccess(current, previous)
	}
	if err != nil {
		return nil, err
	}
	result := &codeTools{registry: tool.NewRegistry(), comparison: comparison,
		allowed: map[string]map[string]bool{}, diffs: map[string]model.Diff{}, scope: []Change{}}
	for side, reader := range map[string]tool.FileReader{"old": previous, "new": current} {
		result.allowed[side] = map[string]bool{}
		if reader.Mode == tool.ModeEmpty {
			continue
		}
		if reader.Mode == tool.ModeWorkspace {
			if err := result.loadWorkspaceScope(ctx, input, gitRunner); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := gitRunner.Output(ctx, input.RepoDir, "cat-file", "-e", reader.Ref+"^{commit}"); err != nil {
			return nil, fmt.Errorf("%s revision is not an available commit: %w", side, err)
		}
		data, err := gitRunner.Output(ctx, input.RepoDir, "ls-tree", "-r", "-z", "--full-tree", "--end-of-options", reader.Ref)
		if err != nil {
			return nil, fmt.Errorf("list %s revision: %w", side, err)
		}
		for _, entry := range strings.Split(string(data), "\x00") {
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
	result.registry.Register(&tool.FileFindProvider{Access: access})
	result.registry.Register(tool.NewFileReadDiff(tool.NewDiffMap(diffMap)))
	result.registry.Freeze()
	return result, nil
}

func safePath(filePath string) bool {
	if filePath == "" || path.IsAbs(filePath) || path.Clean(filePath) != filePath || strings.ContainsAny(filePath, "\\\x00\r\n\t:") {
		return false
	}
	for _, component := range strings.Split(strings.ToLower(filePath), "/") {
		if component == ".." || component == ".git" || component == ".ssh" || component == ".aws" || component == ".gnupg" {
			return false
		}
	}
	base := strings.ToLower(path.Base(filePath))
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
		if !code.fileAllowed(side, filePath) {
			observation.ErrorClass = "unavailable"
			return observation, fmt.Errorf("file is unavailable in the authorized %s scope", side)
		}
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
	if strings.HasPrefix(content, "Error:") || strings.Contains(content, "\nError:") || strings.Contains(content, "\nWarning:") {
		observation.ErrorClass = "tool_error"
	}
	if name == "file_find" {
		body := stripSide(content)
		lines := strings.Split(body, "\n")
		observation.Truncated = len(lines) >= 100
		visible := []string{}
		for _, filePath := range lines {
			if code.fileAllowed(side, filePath) {
				visible = append(visible, filePath)
			}
		}
		observation.ReturnedContent = strings.Join(visible, "\n")
		if len(visible) == 0 {
			observation.ReturnedContent = "No authorized files returned."
		}
		if observation.Truncated {
			observation.Continuation = "Narrow query_name; the underlying enumeration returned its 100-file limit before scope filtering."
		}
		return observation, nil
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
		for _, line := range strings.Split(stripSide(content), "\n") {
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
	if name == "file_read_diff" {
		paths, _ := args["path_array"].([]any)
		for _, rawPath := range paths {
			filePath, _ := rawPath.(string)
			changed, exists := code.diffs[filePath]
			if !exists {
				continue
			}
			for _, hunk := range diff.ParseHunks(changed.Diff) {
				oldLine, newLine := hunk.OldStart, hunk.NewStart
				for _, line := range hunk.Lines {
					if line.Type != diff.HunkAdded && oldLine < hunk.OldStart+hunk.OldCount {
						observation.Locations = append(observation.Locations, Location{"old", code.comparison.Old, changed.OldPath, oldLine, oldLine})
						oldLine++
					}
					if line.Type != diff.HunkDeleted && newLine < hunk.NewStart+hunk.NewCount {
						observation.Locations = append(observation.Locations, Location{"new", code.comparison.Head, changed.NewPath, newLine, newLine})
						newLine++
					}
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
