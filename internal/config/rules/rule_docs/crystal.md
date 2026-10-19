> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the code under review; do not infer behavior of methods or macros defined outside this file.

#### Obvious Typos or Spelling Errors
- Spelling errors in method names, class/struct/module names, instance/class variable names, or constant names at their declaration sites; do not report spelling errors at call sites
- Typos in `Log` messages, raised exception messages, docstrings, or other public diagnostics that affect readability

#### Type Inference and Nil Safety
- Union-typed values (`T | Nil`) accessed without a `nil` check, `.not_nil!`, or a safe-navigation pattern, risking a compile-time union that silently includes `Nil` in a path the author did not expect
- Overuse of `.not_nil!` or `as(T)` to force a type instead of narrowing with `if`/`case`/`responds_to?`, which converts a compile-time type error into a runtime `NilAssertionError` or `TypeCastError`
- Instance variables declared without an initializer and without a type restriction, letting the compiler infer a wider union (including `Nil`) than intended
- Methods whose return type varies by branch in a way that widens the inferred return union unexpectedly, especially when an early branch returns `nil` implicitly

#### Macro Hygiene and Code Generation
- Macros that expand to code referencing identifiers not defined within the macro's own scope, causing name capture or clashing with caller-scope locals
- `{{ }}` interpolation of untrusted or externally derived strings into generated code paths, expressions, or method names (macro-based code injection)
- `macro` or `{% %}` compile-time logic that assumes a fixed argument shape without validating arity or type, producing a confusing compile error far from the call site
- Generated methods/classes via `macro method_missing`, `{% for %}`, or `finished` hooks that unintentionally override or shadow existing methods

#### Fibers, Concurrency, and Channels
- Shared mutable state (class variables, `Hash`/`Array` instances) written by multiple fibers via `spawn` without a `Mutex`, `Channel`, or single-owner design; Crystal fibers are cooperative but a yield point (I/O, `Channel#receive`, `sleep`) can interleave unexpectedly
- `Channel#send` without a corresponding `receive`, or unbuffered channels used in a way that can deadlock when no other fiber is scheduled to receive
- Fibers spawned with `spawn` whose failures are never observed — unhandled exceptions inside a spawned block terminate only that fiber and can be silently lost
- Blocking, non-yielding CPU-bound work inside a `spawn`ed fiber that starves other fibers on the same thread when running in single-threaded (non-MT) mode
- Loop-captured variables shared across multiple `spawn` calls without a per-iteration local binding, so all fibers observe the same mutated value

#### C Bindings and Memory Safety
- `lib` bindings (`fun`, `struct`, `union`) with a C signature that does not match the actual C declaration in argument types, return type, or calling convention, risking memory corruption
- `Pointer(T)` arithmetic, `malloc`/`free` via `LibC`, or `.to_unsafe` calls without validating allocation size, alignment, lifetime, and null-ness before dereferencing
- Crystal-managed objects passed to C code that retains the pointer beyond the object's GC-visible lifetime (e.g. no `pointerof`/`GC.add_finalizer` safeguard), risking use-after-free once the GC moves or collects it
- C strings obtained via `.to_unsafe` or `String.new(pointer)` without confirming null-termination and correct encoding/length
- Missing `finalize`/`GC.add_finalizer` cleanup for C-allocated resources (file handles, buffers) acquired in a wrapper class

#### Exception Handling
- `rescue` without a specific exception type (bare `rescue`) that swallows unrelated errors, including `Exception` catching things a narrower `rescue IO::Error` should isolate
- Empty or logging-only `rescue` blocks that discard the original exception without re-raising or returning an explicit error signal
- Resources opened without `ensure` (or a block form like `File.open(...) do |f| ... end`) when an exception between acquisition and use can leak file handles, sockets, or locks
- Raising a plain `Exception` instead of a specific, well-named exception subclass, forcing callers into overly broad `rescue` clauses

#### Shards and Versioning
- `shard.yml` dependencies pinned to a branch or unconstrained version instead of a semantic version requirement, risking non-reproducible builds
- Missing `shard.lock` commit for an application (as opposed to a library), which defeats reproducible dependency resolution
- Version requirements incompatible with the declared `crystal` version constraint in `shard.yml`, risking a build that fails on the stated minimum compiler version

#### Security-Sensitive Code
- Untrusted input interpolated into `Process.run`/`Process.exec` with `shell: true`, or into a system/backtick command, without validation (command injection)
- Untrusted input interpolated directly into SQL strings instead of using parameterized queries via a database driver
- `Marshal.load` or other deserialization of untrusted data
- Logging secrets, tokens, credentials, or personally identifiable information
