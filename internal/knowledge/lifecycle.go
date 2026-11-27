// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	StatusVerified         = "verified"
	StatusCurrent          = "current"
	StatusCandidate        = "candidate"
	StatusHistorical       = "historical"
	StatusChanged          = "changed"
	StatusTransition       = "changed_in_review"
	StatusUnverified       = "unverified"
	StatusExpired          = "expired"
	StatusSuperseded       = "superseded"
	StatusConflict         = "conflict"
	StatusRejected         = "rejected"
	StatusDiscussionSource = "discussion_source"
)

func validDigest(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 64 && err == nil
}

func (t Tree) Prepare(ctx context.Context, f Fact) (Fact, error) {
	if t.Workspace || !validOID(t.Commit) {
		return Fact{}, fmt.Errorf("record memory at a committed revision")
	}
	if !validScope(f.Scope) || !t.permits(f.Source) || f.ReviewID != "" {
		return Fact{}, fmt.Errorf("reusable memory requires allowed repository source and scope paths")
	}
	switch f.Category {
	case CategoryBusinessRule, CategoryDomainConstraint, CategoryCodingConvention, CategoryInvariant, CategoryDesign:
	default:
		return Fact{}, fmt.Errorf("unsupported knowledge category %q", f.Category)
	}
	switch f.Authority {
	case AuthorityConfirmed, AuthorityInferred, AuthorityHistorical:
	default:
		return Fact{}, fmt.Errorf("unsupported knowledge authority %q", f.Authority)
	}
	f.Claim = strings.TrimSpace(f.Claim)
	if f.Claim == "" || len(f.Claim) > 1000 || len(f.Citations) == 0 || len(f.Citations) > 8 || len(f.Conditions) > 8 {
		return Fact{}, fmt.Errorf("expected a short claim with 1-8 citations and up to 8 conditions")
	}
	if len(f.Subject) > 160 || len(f.Predicate) > 160 || len(f.Value) > 1024 || len(f.Identifiers) > 16 || len(f.Origin) > 160 {
		return Fact{}, fmt.Errorf("knowledge claim metadata exceeds its size limit")
	}
	if len(f.Value) > 0 && !scalarLiteral(f.Value) {
		return Fact{}, fmt.Errorf("a comparable claim value must be a JSON scalar")
	}
	if f.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, f.ExpiresAt); err != nil {
			return Fact{}, fmt.Errorf("parse memory expiry: %w", err)
		}
	}
	for _, id := range f.Identifiers {
		if id == "" || len(id) > 160 || strings.ContainsFunc(id, unicode.IsControl) {
			return Fact{}, fmt.Errorf("invalid knowledge identifier")
		}
	}
	for _, id := range f.Supersedes {
		if !validDigest(id) || f.Authority != AuthorityConfirmed {
			return Fact{}, fmt.Errorf("only confirmed knowledge can explicitly supersede an existing memory ID")
		}
	}
	f.ID, f.Revision, f.Feedback, f.contextStatus = "", t.Commit, nil, ""
	f.Citations = slices.Clone(f.Citations)
	f.Conditions = slices.Clone(f.Conditions)
	recheck := newRechecker(ctx, t)
	stamp := func(c Citation) (Citation, error) {
		if c.Path != "" && !t.permits(c.Path) {
			return Citation{}, fmt.Errorf("invalid or excluded citation path %q", c.Path)
		}
		if !c.live() {
			c.CurrentPath, c.CurrentStartLine, c.CurrentEndLine = "", 0, 0
			return c, checkSupplemental(c)
		}
		file := recheck.file(c.Path)
		if file.err != nil {
			return Citation{}, file.err
		}
		out, err := file.cite(c.Path, t.Commit, c.StartLine, c.EndLine)
		if err != nil {
			return Citation{}, err
		}
		if c.Quote != "" && c.Quote != out.Quote {
			return Citation{}, fmt.Errorf("citation quote does not match %s:%d", c.Path, c.StartLine)
		}
		out.Kind = c.Kind
		return out, nil
	}
	primary := false
	for i, c := range f.Citations {
		citation, err := stamp(c)
		if err != nil {
			return Fact{}, err
		}
		primary = primary || citation.live() && citation.Path == f.Source
		f.Citations[i] = citation
	}
	if !primary && f.Authority != AuthorityHistorical {
		return Fact{}, fmt.Errorf("the primary source needs a current citation")
	}
	for i, condition := range f.Conditions {
		if strings.TrimSpace(condition.Description) == "" || len(condition.Description) > 1000 {
			return Fact{}, fmt.Errorf("condition requires a short description")
		}
		if len(condition.Key) > 160 || len(condition.Equals) > 256 ||
			(strings.TrimSpace(condition.Key) == "") != (len(condition.Equals) == 0) ||
			len(condition.Equals) > 0 && (!scalarLiteral(condition.Equals) || condition.Citation == nil) {
			return Fact{}, fmt.Errorf("a comparable condition requires a key, scalar value and current citation")
		}
		if condition.Citation == nil {
			continue
		}
		c, err := stamp(*condition.Citation)
		if err != nil {
			return Fact{}, err
		}
		if !c.live() {
			return Fact{}, fmt.Errorf("condition requires current evidence")
		}
		f.Conditions[i].Citation = &c
	}
	return f, ctx.Err()
}

