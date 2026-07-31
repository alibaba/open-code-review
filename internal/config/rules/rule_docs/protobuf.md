> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in message, field, enum, enum-value, service, or rpc names at their declaration sites; do not report spelling errors at reference sites
- Comments or option strings with spelling errors that affect readability of the public API surface

#### Field Numbers and Wire Compatibility
- Reused or renumbered field tags that break existing clients or servers (Wire Compatibility)
- Changing a field's type, label (`optional`/`repeated`/`required`), or oneof membership in a way that breaks wire or JSON compatibility
- Deleting a field without adding both its number and name to `reserved`
- Renaming a field without `json_name` consideration when JSON clients depend on the old name
- Do not flag purely additive new fields with fresh numbers, or documentation-only comment changes

#### Message and Field Design
- Missing `optional` (proto3) where absence must be distinguishable from the zero value
- `map` used where order matters, or `repeated` used where key lookup would be clearer
- oneof fields that leave an invalid zero-state representable when an explicit sentinel was intended
- Nested messages that re-encode the same domain concept already modeled elsewhere in the package
- Do not report stylistic preference for `message` vs `group` (groups are legacy) when the schema is already consistent

#### Enums and Defaults
- First enum value is not a zero `*_UNSPECIFIED` (or equivalent) sentinel
- Relying on implicit zero defaults across schema versions when clients treat zero as meaningful data
- Inserting new enum values in the middle of an existing numeric range used by older clients
- Do not flag additive enum values appended at the end with new numbers

#### Services and RPC Design
- Non-idempotent methods modeled as if they were safe to retry without client-visible side effects
- Multiple rpcs sharing the same request or response message type when distinct contracts would prevent accidental field coupling
- Unbounded client/server streaming without documented flow control, page size, or deadline expectations
- Missing request or response message wrappers that force primitive/scalar request bodies
- Do not flag standard google.api annotations or well-known types used correctly

#### Security and Resource Limits
- `google.protobuf.Any` accepted from untrusted input without type allowlisting
- Unbounded `repeated`/`map` fields or recursive message depth on untrusted payloads with no application-level limits
- Secrets, tokens, or credentials embedded in field defaults, examples, or comments
- File paths, URLs, or SQL fragments carried as unconstrained strings without validation guidance at the service boundary
- Do not report when limits are enforced outside the schema and that boundary is clearly documented
