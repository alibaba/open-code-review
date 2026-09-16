#### C# Review Principles
> Favor precision over recall: report only defects that are likely real in the changed code and its reachable context. A false positive costs reviewer trust. Treat correctness and security findings as blocking; style-only suggestions are non-blocking. Focus on language-specific risks that ordinary formatting and deterministic tooling do not already cover.

Before reporting a non-local claim, use `file_read` and `code_search` to establish call sites, dependency-injection lifetimes, ownership of disposables, and whether a type is reachable concurrently. Do not infer nullability, thread affinity, attacker control, or an EF Core change-tracking state solely from a name or a `using` directive. Do not duplicate findings the compiler, nullable reference type analysis, Roslyn analyzers, or the IDE report reliably, unless the diff shows a concrete user-visible consequence those tools will not express.

#### Nullability
- A null flowing into a member declared non-nullable, where the nullable annotation context is enabled and the producing path can actually yield null. Check whether the project enables `<Nullable>enable</Nullable>` before treating annotations as contracts.
- The null-forgiving operator applied to a value the surrounding code cannot guarantee, particularly on a deserialization result, a dictionary lookup, a `FirstOrDefault`, or a configuration binding.
- A public API that returns null for a "not found" case while its signature, XML docs, or siblings promise otherwise. Prefer a `TryGet` pattern, a nullable return type, or a thrown typed exception, applied consistently across the type.
- Null checks on a type that overloads the equality operator, where the pattern form and the operator form do not agree; and reference comparisons on records or structs that override equality.

#### Async and Tasks
- `async void` outside an event handler. An exception thrown from it cannot be caught by the caller and will usually crash the process.
- `.Result`, `.Wait()`, `.GetAwaiter().GetResult()`, or a blocking `Task.WaitAll` on a path that can run under a synchronization context or a constrained thread pool: a deadlock and a thread-starvation risk. Flag it in library and request paths; a documented composition root is acceptable.
- A `CancellationToken` accepted and then not passed to the calls that accept one, or a long-running awaiting path with no token at all where the caller can go away.
- A `Task` created and neither awaited nor otherwise observed, so its exceptions are lost. Fire-and-forget is acceptable only with an explicit continuation or logging of faults.
- `ConfigureAwait(false)` missing in library code that can be consumed by an app with a synchronization context; and conversely, code that relies on resuming on a captured context after `ConfigureAwait(false)`.
- Parallel iteration (`Task.WhenAll` over a loop, `Parallel.ForEachAsync`) that shares a dependency which is not thread-safe: an EF Core `DbContext`, a handler with mutable default headers, a non-concurrent collection.

#### Disposal and Resource Lifetime
- An `IDisposable` or `IAsyncDisposable` created and not disposed on every path, including exceptional ones. Verify ownership first: a dependency injected into a type is usually not that type's to dispose.
- A disposable captured in a field of a type that is itself not disposable, or disposed while another live reference can still use it.
- `HttpClient` constructed per call, which exhausts sockets; or, conversely, a long-lived instance whose handler never observes DNS changes. Prefer `IHttpClientFactory` where it is available.
- `using` on a type whose synchronous `Dispose` is a no-op but whose real cleanup is `DisposeAsync`, losing the asynchronous flush.
- A stream, connection, reader, or semaphore released in a `finally` that can itself throw and mask the primary exception.

#### Exceptions
- `catch` blocks that swallow, returning null, `default`, or an empty collection and continuing, so a failure becomes indistinguishable from an empty result.
- A broad `catch` at a non-boundary; and `throw ex;`, which resets the stack trace where `throw;` preserves it.
- Control flow implemented with exceptions on a hot path, and conversely a failure that silently becomes a success sentinel.
- Exceptions crossing an API boundary that leak internal detail (paths, connection strings, SQL) into a response or a log.
- `finally` blocks that return or throw, discarding the in-flight exception.

