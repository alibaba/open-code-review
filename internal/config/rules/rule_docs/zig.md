> Favor precision over recall: report only issues that are likely to cause incorrect behavior, memory unsafety, security vulnerabilities, or material performance problems. Do not report formatting handled by `zig fmt`, and account for the project's Zig version, build mode (`Debug`, `ReleaseSafe`, `ReleaseFast`, `ReleaseSmall`), and active `comptime` configuration before raising compatibility findings.

#### Memory Safety and Illegal Behavior
- Slices, pointers, or `[]const u8` views that outlive the storage they refer to, especially addresses derived from stack locals, temporaries, or a buffer that is reused or freed
- Detectable illegal behavior — out-of-bounds indexing, integer overflow, `@intCast`/`@truncate` narrowing that loses value, null-unwrap of an optional, or invalid `@ptrCast`/`@alignCast` — reachable in `ReleaseFast` or `ReleaseSmall`, where safety checks are disabled and the same code becomes silent undefined behavior
- Reads of `undefined` memory, or use of a value before it is fully initialized
- `@ptrCast`, `@alignCast`, `@bitCast`, or pointer arithmetic without a locally established type, alignment, provenance, and lifetime invariant
- Do not report ordinary value copies or bounds-checked access in `Debug`/`ReleaseSafe` without evidence of a real lifetime or aliasing defect

#### Allocators and Resource Cleanup
- Memory obtained from an `Allocator` without a matching `free`/`destroy`, or freed with a different allocator than the one that allocated it
- Resources acquired without a corresponding `defer` or `errdefer`, so an early `return` or error path leaks them or leaves partial state
- `errdefer` missing on a value that is cleaned up only on the success path, causing a leak when a later step in the same function fails
- Double-free or use-after-free from a `deinit` that runs on an already-released or aliased object
- Ignoring the result of an allocation or a fallible call at a boundary where failure changes correctness

#### Errors, Optionals, and Control Flow
- Error unions discarded with `catch unreachable`, `catch undefined`, or `_ =` where the error is actually reachable at runtime
- `orelse unreachable` or `.?` on an optional that can legitimately be null for untrusted or runtime input
- `unreachable` or `@panic` used for ordinary invalid input in reusable library or server code, especially where it becomes illegal behavior in release-unsafe modes
- `switch` on an error set or tagged union that silently handles unrelated cases with `else` and hides a newly added variant
- Assertions (`std.debug.assert`) used to validate untrusted input, since they are compiled out in release-unsafe builds

#### Comptime, Generics, and Build Code
- `comptime` code or `@This()`-based generics that read mutable external state and make builds non-reproducible without an explicit requirement
- Type-parameter functions that assume capabilities (fields, methods, layout) a caller's type may not provide, producing confusing compile errors instead of a checked constraint
- `build.zig` steps that fetch, execute, or trust untrusted input, or that hardcode absolute paths and platform assumptions
- Do not report ordinary `comptime` use when the generated behavior is clear and inputs are validated

#### Concurrency and C Interop
- Shared mutable state accessed from multiple threads without a `std.Thread.Mutex`, atomic, or established single-owner design
- Locks held across blocking operations or callbacks, creating deadlock or starvation risk
- `extern`/`export` declarations or `callconv` annotations with incompatible types, struct layout, nullability, or ownership relative to the C side
- C strings or buffers consumed without validating length, null termination, encoding, and lifetime
- User-controlled data passed to process spawning, path access, SQL construction, or deserialization without validation, and secrets embedded in source, logs, or error messages
