// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/diff"
)

// DiscussionLanding preserves the original right-side review location separately
// from the accepted merge revision in Citation.Revision. Quote is source code;
// Citation.Quote remains the reviewer's original statement.
type DiscussionLanding struct {
	OriginalRevision string `json:"original_revision"`
	OriginalStart    int    `json:"original_start_line"`
	OriginalEnd      int    `json:"original_end_line"`
	Quote            string `json:"source_quote"`
	MergedAt         string `json:"merged_at"`
	LandedStart      int    `json:"landed_start_line"`
}

func (l *DiscussionLanding) value() DiscussionLanding {
	if l == nil {
		return DiscussionLanding{}
	}
	return *l
}

// An export may omit its revision or name either end of the verified squash.
// A different explicit revision or a different verified landing is evidence
// of a conflicting edition.
func matchesDiscussionOrigin(revision string, landing DiscussionLanding, copyRevision string, copyLanding *DiscussionLanding) bool {
	if landing == copyLanding.value() {
		return revision == copyRevision
	}
	return landing.OriginalRevision != "" && copyLanding == nil &&
		(copyRevision == "" || copyRevision == revision || copyRevision == landing.OriginalRevision)
}

type discussionKey struct {
	note    Discussion
	landing DiscussionLanding
}

func (d Discussion) editionKey() discussionKey {
	l := d.landing.value()
	d.remote, d.position, d.landing, d.exportPath = nil, discussionPosition{}, nil, ""
	return discussionKey{d, l}
}

func (f Fact) hasFutureLanding(at time.Time) bool {
	if at.IsZero() {
		return false
	}
	future := false
	f.eachCitation(func(c Citation) {
		if c.Landing != nil {
			merged, err := time.Parse(time.RFC3339Nano, c.Landing.MergedAt)
			edited, editErr := time.Parse(time.RFC3339Nano, c.ObservedAt)
			if err != nil || editErr != nil || merged.After(at) || edited.After(at) {
				future = true
			}
		}
	})
	return future
}

type discussionPosition struct {
	revision   string
	start, end int
	hunkQuote  string
}

func (p discussionPosition) valid() bool {
	return validOID(p.revision) && p.start > 0 && p.end >= p.start && p.end <= maxEvidenceBytes && p.end-p.start < 12
}

type gitlabRangeLine struct {
	Type    string
	NewLine int `json:"new_line"`
}

type gitlabDiscussionRange struct {
	Start, End gitlabRangeLine
}

func gitlabDiscussionPosition(revision string, line int, span *gitlabDiscussionRange) discussionPosition {
	p := discussionPosition{revision: revision, start: line, end: line}
	if span != nil {
		for _, edge := range []gitlabRangeLine{span.Start, span.End} {
			if edge.NewLine <= 0 || edge.Type != "" && edge.Type != "new" {
				return discussionPosition{}
			}
		}
		if line != 0 && line != span.End.NewLine {
			return discussionPosition{}
		}
		p.start, p.end = span.Start.NewLine, span.End.NewLine
	}
	if !p.valid() {
		return discussionPosition{}
	}
	return p
}

func usableLandingQuote(quote string) bool {
	return len(quote) <= 600 && strings.ContainsFunc(quote, unicode.IsLetter) &&
		utf8.ValidString(quote) && !strings.ContainsRune(quote, 0) && !discoveryCredential.MatchString(quote)
}

func githubDiscussionPosition(rev string, start, end int, hunk string) (discussionPosition, bool) {
	if start == 0 {
		start = end
	}
	p := discussionPosition{revision: rev, start: start, end: end}
	if !p.valid() || len(hunk) > 64<<10 {
		return discussionPosition{}, false
	}
	hunks := diff.ParseHunks(strings.TrimRight(hunk, "\r\n"))
	if len(hunks) != 1 || hunks[0].NewStart <= 0 || hunks[0].NewStart > start {
		return discussionPosition{}, false
	}
	h := hunks[0]
	n := h.NewStart
	var lines []string
	for _, line := range h.Lines {
		if line.Type == diff.HunkDeleted {
			continue
		}
		if n >= start && n <= end {
			lines = append(lines, line.Content)
		}
		n++
	}
	p.hunkQuote = strings.Join(lines, "\n")
	if len(lines) == end-start+1 && usableLandingQuote(p.hunkQuote) {
		return p, true
	}
	// GitHub can report an outdated line beyond the truncated hunk. The hunk's
	// final displayed addition still needs exact verification in the original blob.
	if start == end && n <= end && end-h.NewStart < h.NewCount && len(h.Lines) != 0 {
		last := h.Lines[len(h.Lines)-1]
		if last.Type == diff.HunkAdded && usableLandingQuote(last.Content) {
			p.start, p.end, p.hunkQuote = n-1, n-1, last.Content
			return p, false
		}
	}
	return discussionPosition{}, false
}

