// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// A supplied comment is a source in its own right. Its body neither defines a
// rule nor selects its file. Bind it only to an accepted, inspected file.
func (t Tree) directDiscussionSources(r *rechecker, opts DiscoverOptions, notes []Discussion, result *DiscoveryResult) ([]Fact, error) {
	seen := map[Discussion]bool{}
	var out []Fact
	for _, d := range notes {
		d.exportPath = ""
		if d.UpdatedAt == "" || seen[d] || discoveryCredential.MatchString(d.Body) {
			continue
		}
		seen[d] = true
		p := d.Path
		if p == "" {
			p = opts.Scope // An explicitly selected single file, never a directory.
		}
		if !opts.allowed(p) || discoveryKind(p) == "documentation" {
			continue
		}
		file := r.file(p)
		if file.err != nil && !errors.Is(file.err, errIneligibleFile) {
			return nil, file.err
		}
		if file.err != nil || len(discoveryExcerpts(file.data, discoverySource{Kind: discoveryKind(p)}, 1000, result, nil)) == 0 {
			result.limit("withheld discussion sources without an admissible implementation at the named file")
			result.withholdEdition(t.Commit, d, "")
			continue
		}
		source := discoverySource{Kind: "discussion", Path: d.Path, Revision: d.Revision, Reference: d.ID, URL: d.URL, ObservedAt: d.UpdatedAt}
		excerpts := discoveryExcerpts([]byte(d.Body), source, maxCitationQuote, result, nil)
		if len(excerpts) == 0 {
			// A long single-line comment still has an exact, displayable prefix.
			body := strings.ReplaceAll(d.Body, "\r\n", "\n")
			line := 0
			for text := range strings.SplitSeq(body, "\n") {
				line++
				if strings.TrimSpace(text) == "" {
					continue
				}
				n := min(len(text), maxCitationQuote)
				for n < len(text) && !utf8.RuneStart(text[n]) {
					n--
				}
				source.StartLine, source.Text, source.digest = line, text[:n], Digest([]byte(body))
				excerpts = append(excerpts, source)
				break
			}
		}
		f := Fact{Category: CategoryDiscussionSource, Authority: AuthorityHistorical, Origin: "discovery", Revision: t.Commit,
			Source: p, Scope: p, SourceHash: file.digest, ObservedAt: d.UpdatedAt, ExpiresAt: d.ExpiresAt, DiscussionBody: d.Body}
		for _, e := range excerpts {
			c := Citation{Kind: "discussion", Path: d.Path, Revision: d.Revision, Reference: d.ID, URL: d.URL,
				ObservedAt: d.UpdatedAt, Digest: e.digest, Quote: e.Text, StartLine: e.StartLine, EndLine: e.StartLine + strings.Count(e.Text, "\n")}
			if checkSupplemental(c) == nil {
				f.Citations = append(f.Citations, c)
			}
		}
		if len(f.Citations) != 0 {
			out = append(out, f)
		}
	}
	return out, nil
}

func (f Fact) discussionRef() string {
	if f.Category == CategoryDiscussionSource && len(f.Citations) != 0 {
		return f.Citations[0].Reference
	}
	return ""
}

func discussionSourceID(ref string) string {
	return Digest([]byte(CategoryDiscussionSource + "\x00" + ref))
}

type discussionNoticeKind uint8

const (
	_ discussionNoticeKind = iota
	noticeUnavailable
	noticeExpiry
)

// A deadline belongs to the exact imported version, not to every use of the
// same PR comment ID before and after a correction.
func discussionExpiryID(c Citation) string {
	stamp := c.ObservedAt
	if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		stamp = at.UTC().Format(time.RFC3339Nano)
	}
	return Digest([]byte(strings.Join([]string{"discussion-expiry", c.Reference, c.Path, c.Revision, c.URL, stamp, c.Digest}, "\x00")))
}

func earlierExpiry(current, supplied string) string {
	if supplied == "" {
		return current
	}
	until, err := time.Parse(time.RFC3339Nano, supplied)
	if err != nil {
		return current
	}
	at, err := time.Parse(time.RFC3339Nano, current)
	if current == "" || err == nil && until.Before(at) {
		return until.UTC().Format(time.RFC3339Nano)
	}
	return current
}

func (r *rechecker) atRevision(commit string) *rechecker {
	if !r.tree.Workspace && r.tree.Commit == commit {
		return r
	}
	if r.origins == nil {
		r.origins = map[string]*rechecker{}
	}
	if origin := r.origins[commit]; origin != nil {
		return origin
	}
	tree := r.tree
	tree.Commit, tree.Workspace = commit, false
	origin := newRechecker(r.ctx, tree)
	r.origins[commit] = origin
	return origin
}

