// Package gitflic posts code review results onto GitFlic merge requests
// (gitflic.ru or a self-hosted instance) as discussions.
package gitflic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the GitFlic SaaS REST API endpoint.
const DefaultBaseURL = "https://api.gitflic.ru"

// Client is a minimal GitFlic REST API client.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient creates a Client for the given API base URL and access token.
// An empty baseURL falls back to DefaultBaseURL.
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Discussion is the payload for POST .../discussions/create.
// A general comment needs only Message. An inline (code) comment requires
// all four of NewLine, OldLine, NewPath and OldPath: if any of them is
// missing, GitFlic silently creates a general comment instead.
//
// NewLine and OldLine are pointers so that an unset value is omitted from the
// payload (nil) rather than serialized as 0: with a plain int and omitempty a
// deliberate caller-set 0 would be silently dropped, turning an intended
// inline comment into a general one.
type Discussion struct {
	Message string `json:"message"`
	NewLine *int   `json:"newLine,omitempty"`
	OldLine *int   `json:"oldLine,omitempty"`
	NewPath string `json:"newPath,omitempty"`
	OldPath string `json:"oldPath,omitempty"`
}

// CreateDiscussion posts a discussion (general or inline) on a merge request.
func (c *Client) CreateDiscussion(ctx context.Context, owner, project, mrID string, d Discussion) error {
	endpoint := fmt.Sprintf("%s/project/%s/%s/merge-request/%s/discussions/create",
		c.baseURL, url.PathEscape(owner), url.PathEscape(project), url.PathEscape(mrID))

	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("gitflic API %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}
	return nil
}
