> Favor precision over recall: report only defects that are likely to cause incorrect behavior, security vulnerabilities, resource leaks, or material performance problems. Do not report formatting that `dotnet format` can fix, a naming choice an analyzer or `.editorconfig` already governs, or a project's established C# style replaced with a personal preference. This rule covers C# source (`.cs`) files; account for the target framework, the language version, and whether nullable reference types are enabled before raising compatibility or nullability findings.

#### Nullability and Reference Types
- Dereferencing a value the surrounding code can reach as `null`, including a field left unset by a constructor path, an `out` parameter on a failed `Try` call, or a lookup result used without checking
- `!` (null-forgiving) applied to a value whose nullability is not established locally, or used to silence a warning that reflects a real reachable null
- A public API annotated non-nullable while an implementation path can return `null`, or a nullable parameter dereferenced without a guard; do not flag annotations in a project that has not enabled nullable reference types
- `as` followed by use without a null check, where a failed cast is reachable for the input the method accepts

#### Resource Lifetime and Disposal
- An `IDisposable` or `IAsyncDisposable` the reviewed code owns -- streams, `DbConnection`, `SemaphoreSlim`, `CancellationTokenRegistration`, timers -- left without `using`, `await using`, or a `try/finally` that covers exceptions and early returns; a dependency owned elsewhere, such as a scoped `DbContext` the container injected and disposes, is not this code's to release
- A disposable stored in a field or captured by a closure that outlives the scope which disposed it, or disposed while an in-flight asynchronous operation still consumes it
- A type that owns disposable state and implements neither `IDisposable` nor `IAsyncDisposable`, leaving callers no way to release it; either interface alone is sufficient, and a type that only passes ownership on to its caller needs neither
- `HttpClient` constructed per request rather than reused or obtained from `IHttpClientFactory`, where socket exhaustion or stale DNS is a realistic consequence; a single short-lived client in a console entry point is not a finding

#### Async, Tasks, and Cancellation
- `async void` on anything other than an event handler, so the caller cannot await it and a thrown exception reaches no handler
- `.Result`, `.Wait()`, or `GetAwaiter().GetResult()` on a path that can require asynchronous progress, risking deadlock or thread-pool starvation
- A `CancellationToken` accepted but not passed into a cancellable I/O, delay, HTTP, or database call that takes one
- A `Task` started and neither awaited, returned, nor given a continuation that observes its exception; do not flag a deliberately detached operation whose lifetime and error reporting are explicit
- Shared mutable state touched from concurrent continuations, parallel loops, or event callbacks without synchronization, including a non-thread-safe collection used from several tasks

#### LINQ, Collections, and Enumeration
- Enumerating an `IEnumerable` more than once where the source is a deferred query, a database call, or has side effects, so the work repeats or the results differ between passes
- `First()`, `Single()`, `Last()`, indexer, or `Dictionary` key access without establishing the element or key exists for non-constant input
- A deferred query capturing a loop variable, a disposed context, or state mutated before enumeration, so it evaluates against something other than what the code reads as
- A query executed per element of an outer loop where the data volume makes the round trips material, or an in-memory filter applied after the provider already fetched the whole table; do not restructure readable query code purely for micro-optimizations

#### Equality, Structs, and Value Semantics
- `Equals` overridden without `GetHashCode` (or the reverse), or either implemented over mutable state while instances are used as dictionary keys or set members
- A mutable struct whose copying demonstrably loses writes -- mutated through a `readonly` field, an `in` parameter, or a collection indexer that hands back a copy, so the update lands on a temporary and is discarded; copying and independently mutating a value type is ordinary C# and is not on its own a finding
- Reference equality used where value equality is intended, or `==` on types whose operator is not overloaded as the comparison assumes
- `DateTime` used where an offset or time zone matters, or comparisons mixing `Utc`, `Local`, and `Unspecified` kinds

#### Interop, Serialization, and Trust Boundaries
- Untrusted values flowing into SQL, shell commands, file paths, URLs, LDAP queries, deserialization, or rendered HTML without parameterization, validation, or encoding
- Secrets, connection strings, or tokens written into source, logs, or exception messages
- P/Invoke, reflection, or serialization bindings whose declared types, marshalling, nullability, ownership, or enum values do not match the external contract
- A deserializer configured to resolve arbitrary types from untrusted input, or a model bound from a request exposing fields the endpoint does not intend to accept
- A caught exception swallowed or rethrown as `throw ex`, discarding the original stack trace, where the failure needs to propagate or be recorded
