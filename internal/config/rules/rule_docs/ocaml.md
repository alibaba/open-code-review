> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. This rule covers both OCaml (`.ml`/`.mli`) and ReasonML (`.re`/`.rei`), which are two surface syntaxes over the same semantics and compiler; apply every finding below to whichever syntax the file uses, and only flag JSX/BuckleScript-specific items in `.re`/`.rei` files.

#### Pattern Matching and Warning Discipline
- `match` expressions (or ReasonML `switch`) missing a case for a reachable constructor, especially after a variant type gains a new constructor; do not report matches that are provably exhaustive
- Wildcard `_` patterns added to silence a non-exhaustive-match warning instead of handling the missing case, which hides future constructor additions
- Catch-all `| _ -> ...` branches placed before more specific patterns, silently shadowing them
- Project or file missing `-warn-error +8` (or the dune equivalent enabling exhaustiveness as an error) where the surrounding code otherwise relies on the compiler catching non-exhaustive matches
- `[@warning "-8"]` or similar warning-suppression attributes applied broadly instead of to the single justified line

#### Mutable State and Aliasing
- `ref` cells or mutable record fields shared between closures, callbacks, or list/array iterations in a way that lets one hold stale or unexpectedly-updated state
- Mutable arrays (`Array.t`) passed to a function that both reads and mutates them where the caller still holds and relies on the original values
- Physical equality (`==`) or `Stdlib.compare`/structural `=` used interchangeably with mutable values without considering that mutation changes structural equality results after the fact
- Loops using `for`/`while` with an external mutable accumulator where a fold or recursive helper would make the state explicit and avoid an unintended shared mutation
- `Array.make`/`Bytes.make` reused across iterations by copying a reference to the same underlying buffer instead of a fresh allocation

#### Exceptions, Result, and Option Handling
- Functions that raise an exception for an expected, recoverable failure (e.g. `Not_found`, `Failure`, a custom exception) in code paths that should instead return `option` or `result` to make the failure visible in the type
- `Option.get`/`Option.value_exn`-style unwraps (or ReasonML `switch` fallthrough to `None -> assert false`) on a value that is not provably `Some` at that point
- `try ... with _ -> ...` (or bare `with exn -> ...` that doesn't re-raise) swallowing an exception without logging or converting it to a typed error, hiding the real failure from the caller
- Mixing `raise`-based and `result`-based error handling for the same operation across a module, forcing callers to guess which convention applies
- `Result.get_ok`/`Result.get_error`-style unwraps used where the `Error` case is reachable at runtime

#### Module System and Functors
- Functor applications (`module M = F(Arg)`) where `Arg` does not actually satisfy the invariants the functor's module type implies (e.g. an ordering module whose `compare` is not a total order), which can silently corrupt functor-built data structures
- Module types (`.mli` signatures) that leak an internal representation type instead of keeping it abstract, defeating the invariants the module is meant to enforce
- Re-exporting a submodule's mutable state through multiple module aliases in a way that creates hidden shared state between call sites that appear independent
- Shadowing a standard library module (e.g. `module List = ...`) without preserving the functions callers expect, causing silently different behavior at unrelated call sites
- First-class modules (`(module M : S)`) packed and unpacked in a hot path without considering the allocation and dynamic-dispatch cost versus a functor

#### Unsafe Interop and Escape Hatches
- `Obj.magic`, `Obj.repr`/`Obj.obj`, or other `Obj` module usage without a comment establishing the exact runtime representation invariant being relied on; this bypasses the type system entirely and any drift in representation is undefined behavior
- `Marshal` used to serialize closures or values across incompatible compiler versions/architectures, which is unsound and can crash or corrupt data
- C stub bindings (`external ... = "..."`) missing GC-safety annotations (`noalloc`, correct use of `caml_enter_blocking_section`) when they call into blocking or allocating C code
- Unsafe array/string access (`Array.unsafe_get`/`unsafe_set`, `Bytes.unsafe_*`) used without a preceding, visible bounds check that the compiler cannot otherwise verify

#### Dune Build Correctness
- `(libraries ...)` stanzas missing a dependency that the module actually references, relying on transitive re-exports that can break under a stricter dune profile
- Public/private library boundaries (`(public_name ...)` vs internal libraries) that expose internal modules unintentionally through a `.mli`-less module
- `(preprocess (pps ...))` (e.g. ppx_deriving, ppx_let) applied inconsistently across a project such that generated code silently differs between modules that should share a derivation
- Test executables or `(rule ...)` stanzas that shell out to unvalidated external input as part of the build, or that depend on files not declared in `(deps ...)`, making builds non-reproducible

#### ReasonML JSX and JS Interop
- `[@bs.deriving]`/`@genType`/`@react.component` (BuckleScript/ReScript-style) bindings whose declared JS type does not match the actual JS value's shape, especially for `null`/`undefined` versus OCaml `option`
- Untrusted JS input consumed via `Js.Json.t` decoded with an unsafe cast instead of a validating decoder, letting malformed JSON pass through the type boundary
- Promises (`Js.Promise.t`) whose rejection path is never handled, silently swallowing async JS errors
- JSX children or props built by string concatenation of untrusted values that get rendered into the DOM without escaping, when working with a raw `dangerouslySetInnerHTML`-equivalent binding
