// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"path"
	"slices"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/diff"
)

// ReviewReply preserves comment identity separately from a mutable inline
// position. Fact.Revision is the commit the provider reports for submission.
type ReviewReply struct {
	ID                   string `json:"id"`
	ReplyTo              string `json:"reply_to,omitempty"`
	URL                  string `json:"url,omitempty"`
	PublishedAt          string `json:"published_at"`
	EditedAt             string `json:"edited_at"`
	OriginalRevision     string `json:"original_revision,omitempty"`
	StartLine            int    `json:"start_line,omitempty"`
	EndLine              int    `json:"end_line,omitempty"`
	ReportedOriginalLine int    `json:"reported_original_line,omitempty"`
	SourceQuote          string `json:"source_quote,omitempty"`
	// A scope-limited import knows the note left that scope without
	// authorizing storage of the new path or text.
	OmittedScopeDigest string `json:"omitted_scope_digest,omitempty"`
	// Populated only after matching the original comment in this review.
	OriginalURL        string             `json:"original_url,omitempty"`
	SubmittedStartLine int                `json:"submitted_start_line,omitempty"`
	SubmittedEndLine   int                `json:"submitted_end_line,omitempty"`
	EarlierNotes       string             `json:"earlier_notes,omitempty"`
	LaterNotes         string             `json:"later_notes,omitempty"`
	Change             *ReviewReplyChange `json:"changed_code,omitempty"`
}

// ReviewReplyChange describes the implementation around the surviving
// operation at submission and in the proposed code. It is computed on read.
type ReviewReplyChange struct {
	Submitted Citation          `json:"submitted_function"`
	Current   Citation          `json:"current_function"`
	Hunks     []ReviewReplyHunk `json:"changed_lines"`
}

type ReviewReplyHunk struct {
	Submitted     Citation `json:"submitted"`
	Current       Citation `json:"current"`
	LayoutChanged bool     `json:"layout_changed,omitempty"`
}

func reviewNotes(opts DiscoverOptions, cutoff time.Time) []Fact {
	var notes []Fact
	for _, d := range opts.Discussions {
		if d.ReviewID == "" || d.PublishedAt == "" || d.UpdatedAt == "" || !validOID(d.PublishedRevision) ||
			discussionTime(d).After(cutoff) ||
			discoveryCredential.MatchString(d.ID) || discoveryCredential.MatchString(d.ReviewID) {
			continue
		}
		n := &ReviewReply{ID: d.ID, ReplyTo: d.ReplyTo, PublishedAt: d.PublishedAt, EditedAt: d.UpdatedAt}
		f := Fact{ReviewID: d.ReviewID, Revision: d.PublishedRevision, Category: CategoryDecision, Origin: "review-reply",
			Authority: AuthorityHistorical, ObservedAt: discussionTime(d).UTC().Format(time.RFC3339Nano), ExpiresAt: d.ExpiresAt, ReviewReply: n}
		// Withhold the text of an ineligible edition. Its date still retires
		// the older version of the same note in this PR.
		if !inDiscoveryScope(d.Path, opts.Scope) {
			n.OmittedScopeDigest = Digest([]byte(opts.Scope))
		} else if opts.allowed(d.Path) && discoveryKind(d.Path) != "documentation" && !discoveryCredential.MatchString(d.Body) &&
			!discoveryCredential.MatchString(d.URL) && !discoveryCredential.MatchString(d.SourceQuote) {
			f.Source, f.Scope, f.DiscussionBody = d.Path, d.Path, strings.ReplaceAll(d.Body, "\r\n", "\n")
			n.URL, n.OriginalRevision, n.StartLine, n.EndLine, n.SourceQuote = d.URL, d.OriginalRevision, d.StartLine, d.EndLine, d.SourceQuote
			n.ReportedOriginalLine = d.ReportedOriginalLine
		}
		notes = append(notes, f)
	}
	return notes
}

type reviewNoteKey struct{ review, id string }

func reviewNoteID(review, id string) string {
	return Digest([]byte("review-note\x00" + review + "\x00" + id))
}

// Draft edits can share a single public submission time. The latest edit
// determines which text first became visible when the review was submitted.
type reviewNoteTime struct {
	visible, edited time.Time
}

func (stamp reviewNoteTime) compare(other reviewNoteTime) int {
	if order := stamp.visible.Compare(other.visible); order != 0 {
		return order
	}
	return stamp.edited.Compare(other.edited)
}

func omittedReviewNote(scopes map[string]reviewNoteTime, file string, stamp reviewNoteTime) bool {
	for p := file; len(scopes) > 0 && validPath(p); p = path.Dir(p) {
		if omitted, ok := scopes[Digest([]byte(p))]; ok && omitted.compare(stamp) >= 0 {
			return true
		}
	}
	return false
}

