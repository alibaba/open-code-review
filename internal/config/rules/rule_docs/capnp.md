> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat wire-compatibility breaks as blocking, and naming or layout preferences as non-blocking.

#### Ordinals and Wire Compatibility
- Changing the `@N` ordinal of an existing field or method; the ordinal is that member's fixed slot, so it is the one thing that must never move
- Filling an ordinal left behind by a removed member instead of holding it with an `obsolete`/`obsoleteN` placeholder of the original width (`obsoleteSave @7 :AnyPointer`, `obsolete3 @3 :Bool`)
- Deleting a member outright rather than renaming it to `obsolete*` and leaving its ordinal and type in place
- Adding a member at an ordinal already used elsewhere in the same struct, union, or interface
- Do not report a rename that leaves the ordinal alone; names are not on the wire, so renaming is free
- Do not report declaration order that disagrees with ordinal order, which is legal and common (`rpc.capnp` declares `disembargo @13` above `obsoleteSave @7`)

#### Types and Defaults
- Widening a fixed-width field, such as `UInt32` to `UInt64` or `Float32` to `Float64`: slots are fixed-width at fixed offsets, so this is a break, unlike widening a protobuf varint
- Any other change to an existing field's type, including a signedness flip or swapping an enum for the integer that backs it
- Changing the default value of an existing field; Cap'n Proto encodes values XOR the default, so the same bytes decode differently on either side of the change
- `Text` used to carry arbitrary bytes where `Data` is meant, since `Text` asserts NUL-terminated UTF-8 and readers may validate it
- Do not report a field appended at the next unused ordinal, which is backward compatible

#### Unions, Groups, and Type IDs
- Moving an existing field into or out of a union or group, with one legal exception: wrapping an existing field in a brand-new union where it is the first member
- A union whose lowest ordinal is not a `Void` sentinel, leaving no representable "unset" state
- Adding a member to an existing union without confirming readers handle an unknown discriminant; older code sees a value outside the enum it was compiled against
- Renaming a struct, interface, or file with no explicit `@0x...` id pinned: the id is derived from the name, so the rename silently changes it and breaks anything holding the old one
- Do not report an explicit `@0x...` id carried through a rename; that is the fix, not the defect

#### Interfaces and Methods
- Renumbering an existing method, or reusing the ordinal of one that was removed
- Changing an existing method's parameter or result struct in any way the field rules above forbid
- Removing a method rather than renaming it to `obsolete*` and keeping the ordinal (`sandstorm` keeps `obsoleteHttpGet @1` and `obsoleteGetGrainSize @3`)
- Capabilities returned with no documented lifetime, where dropping the client silently cancels work still in progress
- Do not report a method rename that keeps its ordinal

#### Security and Resource Limits
- `AnyPointer` accepted from untrusted input and cast without a type check
- Unbounded `List`, `Text`, or `Data` from untrusted input with no traversal limit or nesting limit set on the reader
- Secrets, tokens, or credentials embedded in constants, defaults, or comments
- Do not report when reader limits are set at the call site and that boundary is clearly documented
