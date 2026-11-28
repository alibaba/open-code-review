// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
	"github.com/alibaba/open-code-review/internal/gitcmd"
)

type Discussion struct {
	ID                   string `json:"id"`
	URL                  string `json:"url,omitempty"`
	Path                 string `json:"path,omitempty"`
	Revision             string `json:"revision,omitempty"`
	Body                 string `json:"body"`
	CreatedAt            string `json:"created_at,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
	ExpiresAt            string `json:"expires_at,omitempty"`
	ReviewID             string `json:"review_id,omitempty"`
	ReplyTo              string `json:"reply_to,omitempty"`
	PublishedAt          string `json:"published_at,omitempty"`
	PublishedRevision    string `json:"published_revision,omitempty"`
	OriginalRevision     string `json:"original_revision,omitempty"`
	StartLine            int    `json:"start_line,omitempty"`
	EndLine              int    `json:"end_line,omitempty"`
	ReportedOriginalLine int    `json:"reported_original_line,omitempty"`
	SourceQuote          string `json:"source_quote,omitempty"`
	exportPath           string
	position             discussionPosition
	remote               *discussionMerge
	landing              *DiscussionLanding
}

const maxDiscussionItems = 256

func validDiscussionID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 256 && !strings.ContainsFunc(id, unicode.IsControl)
}

func validDiscussionReviewID(id string) bool {
	if !validDiscussionID(id) || len(id) > 200 || id != strings.TrimSpace(id) || !utf8.ValidString(id) ||
		strings.ContainsAny(id, "\u2028\u2029") || discoveryCredential.MatchString(id) {
		return false
	}
	if strings.Contains(id, "://") {
		return id == DiscussionReviewID(id)
	}
	return !strings.HasPrefix(id, "//") && !strings.HasPrefix(id, `\\`)
}

func validateDiscussions(items []Discussion) error {
	if len(items) > maxDiscussionItems {
		return fmt.Errorf("discussion import exceeds %d comments", maxDiscussionItems)
	}
	undated := map[string]Discussion{}
	for i, d := range items {
		if !validDiscussionID(d.ID) {
			return fmt.Errorf("discussion %d needs a stable comment ID", i+1)
		}
		if strings.TrimSpace(d.Body) == "" || len(d.Body) > 64<<10 || !utf8.ValidString(d.Body) || strings.IndexByte(d.Body, 0) >= 0 {
			return fmt.Errorf("discussion %d needs a text body of up to 64 KiB", i+1)
		}
		if d.UpdatedAt == "" {
			if prior, exists := undated[d.ID]; exists && prior.editionKey() != d.editionKey() {
				return fmt.Errorf("discussion %d has contradictory editions without edit timestamps", i+1)
			}
			undated[d.ID] = d
		}
		for _, revision := range []string{d.Revision, d.PublishedRevision, d.OriginalRevision} {
			if revision != "" && !validOID(revision) {
				return fmt.Errorf("discussion %d has an invalid commit ID", i+1)
			}
		}
		for _, stamp := range []string{d.CreatedAt, d.UpdatedAt, d.ExpiresAt, d.PublishedAt} {
			if stamp != "" {
				if _, err := time.Parse(time.RFC3339, stamp); err != nil {
					return fmt.Errorf("discussion %d has an invalid RFC3339 timestamp", i+1)
				}
			}
		}
		created, _ := time.Parse(time.RFC3339, d.CreatedAt)
		if d.CreatedAt != "" && d.UpdatedAt != "" {
			updated, _ := time.Parse(time.RFC3339, d.UpdatedAt)
			if updated.Before(created) {
				return fmt.Errorf("discussion %d was edited before its creation time", i+1)
			}
		}
		if d.CreatedAt != "" && d.PublishedAt != "" {
			published, _ := time.Parse(time.RFC3339, d.PublishedAt)
			if published.Before(created) {
				return fmt.Errorf("discussion %d was published before its creation time", i+1)
			}
		}
		if d.ReviewID != "" && !validDiscussionReviewID(d.ReviewID) {
			return fmt.Errorf("discussion %d needs a stable review ID or canonical pull or merge request URL", i+1)
		}
		if d.ReplyTo != "" && (!validDiscussionID(d.ReplyTo) || d.ReplyTo == d.ID || discoveryCredential.MatchString(d.ReplyTo)) {
			return fmt.Errorf("discussion %d has an invalid parent comment ID", i+1)
		}
		if d.StartLine != 0 || d.EndLine != 0 || d.SourceQuote != "" {
			p := discussionPosition{revision: d.OriginalRevision, start: d.StartLine, end: d.EndLine}
			if !p.valid() || d.SourceQuote != "" && (!usableLandingQuote(d.SourceQuote) || strings.Count(d.SourceQuote, "\n") != p.end-p.start) {
				return fmt.Errorf("discussion %d has an invalid original source anchor", i+1)
			}
		}
		if d.ReportedOriginalLine != 0 && (d.ReportedOriginalLine <= d.EndLine || d.ReportedOriginalLine > maxEvidenceBytes ||
			d.StartLine != d.EndLine || d.SourceQuote == "") {
			return fmt.Errorf("discussion %d has an invalid reported original line", i+1)
		}
		if d.Path != "" && !validPath(d.Path) && !allowedext.IsSecretPath(d.Path) {
			return fmt.Errorf("discussion %d has an invalid repository path", i+1)
		}
		if d.URL != "" {
			u, err := url.Parse(d.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || len(d.URL) > 2000 {
				return fmt.Errorf("discussion %d needs a credential-free HTTPS permalink", i+1)
			}
			if review := DiscussionReviewID(d.URL); d.ReviewID != "" && review != "" && review != d.ReviewID {
				return fmt.Errorf("discussion %d permalink belongs to a different review", i+1)
			}
		}
	}
	return nil
}

func LoadDiscussions(ctx context.Context, repoDir, source string) ([]Discussion, error) {
	if strings.Contains(source, "://") {
		pr, err := parseDiscussionURL(source)
		if err != nil {
			return nil, err
		}
		if err := pr.checkRemote(ctx, repoDir); err != nil {
			return nil, err
		}
		return pr.load(ctx, runDiscussionAPI)
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("discussion export must be a regular JSON file up to 1 MiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("discussion export exceeds 1 MiB")
	}
	var comments []Discussion
	if err := decodeDiscoveryJSON(raw, &comments); err != nil || comments == nil {
		return nil, errors.New("expected a JSON array of discussions with id, body and optional provenance fields")
	}
	// A tracked export is review history, even when its filename ends in .json.
	relative, err := localDiscussionExportPath(repoDir, source)
	if err != nil {
		return nil, err
	}
	for i := range comments {
		comments[i].exportPath = relative
	}
	return comments, validateDiscussions(comments)
}

func localDiscussionExportPath(repoDir, source string) (string, error) {
	root, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		return "", err
	}
	actual, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	actual, err = filepath.Abs(actual)
	if err != nil {
		return "", err
	}
	if relative, err := filepath.Rel(root, actual); err == nil && filepath.IsLocal(relative) {
		return filepath.ToSlash(relative), nil
	}
	return "", nil
}

// WithoutDiscussionExports keeps a supplied transcript out of repository
// lookups. Its individual comments have their own effective dates and scopes.
func (t Tree) WithoutDiscussionExports(sources []string) Tree {
	excluded := map[string]bool{}
	for _, source := range sources {
		if !strings.Contains(source, "://") {
			if p, err := localDiscussionExportPath(t.Repo, source); err == nil && p != "" {
				excluded[p] = true
			}
		}
	}
	if len(excluded) > 0 {
		allow := t.AllowPath
		t.AllowPath = func(p string) bool { return !excluded[p] && (allow == nil || allow(p)) }
	}
	return t
}

type discussionPR struct {
	cli, host, project, number, link string
}

var projectPart = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func parseDiscussionURL(raw string) (discussionPR, error) {
	var pr discussionPR
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.RawPath != "" || len(raw) > 2000 {
		return pr, errors.New("expected a credential-free HTTPS pull or merge request URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	n := len(parts)
	switch {
	case n == 4 && parts[2] == "pull":
		pr.cli, pr.project = "gh", strings.Join(parts[:2], "/")
	case n >= 5 && parts[n-3] == "-" && parts[n-2] == "merge_requests":
		pr.cli, pr.project = "glab", strings.Join(parts[:n-3], "/")
	default:
		return pr, errors.New("expected a GitHub pull URL or GitLab merge request URL")
	}
	for _, part := range strings.Split(pr.project, "/") {
		if !projectPart.MatchString(part) || part == "." || part == ".." {
			return pr, errors.New("invalid project in review URL")
		}
	}
	number, err := strconv.ParseUint(parts[n-1], 10, 64)
	if err != nil || number == 0 {
		return pr, errors.New("invalid review number")
	}
	pr.host, pr.number = strings.ToLower(u.Host), strconv.FormatUint(number, 10)
	u.Host, u.Fragment, u.Path = pr.host, "", strings.TrimSuffix(u.Path, "/")
	pr.link = u.String()
	return pr, nil
}

// DiscussionReviewID returns the stable identity of a provider review URL.
func DiscussionReviewID(source string) string {
	pr, err := parseDiscussionURL(source)
	if err != nil {
		return ""
	}
	return pr.reviewID()
}

func (p discussionPR) reviewID() string {
	if p.cli == "gh" {
		return "https://" + p.host + "/" + strings.ToLower(p.project) + "/pull/" + p.number
	}
	return "https://" + p.host + "/" + p.project + "/-/merge_requests/" + p.number
}

func (p discussionPR) matchesRemote(raw string) bool {
	var host, project string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "git") {
			return false
		}
		host, project = u.Host, u.Path
		if u.Scheme == "ssh" && u.Port() == "22" {
			host = u.Hostname()
		}
	} else {
		var ok bool
		host, project, ok = strings.Cut(raw, ":")
		if !ok || strings.ContainsAny(host, "/\\") {
			return false
		}
		if _, after, hasUser := strings.Cut(host, "@"); hasUser {
			host = after
		}
	}
	project = strings.TrimSuffix(strings.Trim(project, "/"), ".git")
	return strings.EqualFold(p.host, host) && strings.EqualFold(p.project, project)
}

func (p discussionPR) checkRemote(ctx context.Context, dir string) error {
	runner, err := gitcmd.New(1).LocalForRepository(ctx, dir)
	if err != nil {
		return err
	}
	raw, err := runner.Output(ctx, dir, "config", "--local", "--null", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return fmt.Errorf("read repository remotes: %w", err)
		}
	}
	for record := range strings.SplitSeq(string(raw), "\x00") {
		if _, remote, ok := strings.Cut(record, "\n"); ok && p.matchesRemote(remote) {
			return nil
		}
	}
	return errors.New("review URL must match a configured Git remote's host and project")
}

type discussionAPI func(ctx context.Context, cli, host, endpoint string) ([]byte, error)

func runDiscussionAPI(ctx context.Context, cli, host, endpoint string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, cli, "api", "--hostname", host, "--method", "GET", endpoint)
	cmd.Dir = os.TempDir()
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s to read review discussion: %w", cli, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(out, (2<<20)+1))
	if len(raw) > 2<<20 || readErr != nil {
		_ = cmd.Process.Kill()
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if readErr != nil {
		return nil, readErr
	}
	if len(raw) > 2<<20 {
		return nil, errors.New("provider discussion page exceeds 2 MiB; import a smaller JSON export")
	}
	if err != nil {
		return nil, fmt.Errorf("%s could not read %s on %s (check authentication and read access): %w", cli, strings.SplitN(endpoint, "?", 2)[0], host, err)
	}
	return raw, nil
}

func (p discussionPR) load(ctx context.Context, api discussionAPI) ([]Discussion, error) {
	const pageSize = 25
	base := "repos/" + p.project
	routes := []string{base + "/pulls/" + p.number + "/comments", base + "/pulls/" + p.number + "/reviews", base + "/issues/" + p.number + "/comments"}
	if p.cli == "glab" {
		routes = []string{"projects/" + url.PathEscape(p.project) + "/merge_requests/" + p.number + "/discussions"}
	}
	github := githubDiscussionLoad{reviews: map[int64]githubReviewPublication{}, inlineReviews: map[string]int64{}}
	var items []Discussion
	for _, route := range routes {
		for page := 1; ; page++ {
			raw, err := api(ctx, p.cli, p.host, fmt.Sprintf("%s?per_page=%d&page=%d", route, pageSize, page))
			if err != nil {
				return nil, err
			}
			var records []json.RawMessage
			if err := json.Unmarshal(raw, &records); err != nil || records == nil {
				return nil, errors.New("review provider returned an invalid discussion page")
			}
			if page > 4 && len(records) != 0 {
				return nil, errors.New("review exceeds 100 items in a provider endpoint; import a smaller JSON export")
			}
			for _, record := range records {
				comments, err := p.comments(route, record, github)
				if err != nil {
					return nil, err
				}
				items = append(items, comments...)
				if len(items) > maxDiscussionItems {
					return nil, fmt.Errorf("review exceeds %d comments; import a smaller JSON export", maxDiscussionItems)
				}
			}
			if len(records) < pageSize {
				break
			}
		}
	}
	if p.cli == "gh" {
		items = github.attachPublications(items)
	}
	if err := validateDiscussions(items); err != nil {
		return nil, err
	}
	if slices.ContainsFunc(items, func(d Discussion) bool { return d.position.valid() }) {
		merge := p.loadMerge(ctx, api)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for i := range items {
			if items[i].position.valid() {
				items[i].remote = merge
			}
		}
	}
	return items, nil
}

type githubDiscussionRecord struct {
	ID          int64
	Body        string
	Path        string
	State       string
	HTMLURL     string `json:"html_url"`
	PullURL     string `json:"pull_request_url"`
	Review      int64  `json:"pull_request_review_id"`
	ReplyTo     int64  `json:"in_reply_to_id"`
	Revision    string `json:"commit_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	SubmittedAt string `json:"submitted_at"`
	Original    string `json:"original_commit_id"`
	Start       int    `json:"original_start_line"`
	End         int    `json:"original_line"`
	Side        string `json:"side"`
	StartSide   string `json:"start_side"`
	Hunk        string `json:"diff_hunk"`
}

