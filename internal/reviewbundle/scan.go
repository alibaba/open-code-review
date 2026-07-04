package reviewbundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/scan"
)

const ScanManifestSchemaVersion = "agent-review-manifest/v1"

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
	EncodedWriter    io.Writer
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
	filteredItems, oversizedSkipped := filterOversizedScanItems(items, DefaultReviewMaxTokens)
	manifest.SkippedFiles = append(manifest.SkippedFiles, oversizedSkipped...)
	included, budgetTruncated, err := filterAndBudgetScanItems(ctx, manifest, filteredItems, options)
	if err != nil {
		manifest.Partial = true
		return nil, nil, err
	}
	manifest.EstimatedTokens = estimateAgentContentTokens(included)
	manifest.Summary.ReviewableFiles = len(included)
	manifest.Summary.ExcludedFiles = manifest.Summary.TotalFiles - len(included)
	for _, item := range included {
		manifest.Summary.Insertions += int64(item.LineCount)
	}
	manifest.Partial = budgetTruncated || len(manifest.SkippedFiles) > 0 || len(included) == 0
	manifest.TargetHash = hashScanItems(included)
	if len(included) == 0 && len(manifest.SkippedFiles) == 0 {
		return nil, nil, &ProtocolError{Code: "empty_target", Message: "no reviewable scan files found"}
	}

	batches := scan.GroupBatches(
		included,
		batchStrategy,
		options.BatchSize,
	)
	for batchIndex, batch := range batches {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		if err := appendScanBundles(
			ctx,
			manifest,
			batch,
			batchIndex,
			manifest.TargetHash,
			detailResolver,
			maxBundleSize,
		); err != nil {
			return nil, nil, err
		}
		clearScanItemsContent(batch)
	}
	manifestID, err := computeManifestID(manifest)
	if err != nil {
		return nil, nil, err
	}
	manifest.ManifestID = manifestID
	if options.EncodedWriter != nil {
		if err := encodeScanManifest(manifest, options.EncodedWriter); err != nil {
			return nil, nil, err
		}
		return manifest, nil, nil
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal scan manifest: %w", err)
	}
	if err := validateProtocolDocumentSize(encoded); err != nil {
		return nil, nil, err
	}
	return manifest, encoded, nil
}

func clearScanItemsContent(items []model.ScanItem) {
	for index := range items {
		items[index].Content = ""
	}
}

func encodeScanManifest(manifest *ScanManifest, writer io.Writer) error {
	limitWriter := &protocolDocumentWriter{w: writer}
	encoder := json.NewEncoder(limitWriter)
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("marshal scan manifest: %w", err)
	}
	return limitWriter.limitError()
}

func filterAndBudgetScanItems(
	ctx context.Context,
	manifest *ScanManifest,
	items []model.ScanItem,
	options ScanOptions,
) ([]model.ScanItem, bool, error) {
	tokenEstimates := make(map[string]int64, len(items))
	if options.MaxTokenBudget > 0 {
		for _, item := range items {
			tokenEstimates[item.Path] = scan.EstimateItemTokens(item, true)
		}
		sort.SliceStable(items, func(i, j int) bool {
			left := tokenEstimates[items[i].Path]
			right := tokenEstimates[items[j].Path]
			if left != right {
				return left < right
			}
			return items[i].Path < items[j].Path
		})
	}
	included := make([]model.ScanItem, 0, len(items))
	var budgetUsed int64
	budgetTruncated := false
	for _, item := range items {
		select {
		case <-ctx.Done():
			return included, budgetTruncated || len(included) < len(items), ctx.Err()
		default:
		}
		reason := scan.ExcludeReason(item, options.FileFilter)
		if reason != model.ExcludeNone {
			manifest.SkippedFiles = append(manifest.SkippedFiles, ScanSkippedFile{
				Path: item.Path, Reason: string(reason),
			})
			continue
		}
		estimated := tokenEstimates[item.Path]
		if estimated == 0 {
			estimated = scan.EstimateItemTokens(item, true)
		}
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
	sort.Slice(included, func(i, j int) bool { return included[i].Path < included[j].Path })
	return included, budgetTruncated, nil
}

func appendScanBundles(
	ctx context.Context,
	manifest *ScanManifest,
	items []model.ScanItem,
	batchIndex int,
	targetHash string,
	resolver rules.DetailResolver,
	maxBundleSize int64,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	bundle, err := buildScanBundle(items, batchIndex, targetHash, resolver, maxBundleSize)
	if err == nil {
		manifest.Bundles = append(manifest.Bundles, *bundle)
		return nil
	}
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) || protocolError.Code != "bundle_too_large" {
		return err
	}
	if len(items) <= 1 {
		if len(items) == 1 {
			manifest.SkippedFiles = append(manifest.SkippedFiles, ScanSkippedFile{
				Path:   items[0].Path,
				Reason: "bundle_too_large",
			})
			if manifest.Summary.ReviewableFiles > 0 {
				manifest.Summary.ReviewableFiles--
			}
			manifest.Summary.ExcludedFiles++
			if manifest.Summary.Insertions >= int64(items[0].LineCount) {
				manifest.Summary.Insertions -= int64(items[0].LineCount)
			} else {
				manifest.Summary.Insertions = 0
			}
			manifest.Partial = true
			return nil
		}
		return err
	}
	midpoint := len(items) / 2
	candidate := *manifest
	candidate.SkippedFiles = append([]ScanSkippedFile(nil), manifest.SkippedFiles...)
	candidate.Bundles = append([]Bundle(nil), manifest.Bundles...)
	if err := appendScanBundles(ctx, &candidate, items[:midpoint], batchIndex, targetHash, resolver, maxBundleSize); err != nil {
		return err
	}
	if err := appendScanBundles(ctx, &candidate, items[midpoint:], batchIndex, targetHash, resolver, maxBundleSize); err != nil {
		return err
	}
	*manifest = candidate
	return nil
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
