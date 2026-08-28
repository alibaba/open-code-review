> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat wire-compatibility breaks as blocking, and naming or layout preferences as non-blocking.

#### Field IDs and Wire Compatibility
- Reusing the id of a deleted field; Thrift has no `reserved` keyword, so a retired id must be held open by a placeholder field carrying a "do not reuse this id" comment
- Renumbering an existing field, or inserting a new field by shifting the ids of everything after it, instead of appending the next unused id
- Changing the declared type of an existing id, including `i32` to `i64` and swapping an enum for the integer that backs it; the type byte travels in the field header
- Deleting a field that peers still send without leaving its id held open for the same reason
- Do not report purely additive fields that take a fresh unused id, comment-only edits, or `namespace` and `include` changes

#### Requiredness and Defaults
- Adding a `required` field to an existing struct: `required` is permanent and unskippable, so every existing peer fails to deserialize in both directions the moment one side adopts it
- Flipping an existing field between `required` and `optional`, which changes what a peer is allowed to omit
- Changing the default value of an existing optional field; an unset field and a field holding the default are indistinguishable to the peer, so the change lands silently
- Fields left with default requiredness where absence must be distinguishable from the zero value
- Do not report the choice of default requiredness itself when the file is internally consistent

#### Services and Methods
- Renaming a service method: method names travel on the wire in `TMessageBegin`, unlike field names, so a rename breaks every existing caller
- Changing the ids of an existing method's parameters, or adding a parameter declared `required`
- Adding an exception to an existing `throws` clause that older clients have no branch to decode
- Changing a method to or from `oneway`, which changes whether the caller waits for a reply at all
- Do not report new methods appended to an existing service; those are backward compatible

#### Enums and Constants
- Enum members declared without explicit numeric values, which makes every value positional and shifts them all on the first insertion
- Inserting a new enum member into the middle of an existing numeric range instead of appending
- Code that treats an unknown enum value as unreachable; peers on a newer schema will send values this build has never seen
- Do not report enum members appended with new explicit values

#### Security and Resource Limits
- Unbounded `list`, `set`, `map`, `string`, or `binary` fields carried over an untrusted transport with no application-level size limit
- Recursive struct definitions with no documented depth bound on untrusted input
- Secrets, tokens, or credentials embedded in constants, default values, or comments
- `string` used to carry non-UTF-8 bytes where `binary` is meant, at a boundary that validates neither
- Do not report when limits are enforced by transport or server configuration and that boundary is clearly documented
