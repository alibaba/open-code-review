package reviewbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// LoadBundle strictly decodes one review bundle protocol document.
func LoadBundle(reader io.Reader) (*Bundle, error) {
	data, err := readLimited(reader)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	var bundle Bundle
	if err := decodeStrict(bytes.NewReader(data), &bundle); err != nil {
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
	if err := validateBundleDocument(data); err != nil {
		return nil, err
	}
	return &bundle, nil
}

// LoadComments strictly decodes one external-comments protocol document.
func LoadComments(reader io.Reader) (*Comments, error) {
	data, err := readLimited(reader)
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
	comments.sourceSHA256 = hashFields(data)
	if err := validateCommentsDocument(data); err != nil {
		return nil, err
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
	if comments.Summary.IssuesFound != len(comments.Comments) {
		return nil, fmt.Errorf(
			"invalid comments schema: summary.issues_found (%d) must equal len(comments) (%d)",
			comments.Summary.IssuesFound,
			len(comments.Comments),
		)
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
		fileLevel := false
		if rawValue, ok := comment["file_level_comment"]; ok {
			if err := json.Unmarshal(rawValue, &fileLevel); err != nil {
				return fmt.Errorf("invalid comments schema: comments[%d].file_level_comment must be a boolean", index)
			}
		}
		if fileLevel {
			var startLine, endLine int
			if rawValue, ok := comment["start_line"]; ok {
				if err := json.Unmarshal(rawValue, &startLine); err != nil {
					return fmt.Errorf("invalid comments schema: comments[%d].start_line must be an integer", index)
				}
			}
			if rawValue, ok := comment["end_line"]; ok {
				if err := json.Unmarshal(rawValue, &endLine); err != nil {
					return fmt.Errorf("invalid comments schema: comments[%d].end_line must be an integer", index)
				}
			}
			if startLine != 0 || endLine != 0 {
				return fmt.Errorf(
					"invalid comments schema: comments[%d] file_level_comment requires start_line=0 and end_line=0",
					index,
				)
			}
		}
	}
	return nil
}

// LoadScanManifest strictly decodes one full-file scan manifest.
func LoadScanManifest(reader io.Reader) (*ScanManifest, error) {
	data, err := readLimited(reader)
	if err != nil {
		return nil, fmt.Errorf("read scan manifest: %w", err)
	}
	var manifest ScanManifest
	if err := decodeStrict(bytes.NewReader(data), &manifest); err != nil {
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
		encodedBundle, err := json.Marshal(&manifest.Bundles[index])
		if err != nil {
			return nil, fmt.Errorf("marshal scan bundle %d: %w", index, err)
		}
		if err := validateBundleDocument(encodedBundle); err != nil {
			return nil, fmt.Errorf("invalid scan manifest schema: bundle %d: %w", index, err)
		}
	}
	computedID, err := computeManifestID(&manifest)
	if err != nil {
		return nil, fmt.Errorf("verify manifest_id: %w", err)
	}
	if manifest.ManifestID != computedID {
		return nil, fmt.Errorf("invalid scan manifest schema: manifest_id does not match manifest content")
	}
	if err := validateManifestDocument(data); err != nil {
		return nil, err
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
