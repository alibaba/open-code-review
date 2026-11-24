> Favor precision over recall: report only defects that are likely to cause incorrect results, data loss, integrity violations, security issues, or material production impact. Do not report formatting, identifier quoting, or keyword casing that `sqlfluff` or a project formatter already governs, and do not rewrite a dialect-legal construct into another dialect's idiom. This rule covers SQL source (`.sql`) files -- schema migrations, stored procedures, and hand-written queries. Establish the target dialect (PostgreSQL, MySQL, SQLite, SQL Server, or another engine named in the file or surrounding migration tooling) before raising a finding that is engine-specific; stay silent when the dialect is unclear.

#### Destructive and Irreversible Migrations
- `DROP TABLE`, `DROP COLUMN`, `TRUNCATE`, or a `DELETE` without a `WHERE` that removes production data, with no archive, backfill, or down-migration in the same change
- Narrowing a column type, shortening a `VARCHAR`, or adding `NOT NULL` without a default or backfill that covers existing rows
- Renaming a table or column that application SQL still references, with no lockstep code change in the diff
- Do not flag a reversible additive migration (new nullable column, new table) whose down path is present or whose tooling records it

#### Locking and Online-Migration Hazards
- `CREATE INDEX` on a large table without `CONCURRENTLY` (PostgreSQL) or the online option the dialect provides, taking an `ACCESS EXCLUSIVE` or table-rewrite lock
- `ALTER TABLE ... ADD COLUMN ... DEFAULT` on engines that rewrite the table for a non-nullable default (PostgreSQL before 11, older MySQL), or an `ALTER` that cannot run in place
- A migration that holds an open transaction across a data backfill of a large table, or that mixes a schema change and long DML in one transaction
- `lock_timeout` / `statement_timeout` absent on a lock-sensitive migration when sibling migrations in the repo set them; do not require them on empty or small lookup tables
- Do not flag `CONCURRENTLY`, MySQL `ALGORITHM=INPLACE` / `LOCK=NONE`, or SQL Server `ONLINE` index operations that are already used

#### NULL Semantics
- `= NULL` or `!= NULL` instead of `IS NULL` / `IS NOT NULL`, which never matches
- `NOT IN (subquery)` whose subquery can return `NULL`, making the predicate unknown for every row
- Aggregates (`SUM`, `AVG`) or `UNIQUE` constraints over columns that contain `NULL` where the surrounding code treats `NULL` as zero or as a competing key
- `COALESCE` / `NULLIF` / `IS DISTINCT FROM` used in a way that collapses a meaningful `NULL` into a sentinel the rest of the query does not expect

#### Joins, Filters, and Result Correctness
- A `JOIN` missing its predicate, producing an accidental cross join
- A `LEFT` / `RIGHT JOIN` whose outer side is required in `WHERE`, turning it into an inner join
- A tenant, org, or soft-delete filter present on sibling queries but omitted here, so rows leak across tenants or include deleted rows
- `LIMIT` / `TOP` / `FETCH` without `ORDER BY` on a query whose caller needs a deterministic slice
- `UNION` used where `UNION ALL` is intended (or the reverse) when duplicate rows change the result the caller relies on

#### Constraints and Data Integrity
- A column the application treats as required, unique, or a foreign key with no matching `NOT NULL`, `UNIQUE`, or `REFERENCES` in the schema change
- `ON DELETE` / `ON UPDATE CASCADE` (or `SET NULL`) on a parent that will delete or null children the application does not expect
- A `CHECK` or exclusion constraint that does not match the values the application writes, or a `UNIQUE` that existing rows will violate with no dedup step
- Do not demand constraints that the dialect or an existing migration policy deliberately defers (SQLite foreign-key enforcement off, deferred PostgreSQL foreign keys)

#### Performance and Index Use
- A function or cast on an indexed column in `WHERE` / `JOIN` that makes the predicate non-sargable (wrapping a timestamp column in `DATE()`, applying `LOWER()` to a column that is not citext)
- Leading-wildcard `LIKE ('%foo')` or a non-prefix pattern on a large table with no supporting trigram or full-text index
- `SELECT *` in a view, materialized view, or hot query whose caller needs a stable column list, so adding a column later breaks consumers
- An unbounded `SELECT` into application code with no `LIMIT` or pagination where the table size makes a full result material
- Do not flag `SELECT *` in a one-off admin script or a migration that inspects rows locally

#### Dynamic SQL and Privilege Boundaries
- String-concatenated `EXECUTE` / `EXEC` / `sp_executesql` / `PREPARE` whose SQL text includes untrusted values instead of parameter placeholders
- `SECURITY DEFINER` (PostgreSQL) or `SQL SECURITY DEFINER` (MySQL) routines that interpolate arguments into dynamic SQL, or that omit a fixed `search_path`, allowing privilege escalation
- `GRANT ALL` or a routine running as a superuser / `dbo` when a narrower owner would do
- Do not flag parameterized `sp_executesql`, `PREPARE` with bound parameters, or static SQL inside a definer routine whose `search_path` is pinned
