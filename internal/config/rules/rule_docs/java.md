#### Obvious Typos or Spelling Errors
- Spelling errors in variable names, method names, or class names at their declaration sites (confirm by searching for naming conventions of similar identifiers using `code_search`)
- Strings in log messages or exception messages containing spelling errors that affect readability
- Do not report spelling errors at reference sites, as these are typically determined by the declaration

#### Dead Code
- Code blocks that can never be reached (e.g., branches where the condition is always false, code after a return statement)
- Variables that are declared but never read or referenced
- Large blocks of commented-out code (with no apparent intent to preserve)

#### Logic Error Detection
- Incorrect if-condition logic (use `file_read` to examine surrounding context and confirm expected logic)
- Boundary condition handling errors (pay special attention to index and array length checks)
- Misuse of boolean logic operators (precedence and short-circuit evaluation issues)
- Obvious infinite loops or recursion without termination conditions
- Use of return/break/continue where exiting is not intended
- Missing break statements in switch cases causing unintended fall-through
- Intentional fall-through lacking explanatory comments
- Code patterns that may cause NPE (confirm risk by inspecting the data source call chain using `file_read` and `code_search`)
- Missing parentheses in logical expressions that may cause execution order to differ from intent

#### Severe Performance Issues
- Database queries executed inside loops (use `code_search` to confirm whether the method call involves database operations)
- N+1 query problems (suggest batch query optimizations)
- Processing large datasets without pagination (use `file_read` to understand data scale and processing context)
- Inefficient algorithm implementations in nested loops (O(n^2) or higher complexity where a more optimal solution exists)

#### Thread Safety Issue Detection
Only flag thread safety issues in the following cases:
- **Race conditions**: A "check-then-act" pattern exists where intermediate state may be altered by another thread
- **Non-atomic compound operations**: Multi-step operations that require atomicity but lack synchronization mechanisms
- **Unsafe lazy initialization**: Double-checked locking defects in singleton patterns or cache implementations
- **Concurrent writes to thread-unsafe collections**: Modifications to non-thread-safe collections such as ArrayList or HashMap in a multi-threaded environment

Do not report in the following cases:
- **Local variables within methods**: These are inherently thread-safe, as each thread has its own copy
- **Single-threaded context usage**: No evidence of multi-threaded invocation (confirm by searching for relevant call contexts using `code_search`)
- **Read-only operations**: Even with non-thread-safe data structures, if only read operations are performed
- **Immutable objects**: References to final fields pointing to immutable objects
- **Proper synchronization already in place**: Code already uses synchronized, Lock, atomic classes, or other correct synchronization mechanisms
- **Components designed for single-threaded use**: Such as the building phase of a Builder pattern, temporary data transfer objects, etc.
