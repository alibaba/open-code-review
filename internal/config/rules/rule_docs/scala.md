#### Null and Option Safety
- Scala permits `null`, so a `null` can reach a call site that expects an `Option`, or a `Some(null)` can be constructed and then unwrapped into a `NullPointerException`
- `Option.get` / `.head` on a value that is not statically known to be defined throws at runtime; prefer `getOrElse`, `fold`, `map`/`flatMap`, or a pattern match
- `isDefined` followed by `.get` is a check-then-act race when the value can change between the two calls
- Java interop: a platform type from a Java API can be `null` without the compiler noticing

#### Non-Exhaustive Pattern Matching
- A `match` over a `sealed` trait / `sealed abstract class` that omits cases throws `scala.MatchError`; verify every subtype is covered (the compiler warns for sealed types, but not for open hierarchies or reflection-driven values)
- A bare `case _ =>` at the end of a match silently swallows future variants and logic errors
- Extractors in case classes with custom `unapply` can be partial; confirm the extractor cannot throw
- Type parameters erased at runtime: `case x: List[_] =>` accepts any `List`, while a `case x: List[Int]` throws `ClassCastException` when an element is not an `Int`

#### Type Erasure and Unchecked Casts
- `asInstanceOf[T]` and `isInstanceOf[T]` on a generic type are unchecked at runtime; the cast succeeds and the `ClassCastException` surfaces later at an unrelated call site
- `Array[T]` requires an implicit `ClassTag[T]`; an incorrect tag produces a heap-pollution `ArrayStoreException` on write
- `Map` / `Set` keys collected through a widened or erased type (`Set[Any]`, `Map[String, Any]`) are compared with `equals`, not with `==`, so `1` and `1L` are both stored even though `1 == 1L` is true; a later `contains(1L)` then misses

#### Immutability and State
- `var` in a class body or a captured `var` in a closure; prefer `val`, and immutable collections (`List`, `Vector`, `Map`) over their mutable counterparts (`ArrayBuffer`, `mutable.Map`)
- A mutable collection (`ArrayBuffer`, `ListBuffer`, `mutable.Set`) returned or stored as a shared reference lets one caller mutate state another caller observes
- A `val` initializer that reads a `val` declared later in the same body; Scala initializes in declaration order, so the read sees the default value rather than the intended one

#### Implicit Resolution
- Implicit conversions that make a method apply to a type it was never written for; the resolved call site no longer matches the declared signature
- An implicit in scope that silently changes behavior of unrelated code in the same scope (ambiguous or shadowed implicits)
- Implicit parameter lists (e.g. `def f(implicit ec: ExecutionContext)`) resolved from an unexpected instance, changing the execution model of the call

#### Equality and Copying
- `==` on arrays compares references, not contents; use `sameElements` or `toSeq`
- `case class` `copy` applied to a stale instance, reintroducing field values a newer instance had already updated
- Custom `equals` without a matching `hashCode` (or vice versa) breaks `Set` / `Map` membership and `distinct`
- Overriding `equals` on a non-final class without also overriding `canEqual`, so unequal subclasses compare equal

#### Resource and Exception Handling
- Files, sockets, and streams opened without `Using` / `try-finally` are leaked on an exception path
- A `catch` block that logs and continues swallows the original exception, so the caller cannot tell success from failure
- A broad `catch { case e: Exception => }` also catches control-flow and cancellation exceptions that a `NonFatal` guard would let through

#### Functional Pitfalls
- A `.view` left unforced keeps the underlying collection alive and defers work; the query is re-executed on each traversal unless materialized with `.toSeq` / `.force`
- `foldLeft` / `reduce` on an empty collection that throws instead of supplying a neutral element
- Nested `foreach` used for its side effects where `withFilter` + `map` or an explicit `for` comprehension makes the intent clear

#### Performance Issues
- O(n^2) from repeated `List` append inside a loop; build with a `ListBuffer`, prepend, or `foldLeft`
- `filter` / `map` chained over a large collection creating intermediate collections; use a `View` or `Iterator` for lazy pipelines
- Repeated `get` on a `Map` inside a loop over that same `Map`; hoist the lookup or iterate over `.view`
- Implicit boxing on primitive collections (`Int` → `java.lang.Integer`) on hot paths
- A recursive method with no accumulator that is not tail-recursive; Scala does not guarantee tail-call optimization

#### Typo and Naming
- Spelling errors in method, class, or `val` names at their declaration sites (confirm against the naming conventions used by similar identifiers in the repository with `code_search`)
- Types and traits in `UpperCamelCase`; methods and values in `lowerCamelCase`; constants in `SCREAMING_SNAKE_CASE`
- Do not report spelling errors at reference sites; they are defined at the declaration
