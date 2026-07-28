#### Go Review Principles
> Favor precision over recall: report only defects that are likely real in the changed code and its reachable context. A false positive costs reviewer trust. Treat correctness and security findings as blocking; style-only suggestions are non-blocking. Focus on language-specific risks that ordinary formatting and deterministic tooling do not already cover.

Before reporting a non-local claim, use `file_read` and `code_search` to establish the relevant call sites, ownership, synchronization, and input boundaries. Do not infer concurrent invocation, attacker control, resource ownership, or an error contract solely from a function name or package import. Do not duplicate findings that `go vet`, Staticcheck, `go test -race`, the compiler, or `gofmt` can determine reliably unless the diff shows a concrete user-visible consequence those tools will not express.

#### Errors, Panics, and API Contracts
- Errors returned from calls that are ignored, overwritten, or converted into success/default values that hide a failed operation. A deliberately best-effort operation is acceptable only when the ignored failure is safe and documented or otherwise evident from the context.
- Error wrapping that loses the original cause (`fmt.Errorf("...: %v", err)` when callers need `errors.Is`/`errors.As`), wraps a nil error, returns a misleading sentinel, or exposes internal/sensitive details across a public boundary. Prefer `%w` when preserving identity is required.
- Errors handled without useful context at a boundary where the operation, resource, or input would otherwise be unclear. Do not demand wrapping at every propagation layer or where a sentinel/typed error is the intended contract.
- `panic`, `log.Fatal`, `os.Exit`, or a must-style helper in request, worker, library, or cleanup paths where a recoverable error can be returned. Do not flag a panic that enforces an impossible internal invariant or an explicitly documented programmer contract.
- Deferred cleanup that overwrites a primary error, drops a meaningful `Close`/`Commit`/`Rollback` error, or returns success after cleanup makes the result invalid. Account for APIs where `Close` errors are intentionally irrelevant after a successful read.

#### Nil, Interfaces, and Value Semantics
- A typed nil pointer, map, slice, function, channel, or error stored in a non-nil interface and later treated as absent. Check the concrete assignment and all interface checks before reporting.
- Nil maps written to, nil channels used unintentionally (which block forever), or nil pointers dereferenced on paths inputs or constructors can actually produce.
- Copying a value after first use when it contains `sync.Mutex`, `sync.RWMutex`, `sync.Once`, `sync.Pool`, `atomic` state, or another non-copyable synchronization primitive. Flag copies through value receivers, assignment, return, append, map values, or serialization only when the value can have been used first.
- Value receivers or copies that silently mutate only a copy when callers expect shared state, especially for structs holding maps, slices, pointers, locks, or atomic state. Do not flag intentional immutable value objects.
- `sync.Once` used for work that must be retried after a failure. `Once.Do` accepts a `func()` and considers it complete even if that function panics; an error captured by the closure also does not make a later `Do` retry it.

#### Context, Goroutines, and Cancellation
- Request-scoped work started with `context.Background()`/`TODO()` or a fresh context when it should inherit the caller's deadline, cancellation, values, or tracing. Background work that is explicitly independent is valid.
- `context.Context` stored in a struct or replaced with a custom context interface when it should be passed explicitly as the first parameter. Allow it only when a required external/standard-library interface fixes the method signature, or when the surrounding API makes the ownership and lifetime unambiguous.
- `context.WithCancel`, `WithTimeout`, or `WithDeadline` whose cancel function is not called once the derived context is no longer needed, unless ownership is deliberately transferred and its eventual call is evident. A deferred cancel immediately after creation is normally correct.
- Blocking I/O, waits, retries, selects, or loops on a request/worker path that lack a cancellation or deadline path where the dependency can stall. Confirm the operation can outlive its caller before flagging.
- Goroutines that can outlive their owner because they wait forever on a channel, lock, I/O operation, or unbounded retry; lack a shutdown signal; or have no way for errors/completion to be observed when that matters.
- Fire-and-forget goroutines that capture request-local mutable data, write to a response after the handler returns, panic without recovery at a process boundary, or race with cleanup. Do not require joining genuinely independent background work.
- Loop-variable or mutable outer-variable captures in goroutines/callbacks where the closure can observe a later value. Verify the module's `go` directive and whether the code creates a new variable per iteration before reporting; Go 1.22 language semantics changed range-loop variable behavior, while older-module semantics can retain the shared variable.

#### Channels, Locks, and Shared State
Only report races or deadlocks with evidence that the state is reachable concurrently; inspect surrounding call sites where that is not local. Do not flag immutable data, per-goroutine locals, synchronization already guaranteed by ownership, or a race detector/static-analysis concern without a concrete interleaving.

- Unsynchronized concurrent reads/writes of maps, slices, pointers, counters, caches, or compound state; check-then-act sequences such as load-then-create, existence-then-insert, or compare-then-update that can interleave.
- Holding a mutex/RWMutex across blocking I/O, channel operations, callbacks, network/database calls, or long CPU work when another path needs the lock to make progress. Check lock ordering and ownership before claiming deadlock.
- `RLock` used while mutating protected data; unlocked mutation of a field whose peers consistently protect it; or atomic and non-atomic access mixed for the same state.
- Sends or receives that can block indefinitely because the peer may stop, a buffer can fill, or shutdown/cancellation is not selected. Do not flag a deliberately synchronous handoff with a proven peer.
- Closing a channel from multiple possible senders, sending after a receiver/owner can close it, or double-closing a channel. The closer should normally be the sole sender/owner; establish that ownership from the surrounding code first.
- `select` with a `default` that causes busy spinning, drops required work, or bypasses cancellation; timeouts/retries that spin without backoff or a bound.
- WaitGroups with `Add` racing with `Wait`, missing `Done`, copied after first use, or a counter whose completion cannot be reached. Do not require `errgroup` or channels where a WaitGroup is adequate.