type githubReviewPublication struct {
	at, revision string
	pending      bool
}

type githubDiscussionLoad struct {
	reviews       map[int64]githubReviewPublication
	inlineReviews map[string]int64
}

func (g githubDiscussionLoad) recordReview(c githubDiscussionRecord) {
	if c.ID <= 0 {
		return
	}
	publication := githubReviewPublication{pending: strings.EqualFold(c.State, "PENDING")}
	if _, err := time.Parse(time.RFC3339, c.SubmittedAt); err == nil && !publication.pending {
		publication.at = c.SubmittedAt
		if validOID(c.Revision) {
			publication.revision = c.Revision
		}
	}
	if prior, ok := g.reviews[c.ID]; ok && prior != publication {
		publication = githubReviewPublication{pending: prior.pending || publication.pending}
	}
	g.reviews[c.ID] = publication
}

func (g githubDiscussionLoad) attachPublications(items []Discussion) []Discussion {
	public := items[:0]
	for _, d := range items {
		publication := g.reviews[g.inlineReviews[d.ID]]
		if publication.pending {
			continue
		}
		if submitted, err := time.Parse(time.RFC3339, publication.at); err == nil {
			d.PublishedAt, d.PublishedRevision = publication.at, publication.revision
			// GitHub can record the comment just after its review is submitted.
			if created, err := time.Parse(time.RFC3339, d.CreatedAt); err == nil && created.After(submitted) {
				d.PublishedAt = d.CreatedAt
			}
		}
		public = append(public, d)
	}
	return public
}

