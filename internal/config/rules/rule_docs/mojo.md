> Favor precision over recall: only raise an issue when you are confident it is a real defect. Mojo is a young, fast-evolving language layered on Python/MLIR semantics — do not report a construct as wrong solely because it looks unfamiliar, and account for the project's declared Mojo version before flagging syntax or stdlib API changes.

#### Obvious Typos or Spelling Errors
- Spelling errors in `struct`, `fn`, `trait`, and `alias` names at their declaration sites; do not report spelling errors at call sites
- Typos in docstrings, error messages, or `assert_*` failure messages that affect readability

#### Ownership, Borrowing, and Lifetimes
- `borrowed`/`inout`/`owned` argument conventions that do not match how the parameter is actually used (e.g. `borrowed` on a parameter that is mutated, or `owned` taken when a borrow would suffice and now forces an unnecessary copy)
- Values moved with `^` (the transfer sigil) while a reference to the original binding is still read afterward, or a transfer used where an implicit copy was actually intended
- `__copyinit__`/`__moveinit__`/`__del__` implementations that do not correctly duplicate or release owned resources (buffers, file handles, pointers), risking a double-free, use-after-free, or resource leak
- Structs holding a raw pointer or `UnsafePointer` field without a corresponding destructor, so instances leak the pointee when they go out of scope

#### Value Semantics and Struct Design
- `struct`s intended to have reference semantics but missing `@register_passable` or `@value` decisions appropriate to the type, causing unexpected copies of large payloads on every pass
- Overloaded operators or `fn __eq__`/`__hash__` pairs that are inconsistent with each other, breaking use as dictionary keys or set members
- Implicit conversions between numeric SIMD/scalar types that silently narrow precision (e.g. `Float64` to `Float32`) in a hot numeric path without an explicit cast signaling intent

#### Unsafe and Low-Level Interop
- `UnsafePointer`, `DTypePointer`, or raw memory APIs (`alloc`, `free`, `bitcast`, `load`/`store`) used without a matching deallocation, correct alignment, or a verified bounds check on the access
- `external_call`/FFI bindings to C/Python whose declared signature (types, calling convention, ownership of returned memory) does not match the actual foreign function
- Pointer arithmetic or manual indexing into a `Tensor`/`Buffer`'s underlying storage that bypasses shape/stride validation, risking out-of-bounds reads/writes
- Untrusted input (sizes, indices, buffer lengths) flowing into an unsafe pointer operation without validation at the boundary where it enters Mojo code

#### Python Interop Boundary
- Values crossing the `PythonObject`/native-Mojo boundary without validating type and structure, since Python's dynamic typing gives no compile-time guarantee on the Mojo side
- Python exceptions raised across the interop boundary that are not caught and translated into a Mojo-side error path, leaving the caller with an unhandled failure
- Performance-critical loops that call back into Python object methods per-iteration instead of converting to native Mojo types once, negating Mojo's performance advantage over pure Python

#### Concurrency and Parallelism
- `parallelize`/`vectorize` closures that capture and mutate shared external state without synchronization, creating data races across parallel lanes or threads
- SIMD-width assumptions (fixed vector widths) hardcoded in a kernel that do not adapt to the target hardware's actual SIMD width, silently processing fewer or more elements than intended
- Parallel loop bodies with a loop-carried dependency (an accumulator or index computed from a previous iteration) that is incorrect to parallelize as written

#### Performance Anti-Patterns
- Unnecessary `owned`/copy semantics in hot numeric kernels where the equivalent NumPy/Python code would have used a view, defeating the point of dropping into Mojo for speed
- Missing `@parameter`/compile-time specialization on shapes, dtypes, or loop bounds that are known at compile time and would allow the compiler to unroll or vectorize more aggressively
- Bounds checks or dynamic dispatch left in an inner loop that could be hoisted out via compile-time `alias`/`@parameter if` specialization

#### Security-Sensitive Areas
- Secrets, credentials, or API keys embedded directly in Mojo source rather than loaded from environment/config at runtime
- Untrusted data deserialized or interpreted via raw pointer casts (`bitcast`) without validating the source buffer's length and layout first
