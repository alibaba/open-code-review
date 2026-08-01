#### Obvious Typos or Spelling Errors
- Spelling errors in type names, function names, variable names, enum variants, trait names, or module names at their declaration sites; do not report spelling errors at call sites
- Strings in log messages, panic messages, error messages, or public diagnostics containing spelling errors that affect readability

#### Ownership and Lifetime Correctness
- Incorrectly returned references, borrowed values escaping their valid scope, or lifetime relationships that make an API unsound or unusable
- Excessive or unnecessary `clone()` calls introduced to satisfy borrowing when a borrow, iterator, `Cow`, or ownership transfer would be clearer and cheaper
- Interior mutability (`RefCell`, `Cell`, `Mutex`) used to work around ownership without a real shared-mutability requirement
- Reference cycles with `Rc<RefCell<T>>` or `Arc<Mutex<T>>` where `Weak` should be used to break ownership cycles

#### Error Handling and Panics
- `unwrap()`, `expect()`, `panic!`, `todo!`, or `unimplemented!` in production/library paths where the failure is recoverable or can be propagated with `Result`
- Errors converted to strings too early or discarded without context; prefer preserving the original error and adding actionable context at boundaries
- `Result` or `Option` values ignored, swallowed, or mapped to misleading defaults
- Public APIs that panic on ordinary invalid input instead of returning a typed error, unless the panic documents a clear programming invariant

#### Unsafe Code Boundaries
- `unsafe` blocks that are broader than necessary or hide multiple unrelated invariants
- Missing or stale safety rationale for `unsafe` blocks, `unsafe fn`, `unsafe impl Send`, or `unsafe impl Sync`
- Raw pointer dereferences without clear validity, alignment, initialization, aliasing, and lifetime guarantees
- FFI boundaries that do not validate null pointers, buffer lengths, ownership transfer, string encoding, or allocator compatibility
- `static mut`, unchecked `transmute`, `MaybeUninit`, `mem::zeroed`, or manual drop logic used without a documented invariant that makes the operation sound

#### Concurrency and Shared State
- Holding `Mutex`, `RwLock`, or `RefCell` guards longer than necessary, especially across calls into user code or potentially blocking operations
- Holding synchronous locks across `.await`, or using blocking I/O, sleeps, or CPU-heavy work directly inside async tasks
- Check-then-act races around shared state, cache initialization, file creation, or atomics
- Atomic operations with ordering that is too weak for the data being protected, or overly strong orderings that hide the intended synchronization contract
- Unsafe `Send` or `Sync` implementations that do not prove all contained state is thread-safe under the documented invariants

#### Async and Cancellation Safety
- Spawned tasks whose `JoinHandle` is dropped when failures, cancellation, or shutdown still need to be observed
- Futures that are not cancellation-safe around partial writes, lock acquisition, transactions, or resource cleanup
- Async functions that use synchronous filesystem, network, or process APIs in request/worker paths where the runtime can be blocked
- Retry loops without backoff, timeout, cancellation propagation, or bounded attempts

#### Collections, Iterators, and Performance
- Avoid unnecessary allocations in hot paths, such as repeated `String` construction, `format!`, `collect()`, or `to_vec()` where borrowing or streaming is sufficient
- Prefer iterator adapters and standard library collection APIs when they make ownership and complexity clearer; avoid dense iterator chains that obscure error handling or side effects
- Ensure hash maps, vectors, and strings are preallocated when the expected size is known and growth cost is material
- Avoid O(n^2) lookups from nested loops when a `HashMap`, `HashSet`, sorting, or indexing strategy would clearly reduce complexity

#### Type and API Design
- Model domain states with enums, newtypes, and typed IDs instead of booleans, strings, or primitive integers when invalid states would otherwise be representable
- Prefer standard conversion and borrowing traits (`From`, `TryFrom`, `AsRef`, `Borrow`, `IntoIterator`) when designing reusable APIs
- Public structs, enums, traits, and errors should have useful names, visibility, trait derives, and documentation appropriate to the crate boundary
- Avoid exposing concrete collection or synchronization types in public APIs when a slice, iterator, trait, or narrower abstraction would preserve flexibility

#### Macros and Metaprogramming
Only flag when the diff actually defines a `macro_rules!` or procedural macro; do not report on ordinary macro invocations.
- An `$x:expr` fragment interpolated more than once in the expansion, so the caller's expression — and any side effects — runs multiple times; bind it to a `let` once inside the expansion
- Exported or publicly used macros that reference items without `$crate::`, so name resolution breaks or binds the wrong item when the macro is invoked from another crate
- Token-tree (`$t:tt`) fragments re-emitted without parentheses, where operator precedence can silently change the intended meaning; this applies only to token-level (`tt`) interpolation, since an `:expr` fragment and a whole expansion are each parsed as one complete expression
- Procedural macros that `unwrap()`, `expect()`, or `panic!` on malformed input instead of emitting a `syn::Error` / `compile_error!` with a useful span
- Macro-hygiene assumptions that break: generated identifiers relying on names from the call-site scope, or items that collide when the macro is invoked more than once in the same module

#### Security-Sensitive Code
- Validate path, URL, command, SQL, and serialized input before use; do not build shell commands or SQL with unchecked string concatenation
- Do not log secrets, tokens, credentials, private keys, or personally identifiable information
- Check integer conversions, byte slicing, and length arithmetic for overflow, truncation, and UTF-8 boundary errors
- Cryptographic, random, authentication, and authorization code must use well-reviewed crates and explicit error handling; flag ad hoc implementations
