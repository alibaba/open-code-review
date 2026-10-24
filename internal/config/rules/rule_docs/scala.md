> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Judge only what is observable in the file under review and the tools' evidence; do not assume a specific Scala version (2 or 3), compiler flag, build tool, effect system, actor framework, or target platform (JVM, Scala.js, Scala Native) unless the repository shows it.

#### Obvious Typos or Spelling Errors
- Spelling errors in class, trait, object, method, or value names at their declaration sites; do not report spelling errors at reference sites
- Typos in string literals, log messages, or exception messages that affect readability
- Do not flag identifiers that follow established domain terms elsewhere in the repository

#### Dead Code
- Code blocks that can never be reached (branches whose condition is provably always false, statements after a terminal `return` or `throw`)
- Private methods, values, or unused imports that are declared but never referenced
- Large blocks of commented-out code with no apparent intent to preserve

#### Nullability and Option Safety
- Calling `.get` or `.head` on an `Option` or collection that can demonstrably be empty at that point (`None` from `Option(...)`, `find`, `lastOption`, filtered collections, empty-list construction)
- Unwrapping an `Option` with `.get` when the surrounding code already handles the empty case, or when `getOrElse`/pattern matching expresses the intent more safely
- Values crossing a Java boundary annotated or documented as nullable being dereferenced without a null guard or `Option(...)` wrap
- Do not flag `.get` on an `Option` that the diff provably populated with `Some(...)` immediately before, or in test code asserting the presence of a value

#### Collection Semantics
- Multiple traversal or `hasNext`-style consumption of a single-use `Iterator` (after exhaustion it silently yields nothing)
- Materializing or iterating a collection that may be unbounded or infinite (`Stream.continually`, `LazyList.continually`, `Iterator.continually`) without an explicit `take`/`takeWhile` bound
- Mutating a `var` shared collection or a collection captured by a closure that runs concurrently (Futures, callbacks) without synchronization
- `view`/`LazyList` pipelining whose terminal operation is executed more than once, re-running side effects
- Do not flag lazy pipelines evaluated exactly once, or transformations on `Seq`/`List` that are immutable

#### Futures and Concurrency
- Blocking calls (`Await.result`, `.sync`, IO, locks) on the same `ExecutionContext` that runs the `Future` — a blocked thread starves the pool; only flag when the diff shows the pool is also used for the Future body
- A `Future`'s failure left unobserved: the value is dropped without `recover`, `onComplete`, `andThen`, or returning the Future to the caller
- `Promise` completed more than once where the second completion is silently discarded and the diff shows both paths reachable
- Creating a `Future` (or `Future.sequence`) whose result is discarded while side effects are assumed to have completed ("fire-and-forget" without documentation or synchronization)
- Do not flag code that awaits inside `scala.concurrent.blocking` or a dedicated pool, or test code that must block

#### Pattern Matching and Type Tests
- Non-exhaustive `match` whose missing cases are provably reachable from the scrutinee's type (sealed hierarchy with known subtypes) and lack a `case _` fallback
- Refutable pattern bindings in `val` position (`val Some(x) = ...`, `val x :: xs = ...`) that throw at runtime on non-matching input; prefer `.map`/`getOrElse` or a `match` with a fallback
- Type tests erased at runtime: `case m: Map[String, Int]` cannot check type parameters (unchecked warning); a mismatch is only a real defect when the code relies on the erased parameter for correctness
- Do not flag `case _` fallbacks that deliberately ignore cases, or `@unchecked` annotations with an adjacent invariant comment

#### Initialization Order
- A superclass referring to a subclass member during construction (member is still default-initialized), visible when the diff shows both sides of the hierarchy
- Cyclic `lazy val` initialization (two lazy values whose initializers reference each other) — deadlock or stack overflow at first access
- `val` overrides in a subclass changing a value a superclass method reads during construction
- Do not flag initialization order in traits extended by `object`s with no external construction

#### Equality, Hashing, and Ordering
- Overriding `equals` without `hashCode` (or vice versa) on a class used as a map key or set member
- Comparing values with `==` where reference identity is required (`eq` for reference equality) or vice versa on objects with custom `equals`
- Mutable fields participating in `hashCode` of instances stored in hashed collections
- An `Ordering`/`Ordering.by` inconsistent with `equals` used with sorted collections that assume consistency (`TreeSet`, `TreeMap` lookup semantics)
- Do not flag `==` on case classes (structural equality is correct) or numeric comparisons

#### Numeric Hazards
- Silent narrowing (`Int`→`Short`/`Byte` via `toShort`/`toByte`, `Long`→`Int` via `toInt`) applied to values whose range is not provably bounded
- `Int` division truncation where a fractional result is clearly intended (averages, ratios, percentages)
- Overflow-prone arithmetic on `Int` (`a * b` on unbounded inputs) where the diff provides the values or a `Long` widening exists nearby
- Floating-point `==` comparisons where the values are results of arithmetic (sums of doubles); prefer tolerance comparison — only flag when the compared values are computed, not literal constants
- `0.0`/`NaN` sentinels compared with `==` where `isNaN`/sign checks are required

#### Exceptions and Resource Lifetime
- `catch { case _ => }` (or `case _: Exception =>`) that swallows the error without logging, rethrowing, or returning a signal — only flag when nothing in the handler observes the failure
- Resources (files, streams, connections) acquired without `Using`/`try/finally` guarantees on the same code path
- `Try(...).recover` that replaces a failure with a default indistinguishable from a successful result where callers then proceed on missing data
- Do not flag handlers that log, rethrow, wrap, or return an explicit error value, or `NonFatal` matches that rethrow fatal errors

#### Java and Platform Interop
- Java collections passed to Scala code and treated as Scala collections (or vice versa) without converters (`scala.jdk.CollectionConverters._`), causing surprising behavior (Java maps are not Scala maps: `get` returns null, not `Option`)
- Implicit conversions between Java/Scala types applied silently where behavior differs (`java.lang.Boolean` nullability)
- Do not flag explicit converter usage or code entirely on one side of the boundary

#### Published-API Compatibility
- Removing, renaming, or narrowing the visibility of a public or protected member without a deprecation path (check the module's published nature from build files or usage)
- Changing a case class field list or default parameter of a public constructor — breaks binary compatibility and named-argument callers
- Changing implicit resolution order or shadowing an implicit in a way callers cannot see (low-priority implicit parent no longer consulted)
- Adding a `final` to a member that subclasses in the same repository already override
- Do not flag changes in internal packages, test code, or pre-1.0 modules without published artifacts
