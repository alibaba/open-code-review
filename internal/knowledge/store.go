// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"modernc.org/sqlite"
)

func init() {
	// SQLite's built-in lower() folds ASCII alone. Search archived editions
	// with the same Unicode mapping as the final displayed-edition filter.
	sqlite.MustRegisterDeterministicScalarFunction("ocr_discussion_lower", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		text, _ := args[0].(string)
		return strings.ToLower(text), nil
	})
}

type Store struct {
	db                *sql.DB
	repo              string
	lifecycle         bool
	discussionEdits   bool
	editionNotices    bool
	schemaWithdrawals bool
	schemaParents     bool
	schemaAdditions   bool
}

func Open(ctx context.Context, filename, repo string, write bool) (*Store, error) {
	if filename == "" || repo == "" {
		return nil, fmt.Errorf("a memory database path and repository identity are required")
	}
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	mode := "ro"
	if write {
		file, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if err = file.Close(); err != nil {
			return nil, err
		}
		mode = "rw"
	}
	p := filepath.ToSlash(absolute)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	uri := url.URL{Scheme: "file", Path: p, RawQuery: "mode=" + mode + "&_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var lifecycle, discussionEdits, editionNotices, schemaWithdrawals, schemaParents, schemaAdditions bool
	if write {
		err = initialize(ctx, db)
		lifecycle = err == nil
		discussionEdits = err == nil
		editionNotices = err == nil
		schemaWithdrawals = err == nil
		schemaParents = err == nil
		schemaAdditions = err == nil
	} else {
		err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE name='fact_observations'), EXISTS(SELECT 1 FROM sqlite_schema WHERE name='discussion_edits'), EXISTS(SELECT 1 FROM sqlite_schema WHERE name='discussion_unavailable'), EXISTS(SELECT 1 FROM sqlite_schema WHERE name='schema_withdrawals'), EXISTS(SELECT 1 FROM sqlite_schema WHERE name='schema_parents'), EXISTS(SELECT 1 FROM sqlite_schema WHERE name='schema_parent_additions')").Scan(&lifecycle, &discussionEdits, &editionNotices, &schemaWithdrawals, &schemaParents, &schemaAdditions)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, repo: repo, lifecycle: lifecycle, discussionEdits: discussionEdits, editionNotices: editionNotices, schemaWithdrawals: schemaWithdrawals, schemaParents: schemaParents, schemaAdditions: schemaAdditions}, nil
}

func initialize(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS facts (
 fact_rowid INTEGER PRIMARY KEY, repo TEXT NOT NULL, id TEXT NOT NULL, review TEXT NOT NULL, body TEXT NOT NULL,
 recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, revoked_at TEXT,
 UNIQUE(repo,id));
 CREATE INDEX IF NOT EXISTS facts_scope ON facts(repo,review,revoked_at);
 CREATE TABLE IF NOT EXISTS fact_revisions(fact_row INTEGER NOT NULL, revision TEXT NOT NULL,
 PRIMARY KEY(fact_row,revision));
 CREATE VIRTUAL TABLE IF NOT EXISTS fact_search USING fts5(text, tokenize='trigram');
 CREATE TABLE IF NOT EXISTS fact_observations(fact_row INTEGER NOT NULL, revision TEXT NOT NULL, body TEXT NOT NULL,
 PRIMARY KEY(fact_row,revision));
 CREATE TABLE IF NOT EXISTS discussion_edits(fact_row INTEGER NOT NULL, revision TEXT NOT NULL, body TEXT NOT NULL,
 PRIMARY KEY(fact_row,revision,body));
 CREATE TABLE IF NOT EXISTS discussion_unavailable(repo TEXT NOT NULL, source_id TEXT NOT NULL, revision TEXT NOT NULL, observed_at TEXT NOT NULL, scope_digest TEXT NOT NULL,
 PRIMARY KEY(repo,source_id,revision,observed_at,scope_digest));
 CREATE TABLE IF NOT EXISTS schema_withdrawals(fact_row INTEGER PRIMARY KEY, withdrawn_at TEXT NOT NULL, learned_at TEXT);
 CREATE TABLE IF NOT EXISTS schema_parents(fact_row INTEGER NOT NULL, parent_row INTEGER NOT NULL, learned_at TEXT, PRIMARY KEY(fact_row,parent_row));
 CREATE TABLE IF NOT EXISTS schema_parent_additions(fact_row INTEGER NOT NULL, parent_row INTEGER NOT NULL, PRIMARY KEY(fact_row,parent_row));
 CREATE TABLE IF NOT EXISTS fact_feedback(
 event_id INTEGER PRIMARY KEY, fact_row INTEGER NOT NULL, action TEXT NOT NULL, revision TEXT NOT NULL,
 actor TEXT NOT NULL, reason TEXT NOT NULL, replacement_id TEXT NOT NULL, recorded_at TEXT NOT NULL,
 UNIQUE(fact_row,action,revision,actor,reason,replacement_id));
 CREATE TABLE IF NOT EXISTS discoveries(repo TEXT NOT NULL, key TEXT NOT NULL, PRIMARY KEY(repo,key));`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Scanned(ctx context.Context, key string) (bool, error) {
	if !s.lifecycle {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM discoveries WHERE repo=? AND key=?)", s.repo, key).Scan(&exists)
	return exists, err
}

func (s *Store) SaveDiscovery(ctx context.Context, key string, facts []Fact) error {
	if key == "" {
		return fmt.Errorf("a discovery key is required")
	}
	return s.save(ctx, key, facts, nil)
}

func (s *Store) Save(ctx context.Context, facts []Fact) error { return s.save(ctx, "", facts, nil) }

func (s *Store) Learn(ctx context.Context, tree Tree, source, scope, property string) ([]Checked, error) {
	facts, err := tree.Learn(ctx, source, scope, property)
	if err != nil {
		return nil, err
	}
	report := make([]Checked, len(facts))
	if err := s.save(ctx, "", facts, report); err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Store) save(ctx context.Context, key string, facts []Fact, report []Checked) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if key != "" {
		result, err := tx.ExecContext(ctx, "INSERT INTO discoveries(repo,key) VALUES(?,?) ON CONFLICT DO NOTHING", s.repo, key)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count == 0 {
			return err
		}
	}
	ids := make([]string, len(facts))
	for i, f := range facts {
		if f.discussionNotice != 0 {
			_, err := tx.ExecContext(ctx, "INSERT INTO discussion_unavailable(repo,source_id,revision,observed_at,scope_digest) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING", s.repo, f.ID, f.Revision, f.ObservedAt, f.editionScopeDigest)
			if err != nil {
				return err
			}
			ids[i] = f.ID
			continue
		}
		f = observationBody(f)
		f.ID, err = stableFactID(f)
		if err != nil {
			return err
		}
		ids[i] = f.ID
		if report != nil {
			report[i].Status = StatusVerified
		}
		body, err := json.Marshal(f)
		if err != nil {
			return err
		}
		var rowid int64
		var recordedAt any
		if f.schema() {
			recordedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO facts(repo,id,review,body,recorded_at) VALUES(?,?,?,?,COALESCE(?,CURRENT_TIMESTAMP))
   ON CONFLICT(repo,id) DO NOTHING RETURNING rowid`, s.repo, f.ID, f.ReviewID, string(body), recordedAt).Scan(&rowid)
		existing := err == sql.ErrNoRows
		if existing {
			var revoked sql.NullString
			err = tx.QueryRowContext(ctx, "SELECT rowid,revoked_at FROM facts WHERE repo=? AND id=?", s.repo, f.ID).Scan(&rowid, &revoked)
			if err == nil && f.explicitSchemaLearn {
				if !revoked.Valid {
					at := time.Now().UTC().Format(time.RFC3339Nano)
					_, err = tx.ExecContext(ctx, "UPDATE schema_withdrawals SET learned_at=? WHERE fact_row=? AND learned_at IS NULL", at, rowid)
					if err == nil {
						_, err = tx.ExecContext(ctx, "UPDATE schema_parents SET learned_at=? WHERE fact_row=? AND learned_at IS NULL", at, rowid)
					}
				} else if report != nil {
					report[i].Status, report[i].Detail = StatusWithdrawn, "this memory ID was previously withdrawn and remains inactive"
				}
			}
		} else if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO fact_search(rowid,text) VALUES(?,?)", rowid, searchText(f))
		}
		if err == nil && len(f.schemaParents) != 0 {
			err = s.saveSchemaParents(ctx, tx, rowid, f.schemaParents, existing)
		}
		if err != nil {
			return err
		}
		if f.Category == CategoryDiscussionSource || f.ReviewReply != nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO discussion_edits(fact_row,revision,body) VALUES(?,?,?) ON CONFLICT DO NOTHING", rowid, f.Revision, string(body))
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO fact_revisions(fact_row,revision) VALUES(?,?) ON CONFLICT DO NOTHING", rowid, f.Revision)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO fact_observations(fact_row,revision,body) VALUES(?,?,?) ON CONFLICT DO NOTHING", rowid, f.Revision, string(body))
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for i, id := range ids {
		facts[i].ID = id
		if report != nil {
			report[i].Fact = facts[i]
		}
	}
	return nil
}

