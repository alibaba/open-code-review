package reviewstore

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
)

type ProjectInfo struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	RepoDir string `json:"repo_dir,omitempty"`
	WebURL  string `json:"web_url,omitempty"`
}

type GitLabInfo struct {
	ServerURL       string `json:"server_url,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	MergeRequestIID string `json:"merge_request_iid,omitempty"`
	PipelineID      string `json:"pipeline_id,omitempty"`
	JobID           string `json:"job_id,omitempty"`
}

type ReviewInfo struct {
	Mode             string `json:"mode,omitempty"`
	SourceBranch     string `json:"source_branch,omitempty"`
	TargetBranch     string `json:"target_branch,omitempty"`
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	Commit           string `json:"commit,omitempty"`
	Model            string `json:"model,omitempty"`
	FilesReviewed    int64  `json:"files_reviewed"`
	CommentCount     int64  `json:"comments"`
	TotalTokens      int64  `json:"total_tokens"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64  `json:"cache_write_tokens,omitempty"`
	Duration         string `json:"duration,omitempty"`
	DurationSeconds  int64  `json:"duration_seconds,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
}

type Warning struct {
	File    string `json:"file,omitempty"`
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

type Result struct {
	ID        string             `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	Project   ProjectInfo        `json:"project"`
	GitLab    GitLabInfo         `json:"gitlab,omitempty"`
	Review    ReviewInfo         `json:"review"`
	Comments  []model.LlmComment `json:"comments"`
	Warnings  []Warning          `json:"warnings,omitempty"`
}

type ProjectSummary struct {
	EncodedKey  string
	Project     ProjectInfo
	ReviewCount int
	LastReview  time.Time
}

type ReviewSummary struct {
	ID        string
	CreatedAt time.Time
	Project   ProjectInfo
	GitLab    GitLabInfo
	Review    ReviewInfo
}

type reviewSummaryFile struct {
	ID        string      `json:"id"`
	CreatedAt time.Time   `json:"created_at"`
	Project   ProjectInfo `json:"project"`
	GitLab    GitLabInfo  `json:"gitlab,omitempty"`
	Review    ReviewInfo  `json:"review"`
}

type ReviewFilter struct {
	Project      string
	SourceBranch string
	TargetBranch string
}

func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "reviews"), nil
}

func Save(root string, result Result) (string, error) {
	if root == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return "", err
		}
	}
	if result.ID == "" {
		id, err := generateID()
		if err != nil {
			return "", err
		}
		result.ID = id
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	if result.Project.Name == "" {
		result.Project.Name = result.Project.RepoDir
	}
	projectKey := ProjectKey(result.Project)
	dir := filepath.Join(root, projectKey)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create review result dir: %w", err)
	}

	path := filepath.Join(dir, result.ID+".json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal review result: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return "", fmt.Errorf("write review result: %w", err)
	}
	return path, nil
}

func ListProjects(root string) ([]ProjectSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reviews dir: %w", err)
	}

	var projects []ProjectSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		reviews, err := ListReviews(root, entry.Name(), ReviewFilter{})
		if err != nil || len(reviews) == 0 {
			continue
		}
		summary := ProjectSummary{
			EncodedKey:  entry.Name(),
			Project:     reviews[0].Project,
			ReviewCount: len(reviews),
			LastReview:  reviews[0].CreatedAt,
		}
		projects = append(projects, summary)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastReview.After(projects[j].LastReview)
	})
	return projects, nil
}

func ListReviews(root, encodedProject string, filter ReviewFilter) ([]ReviewSummary, error) {
	if !isSafePathSegment(encodedProject) {
		return nil, fmt.Errorf("invalid review path")
	}
	dir := filepath.Join(root, encodedProject)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read project review dir: %w", err)
	}

	var reviews []ReviewSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		review, err := loadSummary(root, encodedProject, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			log.Printf("[ocr] warning: skip review result %s/%s: %v", encodedProject, entry.Name(), err)
			continue
		}
		if filter.SourceBranch != "" && review.Review.SourceBranch != filter.SourceBranch {
			continue
		}
		if filter.TargetBranch != "" && review.Review.TargetBranch != filter.TargetBranch {
			continue
		}
		reviews = append(reviews, review)
	}

	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].CreatedAt.After(reviews[j].CreatedAt)
	})
	return reviews, nil
}

func ListAllReviews(root string, filter ReviewFilter) ([]ReviewSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reviews dir: %w", err)
	}

	var reviews []ReviewSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectReviews, err := ListReviews(root, entry.Name(), ReviewFilter{
			SourceBranch: filter.SourceBranch,
			TargetBranch: filter.TargetBranch,
		})
		if err != nil {
			return nil, err
		}
		for _, review := range projectReviews {
			if filter.Project != "" && !matchesProjectFilter(entry.Name(), review.Project, filter.Project) {
				continue
			}
			reviews = append(reviews, review)
		}
	}

	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].CreatedAt.After(reviews[j].CreatedAt)
	})
	return reviews, nil
}

func Load(root, encodedProject, reviewID string) (*Result, error) {
	path, err := reviewResultPath(root, encodedProject, reviewID)
	if err != nil {
		return nil, fmt.Errorf("invalid review path")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open review result: %w", err)
	}
	defer f.Close()

	var result Result
	dec := json.NewDecoder(f)
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode review result: %w", err)
	}
	return &result, nil
}

func loadSummary(root, encodedProject, reviewID string) (ReviewSummary, error) {
	path, err := reviewResultPath(root, encodedProject, reviewID)
	if err != nil {
		return ReviewSummary{}, fmt.Errorf("invalid review path")
	}
	f, err := os.Open(path)
	if err != nil {
		return ReviewSummary{}, fmt.Errorf("open review result: %w", err)
	}
	defer f.Close()

	var result reviewSummaryFile
	dec := json.NewDecoder(f)
	if err := dec.Decode(&result); err != nil {
		return ReviewSummary{}, fmt.Errorf("decode review result summary: %w", err)
	}
	return ReviewSummary{
		ID:        result.ID,
		CreatedAt: result.CreatedAt,
		Project:   result.Project,
		GitLab:    result.GitLab,
		Review:    result.Review,
	}, nil
}

func ProjectKey(project ProjectInfo) string {
	key := project.ID
	if key == "" {
		key = project.Name
	}
	if key == "" {
		key = project.RepoDir
	}
	if key == "" {
		key = "unknown"
	}
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

func matchesProjectFilter(encodedProject string, project ProjectInfo, filter string) bool {
	return filter == encodedProject || filter == project.ID || filter == project.Name || filter == project.RepoDir
}

func reviewResultPath(root, encodedProject, reviewID string) (string, error) {
	if !isSafePathSegment(encodedProject) || !isSafePathSegment(reviewID) {
		return "", fmt.Errorf("invalid review path")
	}
	root = filepath.Clean(root)
	path := filepath.Clean(filepath.Join(root, encodedProject, reviewID+".json"))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid review path")
	}
	return path, nil
}

func isSafePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	if strings.Contains(segment, "..") || strings.Contains(segment, "/") || strings.Contains(segment, `\`) {
		return false
	}
	return !filepath.IsAbs(segment) && filepath.Base(segment) == segment
}

func generateID() (string, error) {
	return generateIDFromReader(rand.Reader)
}

func generateIDFromReader(reader io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(reader, b); err != nil {
		return "", fmt.Errorf("generate review id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
