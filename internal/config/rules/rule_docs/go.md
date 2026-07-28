#### Go Review Principles
> Favor precision over recall: report only defects that are likely real in the changed code and its reachable context. A false positive costs reviewer trust. Treat correctness and security findings as blocking; style-only suggestions are non-blocking. Focus on language-specific risks that ordinary formatting and deterministic tooling do not already cover.

Before reporting a non-local claim, use `file_read` and `code_search` to establish the relevant call sites, ownership, synchronization, and input boundaries. Do not infer concurrent invocation, attacker control, resource ownership, or an error contract solely from a function name or package import. Do not duplicate findings that `go vet`, Staticcheck, `go test -race`, the compiler, or `gofmt` can determine reliably unless the diff shows a concrete user-visible consequence those tools will not express.

#### Errors, Panics, and API Contracts
- Errors returned from calls that are ignored, overwritten, or converted into success/default values that hide a failed operation. A deliberately best-effort operation is acceptable only when the ignored failure is safe and documented or evident from the context.
- Error wrapping that loses the original cause (`fmt.Errorf("...: %v", err)` when callers need `errors.Is`/`errors.As`), wraps nil, returns a misleading sentinel, or exposes internal/sensitive details at a public boundary. Prefer `%w` when preserving identity is required.
- `panic`, `log.Fatal`, `os.Exit`, or a must-style helper in request, worker, library, or cleanup paths where a recoverable error can be returned. Do not flag an impossible internal invariant or documented programmer contract.
- Deferred cleanup that overwrites a primary error, drops a meaningful `Close`/`Commit`/`Rollback` error, or returns success after cleanup makes the result invalid.

#### Nil, Interfaces, and Value Semantics
- A typed nil pointer, map, slice, function, channel, or error stored in a non-nil interface and later treated as absent. Check the concrete assignment and all interface checks first.
- Nil maps written to, nil channels used unintentionally (which block forever), or nil pointers dereferenced on paths inputs or constructors can actually produce.
- Copying a value after first use when it contains `sync.Mutex`, `sync.RWMutex`, `sync.Once`, `sync.Pool`, `atomic` state, or another non-copyable synchronization primitive. Flag copies through value receivers, assignment, return, append, map values, or serialization only when the value can have been used first.
- Value receivers or copies that silently mutate only a copy when callers expect shared state, especially for structs holding maps, slices, pointers, locks, or atomic state. Do not flag intentional immutable value objects.
- `sync.Once` used for work that must retry after failure. `Once.Do` considers its `func()` complete even if it panics; an error captured by that closure also does not make a later `Do` retry it.

#### Context, Goroutines, and Cancellation
- Request-scoped work started with `context.Background()`/`TODO()` or a fresh context when it should inherit the caller's deadline, cancellation, values, or tracing. Independent background work is valid.
- `context.Context` stored in a struct or replaced with a custom context interface when it should be passed explicitly as the first parameter. Allow it only where a required external interface fixes the signature or ownership/lifetime is unambiguous.
- `context.WithCancel`, `WithTimeout`, or `WithDeadline` whose cancel function is not called once the derived context is no longer needed, unless ownership transfer and eventual cancellation are evident.
- Blocking I/O, waits, retries, selects, or loops on a request/worker path that lack cancellation or deadline where the dependency can stall. Confirm the operation can outlive its caller first.
- Goroutines that can outlive their owner because they wait forever on a channel, lock, I/O operation, or unbounded retry; lack shutdown; or have no way for errors/completion to be observed when that matters.
- Fire-and-forget goroutines that capture request-local mutable data, write to a response after the handler returns, panic without recovery at a process boundary, or race with cleanup. Do not require joining independent background work.
- Loop-variable or mutable outer-variable captures in goroutines/callbacks where a closure can observe a later value. Verify the module's `go` directive and whether a new variable is created per iteration; Go 1.22 language semantics changed range-loop variables while older-module semantics can retain the shared variable.

#### Channels, Locks, and Shared State
Only report races or deadlocks with evidence that state is reachable concurrently; inspect surrounding call sites where that is not local. Do not flag immutable data, per-goroutine locals, or synchronization guaranteed by ownership.

- Unsynchronized concurrent reads/writes of maps, slices, pointers, counters, caches, or compound state; check-then-act sequences that can interleave.
- Holding a mutex/RWMutex across blocking I/O, channel operations, callbacks, network/database calls, or long CPU work when another path needs the lock to progress. Check lock ordering before claiming deadlock.
- `RLock` used while mutating protected data; unlocked mutation of a field whose peers protect it; or atomic and non-atomic access mixed for the same state.
- Sends/receives that can block indefinitely because a peer may stop, a buffer may fill, or shutdown/cancellation is not selected. Do not flag a synchronous handoff with a proven peer.
- Multiple possible channel closers, send-on-closed-channel risk, or double-close. Establish sender/owner responsibility first.
- `select` defaults that busy-spin, drop required work, or bypass cancellation; unbounded retries without backoff/cancellation.
- WaitGroups with `Add` racing with `Wait`, missing `Done`, copies after first use, or counters that cannot reach completion.

