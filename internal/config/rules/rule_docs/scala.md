#### Scala Review Principles
> Favor precision over recall: report only defects likely real in changed code and reachable execution paths. Prioritize crashes, data corruption, security issues, data races, and resource-lifetime bugs. Do not report style preferences.

Before reporting non-local behavior, use `file_read` and `code_search` to verify ownership, callers, synchronization, lifecycle, and input sources. Do not infer threading, execution context, or effect-system semantics from names alone, and do not duplicate scalac, Wartremover, or Scalafix findings unless the diff creates concrete correctness impact. Recommendations that depend on the compiler flags, target platform (JVM, Scala.js, Scala Native), collection library, actor framework, or effect system must be grounded in repository evidence rather than assumed from the `.scala` extension.

#### Null, Option, and Java Interop Boundaries
- `null` values flowing across the Scala/Java boundary, stored in fields of `AnyRef`/`Any` types, or returned from untyped Java APIs without an explicit null check at the boundary.
- `.get`, `.head`, `.last`, `.tail`, or `.reduce` on a possibly-empty `Option` or iterator where the empty case is reachable and not handled.
- `match { case Some(x) => ... }` that lacks an explicit `case None =>` where the absence path affects behavior, data correctness, or logging.
- Implicit conversions from Java types (`String`, boxed primitives, `java.util.*`) into Scala collections on the hot path that allocate per call.
- `@varargs` Java methods invoked with mutable arguments, or Scala varargs passed where the Java side stores the array beyond the call.

#### Collections, Iterators, and Lazy Views
- Iterators consumed more than once, captured across suspension (`Future`, I/O, lazy views), or stored where their single-use semantics would silently yield an empty result.
- Lazy views (`view`, `Stream`/`LazyList`, `Iterator.continually`) that pin a head or resource but never force evaluation, leaking the underlying file/socket/lock.
- `Iterable.size` invoked on a view or on a non-`StrictOptimizedCollection` source, turning O(1) intent into O(n).
- Implicit conversions (`.toList`, `.toSeq`, `.toSet`) inside hot loops where the conversion allocates on every iteration instead of once outside the loop.
- `Array` aliased and mutated through a shared reference across threads without explicit synchronization.

