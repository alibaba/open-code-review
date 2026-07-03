package reviewbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/reviewfilter"
)

// DefaultMaxBundleBytes is the default hard limit for a single JSON bundle.
const DefaultMaxBundleBytes int64 = 4 * 1024 * 1024

// PrepareOptions configures deterministic review bundle generation.
type PrepareOptions struct {
	RepoDir       string
	Target        TargetSpec
	Resolver      rules.Resolver
	FileFilter    *rules.FileFilter
	GitRunner     *gitcmd.Runner
	MaxBundleSize int64
}

// ProtocolError is an error with a stable machine-readable code.
type ProtocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ProtocolError) Error() string {
	return e.Code + ": " + e.Message
}

// Prepare builds and serializes a deterministic bundle without invoking an LLM.
func Prepare(ctx context.Context, options PrepareOptions) (*Bundle, []byte, error) {
	if options.RepoDir == "" {
		return nil, nil, fmt.Errorf("repository directory is required")
	}
	if options.GitRunner == nil {
		return nil, nil, fmt.Errorf("git runner is required")
	}
	detailResolver, ok := options.Resolver.(rules.DetailResolver)
	if !ok {
		return nil, nil, fmt.Errorf("rule resolver must expose source details")
	}
	maxBundleSize := options.MaxBundleSize
	if maxBundleSize <= 0 {
		maxBundleSize = DefaultMaxBundleBytes
	}

	target, workspaceState, err := ResolveTarget(
		ctx, options.RepoDir, options.Target, options.GitRunner,
	)
	if err != nil {
		return nil, nil, err
	}
	changes, err := loadTargetDiffs(ctx, options)
	if err != nil {
		return nil, nil, fmt.Errorf("load target diffs: %w", err)
	}

	bundle := &Bundle{
		SchemaVersion:  BundleSchemaVersion,
		Target:         target,
		WorkspaceState: workspaceState,
		Rules:          make(map[string]Rule),
		Files:          make([]File, 0, len(changes)),
		Contract:       DefaultContract(),
	}
	bundle.Contract.MaxBundleBytes = maxBundleSize
	buildBundleEvidence(bundle, changes, detailResolver, options.FileFilter)
	bundle.Target.DiffSHA256 = hashDiffs(changes)

	bundleID, err := computeBundleID(bundle)
	if err != nil {
		return nil, nil, err
	}
	bundle.BundleID = bundleID
	encoded, err := marshalWithStableSize(bundle)
	if err != nil {
		return nil, nil, err
	}
	if int64(len(encoded)) > maxBundleSize {
		return nil, nil, &ProtocolError{
			Code: "bundle_too_large",
			Message: fmt.Sprintf(
				"encoded bundle is %d bytes; maximum is %d bytes",
				len(encoded),
				maxBundleSize,
			),
		}
	}
	return bundle, encoded, nil
}

func loadTargetDiffs(ctx context.Context, options PrepareOptions) ([]model.Diff, error) {
	var provider *diff.Provider
	switch {
	case options.Target.Commit != "":
		provider = diff.NewCommitProvider(
			options.RepoDir, options.Target.Commit, options.GitRunner,
		)
	case options.Target.From != "":
		provider = diff.NewProvider(
			options.RepoDir, options.Target.From, options.Target.To, options.GitRunner,
		)
	default:
		provider = diff.NewWorkspaceProvider(options.RepoDir, options.GitRunner)
	}
	return provider.GetDiff(ctx)
}

func buildBundleEvidence(
	bundle *Bundle,
	changes []model.Diff,
	resolver rules.DetailResolver,
	fileFilter *rules.FileFilter,
) {
	filter := reviewfilter.Filter{FileFilter: fileFilter}
	ruleIDs := make(map[string]string)
	for _, change := range changes {
		path := reviewfilter.EffectivePath(change)
		excludeReason := filter.ExcludeReason(change)
		if excludeReason == model.ExcludeNone && change.IsDeleted {
			excludeReason = model.ExcludeDeleted
		}
		reviewable := excludeReason == model.ExcludeNone
		detail := resolver.ResolveDetail(path)
		ruleID := internRule(bundle.Rules, ruleIDs, detail)
		hunks := convertHunks(diff.ParseHunks(change.Diff))

		contentSHA256 := hashFields([]byte(change.NewFileContent))
		if change.IsDeleted {
			contentSHA256 = ""
		}

		bundle.Files = append(bundle.Files, File{
			Path:          path,
			OldPath:       change.OldPath,
			Status:        reviewfilter.Status(change),
			Reviewable:    reviewable,
			ExcludeReason: excludeReason,
			Insertions:    change.Insertions,
			Deletions:     change.Deletions,
			ContentSHA256: contentSHA256,
			RuleID:        ruleID,
			Patch:         change.Diff,
			Hunks:         hunks,
		})
		bundle.Summary.TotalFiles++
		bundle.Summary.Insertions += change.Insertions
		bundle.Summary.Deletions += change.Deletions
		if reviewable {
			bundle.Summary.ReviewableFiles++
		} else {
			bundle.Summary.ExcludedFiles++
		}
	}
}

func internRule(
	ruleTable map[string]Rule,
	ruleIDs map[string]string,
	detail rules.RuleDetail,
) string {
	key := hashFields(
		[]byte(detail.Source),
		[]byte(detail.Pattern),
		[]byte(detail.Rule),
	)
	if existing, ok := ruleIDs[key]; ok {
		return existing
	}
	hashSuffix := strings.TrimPrefix(key, "sha256:")
	ruleID := "rule-" + hashSuffix[:16]
	if _, exists := ruleTable[ruleID]; exists {
		ruleID = "rule-" + hashSuffix
	}
	ruleIDs[key] = ruleID
	ruleTable[ruleID] = Rule{
		Source:  detail.Source,
		Pattern: detail.Pattern,
		Content: detail.Rule,
	}
	return ruleID
}

func convertHunks(parsed []diff.Hunk) []Hunk {
	hunks := make([]Hunk, 0, len(parsed))
	for _, parsedHunk := range parsed {
		hunks = append(hunks, Hunk{
			OldStart: parsedHunk.OldStart,
			OldCount: parsedHunk.OldCount,
			NewStart: parsedHunk.NewStart,
			NewCount: parsedHunk.NewCount,
		})
	}
	return hunks
}

func hashDiffs(changes []model.Diff) string {
	fields := make([][]byte, 0, len(changes)*3)
	for _, change := range changes {
		fields = append(
			fields,
			[]byte(change.OldPath),
			[]byte(change.NewPath),
			[]byte(change.Diff),
		)
	}
	return hashFields(fields...)
}

func computeBundleID(bundle *Bundle) (string, error) {
	// Only scalar identity fields are changed on this shallow copy.
	canonical := *bundle
	canonical.BundleID = ""
	canonical.Contract.BundleSizeBytes = 0
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal bundle identity: %w", err)
	}
	return hashFields(encoded), nil
}

func marshalWithStableSize(bundle *Bundle) ([]byte, error) {
	var encoded []byte
	sizes := make([]int64, 0, 8)
	for range 8 {
		var err error
		encoded, err = json.Marshal(bundle)
		if err != nil {
			return nil, fmt.Errorf("marshal review bundle: %w", err)
		}
		size := int64(len(encoded))
		sizes = append(sizes, size)
		if bundle.Contract.BundleSizeBytes == size {
			return encoded, nil
		}
		bundle.Contract.BundleSizeBytes = size
	}
	return nil, fmt.Errorf("stabilize encoded bundle size; observed sizes: %v", sizes)
}