func observationBody(f Fact) Fact {
	f.Feedback, f.contextStatus, f.contextConflicts = nil, "", nil
	f.ScopeWitnesses = nil
	clear := func(c Citation) Citation {
		c.CurrentPath, c.CurrentStartLine, c.CurrentEndLine = "", 0, 0
		return c
	}
	return mapCitations(f, clear)
}

func mapCitations(f Fact, project func(Citation) Citation) Fact {
	f.Citations = slices.Clone(f.Citations)
	f.Conditions = slices.Clone(f.Conditions)
	for i, c := range f.Citations {
		f.Citations[i] = project(c)
	}
	for i, c := range f.Conditions {
		if c.Citation != nil {
			out := project(*c.Citation)
			f.Conditions[i].Citation = &out
		}
	}
	return f
}

func stableFactID(f Fact) (string, error) {
	if f.ReviewReply != nil {
		return reviewNoteID(f.ReviewID, f.ReviewReply.ID), nil
	}
	if ref := f.discussionRef(); ref != "" {
		// One externally addressable source across edits, even when several
		// edits were imported while the accepted Git tree stayed identical.
		return discussionSourceID(ref), nil
	}
	f = observationBody(f)
	f.ID, f.Revision, f.ObservedAt = "", "", ""
	f.AcceptedCall = nil // Derived from the accepted citation pair, including in older databases.
	if f.Origin == "discovery" && slices.ContainsFunc(f.Citations, func(c Citation) bool { return c.Kind == "discussion" }) {
		// Refreshing an inherited deadline preserves the ID and its feedback.
		f.ExpiresAt = ""
	}
	f = mapCitations(f, func(c Citation) Citation {
		if c.live() {
			c.Revision, c.Quote, c.StartLine, c.EndLine = "", "", 0, 0
		}
		return c
	})
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return Digest(raw), nil
}

