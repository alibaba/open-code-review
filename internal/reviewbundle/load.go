package reviewbundle

import (
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
	return &bundle, nil
}

// LoadComments strictly decodes one external-comments protocol document.
func LoadComments(reader io.Reader) (*Comments, error) {
	var comments Comments
	if err := decodeStrict(reader, &comments); err != nil {
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
		return nil, fmt.Errorf("invalid comments schema: comments must be an array")
	}
	return &comments, nil
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