// Rebuild the conversation from the latest visible edition of each comment.
// Root and reply can be imported separately, or refreshed in either order.
func (r *rechecker) joinReviewReplies(facts []Fact) []Fact {
	if !slices.ContainsFunc(facts, func(f Fact) bool { return f.ReviewReply != nil }) {
		return facts
	}
	type edition struct {
		fact      Fact
		stamp     reviewNoteTime
		ambiguous bool
	}
	latest := map[reviewNoteKey]edition{}
	omissions := map[reviewNoteKey]map[string]reviewNoteTime{}
	out := make([]Fact, 0, len(facts))
	for _, f := range facts {
		n := f.ReviewReply
		if n == nil {
			out = append(out, f)
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, f.ObservedAt)
		if err != nil || at.After(r.now) || !r.ancestor(f.Revision) || !r.observedBy(n.PublishedAt) {
			continue
		}
		key := reviewNoteKey{f.ReviewID, n.ID}
		edited, _ := time.Parse(time.RFC3339Nano, n.EditedAt)
		stamp := reviewNoteTime{at, edited}
		if scope := n.OmittedScopeDigest; scope != "" {
			if omissions[key] == nil {
				omissions[key] = map[string]reviewNoteTime{}
			}
			if prior, ok := omissions[key][scope]; !ok || stamp.compare(prior) > 0 {
				omissions[key][scope] = stamp
			}
			continue
		}
		old, exists := latest[key]
		order := stamp.compare(old.stamp)
		switch {
		case !exists || order > 0:
			latest[key] = edition{fact: f, stamp: stamp}
		case order == 0:
			old.ambiguous = old.ambiguous || f.Revision != old.fact.Revision || f.Scope != old.fact.Scope ||
				f.DiscussionBody != old.fact.DiscussionBody || *n != *old.fact.ReviewReply
			old.fact.ExpiresAt = earlierExpiry(old.fact.ExpiresAt, f.ExpiresAt)
			latest[key] = old
		}
	}
	roots := map[reviewNoteKey]Fact{}
	threads := map[reviewNoteKey][]Fact{}
	for k, e := range latest {
		f, n := e.fact, e.fact.ReviewReply
		if e.ambiguous || f.DiscussionBody == "" || !r.tree.permits(f.Scope) || omittedReviewNote(omissions[k], f.Scope, e.stamp) ||
			f.ExpiresAt != "" && r.observedBy(f.ExpiresAt) {
			continue
		}
		if n.ReplyTo == "" {
			roots[k] = f
		} else if n.ReplyTo != n.ID {
			k.id = n.ReplyTo
			threads[k] = append(threads[k], f)
		}
	}
	var joined []Fact
	for k, replies := range threads {
		root, ok := roots[k]
		if !ok || !validOID(root.ReviewReply.OriginalRevision) || !usableLandingQuote(root.ReviewReply.SourceQuote) ||
			!r.atRevision(root.Revision).ancestor(root.ReviewReply.OriginalRevision) {
			continue
		}
		replies = slices.DeleteFunc(replies, func(f Fact) bool {
			published, _ := time.Parse(time.RFC3339Nano, f.ReviewReply.PublishedAt)
			return f.Scope != root.Scope || latest[k].stamp.visible.After(published) || !r.atRevision(f.Revision).ancestor(root.Revision)
		})
		slices.SortFunc(replies, func(a, b Fact) int {
			pa, _ := time.Parse(time.RFC3339Nano, a.ReviewReply.PublishedAt)
			pb, _ := time.Parse(time.RFC3339Nano, b.ReviewReply.PublishedAt)
			if cmp := pa.Compare(pb); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.ReviewReply.ID, b.ReviewReply.ID)
		})
		for i, f := range replies {
			n := *f.ReviewReply
			n.OriginalURL, n.OriginalRevision = root.ReviewReply.URL, root.ReviewReply.OriginalRevision
			n.StartLine, n.EndLine, n.SourceQuote = root.ReviewReply.StartLine, root.ReviewReply.EndLine, root.ReviewReply.SourceQuote
			n.ReportedOriginalLine = root.ReviewReply.ReportedOriginalLine
			n.EarlierNotes = r.reviewConversation(replies[max(0, i-2):i])
			n.LaterNotes = r.reviewConversation(replies[i+1 : min(i+3, len(replies))])
			f.ReviewReply, f.Finding, f.Reason = &n, reviewFinding(root.DiscussionBody), f.DiscussionBody
			f.DiscussionBody = ""
			joined = append(joined, f)
		}
	}
	slices.SortFunc(joined, func(a, b Fact) int { return strings.Compare(a.ID, b.ID) })
	return append(out, joined...)
}

// The neighboring turns supply context only. Each retains its own submission
// revision and observation time, even if another reviewer later acknowledges it.
func (r *rechecker) reviewConversation(notes []Fact) string {
	var lines []string
	for _, f := range notes {
		if slices.ContainsFunc(f.Feedback, func(decision FeedbackRecord) bool {
			return (decision.Action == FeedbackReject || decision.Action == FeedbackReplace) &&
				r.observedBy(decision.RecordedAt) && r.ancestor(decision.Revision)
		}) {
			continue
		}
		lines = append(lines, f.ReviewReply.ID+" at "+f.Revision+" ("+f.ObservedAt+"): "+shortText(f.DiscussionBody, 260))
	}
	return strings.Join(lines, "\n")
}

