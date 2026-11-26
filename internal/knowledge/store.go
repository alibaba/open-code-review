// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

type Store struct {
	db        *sql.DB
	repo      string
	lifecycle bool
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
	var lifecycle bool
	if write {
		err = initialize(ctx, db)
		lifecycle = err == nil
	} else {
		err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE name='fact_observations')").Scan(&lifecycle)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, repo: repo, lifecycle: lifecycle}, nil
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
	return s.save(ctx, key, facts)
}

func (s *Store) Save(ctx context.Context, facts []Fact) error { return s.save(ctx, "", facts) }

func (s *Store) save(ctx context.Context, key string, facts []Fact) error {
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
		f = observationBody(f)
		f.ID, err = stableFactID(f)
		if err != nil {
			return err
		}
		ids[i] = f.ID
		body, err := json.Marshal(f)
		if err != nil {
			return err
		}
		var rowid int64
		err = tx.QueryRowContext(ctx, `INSERT INTO facts(repo,id,review,body) VALUES(?,?,?,?)
   ON CONFLICT(repo,id) DO NOTHING RETURNING rowid`, s.repo, f.ID, f.ReviewID, string(body)).Scan(&rowid)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, "SELECT rowid FROM facts WHERE repo=? AND id=?", s.repo, f.ID).Scan(&rowid)
		} else if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO fact_search(rowid,text) VALUES(?,?)", rowid, searchText(f))
		}
		if err != nil {
			return err
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
	}
	return nil
}

func observationBody(f Fact) Fact {
	f.Feedback, f.contextStatus, f.contextConflicts = nil, "", nil
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
	f = observationBody(f)
	f.ID, f.Revision, f.ObservedAt = "", "", ""
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
	terms = append(terms, f.Identifiers...)
	f.eachCitation(func(c Citation) { terms = append(terms, c.Path, c.Anchor, c.Reference, c.Quote) })
	for _, cond := range f.Conditions {
		terms = append(terms, cond.Description)
	}
	return strings.Join(terms, " ")
}

func (s *Store) Revoke(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE facts SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP) WHERE repo=? AND id=?", s.repo, id)
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	facts, err := s.list(ctx, tx, review, search, "")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return facts, nil
}

func (s *Store) list(ctx context.Context, tx *sql.Tx, review, search, id string) ([]Fact, error) {
	body := "facts.body"
	join := ""
	if s.lifecycle {
		body = "COALESCE(o.body,facts.body)"
		join = " LEFT JOIN fact_observations AS o ON o.fact_row=facts.rowid AND o.revision=r.revision"
	}
	query := "SELECT " + body + ", r.revision, facts.recorded_at FROM facts JOIN fact_revisions AS r ON r.fact_row=facts.rowid" + join +
		" WHERE repo=? AND revoked_at IS NULL"
	args := []any{s.repo}
	if id != "" {
		query += " AND id=?"
		args = append(args, id)
	} else {
		query += " AND (review='' OR review=?)"
		args = append(args, review)
	}
	if search != "" {
		if utf8.RuneCountInString(search) < 3 {
			return nil, fmt.Errorf("memory search requires at least three characters")
		}
		query += " AND facts.rowid IN (SELECT rowid FROM fact_search WHERE fact_search MATCH ?)"
		args = append(args, "\""+strings.ReplaceAll(search, "\"", "\"\"")+"\"")
	}
	rows, err := tx.QueryContext(ctx, query+" ORDER BY (review <> '') DESC, id, r.revision", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Fact{}
	for rows.Next() {
		var body, revision, recordedAt string
		var f Fact
		if err = rows.Scan(&body, &revision, &recordedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &f); err != nil {
			return nil, fmt.Errorf("decode memory record: %w", err)
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
	feedback, err := s.readFeedback(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	for i := range result {
		result[i].Feedback = feedback[result[i].ID]
	}
	return result, nil
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
		facts, err := s.list(ctx, tx, "", "", id)
		if err != nil {
			return Checked{}, err
		}
		facts = slices.DeleteFunc(facts, func(f Fact) bool { return !r.ancestor(f.Revision) })
		if len(facts) == 0 {
			return Checked{}, fmt.Errorf("no observation of memory %q at this revision", id)
		}
		checked, err := tree.Check(ctx, facts)
		if err != nil {
			return Checked{}, err
		}
		return checked[0], nil
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
		if !correspondingRecords(target.Fact, replacement.Fact) || !confirmable(replacement) {
			return Fact{}, fmt.Errorf("replacement requires current evidence for the same scope and subject")
		}
		confirm := record
		confirm.Action, confirm.ReplacementID = FeedbackConfirm, ""
		if err := s.writeFeedback(ctx, tx, replacement.ID, confirm); err != nil {
			return Fact{}, err
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
