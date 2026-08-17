#### Swift Review Principles
> Favor precision over recall: report only defects likely real in changed code and reachable execution paths. Prioritize crashes, data corruption, security issues, privacy issues, and concurrency bugs. Do not report style preferences.

Before reporting non-local behavior, use `file_read` and `code_search` to verify ownership, callers, synchronization, lifecycle, and input sources. Do not infer threading, retain cycles, or error contracts only from names or types. Do not duplicate compiler, SwiftLint, or Xcode analyzer findings unless the diff creates concrete correctness impact.

#### Optionals and Runtime Failures
- Force unwrap, force cast, or `try!` on runtime-derived values (user input, network responses, persistence, decoding, external state) where failure is reachable and not handled.
- Implicitly unwrapped optionals outside controlled framework lifecycle patterns where access can occur before initialization or after invalidation.
- Optional handling that converts required failure into silent incorrect behavior, missing data, or invalid state.

#### Memory Ownership and ARC
- Escaping closures stored by an object that strongly capture that same object, creating a retain cycle.
- `[unowned]` captures in escaping closures where object lifetime is not guaranteed until execution.
- Delegate, observer, callback, timer, or task relationships that create ownership cycles or continue work after owner destruction.
- Combine subscriptions capturing `self` strongly inside an owner of the cancellable when it prevents expected deallocation.
- Async streams, notifications, timers, or subscriptions started without lifecycle cleanup when they continue after dismissal/deallocation.

#### Error Handling
- Throwing operations or `Result` failures ignored, replaced with success values, or hidden when failure changes behavior or data correctness.
- `try?` removing required failure information where callers need failure distinction.
- Empty error handling blocks suppressing failures affecting integrity, security, or user-visible behavior.
- Error wrapping removing typed error information required by callers.
- `fatalError`/`preconditionFailure` used for recoverable runtime failures instead of typed propagation.

#### Swift Concurrency and Isolation
- Mutable state accessed across actor boundaries without isolation or synchronization where concurrent access is possible.
- Non-`Sendable` values crossing isolation boundaries where races or unsafe assumptions are possible.
- `@unchecked Sendable` or `nonisolated(unsafe)` introduced without a proven thread-safety invariant.
- Actor-isolated state accessed from callbacks, delegates, or closures without preserving isolation.
- Fire-and-forget tasks that outlive owners, cannot be cancelled, or continue side effects after lifecycle ends.
- Detached tasks used where inherited actor context, priority, cancellation, or isolation is required.
- Async work ignoring cancellation and continuing expensive computation or side effects.
- Continuation wrappers that can resume multiple times, never resume, or resume after ownership/lifecycle invalidation.
- Locks or synchronous waits used across `await` boundaries.
- Independent async operations introduced sequentially causing measurable user-visible latency regression.

#### SwiftUI State and Lifecycle
- View-owned reference state recreated across renders because ownership/lifetime is incorrect.
- Dynamic collections using unstable identity causing incorrect row reuse or state association.
- Side effects executed from `body` or computed properties causing repeated execution.
- Lifecycle async work continuing after disappearance when cancellation ownership is required.
- `.task(id:)` missing where replaced inputs can allow stale results to overwrite newer state.
- UI state mutated outside required main actor isolation when concurrent updates are possible.
- Lifecycle effects duplicated or misattributed across remount, presentation, or dismissal paths.
- User-visible strings added or changed without localization coverage.

#### Persistence and Data Integrity (SwiftData / Core Data)
- Persistence writes leaving stored state partially updated or inconsistent after failure.
- Schema or relationship changes without compatible migration handling for existing data.
- Relationship configuration changes causing orphaned objects, invalid references, or incorrect delete behavior.
- `@Query`/fetch predicates or sort descriptors matching incorrect data or causing avoidable expensive fetches.
- Cached or persisted values treated as authoritative when they can become stale and affect correctness.

#### Health and Privacy Data
- HealthKit access performed without required authorization handling or safe fallback behavior.
- Health or sensitive data written to logs, analytics, insecure storage, or plaintext persistence.
- Health claims introduced without required supporting source or compliance basis.
- Health queries, observer queries, or background delivery registrations missing lifecycle handling, causing missed updates or unnecessary resource use.

#### Purchases and Entitlements
- Purchase, restore, or entitlement state failing to handle pending, offline, or verification outcomes.
- Transaction listeners missing, incorrectly scoped, or failing to consume verified transactions.
- Paywall or entitlement UI using stale state instead of canonical entitlement state.
- Trial, restore, or purchase error paths granting or revoking entitlement incorrectly.

#### Combine and Reactive Streams
- Combine subscriptions causing ownership cycles or continuing after intended lifecycle.
- UI updates delivered without required scheduler guarantees (`receive(on:)`/equivalent), causing incorrect thread execution.
- Expensive upstream work executed on inappropriate schedulers where it blocks UI or causes latency.
- Streams without cancellation/backpressure handling where unbounded work or memory growth is possible.

#### Networking
- Authentication tokens, credentials, or sensitive data exposed through logs, storage, or requests.
- Signed/authenticated URLs with bypassed expiry, validation, or authorization checks.
- Retry logic causing request storms or missing backoff for transient failures.
- Cache handling serving stale or unauthorized responses.
- Disabled transport protections or weakened certificate validation where an existing security boundary depends on it.
- Client-controlled identity, authorization, or payment values trusted without server validation.

#### Web Views, Deep Links, and External Input
- WKWebView JavaScript bridges accepting unvalidated messages or exposing privileged actions.
- Navigation handlers allowing untrusted URLs or schemes without validation.
- Deep-link inputs changing authenticated state or sensitive actions without validation.

#### Performance and Resource Usage
- Expensive synchronous work on the main actor/thread blocking interaction.
- Repeated expensive work on frequently executed paths causing measurable regressions.
- Unbounded memory growth from collections, caches, tasks, streams, or retained objects.
- Inefficient algorithms on demonstrably large collections causing user-visible slowdown.

#### Security
- Secrets, credentials, tokens, private keys, or sensitive user data added to source, logs, fixtures, or insecure storage.
- User-controlled input passed into executable contexts, unsafe URLs, queries, or commands without validation.

#### Unsafe Interoperability
- Unsafe pointer, buffer, or memory APIs used without guaranteed lifetime or bounds.
- Objective-C/C bridging violating ownership, nullability, or lifetime assumptions.

#### Testing Correctness
- Tests relying on arbitrary sleeps or timing delays instead of async expectations or direct awaiting.
- Tests not exercising changed behavior paths where regressions are likely.
- Tests sharing mutable global state causing isolation failures.
- Async tests leaving tasks running after completion.
- Assertions that cannot fail for the regression they intend to detect.
- Tests depending on uncontrolled environment state (network, time, locale, global persistence) where isolation is required.
