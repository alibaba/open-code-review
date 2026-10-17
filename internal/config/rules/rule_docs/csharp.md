> Favor precision over recall: report only defects that are likely to cause incorrect behavior, security vulnerabilities, resource leaks, or material performance problems. Do not report formatting or style conventions that can be enforced by Roslyn analyzers, dotnet format, or editorconfig. This rule covers C# source files (`.cs`) across modern .NET (.NET Core / .NET 5+) and ASP.NET Core applications; account for the target framework and surrounding APIs before raising findings.

#### Nullability, Type Safety, and Invariants
- Dereferencing values that can legitimately be null at runtime without validation, particularly when reading unvalidated external input, JSON/model deserialization results, dictionary lookups, or optional configuration
- Using the null-forgiving operator (`!`) when surrounding control or data flow establishes that `null` is legitimately reachable at runtime (such as unvalidated external payloads, deserialization boundaries, or missing map keys), masking reachable null dereferences; do not report `!` when locally established domain invariants guarantee safety
- Direct casts `(T)x` where the runtime type is not guaranteed and can throw `InvalidCastException`, or `as` casts whose possibly-null result is dereferenced without checking whether the conversion succeeded; prefer type guards/pattern matching when the runtime type is uncertain
- Value type or struct copy semantics bugs where mutating a member of a copied struct or property return silently discards state mutations

#### Async, Tasks, and Cancellation
- Blocking on asynchronous work via `.Result`, `.GetAwaiter().GetResult()`, or `.Wait()` in contexts where thread-pool starvation, UI thread hangs, synchronization-context deadlocks, or blocking inside an otherwise asynchronous request path can realistically occur; do not report safe intentional blocking such as CLI/console entrypoints, test harness boundaries, or tasks already proven complete by surrounding code
- Using `async void` outside legitimate event-handler or framework-mandated callbacks, making completion impossible for callers to await and causing exceptions to escape normal `Task`-based error handling
- Instantiating background work or `Task.Run` without awaiting, observing exceptions, or providing a mechanism to handle failures
- Accepting a `CancellationToken` in a method signature but omitting it from downstream cancellable operations such as HTTP calls, database queries, delays, or stream I/O where cancellation is intended to propagate
- Mixing synchronous blocking I/O (e.g. `File.ReadAllText`, `Thread.Sleep`, synchronous socket operations) into high-throughput asynchronous request paths where async equivalents should be used

#### Resource Lifetime and Disposal
- Locally created `IDisposable` or `IAsyncDisposable` resources whose ownership remains with the current code but which lack a cleanup path covering success, failure, and early return; do not report resources whose lifetime is managed by DI/framework infrastructure or whose ownership is explicitly transferred elsewhere
- Returning an `IDisposable` instance from within a `using` block that disposes it prior to returning to the caller
- Disposing a resource while asynchronous or background operations are still actively reading from or writing to it
- Repeated short-lived or per-request construction of `HttpClient` without connection reuse where the churn realistically causes socket exhaustion, resource leaks, or connection starvation; account for legitimate patterns including singleton `HttpClient`, `IHttpClientFactory`, explicit `SocketsHttpHandler` with `PooledConnectionLifetime`, or long-lived managed instances

#### ASP.NET Core and Dependency Injection
- Captive dependencies: registering a service as `Singleton` that directly captures or injects a `Scoped` dependency (such as `DbContext` or request-bound services)
- Accessing request-scoped services or `HttpContext` inside singleton background services (`IHostedService`, `BackgroundService`) without creating and managing an explicit `IServiceScope`
- Modifying response headers or the status code after `HttpResponse.HasStarted`, when those values can no longer be changed; also flag middleware that invokes later components after it has already produced a response when doing so creates conflicting response behavior
- Sensitive endpoints or actions missing authorization where surrounding context demonstrates that access control is required and no effective authorization mechanism is applied; account for controller-level `[Authorize]`, route group `RequireAuthorization()`, global/fallback authorization policies, middleware, or authorization filters before raising a finding
- Initiating background work tied to the request lifetime or passing `HttpContext.RequestAborted` to asynchronous operations that must outlive the incoming HTTP request

#### Entity Framework Core and Data Access
- Concurrently invoking database operations on a single `DbContext` instance across multiple threads or concurrent tasks; `DbContext` is strictly non-thread-safe
- Repeated database queries inside loops that create an N+1 access pattern with material query amplification at the expected data scale; confirm that the calls actually execute database I/O before reporting
- Building SQL queries using raw string interpolation or string concatenation with untrusted input in `FromSqlRaw` or `ExecuteSqlRaw` instead of using parameterized queries or `FromSqlInterpolated`
- Executing synchronous database calls (such as `.ToList()` or `.SaveChanges()`) on high-concurrency or asynchronous request-processing paths where blocking threads introduces material latency or thread-pool starvation risk; do not report synchronous database APIs in console applications, batch workloads, migrations, startup routines, or explicitly synchronous workflows without evidence of an issue

#### Concurrency and Shared State
- Check-then-act race conditions (e.g. `ContainsKey` followed by indexer access or insertion) and concurrent writes on non-thread-safe collections (e.g. `Dictionary<K,V>`, `List<T>`); use `ConcurrentDictionary` or proper synchronization
- Unsynchronized mutation of `static` mutable fields, properties, or shared caches across request threads
- Synchronization hazards involving locks accessible to outside callers, such as locking on `this`, `typeof(T)`, public objects, or interned string literals, or omitting `try/finally` around manual `SemaphoreSlim.Wait()` and `Release()`; private dedicated reference-type lock objects are normal

#### Security and Input Validation
- Concatenating untrusted input into file system paths without canonicalization and boundary verification against an allowed base directory (`Path.GetFullPath`)
- Passing unsanitized input to process execution APIs (`Process.Start`, `ProcessStartInfo`) without proper argument escaping
- Use of `BinaryFormatter` or equivalent unsafe polymorphic deserialization on untrusted payloads; account for the target framework, since the in-box `BinaryFormatter` implementation throws on .NET 9+ while compatibility packages and older runtimes retain its inherent deserialization risk
- Hardcoded secrets, API tokens, connection strings, private keys, or passwords embedded in source code

#### Collections, LINQ, and Error Handling
- Multiple enumeration of `IEnumerable` sequences when the underlying source is expensive, remote/database-backed, stateful, mutating, non-repeatable, or side-effecting, producing materially different behavior or performance; do not report trivial repeated enumeration over small in-memory collections
- Mutating a collection during iteration with `foreach`, resulting in `InvalidOperationException`
- Unchecked `First()`, `Single()`, indexing, or dictionary access when the surrounding contract does not establish that the required element/key exists; require an appropriate precondition, guarded lookup, or explicit missing-value path rather than merely replacing the operation with a default-returning variant
- Swallowing exceptions silently in empty catch blocks without logging, remediation, or re-throwing
- Using `throw ex;` inside a catch block instead of `throw;`, erasing the original exception stack trace
