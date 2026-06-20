package reviewstore

import (
	"strings"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestSaveListLoadResult(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project: ProjectInfo{
			ID:      "42",
			Name:    "group/project",
			RepoDir: "/repo/project",
		},
		Review: ReviewInfo{
			Mode:          "range",
			SourceBranch:  "feature/a",
			TargetBranch:  "main",
			CommentCount:  1,
			FilesReviewed: 2,
		},
		Comments: []model.LlmComment{{
			Path:      "main.go",
			Content:   "check this",
			StartLine: 10,
			EndLine:   10,
		}},
	}

	if _, err := Save(root, result); err != nil {
		t.Fatalf("Save: %v", err)
	}

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects len = %d, want 1", len(projects))
	}
	if projects[0].Project.Name != "group/project" {
		t.Fatalf("project name = %q", projects[0].Project.Name)
	}

	reviews, err := ListReviews(root, projects[0].EncodedKey, ReviewFilter{SourceBranch: "feature/a", TargetBranch: "main"})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d, want 1", len(reviews))
	}
	if reviews[0].Review.CommentCount != 1 {
		t.Fatalf("comment count = %d, want 1", reviews[0].Review.CommentCount)
	}

	loaded, err := Load(root, projects[0].EncodedKey, reviews[0].ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Comments) != 1 || loaded.Comments[0].Path != "main.go" {
		t.Fatalf("unexpected comments: %#v", loaded.Comments)
	}
}

func TestListReviewsFiltersBranches(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{ID: "p1", Name: "p1"}
	for _, source := range []string{"feature/a", "feature/b"} {
		_, err := Save(root, Result{
			CreatedAt: time.Now().UTC(),
			Project:   project,
			Review: ReviewInfo{
				SourceBranch: source,
				TargetBranch: "main",
			},
		})
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	reviews, err := ListReviews(root, ProjectKey(project), ReviewFilter{SourceBranch: "feature/a"})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d, want 1", len(reviews))
	}
	if reviews[0].Review.SourceBranch != "feature/a" {
		t.Fatalf("source = %q", reviews[0].Review.SourceBranch)
	}
}

func TestListAllReviewsFiltersProjectAndBranches(t *testing.T) {
	root := t.TempDir()
	results := []Result{
		{
			Project: ProjectInfo{ID: "42", Name: "group/project"},
			Review:  ReviewInfo{SourceBranch: "feature/a", TargetBranch: "main"},
		},
		{
			Project: ProjectInfo{ID: "43", Name: "group/other"},
			Review:  ReviewInfo{SourceBranch: "feature/a", TargetBranch: "main"},
		},
		{
			Project: ProjectInfo{ID: "42", Name: "group/project"},
			Review:  ReviewInfo{SourceBranch: "feature/b", TargetBranch: "main"},
		},
	}
	for _, result := range results {
		if _, err := Save(root, result); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	reviews, err := ListAllReviews(root, ReviewFilter{
		Project:      "group/project",
		SourceBranch: "feature/a",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("ListAllReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d, want 1", len(reviews))
	}
	if reviews[0].Project.Name != "group/project" || reviews[0].Review.SourceBranch != "feature/a" {
		t.Fatalf("unexpected review: %#v", reviews[0])
	}
}

func TestLoadRejectsUnsafePathSegments(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		project string
		review  string
	}{
		{name: "project traversal", project: "..", review: "review-id"},
		{name: "review traversal", project: "project", review: "../review-id"},
		{name: "project slash", project: "project/child", review: "review-id"},
		{name: "review slash", project: "project", review: "dir/review-id"},
		{name: "project backslash", project: `project\child`, review: "review-id"},
		{name: "review backslash", project: "project", review: `dir\review-id`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(root, tt.project, tt.review); err == nil {
				t.Fatal("Load succeeded, want invalid path error")
			}
		})
	}
}

func TestListReviewsRejectsUnsafeProjectSegment(t *testing.T) {
	root := t.TempDir()
	for _, project := range []string{"..", "../project", "project/child", `project\child`} {
		t.Run(project, func(t *testing.T) {
			if _, err := ListReviews(root, project, ReviewFilter{}); err == nil {
				t.Fatal("ListReviews succeeded, want invalid path error")
			}
		})
	}
}

func TestProjectKeyUsesReadablePathEncoding(t *testing.T) {
	tests := []struct {
		name    string
		project ProjectInfo
		want    string
	}{
		{
			name:    "project name path",
			project: ProjectInfo{Name: "group/project"},
			want:    "group-project",
		},
		{
			name:    "repo dir path",
			project: ProjectInfo{RepoDir: "/Users/kite/Desktop/my-project"},
			want:    "Users-kite-Desktop-my-project",
		},
		{
			name:    "mixed separators",
			project: ProjectInfo{Name: `group\project/service`},
			want:    "group-project-service",
		},
		{
			name:    "dotted name",
			project: ProjectInfo{Name: "my..project"},
			want:    "my..project",
		},
		{
			name:    "fallback unknown",
			project: ProjectInfo{},
			want:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectKey(tt.project); got != tt.want {
				t.Fatalf("ProjectKey() = %q, want %q", got, tt.want)
			}
			if !isSafePathSegment(ProjectKey(tt.project)) {
				t.Fatalf("ProjectKey() produced unsafe segment %q", ProjectKey(tt.project))
			}
		})
	}
}

func TestGenerateIDPropagatesRandomReadError(t *testing.T) {
	if _, err := generateIDFromReader(strings.NewReader("short")); err == nil {
		t.Fatal("generateIDFromReader succeeded, want error")
	}
}