type discussionMerge struct {
	pr       discussionPR
	api      discussionAPI
	revision string
	mergedAt string
}

func (p discussionPR) loadMerge(ctx context.Context, api discussionAPI) *discussionMerge {
	merge := &discussionMerge{pr: p, api: api}
	if p.cli == "gh" {
		var record struct {
			Merged bool
			At     string `json:"merged_at"`
			SHA    string `json:"merge_commit_sha"`
			Number int
			Base   struct {
				Repo struct {
					Name string `json:"full_name"`
				}
			}
		}
		raw, err := api(ctx, p.cli, p.host, "repos/"+p.project+"/pulls/"+p.number)
		if err != nil || json.Unmarshal(raw, &record) != nil || !record.Merged || strconv.Itoa(record.Number) != p.number ||
			!strings.EqualFold(record.Base.Repo.Name, p.project) {
			return nil
		}
		merge.revision, merge.mergedAt = record.SHA, record.At
	} else {
		var record struct {
			IID     int
			State   string
			URL     string `json:"web_url"`
			Project int    `json:"project_id"`
			Target  int    `json:"target_project_id"`
			At      string `json:"merged_at"`
			SHA     string `json:"merge_commit_sha"`
			Squash  string `json:"squash_commit_sha"`
		}
		raw, err := api(ctx, p.cli, p.host, "projects/"+url.PathEscape(p.project)+"/merge_requests/"+p.number)
		if err != nil || json.Unmarshal(raw, &record) != nil || record.State != "merged" || strconv.Itoa(record.IID) != p.number ||
			record.Project <= 0 || record.Project != record.Target || !strings.EqualFold(record.URL, p.link) {
			return nil
		}
		merge.revision, merge.mergedAt = record.SHA, record.At
		if merge.revision == "" {
			merge.revision = record.Squash
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, merge.mergedAt); err != nil || !validOID(merge.revision) {
		return nil
	}
	return merge
}

// LandingReceipt changes only for provider-attested metadata already knowable
// at the cutoff. Local JSON cannot supply a provider response.
func (d Discussion) LandingReceipt(cutoff time.Time) string {
	if d.remote == nil || !d.position.valid() || d.UpdatedAt == "" {
		return ""
	}
	at, _ := time.Parse(time.RFC3339Nano, d.remote.mergedAt)
	if at.After(cutoff) || discussionTime(d).After(cutoff) {
		return ""
	}
	return Digest([]byte(strings.Join([]string{d.ID, d.remote.revision, d.remote.mergedAt, d.position.revision,
		strconv.Itoa(d.position.start), strconv.Itoa(d.position.end), d.position.hunkQuote}, "\x00")))
}

// uniqueLandingLine accepts a complete line range only when the reviewed
// expression still has one possible location in this exact file.
func uniqueLandingLine(data []byte, quote string) int {
	if !usableLandingQuote(quote) || len(data) > maxEvidenceBytes || bytes.IndexByte(data, 0) >= 0 {
		return 0
	}
	text := "\n" + strings.ReplaceAll(string(data), "\r\n", "\n") + "\n"
	needle := "\n" + quote + "\n"
	i := strings.Index(text, needle)
	if i < 0 || strings.Index(text[i+1:], needle) >= 0 {
		return 0
	}
	return 1 + strings.Count(text[:i], "\n")
}

func originalLines(data []byte, p discussionPosition) string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if !p.valid() || p.end > len(lines) {
		return ""
	}
	return strings.Join(lines[p.start-1:p.end], "\n")
}