func scalarLiteral(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return json.Valid(raw) && raw[0] != '[' && raw[0] != '{'
}

type checkedFile struct {
	citationFile
	err error
}

type schemaAssertions struct {
	values []Constraint
	err    error
}

type rechecker struct {
	ctx                   context.Context
	tree                  Tree
	now                   time.Time
	files                 map[string]checkedFile
	schemas               map[string]schemaAssertions
	reachable             map[string]bool
	origins               map[string]*rechecker
	moves                 map[[2]string][]string
	python                map[[2]string]*pythonWitnesses
	discussionDefinitions map[string]discussionDefinitionFile
}

func newRechecker(ctx context.Context, t Tree) *rechecker {
	now := t.Now
	if now.IsZero() {
		now = time.Now()
	}
	return &rechecker{ctx: ctx, tree: t, now: now, files: map[string]checkedFile{}, schemas: map[string]schemaAssertions{}, reachable: map[string]bool{}, moves: map[[2]string][]string{}, python: map[[2]string]*pythonWitnesses{}}
}

func (r *rechecker) file(p string) checkedFile {
	if file, ok := r.files[p]; ok {
		return file
	}
	data, err := r.tree.read(r.ctx, p)
	file := checkedFile{newCitationFile(p, data), err}
	r.files[p] = file
	return file
}

func (r *rechecker) ancestor(revision string) bool {
	if reachable, ok := r.reachable[revision]; ok {
		return reachable
	}
	reachable := false
	t := r.tree
	if validOID(revision) && validOID(t.Commit) && t.Git != nil {
		_, err := t.Git.Output(r.ctx, t.Repo, "merge-base", "--is-ancestor", revision, t.Commit)
		reachable = err == nil
	}
	r.reachable[revision] = reachable
	return reachable
}

// CheckReview admits mined rules only from the accepted side of the change.
// A shared database can already contain observations mined from the proposal.
// User-supplied decisions are scoped separately and may refer to an earlier push.
func (t Tree) CheckReview(ctx context.Context, base string, facts []Fact) ([]Checked, error) {
	if base != t.Commit {
		accepted := t
		accepted.Commit, accepted.Workspace = base, false
		origin := newRechecker(ctx, accepted)
		kept := make([]Fact, 0, len(facts))
		for _, f := range facts {
			if f.Origin != "discovery" && !f.schema() || origin.ancestor(f.Revision) {
				kept = append(kept, f)
			} else if f.schema() && validOID(base) && origin.checkSchema(Checked{Fact: f}).Status == StatusVerified {
				// An explicit mapping may have been recorded on the proposal.
				// Admit it only if the same property already held at the base.
				kept = append(kept, f)
			}
		}
		facts = kept
	}
	return t.Check(ctx, facts)
}

