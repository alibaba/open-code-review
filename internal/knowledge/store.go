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
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	repo string
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
	if write {
		err = initialize(ctx, db)
	} else {
		err = db.PingContext(ctx)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, repo: repo}, nil
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
 CREATE VIRTUAL TABLE IF NOT EXISTS fact_search USING fts5(text, tokenize='trigram');`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(ctx context.Context, facts []Fact) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range facts {
		f := &facts[i]
		key := *f
		key.ID = ""
		key.Revision = ""
		body, err := json.Marshal(key)
		if err != nil {
			return err
		}
		f.ID = Digest(body)
		body, err = json.Marshal(f)
		if err != nil {
			return err
		}
		var rowid int64
		err = tx.QueryRowContext(ctx, `INSERT INTO facts(repo,id,review,body) VALUES(?,?,?,?)
   ON CONFLICT(repo,id) DO NOTHING RETURNING rowid`, s.repo, f.ID, f.ReviewID, string(body)).Scan(&rowid)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, "SELECT rowid FROM facts WHERE repo=? AND id=?", s.repo, f.ID).Scan(&rowid)
		} else if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO fact_search(rowid,text) VALUES(?,?)", rowid, strings.Join([]string{f.Scope, f.Source, f.Property, f.Keyword, string(f.Value), f.Finding, f.Reason}, " "))
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO fact_revisions(fact_row,revision) VALUES(?,?) ON CONFLICT DO NOTHING", rowid, f.Revision)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
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
	query := "SELECT body, revision FROM facts JOIN fact_revisions ON fact_row=facts.rowid WHERE repo=? AND (review='' OR review=?) AND revoked_at IS NULL"
	args := []any{s.repo, review}
	if search != "" {
		if utf8.RuneCountInString(search) < 3 {
			return nil, fmt.Errorf("memory search requires at least three characters")
		}
		query += " AND facts.rowid IN (SELECT rowid FROM fact_search WHERE fact_search MATCH ?)"
		args = append(args, "\""+strings.ReplaceAll(search, "\"", "\"\"")+"\"")
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY (review <> '') DESC, id, revision", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Fact{}
	for rows.Next() {
		var body, revision string
		var f Fact
		if err = rows.Scan(&body, &revision); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &f); err != nil {
			return nil, fmt.Errorf("decode memory record: %w", err)
		}
		f.Revision = revision
		result = append(result, f)
	}
	return result, rows.Err()
}
