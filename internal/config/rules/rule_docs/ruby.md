> Favor precision over recall: report only defects that are likely to cause incorrect behavior, security vulnerabilities, resource leaks, or material performance problems. Do not report formatting that RuboCop or standardrb can fix, a naming choice those linters already govern, or a project's established Ruby style replaced with a personal preference. This rule covers Ruby source (`.rb`), Rake tasks (`.rake`), gemspecs, `Gemfile`, and `Rakefile`. ERB templates (`.erb`) use `erb.md`. Account for the Ruby version, whether frozen string literals are in effect, and whether the file is Rails or ActiveRecord code before raising framework-specific findings.

#### Nil, False, and Absent Values
- A `&.` chain that swallows a nil the surrounding code must handle, so a later call is skipped and a required side effect, return value, or validation never runs
- `||=` used to fill in a missing value when `false` is a legitimate stored result, so a false setting is overwritten on every read
- `Hash#[]` (including `params[:key]` and a string-key versus symbol-key mismatch) used where a missing key must be distinguished from an explicit `nil`, or where `fetch` with a default or block is required
- A value that can be `nil` or `false` tested only by truthiness when those two states must take different paths
- Do not flag `&.` or `||` on a path that local guards, types, or an RBS contract already prove non-nil

#### Mutation and Frozen Strings
- Mutating a method argument, a frozen string or array, or a constant while the caller still holds the original, including `<<`, `gsub!`, `compact!`, and in-place Hash updates
- A string literal mutated under `# frozen_string_literal: true`, or `<<` onto a value shared across requests, class state, or a memoized constant, so later readers see the mutated contents
- An object duplicated without copying nested mutables the later write is meant to isolate
- Do not report mutation that is the documented purpose of a bang method on a locally owned object

#### Exceptions and Control Flow
- `rescue Exception` or `rescue Object` catching `SystemExit`, `NoMemoryError`, `SignalException`, or other conditions that must propagate
- A bare `rescue` or `rescue => e` wrapping more than the call that can fail, converting an unexpected `StandardError` into success or a misleading default
- `ensure` that `return`s, `break`s, or `throw`s, discarding the exception that caused `ensure` to run
- `retry` without a bounded counter, backoff, or a change that can make progress, so a persistent failure loops
- An exception rescued and discarded on a path where the failure must be recorded or propagated

#### Rails and ActiveRecord Data Access
- A query executed per element of an outer loop where the association can be loaded with `includes`, `preload`, or a single follow-up query, and the row count makes the extra round trips material
- Loading an unbounded relation with `each` or `to_a` where `find_each` or `in_batches` is required for the established volume
- SQL assembled by interpolating untrusted values into `where("... #{...}")`, `order`, `joins`, or `execute` instead of placeholders and binds; column and table names still need an allowlist because they cannot be bound
- Mass assignment of request params onto a model without strong parameters, or `update_column` / `update_all` / `update_columns` skipping validations, callbacks, or `updated_at` when those are part of the contract
- A model callback with a side effect (mail, payment, job enqueue, nested write) that can re-run, run after a rolled-back transaction, or cannot be skipped on a path that must bypass it
- Do not restructure readable queries purely for micro-optimizations, and do not flag `update_column` when skipping callbacks is the documented intent and nearby invariants still hold

#### Security and Trust Boundaries
- Untrusted values reaching `send`, `public_send`, `constantize`, `__send__`, `eval`, `instance_eval`, `class_eval`, or `module_eval`
- `Marshal.load` or `YAML.load` (versus `safe_load`, and versus `unsafe_load` on Psych versions that made `load` safe) on untrusted data, or `JSON.load` with `create_additions` enabled
- `html_safe` or `raw` wrapping attacker-controlled markup, or `system` / backticks / `%x` interpolating unsanitized input instead of a fixed command and a separate argument list
- Secrets, tokens, or credentials written into source, logs, exception messages, or committed configuration
- Do not flag a constant allowlist of method names looked up before `public_send`, or `YAML.safe_load` with a permitted-class list that matches the payload

#### Concurrency, Blocks, and Procs
- Class variables (`@@`), class-instance mutable state, or memoized `@ivar ||= ...` on a class or singleton used as a cache in a threaded server (Puma, Sidekiq) without synchronization, so requests race or leak data across tenants
- `return` inside a `Proc` (including a block stored and called later) intended to return from an enclosing method that may already have returned, raising `LocalJumpError`; a `lambda` returns from itself
- `Symbol#to_proc` (`&:foo`) used where the proc is called with extra arguments the named method does not accept, or where the receiver can be nil
- Do not flag ordinary request-local memoization on an instance that is not shared across threads
