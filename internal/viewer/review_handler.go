package viewer

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/open-code-review/open-code-review/internal/reviewstore"
)

func ReviewsRoot() (string, error) {
	return reviewstore.DefaultRoot()
}

type reviewProjectsData struct {
	Projects []reviewstore.ProjectSummary
}

func handleReviewProjects(w http.ResponseWriter, r *http.Request, root string) {
	if r.URL.Path != "/reviews" {
		http.NotFound(w, r)
		return
	}
	projects, err := reviewstore.ListProjects(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderTemplate(w, "review_projects.html", reviewProjectsData{Projects: projects})
}

type projectReviewsData struct {
	EncodedProject string
	ProjectName    string
	SourceBranch   string
	TargetBranch   string
	Reviews        []reviewstore.ReviewSummary
}

func handleProjectReviews(w http.ResponseWriter, r *http.Request, root, project string) {
	source := r.URL.Query().Get("source")
	target := r.URL.Query().Get("target")
	reviews, err := reviewstore.ListReviews(root, project, reviewstore.ReviewFilter{
		SourceBranch: source,
		TargetBranch: target,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := project
	for _, review := range reviews {
		if review.Project.Name != "" {
			name = review.Project.Name
			break
		}
	}

	renderTemplate(w, "project_reviews.html", projectReviewsData{
		EncodedProject: project,
		ProjectName:    name,
		SourceBranch:   source,
		TargetBranch:   target,
		Reviews:        reviews,
	})
}

type reviewDetailData struct {
	EncodedProject string
	ProjectName    string
	Result         *reviewstore.Result
}

func handleReviewDetail(w http.ResponseWriter, r *http.Request, root, project, reviewID string) {
	result, err := reviewstore.Load(root, project, reviewID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load review result: %v", err), http.StatusNotFound)
		return
	}
	name := result.Project.Name
	if name == "" {
		name = project
	}
	renderTemplate(w, "review_detail.html", reviewDetailData{
		EncodedProject: project,
		ProjectName:    name,
		Result:         result,
	})
}

func handleAPIReviews(w http.ResponseWriter, r *http.Request, root string) {
	if r.URL.Path != "/api/reviews" {
		http.NotFound(w, r)
		return
	}
	reviews, err := reviewstore.ListAllReviews(root, reviewstore.ReviewFilter{
		Project:      r.URL.Query().Get("project"),
		SourceBranch: r.URL.Query().Get("source"),
		TargetBranch: r.URL.Query().Get("target"),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
	})
}

func handleAPIProjectReviews(w http.ResponseWriter, r *http.Request, root, project string) {
	reviews, err := reviewstore.ListReviews(root, project, reviewstore.ReviewFilter{
		SourceBranch: r.URL.Query().Get("source"),
		TargetBranch: r.URL.Query().Get("target"),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project": project,
		"reviews": reviews,
	})
}

func handleAPIReviewDetail(w http.ResponseWriter, r *http.Request, root, project, reviewID string) {
	result, err := reviewstore.Load(root, project, reviewID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		fmt.Printf("[viewer] json encode error: %v\n", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}