func (t Tree) Check(ctx context.Context, facts []Fact) ([]Checked, error) {
	r := newRechecker(ctx, t)
	editions := r.discussionEditions(facts)
	positions := make(map[string]int)
	result := make([]Checked, 0, len(facts))
	for _, f := range facts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if f.discussionNotice != 0 {
			continue // An edit timestamp retires old evidence and has no claim to display.
		}
		if f.hasFutureLanding(r.now) {
			continue
		}
		if !validOID(f.Revision) || !validPath(f.Source) || !validScope(f.Scope) ||
			f.IsDecision() && !validPath(f.Scope) || f.schema() && !validSource(f.Source) {
			return nil, fmt.Errorf("invalid stored evidence for %s", f.ID)
		}
		if f.ID == "" {
			id, err := stableFactID(f)
			if err != nil {
				return nil, err
			}
			f.ID = id
		}
		checked := r.observation(editions.withCitedExpiry(f))
		if f.Category == CategoryDiscussionSource && !editions.admits(f) {
			continue
		}
		if f.Category != CategoryDiscussionSource && editions.obsolete(f) {
			checked.Status, checked.Detail = StatusSuperseded, "cited discussion was edited or withdrawn"
		}
		if pos, exists := positions[f.ID]; exists {
			if observationRank(checked.Status) > observationRank(result[pos].Status) ||
				preferDiscussionWitnesses(checked, result[pos]) {
				result[pos] = checked
			}
			continue
		}
		positions[f.ID] = len(result)
		result = append(result, checked)
	}
	markSuperseded(result, positions)
	suppressInactiveDiscussions(result)
	markConflicts(result)
	linkRejectedObservations(result)
	return result, ctx.Err()
}

// Rewording a previously rejected assertion may give it a new observation ID.
// Preserve the new provenance but surface the disagreement with the maintainer
// when its typed value, scope, conditions and live premises still match.
func linkRejectedObservations(checked []Checked) {
	type prior struct {
		id    string
		value json.RawMessage
	}
	byClaim := map[string][]prior{}
	key := func(f Fact) string {
		f.Claim, f.Identifiers, f.Authority, f.contextStatus = "", nil, AuthorityInferred, StatusCandidate
		return f.equivalentLeadKey()
	}
	for _, c := range checked {
		if c.Status != StatusRejected {
			continue
		}
		if k := key(c.Fact); k != "" {
			byClaim[k] = append(byClaim[k], prior{c.ID, c.Value})
		}
	}
	if len(byClaim) == 0 {
		return
	}
	for i := range checked {
		c := &checked[i]
		if c.Status != StatusCandidate {
			continue
		}
		for _, old := range byClaim[key(c.Fact)] {
			if equal, _ := equalJSONLiteral(c.Value, old.value); equal {
				c.Status, c.Detail = StatusConflict, "equivalent supported assertion was rejected"
				c.Conflicts = append(c.Conflicts, old.id)
			}
		}
	}
}

func observationRank(status string) int {
	switch status {
	case StatusRejected, StatusSuperseded, StatusExpired:
		return 4
	case StatusVerified, StatusCurrent, StatusCandidate, StatusHistorical, StatusDiscussionSource:
		return 3
	case StatusChanged:
		return 2
	default:
		return 1
	}
}

func (r *rechecker) observation(f Fact) Checked {
	checked := Checked{Fact: f, Status: StatusUnverified}
	if !r.ancestor(f.Revision) {
		checked.Detail = "source revision is not a proven ancestor"
		return checked
	}
	if !r.tree.permits(f.Source) || f.Scope != "." && !r.tree.permits(f.Scope) {
		checked.Detail = "memory source or scope is excluded"
		return checked
	}
	if f.IsDecision() && !r.observedBy(f.ObservedAt) {
		checked.Detail = "decision observation is outside the review time"
		return checked
	}
	for _, p := range f.EvidencePaths() {
		if !r.tree.permits(p) {
			checked.Detail = "memory citation is excluded"
			return checked
		}
	}
	var discussionFailure string
	f.eachCitation(func(c Citation) {
		if c.Kind != "discussion" || discussionFailure != "" {
			return
		}
		switch {
		case !r.observedBy(c.ObservedAt):
			discussionFailure = "discussion citation is outside the review time"
		case !r.landingVisible(c):
			discussionFailure = "original reviewed code no longer matches the landed and current file"
		}
	})
	if discussionFailure != "" {
		checked.Detail = discussionFailure
		return checked
	}
	for _, feedback := range f.Feedback {
		if !r.observedBy(feedback.RecordedAt) || !r.ancestor(feedback.Revision) {
			continue
		}
		switch feedback.Action {
		case FeedbackReject:
			checked.Status, checked.Detail = StatusRejected, feedback.Reason
			return checked
		case FeedbackReplace:
			checked.Status, checked.Detail = StatusSuperseded, "replaced by "+feedback.ReplacementID
			return checked
		case FeedbackConfirm:
			if !f.IsDecision() && !f.schema() && f.Authority != AuthorityHistorical {
				checked.Authority = AuthorityConfirmed
			}
		}
	}
	if f.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, f.ExpiresAt)
		if err != nil {
			checked.Detail = "invalid memory expiry"
			return checked
		}
		if !r.now.Before(expires) {
			checked.Status, checked.Detail = StatusExpired, "memory reached its expiry"
			return checked
		}
	}
	if f.schema() {
		return r.checkSchema(checked)
	}
	if f.Category == CategoryDiscussionSource {
		return r.checkDiscussionSource(checked)
	}
	if f.IsDecision() {
		return r.checkDecision(checked)
	}
	return r.checkClaim(checked)
}

