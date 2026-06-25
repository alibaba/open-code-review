package scan

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func cmt(path, content string) model.LlmComment {
	return model.LlmComment{Path: path, Content: content}
}

func TestApplyDedupGroups_MergeAndKeep(t *testing.T) {
	originals := []model.LlmComment{
		cmt("a.go", "missing nil check"),
		cmt("b.go", "missing nil check"),
		cmt("c.go", "race on shared map"),
		cmt("d.go", "missing nil check"),
	}
	raw := `{
	  "groups": [
	    {"members": ["c-0", "c-1", "c-3"], "merged_content": "missing nil check (3 files)"},
	    {"members": ["c-2"]}
	  ]
	}`
	got, ok := applyDedupGroups(raw, originals)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped comments, got %d", len(got))
	}
	if got[0].Path != "a.go" {
		t.Errorf("canonical should be members[0] = a.go, got %s", got[0].Path)
	}
	if got[0].Content != "missing nil check (3 files)" {
		t.Errorf("canonical content not merged: %q", got[0].Content)
	}
	if got[1].Path != "c.go" || got[1].Content != "race on shared map" {
		t.Errorf("singleton group should be passed through: %+v", got[1])
	}
}

func TestApplyDedupGroups_KeepCanonicalContentWhenNoMergedContent(t *testing.T) {
	originals := []model.LlmComment{
		cmt("a.go", "original A"),
		cmt("b.go", "original B"),
	}
	// Multi-member but no merged_content → keep members[0]'s original content
	raw := `{"groups": [{"members": ["c-0", "c-1"]}]}`
	got, ok := applyDedupGroups(raw, originals)
	if !ok || len(got) != 1 {
		t.Fatalf("unexpected: ok=%v len=%d", ok, len(got))
	}
	if got[0].Content != "original A" {
		t.Errorf("expected canonical's original content, got %q", got[0].Content)
	}
}

func TestApplyDedupGroups_RejectsBadShapes(t *testing.T) {
	originals := []model.LlmComment{cmt("a.go", "x"), cmt("b.go", "y")}
	cases := map[string]string{
		"empty input":   ``,
		"non-json":      `not json at all`,
		"missing id":    `{"groups": [{"members": ["c-0"]}]}`, // c-1 not covered
		"duplicate id":  `{"groups": [{"members": ["c-0", "c-0"]}, {"members": ["c-1"]}]}`,
		"unknown id":    `{"groups": [{"members": ["c-0"]}, {"members": ["c-99"]}]}`,
		"empty members": `{"groups": [{"members": []}, {"members": ["c-0", "c-1"]}]}`,
		"missing one":   `{"groups": [{"members": ["c-1"]}]}`, // c-0 missing
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := applyDedupGroups(raw, originals); ok {
				t.Errorf("expected ok=false for %s, raw=%q", name, raw)
			}
		})
	}
}

func TestApplyDedupGroups_AcceptsMarkdownFences(t *testing.T) {
	originals := []model.LlmComment{cmt("a.go", "x"), cmt("b.go", "y")}
	raw := "```json\n" + `{"groups":[{"members":["c-0","c-1"],"merged_content":"merged"}]}` + "\n```"
	got, ok := applyDedupGroups(raw, originals)
	if !ok || len(got) != 1 || got[0].Content != "merged" {
		t.Errorf("expected merged single comment, ok=%v got=%+v", ok, got)
	}
}

func TestBuildDedupCommentsJSON_IncludesIDsAndKeyFields(t *testing.T) {
	cs := []model.LlmComment{
		{Path: "a.go", Content: "first", ExistingCode: "x := nil"},
		{Path: "b.go", Content: "second"},
	}
	got := buildDedupCommentsJSON(cs)
	for _, want := range []string{
		`"id":"c-0"`,
		`"id":"c-1"`,
		`"path":"a.go"`,
		`"content":"first"`,
		`"existing_code":"x := nil"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in payload: %s", want, got)
		}
	}
	// existing_code omitted when empty
	if strings.Contains(got, `"existing_code":""`) {
		t.Errorf("empty existing_code should be omitted, got: %s", got)
	}
}
