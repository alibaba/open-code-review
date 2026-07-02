package reviewbundle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

func TestLoadCommentsRejectsUnknownFields(t *testing.T) {
	input := `{
		"schema_version":"codex-review-comments/v1",
		"bundle_id":"sha256:test",
		"summary":{"files_reviewed":0,"issues_found":0},
		"comments":[],
		"unexpected":true
	}`
	_, err := LoadComments(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadComments() error = %v, want unknown field", err)
	}
}

func TestLoadCommentsRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "summary files reviewed",
			input: `{"schema_version":"codex-review-comments/v1","bundle_id":"sha256:test","summary":{"issues_found":0},"comments":[]}`,
			want:  "summary.files_reviewed",
		},
		{
			name: "comment recommendation",
			input: `{
				"schema_version":"codex-review-comments/v1",
				"bundle_id":"sha256:test",
				"summary":{"files_reviewed":1,"issues_found":1},
				"comments":[{
					"path":"main.go","start_line":1,"end_line":1,
					"priority":"medium","category":"bug","title":"title",
					"content":"content","confidence":0.9
				}]
			}`,
			want: "comments[0].recommendation",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadComments(strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadComments() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadBundleRejectsTamperedBundleID(t *testing.T) {
	bundle := validIdentifiedBundle(t)
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	var tampered map[string]any
	if err := json.Unmarshal(encoded, &tampered); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	tampered["summary"] = map[string]any{"total_files": 999}
	encoded, err = json.Marshal(tampered)
	if err != nil {
		t.Fatalf("marshal tampered bundle: %v", err)
	}
	_, err = LoadBundle(strings.NewReader(string(encoded)))
	if err == nil || !strings.Contains(err.Error(), "bundle_id does not match") {
		t.Fatalf("LoadBundle(tampered) error = %v, want bundle_id mismatch", err)
	}
}

func TestLoadScanManifestRejectsTamperedNestedBundleID(t *testing.T) {
	bundle := validIdentifiedBundle(t)
	bundle.Summary.TotalFiles = 999
	manifest := &ScanManifest{
		SchemaVersion: ScanManifestSchemaVersion,
		TargetHash:    "sha256:target",
		BatchStrategy: "none",
		BatchSize:     1,
		Bundles:       []Bundle{*bundle},
	}
	manifestID, err := computeManifestID(manifest)
	if err != nil {
		t.Fatalf("compute manifest id: %v", err)
	}
	manifest.ManifestID = manifestID
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	_, err = LoadScanManifest(strings.NewReader(string(encoded)))
	if err == nil || !strings.Contains(err.Error(), "bundle 0 bundle_id does not match") {
		t.Fatalf("LoadScanManifest(tampered nested bundle) error = %v, want nested bundle_id mismatch", err)
	}
}

func TestValidateCommentsRejectsProtocolAndEvidenceErrors(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 4},
		Comments: []ReviewComment{
			{
				Path:           "../secret.go",
				StartLine:      1,
				EndLine:        1,
				Priority:       "high",
				Category:       "bug",
				Title:          "escape",
				Content:        "escape",
				Recommendation: "fix",
				Confidence:     1,
			},
			{
				Path:           "missing.go",
				StartLine:      1,
				EndLine:        1,
				Priority:       "medium",
				Category:       "test",
				Title:          "missing",
				Content:        "missing",
				Recommendation: "fix",
				Confidence:     0.5,
			},
			{
				Path:           "main.go",
				StartLine:      0,
				EndLine:        2,
				Priority:       "low",
				Category:       "maintainability",
				Title:          "range",
				Content:        "range",
				Recommendation: "fix",
				Confidence:     0.5,
			},
			{
				Path:           "main.go",
				StartLine:      1,
				EndLine:        1,
				Priority:       "urgent",
				Category:       "style",
				Title:          "enum",
				Content:        "enum",
				Recommendation: "fix",
				Confidence:     2,
			},
		},
	}

	result := ValidateComments(context.Background(), bundle, comments, "", gitcmd.New(1))
	if result.Valid {
		t.Fatal("ValidateComments() valid = true, want false")
	}
	assertValidationCode(t, result.Errors, "path_escape")
	assertValidationCode(t, result.Errors, "unknown_path")
	assertValidationCode(t, result.Errors, "invalid_line_range")
	assertValidationCode(t, result.Errors, "invalid_priority")
	assertValidationCode(t, result.Errors, "invalid_category")
	assertValidationCode(t, result.Errors, "invalid_confidence")
}

func TestValidateCommentsWarnsOutsideChangedHunk(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 1},
		Comments: []ReviewComment{{
			Path:           "main.go",
			StartLine:      9,
			EndLine:        9,
			Priority:       "medium",
			Category:       "bug",
			Title:          "context",
			Content:        "context",
			Recommendation: "fix",
			Confidence:     0.8,
		}},
	}

	result := ValidateComments(context.Background(), bundle, comments, "", nil)
	if !result.Valid {
		t.Fatalf("ValidateComments() errors = %#v", result.Errors)
	}
	assertValidationCode(t, result.Warnings, "outside_changed_hunk")
}

func validationBundle() *Bundle {
	return &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:bundle",
		Target:        Target{Mode: TargetRange, HeadSHA: "0123456789abcdef"},
		Files: []File{{
			Path:       "main.go",
			Reviewable: true,
			Hunks:      []Hunk{{NewStart: 3, NewCount: 2}},
		}},
		Contract: DefaultContract(),
	}
}

func validIdentifiedBundle(t *testing.T) *Bundle {
	t.Helper()
	bundle := validationBundle()
	bundle.BundleID = ""
	bundleID, err := computeBundleID(bundle)
	if err != nil {
		t.Fatalf("compute bundle id: %v", err)
	}
	bundle.BundleID = bundleID
	return bundle
}

func assertValidationCode(t *testing.T, notices []ValidationNotice, code string) {
	t.Helper()
	for _, notice := range notices {
		if notice.Code == code {
			return
		}
	}
	t.Errorf("notices = %#v, want code %q", notices, code)
}