func (r *rechecker) observedBy(when string) bool {
	if when == "" {
		return r.tree.Now.IsZero()
	}
	at, err := time.Parse(time.RFC3339Nano, when)
	return err == nil && !at.After(r.now)
}

func (r *rechecker) checkSchema(c Checked) Checked {
	a, ok := r.schemas[c.Source]
	if !ok {
		file := r.file(c.Source)
		a.err = file.err
		if a.err == nil {
			a.values, a.err = Constraints(file.data)
		}
		r.schemas[c.Source] = a
	}
	if a.err != nil {
		c.Detail = a.err.Error()
		return c
	}
	c.Category, c.Authority = CategorySchema, AuthorityStructured
	c.Status, c.Detail = StatusChanged, "recorded schema constraint changed"
	for _, v := range a.values {
		if c.Property == v.Property && c.Keyword == v.Keyword && sameConstraint(c.Fact, v) {
			c.Status, c.Detail = StatusVerified, ""
			break
		}
	}
	return c
}

func (r *rechecker) checkDecision(c Checked) Checked {
	switch {
	case c.ReviewID == "" || c.Finding == "" || c.Reason == "":
		c.Detail = "decision lacks a review identifier, finding or rationale"
		return c
	case len(c.Citations) != 0:
		var live, allConditions bool
		c, live, allConditions = r.checkPremises(c)
		if !live || !allConditions || c.Status == StatusChanged || c.Detail != "" {
			return c
		}
	default:
		if status, detail := r.matchDecisionFile(c.Fact, c.Source, c.SourceHash, "decision evidence"); status != "" {
			c.Status, c.Detail = status, detail
			return c
		}
	}
	anchoredScope := slices.ContainsFunc(c.Citations, func(v Citation) bool { return v.live() && v.CurrentPath == c.Scope })
	if !anchoredScope {
		if status, detail := r.matchDecisionFile(c.Fact, c.Scope, c.CodeHash, "reviewed file"); status != "" {
			c.Status, c.Detail = status, detail
			return c
		}
	}
	c.Category, c.Authority = CategoryDecision, AuthorityHistorical
	c.Status, c.Detail = StatusHistorical, "recorded rationale requires checking against the current review"
	return c
}

func (r *rechecker) matchDecisionFile(f Fact, p, expected, label string) (string, string) {
	file := r.file(p)
	if file.err != nil {
		return StatusUnverified, file.err.Error()
	}
	if file.digest == expected {
		return "", ""
	}
	if file.goDoc != nil {
		old := r.tree
		old.Commit, old.Workspace = f.Revision, false
		data, err := old.read(r.ctx, p)
		if err == nil && Digest(data) == expected {
			doc := parseGoEvidence(p, data)
			if doc != nil && doc.whole == file.goDoc.whole {
				return "", ""
			}
		}
	}
	return StatusChanged, label + " changed"
}

func (r *rechecker) checkClaim(c Checked) Checked {
	if c.Claim == "" {
		c.Detail = "memory lacks a structured constraint or claim"
		return c
	}
	if c.Authority == AuthorityHistorical {
		c.Status, c.Detail = StatusHistorical, "source is historical"
		return c
	}
	var live, allConditions bool
	c, live, allConditions = r.checkPremises(c)
	if c.Detail != "" {
		return c
	}
	if !live {
		c.Detail = "no current code or configuration citation"
		return c
	}
	if c.Authority == AuthorityConfirmed && allConditions {
		c.Status, c.Detail = StatusCurrent, "confirmed claim with matching evidence and condition anchors"
	} else {
		c.Status, c.Detail = StatusCandidate, "check the claim and its applicability against current code"
	}
	return c
}

