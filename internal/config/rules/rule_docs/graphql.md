> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the schema (SDL) or operation text under review; do not infer resolver behavior that lives in code outside this file.

#### Obvious Typos or Spelling Errors
- Spelling errors in type, field, enum-value, argument, input, directive, or fragment names at their declaration sites; do not report spelling errors at reference sites
- Typos in descriptions or field names that affect readability of the public API surface

#### Schema Evolution and Breaking Changes
- Removing or renaming an existing type, field, enum value, or argument that clients may already depend on
- Making a previously nullable input field or argument non-null, or adding a new required (non-null, no-default) argument to an existing field
- Changing a field or argument type to an incompatible type
- `@deprecated` applied without a non-empty `reason`
- Do not flag purely additive changes: new types, new fields, new enum values appended, or new optional (nullable / defaulted) arguments
- GraphQL has no numeric field tags — do not import Protocol Buffers field-number or renumbering concepts

#### Naming Conventions
- Types (object, interface, union, enum, input, scalar) should be `PascalCase`; fields, arguments, and input fields `camelCase`; enum values `UPPER_CASE`
- Redundant `query`/`get` prefixes on `Query` fields, `mutation`/`subscription` affixes on their root fields, and `type`/`enum`/`interface`/`union` affixes in type names
- Do not flag names that already follow these conventions merely to suggest a synonym

#### Schema Design
- Nullability that hides required-vs-optional intent (e.g. a field that can never be null typed as nullable, or a genuinely optional field typed non-null)
- Types unreachable from any root field (`Query`/`Mutation`/`Subscription`) — dead schema
- Missing descriptions on public types and fields that form the API contract
- Names prefixed with `__` (reserved for introspection)
- List fields returning a collection without a pagination or limit argument (`first`/`last`/`limit`/`after`), which allows unbounded result sets

#### Operations and Fragments
- Selecting `@deprecated` fields in queries, mutations, or fragments
- Fragment cycles, unused fragments, and unused or undefined operation variables
- Anonymous operations where a named operation aids caching and debugging
- Missing leaf selections on fields that return object/interface/union types
- Do not flag well-formed operations that merely differ in stylistic preference

#### Security and Resource Limits
- Only flag when the condition is observable in the schema or operation text under review
- Unbounded list fields (see Schema Design) or deeply nested / recursive selections with no documented depth or complexity limit (query-depth DoS surface)
- A field carrying clearly sensitive data (token, secret, password, or PII by name or description) exposed without an accompanying auth-related directive or comment
- An explicit directive, configuration, or comment in the diff that enables introspection on an untrusted surface
- Do not infer resolver-level N+1 cost, dataloader/batching usage, or runtime introspection state — those live in resolver code, not in schema or operation files
- Do not report when a limit is enforced and clearly documented outside the schema
