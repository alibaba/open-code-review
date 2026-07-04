package reviewbundle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmbeddedSchemasAreStrictVersionedJSON(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wantID  string
	}{
		{name: "bundle", content: BundleSchema(), wantID: BundleSchemaVersion},
		{name: "comments", content: CommentsSchema(), wantID: CommentsSchemaVersion},
		{name: "manifest", content: ManifestSchema(), wantID: ScanManifestSchemaVersion},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal(test.content, &schema); err != nil {
				t.Fatalf("decode embedded schema: %v", err)
			}
			if got := schema["$id"]; got != "https://github.com/alibaba/open-code-review/schemas/agent-review-bundle/v1" &&
				got != "https://github.com/alibaba/open-code-review/schemas/agent-review-comments/v1" &&
				got != "https://github.com/alibaba/open-code-review/schemas/agent-review-manifest/v1" {
				t.Fatalf("$id = %v, want absolute schema URI", got)
			}
			if got := schema["additionalProperties"]; got != false {
				t.Fatalf("additionalProperties = %v, want false", got)
			}
		})
	}
}

func TestBundleJSONUsesStableProtocolFields(t *testing.T) {
	bundle := Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:bundle",
		Target: Target{
			Mode:       TargetWorkspace,
			BaseSHA:    "base",
			HeadSHA:    "head",
			DiffSHA256: "sha256:diff",
		},
		Summary: Summary{TotalFiles: 1, ReviewableFiles: 1, Insertions: 2},
		Rules: map[string]Rule{
			"rule-1": {Source: "system", Pattern: "**/*.go", Content: "Review Go."},
		},
		Files: []File{{
			Path:          "main.go",
			OldPath:       "main.go",
			Status:        "modified",
			Reviewable:    true,
			Insertions:    2,
			ContentSHA256: "sha256:content",
			RuleID:        "rule-1",
			Patch:         "@@ -1 +1,2 @@",
			Hunks:         []Hunk{{OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 2}},
		}},
		Contract: DefaultContract(),
	}

	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	text := string(encoded)
	for _, field := range []string{
		`"schema_version":"agent-review-bundle/v1"`,
		`"bundle_id":"sha256:bundle"`,
		`"diff_sha256":"sha256:diff"`,
		`"content_sha256":"sha256:content"`,
		`"line_numbers":"one_based_new_file"`,
	} {
		if !strings.Contains(text, field) {
			t.Errorf("encoded bundle missing %s: %s", field, text)
		}
	}
}
