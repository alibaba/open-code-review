> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in variable, function, class, or module names at their declaration sites; do not report spelling errors at reference sites, as these are determined by the declaration
- Strings in log messages or exception messages containing spelling errors that affect readability

#### Dead Code
- Code blocks that can never be reached (e.g., branches where the condition is always false, code after a `return`, `raise`, `break`, or `continue`)
- Variables, imports, or function parameters that are declared but never read or referenced
- Large blocks of commented-out code with no apparent intent to preserve

#### Mutable Default Arguments and Shared State
- Mutable default arguments such as `def f(x=[])` or `def f(x={})`; the default is created once and shared across every call. Default to `None` and build the value inside the body
- Class-level mutable attributes shared unintentionally across instances when a per-instance value was intended
- Module-level mutable globals (lists, dicts, caches) mutated across requests or threads, retaining state in ways that surprise the caller
- Closures that capture a loop variable by reference and all end up seeing its final value
- Do not report when the function never mutates the argument, or when the shared default is a deliberate, documented cache or sentinel

#### Boundary and Edge-Case Handling
- Empty inputs assumed to be non-empty: indexing `xs[0]`, `max()`/`min()`, or slicing without first handling the empty `list`, `str`, `dict`, or iterator
- Off-by-one and out-of-range access on indices, ranges, or slices, especially at the first/last element
- `None` reaching code that assumes a value, when an upstream call or default can legitimately return `None` (confirm the data source with `file_read` before flagging)
- Comparing floats for exact equality with `==`; use `math.isclose` or an explicit tolerance, since floating-point results are not exact
- Integer/float and division assumptions: unintended truncation with `//`, or `ZeroDivisionError` when a divisor can be zero
- Heterogeneous or unexpected element types in a collection that the code assumes are uniform (e.g., mixing `None`, numbers, and strings)
- Dictionary access by key without handling the missing-key case (`d[k]` vs `d.get(k)`), or set/dict operations that assume a key is present
- Do not report edge cases that a caller or type contract has already ruled out, or inputs that cannot occur given validated boundaries upstream

#### Error Handling and Exceptions
- Bare `except:` swallows everything, including `KeyboardInterrupt` and `SystemExit`; catch `except Exception` at minimum, and prefer the specific exception types you expect
- `except Exception` that is still broader than the failure being handled; narrow it to the exceptions actually raised by the guarded call
- Exceptions caught and silently discarded (`pass`) without logging or re-raising
- Original traceback lost when re-raising; prefer `raise NewError(...) from err` to preserve the cause
- Broad `try` blocks that wrap far more than the line that can actually fail, hiding where the error originates
- `assert` used for runtime validation of external input — assertions are stripped under `python -O`

#### Identity and Equality Comparisons
- Using `is`/`is not` to compare against literals such as strings, numbers, or tuples; this relies on implementation-specific interning rather than value equality — use `==` (a real correctness risk)
- Comparing against `True`/`False` with `==`, where a truthy-but-not-`True` value (e.g. `1`, a non-empty container) would compare unequal; prefer a plain truthiness check
- Reserve `is` for identity checks against singletons and sentinels
- Comparing against `None` with `==`/`!=` rather than `is`/`is not` is a style preference; report as minor, not blocking

#### Resource Management
- Files, sockets, locks, or database connections opened without a `with` statement, risking leaks on early return or exception
- Context managers available but bypassed in favor of manual `open()`/`close()` pairs
- Resources acquired in a `try` whose `finally` cleanup is missing or incomplete on the error path
- Iterators or generators holding resources open longer than necessary
- Do not report short-lived scripts, or handles already managed by an enclosing `with` or framework-managed lifecycle (confirm the surrounding scope with `file_read` before flagging)

#### Performance
Confirm data scale and that the code is on a hot path before flagging:
- Building strings with `+=` in a loop instead of accumulating in a list and `"".join(...)`, or using an f-string
- Repeated membership tests against a `list` where a `set` or `dict` would turn O(n) lookups into O(1)
- Building a full list when a generator would avoid holding everything in memory
- Recomputing inside a loop a value that is invariant across iterations (e.g., compiling a regex, attribute lookups in hot paths)
- Passing an eagerly formatted f-string to `logging` (e.g., `logging.info(f"...")`) instead of `logging.info("%s", value)`, which defeats lazy formatting when the level is disabled

#### Concurrency and Async
Only flag concurrency issues when there is evidence of multi-threaded, multi-process, or async invocation (confirm the call context before reporting):
- CPU-bound work parallelized with `threading` under the GIL where `multiprocessing` or a process pool is the right tool (traditional CPython; free-threaded builds excepted); I/O-bound work is the case threads actually help
- Check-then-act races on shared state without a `Lock`, or non-atomic compound updates assumed to be atomic
- Blocking calls (synchronous I/O, `time.sleep`, `requests`, CPU-heavy work) inside `async def`, stalling the event loop; use the async equivalent or run them in an executor
- `asyncio` tasks created and never awaited, so exceptions are swallowed and the work may be garbage-collected before it finishes
- Shared mutable state across threads or tasks without synchronization or a thread-safe structure

Do not report local variables (each thread has its own), read-only access to shared data, or code with no evidence of concurrent use.

#### Security-Sensitive Code
Validate the data source before flagging; confirm the input is actually attacker-controlled rather than a trusted constant:
- `eval`, `exec`, or `compile` on untrusted input; this is arbitrary code execution
- `subprocess` with `shell=True` built from unsanitized input; pass an argument list and avoid the shell
- `pickle`, `marshal`, or `yaml.load` (without `SafeLoader`) on untrusted data; deserialization can execute arbitrary code
- SQL built by string concatenation or f-strings instead of parameterized queries
- Secrets, tokens, passwords, or PII written to logs or committed in source
- Weak or misused cryptography (`hashlib.md5`/`sha1` for passwords, `random` for security tokens); use `secrets` and vetted libraries
- Untrusted file paths joined without validation, allowing path traversal
