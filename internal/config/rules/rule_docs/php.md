#### PHP Review Principles
> Favor precision over recall: report only defects that are likely real in the changed code and its reachable context. Treat correctness and security findings as blocking; style-only suggestions are non-blocking. Account for the project's PHP version and framework conventions before reporting version- or lifecycle-dependent behavior.

Before making a non-local claim, use `file_read` and `code_search` to verify callers, input sources, framework configuration, template context, and resource ownership. Do not duplicate findings reliably enforced by PHPStan, Psalm, PHP_CodeSniffer, the formatter, or the PHP compiler unless the diff demonstrates a concrete consequence those tools do not express.

#### Type Juggling, Equality, and Null Semantics
- Loose comparison (`==` or `!=`) whose coercion can make distinct security- or domain-sensitive values compare equal. Prefer strict comparison when operands are expected to have the same type; do not flag deliberate, validated normalization.
- Truthiness or `empty()` checks that incorrectly treat `0`, `"0"`, `false`, `null`, and an empty value as equivalent when those states have different meanings.
- `isset()` used when a present key with a `null` value must be distinguished from a missing key; use `array_key_exists()` when presence, rather than non-nullness, is the contract.
- Nullable, union, or `false`-returning APIs whose failure value reaches code that assumes a usable object, scalar, or resource. Confirm the declared and runtime contract before flagging.
- Numeric-string, arithmetic, or comparison behavior that depends on a different PHP version from the one supported by `composer.json`, CI, or deployment configuration.

#### Arrays, Iteration, and Value Semantics
- Array keys read without handling a reachable missing-key path, especially request data, decoded JSON, database rows, or optional configuration.
- A `foreach` value variable iterated by reference and then reused without `unset()`, leaving it aliased to the final element and allowing later assignments to corrupt the array.
- Array union (`+`), `array_merge`, spread syntax, or numeric-key reindexing used with semantics different from the intended overwrite and ordering behavior.
- Callbacks or closures that capture a loop variable by reference and later observe an unintended final or mutated value.
- Mutation during iteration that can skip, duplicate, or unexpectedly retain elements. Do not flag mutation whose traversal behavior is deliberate and locally evident.

#### Errors, Exceptions, and API Contracts
- `Throwable` or `Exception` caught and silently discarded, converted into success, or replaced with a misleading default on a path where the failure matters.
- Catching a broad exception around unrelated operations so the handler cannot distinguish the expected failure from a programming or infrastructure defect.
- A codebase contract inconsistently mixing exceptions, `false`, and `null` for the same failure, causing callers to miss an error path.
- Cleanup, rollback, or response-finalization code that hides the primary exception or returns success after the operation failed.
- Warnings or errors suppressed with `@` where suppression can turn a meaningful failure into invalid state. Do not flag a narrowly documented compatibility probe that checks the result safely.

#### Resources, Transactions, and Request Lifecycle
- Transactions, locks, database cursors, or resources in long-running processes not released, committed, or rolled back on every reachable path when delayed cleanup can exhaust capacity or break correctness. Do not flag ordinary request-scoped streams or files merely because PHP can release them at request shutdown.
- Database transactions with early returns or exception paths that can leave the transaction open, or nested transaction assumptions unsupported by the active driver/framework.
- cURL or stream operations lacking timeouts on a request or worker path where a remote endpoint can stall execution.
- Session locks held across slow network, database, or CPU work when concurrent requests for the same session must proceed.
- Do not report resources owned by a framework, dependency-injection container, generator consumer, or caller when ownership transfer is established by the surrounding code.

#### Database and ORM Correctness
- SQL assembled from untrusted values instead of parameter binding. Identifiers such as column names and sort directions cannot usually be bound and require an allowlist.
- Raw ORM expressions, query fragments, or dynamic table/column names that bypass the framework's normal parameterization with attacker-controlled data.
- Missing transaction boundaries when a changed multi-step write must be atomic, or side effects ordered so a rollback cannot restore consistency.
- N+1 queries or repeated remote calls only when the loop is reachable at meaningful scale and eager loading or batching preserves behavior.
- Mass-assignment exposure only when request-controlled fields reach a model and the framework's fillable/guarded/schema configuration does not already constrain them.

#### Web and Template Security Boundaries
Confirm attacker control and the output or execution context before reporting. Framework validation and auto-escaping may make an otherwise dangerous-looking operation safe.

- Untrusted output rendered without context-appropriate escaping for HTML text, attributes, URLs, JavaScript, or CSS. For `.phtml` templates, verify whether the view helper already escapes the value and whether raw HTML is intentional and sanitized.
- Authorization enforced only in a client, template, or hidden control rather than at the server-side operation; check route middleware, policies, voters, and controller guards before flagging.
- State-changing browser requests missing required CSRF protection when cookie-based authentication makes cross-site invocation possible. Do not flag token-authenticated APIs that are not vulnerable to ambient credentials.
- Redirects, response headers, or cookies built from untrusted data without validation or appropriate `Secure`, `HttpOnly`, and `SameSite` protections where those properties are required.
- File uploads trusted by client filename, extension, or MIME header alone; verify server-side type checks, generated storage names, destination boundaries, and executable-file handling.
- User-controlled paths used for filesystem access without normalization and boundary enforcement, enabling traversal, symlink escape, or unintended overwrite.

#### Dynamic Execution, Deserialization, and Outbound Requests
- `eval`, dynamic `include`/`require`, variable function calls, reflection, or shell commands reached by untrusted input without a strict allowlist.
- `unserialize()` on attacker-controlled data, including signed data where key management or verification is absent. Prefer a non-executable format; `allowed_classes` reduces object injection but does not make arbitrary data trustworthy.
- Shell commands built through concatenation or incomplete escaping. Prefer direct process APIs with separate arguments and validate option-like attacker-controlled values.
- Outbound URLs derived from untrusted input without required scheme, host, port, redirect, and private-network restrictions, enabling SSRF or credential forwarding.
- Weak randomness or password handling: predictable token generation, reversible password storage, manual password hashing, or non-constant-time comparison of secrets. Prefer `random_bytes`, `password_hash`, `password_verify`, and `hash_equals` as appropriate.
- Secrets, session identifiers, authorization headers, passwords, private keys, or sensitive personal data logged, returned in errors, or embedded in source.

#### Performance and Review Scope
- Report performance issues only with evidence of meaningful data scale or a hot path: repeated queries, accidental full-result materialization, quadratic array operations, or expensive work repeated inside a loop.
- Suggest tests only for concrete changed failure modes involving coercion, boundary values, errors, transactions, authorization, escaping, or framework configuration.
- Do not make formatting, naming, import ordering, modern-syntax preferences, or advice already enforced by deterministic PHP tooling into blocking findings.
