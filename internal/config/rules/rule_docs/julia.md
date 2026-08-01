> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the code under review; do not infer behavior of methods or macros defined outside this file.

#### Obvious Typos or Spelling Errors
- Spelling errors in function names, struct/type names, field names, module names, or constant names at their declaration sites; do not report spelling errors at call sites
- Typos in log messages, `@error`/`@warn` text, exception messages, docstrings, or other public diagnostics that affect readability

#### Type Stability
- Functions whose return type depends on a runtime value in a way the compiler cannot infer (type-unstable functions), forcing boxed `Any` results and defeating the JIT
- Struct fields declared with abstract or non-concrete types (e.g. `field::Real`, `field::AbstractArray`, or untyped fields defaulting to `Any`); prefer concrete types or type parameters so instances are stored efficiently
- Containers created as `[]`, `Vector{Any}`, or `Dict()` without element types when a concrete element type is known
- Accumulator or loop variables whose type changes across iterations (e.g. initializing `x = 0` then assigning a `Float64`), which widens the inferred type

#### Multiple Dispatch and Method Definitions
- Method signatures typed as `::Any` (or untyped) that are broader than intended and silently capture unrelated argument types, creating ambiguity or wrong-method selection
- Method ambiguities: two methods equally specific for some argument tuple, so a call errors or resolves unpredictably
- Type piracy: defining or extending a method where neither the function nor any of the argument types is owned by this module, which can change behavior for unrelated code
- Overloading `Base` functions (`==`, `hash`, `show`, `length`, `iterate`) inconsistently — e.g. defining `==` without a matching `hash`

#### Bounds and Indexing Safety
- `@inbounds` or `@simd` applied to a loop whose indices are not provably within bounds, which turns an out-of-bounds access into undefined behavior instead of a checked error
- 1-based indexing mistakes: off-by-one errors, assuming 0-based access, or hardcoding `1:length(x)` where `eachindex(x)` (or `firstindex`/`lastindex`) is correct for arbitrary or offset arrays
- Assuming a specific axis origin for arrays that may not start at index 1

#### Missing, Nothing, and Error Handling
- Conflating `nothing` (absence), `missing` (unknown data, propagates through comparisons), and `NaN`; comparisons like `x == nothing` or `x == missing` instead of `isnothing(x)` / `ismissing(x)` / `===`
- Functions that sometimes `throw` and sometimes `return nothing` for the same failure mode, forcing callers to handle both
- Relying on `@assert` for input validation or security checks: assertions may be disabled and must not guard correctness-critical invariants; use explicit `throw` with a typed exception instead
- Swallowing exceptions with an empty `catch` block or rethrowing without context; broad `catch` that hides real errors

#### Concurrency and Shared State
- Data races on shared mutable state updated from `Threads.@threads`, `Threads.@spawn`, or `@async` tasks without a lock, atomic, or per-task accumulation
- Mutating non-thread-safe globals or shared collections (`push!`, `setindex!`) concurrently from multiple tasks
- Assuming `@async` provides parallelism: it schedules a cooperative task on the current thread, so CPU-bound work needs `Threads.@spawn` instead
- Tasks spawned in a loop that capture and mutate a variable declared *outside* the loop, so every task shares one binding
- Non-reentrant use of a shared `Random` RNG across threads, giving correlated or racy results

#### Performance Anti-Patterns
- Untyped, non-const global variables read inside hot functions, which the compiler cannot specialize on; annotate with `const` or pass as arguments
- Abstract field types or `Any` containers in performance-sensitive structs (see Type Stability)
- Unnecessary allocations in hot loops: repeated array/`String` construction, slicing that copies where a `@view` would suffice, or splatting large collections into calls
- Growing arrays element-by-element without `sizehint!` when the final size is known

#### Security-Sensitive Code
- `eval`, `Meta.parse`, `include_string`, or `@eval` applied to untrusted or externally derived input (code injection)
- Untrusted input interpolated into an explicit shell invocation such as `sh -c` or `bash -c` (command injection); note that a plain backtick command passes its interpolated arguments straight to the process without a shell, so the risk arises only when a shell is invoked deliberately
- `ccall`, `unsafe_load`/`unsafe_store!`, `unsafe_wrap`, `pointer`, or `unsafe_string` used without validating length, alignment, lifetime, and null-ness of the underlying memory
- Building SQL or file paths through unchecked string concatenation/interpolation of external input
- Logging secrets, tokens, credentials, or personally identifiable information
