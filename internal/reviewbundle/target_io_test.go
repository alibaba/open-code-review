package reviewbundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashScanTargetFileMatchesBundleContentSHA256(t *testing.T) {
	repository := t.TempDir()
	content := []byte("package sample\n\nfunc ScanHashTarget() {}\n")
	path := "scan.go"
	if err := os.WriteFile(filepath.Join(repository, path), content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := hashScanTargetFileAtPath(repository, path)
	if err != nil {
		t.Fatalf("hashScanTargetFileAtPath() error = %v", err)
	}
	if digest != hashFields(content) {
		t.Fatalf("hash = %q, want %q", digest, hashFields(content))
	}
}

func TestValidateCommentsDetectsStaleScanBundle(t *testing.T) {
	repository := t.TempDir()
	path := "scan.go"
	content := []byte("package sample\n\nfunc Stale() {}\n")
	if err := os.WriteFile(filepath.Join(repository, path), content, 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:scan",
		Target:        Target{Mode: TargetScan},
		Summary:       Summary{ReviewableFiles: 1},
		Files: []File{{
			Path:          path,
			Reviewable:    true,
			ContentSHA256: hashFields(content),
		}},
		Contract: DefaultContract(),
	}
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Comments:      []ReviewComment{},
		Summary:       CommentsSummary{IssuesFound: 0, FilesReviewed: 0},
	}
	result := ValidateComments(t.Context(), bundle, comments, repository, nil)
	if !result.Valid {
		t.Fatalf("ValidateComments() valid = false, errors = %+v", result.Errors)
	}
	if err := os.WriteFile(filepath.Join(repository, path), []byte("package sample\n\nfunc Changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result = ValidateComments(t.Context(), bundle, comments, repository, nil)
	if result.Valid {
		t.Fatal("ValidateComments() valid = true, want stale scan bundle")
	}
	if len(result.Errors) == 0 || result.Errors[0].Code != "stale_bundle" {
		t.Fatalf("errors = %+v, want stale_bundle", result.Errors)
	}
}
