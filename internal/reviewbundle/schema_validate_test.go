package reviewbundle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateBundleDocumentAcceptsDeletedFileWithoutContentHash(t *testing.T) {
	document := []byte(`{
		"schema_version":"agent-review-bundle/v1",
		"bundle_id":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"target":{"mode":"workspace","from":"","to":"","commit":"","base_sha":"base","head_sha":"head","merge_base_sha":"","diff_sha256":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		"summary":{"total_files":1,"reviewable_files":0,"excluded_files":1,"insertions":0,"deletions":1},
		"rules":{"rule-1":{"source":"system","pattern":"**/*.go","content":"rule"}},
		"files":[{"path":"old.go","old_path":"old.go","status":"deleted","reviewable":false,"exclude_reason":"deleted","insertions":0,"deletions":1,"content_sha256":"","rule_id":"rule-1","patch":"diff","hunks":[]}],
		"contract":{"comment_schema":"agent-review-comments/v1","line_numbers":"one_based_new_file","allowed_priorities":["high","medium","low"],"allowed_categories":["bug","security","performance","concurrency","maintainability","test"],"max_bundle_bytes":4194304,"bundle_size_bytes":0,"requires_reflection":true}
	}`)
	if err := validateBundleDocument(document); err != nil {
		t.Fatalf("validateBundleDocument() error = %v", err)
	}
}

func TestValidateCommentsDocumentRejectsInvalidFileLevelRange(t *testing.T) {
	document := []byte(`{
		"schema_version":"agent-review-comments/v1",
		"bundle_id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"summary":{"files_reviewed":1,"issues_found":1},
		"comments":[{"path":"main.go","start_line":1,"end_line":1,"file_level_comment":true,"priority":"high","category":"bug","title":"t","content":"c","recommendation":"r","confidence":1}]
	}`)
	err := validateCommentsDocument(document)
	if err == nil || !strings.Contains(err.Error(), "_line") {
		t.Fatalf("validateCommentsDocument() error = %v, want file-level line rule", err)
	}
}

func TestValidateCommentsDocumentAcceptsLineCommentWithoutFileLevelFlag(t *testing.T) {
	document := []byte(`{
		"schema_version":"agent-review-comments/v1",
		"bundle_id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"summary":{"files_reviewed":1,"issues_found":1},
		"comments":[{"path":"main.go","start_line":1,"end_line":1,"priority":"high","category":"bug","title":"t","content":"c","recommendation":"r","confidence":1}]
	}`)
	if err := validateCommentsDocument(document); err != nil {
		t.Fatalf("validateCommentsDocument() error = %v, want line comment accepted", err)
	}
}

func TestPreparedBundlePassesEmbeddedSchema(t *testing.T) {
	bundle := validationBundle()
	bundle.BundleID = ""
	bundleID, err := computeBundleID(bundle)
	if err != nil {
		t.Fatalf("computeBundleID() error = %v", err)
	}
	bundle.BundleID = bundleID
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	if err := validateBundleDocument(encoded); err != nil {
		t.Fatalf("validateBundleDocument() error = %v", err)
	}
}

func TestPreparedManifestPassesEmbeddedSchema(t *testing.T) {
	bundle := validIdentifiedBundle(t)
	manifest := &ScanManifest{
		SchemaVersion:   ScanManifestSchemaVersion,
		ManifestID:      "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Root:            "/tmp/repo",
		TargetHash:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		BatchStrategy:   "none",
		BatchSize:       1,
		EstimatedTokens: 0,
		Summary:         bundle.Summary,
		Partial:         false,
		SkippedFiles:    []ScanSkippedFile{},
		Bundles:         []Bundle{*bundle},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := validateManifestDocument(encoded); err != nil {
		t.Fatalf("validateManifestDocument() error = %v", err)
	}
}