type landingFiles map[string][]byte

func (files landingFiles) original(ctx context.Context, d Discussion, result *DiscoveryResult) []byte {
	merge, p := d.remote, d.position
	key := merge.pr.link + "\x00" + p.revision + "\x00" + d.Path
	if body, ok := files[key]; ok {
		return body
	}
	if len(files) >= discoveryComments {
		result.limit("sampled up to 12 immutable review files for merged discussions")
		return nil
	}
	files[key] = nil
	endpoint := "projects/" + url.PathEscape(merge.pr.project) + "/repository/files/" + url.PathEscape(d.Path) + "?ref=" + p.revision
	if merge.pr.cli == "gh" {
		parts := strings.Split(d.Path, "/")
		for i, s := range parts {
			parts[i] = url.PathEscape(s)
		}
		endpoint = "repos/" + merge.pr.project + "/contents/" + strings.Join(parts, "/") + "?ref=" + p.revision
	}
	raw, err := merge.api(ctx, merge.pr.cli, merge.pr.host, endpoint)
	if err != nil {
		result.retryLanding = true
		result.limit("could not retrieve an immutable original file for a merged discussion")
		return nil
	}
	var f struct {
		Path, Type, SHA   string
		FilePath          string `json:"file_path"`
		Commit            string `json:"commit_id"`
		Blob              string `json:"blob_id"`
		Encoding, Content string
		Size              int
	}
	if json.Unmarshal(raw, &f) != nil || f.Encoding != "base64" || f.Size <= 0 || f.Size > maxEvidenceBytes {
		return nil
	}
	blob := f.Blob
	if merge.pr.cli == "gh" {
		if f.Type != "file" || f.Path != d.Path {
			return nil
		}
		blob = f.SHA
	} else if f.FilePath != d.Path || f.Commit != p.revision {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil || len(data) != f.Size {
		return nil
	}
	// Verify the Git object ID returned with the immutable, target-project file.
	var h hash.Hash = sha1.New()
	if len(blob) == 64 {
		h = sha256.New()
	}
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	if hex.EncodeToString(h.Sum(nil)) != blob {
		return nil
	}
	files[key] = data
	return data
}

func (r *rechecker) landDiscussion(d Discussion, cutoff time.Time, files landingFiles, result *DiscoveryResult) (Discussion, bool) {
	if d.LandingReceipt(cutoff) == "" || !r.ancestor(d.remote.revision) {
		return d, false
	}
	original := r.atRevision(d.position.revision).file(d.Path)
	data := original.data
	if original.err != nil {
		data = files.original(r.ctx, d, result)
	}
	quote := originalLines(data, d.position)
	if !usableLandingQuote(quote) || d.position.hunkQuote != "" && d.position.hunkQuote != quote {
		return d, false
	}
	landed := uniqueLandingLine(r.atRevision(d.remote.revision).file(d.Path).data, quote)
	if landed == 0 || uniqueLandingLine(r.file(d.Path).data, quote) == 0 {
		return d, false
	}
	d.Revision = d.remote.revision
	d.landing = &DiscussionLanding{OriginalRevision: d.position.revision, OriginalStart: d.position.start, OriginalEnd: d.position.end,
		Quote: quote, MergedAt: d.remote.mergedAt, LandedStart: landed}
	return d, true
}

func (r *rechecker) landingVisible(c Citation) bool {
	l := c.Landing
	if l == nil {
		return true
	}
	if c.Kind != "discussion" || !validPath(c.Path) || !r.observedBy(l.MergedAt) || !r.ancestor(c.Revision) ||
		!validOID(l.OriginalRevision) || l.OriginalStart <= 0 || l.OriginalEnd-l.OriginalStart != strings.Count(l.Quote, "\n") ||
		l.LandedStart <= 0 || !usableLandingQuote(l.Quote) {
		return false
	}
	return uniqueLandingLine(r.atRevision(c.Revision).file(c.Path).data, l.Quote) == l.LandedStart &&
		uniqueLandingLine(r.file(c.Path).data, l.Quote) > 0
}
