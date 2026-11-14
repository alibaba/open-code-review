#### Scala Review Principles

> Favor precision over recall: report only defects that are likely real in the changed code and its reachable context. Treat correctness, concurrency, resource-safety, and security findings as blocking; do not report style or idiom preferences.

Before making a version- or runtime-dependent claim, inspect the build configuration for `scalaVersion`, `crossScalaVersions`, compiler options, and the target platform. Verify whether the code uses the Scala standard library directly or an ecosystem abstraction such as an effect runtime, actor dispatcher, distributed collection, or framework-owned lifecycle. Do not replace the project's established abstraction with `Future`, `Option`, or a particular collection merely as a preference.

Use repository context to establish callers, value ranges, nullability, collection size, execution context, resource ownership, and initialization order. Do not duplicate compiler, linter, or static-analyzer diagnostics unless warnings are suppressed, are not fatal in the project, or the change demonstrates a concrete runtime consequence.

#### Null, Option, and Absent Values

- A value that can actually be `null`, especially from Java interop, uninitialized fields, deserialization, or legacy APIs, reaches member access, unboxing, pattern extraction, or another operation that assumes a non-null value
- `Some(nullableExpression)` preserves `null` as a present value and later code assumes the contained value is non-null; use `Option(nullableExpression)` only when `null` is intended to mean absence
- `Option.get` is called when `None` is reachable; do not report it when the same local path proves non-emptiness or the type or domain establishes the invariant
- `getOrElse(null)` or `orNull` moves absence back into Scala code and a reachable consumer assumes non-nullness; do not flag `orNull` at an interop boundary that explicitly requires `null`
- In Scala 3 code compiled with explicit nulls, `.nn` or `scala.language.unsafeNulls` bypasses a nullable type without a proven runtime check; do not assume explicit nulls is enabled merely because the project uses Scala 3
- Do not request replacing every nullable value with `Option`; confirm that the API can change and that `null` is not required by an interoperability contract

#### Collections, Iterators, and Laziness

- A partial collection operation such as `head`, `tail`, `last`, `init`, `reduce`, `min`, `max`, direct indexing, `Iterator.next`, or `Map.apply` on a map whose default is known to throw is used when an empty, missing, or out-of-bounds case is reachable and not guarded by a local invariant; account for `withDefault`, `withDefaultValue`, and an overridden `default` before reporting a missing key
- An `Iterator` is traversed by one operation and then reused as though it were a reusable collection, or the original iterator and a derived iterator are consumed independently even though they share traversal state
- The result of an immutable collection update or transformation is discarded where the code intends to mutate the original collection
- A `View`, `Iterator`, `LazyList`, `Stream`, `mapValues`, or `filterKeys` transformation contains side effects whose required timing or execution count is changed by lazy evaluation
- Code assumes a lazy view is a stable snapshot even though the underlying mutable collection can change, or assumes transformed values are computed only once; require materialization only when snapshot semantics are established
- A potentially infinite `LazyList` or `Stream` is fully forced by `size`, `sum`, `max`, sorting, strict conversion, or another whole-collection operation, causing non-termination or unbounded work
- A long-lived reference to the head of a `LazyList` or `Stream` retains an ever-growing memoized prefix in a path that continually forces new elements, causing demonstrable memory growth
- Repeated `List` indexing, `length`, or append inside a traversal creates quadratic work on a collection known to be meaningfully large; do not report complexity issues without evidence of scale or a hot path

#### Future, ExecutionContext, and Blocking

- Multiple callbacks mutate shared state while relying on callback registration order, or rely on sequential execution on an `ExecutionContext` and target that can run them concurrently; the `Future` API does not guarantee registration order, while actual concurrency depends on the execution context and platform
- `Await`, thread sleep, synchronous I/O, lock acquisition, or another blocking call runs on a bounded or latency-sensitive execution context where it can starve required work or deadlock progress
- Code assumes `blocking` will compensate for blocked threads on an execution context such as one backed by a fixed executor, even though compensation is implementation-dependent
- A synchronous execution context runs callbacks inline where correctness requires an asynchronous boundary, or recursive submission can exhaust the stack
- `map` produces a nested `Future` where callers require the inner completion or failure to be part of the returned operation; use `flatMap` or equivalent composition only when that contract is established
- A `Future` whose completion or failure matters is started and discarded, so failures are unobserved or shutdown proceeds before required work completes; do not flag intentional best-effort telemetry or explicitly supervised fire-and-forget work
- Code assumes a computation created with `Future.apply` is lazy or reruns when the returned `Future` is reused; `Future.apply` schedules its body at the call site and the returned future represents one completion. Do not infer eagerness for an arbitrary `Future` backed by a `Promise` or another abstraction
- Multiple legitimate contenders call `Promise.complete`, `success`, or `failure`, making every losing completion throw; use the corresponding `tryComplete`, `trySuccess`, or `tryFailure` operation, or establish single-writer ownership
- Do not flag bounded `Await` merely because it appears in a test or at a deliberate application shutdown boundary, and do not apply standard-library `ExecutionContext` advice to a framework dispatcher or effect runtime without confirming its contract

