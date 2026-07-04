package reviewbundle

import (
	"context"
	"encoding/json"
	"fmt"
)

// PreparePartitioned builds a deterministic diff manifest whose bundle parts
// each obey PrepareOptions.MaxBundleSize.
func PreparePartitioned(
	ctx context.Context,
	options PrepareOptions,
) (*ScanManifest, []byte, error) {
	maxBundleSize := options.MaxBundleSize
	if maxBundleSize <= 0 {
		maxBundleSize = DefaultMaxBundleBytes
	}
	base, err := prepareBundleCore(ctx, options)
	if err != nil {
		return nil, nil, err
	}
	manifest := &ScanManifest{
		SchemaVersion: ScanManifestSchemaVersion,
		Root:          options.RepoDir,
		TargetHash:    base.Target.DiffSHA256,
		BatchStrategy: "diff",
		BatchSize:     1,
		Summary:       base.Summary,
		SkippedFiles:  make([]ScanSkippedFile, 0),
		Bundles:       make([]Bundle, 0),
	}
	current, err := newPartitionPacker(base, maxBundleSize)
	if err != nil {
		return nil, nil, err
	}
	for _, file := range base.Files {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		addedSize, estimateErr := current.estimateAddition(base, file)
		if estimateErr != nil {
			return nil, nil, estimateErr
		}
		if len(current.files) > 0 && current.estimatedSize+addedSize > maxBundleSize {
			if err := flushDiffPartition(ctx, manifest, base, current.files, maxBundleSize); err != nil {
				return nil, nil, err
			}
			current, err = newPartitionPacker(base, maxBundleSize)
			if err != nil {
				return nil, nil, err
			}
		}
		current.add(file, addedSize)
	}
	if len(current.files) > 0 {
		if err := flushDiffPartition(ctx, manifest, base, current.files, maxBundleSize); err != nil {
			return nil, nil, err
		}
	}
	if len(manifest.Bundles) == 0 && len(manifest.SkippedFiles) == 0 {
		return nil, nil, &ProtocolError{
			Code:    "empty_target",
			Message: "no reviewable diff bundles remain after partitioning",
		}
	}
	manifest.Partial = len(manifest.SkippedFiles) > 0
	manifest.EstimatedTokens = estimateDiffManifestTokens(manifest.Bundles)
	manifestID, err := computeManifestID(manifest)
	if err != nil {
		return nil, nil, err
	}
	manifest.ManifestID = manifestID
	encoded, err := marshalManifest(manifest)
	if err != nil {
		return nil, nil, err
	}
	return manifest, encoded, nil
}

func flushDiffPartition(
	ctx context.Context,
	manifest *ScanManifest,
	base *Bundle,
	files []File,
	maxBundleSize int64,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return appendDiffPartition(ctx, manifest, base, files, maxBundleSize)
}

func appendDiffPartition(
	ctx context.Context,
	manifest *ScanManifest,
	base *Bundle,
	files []File,
	maxBundleSize int64,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	bundle, encoded, err := buildDiffPartition(base, files, maxBundleSize)
	if err == nil && int64(len(encoded)) <= maxBundleSize {
		manifest.Bundles = append(manifest.Bundles, *bundle)
		return nil
	}
	if len(files) <= 1 {
		path := ""
		size := 0
		if len(files) == 1 {
			path = files[0].Path
			removeSkippedFileFromManifestSummary(&manifest.Summary, files[0])
			if encoded != nil {
				size = len(encoded)
			}
		}
		manifest.SkippedFiles = append(manifest.SkippedFiles, ScanSkippedFile{
			Path:   path,
			Reason: "bundle_too_large",
		})
		manifest.Partial = true
		if len(files) == 1 && size > 0 {
			return nil
		}
		if err != nil {
			return err
		}
		return singleFilePartitionError(path, size, maxBundleSize)
	}
	midpoint := len(files) / 2
	if err := appendDiffPartition(ctx, manifest, base, files[:midpoint], maxBundleSize); err != nil {
		return err
	}
	return appendDiffPartition(ctx, manifest, base, files[midpoint:], maxBundleSize)
}

