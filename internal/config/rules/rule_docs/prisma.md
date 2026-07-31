#### Prisma Schema Review Principles
> Favor precision over recall: report only defects likely real in the changed schema and its reachable application, migration, and datasource context. Treat data-loss, integrity, security, and compatibility findings as blocking; style-only suggestions are non-blocking. Do not duplicate errors that `prisma validate`, `prisma format`, migration tooling, or the database determine mechanically unless the diff reveals a concrete production consequence.

Before reporting a non-local claim, use `file_read` and `code_search` to inspect the datasource provider, Prisma version, migration history, generated-client call sites, queries, and existing schema conventions. Do not assume a relation action, index, native type, field, or generator setting is unsafe without evidence of the database provider, deployed data, or application behavior it affects.

#### Relations and Referential Integrity
- Relation fields whose optionality, scalar foreign-key field, `fields`, or `references` declarations disagree, allowing an invalid or unrepresentable relationship. Confirm whether the relation is relational or MongoDB and whether the affected fields are actually changed.
- `onDelete` or `onUpdate` actions that can unexpectedly delete, null, or orphan data; `SetNull` on a required relation; cascades that create destructive paths or cycles; or an action unsupported by the configured provider. Report only with evidence of affected data ownership and delete/update flows.
- Ambiguous multiple relations between the same models that lack the relation names needed to bind intended fields, or a relation name changed on only one side.
- Changes to `relationMode` that remove database-enforced foreign keys or shift integrity enforcement to Prisma without corresponding application safeguards. Do not report intentional modes used for a documented database limitation.
- Implicit many-to-many relations changed where explicit join models are required for relation metadata, referential actions, payload fields, or stable database mappings.

#### Schema Evolution and Data Compatibility
- Removing, renaming, narrowing, making required, or changing the meaning of a model, field, enum value, identifier, unique constraint, mapping, native type, or default in a way that can lose existing data, fail a migration, or break deployed client code. Inspect migrations and call sites before flagging.
- Adding a non-null field without a safe backfill/default/migration strategy for existing rows; changing a default that changes behavior for new records; or using a database default that does not match the Prisma/client expectation.
- Changing `@id`, `@@id`, `@unique`, `@@unique`, `@map`, or `@@map` in a way that alters identity, upsert/connect selectors, generated client names, existing database column/table names, or externally stored references.
- Removing or renaming an enum value that existing rows, migrations, or application code can still use. Do not flag additive enum values unless provider/application compatibility establishes a real risk.
- Native database types, `@db.*` attributes, and provider-specific features incompatible with the configured provider, deployed database version, existing values, precision/scale, length, or timezone semantics.

#### Indexes, Constraints, and Query Behavior
- Missing, removed, or incorrectly ordered `@@index`, `@@unique`, or composite constraints only when application queries, relation lookups, uniqueness guarantees, or migration behavior demonstrate a concrete need. Do not require indexes based solely on a field name or hypothetical scale.
- A unique constraint added to existing data without a deduplication/migration path, or removed when callers depend on uniqueness for authentication, tenancy, idempotency, `connect`, or `upsert`.
- Composite indexes/unique constraints that do not match changed equality, ordering, or relation access patterns, producing an unusable selector or avoidable production query regression.
- Changes to full-text, partial, clustered, sort, operator-class, or other provider-specific index options that the configured provider/version does not support or that change correctness semantics.

#### Datasource, Generators, and Deployment Safety
- Hard-coded database URLs, credentials, tokens, or connection parameters in a schema or associated Prisma configuration where they can be committed, logged, or deployed to the wrong environment. Prefer environment-based configuration and confirm the value is actually secret rather than a safe local/test URL.
- Datasource provider, schema, extension, shadow-database, direct-connection, or connection-pooling changes incompatible with the target environment or migration workflow. Check Prisma configuration and deployment setup first.
- Generator provider, output, binary-target, engine, preview-feature, or client-generation changes that can break builds, runtime deployment targets, generated imports, or CI. Do not flag a generator setting merely because it differs from a default.
- Preview or experimental features enabled, removed, or changed without compatibility evidence; ensure the project's Prisma version supports the configured feature.

#### Security and Sensitive Data
- Models or fields that newly expose secrets, credentials, access tokens, password hashes, private keys, financial data, or personal data through generated clients, logs, admin tooling, or overly broad relations. Confirm the field's actual use and access boundary.
- Missing tenant/owner relation, uniqueness, or integrity constraint only when code and schema together show that cross-tenant access, duplicate identities, or authorization bypass is possible. Do not infer authorization requirements from generic model names.
- Unsafe defaults, cascades, mappings, or nullable ownership fields that let destructive operations cross an established tenant or authorization boundary.

#### Review Scope
- Focus on correctness, integrity, migration safety, performance with demonstrated query evidence, security, and deployment compatibility.
- Do not report formatting, model/field naming preferences, relation naming style, documentation requests, or speculative indexes as findings.
- When the schema change is intentionally accompanied by a migration, generated-client update, or application code change, review the complete change set before reporting a compatibility issue.
