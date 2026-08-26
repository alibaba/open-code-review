> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the code under review; do not infer behavior of ports, flags, or modules defined outside this file.

#### Obvious Typos or Spelling Errors
- Spelling errors in module names, type names, function names, or field names at their declaration sites; do not report spelling errors at call sites
- Typos in `Debug.log` labels, error messages surfaced to users, or docstrings that affect readability

#### The Elm Architecture (Model / Update / View)
- `update` branches that return the incoming `Model` unchanged (a no-op case) when the message implies a state change, silently dropping user actions
- `Cmd` values produced by `update` but not returned (e.g. constructed and discarded), so a command that should fire never runs
- `Msg` constructors added to the type but never matched in `update`, or matched with a catch-all `_ ->` that masks a forgotten case
- View functions that read fields directly instead of routing through `update`, bypassing the single source of truth and desyncing displayed state from `Model`
- Subscriptions declared but not wired into `Sub.batch`, or a `Sub.none` left in place after conditional subscriptions were intended

#### Maybe and Result Instead of Exceptions
- `Maybe.withDefault` or pattern matches that silently substitute a default for `Nothing` in a path where the absence should surface as user-visible feedback or an error state
- Chains of `case ... of` on `Maybe`/`Result` that do not handle every constructor, relying on the compiler's exhaustiveness only because a wildcard `_ ->` was added, hiding a missed case
- Using `Debug.todo` or a partial function (e.g. indexing with an assumption that a `List` is non-empty) as a stand-in for proper `Maybe`/`Result` handling
- `Result.map`/`Result.andThen` chains that lose the original `Err` value or replace it with a generic message, discarding information needed to diagnose the failure

#### Ports and JavaScript Interop
- Port modules that send or receive JSON without a matching decoder/encoder on both the Elm and JavaScript sides, risking a runtime decode failure with no Elm-side type safety
- Outgoing ports (`port toJs : Value -> Cmd msg`) called with data whose shape does not match what the JavaScript listener expects, since the compiler cannot check the JS side
- Incoming ports (`port fromJs : (Value -> msg) -> Sub msg`) whose payload is passed directly to application logic without running a `Json.Decode` decoder first, trusting unvalidated external data
- Ports left subscribed after the component that needed them has been removed, leaking a listener that never resolves

#### Decoder and Encoder Correctness
- `Json.Decode` decoders using `Json.Decode.field` chains that do not match the actual JSON shape from the API, especially optional fields decoded without `Json.Decode.maybe` or a default
- Decoders combined with `Json.Decode.andThen` that construct a value which does not satisfy the type's invariants (e.g. an empty string accepted where a non-empty identifier is required)
- Encoders (`Json.Encode`) that omit fields the consuming API requires, or that encode a `Maybe` as `null` when the API expects the field to be absent entirely (or vice versa)
- Decoder/encoder pairs that have drifted out of sync with each other or with the `Model` type they represent, so a round trip silently loses or corrupts data

#### Debug and Production Hygiene
- `Debug.log` or `Debug.toString` left in code paths that ship to production; `elm make --optimize` fails to compile with `Debug.log`/`Debug.todo` present, so leftover calls block optimized builds
- `Debug.todo` used as a placeholder for unimplemented branches that are reachable in normal application flow rather than genuinely unreachable states
- Comparing custom types or records with `==` where the comparison actually needs `Debug.toString` or a dedicated field-by-field comparison, given Elm's structural equality semantics

#### Package Versioning and Dependencies
- Changes to exposed module APIs (function signatures, exposed types, exposed constructors) without a matching version bump in `elm.json` under Elm's enforced semantic versioning
- Dependency version ranges in `elm.json` widened or narrowed without verifying the actual compatibility, since `elm.json` constraints are enforced strictly by the compiler and package tooling
- New dependencies added to `elm.json` that are unused in the module, or used dependencies missing from `elm.json`

#### Performance and Correctness Anti-Patterns
- Recursive `view` or `update` helpers that rebuild large `List`s or record structures on every call where a `Dict` or memoized value would avoid repeated work
- Large `List.length`, `List.reverse`, or repeated `List.append` (`++`) usage in hot paths where the O(n) or O(n²) cost is avoidable with a different data structure or accumulator pattern
- Untrusted input passed to `String.toInt`/`String.toFloat` without handling the `Maybe` result, or used to build a `Json.Decode` failure message shown verbatim to the user
