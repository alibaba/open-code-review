package reviewbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// LoadBundle strictly decodes one review bundle protocol document.
func LoadBundle(reader io.Reader) (*Bundle, error) {
	var bundle Bundle
	if err := decodeStrict(reader, &bundle); err != nil {
		return nil, fmt.Errorf("invalid bundle schema: %w", err)
	}
	if bundle.SchemaVersion != BundleSchemaVersion {
		return nil, fmt.Errorf(
			"invalid bundle schema version %q, want %q",
			bundle.SchemaVersion,
			BundleSchemaVersion,
		)
	}
	if bundle.BundleID == "" {
		return nil, fmt.Errorf("invalid bundle schema: bundle_id is required")
	}
	computedID, err := computeBundleID(&bundle)
	if err != nil {
		return nil, fmt.Errorf("verify bundle_id: %w", err)
	}
	if bundle.BundleID != computedID {
		return nil, fmt.Errorf("invalid bundle schema: bundle_id does not match bundle content")
	}
	return &bundle, nil
}

// LoadComments strictly decodes one external-comments protocol document.
func LoadComments(reader io.Reader) (*Comments, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read comments: %w", err)
	}
	if err := validateCommentsShape(data); err != nil {
		return nil, err
	}
	var comments Comments
	if err := decodeStrict(bytes.NewReader(data), &comments); err != nil {
		return nil, fmt.Errorf("invalid comments schema: %w", err)
	}
	if comments.SchemaVersion != CommentsSchemaVersion {
		return nil, fmt.Errorf(
			"invalid comments schema version %q, want %q",
			comments.SchemaVersion,
			CommentsSchemaVersion,
		)
	}
	if comments.BundleID == "" {
		return nil, fmt.Errorf("invalid comments schema: bundle_id is required")
	}
	if comments.Comments == nil {
		return nil, fmt.Errorf("invalid comments schema: comments field is required and must be an array")
	}
	return &comments, nil
}

func validateCommentsShape(data []byte) error {
	var raw struct {
		Summary  map[string]json.RawMessage   `json:"summary"`
		Comments []map[string]json.RawMessage `json:"comments"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid comments schema: %w", err)
	}
	if raw.Summary == nil {
		return fmt.Errorf("invalid comments schema: summary field is required")
	}
	for _, field := range []string{"files_reviewed", "issues_found"} {
		if _, ok := raw.Summary[field]; !ok {
			return fmt.Errorf("invalid comments schema: summary.%s is required", field)
		}
	}
	if raw.Comments == nil {
		return fmt.Errorf("invalid comments schema: comments field is required and must be an array")
	}
	required := []string{
		"path", "start_line", "end_line", "priority", "category",
		"title", "content", "recommendation", "confidence",
	}
	for index, comment := range raw.Comments {
		for _, field := range required {
			if _, ok := comment[field]; !ok {
				return fmt.Errorf("invalid comments schema: comments[%d].%s is required", index, field)
			}
		}
	}
	return nil
}

// LoadScanManifest strictly decodes one full-file scan manifest.
func LoadScanManifest(reader io.Reader) (*ScanManifest, error) {
	var manifest ScanManifest
	if err := decodeStrict(reader, &manifest); err != nil {
		return nil, fmt.Errorf("invalid scan manifest schema: %w", err)
	}
	if manifest.SchemaVersion != ScanManifestSchemaVersion {
		return nil, fmt.Errorf(
			"invalid scan manifest schema version %q, want %q",
			manifest.SchemaVersion,
			ScanManifestSchemaVersion,
		)
	}
	if manifest.ManifestID == "" {
		return nil, fmt.Errorf("invalid scan manifest schema: manifest_id is required")
	}
	if manifest.Bundles == nil {
		return nil, fmt.Errorf("invalid scan manifest schema: bundles is required")
	}
	for index := range manifest.Bundles {
		computedID, err := computeBundleID(&manifest.Bundles[index])
		if err != nil {
			return nil, fmt.Errorf("verify scan bundle %d: %w", index, err)
		}
		if manifest.Bundles[index].BundleID != computedID {
			return nil, fmt.Errorf("invalid scan manifest schema: bundle %d bundle_id does not match bundle content", index)
		}
	}
	computedID, err := computeManifestID(&manifest)
	if err != nil {
		return nil, fmt.Errorf("verify manifest_id: %w", err)
	}
	if manifest.ManifestID != computedID {
		return nil, fmt.Errorf("invalid scan manifest schema: manifest_id does not match manifest content")
	}
	return &manifest, nil
}

func decodeStrict(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
