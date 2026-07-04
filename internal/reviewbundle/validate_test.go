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
		"schema_version":"agent-review-comments/v1",
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
			input: `{"schema_version":"agent-review-comments/v1","bundle_id":"sha256:test","summary":{"issues_found":0},"comments":[]}`,
			want:  "summary.files_reviewed",
		},
		{
			name: "comment recommendation",
			input: `{
				"schema_version":"agent-review-comments/v1",
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

func TestLoadProtocolRejectsSchemaEdges(t *testing.T) {
	validBundleID := "sha256:" + strings.Repeat("a", 64)
	validComment := `{
		"schema_version":"agent-review-comments/v1",
		"bundle_id":"` + validBundleID + `",
		"summary":{"files_reviewed":1,"issues_found":1},
		"comments":[{
			"path":"main.go","start_line":1,"end_line":1,
			"priority":"medium","category":"bug","title":"title",
			"content":"content","recommendation":"fix","confidence":0.9
		}]
	}`
	tests := []struct {
		name string
		load func(string) error
		body string
		want string
	}{
		{
			name: "bundle version",
			load: func(body string) error {
				_, err := LoadBundle(strings.NewReader(body))
				return err
			},
			body: `{"schema_version":"bad","bundle_id":"sha256:test"}`,
			want: "invalid bundle schema version",
		},
		{
			name: "bundle id",
			load: func(body string) error {
				_, err := LoadBundle(strings.NewReader(body))
				return err
			},
			body: `{"schema_version":"agent-review-bundle/v1"}`,
			want: "bundle_id is required",
		},
		{
			name: "comments body",
			load: func(body string) error {
				_, err := LoadComments(strings.NewReader(body))
				return err
			},
			body: strings.Replace(validComment, `"comments":[{`, `"comments":null,"ignored":[{`, 1),
			want: "comments field is required",
		},
		{
			name: "comments summary mismatch",
			load: func(body string) error {
				_, err := LoadComments(strings.NewReader(body))
				return err
			},
			body: strings.Replace(validComment, `"issues_found":1`, `"issues_found":0`, 1),
			want: "summary.issues_found",
		},
		{
			name: "file level line range",
			load: func(body string) error {
				_, err := LoadComments(strings.NewReader(body))
				return err
			},
			body: strings.Replace(validComment, `"confidence":0.9`, `"confidence":0.9,"file_level_comment":true`, 1),
			want: "file_level_comment requires start_line=0 and end_line=0",
		},
		{
			name: "manifest version",
			load: func(body string) error {
				_, err := LoadScanManifest(strings.NewReader(body))
				return err
			},
			body: `{"schema_version":"bad","manifest_id":"sha256:test","bundles":[]}`,
			want: "invalid scan manifest schema version",
		},
		{
			name: "manifest id",
			load: func(body string) error {
				_, err := LoadScanManifest(strings.NewReader(body))
				return err
			},
			body: `{"schema_version":"agent-review-manifest/v1","bundles":[]}`,
			want: "manifest_id is required",
		},
		{
			name: "manifest bundles",
			load: func(body string) error {
				_, err := LoadScanManifest(strings.NewReader(body))
				return err
			},
			body: `{"schema_version":"agent-review-manifest/v1","manifest_id":"sha256:test"}`,
			want: "bundles is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.load(tt.body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("load error = %v, want %q", err, tt.want)
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
		SchemaVersion:   ScanManifestSchemaVersion,
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

func TestLoadScanManifestRejectsMalformedNestedBundle(t *testing.T) {
	bundle := validIdentifiedBundle(t)
	bundle.Target.Mode = ""
	bundle.Contract = Contract{}
	bundle.BundleID = ""
	bundleID, err := computeBundleID(bundle)
	if err != nil {
		t.Fatalf("compute bundle id: %v", err)
	}
	bundle.BundleID = bundleID
	manifest := &ScanManifest{
		SchemaVersion:   ScanManifestSchemaVersion,
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
	if err == nil || !strings.Contains(err.Error(), "bundle 0") {
		t.Fatalf("LoadScanManifest(malformed nested bundle) error = %v, want nested bundle schema error", err)
	}
}

func TestValidateCommentsRejectsProtocolAndEvidenceErrors(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 5},
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
			{
				Path:           "./main.go",
				StartLine:      3,
				EndLine:        3,
				Priority:       "medium",
				Category:       "bug",
				Title:          "canonical",
				Content:        "canonical",
				Recommendation: "fix",
				Confidence:     0.8,
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
	assertValidationCode(t, result.Errors, "non_canonical_path")
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

func TestValidateCommentsAcceptsUniqueExistingCodeForFileLevelComment(t *testing.T) {
	repository := t.TempDir()
	content := "package sample\n\nconst bad = 1\n"
	writeTargetFile(t, repository, "main.go", content)
	bundle := validationBundle()
	bundle.Target = Target{
		Mode:       TargetScan,
		DiffSHA256: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
	}
	bundle.Files[0].ContentSHA256 = hashFields([]byte(content))
	bundle.Files[0].Hunks = []Hunk{}
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 1},
		Comments: []ReviewComment{{
			Path:             "main.go",
			StartLine:        0,
			EndLine:          0,
			Priority:         "medium",
			Category:         "bug",
			Title:            "file level",
			Content:          "file level issue",
			Recommendation:   "fix it",
			ExistingCode:     "const bad = 1",
			Confidence:       0.9,
			FileLevelComment: true,
		}},
	}

	result := ValidateComments(context.Background(), bundle, comments, repository, gitcmd.New(1))
	if !result.Valid {
		t.Fatalf("ValidateComments() errors = %#v", result.Errors)
	}
}

func TestValidateCommentsRejectsCheapEnvelopeErrors(t *testing.T) {
	nilResult := ValidateComments(context.Background(), nil, nil, "", nil)
	if nilResult.Valid {
		t.Fatal("ValidateComments(nil, nil) valid = true, want false")
	}
	assertValidationCode(t, nilResult.Errors, "invalid_schema")

	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: "bad",
		BundleID:      "sha256:other",
		Summary:       CommentsSummary{FilesReviewed: 2, IssuesFound: 0},
		Comments: []ReviewComment{{
			Path:           "main.go",
			StartLine:      1,
			EndLine:        1,
			Priority:       "medium",
			Category:       "bug",
			Title:          "",
			Content:        "content",
			Recommendation: "fix",
			Confidence:     0.5,
		}},
	}
	result := ValidateComments(context.Background(), bundle, comments, "", nil)
	if result.Valid {
		t.Fatal("ValidateComments() valid = true, want false")
	}
	assertValidationCode(t, result.Errors, "invalid_schema")
	assertValidationCode(t, result.Errors, "bundle_id_mismatch")
	assertValidationCode(t, result.Errors, "invalid_comment")
	assertValidationCode(t, result.Errors, "invalid_summary")
}

func TestValidateCommentsRejectsMovedRangeRef(t *testing.T) {
	repository := initPrepareRepository(t)
	runTargetGit(t, repository, "checkout", "-q", "-b", "feature")
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = 1\n")
	runTargetGit(t, repository, "commit", "-am", "first change")

	bundle, _, err := Prepare(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Target:        TargetSpec{From: "master", To: "feature"},
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: DefaultMaxBundleBytes,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = 2\n")
	runTargetGit(t, repository, "commit", "-am", "second change")

	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []ReviewComment{},
	}
	result := ValidateComments(context.Background(), bundle, comments, repository, gitcmd.New(2))
	if result.Valid {
		t.Fatalf("ValidateComments() valid = true, want stale target rejection")
	}
	assertValidationCode(t, result.Errors, "stale_bundle")
}

func validationBundle() *Bundle {
	return &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:bundle",
		Target: Target{
			Mode:       TargetRange,
			HeadSHA:    "0123456789abcdef",
			DiffSHA256: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		},
		Summary: Summary{TotalFiles: 1, ReviewableFiles: 1},
		Rules: map[string]Rule{
			"rule-1": {Source: "system", Pattern: "**/*.go", Content: "Review Go."},
		},
		Files: []File{{
			Path:          "main.go",
			OldPath:       "main.go",
			Status:        "modified",
			Reviewable:    true,
			Insertions:    1,
			ContentSHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RuleID:        "rule-1",
			Patch:         "@@",
			Hunks:         []Hunk{{NewStart: 3, NewCount: 2}},
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