#### LINQ, Collections, and Equality
- Deferred execution captured and enumerated after the underlying source has changed or been disposed, especially an `IEnumerable<T>` returned from a method whose `DbContext` or `using` scope has already ended.
- Multiple enumeration of a sequence that is expensive or not idempotent (a query, a reader, a generator). Materialize once where the cost is real.
- `First`, `Single`, or `Last` where the empty or multiple case is reachable, and their `OrDefault` variants where the default is then treated as a valid value.
- A mutable reference type used as a dictionary key, or a key type that overrides `Equals` without `GetHashCode`, or overrides neither while being compared by value elsewhere.
- A struct that is large, mutable, or captured in a way that silently copies. Suggest `readonly struct` or `in` parameters only where copies are demonstrably on a hot path.
- Concurrent mutation of a collection that is not thread-safe and is reachable from more than one thread; and `ConcurrentDictionary.GetOrAdd` whose factory has side effects or can run more than once.

#### Entity Framework Core and Data Access
- A `DbContext` resolved as a singleton, captured in one, shared across threads, or used concurrently. It is not thread-safe and is registered scoped by default.
- A query that materializes more than it needs: a missing filter before materialization, no-tracking absent on a read-only path that loads many entities, or a projection evaluated in memory that should have been translated.
- N+1 access patterns, where a navigation property is touched inside a loop over a query result without an eager load or an explicit projection.
- Concatenated or interpolated SQL reaching a raw-SQL API. The interpolated variant parameterizes; the raw variant handed an interpolated string does not.
- `SaveChanges` called inside a loop where one call would do, or omitted entirely on a mutation path.
- A multi-step mutation with no transaction, where partial application is both visible and harmful.

#### ASP.NET Core
- A scoped or transient dependency captured by a singleton, the classic source of a stale `DbContext` or a leaked request scope.
- A controller action, minimal-API endpoint, or hub method missing an authorization attribute its siblings carry, or relying on a client-supplied identifier rather than the authenticated principal to decide access.
- Over-posting: binding directly to an entity, so a client can set fields the endpoint never intended to expose.
- User-controlled input reaching a redirect, a file path, a process start, a deserializer, or an outbound request without validation.
- Secrets, connection strings, tokens, or personal data written to logs or returned in an error response; and structured-logging calls that interpolate the message template instead of passing parameters.
- Middleware ordering changes: authentication after authorization, exception handling registered after the component that throws, or a pipeline component registered twice so the later registration silently replaces the earlier.
- The options interfaces chosen so that a reload is silently never observed, or a snapshot resolved inside a singleton.

#### Time, Randomness, and Determinism
- `DateTime.Now`, `DateTime.UtcNow`, or `DateTimeOffset.Now` read statically in logic that *branches* on the current time: expiry, scheduling, retention, throttling. Injecting `TimeProvider` or an equivalent abstraction is what makes such logic testable. Simple audit stamping is not the same thing and should not be flagged.
- `Now` where `UtcNow` is meant, and `DateTime` used where the offset matters, with a value crossing a process, a wire, or a storage boundary without its kind or offset.
- A random generator constructed per call, producing correlated sequences, or shared across threads without synchronization; and a general-purpose generator used where a cryptographic one is required because the value is a token, a key, or a nonce.
- A newly generated GUID treated as sequential or sortable, and string comparison or parsing that depends on the ambient culture where an invariant one is meant.

#### Concurrency
- A lock taken on a publicly reachable object, such as the instance itself, a type, or an interned string, which lets unrelated code deadlock the type.
- A critical section held by a synchronous primitive across an asynchronous call. `SemaphoreSlim` with its asynchronous wait is the correct primitive.
- Check-then-act on shared state without synchronization, and lazy initialization that can run its factory more than once where that is not safe.
- `volatile`, interlocked operations, or a memory-model assumption used where the surrounding code does not actually establish the ordering claimed. Only report with evidence of concurrent reachability.

#### Testing
- A test that asserts on wall-clock time, real delays, or an external resource, making it non-deterministic. A fake time provider pins an instant without sleeping.
- A test whose assertion cannot fail: asserting on the substitute's own configured return, no assertion at all, or a `try`/`catch` that swallows the failure.
- Shared mutable state between tests, such as a static field or a fixture reused across a parallel collection, that makes execution order significant.
