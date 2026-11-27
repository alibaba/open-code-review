// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"time"

	"modernc.org/sqlite"
)

const withdrawalTimeFormat = "2006-01-02T15:04:05.000000000Z"

func init() {
	// Keep nanosecond precision when comparing timestamps written in different
	// UTC formats. SQLite's datetime functions have millisecond precision.
	sqlite.MustRegisterDeterministicScalarFunction("ocr_withdrawal_time", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if args[0] == nil {
			return nil, nil
		}
		stamp, _ := args[0].(string)
		at, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			at, err = time.Parse(time.DateTime, stamp)
		}
		if err != nil {
			return nil, err
		}
		return at.UTC().Format(withdrawalTimeFormat), nil
	})
}

// A newly learned root cannot bridge an earlier withdrawal for the same
// source, consumer and property. Legacy stores recorded whole seconds; only
// the end of that second establishes the grant.
const schemaRootAuthorization = `WITH RECURSIVE
 local_facts(id,recorded_at) AS (
  SELECT rowid,CASE WHEN length(recorded_at)=19 THEN datetime(recorded_at,'+1 second') ELSE recorded_at END
  FROM facts WHERE repo=?
 ),
 links(child,parent,learned_at,since) AS (%s),
 clock(at) AS (VALUES(?)),
 interrupted(id) AS (
  SELECT l.id FROM local_facts l JOIN facts f ON f.rowid=l.id CROSS JOIN clock
  WHERE clock.at IS NOT NULL AND clock.at<ocr_withdrawal_time(l.recorded_at)
  AND json_extract(f.body,'$.keyword') IN ('const','enum')
  AND NOT EXISTS(SELECT 1 FROM links d WHERE d.child=l.id)
  AND EXISTS(SELECT 1 FROM facts old WHERE old.repo=f.repo AND old.rowid<l.id AND old.revoked_at IS NOT NULL
   AND ocr_withdrawal_time(old.revoked_at)<=clock.at
   AND json_extract(old.body,'$.source')=json_extract(f.body,'$.source')
   AND json_extract(old.body,'$.scope')=json_extract(f.body,'$.scope')
   AND json_extract(old.body,'$.property')=json_extract(f.body,'$.property'))
 ) `

const schemaParentLinks = `SELECT d.fact_row,d.parent_row,d.learned_at,NULL
 FROM schema_parents d JOIN local_facts f ON f.id=d.fact_row`

// A derived child needs one available parent or its own explicit grant. Its
// direct withdrawal always wins. UNION terminates even when links form a cycle.
const schemaParentAuthorization = `, nodes(id) AS (SELECT child FROM links UNION SELECT parent FROM links),
 eligible(id) AS (
  SELECT n.id FROM nodes n JOIN facts f ON f.rowid=n.id CROSS JOIN clock
  WHERE (f.revoked_at IS NULL OR clock.at<ocr_withdrawal_time(f.revoked_at))
  AND NOT EXISTS(SELECT 1 FROM interrupted i WHERE i.id=n.id)
  AND NOT EXISTS(SELECT 1 FROM schema_withdrawals w WHERE w.fact_row=n.id
   AND (clock.at IS NULL OR ocr_withdrawal_time(w.withdrawn_at)<=clock.at)
   AND (w.learned_at IS NULL OR clock.at<ocr_withdrawal_time(w.learned_at)))
 ), supported(id) AS (
  SELECT e.id FROM eligible e CROSS JOIN clock WHERE NOT EXISTS(SELECT 1 FROM links d WHERE d.child=e.id)
   OR EXISTS(SELECT 1 FROM links d WHERE d.child=e.id AND d.learned_at IS NOT NULL
    AND (clock.at IS NULL OR ocr_withdrawal_time(d.learned_at)<=clock.at))
  UNION
  SELECT d.child FROM supported p JOIN links d ON d.parent=p.id JOIN eligible e ON e.id=d.child CROSS JOIN clock
  WHERE clock.at IS NULL OR d.since IS NULL OR ocr_withdrawal_time(d.since)<=clock.at
 ) `

// A later mapping cannot fill a gap before its parent existed.
const additionalSchemaLinks = `UNION ALL
 SELECT a.fact_row,a.parent_row,NULL,p.recorded_at
 FROM schema_parent_additions a JOIN local_facts p ON p.id=a.parent_row`

func (s *Store) saveSchemaParents(ctx context.Context, tx *sql.Tx, row int64, parents []string, existing bool) error {
	ids, _ := json.Marshal(parents)
	query := `INSERT INTO schema_parents(fact_row,parent_row)
 SELECT ?1,f.rowid FROM facts f WHERE f.repo=?2 AND f.id IN (SELECT value FROM json_each(?3))`
	if existing {
		// A record first learned explicitly already has its own authorization.
		// Additional mappings apply only to children originally derived here.
		query = `INSERT INTO schema_parent_additions(fact_row,parent_row)
 SELECT ?1,f.rowid FROM facts f WHERE f.repo=?2 AND f.rowid!=?1 AND f.id IN (SELECT value FROM json_each(?3))
 AND EXISTS(SELECT 1 FROM schema_parents d WHERE d.fact_row=?1)
 AND NOT EXISTS(SELECT 1 FROM schema_parents d WHERE d.fact_row=?1 AND d.parent_row=f.rowid)`
	}
	_, err := tx.ExecContext(ctx, query+" ON CONFLICT DO NOTHING", row, s.repo, string(ids))
	return err
}
