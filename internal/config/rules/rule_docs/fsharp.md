> Favor precision over recall: report only defects that are likely to cause incorrect behavior, security vulnerabilities, resource leaks, or material performance problems. Do not report formatting that Fantomas can fix, or replace a project's established F# style with a personal preference. This rule covers F# implementation (`.fs`), signature (`.fsi`), and script (`.fsx`) files; account for the target framework and the surrounding .NET APIs before raising compatibility findings.

#### Discriminated Unions and Pattern Matching
- `match` expressions that omit a reachable discriminated-union or `option` case, especially after a union gains a new case; do not report a match that the compiler can prove exhaustive
- Catch-all `_` branches used only to suppress an incomplete-pattern warning when an omitted case needs distinct behavior or error handling
- Active patterns or guards whose ordering shadows a later reachable case, silently selecting the wrong branch
- `Option.get`, `ValueOption.get`, accessing `.Value` on an option/value option, or project-specific partial `Result` unwraps where `None`, `ValueNone`, or `Error` can occur for runtime, external, or untrusted input
- Treating a domain failure as an exception while callers are otherwise required to handle it through `Result`; do not flag an intentionally documented exception boundary

#### Resource Lifetime and Mutable State
- `IDisposable`, streams, database connections, locks, or cancellation registrations acquired without `use`, `use!`, `try/finally`, or an equivalent cleanup path that covers exceptions and early returns
- A resource returned from a `use` scope, captured by a closure that outlives the scope, or disposed before an asynchronous workflow that still consumes it completes
- Mutable state shared between agents, tasks, event callbacks, or parallel collection operations without synchronization or a clear single-owner protocol
- Reusing a mutable buffer, array, or record across asynchronous operations where a later mutation can race with a consumer; ordinary local mutation with no escaping reference is not a finding

#### Async, Tasks, and Cancellation
- `Task` or `Async` workflows that drop a reachable exception, cancellation, or result instead of propagating or intentionally handling it at the boundary
- Calling `.Result`, `.Wait()`, or `Async.RunSynchronously` on a context that can require asynchronous progress, producing a deadlock or thread-pool starvation risk
- Accepting a `CancellationToken` but failing to pass it into a cancellable I/O, delay, HTTP, database, or child task operation when the API supports it
- Starting background work without retaining, awaiting, supervising, or observing the task's exception; do not flag a deliberately detached process when its lifetime and error reporting are explicit

#### Sequences, Collections, and Performance
- Enumerating a `seq` with side effects or an expensive source multiple times when the result is expected to be stable or when repeated execution changes behavior; materialize once only when the code needs repeated traversal
- Calling `Seq.head`, `Seq.reduce`, indexed access, or a map lookup without establishing that the collection/key is present for non-constant input
- Accidentally forcing a lazy sequence in a hot path, or composing a sequence pipeline whose deferred exceptions escape a boundary that promises eager validation
- Quadratic list append, repeated immutable-map updates, or repeated string concatenation in a loop where input size can make the behavior materially expensive; do not replace concise collection code merely for micro-optimizations

#### .NET Interop and Type Boundaries
- Passing F# `option` values through a .NET API as though they were `null`, or treating a nullable/reference return from .NET as non-null without a local invariant
- P/Invoke, reflection, serialization, or JSON bindings whose declared types, field names, nullability, ownership, or enum values do not match the external contract
- Runtime downcasts or unchecked/default-producing operations (`:?>`, `unbox`, `Unchecked.defaultof`, or reflection-based invocation) without a locally established runtime type/nullability invariant
- Untrusted values flowing into SQL, shell commands, file paths, URLs, deserialization, or HTML without validation or parameterization, and secrets written to source, logs, or error messages

#### Signatures and Module Boundaries
- An `.fsi` signature whose exposed types, arity, generic constraints, mutability, or visibility do not match the implementation or intended public API
- Public functions that leak mutable implementation state or an internal representation where callers can violate the module's invariants
- Module initialization with observable I/O, non-deterministic global state, or exceptions that make importing the module fail unexpectedly; do not report explicit, documented application bootstrap code
