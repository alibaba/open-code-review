> Favor precision over recall: report only issues that are likely to cause incorrect behavior, memory unsafety, security vulnerabilities, or material performance problems. Do not report formatting handled by `nimpretty`, and account for the project's Nim version, memory-management mode, and compile-time defines before raising compatibility findings.

#### Memory and Lifetime Safety
- References, pointers, slices, or `openArray` views that outlive the storage they refer to, especially addresses derived from stack locals, temporary sequences, or strings
- `cast`, `addr`, `unsafeAddr`, manual allocation, or pointer arithmetic without a locally established type, alignment, bounds, ownership, and lifetime invariant
- Mismatched allocation and deallocation APIs, double destruction, or missing cleanup for manually managed resources
- Reference cycles that retain resources indefinitely under ARC or another memory-management mode without cycle collection; do not report this for ORC, which includes a cycle collector
- Do not report ordinary managed references or value copies without evidence of a lifetime or ownership defect

#### Bounds, Values, and Control Flow
- Array, sequence, or string indexing where a reachable index can be negative or exceed `low`/`high`, including incorrect inclusive range boundaries
- Integer conversions, `ord`, enum casts, or arithmetic that can overflow, truncate, or produce an invalid enum value under the project's overflow-check settings
- Variant objects whose discriminant is changed or read inconsistently with the active branch
- `case` statements, object construction, or result paths that leave a reachable value unhandled or unintentionally return the default value
- Assertions used to validate untrusted or runtime input when assertion checks may be disabled in release builds

#### Errors and Resource Cleanup
- `except:` or overly broad exception handling that swallows defects, cancellation, or actionable context and then returns a plausible result
- Resources acquired without `defer`, `try/finally`, or an ownership abstraction when an exception or early return can leak them
- `raiseAssert`, `quit`, or unrecoverable defects used for ordinary invalid input in reusable library or server code
- Error-code or `Option`/`Result` values ignored at boundaries where failure changes correctness or leaves partial state behind

#### Templates, Macros, and Compile-Time Code
- Templates that evaluate an argument more than once when the argument may have side effects; bind the expression to a local `let` so it is evaluated once per template invocation
- Macros that construct identifiers or AST nodes without preserving hygiene, source information, or the expected symbol binding
- `static`, `compileTime`, or macro execution that reads mutable external state and makes builds non-reproducible without an explicit project requirement
- Untrusted text incorporated into generated Nim, shell commands, or compiler invocations without strict validation
- Do not report ordinary template or macro use when the generated behavior is clear and arguments are evaluated safely

#### Concurrency, Async, and Effects
- Shared mutable state accessed by threads without a lock, channel, atomic operation, or established single-owner design
- Locks held across blocking operations, callbacks, or `await`, creating deadlock or starvation risks
- Futures started without awaiting, returning, or otherwise observing failures when completion matters to correctness
- Blocking file, process, sleep, or network operations introduced into an async request path
- Thread procedures or callbacks that capture data whose lifetime ends before the thread or foreign caller finishes

#### FFI and Security Boundaries
- `importc`, `exportc`, `dynlib`, or callback declarations with incompatible calling conventions, types, struct layout, nullability, or ownership rules
- C strings or buffers consumed without validating null termination, length, encoding, and lifetime
- User-controlled data passed to `execShellCmd`, a shell invocation, SQL construction, path access, deserialization, or code evaluation without appropriate validation or parameterization
- Secrets, credentials, tokens, or private data embedded in source, command arguments, logs, exceptions, or generated artifacts
- Cryptographic keys or security tokens generated with non-cryptographic randomness or ad hoc cryptographic code