func searchText(f Fact) string {
	terms := []string{f.Scope, f.Source, f.Property, f.Keyword, string(f.Value), f.Finding, f.Reason, f.Category, f.Claim, f.Subject, f.Predicate}
	if f.ReviewReply != nil {
		terms = append(terms, f.DiscussionBody)
	}
	terms = append(terms, f.Identifiers...)
	f.eachCitation(func(c Citation) { terms = append(terms, c.Path, c.Anchor, c.Reference, c.Quote) })
	for _, cond := range f.Conditions {
		terms = append(terms, cond.Description)
	}
	return strings.Join(terms, " ")
}

// A discussion keeps one ID through edits. Match the edition chosen for
// display, including its full original body beyond the review excerpts.
func (f Fact) MatchesDiscussionSearch(phrase string) bool {
	q := strings.ToLower(phrase)
	return strings.Contains(strings.ToLower(f.DiscussionBody), q) || strings.Contains(strings.ToLower(searchText(f)), q)
}

func (s *Store) Revoke(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE facts SET revoked_at=COALESCE(revoked_at,?) WHERE repo=? AND id=?", time.Now().UTC().Format(time.RFC3339Nano), s.repo, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("unknown memory ID %q", id)
	}
	return nil
}

func (s *Store) List(ctx context.Context, review, search string) ([]Fact, error) {
	return s.ListAt(ctx, review, search, time.Time{})
}

