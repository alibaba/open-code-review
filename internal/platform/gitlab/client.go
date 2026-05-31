package gitlab

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

// Client is a minimal GitLab REST client for merge request operations.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient creates a GitLab client. Empty baseURL defaults to https://gitlab.com.
func NewClient(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func escapeProject(project string) string {
	return url.PathEscape(project)
}

// doWithResponse executes an HTTP request and returns the response headers.
// It reads and closes the body internally. Callers that only need the error
// should use do instead.
func (c *Client) doWithResponse(ctx context.Context, method, path string, in, out any) (http.Header, error) {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, HTTPError{StatusCode: resp.StatusCode, Body: string(data), Path: path}
	}
	if out != nil {
		return resp.Header, json.Unmarshal(data, out)
	}
	return resp.Header, nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	_, err := c.doWithResponse(ctx, method, path, in, out)
	return err
}

// HTTPError represents a non-2xx response from the GitLab API.
type HTTPError struct {
	StatusCode int
	Body       string
	Path       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("gitlab %s returned %d: %s", e.Path, e.StatusCode, e.Body)
}
