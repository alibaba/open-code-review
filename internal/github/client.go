// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	apiVersion     = "2022-11-28"
	pageSize       = 100
	maxResultPages = 30
)

var ErrNoMatchingPullRequest = errors.New("no matching pull request")

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API returned status %d: %s", e.StatusCode, e.Body)
}

// Client represents a GitHub API client scoped to one repository and PR.
type Client struct {
	token      string
	repoOwner  string
	repoName   string
	prNumber   int
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new GitHub API client.
func NewClient(token, repoOwner, repoName string, prNumber int) *Client {
	baseURL := strings.TrimRight(os.Getenv("GITHUB_API_URL"), "/")
	if baseURL == "" {
		serverURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_SERVER_URL")), "/")
		if serverURL == "" || strings.EqualFold(serverURL, "https://github.com") {
			baseURL = "https://api.github.com"
		} else {
			baseURL = serverURL + "/api/v3"
		}
	}
	return &Client{
		token:      token,
		repoOwner:  repoOwner,
		repoName:   repoName,
		prNumber:   prNumber,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Comment represents a right-side pull request review comment.
type Comment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

// ReviewRequest represents a GitHub pull request review.
type ReviewRequest struct {
	Body     string    `json:"body"`
	Event    string    `json:"event"`
	Comments []Comment `json:"comments,omitempty"`
}

// ReviewResponse represents the response from creating a review.
type ReviewResponse struct {
	ID       int64  `json:"id"`
	HTMLURL  string `json:"html_url"`
	Body     string `json:"body"`
	CommitID string `json:"commit_id"`
}

// PullRequest is the PR metadata needed to verify a posting target.
type PullRequest struct {
	Number int `json:"number"`
	Head   struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// ChangedFile is the portion of GitHub's changed-file response needed to
// prove that a right-side line belongs to the PR diff.
type ChangedFile struct {
	Filename string `json:"filename"`
	Patch    string `json:"patch"`
}

// CreateReview creates a pull request review with inline comments.
func (c *Client) CreateReview(ctx context.Context, commitSHA string, review ReviewRequest) (*ReviewResponse, error) {
	type reviewPayload struct {
		CommitID string    `json:"commit_id,omitempty"`
		Body     string    `json:"body"`
		Event    string    `json:"event"`
		Comments []Comment `json:"comments,omitempty"`
	}

	comments := make([]Comment, 0, len(review.Comments))
	for _, comment := range review.Comments {
		if comment.Line > 0 {
			comments = append(comments, comment)
		}
	}
	payload := reviewPayload{
		CommitID: commitSHA,
		Body:     review.Body,
		Event:    review.Event,
		Comments: comments,
	}

	var response ReviewResponse
	err := c.doJSON(ctx, http.MethodPost, c.pullURL("reviews"), payload, &response, http.StatusOK, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// ListReviews returns all reviews currently attached to the pull request. It
// is used to reconcile an ambiguous write failure before attempting a fallback
// that could otherwise duplicate findings GitHub already accepted.
func (c *Client) ListReviews(ctx context.Context) ([]ReviewResponse, error) {
	var reviews []ReviewResponse
	for page := 1; page <= maxResultPages; page++ {
		u, err := url.Parse(c.pullURL("reviews"))
		if err != nil {
			return nil, fmt.Errorf("build reviews URL: %w", err)
		}
		query := u.Query()
		query.Set("per_page", fmt.Sprint(pageSize))
		query.Set("page", fmt.Sprint(page))
		u.RawQuery = query.Encode()

		var batch []ReviewResponse
		if err := c.doJSON(ctx, http.MethodGet, u.String(), nil, &batch, http.StatusOK); err != nil {
			return nil, err
		}
		reviews = append(reviews, batch...)
		if len(batch) < pageSize {
			return reviews, nil
		}
	}
	return nil, fmt.Errorf("pull request review list exceeds %d entries", pageSize*maxResultPages)
}

// CreateIssueComment creates a non-inline comment on the PR.
func (c *Client) CreateIssueComment(ctx context.Context, body string) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, c.repoOwner, c.repoName, c.prNumber)
	var response struct {
		HTMLURL string `json:"html_url"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]string{"body": body}, &response, http.StatusCreated); err != nil {
		return "", err
	}
	return response.HTMLURL, nil
}

// GetPRInfo gets the current pull request head and base metadata.
func (c *Client) GetPRInfo(ctx context.Context) (*PullRequest, error) {
	var response PullRequest
	if err := c.doJSON(ctx, http.MethodGet, c.pullURL(""), nil, &response, http.StatusOK); err != nil {
		return nil, err
	}
	if response.Head.SHA == "" {
		return nil, fmt.Errorf("GitHub pull request response omitted head SHA")
	}
	return &response, nil
}

// ListChangedFiles returns the complete changed-file list for the PR.
func (c *Client) ListChangedFiles(ctx context.Context) ([]ChangedFile, error) {
	var files []ChangedFile
	for page := 1; page <= maxResultPages; page++ {
		u, err := url.Parse(c.pullURL("files"))
		if err != nil {
			return nil, fmt.Errorf("build changed-files URL: %w", err)
		}
		query := u.Query()
		query.Set("per_page", fmt.Sprint(pageSize))
		query.Set("page", fmt.Sprint(page))
		u.RawQuery = query.Encode()

		var batch []ChangedFile
		if err := c.doJSON(ctx, http.MethodGet, u.String(), nil, &batch, http.StatusOK); err != nil {
			return nil, err
		}
		files = append(files, batch...)
		if len(batch) < pageSize {
			return files, nil
		}
	}
	return nil, fmt.Errorf("pull request changed-file list exceeds %d files", pageSize*maxResultPages)
}

// FindPRByHead finds the unique open PR whose base branch and head SHA match
// the exact input reviewed by the CLI. Matching by SHA works for fork PRs and
// avoids guessing a PR number from a branch name.
func FindPRByHead(ctx context.Context, token, repoOwner, repoName, baseBranch, headSHA string) (int, error) {
	if headSHA == "" {
		return 0, fmt.Errorf("reviewed head SHA is empty")
	}
	client := NewClient(token, repoOwner, repoName, 0)
	var matches []int
	for page := 1; page <= maxResultPages; page++ {
		u, err := url.Parse(fmt.Sprintf("%s/repos/%s/%s/pulls", client.baseURL, repoOwner, repoName))
		if err != nil {
			return 0, fmt.Errorf("build pull request discovery URL: %w", err)
		}
		query := u.Query()
		query.Set("state", "open")
		query.Set("base", baseBranch)
		query.Set("per_page", fmt.Sprint(pageSize))
		query.Set("page", fmt.Sprint(page))
		u.RawQuery = query.Encode()

		var pulls []PullRequest
		if err := client.doJSON(ctx, http.MethodGet, u.String(), nil, &pulls, http.StatusOK); err != nil {
			return 0, err
		}
		for _, pull := range pulls {
			if pull.Head.SHA == headSHA && pull.Base.Ref == baseBranch {
				matches = append(matches, pull.Number)
			}
		}
		if len(pulls) < pageSize {
			break
		}
		if page == maxResultPages {
			return 0, fmt.Errorf("open pull request list exceeds %d entries", pageSize*maxResultPages)
		}
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("%w for base=%s and head SHA=%s", ErrNoMatchingPullRequest, baseBranch, headSHA)
	case 1:
		return matches[0], nil
	default:
		return 0, fmt.Errorf("multiple open pull requests match base=%s and head SHA=%s", baseBranch, headSHA)
	}
}

func (c *Client) pullURL(suffix string) string {
	base := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, c.repoOwner, c.repoName, c.prNumber)
	if suffix == "" {
		return base
	}
	return base + "/" + suffix
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, requestBody, responseBody any, successCodes ...int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal GitHub request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create GitHub request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send GitHub request: %w", err)
	}
	defer resp.Body.Close()

	for _, code := range successCodes {
		if resp.StatusCode == code {
			if responseBody == nil {
				return nil
			}
			if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
				return fmt.Errorf("decode GitHub response: %w", err)
			}
			return nil
		}
	}

	errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	errorText := strings.TrimSpace(string(errorBody))
	if c.token != "" {
		errorText = strings.ReplaceAll(errorText, c.token, "[REDACTED]")
	}
	return &APIError{StatusCode: resp.StatusCode, Body: errorText}
}

// ParseRepoInfo parses repository owner and name from a Git remote URL.
func ParseRepoInfo(remoteURL string) (owner, repo string, err error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", "", fmt.Errorf("repository remote URL is empty")
	}

	var remoteHost, repoPath string
	if !strings.Contains(remoteURL, "://") {
		hostPart, pathPart, found := strings.Cut(remoteURL, ":")
		if !found || pathPart == "" {
			return "", "", fmt.Errorf("unable to parse repository info from URL: %s", remoteURL)
		}
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		remoteHost = hostPart
		repoPath = pathPart
	} else {
		u, parseErr := url.Parse(remoteURL)
		if parseErr != nil || u.Hostname() == "" {
			return "", "", fmt.Errorf("unable to parse repository info from URL: %s", remoteURL)
		}
		remoteHost = u.Hostname()
		repoPath = strings.TrimPrefix(u.Path, "/")
	}

	githubHost, hostErr := configuredGitHubHost()
	if hostErr != nil {
		return "", "", hostErr
	}
	if !strings.EqualFold(remoteHost, githubHost) {
		return "", "", fmt.Errorf("remote host %q does not match configured GitHub host %q", remoteHost, githubHost)
	}

	repoPath = strings.TrimSuffix(strings.TrimSpace(repoPath), ".git")

	parts := strings.Split(repoPath, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unable to parse repository info from URL: %s", remoteURL)
	}
	return parts[0], parts[1], nil
}

func configuredGitHubHost() (string, error) {
	serverURL := strings.TrimSpace(os.Getenv("GITHUB_SERVER_URL"))
	if serverURL == "" {
		return "github.com", nil
	}
	u, err := url.Parse(serverURL)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid GITHUB_SERVER_URL %q", serverURL)
	}
	return u.Hostname(), nil
}
