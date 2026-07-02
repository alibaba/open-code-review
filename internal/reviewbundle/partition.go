package reviewbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	fullOptions := options
	fullOptions.MaxBundleSize = math.MaxInt64
	full, _, err := Prepare(ctx, fullOptions)
	if err != nil {
		return nil, nil, err
	}
	manifest := &ScanManifest{
		SchemaVersion: ScanManifestSchemaVersion,
		Root:          options.RepoDir,
		TargetHash:    full.Target.DiffSHA256,
		BatchStrategy: "diff",
		BatchSize:     1,
		Summary:       full.Summary,
		SkippedFiles:  make([]ScanSkippedFile, 0),
		Bundles:       make([]Bundle, 0),
	}
	current := newPartitionPacker(full, maxBundleSize)
	for _, file := range full.Files {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		addedSize, err := current.estimateAddition(full, file)
		if err != nil {
			return nil, nil, err
		}
		if len(current.files) > 0 && current.estimatedSize+addedSize > maxBundleSize {
			previous, encoded, buildErr := buildDiffPartition(full, current.files, maxBundleSize)
			if buildErr != nil {
				return nil, nil, buildErr
			}
			if int64(len(encoded)) > maxBundleSize {
				return nil, nil, partitionSizeError(len(encoded), maxBundleSize)
			}
			manifest.Bundles = append(manifest.Bundles, *previous)
			current = newPartitionPacker(full, maxBundleSize)
		}
		current.add(file, addedSize)
		if len(current.files) == 1 && current.estimatedSize > maxBundleSize {
			_, singleEncoded, buildErr := buildDiffPartition(full, current.files, maxBundleSize)
			if buildErr != nil {
				return nil, nil, buildErr
			}
			if int64(len(singleEncoded)) > maxBundleSize {
				return nil, nil, singleFilePartitionError(file.Path, len(singleEncoded), maxBundleSize)
			}
		}
	}
	if len(current.files) > 0 {
		bundle, encoded, buildErr := buildDiffPartition(full, current.files, maxBundleSize)
		if buildErr != nil {
			return nil, nil, buildErr
		}
		if int64(len(encoded)) > maxBundleSize {
			return nil, nil, partitionSizeError(len(encoded), maxBundleSize)
		}
		manifest.Bundles = append(manifest.Bundles, *bundle)
	}
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

type partitionPacker struct {
	files         []File
	ruleIDs       map[string]struct{}
	estimatedSize int64
}

func newPartitionPacker(full *Bundle, maxBundleSize int64) *partitionPacker {
	_, encoded, err := buildDiffPartition(full, nil, maxBundleSize)
	estimated := int64(4096)
	if err == nil {
		estimated = int64(len(encoded))
	}
	return &partitionPacker{
		files:         make([]File, 0),
		ruleIDs:       make(map[string]struct{}),
		estimatedSize: estimated,
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

func partitionSizeError(size int, maximum int64) error {
	return &ProtocolError{
		Code: "bundle_too_large",
		Message: fmt.Sprintf(
			"estimated partition produced a %d-byte bundle; maximum is %d",
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
	return encoded, nil
}
