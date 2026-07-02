package reviewbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/scan"
)

const ScanManifestSchemaVersion = "codex-review-manifest/v1"

// ScanOptions configures deterministic full-file scan preparation.
type ScanOptions struct {
	RepoDir          string
	Paths            []string
	Resolver         rules.Resolver
	FileFilter       *rules.FileFilter
	GitRunner        *gitcmd.Runner
	MaxFileSizeBytes int64
	MaxTokenBudget   int64
	MaxBundleSize    int64
	BatchStrategy    string
	BatchSize        int
}

// ScanManifest links all deterministic full-file review bundles.
type ScanManifest struct {
	SchemaVersion   string            `json:"schema_version"`
	ManifestID      string            `json:"manifest_id"`
	Root            string            `json:"root"`
	TargetHash      string            `json:"target_hash"`
	BatchStrategy   string            `json:"batch_strategy"`
	BatchSize       int               `json:"batch_size"`
	EstimatedTokens int64             `json:"estimated_tokens"`
	Summary         Summary           `json:"summary"`
	Partial         bool              `json:"partial"`
	SkippedFiles    []ScanSkippedFile `json:"skipped_files"`
	Bundles         []Bundle          `json:"bundles"`
	Warnings        []ProtocolNotice  `json:"warnings,omitempty"`
}

// ScanSkippedFile records every enumerated file not included for review.
type ScanSkippedFile struct {
	Path            string `json:"path"`
	Reason          string `json:"reason"`
	EstimatedTokens int64  `json:"estimated_tokens,omitempty"`
}

