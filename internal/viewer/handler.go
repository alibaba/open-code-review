// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func handleRepos(w http.ResponseWriter, r *http.Request, root string) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	repos, err := DiscoverRepos(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	renderTemplate(w, "repos.html", map[string]any{
		"Repos": repos,
	})
}

type sessionsData struct {
	EncodedRepo string
	RepoName    string
	Sessions    []SessionSummary
}

func handleSessions(w http.ResponseWriter, r *http.Request, root, repo string) {
	summaries, err := ListSessions(root, repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Derive a display name from the first session's CWD
	name := repo
	for _, s := range summaries {
		if s.CWD != "" {
			name = filepath.Base(s.CWD)
			break
		}
	}

	renderTemplate(w, "sessions.html", sessionsData{
		EncodedRepo: repo,
		RepoName:    name,
		Sessions:    summaries,
	})
}

type sessionPageData struct {
	EncodedRepo string
	RepoName    string
	Session     *ViewSession
}

func handleSession(w http.ResponseWriter, r *http.Request, root, repo, sessionID string) {
	vs, err := LoadSession(root, repo, sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load session: %v", err), http.StatusNotFound)
		return
	}

	// Derive a display name
	name := filepath.Base(vs.Summary.CWD)
	if name == "." || name == "" {
		name = repo
	}

	renderTemplate(w, "session.html", sessionPageData{
		EncodedRepo: repo,
		RepoName:    name,
		Session:     vs,
	})
}

type compareBucket struct {
	Title    string
	Findings []model.LlmComment
}

type comparePageData struct {
	EncodedRepo string
	RepoName    string
	Before      SessionSummary
	After       SessionSummary
	// Warning is empty unless the two runs used different review modes.
	Warning string
	// Buckets holds session.Compare's four buckets in the order the CLI
	// prints them. Unlike the CLI, an empty bucket still renders (as "none"):
	// a section vanishing from a web page is indistinguishable from a broken
	// page, while "Resolved (0)" is itself the answer the reader came for.
	Buckets []compareBucket
}

// toLlmComments adapts the viewer's parsed findings to the model type
// session.Compare consumes. Path, Category and ExistingCode are load-bearing:
// they are the three inputs to Compare's finding key, so dropping any of them
// would collapse findings onto the Content fallback and mis-bucket the diff.
func toLlmComments(comments []*ReviewComment) []model.LlmComment {
	out := make([]model.LlmComment, 0, len(comments))
	for _, c := range comments {
		if c == nil {
			continue
		}
		out = append(out, model.LlmComment{
			Path:           c.FilePath,
			Content:        c.Content,
			SuggestionCode: c.SuggestionCode,
			ExistingCode:   c.ExistingCode,
			StartLine:      c.StartLine,
			EndLine:        c.EndLine,
			Category:       c.Category,
			Severity:       c.Severity,
		})
	}
	return out
}

// modeWarning mirrors the only comparability warning `ocr session compare`
// emits (cmd/opencodereview/session_cmd.go, runSessionCompare): a differing
// review mode warns, and the comparison still renders. The dash for an unset
// mode matches displayMode there and the Mode column in sessions.html.
func modeWarning(before, after SessionSummary) string {
	if before.ReviewMode == after.ReviewMode {
		return ""
	}
	dash := func(mode string) string {
		if mode == "" {
			return "-"
		}
		return mode
	}
	return fmt.Sprintf("review modes differ (%s vs %s); the two runs may not have looked at the same files",
		dash(before.ReviewMode), dash(after.ReviewMode))
}

// handleCompare renders the `ocr session compare` result for two sessions of
// the same repo. repo is the on-disk directory name, passed through from the
// URL exactly as handleSessions/handleSession pass it.
func handleCompare(w http.ResponseWriter, r *http.Request, root, repo string) {
	before := r.URL.Query().Get("before")
	after := r.URL.Query().Get("after")
	if before == "" || after == "" {
		http.Error(w, "query parameters 'before' and 'after' are required", http.StatusBadRequest)
		return
	}
	// ServeMux never inspects query values, so these ids need the same
	// rejection newMux gives the path segments: both end up in filepath.Join
	// inside LoadSession.
	for _, id := range []string{before, after} {
		if unsafeSegment(id) {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}
	}

	bv, err := LoadSession(root, repo, before)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load session: %v", err), http.StatusNotFound)
		return
	}
	av, err := LoadSession(root, repo, after)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load session: %v", err), http.StatusNotFound)
		return
	}

	// encodeRepoPath maps both separators to "-", so two distinct working
	// directories ("/home/a/b" and "/home/a-b") share one viewer directory and
	// this route can be handed a cross-repo pair. Comparing findings across
	// repositories is meaningless, so it is an error here exactly as it is in
	// the CLI (cmd/opencodereview/session_cmd.go, runSessionCompare) - unlike a
	// differing review mode, which only warns.
	if bv.Summary.CWD != av.Summary.CWD {
		http.Error(w, "sessions belong to different repositories", http.StatusBadRequest)
		return
	}

	// Same call shape as the CLI: one reviewed-path set, from the after side.
	result := session.Compare(
		toLlmComments(bv.Comments),
		toLlmComments(av.Comments),
		session.ReviewedPaths(av.Summary.RunManifest),
	)

	name := filepath.Base(av.Summary.CWD)
	if name == "." || name == "" {
		name = repo
	}

	renderTemplate(w, "compare.html", comparePageData{
		EncodedRepo: repo,
		RepoName:    name,
		Before:      bv.Summary,
		After:       av.Summary,
		Warning:     modeWarning(bv.Summary, av.Summary),
		Buckets: []compareBucket{
			{Title: "New", Findings: result.New},
			{Title: "Persisting", Findings: result.Persisting},
			{Title: "Resolved", Findings: result.Resolved},
			{Title: "Not reviewed", Findings: result.NotReviewed},
		},
	})
}
