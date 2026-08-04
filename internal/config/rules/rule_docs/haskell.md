> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Confirm invariants and the strictness or totality guarantees of helpers defined elsewhere before reporting their call sites.

#### Obvious Typos or Spelling Errors
- Spelling errors in module, type, data constructor, typeclass, function, field, or pattern-synonym names at their declaration sites; do not report spelling errors at use sites
- Typos in user-facing error messages, log output, Haddock comments, or public diagnostics that affect readability

#### Totality, Patterns, and Partial Functions
- Non-exhaustive pattern matches in function equations, `case`, lambdas, or `do` bindings when a reachable constructor or empty input would fail at runtime
- Partial list and container operations such as `head`, `tail`, `init`, `last`, `!!`, `foldl1`, `foldr1`, `minimum`, or `maximum` without a locally proven non-empty or in-bounds invariant; prefer pattern matching, safe lookup, or `NonEmpty`
- `fromJust`, `fromRight`, `read`, `toEnum`, `succ`, `pred`, `error`, or `undefined` on ordinary input or recoverable paths; prefer total alternatives such as `maybe`, `readMaybe`, `readEither`, or an explicit error type
- Record selectors used on a sum type when the field is absent from some reachable constructors
- Do not report a partial operation when the same function has already validated the invariant, the type encodes it, or failure deliberately marks an impossible internal state with a clear explanation

#### Laziness, Strictness, and Space Usage
- `foldl` over a large finite collection with a strict accumulator operation, building a chain of thunks; use `foldl'` or a strict accumulator when evaluation order permits
- Retaining the head of a lazy list, `ByteString`, `Text`, or streaming structure while consuming its tail, accidentally keeping the entire input alive
- Calling `length`, `last`, strict conversion, or full sorting on a potentially infinite or intentionally streaming value
- Repeated lazy-to-strict conversion or forcing an entire request/file merely to inspect a prefix, defeating streaming and creating avoidable memory spikes
- Adding `seq`, bang patterns, or deep evaluation speculatively without evidence of a leak or latency problem; strictness changes can alter termination and exception timing

#### Errors, Exceptions, and Resource Safety
- `IO` resources opened without `bracket`, `withFile`, `withBinaryFile`, `finally`, or an equivalent managed abstraction, leaking handles when an exception or asynchronous cancellation arrives
- `catch` or `try` at `SomeException` that unintentionally swallows asynchronous exceptions such as cancellation; catch the expected exception type or rethrow async exceptions
- `throwIO`, `error`, or pattern-match failure used for an expected domain error that callers need to distinguish; return `Either`, `ExceptT`, `Maybe`, or a typed exception as appropriate
- Cleanup implemented as a normal action after the main operation rather than with an exception-safe combinator
- Exception handlers that discard the original cause or silently substitute a plausible value, hiding corruption, partial writes, or failed validation

#### Concurrency, Async Exceptions, and STM
- Threads started with `forkIO` when their exceptions, lifetime, or shutdown must be observed; prefer structured `async`/`withAsync` and ensure results are awaited or linked
- A `takeMVar` followed by work and `putMVar` that can be interrupted, leaving the `MVar` empty; use `modifyMVar`, masking, or another exception-safe combinator
- Blocking operations, unbounded retries, or long pure computations performed inside `atomically`, causing transactions to retry excessively or preventing useful progress
- `unsafeIOToSTM` used for externally visible, non-idempotent, or exception-prone effects that may run multiple times as a transaction retries
- Shared mutable state updated from multiple threads without an `MVar`, `TVar`, atomic primitive, or a design that establishes single ownership
- Lock ordering that can deadlock, or holding an `MVar`/lock while calling unknown user code or waiting on another thread

#### Types, Instances, and API Design
- Orphan typeclass instances that can conflict with instances from another package and make behavior depend on import/build composition; prefer a `newtype` owned by the defining module
- `fromIntegral`, `toEnum`, `fromEnum`, or narrowing conversions that can overflow, wrap, truncate, or accept an invalid range without a checked boundary
- Typeclass instances that violate class laws relied upon by callers, such as inconsistent `Eq`/`Ord`, unlawful `Semigroup`/`Monoid`, or `Hashable` inconsistent with `Eq`
- Public APIs that expose invalid states as unrelated booleans, tuples, or `String` values when an algebraic data type or newtype can encode the invariant
- Constraints substantially broader than the implementation needs, reducing reuse or hiding an unintended effect requirement

#### Collections and Performance
- Repeated left-associated list append (`xs ++ [x]`) or `++` in a loop, which makes construction quadratic; prepend and reverse, use a builder, or accumulate with an appropriate sequence
- Repeated `length`, indexing with `!!`, or linear membership checks inside a traversal when one pass, a fold, or a `Set`/`Map` would avoid quadratic work
- Converting repeatedly among `String`, strict/lazy `Text`, and strict/lazy `ByteString` in a hot or high-volume path
- Using `String` for large text or byte-processing workloads where `Text` or `ByteString` is already the surrounding API and materially avoids per-character allocation
- Forcing parallel work without bounding it, potentially retaining the entire input or creating more sparks/tasks than useful work

#### Unsafe Features, FFI, and Metaprogramming
- `unsafePerformIO` whose result depends on mutable state, invocation order, or arguments not reflected in the value, or that lacks `NOINLINE`/a documented referential-transparency invariant where required
- `unsafeCoerce`, raw address operations, or `GHC.Exts` primitives without a narrowly documented representation and lifetime invariant
- FFI calls that mishandle pointer lifetime, nullability, buffer length, alignment, ownership transfer, finalizers, callbacks, or C string encoding
- Template Haskell, quasiquotation, or generated source that incorporates untrusted data into executable code
- `unsafeInterleaveIO` or lazy I/O when effect ordering, handle lifetime, or exception behavior is relied upon for correctness

#### Security-Sensitive Code
- External input passed to `System.Process.shell`, an explicit shell command, SQL construction, or template/code evaluation without strict validation; prefer `proc` with an argument list and parameterized APIs
- Untrusted paths used without constraining traversal, symlink behavior, and the intended root directory
- Secrets, tokens, private keys, credentials, or personally identifiable information written to logs, exceptions, source, or serialized diagnostics
- Authentication, authorization, cryptographic, or random-token code that uses ad hoc algorithms, non-constant-time secret comparisons where relevant, or a non-cryptographic RNG