func (r *rechecker) checkPremises(c Checked) (Checked, bool, bool) {
	c.Citations = slices.Clone(c.Citations)
	c.Conditions = slices.Clone(c.Conditions)
	live, allConditions := false, true
	match := func(citation Citation, label string) (Citation, bool) {
		out, status, detail := r.matchCitation(c.Revision, citation)
		if status != "" {
			c.Status, c.Detail = status, label+detail
			return out, false
		}
		return out, true
	}
	for i, citation := range c.Citations {
		if !citation.live() {
			continue
		}
		out, ok := match(citation, "citation: ")
		c.Citations[i] = out
		if !ok {
			return c, false, false
		}
		live = true
	}
	for i, cond := range c.Conditions {
		if cond.Citation == nil || !cond.Citation.live() {
			allConditions = false
			continue
		}
		out, ok := match(*cond.Citation, "condition: ")
		c.Conditions[i].Citation = &out
		if !ok {
			return c, false, false
		}
	}
	return c, live, allConditions
}

func (r *rechecker) matchCitation(revision string, c Citation) (Citation, string, string) {
	if c.Revision != revision || !validDigest(c.Digest) || !r.tree.permits(c.Path) {
		return c, StatusUnverified, "invalid citation provenance"
	}
	file := r.file(c.Path)
	if file.err == nil {
		if a, found := file.match(c); found {
			if a.digest == c.Digest {
				return c.atCurrent(c.Path, a), "", ""
			}
			return c, StatusChanged, c.Path + " declaration changed"
		}
		if a, matches := r.relocatedPython(c, file); matches {
			return c.atCurrent(c.Path, a), "", ""
		}
	}
	if c.Anchor != "" {
		if p, a := r.movedAnchor(revision, c); p != "" {
			current := c.atCurrent(p, a)
			if a.digest == c.Digest {
				return current, "", ""
			}
			return current, StatusChanged, p + " moved declaration changed"
		}
	}
	if file.err != nil {
		return c, StatusUnverified, file.err.Error()
	}
	return c, StatusChanged, c.Path + " citation no longer matches"
}

func (r *rechecker) movedAnchor(revision string, c Citation) (string, lineAnchor) {
	dir := path.Dir(c.Path)
	key := [2]string{revision, dir}
	paths, ok := r.moves[key]
	if !ok {
		if r.tree.Git == nil {
			return "", lineAnchor{}
		}
		args := []string{"--literal-pathspecs", "diff", "--name-only", "-z", "--no-renames", "--diff-filter=ACM", revision}
		if !r.tree.Workspace {
			args = append(args, r.tree.Commit)
		}
		if dir != "." {
			args = append(args, "--", dir)
		}
		data, clipped, err := r.tree.discoveryGit(r.ctx, 64<<10, args...)
		if err != nil || clipped {
			return "", lineAnchor{}
		}
		for _, p := range append(strings.Split(string(data), "\x00"), r.tree.Paths...) {
			if r.tree.permits(p) && path.Dir(p) == dir && strings.HasSuffix(p, ".go") {
				paths = append(paths, p)
			}
		}
		slices.Sort(paths)
		paths = slices.Compact(paths)
		r.moves[key] = paths
	}
	if len(paths) > 32 {
		return "", lineAnchor{}
	}
	var found string
	var anchor lineAnchor
	for _, p := range paths {
		if p == c.Path {
			continue
		}
		file := r.file(p)
		if a, exists := file.match(c); exists && a.digest != "" {
			if found != "" {
				return "", lineAnchor{}
			}
			found, anchor = p, a
		}
	}
	return found, anchor
}

func markSuperseded(result []Checked, positions map[string]int) {
	replacements := make(map[int]string)
	for _, c := range result {
		if c.Status != StatusCurrent && c.Status != StatusVerified {
			continue
		}
		for _, id := range c.Supersedes {
			if pos, ok := positions[id]; ok && c.ID != id && correspondingRecords(c.Fact, result[pos].Fact) {
				if previous := replacements[pos]; previous == "" || c.ID < previous {
					replacements[pos] = c.ID
				}
			}
		}
	}
	for pos, replacement := range replacements {
		result[pos].Status, result[pos].Detail = StatusSuperseded, "replaced by "+replacement
	}
}

