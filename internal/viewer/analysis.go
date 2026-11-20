// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/alibaba/open-code-review/internal/behaviortask"
)

type analysisPage struct {
	behaviortask.SavedResult
	Markdown    template.HTML
	Notice      string
	Excluded    int
	InScope     int
	Observation *behaviortask.Observation
	Requests    []*TaskCard
	HasTrace    bool
}

func (page *analysisPage) UncitedObservationIDs() []string {
	cited := make(map[string]bool, len(page.CitedObservationIDs))
	for _, id := range page.CitedObservationIDs {
		cited[id] = true
	}
	var other []string
	for _, id := range page.ObservationIDs {
		if !cited[id] {
			other = append(other, id)
		}
	}
	return other
}

var reportMarkdown = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))

var reportHTML = func() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "em", "strong", "del", "code", "pre", "blockquote", "hr", "br", "table", "thead", "tbody", "tr", "th", "td")
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowURLSchemes("https", "http")
	return policy
}()

func openAnalysis(root, repo, id string) (*os.Root, error) {
	sessions, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer sessions.Close()
	return sessions.OpenRoot(filepath.Join(repo, id, "analysis"))
}

func readAnalysis(directory *os.Root, id string) analysisPage {
	var page analysisPage
	if directory == nil {
		page.Notice = "Analysis artifacts are unavailable."
		return page
	}
	data, err := directory.ReadFile("result.json")
	if err != nil || json.Unmarshal(data, &page.SavedResult) != nil || page.SessionID != id {
		return analysisPage{Notice: "Analysis result is missing or unreadable."}
	}
	if page.Status == "started" {
		page.Notice = "No final Analysis status has been recorded."
	}
	for _, entry := range page.Scope {
		if entry.ExcludedReason != "" {
			page.Excluded++
		} else {
			page.InScope++
		}
	}
	if page.SubmissionStatus == "accepted" {
		text, err := directory.ReadFile("report.md")
		var rendered bytes.Buffer
		if err != nil || reportMarkdown.Convert(text, &rendered) != nil {
			page.Notice = "The accepted report file is missing or unreadable."
		} else {
			page.Markdown = template.HTML(reportHTML.SanitizeBytes(rendered.Bytes()))
		}
	}
	return page
}

func handleAnalysis(w http.ResponseWriter, r *http.Request, root, repo, id string) {
	repo, err := safeSegment(repo)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	id, err = safeSegment(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.OpenInRoot(root, filepath.Join(repo, id+".jsonl"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	var start map[string]any
	if err := json.NewDecoder(file).Decode(&start); err != nil || start["type"] != "session_start" {
		http.NotFound(w, r)
		return
	}
	summary := SessionSummary{SessionID: id}
	applySessionStart(&summary, start)
	if !summary.AnalysisEnabled {
		http.NotFound(w, r)
		return
	}
	directory, err := openAnalysis(root, repo, id)
	if err == nil {
		defer directory.Close()
	}
	if r.URL.Query().Get("log") == "trace" {
		if directory == nil {
			http.NotFound(w, r)
			return
		}
		file, err := directory.Open("trace.jsonl")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.Copy(w, file)
		return
	}
	page := readAnalysis(directory, id)
	if directory != nil {
		if trace, err := directory.Open("trace.jsonl"); err == nil {
			defer trace.Close()
			if conversation, err := readSession(trace, id); err == nil {
				page.HasTrace = true
				page.Requests = conversation.requests
			}
		}
	}
	if observation := r.URL.Query().Get("evidence"); observation != "" {
		if !slices.Contains(page.ObservationIDs, observation) {
			http.NotFound(w, r)
			return
		}
		page.Observation, err = readObservation(directory, observation)
		if err != nil {
			page.Notice = "The selected evidence is missing or unreadable."
		}
	}
	renderTemplate(w, "session.html", sessionPageData{EncodedRepo: repo, RepoName: filepath.Base(summary.CWD), Session: &ViewSession{Summary: summary}, Analysis: &page})
}

func readObservation(directory *os.Root, id string) (*behaviortask.Observation, error) {
	file, err := directory.Open("observations.jsonl")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	for {
		var observation behaviortask.Observation
		if err := decoder.Decode(&observation); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, os.ErrNotExist
			}
			return nil, err
		}
		if observation.ID == id {
			return &observation, nil
		}
	}
}