#### Timers, Tickers, and Resource Lifecycle
- A timer/ticker retained by its owner and left running after work ends, so it can fire, tick, retain reachable state, or keep associated work alive. Do not report missing `Stop` solely as a GC leak: Go 1.23+ can recover unreferenced unstopped timers/tickers; older semantics and `GODEBUG=asynctimerchan=1` differ.
- `time.After` in a repeated/select loop only with evidence of cost: pre-Go-1.23 semantics with many unexpired timers, a high-frequency allocation path, or retained timer owners. Do not call it a leak by itself on Go 1.23+.
- Timer reset/stop code that assumes one behavior across Go versions. Channel timers on Go 1.23+ eliminate stale values after `Reset`/`Stop`; older semantics require stop-and-drain coordination. `AfterFunc` reset/stop does not wait for an already-started callback, so callbacks can overlap unless synchronized.
- `http.Response.Body`, `sql.Rows`, files, sockets, compression readers/writers, transactions, locks, or other closable resources not closed on all reachable paths after acquisition. Do not flag resources handed to a caller/framework that owns closure.
- `defer` inside a loop whose surrounding function can run many iterations or indefinitely, especially when it delays closing files, response bodies, rows, locks, or transactions until function return. Do not flag small statically bounded loops or helpers returning per iteration.
- `sql.Rows` iteration that omits `rows.Err()` after the loop, or rows not closed when iteration can stop early.
- Outbound HTTP calls missing body cleanup, request context, or timeout on a server path that can hang; transactions with a return path after `Begin` lacking rollback/commit.

#### Collections, Slices, Bytes, and Numeric Boundaries
- Returning, caching, or passing a slice/map/byte buffer whose backing storage is later reused/mutated, changing data observed by another owner. Confirm ownership; zero-copy APIs can be intentional.
- `append` to a slice that aliases caller/shared backing storage where mutation escapes; retaining a tiny subslice of a huge buffer where retained memory matters.
- Reachable indexing, slicing, length arithmetic, or capacity assumptions that exceed bounds on empty/boundary input.
- Integer conversion, narrowing, signed/unsigned comparison, size calculation, allocation, or offset arithmetic that can overflow, truncate, wrap, or turn negative input into huge size. Consider architecture-dependent `int` width.
- Reusing mutable buffers, encoders, decoders, scanners, or package/global state concurrently or after returned output depends on their lifetime.

#### Security-Sensitive Boundaries
Confirm attacker control or a trust boundary before reporting. Prefer a concrete exploit path and remediation.

- SQL, shell commands, URLs, paths, headers, templates, regexes, or serialized data assembled from untrusted input without appropriate parameterization, validation, escaping, allowlisting, or scheme/host/path restrictions. `os/exec` argument arrays are safer than a shell but arguments may still need validation.
- Path traversal, symlink-following, unsafe archive extraction, insecure temp files, or permission/ownership assumptions that expose or overwrite files.
- `html/template` replaced with `text/template` for HTML, trusted-template types constructed from untrusted content, or context-inappropriate escaping. Do not flag `text/template` for non-HTML output by default.
- SSRF or credential leakage through outbound URLs: untrusted destinations, absent required allowlists, redirects to internal services, or sensitive headers forwarded cross-host.
- Secrets, credentials, tokens, authorization headers, private keys, or sensitive personal data logged, returned in errors, or embedded in source/configuration.
- `math/rand` or `math/rand/v2` used for security-sensitive keys, tokens, session IDs, reset codes, nonces, or salts; require `crypto/rand` or a vetted cryptographic construction. Do not flag simulation, sampling, load-balancing, or tests.
- `reflect`, `unsafe`, cgo, unsafe pointer conversion, manual memory/layout assumptions, or custom cryptography without narrow documented invariants and required bounds/lifetime checks. Do not flag ordinary reflection alone.

#### Tests and Review Scope
- Review production Go changes by default. `*_test.go` files remain excluded by OCR's default path filter and are reviewed only when user configuration explicitly includes them.
- Suggest tests only for concrete changed correctness, concurrency, error, or boundary failure modes. Favor deterministic tests; do not demand flaky timing-based race tests.
- Do not make formatting, import ordering, naming preferences, simplification, or advice already enforced by `gofmt`, `go vet`, Staticcheck, linters, or the compiler into blocking findings.

#### References
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) and [Effective Go](https://go.dev/doc/effective_go) inform this guidance. Use them to understand semantics, not to turn formatting/style preferences into findings.
- Check current standard-library documentation for version-sensitive APIs, especially [`context`](https://pkg.go.dev/context), [`sync`](https://pkg.go.dev/sync), and [`time`](https://pkg.go.dev/time), before reporting lifecycle or concurrency defects.