func correspondingRecords(a, b Fact) bool {
	if a.ReviewID != b.ReviewID || a.Scope != b.Scope || a.IsDecision() != b.IsDecision() {
		return false
	}
	if a.IsDecision() || a.schema() || b.schema() || a.Subject != "" && b.Subject != "" {
		return conflictKey(a) != "" && conflictKey(a) == conflictKey(b)
	}
	return a.Category == b.Category
}

func normalizedText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func conflictKey(f Fact) string {
	switch {
	case f.IsDecision() && f.ReviewID != "" && f.Finding != "":
		return "decision\x00" + f.ReviewID + "\x00" + normalizedText(f.Finding)
	case f.schema():
		return "schema\x00" + f.Property
	case strings.TrimSpace(f.Subject) != "" && strings.TrimSpace(f.Predicate) != "" && len(f.Value) > 0:
		return "claim\x00" + normalizedText(f.Subject) + "\x00" + normalizedText(f.Predicate)
	default:
		return ""
	}
}

func markConflicts(result []Checked) {
	groups := make(map[string][]int)
	for i, c := range result {
		switch c.Status {
		case StatusVerified, StatusCurrent, StatusCandidate:
		case StatusHistorical:
			if !c.IsDecision() {
				continue
			}
		default:
			continue
		}
		if key := conflictKey(c.Fact); key != "" {
			groups[key] = append(groups[key], i)
		}
	}
	for _, positions := range groups {
		for i, pos := range positions {
			a := &result[pos]
			for _, other := range positions[i+1:] {
				b := &result[other]
				if policiesOverlap(a.Fact, b.Fact) && conflictingValues(a.Fact, b.Fact) {
					a.Conflicts = append(a.Conflicts, b.ID)
					b.Conflicts = append(b.Conflicts, a.ID)
				}
			}
		}
	}
	for i := range result {
		c := &result[i]
		if len(c.Conflicts) > 0 {
			slices.Sort(c.Conflicts)
			c.Status, c.Detail = StatusConflict, "overlapping memory records need reconciliation"
			if c.IsDecision() {
				c.Detail = "multiple review rationales need verification against the current code"
			}
		}
	}
}

func policiesOverlap(a, b Fact) bool {
	scopeOverlaps := a.Scope == b.Scope || !a.IsDecision() && !b.IsDecision() &&
		(scopeContains(a.Scope, b.Scope) || scopeContains(b.Scope, a.Scope))
	return scopeOverlaps && !disjointConditions(a.Conditions, b.Conditions)
}

func disjointConditions(a, b []Condition) bool {
	for _, left := range a {
		if left.Citation == nil || left.Key == "" || !scalarLiteral(left.Equals) {
			continue
		}
		for _, right := range b {
			if right.Citation == nil || left.Key != right.Key || !scalarLiteral(right.Equals) {
				continue
			}
			equal, comparable := equalJSONLiteral(left.Equals, right.Equals)
			if comparable && !equal {
				return true
			}
		}
	}
	return false
}

func conflictingValues(a, b Fact) bool {
	if a.IsDecision() {
		return normalizedText(a.Reason) != normalizedText(b.Reason)
	}
	if a.schema() {
		x, y := constraintValues(a), constraintValues(b)
		return len(x) > 0 && len(y) > 0 && !slices.ContainsFunc(x, func(v json.RawMessage) bool {
			return slices.ContainsFunc(y, func(other json.RawMessage) bool {
				equal, comparable := equalJSONLiteral(v, other)
				return equal || !comparable
			})
		})
	}
	equal, comparable := equalJSONLiteral(a.Value, b.Value)
	return comparable && !equal
}

func constraintValues(f Fact) []json.RawMessage {
	if f.Keyword == "const" {
		return []json.RawMessage{f.Value}
	}
	var values []json.RawMessage
	if err := json.Unmarshal(f.Value, &values); err != nil {
		return nil
	}
	return values
}

func sameConstraint(f Fact, c Constraint) bool {
	if bytes.Equal(f.Value, c.Value) {
		return true
	}
	x, y := constraintValues(f), constraintValues(Fact{Keyword: c.Keyword, Value: c.Value})
	isSubset := func(a, b []json.RawMessage) bool {
		return !slices.ContainsFunc(a, func(v json.RawMessage) bool {
			return !slices.ContainsFunc(b, func(other json.RawMessage) bool {
				equal, _ := equalJSONLiteral(v, other)
				return equal
			})
		})
	}
	return len(x) > 0 && len(y) > 0 && isSubset(x, y) && isSubset(y, x)
}