func (p discussionPR) matchesGitHubRecord(c githubDiscussionRecord) bool {
	if c.HTMLURL != "" && DiscussionReviewID(c.HTMLURL) != p.reviewID() {
		return false
	}
	if c.PullURL == "" {
		return true
	}
	u, err := url.Parse(c.PullURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	host, path := p.host, u.Path
	if host == "github.com" {
		host = "api.github.com"
	} else {
		path = strings.TrimPrefix(path, "/api/v3")
	}
	return strings.EqualFold(u.Host, host) && strings.EqualFold(path, "/repos/"+p.project+"/pulls/"+p.number)
}

func (p discussionPR) githubComments(route string, record json.RawMessage, g githubDiscussionLoad) ([]Discussion, error) {
	var c githubDiscussionRecord
	if err := json.Unmarshal(record, &c); err != nil {
		return nil, fmt.Errorf("read GitHub review comment: %w", err)
	}
	fragment := "discussion_r"
	if strings.HasSuffix(route, "/issues/"+p.number+"/comments") {
		fragment = "issuecomment-"
	} else if !p.matchesGitHubRecord(c) {
		return nil, nil
	} else if strings.HasSuffix(route, "/reviews") {
		g.recordReview(c)
		if g.reviews[c.ID].pending {
			return nil, nil
		}
		fragment = "pullrequestreview-"
		// The REST review timestamp describes submission; the body can be edited later.
		c.CreatedAt = c.SubmittedAt
	}
	if strings.TrimSpace(c.Body) == "" {
		return nil, nil
	}
	if c.ID <= 0 {
		return nil, errors.New("GitHub comment lacks an immutable ID")
	}
	ref := fragment + strconv.FormatInt(c.ID, 10)
	prefix := "github:" + p.host + ":"
	d := Discussion{ID: prefix + ref, URL: p.link + "#" + ref, Path: c.Path, Revision: c.Revision, Body: c.Body, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if fragment != "discussion_r" {
		return []Discussion{d}, nil
	}
	d.ReviewID = p.reviewID()
	if c.ReplyTo > 0 && c.ReplyTo != c.ID {
		d.ReplyTo = prefix + "discussion_r" + strconv.FormatInt(c.ReplyTo, 10)
	}
	if c.Review > 0 {
		g.inlineReviews[d.ID] = c.Review
	}
	if c.Side == "RIGHT" && (c.StartSide == "" || c.StartSide == "RIGHT") {
		position, exact := githubDiscussionPosition(c.Original, c.Start, c.End, c.Hunk)
		if exact && c.UpdatedAt != "" {
			d.position = position
			if d.Revision == "" {
				d.Revision = position.revision
			}
		}
		if c.ReplyTo == 0 && position.valid() {
			d.OriginalRevision, d.StartLine, d.EndLine, d.SourceQuote = position.revision, position.start, position.end, position.hunkQuote
			if !exact {
				d.ReportedOriginalLine = c.End
			}
		}
	}
	return []Discussion{d}, nil
}

func (p discussionPR) comments(route string, record json.RawMessage, github githubDiscussionLoad) ([]Discussion, error) {
	if p.cli == "gh" {
		return p.githubComments(route, record, github)
	}
	var thread struct {
		Notes []struct {
			ID        int64
			Body      string
			System    bool
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Position  struct {
				Path     string                 `json:"new_path"`
				OldPath  string                 `json:"old_path"`
				Revision string                 `json:"head_sha"`
				NewLine  int                    `json:"new_line"`
				Type     string                 `json:"position_type"`
				Range    *gitlabDiscussionRange `json:"line_range"`
			}
		}
	}
	if err := json.Unmarshal(record, &thread); err != nil || thread.Notes == nil {
		return nil, errors.New("invalid GitLab discussion thread")
	}
	var items []Discussion
	var threadPath, revision string
	var position discussionPosition
	for _, note := range thread.Notes {
		if note.Position.Path != "" || note.Position.OldPath != "" {
			threadPath, revision = note.Position.Path, note.Position.Revision
			position = discussionPosition{}
			if note.Position.Type == "text" && note.Position.Path != "" {
				position = gitlabDiscussionPosition(revision, note.Position.NewLine, note.Position.Range)
			}
			if threadPath == "" {
				threadPath = note.Position.OldPath
			}
		}
		if note.System || strings.TrimSpace(note.Body) == "" {
			continue
		}
		if note.ID <= 0 {
			return nil, errors.New("GitLab note lacks an immutable ID")
		}
		ref := "note_" + strconv.FormatInt(note.ID, 10)
		items = append(items, Discussion{ID: "gitlab:" + p.host + ":" + ref, URL: p.link + "#" + ref, Path: threadPath, Revision: revision, position: position, Body: note.Body, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt})
	}
	return items, nil
}
