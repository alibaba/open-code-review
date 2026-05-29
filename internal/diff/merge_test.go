package diff

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestMergeDiffs_Empty(t *testing.T) {
	result := MergeDiffs()
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMergeDiffs_SingleSet(t *testing.T) {
	input := []model.Diff{
		{OldPath: "a/foo.go", NewPath: "b/foo.go", Diff: "hunk1", Insertions: 3, Deletions: 1},
	}
	result := MergeDiffs(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(result))
	}
	if result[0].Diff != "hunk1" {
		t.Errorf("expected Diff 'hunk1', got %q", result[0].Diff)
	}
}

func TestMergeDiffs_DisjointFiles(t *testing.T) {
	set1 := []model.Diff{
		{OldPath: "a/foo.go", NewPath: "b/foo.go", Diff: "foo-change", Insertions: 2, Deletions: 1},
	}
	set2 := []model.Diff{
		{OldPath: "a/bar.go", NewPath: "b/bar.go", Diff: "bar-change", Insertions: 5, Deletions: 3},
	}
	result := MergeDiffs(set1, set2)
	if len(result) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(result))
	}
	paths := map[string]bool{result[0].NewPath: true, result[1].NewPath: true}
	if !paths["b/foo.go"] || !paths["b/bar.go"] {
		t.Errorf("expected foo.go and bar.go, got %v", result)
	}
}

func TestMergeDiffs_SameFileAcrossCommits(t *testing.T) {
	set1 := []model.Diff{
		{OldPath: "a/foo.go", NewPath: "b/foo.go", Diff: "hunk-from-commit1", Insertions: 3, Deletions: 1},
	}
	set2 := []model.Diff{
		{OldPath: "a/foo.go", NewPath: "b/foo.go", Diff: "hunk-from-commit2", Insertions: 5, Deletions: 2},
	}
	result := MergeDiffs(set1, set2)
	if len(result) != 1 {
		t.Fatalf("expected 1 merged diff, got %d", len(result))
	}
	d := result[0]
	if d.Insertions != 8 {
		t.Errorf("expected 8 insertions, got %d", d.Insertions)
	}
	if d.Deletions != 3 {
		t.Errorf("expected 3 deletions, got %d", d.Deletions)
	}
	expected := "hunk-from-commit1\n\nhunk-from-commit2"
	if d.Diff != expected {
		t.Errorf("expected merged diff %q, got %q", expected, d.Diff)
	}
}

func TestMergeDiffs_NewThenModify(t *testing.T) {
	set1 := []model.Diff{
		{OldPath: "/dev/null", NewPath: "b/new.go", Diff: "add-file", Insertions: 10, Deletions: 0, IsNew: true},
	}
	set2 := []model.Diff{
		{OldPath: "a/new.go", NewPath: "b/new.go", Diff: "modify-file", Insertions: 2, Deletions: 1},
	}
	result := MergeDiffs(set1, set2)
	if len(result) != 1 {
		t.Fatalf("expected 1 merged diff, got %d", len(result))
	}
	d := result[0]
	if !d.IsNew {
		t.Error("expected IsNew=true from first commit")
	}
	if d.Insertions != 12 {
		t.Errorf("expected 12 insertions, got %d", d.Insertions)
	}
	if d.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", d.Deletions)
	}
}

func TestMergeDiffs_DeletedFile(t *testing.T) {
	set1 := []model.Diff{
		{OldPath: "a/old.go", NewPath: "/dev/null", Diff: "delete-file", Insertions: 0, Deletions: 5, IsDeleted: true},
	}
	result := MergeDiffs(set1)
	if len(result) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(result))
	}
	if !result[0].IsDeleted {
		t.Error("expected IsDeleted=true")
	}
}

func TestMergeDiffs_BinaryFile(t *testing.T) {
	set1 := []model.Diff{
		{OldPath: "a/image.png", NewPath: "b/image.png", Diff: "", IsBinary: true},
	}
	result := MergeDiffs(set1)
	if len(result) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(result))
	}
	if !result[0].IsBinary {
		t.Error("expected IsBinary=true")
	}
}

func TestMergeDiffs_Mixed(t *testing.T) {
	set1 := []model.Diff{
		{OldPath: "a/foo.go", NewPath: "b/foo.go", Diff: "foo-c1", Insertions: 1, Deletions: 0},
		{OldPath: "/dev/null", NewPath: "b/new.go", Diff: "new-c1", Insertions: 10, Deletions: 0, IsNew: true},
	}
	set2 := []model.Diff{
		{OldPath: "a/foo.go", NewPath: "b/foo.go", Diff: "foo-c2", Insertions: 2, Deletions: 1},
		{OldPath: "a/bar.go", NewPath: "b/bar.go", Diff: "bar-c2", Insertions: 3, Deletions: 0},
	}
	result := MergeDiffs(set1, set2)
	if len(result) != 3 {
		t.Fatalf("expected 3 diffs, got %d", len(result))
	}

	// Find foo.go (merged across commits)
	var fooDiff *model.Diff
	for i := range result {
		if result[i].NewPath == "b/foo.go" {
			fooDiff = &result[i]
			break
		}
	}
	if fooDiff == nil {
		t.Fatal("foo.go not found in result")
	}
	if fooDiff.Insertions != 3 {
		t.Errorf("expected 3 insertions for foo.go, got %d", fooDiff.Insertions)
	}
	if fooDiff.Deletions != 1 {
		t.Errorf("expected 1 deletion for foo.go, got %d", fooDiff.Deletions)
	}
}
