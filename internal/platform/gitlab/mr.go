package gitlab

import (
	"context"
	"fmt"
	"net/http"
)

// ListChangedFiles returns the files changed in a merge request.
// It tries /diffs first and falls back to /changes?access_raw_diffs=true on 404,
// supporting GitLab 13.12 self-hosted instances.
func (c *Client) ListChangedFiles(ctx context.Context, project string, iid int) ([]ChangedFile, error) {
	proj := escapeProject(project)
	diffsPath := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/diffs", proj, iid)

	var diffs []ChangedFile
	err := c.do(ctx, http.MethodGet, diffsPath, nil, &diffs)
	if err == nil {
		return diffs, nil
	}

	// Fallback for GitLab < 14 where /diffs may not exist.
	if httpErr, ok := err.(HTTPError); ok && httpErr.StatusCode == 404 {
		changesPath := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/changes?access_raw_diffs=true", proj, iid)
		var resp struct {
			Changes []ChangedFile `json:"changes"`
		}
		if fallbackErr := c.do(ctx, http.MethodGet, changesPath, nil, &resp); fallbackErr != nil {
			return nil, fallbackErr
		}
		return resp.Changes, nil
	}

	return nil, err
}

// ListDiscussions returns all discussions for a merge request.
// Paginates through all pages using X-Next-Page headers with per_page=100.
func (c *Client) ListDiscussions(ctx context.Context, project string, iid int) ([]Discussion, error) {
	proj := escapeProject(project)
	base := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/discussions?per_page=100", proj, iid)

	var all []Discussion
	page := "1"
	for {
		path := fmt.Sprintf("%s&page=%s", base, page)
		var pageDiscs []Discussion
		hdr, err := c.doWithResponse(ctx, http.MethodGet, path, nil, &pageDiscs)
		if err != nil {
			return nil, err
		}
		all = append(all, pageDiscs...)
		next := hdr.Get("X-Next-Page")
		if next == "" {
			break
		}
		page = next
	}
	return all, nil
}

// CreateNote creates a new note (comment) on a merge request.
func (c *Client) CreateNote(ctx context.Context, project string, iid int, body string) (*Note, error) {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/notes", proj, iid)
	var note Note
	if err := c.do(ctx, http.MethodPost, path, map[string]string{"body": body}, &note); err != nil {
		return nil, err
	}
	return &note, nil
}

// UpdateNote updates an existing note on a merge request.
func (c *Client) UpdateNote(ctx context.Context, project string, iid int, noteID int, body string) error {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/notes/%d", proj, iid, noteID)
	return c.do(ctx, http.MethodPut, path, map[string]string{"body": body}, nil)
}

// DeleteNote deletes a note from a merge request.
func (c *Client) DeleteNote(ctx context.Context, project string, iid int, noteID int) error {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/notes/%d", proj, iid, noteID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// ListDiffVersions returns the diff versions for a merge request.
// GitLab returns them newest-first; the caller should use the first element.
func (c *Client) ListDiffVersions(ctx context.Context, project string, iid int) ([]DiffVersion, error) {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/versions", proj, iid)
	var versions []DiffVersion
	if err := c.do(ctx, http.MethodGet, path, nil, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// GetMergeRequest returns a merge request by IID.
func (c *Client) GetMergeRequest(ctx context.Context, project string, iid int) (*MergeRequest, error) {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d", proj, iid)
	var mr MergeRequest
	if err := c.do(ctx, http.MethodGet, path, nil, &mr); err != nil {
		return nil, err
	}
	return &mr, nil
}

// UpdateDescription updates the description of a merge request.
func (c *Client) UpdateDescription(ctx context.Context, project string, iid int, description string) error {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d", proj, iid)
	return c.do(ctx, http.MethodPut, path, map[string]string{"description": description}, nil)
}

// Unapprove removes approval from a merge request. Best-effort: callers should handle 403.
func (c *Client) Unapprove(ctx context.Context, project string, iid int) error {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/unapprove", proj, iid)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// CreateInlineDiscussion creates an inline discussion at a specific text position.
func (c *Client) CreateInlineDiscussion(ctx context.Context, project string, iid int, body string, position Position) (*Discussion, error) {
	proj := escapeProject(project)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/discussions", proj, iid)
	payload := struct {
		Body     string   `json:"body"`
		Position Position `json:"position"`
	}{Body: body, Position: position}
	var disc Discussion
	if err := c.do(ctx, http.MethodPost, path, payload, &disc); err != nil {
		return nil, err
	}
	return &disc, nil
}