type partitionPacker struct {
	files         []File
	ruleIDs       map[string]struct{}
	estimatedSize int64
}

func newPartitionPacker(full *Bundle, maxBundleSize int64) (*partitionPacker, error) {
	_, encoded, err := buildDiffPartition(full, nil, maxBundleSize)
	if err != nil {
		return nil, fmt.Errorf("estimate empty diff partition: %w", err)
	}
	return &partitionPacker{
		files:         make([]File, 0),
		ruleIDs:       make(map[string]struct{}),
		estimatedSize: int64(len(encoded)),
	}, nil
}

func removeSkippedFileFromManifestSummary(summary *Summary, file File) {
	if file.Reviewable {
		if summary.ReviewableFiles > 0 {
			summary.ReviewableFiles--
		}
		summary.ExcludedFiles++
	}
	if summary.Insertions >= file.Insertions {
		summary.Insertions -= file.Insertions
	} else {
		summary.Insertions = 0
	}
	if summary.Deletions >= file.Deletions {
		summary.Deletions -= file.Deletions
	} else {
		summary.Deletions = 0
	}
}

func (packer *partitionPacker) estimateAddition(full *Bundle, file File) (int64, error) {
	encodedFile, err := json.Marshal(file)
	if err != nil {
		return 0, fmt.Errorf("marshal partition file estimate: %w", err)
	}
	estimate := int64(len(encodedFile) + 128)
	if _, seen := packer.ruleIDs[file.RuleID]; !seen {
		if rule, exists := full.Rules[file.RuleID]; exists {
			encodedRule, err := json.Marshal(rule)
			if err != nil {
				return 0, fmt.Errorf("marshal partition rule estimate: %w", err)
			}
			estimate += int64(len(encodedRule) + len(file.RuleID) + 128)
		}
	}
	return estimate, nil
}

func (packer *partitionPacker) add(file File, estimatedSize int64) {
	packer.files = append(packer.files, file)
	packer.ruleIDs[file.RuleID] = struct{}{}
	packer.estimatedSize += estimatedSize
}

func buildDiffPartition(
	full *Bundle,
	files []File,
	maxBundleSize int64,
) (*Bundle, []byte, error) {
	partition := &Bundle{
		SchemaVersion:  BundleSchemaVersion,
		Target:         full.Target,
		WorkspaceState: full.WorkspaceState,
		Rules:          make(map[string]Rule),
		Files:          append([]File(nil), files...),
		Contract:       DefaultContract(),
		Warnings: []ProtocolNotice{{
			Code: "diff_partition", Message: "deterministic large-diff partition",
		}},
	}
	partition.Contract.MaxBundleBytes = maxBundleSize
	for _, file := range files {
		partition.Summary.TotalFiles++
		partition.Summary.Insertions += file.Insertions
		partition.Summary.Deletions += file.Deletions
		if file.Reviewable {
			partition.Summary.ReviewableFiles++
		} else {
			partition.Summary.ExcludedFiles++
		}
		if rule, exists := full.Rules[file.RuleID]; exists {
			partition.Rules[file.RuleID] = rule
		}
	}
	bundleID, err := computeBundleID(partition)
	if err != nil {
		return nil, nil, err
	}
	partition.BundleID = bundleID
	encoded, err := marshalWithStableSize(partition)
	if err != nil {
		return nil, nil, err
	}
	return partition, encoded, nil
}

func singleFilePartitionError(path string, size int, maximum int64) error {
	return &ProtocolError{
		Code: "bundle_too_large",
		Message: fmt.Sprintf(
			"file %s requires a %d-byte bundle; maximum is %d",
			path,
			size,
			maximum,
		),
	}
}

func marshalManifest(manifest *ScanManifest) ([]byte, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal review manifest: %w", err)
	}
	if err := validateProtocolDocumentSize(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}