// DiscussionWitnesses contains actual, independently read code for a review.
// These lines can establish the implementation only, never the agreement.
func (f Fact) DiscussionWitnesses() []Citation {
	if len(f.ScopeWitnesses) == 0 {
		return nil
	}
	rec := f.promptRecord()
	if !rec.fitsPrompt() {
		return nil
	}
	return rec.ScopeWitnesses
}

func preferDiscussionWitnesses(a, b Checked) bool {
	if a.Status != StatusDiscussionSource || b.Status != StatusDiscussionSource || a.Revision != b.Revision {
		return false
	}
	count := func(f Fact) int {
		n := 0
		for _, c := range f.ScopeWitnesses {
			if c.Symbol != "" {
				n++
			}
		}
		return n
	}
	return count(a.Fact) > count(b.Fact)
}

func (r *rechecker) checkDiscussionSource(c Checked) Checked {
	ref := c.discussionRef()
	if ref == "" || c.Scope != c.Source || c.Claim != "" || c.Authority != AuthorityHistorical || c.ObservedAt == "" || !r.observedBy(c.ObservedAt) {
		c.Detail = "discussion source lacks a dated, exact file binding"
		return c
	}
	body := strings.ReplaceAll(c.DiscussionBody, "\r\n", "\n")
	digest := Digest([]byte(body))
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for _, note := range c.Citations {
		if note.Kind != "discussion" || note.Reference != ref || note.ObservedAt != c.ObservedAt || note.Path != "" && note.Path != c.Scope ||
			note.Revision != "" && !r.ancestor(note.Revision) || checkSupplemental(note) != nil ||
			note.Digest != digest || note.EndLine > len(lines) || !strings.HasPrefix(strings.Join(lines[note.StartLine-1:note.EndLine], "\n"), note.Quote) {
			c.Detail = "invalid discussion source provenance"
			return c
		}
	}
	file := r.file(c.Scope)
	if file.digest != c.SourceHash {
		original := r.atRevision(c.Revision).file(c.Scope)
		if original.digest != c.SourceHash || !sameDiscussionFile(c.Scope, original, file) {
			c.Status, c.Detail = StatusChanged, "discussion's original file entry is no longer identifiable"
			return c
		}
	}
	c.Fact = r.discussionAtHead(c.Fact)
	if file.err != nil || len(c.ScopeWitnesses) == 0 {
		c.Status, c.Detail = StatusChanged, "discussion's accepted file changed"
		return c
	}
	c.Status, c.Detail = StatusDiscussionSource, "dated original discussion; independently check its scope and current behavior"
	return c
}

func (r *rechecker) discussionAtHead(f Fact) Fact {
	f.contextStatus, f.ScopeWitnesses = StatusDiscussionSource, nil
	file := r.file(f.Scope)
	if file.err != nil {
		return f
	}
	excerpts := discoveryExcerpts(file.data, discoverySource{Kind: discoveryKind(f.Scope), Path: f.Scope}, 1000, &DiscoveryResult{}, nil)
	for _, s := range excerpts {
		c, err := file.cite(f.Scope, r.tree.Commit, s.StartLine, s.StartLine+strings.Count(s.Text, "\n"))
		if err == nil {
			f.ScopeWitnesses = append(f.ScopeWitnesses, c.atCurrent(f.Scope, lineAnchor{start: c.StartLine, end: c.EndLine}))
		}
	}
	if len(f.ScopeWitnesses) != 0 {
		f.ScopeWitnesses = append(f.ScopeWitnesses, r.discussionNamedWitnesses(f)...)
	}
	return f
}

var discussionDeclaration = regexp.MustCompile(`^(?:(?:export|default|public|pub|async|private|protected|static|abstract|final|internal|sealed|override)\s+)*(?:def|func|function|fn|fun|class|struct|interface|trait|enum|type|const|let|var|val|module)\s+[\pL_$][\pL\pN_$]*`)
var discussionConfigKey = regexp.MustCompile(`^["']?([\pL_][\pL\pN_.-]*)["']?\s*[:=]`)

// A shared named entry can establish that this is still the discussed file.
// Boilerplate, a common return value or a comment cannot establish continuity.
func sameDiscussionFile(p string, before, after checkedFile) bool {
	if before.err != nil || after.err != nil {
		return false
	}
	if before.digest == after.digest {
		return true
	}
	if before.goDoc != nil && after.goDoc != nil {
		for name, decl := range before.goDoc.anchors {
			if decl.digest != "" && after.goDoc.anchors[name].digest != "" {
				return true
			}
		}
		return false
	}
	matcher := discussionDeclaration
	if discoveryKind(p) == "config" {
		matcher = discussionConfigKey
	}
	entries := func(text string) map[string]int {
		names := map[string]int{}
		for line := range strings.SplitSeq(text, "\n") {
			if name := matcher.FindString(strings.TrimSpace(line)); name != "" {
				names[name]++
			}
		}
		return names
	}
	newEntries := entries(after.text)
	for key, count := range entries(before.text) {
		if count == 1 && newEntries[key] == 1 {
			return true
		}
	}
	return false
}