// PrepareScan enumerates, filters, budgets, groups, and serializes full files.
func PrepareScan(ctx context.Context, options ScanOptions) (*ScanManifest, []byte, error) {
	if options.RepoDir == "" {
		return nil, nil, fmt.Errorf("scan root is required")
	}
	detailResolver, ok := options.Resolver.(rules.DetailResolver)
	if !ok {
		return nil, nil, fmt.Errorf("rule resolver must expose source details")
	}
	maxBundleSize := options.MaxBundleSize
	if maxBundleSize <= 0 {
		maxBundleSize = DefaultMaxBundleBytes
	}
	provider := scan.NewProvider(
		options.RepoDir,
		options.Paths,
		options.GitRunner,
		options.MaxFileSizeBytes,
	)
	items, providerSkipped, err := provider.EnumerateDetailed(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("enumerate scan target: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })

	batchStrategy := scan.ParseBatchStrategy(options.BatchStrategy)
	manifest := &ScanManifest{
		SchemaVersion: ScanManifestSchemaVersion,
		Root:          options.RepoDir,
		BatchStrategy: string(batchStrategy),
		BatchSize:     options.BatchSize,
		SkippedFiles:  make([]ScanSkippedFile, 0),
		Bundles:       make([]Bundle, 0),
	}
	for _, skipped := range providerSkipped {
		manifest.SkippedFiles = append(manifest.SkippedFiles, ScanSkippedFile{
			Path: skipped.Path, Reason: skipped.Reason,
		})
	}
	manifest.Summary.TotalFiles = len(items) + len(providerSkipped)
	included, budgetTruncated := filterAndBudgetScanItems(manifest, items, options)
	manifest.EstimatedTokens = scan.EstimateTokens(included, true, true, true).TotalTokens
	manifest.Summary.ReviewableFiles = len(included)
	manifest.Summary.ExcludedFiles = manifest.Summary.TotalFiles - len(included)
	for _, item := range included {
		manifest.Summary.Insertions += int64(item.LineCount)
	}
	manifest.Partial = budgetTruncated
	manifest.TargetHash = hashScanItems(included)

	batches := scan.GroupBatches(
		included,
		batchStrategy,
		options.BatchSize,
	)
	for batchIndex, batch := range batches {
		bundle, err := buildScanBundle(
			batch,
			batchIndex,
			manifest.TargetHash,
			detailResolver,
			maxBundleSize,
		)
		if err != nil {
			return nil, nil, err
		}
		manifest.Bundles = append(manifest.Bundles, *bundle)
	}
	manifestID, err := computeManifestID(manifest)
	if err != nil {
		return nil, nil, err
	}
	manifest.ManifestID = manifestID
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal scan manifest: %w", err)
	}
	return manifest, encoded, nil
}

func filterAndBudgetScanItems(
	manifest *ScanManifest,
	items []model.ScanItem,
	options ScanOptions,
) ([]model.ScanItem, bool) {
	included := make([]model.ScanItem, 0, len(items))
	var budgetUsed int64
	budgetTruncated := false
	for _, item := range items {
		reason := scan.ExcludeReason(item, options.FileFilter)
		if reason != model.ExcludeNone {
			manifest.SkippedFiles = append(manifest.SkippedFiles, ScanSkippedFile{
				Path: item.Path, Reason: string(reason),
			})
			continue
		}
		estimated := scan.EstimateItemTokens(item, true)
		if options.MaxTokenBudget > 0 && budgetUsed+estimated > options.MaxTokenBudget {
			budgetTruncated = true
			manifest.SkippedFiles = append(manifest.SkippedFiles, ScanSkippedFile{
				Path: item.Path, Reason: "token_budget", EstimatedTokens: estimated,
			})
			continue
		}
		budgetUsed += estimated
		included = append(included, item)
	}
	return included, budgetTruncated
}

func buildScanBundle(
	items []model.ScanItem,
	batchIndex int,
	targetHash string,
	resolver rules.DetailResolver,
	maxBundleSize int64,
) (*Bundle, error) {
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		Target: Target{
			Mode:       TargetScan,
			DiffSHA256: targetHash,
		},
		Rules:    make(map[string]Rule),
		Files:    make([]File, 0, len(items)),
		Contract: DefaultContract(),
	}
	bundle.Contract.MaxBundleBytes = maxBundleSize
	ruleIDs := make(map[string]string)
	for _, item := range items {
		ruleID := internRule(bundle.Rules, ruleIDs, resolver.ResolveDetail(item.Path))
		bundle.Files = append(bundle.Files, File{
			Path:          item.Path,
			OldPath:       item.Path,
			Status:        "scan",
			Reviewable:    true,
			Insertions:    int64(item.LineCount),
			ContentSHA256: hashFields([]byte(item.Content)),
			RuleID:        ruleID,
			Content:       item.Content,
			Hunks:         []Hunk{},
		})
		bundle.Summary.TotalFiles++
		bundle.Summary.ReviewableFiles++
		bundle.Summary.Insertions += int64(item.LineCount)
	}
	bundle.Warnings = []ProtocolNotice{{
		Code:    "scan_batch",
		Message: fmt.Sprintf("deterministic scan batch %d", batchIndex),
	}}
	bundleID, err := computeBundleID(bundle)
	if err != nil {
		return nil, err
	}
	bundle.BundleID = bundleID
	encoded, err := marshalWithStableSize(bundle)
	if err != nil {
		return nil, err
	}
	if int64(len(encoded)) > maxBundleSize {
		return nil, &ProtocolError{
			Code: "bundle_too_large",
			Message: fmt.Sprintf(
				"scan batch %d is %d bytes; maximum is %d bytes",
				batchIndex,
				len(encoded),
				maxBundleSize,
			),
		}
	}
	return bundle, nil
}

func hashScanItems(items []model.ScanItem) string {
	fields := make([][]byte, 0, len(items)*2)
	for _, item := range items {
		fields = append(fields, []byte(item.Path), []byte(item.Content))
	}
	return hashFields(fields...)
}

func computeManifestID(manifest *ScanManifest) (string, error) {
	canonical := *manifest
	canonical.ManifestID = ""
	canonical.Root = ""
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal scan manifest identity: %w", err)
	}
	return hashFields(encoded), nil
}