#### Pattern Matching, Exhaustiveness, and Erasure
- `match` expressions whose case set is not provably exhaustive and which lack a final `case _ =>` or `case _ @ _ =>`; verify against the sealed hierarchy, not just the visible call site.
- Pattern bindings preceded by a lowercase letter that can match both the value `null` and a non-null reference; flag only when the null branch affects behavior.
- Type tests inside generic code (`x.isInstanceOf[List[String]]`, `x.asInstanceOf[Foo[X]]`) where the runtime check is against the erased type and the apparent type argument is lost.
- Pattern guards with side effects (`if checkAndMutate(x))` where the guard mutates shared state or performs I/O, since guard evaluation order is not guaranteed when the match later gains alternatives.
- `@unchecked` annotations on non-trivial `match` expressions in changed code where the unchecked cases were not enumerated or justified.

#### Equality, Hashing, and Ordering
- Case classes whose fields include mutable collections, lazy views, arrays, or `AnyVal`/`AnyRef` mixes; the auto-generated `equals`/`hashCode` will not behave consistently with the mutability.
- Mutable collections used as `HashMap`/`HashSet` keys after insertion; later in-place edits change the hash code and break lookup.
- `Ordering` instances built from a partial comparator or one that throws on equality instead of returning 0; flag if the `Ordering` flows into `sortedSet`/`sortedMap` or library sort.
- `equals` overridden on a non-`final` class without a matching `hashCode` override, or vice versa; verify the override actually appears in the diff.

#### Concurrency, Futures, and ExecutionContext
- Blocking I/O, `Thread.sleep`, JDBC calls, file reads, or synchronous network calls inside a `Future` or `cats-effect`/`ZIO` `IO`/`Task` without an explicit shift to a blocking EC.
- `ExecutionContext.global` (or any implicit EC) captured into a long-lived object where the implicit resolution is no longer available, hiding where work will actually run.
- `Future` results consumed after the awaiting scope ends without `Await`/`.recover`/timeout, leaking incomplete work and swallowed failures.
- `Promise` completed more than once, completed after the consumer has timed out, or completed with `Failure` whose cause is dropped without an attached handler.
- `AtomicReference`/`AtomicBoolean` initialized with `null` and then compared with reference-equality semantics that bypass the atomic guarantees.
- Akka/typed actor message handlers that perform blocking work or call `Thread.sleep` inside `Behaviors.receive`; flag only when the same actor handles other messages during the block.

#### Resource Lifetime and I/O
- `Source.fromFile`, `InputStream`, `DataSource`, `java.sql` connections, or `Iterator` from a managed-resource factory that is not closed by `try`/`finally`, `Using`, or a bracket-like construct.
- `Iterator` produced by `Source.getLines` or `BufferedReader.lines` returned from a method whose caller cannot close it.
- Nested `Using` blocks (or nested `try { ... } finally`) where the inner acquisition can fail after the outer resource is already opened; verify the ordering of close calls.
- Native handles (`FileChannel`, `FileLock`, mmap, JNI handles) opened in Scala without a matching release path, especially across `Future` boundaries.

#### Initialization Order, Lazy Values, and Cycles
- `lazy val` cycles between two or more objects/traits whose initialization order is not guaranteed by Scala's class initializer, producing `NullPointerException` or an `InitializationError`.
- `object` initialization that reads mutable global state from `Java`/`Scala` companion modules before they are constructed; flag where the read happens during `extends`/init-time.
- Trait `val` overrides that depend on constructor-initialized state of the parent, where the parent's val is null or default until super-init completes.
- `private[this]` val shadowed by a public getter with side effects in the same body, where the public name and the local name diverge silently.

#### Numeric, Conversion, and Boundary Edge Cases
- Narrowing primitive conversions (`.toInt`, `.toShort`) where the source can exceed the target range, especially from Java APIs that return `long`/`double` for counts or byte sizes.
- Integer division used where fractional or `BigDecimal`/`Rational` precision was intended, particularly in financial or measurement paths; flag only when the diff changes the operation.
- `==` between `Float`/`Double` and a constructed literal (e.g., `0.1 + 0.2 == 0.3`); flag when the literal was newly introduced in the comparison.
- Implicit numeric widening inside loops (`Int` widened to `Double` on every iteration) where pre-computing the bound would change observable precision or performance.
- `BigDecimal` constructed from `Double` (or via `Double.toString`) where precision-sensitive values should originate from `String` or `Int`/`Long`.

#### Exceptions, Errors, and Recoverability
- `try { ... } catch { case _: Exception => ... }` swallowing `InterruptedException`, `OutOfMemoryError`, `StackOverflowError`, or `VirtualMachineError`; flag only when such throwables can be raised by the wrapped code.
- `NonFatal` applied around code that may throw fatal errors with recoverable semantics (cancellation, signal-based shutdown) where `NonFatal` is the wrong classifier for the call site.
- Exception types used for control flow on the hot path (thrown and caught in the same method) where a return value or `Either`/`Try` would make the intent explicit.
- Public API signatures throwing `Exception`/`Throwable` instead of a typed hierarchy that callers can pattern-match.
- `getOrElse(throw ...)` or `.fold(_ => throw ..., _ => ...)` where the original `Failure` cause is dropped before the rethrow; flag only when the cause was constructed in the diff.

#### Platform and Interop Boundaries
- `scala.collection.JavaConverters` (or `_Converters`) used inconsistently with `scala.jdk.javaapi`/`scala.collection.convert`, producing implicit-conversion ambiguity; flag only when the diff mixes both.
- `@throws[Exception]` (Java interop) declared without an actual `throw` site in the body, or missing where the body actually throws a checked exception for Java callers.
- Scala.js- or Scala Native-specific imports (`scala.scalajs.js`, `scala.scalajs.js.annotation.*`, `scala.native.*`) used in code that must remain cross-compiled to JVM; verify the cross-compile target before flagging.
- Macros (`inline`, `macro`, `Macro paradise` imports) that depend on a compiler-version-specific implementation; verify the project's `scalaVersion` before recommending alternatives.
- Reflection (`scala.reflect`, `ClassTag`, `TypeTag`, `Manifest`) used to bridge types at runtime where a generic parameter or constructor type parameter would remove the reflection.

#### Library Compatibility and Published API
- Use of features behind a language-version or library-version gate (e.g., `given`/`using` syntax, `extension` methods, `scala 3` opaque types, `inline if`) without confirming the project's `scalaVersion` matches.
- Public API additions that change binary, source, or semantic compatibility for downstream consumers (removed methods, changed `case class` field ordering, narrowed trait parents); flag only when the diff actually changes the API.
- Implicit conversions exported from a companion object of a library type, where downstream code may pick them up unintentionally; flag only when the conversion is newly added.
- `Serializable` added to a class that transitively holds non-serializable resources (file handles, threads, sockets, JDBC connections) without a matching `writeObject`/`readObject`.

#### Testing Correctness
- `Future`-based tests asserting on internal scheduler state, parallelism, or completion order instead of the public observable behavior.
- Tests that share a mutable global (`object Config`, `var counter`, system property) across `it(...)` cases without an isolated fixture; flag when the test was newly added to a shared suite.
- `assert` / `assume` with side effects or external I/O where a test failure cannot be reproduced.
- Time-dependent tests using `System.currentTimeMillis`, `Instant.now`, or `LocalDate.now` directly instead of an injectable `Clock`; flag only when the new test asserts on a time-bound condition.
- ScalaCheck/ScalaTest generators used without an explicit `Shrink` for the produced type, so failed examples are not reduced and CI logs bloat.