// ListAt applies local withdrawals as of the requested review time.
func (s *Store) ListAt(ctx context.Context, review, search string, cutoff time.Time) ([]Fact, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	facts, err := s.list(ctx, tx, review, search, "", cutoff)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return facts, nil
}

func withdrawnBy(at sql.NullString, cutoff time.Time) bool {
	if !at.Valid {
		return false
	}
	if cutoff.IsZero() {
		return true
	}
	when, err := time.Parse(time.RFC3339Nano, at.String)
	if err != nil {
		when, err = time.Parse(time.DateTime, at.String) // Legacy SQLite CURRENT_TIMESTAMP (UTC).
	}
	return err != nil || !when.After(cutoff)
}

func (s *Store) list(ctx context.Context, tx *sql.Tx, review, search, id string, cutoff time.Time) ([]Fact, error) {
	body := "facts.body"
	join := ""
	if s.lifecycle {
		body = "COALESCE(o.body,facts.body)"
		join = " LEFT JOIN fact_observations AS o ON o.fact_row=facts.rowid AND o.revision=r.revision"
	}
	if s.discussionEdits {
		body = "COALESCE(e.body,o.body,facts.body)"
		join += " LEFT JOIN discussion_edits AS e ON e.fact_row=facts.rowid AND e.revision=r.revision"
	}
	query := "SELECT " + body + ", r.revision, facts.recorded_at, facts.revoked_at FROM facts JOIN fact_revisions AS r ON r.fact_row=facts.rowid" + join + " WHERE repo=?"
	if cutoff.IsZero() {
		query += " AND revoked_at IS NULL"
	}
	args := []any{s.repo}
	if s.schemaParents || !cutoff.IsZero() {
		var at any
		if !cutoff.IsZero() {
			at = cutoff.UTC().Format(withdrawalTimeFormat)
		}
		// Databases predating parent mappings contain only root grants.
		links := "SELECT NULL,NULL,NULL,NULL WHERE 0"
		parents := ""
		if s.schemaParents {
			links = schemaParentLinks
			if s.schemaAdditions {
				links += " " + additionalSchemaLinks
			}
			parents = schemaParentAuthorization
			query += " AND (NOT EXISTS(SELECT 1 FROM links d WHERE d.child=facts.rowid) OR facts.rowid IN (SELECT id FROM supported))"
		}
		query = fmt.Sprintf(schemaRootAuthorization, links) + parents + query + " AND NOT EXISTS(SELECT 1 FROM interrupted i WHERE i.id=facts.rowid)"
		args = []any{s.repo, at, s.repo}
	}
	if s.schemaWithdrawals {
		during := "w.learned_at IS NULL"
		if !cutoff.IsZero() {
			during = "ocr_withdrawal_time(w.withdrawn_at)<=? AND (w.learned_at IS NULL OR ?<ocr_withdrawal_time(w.learned_at))"
			at := cutoff.UTC().Format(withdrawalTimeFormat)
			args = append(args, at, at)
		}
		query += " AND NOT EXISTS(SELECT 1 FROM schema_withdrawals w WHERE w.fact_row=facts.rowid AND " + during + ")"
	}
	if id != "" {
		query += " AND id=?"
		args = append(args, id)
	} else if canonical := DiscussionReviewID(review); canonical != "" && canonical != review {
		query += " AND (review='' OR review=? OR review=?)"
		args = append(args, review, canonical)
	} else {
		query += " AND (review='' OR review=?)"
		args = append(args, review)
	}
	if search != "" {
		if utf8.RuneCountInString(search) < 3 {
			return nil, fmt.Errorf("memory search requires at least three characters")
		}
		query += " AND (facts.rowid IN (SELECT rowid FROM fact_search WHERE fact_search MATCH ?)"
		args = append(args, "\""+strings.ReplaceAll(search, "\"", "\"\"")+"\"")
		if s.discussionEdits {
			query += " OR EXISTS(SELECT 1 FROM discussion_edits AS de WHERE de.fact_row=facts.rowid AND instr(ocr_discussion_lower(json_extract(de.body,'$.discussion_body')),?)>0)"
			args = append(args, strings.ToLower(search))
		}
		query += ")"
	}
	rows, err := tx.QueryContext(ctx, query+" ORDER BY (review <> '') DESC, id, r.revision", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Fact{}
	for rows.Next() {
		var body, revision, recordedAt string
		var revokedAt sql.NullString
		var f Fact
		if err = rows.Scan(&body, &revision, &recordedAt, &revokedAt); err != nil {
			return nil, err
		}
		if withdrawnBy(revokedAt, cutoff) {
			continue
		}
		if err = json.Unmarshal([]byte(body), &f); err != nil {
			return nil, fmt.Errorf("decode memory record: %w", err)
		}
		if f.hasFutureLanding(cutoff) {
			continue
		}
		f.Revision = revision
		if f.IsDecision() && f.ObservedAt == "" {
			if at, err := time.Parse(time.DateTime, recordedAt); err == nil {
				f.ObservedAt = at.Format(time.RFC3339)
			}
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(result) == 0 || !s.lifecycle {
		return result, nil
	}
	if s.discussionEdits {
		rows, err := tx.QueryContext(ctx, `SELECT id, revoked_at FROM facts WHERE repo=? AND revoked_at IS NOT NULL AND json_extract(body,'$.category')=?`, s.repo, CategoryDiscussionSource)
		if err != nil {
			return nil, err
		}
		revoked := map[string]bool{}
		for rows.Next() {
			var id string
			var at sql.NullString
			if err := rows.Scan(&id, &at); err != nil {
				rows.Close()
				return nil, err
			}
			if withdrawnBy(at, cutoff) {
				revoked[id] = true
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if len(revoked) != 0 {
			result = slices.DeleteFunc(result, func(f Fact) bool {
				return slices.ContainsFunc(f.Citations, func(c Citation) bool {
					return c.Kind == "discussion" && revoked[discussionSourceID(c.Reference)]
				})
			})
		}
	}
	feedback, err := s.readFeedback(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	for i := range result {
		result[i].Feedback = feedback[result[i].ID]
	}
	if s.editionNotices {
		return s.withEditionNotices(ctx, tx, result)
	}
	return result, nil
}

// Attach withdrawals and exact-edition deadlines for references in this
// query. The notice table contains no discussion text or file paths.
func (s *Store) withEditionNotices(ctx context.Context, tx *sql.Tx, facts []Fact) ([]Fact, error) {
	referenced := map[string]map[string]bool{}
	expiryIDs := map[string]bool{}
	include := func(id, scope string) {
		if referenced[id] == nil {
			referenced[id] = map[string]bool{"": true}
		}
		for validPath(scope) {
			referenced[id][Digest([]byte(scope))] = true
			scope = path.Dir(scope)
		}
	}
	for _, f := range facts {
		for _, c := range f.Citations {
			if c.Kind != "discussion" {
				continue
			}
			scope := c.Path
			if scope == "" {
				scope = f.Scope
			}
			ref := discussionSourceID(c.Reference)
			include(ref, scope)
			file := c.Path
			if file == "" {
				file = f.Source
			}
			referenced[ref][exactDiscussionScope(file)] = true
			edition := discussionExpiryID(c)
			include(edition, scope)
			expiryIDs[edition] = true
		}
	}
	if len(referenced) == 0 {
		return facts, nil
	}
	ids, _ := json.Marshal(slices.Sorted(maps.Keys(referenced)))
	rows, err := tx.QueryContext(ctx, "SELECT source_id,revision,observed_at,scope_digest FROM discussion_unavailable WHERE repo=? AND source_id IN (SELECT value FROM json_each(?))", s.repo, string(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		f := Fact{discussionNotice: noticeUnavailable, Origin: "discovery"}
		if err := rows.Scan(&f.ID, &f.Revision, &f.ObservedAt, &f.editionScopeDigest); err != nil {
			return nil, err
		}
		if referenced[f.ID][f.editionScopeDigest] {
			if expiryIDs[f.ID] {
				f.discussionNotice = noticeExpiry
			}
			facts = append(facts, f)
		}
	}
	return facts, rows.Err()
}

const (
	FeedbackConfirm = "confirm"
	FeedbackReject  = "reject"
	FeedbackReplace = "replace"
)

type Feedback struct {
	Action        string
	Reason        string
	ReplacementID string
	Actor         string
}

type FeedbackRecord struct {
	Action        string `json:"action"`
	Reason        string `json:"reason"`
	ReplacementID string `json:"replacement_id,omitempty"`
	Actor         string `json:"actor"`
	Revision      string `json:"revision"`
	RecordedAt    string `json:"recorded_at"`
}

func (s *Store) readFeedback(ctx context.Context, tx *sql.Tx, id string) (map[string][]FeedbackRecord, error) {
	const selectFeedback = `SELECT facts.id, action, revision, actor, reason, replacement_id, fact_feedback.recorded_at
 FROM fact_feedback JOIN facts ON facts.rowid=fact_feedback.fact_row WHERE facts.repo=?`
	query := selectFeedback
	args := []any{s.repo}
	if id != "" {
		query += " AND facts.id=?"
		args = append(args, id)
	}
	rows, err := tx.QueryContext(ctx, query+" ORDER BY event_id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := make(map[string][]FeedbackRecord)
	for rows.Next() {
		var record FeedbackRecord
		var id string
		if err := rows.Scan(&id, &record.Action, &record.Revision, &record.Actor, &record.Reason, &record.ReplacementID, &record.RecordedAt); err != nil {
			return nil, err
		}
		found[id] = append(found[id], record)
	}
	return found, rows.Err()
}

func (s *Store) Feedback(ctx context.Context, tree Tree, id string, input Feedback) (Fact, error) {
	if !validDigest(id) || !validOID(tree.Commit) || tree.Workspace {
		return Fact{}, fmt.Errorf("feedback requires a full memory ID and a committed revision")
	}
	switch input.Action {
	case FeedbackConfirm, FeedbackReject:
		if input.ReplacementID != "" {
			return Fact{}, fmt.Errorf("a replacement ID requires replace feedback")
		}
	case FeedbackReplace:
		if !validDigest(input.ReplacementID) || input.ReplacementID == id {
			return Fact{}, fmt.Errorf("replace feedback requires a different full memory ID")
		}
	default:
		return Fact{}, fmt.Errorf("unknown feedback action %q", input.Action)
	}
	reason, actor := strings.TrimSpace(input.Reason), strings.TrimSpace(input.Actor)
	if reason == "" || len(reason) > 1000 || len(actor) > 160 {
		return Fact{}, fmt.Errorf("feedback requires a short reason and may include an actor")
	}
	if actor == "" {
		actor = "local"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Fact{}, err
	}
	defer tx.Rollback()
	r := newRechecker(ctx, tree)
	load := func(id string) (Checked, error) {
		facts, err := s.list(ctx, tx, "", "", id, tree.Now)
		if err != nil {
			return Checked{}, err
		}
		facts = slices.DeleteFunc(facts, func(f Fact) bool { return !r.ancestor(f.Revision) })
		if len(facts) == 0 {
			return Checked{}, fmt.Errorf("no observation of memory %q at this revision", id)
		}
		// A reply only has a meaning in the thread that contains its finding.
		if f := facts[0]; f.ReviewReply != nil && f.ReviewReply.ReplyTo != "" {
			root, err := s.list(ctx, tx, f.ReviewID, "", reviewNoteID(f.ReviewID, f.ReviewReply.ReplyTo), tree.Now)
			if err != nil {
				return Checked{}, err
			}
			facts = append(facts, root...)
		}
		// Checking just a derived fact would miss a correction, expiry or
		// rejection recorded against the original discussion.
		sources := map[string]bool{}
		for _, f := range facts {
			if f.Category == CategoryDiscussionSource {
				continue
			}
			for _, c := range f.Citations {
				if c.Kind != "discussion" || sources[c.Reference] {
					continue
				}
				sources[c.Reference] = true
				editions, err := s.list(ctx, tx, "", "", discussionSourceID(c.Reference), tree.Now)
				if err != nil {
					return Checked{}, err
				}
				facts = append(facts, editions...)
			}
		}
		checked, err := tree.Check(ctx, facts)
		if err != nil {
			return Checked{}, err
		}
		for _, c := range checked {
			if c.ID == id {
				return c, nil
			}
		}
		return Checked{}, fmt.Errorf("no eligible observation of memory %q at this revision and time", id)
	}
	target, err := load(id)
	if err != nil {
		return Fact{}, err
	}
	if input.Action == FeedbackConfirm && !confirmable(target) {
		return Fact{}, fmt.Errorf("confirmation requires current code or configuration and current condition anchors")
	}
	record := FeedbackRecord{input.Action, reason, input.ReplacementID, actor, tree.Commit, time.Now().UTC().Format(time.RFC3339Nano)}
	if input.Action == FeedbackReplace {
		replacement, err := load(input.ReplacementID)
		if err != nil {
			return Fact{}, err
		}
		sourceReplacement := target.Category == CategoryDiscussionSource && replacement.Status == StatusDiscussionSource && target.Scope == replacement.Scope
		if !sourceReplacement && (!correspondingRecords(target.Fact, replacement.Fact) || !confirmable(replacement)) {
			return Fact{}, fmt.Errorf("replacement requires current evidence for the same scope and subject")
		}
		if !sourceReplacement {
			confirm := record
			confirm.Action, confirm.ReplacementID = FeedbackConfirm, ""
			if err := s.writeFeedback(ctx, tx, replacement.ID, confirm); err != nil {
				return Fact{}, err
			}
		}
	}
	if err = s.writeFeedback(ctx, tx, id, record); err != nil {
		return Fact{}, err
	}
	if err = tx.Commit(); err != nil {
		return Fact{}, err
	}
	target.Feedback = append(slices.Clone(target.Feedback), record)
	if input.Action == FeedbackConfirm && !target.schema() {
		target.Authority = AuthorityConfirmed
	}
	return target.Fact, nil
}

func confirmable(c Checked) bool {
	switch c.Status {
	case StatusVerified:
		return true
	case StatusCandidate, StatusCurrent:
	default:
		return false
	}
	if !slices.ContainsFunc(c.Citations, func(c Citation) bool {
		return c.CurrentPath != "" && (c.Kind == "code" || c.Kind == "config")
	}) {
		return false
	}
	return !slices.ContainsFunc(c.Conditions, func(c Condition) bool {
		return c.Citation == nil || c.Citation.CurrentPath == ""
	})
}

func (s *Store) writeFeedback(ctx context.Context, tx *sql.Tx, id string, r FeedbackRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO fact_feedback(fact_row,action,revision,actor,reason,replacement_id,recorded_at)
 SELECT rowid,?,?,?,?,?,? FROM facts WHERE repo=? AND id=? ON CONFLICT DO NOTHING`,
		r.Action, r.Revision, r.Actor, r.Reason, r.ReplacementID, r.RecordedAt, s.repo, id)
	return err
}