#### Pattern Matching, Exhaustivity, and Erasure

- A match has a reachable uncovered value, or a refutable `val` or assignment pattern can fail at runtime; identify the actual version-dependent failure, such as `MatchError` or `ClassCastException` for some Scala 2 typed bindings, and do not treat Scala 2 for-generator filtering as a thrown match failure
- A lowercase identifier intended to denote an existing stable value is written as a variable pattern, so it matches every value and makes later cases unreachable or behavior incorrect; confirm the intended constant from surrounding declarations before reporting it
- A generic type pattern such as `List[String]` is treated as if its erased runtime type checked the element type, allowing differently typed values through and causing an incorrect branch or later cast failure
- `runtimeChecked` or a scrutinee annotated with `@unchecked` hides a reachable uncovered case, or a type argument annotated with `@unchecked` hides an erased type test; `runtimeChecked` itself does not suppress unchecked type-test warnings. Do not report a suppression when the invariant is documented and established by nearby validation
- A catch-all case silently handles a known algebraic-data-type variant incorrectly after the variant set changes; do not demand removal of catch-all cases used intentionally for forward compatibility
- A non-local `return` inside an escaping or asynchronous closure is intended to return from an enclosing method after that method may have completed, or its control throwable is intercepted by broad exception handling; account for its deprecation in Scala 3.2 and later

#### Object Initialization and Lazy Values

- A superclass constructor calls an overridable method or reads an overridden `val` whose subclass field has not been initialized, yielding `null`, zero, false, or otherwise incomplete state
- A strict field initializer reads a later field in the same initialization sequence before that field is assigned
- `this` escapes from a constructor through a callback, registry, global object, or concurrent task and reachable code can observe partially initialized state
- A visible cycle among lazy values or nested objects can recurse indefinitely, overflow the stack, or deadlock during initialization; do not suggest `lazy val` as a universal initialization fix without checking for cycles
- A lazy initializer performs non-idempotent effects before it can throw, allowing a later access to retry initialization and repeat partial effects
- For Scala 3, account for whether `-Wsafe-init` is enabled; for Scala 2, `-Xcheckinit` adds runtime checks rather than making initialization safe. Do not restate an already-fatal diagnostic without additional impact

#### Equality, Hashing, Ordering, and Key Stability

- `equals` is overridden without a compatible `hashCode`, so equal objects can occupy different hash buckets
- An equality implementation violates reflexivity, symmetry, or transitivity, especially across an open inheritance hierarchy; require `canEqual` only where subclass equality can break those laws
- A field participating in equality or hashing is mutated after the object is inserted into a hash-based map or set, making later lookup or removal fail
- Array `==` or `equals` is used where element-wise equality is clearly intended; use `sameElements` or an appropriate array comparison for content semantics
- Values from unrelated domain types are compared with universal `==` and the comparison silently becomes always false after a refactor; Scala 3 strict equality can prevent this only when the project enables the relevant `CanEqual` policy
- An `Ordering` returns zero for values that must remain distinct, or disagrees with equality where callers rely on consistent sorted-map or sorted-set lookup and deduplication
- Do not request handwritten equality for case classes unless a field has special equality semantics, such as array content, or inheritance exposes a concrete law violation

#### Numeric Correctness

- A narrowing conversion such as `Long.toInt`, `Int.toByte`, or `Double.toFloat` can receive a reachable out-of-range or precision-sensitive value and silently truncates or loses precision
- Arithmetic, collection `sum`, counters, sizes, timestamps, or unit conversions can exceed the selected `Int` or `Long` range under established input bounds
- Integer division is used where the surrounding contract requires a fractional result, or promotion happens only after the division has already truncated the value
- Code tests a floating-point value against `NaN` with `==` or `!=` instead of using `isNaN`, making the intended test always false or always true
- Exact floating-point comparison controls tolerance-sensitive numeric behavior and reachable rounding makes equality unreliable; do not flag exact sentinels, integral values within the exact range, or bit-level protocols
- Decimal or financial input is first converted through `Float` or `Double`, or a `BigDecimal` operation uses a rounding context inconsistent with the stated precision contract
- A range uses inclusive `to` where an exclusive upper bound is required, or `until` where the endpoint must be included, making the boundary element incorrectly processed or skipped