#### Timers, Tickers, and Resource Lifecycle
- A timer or ticker retained by its owner and left running after its work ends, so it can still fire, tick, retain reachable state, or keep associated work alive. Do not report a missing `Stop` solely as a garbage-collection leak: Go 1.23+ can recover unreferenced, unstopped timers and tickers, while older semantics and `GODEBUG=asynctimerchan=1` differ.
- `time.After` in a repeated/select loop only when evidence shows a real cost: for example, pre-Go-1.23 semantics with many unexpired timers, a high-frequency allocation path, or timer channels/owners that remain reachable. Do not call it a leak by itself on Go 1.23+.
- Timer reset/stop code that assumes one set of semantics across Go versions. For channel timers, Go 1.23+ eliminates stale values after `Reset`/`Stop`; older semantics require stop-and-drain coordination. For `AfterFunc`, `Reset`/`Stop` do not wait for an already-started callback, so repeated callbacks may overlap unless explicitly synchronized.
- `http.Response.Body`, `sql.Rows`, files, sockets, compression readers/writers, transactions, locks, or other closable resources not closed on every reachable path after successful acquisition. Do not flag bodies explicitly handed to a caller or framework that owns closure.
- `defer` inside a loop whose surrounding function can run for many iterations or indefinitely, especially when it delays closing files, response bodies, rows, locks, or transactions until the function returns. Do not flag a loop with a small, statically bounded iteration count or a helper function that returns per iteration.
- `sql.Rows` iteration that omits `rows.Err()` after the loop, or rows not closed when iteration may stop early. Do not require a redundant `Close` when the documented helper fully owns it.
- HTTP response bodies neither read nor closed when connection reuse or cleanup depends on it; response/request contexts and timeouts missing on outbound calls that can hang in a server path.
- Transactions that can return/branch after `Begin` without rollback/commit, commit errors ignored when callers rely on durability, or rollback deferred in a way that masks a successful commit incorrectly.

#### Collections, Slices, Bytes, and Numeric Boundaries
- Returning, caching, or passing a slice/map/byte buffer whose backing storage is later reused or mutated, unintentionally changing data observed by another owner. Confirm ownership; zero-copy APIs can be intentional and documented.
- `append` to a slice that aliases caller/shared backing storage where the mutation can escape unexpectedly; retaining a small subslice of a very large buffer where the retained memory matters.
- Indexing, slicing, length arithmetic, or capacity assumptions that can exceed bounds on a reachable empty/boundary input. Validate prior checks and API guarantees first.
- Integer conversion, narrowing, signed/unsigned comparison, size calculation, allocation, or offset arithmetic that can overflow, truncate, wrap, or turn negative input into a huge size. Consider architecture-dependent `int` width.
- Reusing a `bytes.Buffer`, `strings.Builder`, scanner, encoder, decoder, or mutable package/global state concurrently or after returning its bytes/string where API lifetime rules make the result invalid.

#### Security-Sensitive Boundaries
Confirm the value is attacker-controlled or crosses a trust boundary before reporting. Prefer a specific exploit path and remediation over generic warnings.

- SQL, shell commands, URLs, file paths, headers, templates, regexes, or serialized data assembled from untrusted input without appropriate parameterization, validation, escaping, allowlisting, or scheme/host/path restrictions. For `os/exec`, command argument arrays are usually safer than a shell but still need validation when the invoked program interprets arguments.
- Path traversal, symlink-following, unsafe archive extraction, insecure temporary-file creation, or permission/ownership assumptions that can expose or overwrite files. Account for any canonicalization and root containment checks already in place.
- `html/template` replaced with `text/template` for HTML, trusted-template types constructed from untrusted content, or context-inappropriate escaping that permits injection. Do not flag `text/template` for non-HTML output by default.
- SSRF or credential leakage through outbound URLs: untrusted destinations, missing allowlists where required, redirects to internal services, or forwarding sensitive headers/tokens to a different host.
- Secrets, credentials, session tokens, authorization headers, private keys, or sensitive personal data logged, returned in errors, or embedded in source/configuration.
- `math/rand` or `math/rand/v2` used to generate security-sensitive keys, tokens, session identifiers, reset codes, nonces, or salts; require `crypto/rand` or a vetted cryptographic construction instead. Do not flag non-security simulation, sampling, load-balancing, or test randomness.
- `reflect`, `unsafe`, cgo, unsafe pointer conversion, manual memory/layout assumptions, or custom cryptography that lack a narrow, documented invariant and necessary bounds/lifetime checks. Do not flag ordinary reflection merely because it is reflection.

#### Tests and Review Scope
- Review production Go changes by default. `*_test.go` files are excluded by OCR's default path filter; they are reviewed only when explicitly included by user configuration. Do not change that policy as part of this rule.
- For changed correctness, concurrency, error, or boundary behavior, suggest tests only when there is a concrete untested failure mode. Favor deterministic tests; do not demand flaky timing-based race tests when `go test -race`, controlled synchronization, or existing coverage is the proper validation.
- Do not make formatting, import ordering, naming preferences, simplification, or advice already enforced by `gofmt`, `go vet`, Staticcheck, linters, or the compiler into blocking findings.

#### References
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) and [Effective Go](https://go.dev/doc/effective_go) inform the Go-specific guidance above. Use them to understand semantics, not to turn formatting or style preferences into findings.
- Check current standard-library documentation for version-sensitive APIs, especially [`context`](https://pkg.go.dev/context), [`sync`](https://pkg.go.dev/sync), and [`time`](https://pkg.go.dev/time), before reporting a lifecycle or concurrency defect.