type discussionEdition struct {
	date                                 time.Time
	digest, scope, expiry, revision, url string
	ambiguous                            bool
	unavailable                          bool
	origins                              []string
}
type discussionEditions map[string]discussionEdition

func (r *rechecker) discussionEditions(facts []Fact) discussionEditions {
	latest := discussionEditions{}
	for _, f := range facts {
		if f.discussionNotice != 0 {
			when, err := time.Parse(time.RFC3339Nano, f.ObservedAt)
			if err != nil || !r.ancestor(f.Revision) {
				continue
			}
			if f.discussionNotice == noticeExpiry {
				latest[f.ID] = discussionEdition{expiry: earlierExpiry(latest[f.ID].expiry, f.ObservedAt)}
			} else if r.observedBy(f.ObservedAt) && !when.Before(latest[f.ID].date) {
				latest[f.ID] = discussionEdition{date: when, unavailable: true}
			}
			continue
		}
		ref := f.discussionRef()
		if ref == "" || !r.ancestor(f.Revision) || !r.observedBy(f.ObservedAt) {
			continue
		}
		c := f.Citations[0]
		if c.Revision != "" && !r.ancestor(c.Revision) {
			continue
		}
		when, err := time.Parse(time.RFC3339Nano, f.ObservedAt)
		if err != nil {
			continue
		}
		id := discussionSourceID(ref)
		edition, ok := latest[id]
		switch {
		case !ok || when.After(edition.date):
			edition = discussionEdition{date: when, digest: c.Digest, scope: f.Scope, expiry: f.ExpiresAt, revision: c.Revision, url: c.URL}
		case when.Before(edition.date) || edition.unavailable:
			continue
		case edition.digest != c.Digest || edition.scope != f.Scope || edition.expiry != f.ExpiresAt || c.Revision != edition.revision || c.URL != edition.url:
			edition.ambiguous = true
		}
		if c.Revision == "" && !edition.ambiguous {
			// Without a cited Git revision, keep the earliest actual binding
			// on each accepted history. A repeated import does not reassign
			// an unchanged old discussion to a replacement subsystem.
			if !slices.ContainsFunc(edition.origins, func(old string) bool {
				return old == f.Revision || r.atRevision(f.Revision).ancestor(old)
			}) {
				edition.origins = slices.DeleteFunc(edition.origins, func(old string) bool { return r.atRevision(old).ancestor(f.Revision) })
				edition.origins = append(edition.origins, f.Revision)
			}
		}
		latest[id] = edition
	}
	return latest
}

func (ed discussionEditions) withCitedExpiry(f Fact) Fact {
	for _, c := range f.Citations {
		if c.Kind == "discussion" && f.Category != CategoryDiscussionSource {
			f.ExpiresAt = earlierExpiry(f.ExpiresAt, ed[discussionExpiryID(c)].expiry)
		}
	}
	return f
}

func (ed discussionEditions) admits(f Fact) bool {
	v, ok := ed[discussionSourceID(f.discussionRef())]
	when, _ := time.Parse(time.RFC3339Nano, f.ObservedAt)
	return ok && !v.ambiguous && !v.unavailable && when.Equal(v.date) && f.Citations[0].Digest == v.digest && f.Scope == v.scope && f.ExpiresAt == v.expiry &&
		(v.revision != "" || slices.Contains(v.origins, f.Revision))
}

func (ed discussionEditions) obsolete(f Fact) bool {
	return slices.ContainsFunc(f.Citations, func(c Citation) bool {
		v, ok := ed[discussionSourceID(c.Reference)]
		when, _ := time.Parse(time.RFC3339Nano, c.ObservedAt)
		if v.unavailable {
			return c.Kind == "discussion" && !when.After(v.date)
		}
		return c.Kind == "discussion" && ok && (v.ambiguous || !when.Equal(v.date) || c.Digest != v.digest || c.Path != "" && c.Path != v.scope)
	})
}

func suppressInactiveDiscussions(checked []Checked) {
	inactive := map[string]bool{}
	for _, c := range checked {
		if ref := c.discussionRef(); ref != "" && c.Status != StatusDiscussionSource {
			inactive[ref] = true
		}
	}
	for i := range checked {
		c := &checked[i]
		if c.Category != CategoryDiscussionSource && slices.ContainsFunc(c.Citations, func(v Citation) bool { return v.Kind == "discussion" && inactive[v.Reference] }) {
			c.Status, c.Detail = StatusSuperseded, "cited discussion is no longer active"
		}
	}
}