// OCR's publisher prepends a run marker and a severity badge to the actual
// finding. Retain the substance, including any second recommendation.
func reviewFinding(body string) string {
	first, rest, ok := strings.Cut(body, "\n")
	if ok && strings.HasPrefix(first, "<!-- ocr-") && strings.HasSuffix(first, " -->") {
		badge, following, present := strings.Cut(rest, "\n")
		if present && strings.HasPrefix(badge, "![") && strings.Contains(badge, "](https://img.shields.io/badge/") {
			body = following
		}
	}
	return strings.TrimSpace(body)
}

// The inline selection identifies an operation, not evidence that the reply
// is right. Project its surviving lines through both revisions before asking
// the reviewer to check the actual behavior in the current implementation.
func (r *rechecker) checkReviewReply(c Checked) Checked {
	n := c.ReviewReply
	original := r.atRevision(n.OriginalRevision)
	submission := r.atRevision(c.Revision)
	before, submitted, current := original.file(c.Scope), submission.file(c.Scope), r.file(c.Scope)
	c.Status, c.Detail = StatusChanged, "reviewed lines cannot be located in the submitted and current code"
	if before.err != nil || submitted.err != nil || current.err != nil || !usableLandingQuote(n.SourceQuote) {
		return c
	}
	from := discussionPosition{revision: n.OriginalRevision, start: n.StartLine, end: n.EndLine}
	if n.ReportedOriginalLine != 0 {
		// The provider's truncated hunk supplies a precise last line. A
		// similar line elsewhere in the original blob cannot stand in for it.
		if originalLines(before.data, from) != n.SourceQuote || uniqueLandingLine(before.data, n.SourceQuote) != from.start {
			return c
		}
	} else if originalLines(before.data, from) != n.SourceQuote {
		from.start = uniqueLandingLine(before.data, n.SourceQuote)
		from.end = from.start + strings.Count(n.SourceQuote, "\n")
	}
	if !from.valid() {
		return c
	}
	start, end, _, err := submission.projectReplyLines(c.Scope, n.OriginalRevision, from.start, from.end, before, submitted)
	if err != nil {
		c.Status, c.Detail = StatusUnverified, "could not compare the original and submitted code: "+err.Error()
		return c
	}
	if start == 0 {
		return c
	}
	first, last, hunks, err := r.projectReplyLines(c.Scope, c.Revision, start, end, submitted, current)
	if err != nil {
		c.Status, c.Detail = StatusUnverified, "could not compare the submitted and current code: "+err.Error()
		return c
	}
	if first == 0 {
		return c
	}
	witness, err := current.cite(c.Scope, r.tree.Commit, first, last)
	if err != nil || discoveryCredential.MatchString(witness.Quote) {
		return c
	}
	linked := *n
	linked.SubmittedStartLine, linked.SubmittedEndLine = start, end
	if len(hunks) != 0 {
		head := r.tree.Commit
		if r.tree.Workspace {
			head = ""
		}
		linked.Change = changedReplyFunction(c.Scope, c.Revision, head, submitted.text, current.text, start, first, hunks)
	}
	c.ReviewReply = &linked
	c.Citations = []Citation{witness.atCurrent(c.Scope, lineAnchor{start: first, end: last})}
	c.Status, c.Detail = StatusHistorical, "review reply matched its original finding; verify its explanation against the current code"
	return c
}

func (r *rechecker) projectReplyLines(path, earlier string, start, end int, before, after checkedFile) (int, int, []diff.Hunk, error) {
	if before.digest == after.digest {
		return start, end, nil, nil
	}
	args := []string{"--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-color", "--unified=0", "--inter-hunk-context=0", earlier}
	if !r.tree.Workspace {
		args = append(args, r.tree.Commit)
	}
	patch, err := r.tree.Git.Output(r.ctx, r.tree.Repo, append(args, "--", path)...)
	if err != nil {
		return 0, 0, nil, err
	}
	first, last := start, end
	hunks := diff.HunkRanges(string(patch))
	for _, h := range hunks {
		if h.OldCount > 0 && h.OldStart <= end && h.OldStart+h.OldCount > start {
			// The original operation was replaced or removed.
			return 0, 0, nil, nil
		}
		if h.OldStart < start {
			first += h.NewCount - h.OldCount
		}
		if h.OldStart < end {
			last += h.NewCount - h.OldCount
		}
	}
	if first < 1 || last < first || last-first >= 24 {
		return 0, 0, nil, nil
	}
	lines := strings.Split(after.text, "\n")
	old := strings.Split(before.text, "\n")
	if last > len(lines) || end > len(old) || lines[first-1] != old[start-1] || lines[last-1] != old[end-1] {
		return 0, 0, nil, nil
	}
	return first, last, hunks, nil
}