#### Exceptions and Resource Safety

- A broad catch converts a meaningful failure into success, discards the cause, or catches interruption, fatal errors, or control throwables that should propagate; use `NonFatal` only when the contract is genuinely to handle all ordinary failures
- Code assumes `Try` catches every `Throwable` or that every throwable becomes a failed `Future`; `Try` catches only `NonFatal`, and a fatal throwable from a future can escape to the execution context or runtime and leave the future incomplete instead of producing an ordinary failed completion
- `Try.get`, `Failure.get`, or equivalent extraction is used where failure is reachable and callers expect recoverable propagation
- Files, streams, sockets, database handles, or other owned resources are not released on every early-return and exception path
- A lazy `Iterator`, `View`, `LazyList`, or `Stream` that still depends on a resource escapes and is traversed after the resource closes, or callback or asynchronous work can access the resource after its ownership scope closes
- Cleanup loses an operation failure, or owned resources are not closed in the required reverse-acquisition order; use an ownership abstraction with documented suppression and severity semantics, and verify which exception remains primary
- Do not flag resources transferred to a caller, framework, or manager that owns closure. `scala.util.Using` is available in Scala 2.13 and Scala 3; do not prescribe it to a Scala 2.12 source set without a compatible dependency or version-specific source

#### Interoperability and Unsafe Boundaries

- `asInstanceOf`, an erased generic cast, or `@uncheckedVariance` permits a reachable value that violates the assumed runtime type or a mutable write that violates the real element type
- An `asScala` or `asJava` adapter over a mutable Java or Scala collection is treated as an independent copy even though operations forward to the underlying collection; also account for immutable-to-Java wrappers whose mutation methods throw `UnsupportedOperationException`
- A Java API result with unspecified or nullable output is treated as non-null solely because its Scala type appears non-null; account for annotations and the project's explicit-nulls configuration
- Scala.js code invokes or casts untyped external data from `js.Dynamic` or undefined-bearing values without a check required by reachable runtime inputs, or declares a facade whose shape disagrees with the actual JavaScript API; do not demand per-call validation for ordinary use of a correctly declared facade
- Scala Native or JNI code lets a pointer, callback context, zone allocation, borrowed buffer, or native handle outlive its owner, or passes an incompatible size, layout, ownership, or callback signature
- Do not apply JVM, JavaScript, or native-runtime findings until the build establishes that the reviewed source set targets that platform

#### Published API and Compatibility

- In a library that declares binary compatibility, a public member is removed, renamed, hidden, or changed in JVM signature without the required compatibility plan or version boundary
- A public case-class parameter list changes generated `apply`, `unapply`, `copy`, and product shape in a way that breaks established downstream source, binary, serialization, or pattern-match contracts
- A new abstract trait member makes existing downstream implementations incomplete, or a change to a sealed hierarchy breaks consumers that the repository promises to keep source-compatible
- Do not report compatibility concerns for application-internal APIs or an explicitly breaking release; verify the project's compatibility policy, supported Scala lines, and tooling such as MiMa before reporting

#### Security and Dynamic Execution

- External data is interpolated into SQL, a shell command, a template, or executable code without parameterization or strict validation; verify whether a custom interpolator already provides the required escaping or binding
- Untrusted text reaches reflection, a compiler toolbox, a script engine, dynamic class loading, or deserialization that can instantiate or execute unintended code
- An external path used by an operation that promises confinement can escape the intended root through traversal, an absolute path, or symlink resolution; verify the operation's contract and symlink policy before reporting
- Secrets, tokens, private keys, credentials, or sensitive data are embedded in source or written to logs, exceptions, command arguments, or serialized diagnostics
- Security tokens, nonces, or cryptographic keys are generated with `scala.util.Random` or another predictable generator instead of a cryptographically secure source

#### Scala Test Correctness

- An asynchronous test starts assertions in a `Future` or callback but neither returns, awaits, nor registers that work through the test framework, allowing the test to pass before the assertions run
- A test result depends on callback order or on a fixed sleep being long enough; treat use of a global `ExecutionContext` as evidence only when scheduling or resource contention is shown to affect determinism
- `==`, `.equals`, or a framework assertion with reference-equality semantics is used for arrays where element-wise content semantics are intended; do not flag matchers that document deep array comparison
- Parallel tests mutate shared singleton, locale, clock, filesystem, or port state, or reconfigure or shut down a shared `ExecutionContext`, without isolation and cleanup that establish determinism
