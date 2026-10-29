#### PowerShell Review Principles
> Favor precision over recall: report only defects that are likely to affect correctness, security, compatibility, or operational safety. Do not report formatting or naming preferences already covered by PSScriptAnalyzer. Account for the declared PowerShell version, target operating systems, privilege boundary, and whether the file is a script (`.ps1`), module (`.psm1`), or data/manifest file (`.psd1`).

Before reporting non-local behavior, use `file_read` and `code_search` to verify callers, parameter contracts, module exports, execution context, and input trust boundaries. Do not assume every use of aliases, `Write-Host`, the call operator (`&`), or a global variable is a defect without a concrete behavioral impact.

#### Command Construction and Injection
- Untrusted text passed to `Invoke-Expression`, `[scriptblock]::Create`, `powershell -Command`, `pwsh -Command`, `cmd /c`, `sh -c`, or another string-evaluation boundary where an attacker can introduce executable syntax.
- Commands assembled as one interpolated string where subsequent evaluation permits injection or quoting changes the intended arguments; do not report string construction alone.
- Remote content downloaded and executed without authenticity, integrity, or trust validation, including `Invoke-WebRequest ... | Invoke-Expression` patterns.
- User-controlled executable names, module names, script paths, or command paths invoked with `&`, dot-sourcing, `Start-Process`, or remoting without an allowlist or a constrained resolution boundary.
- User-controlled wildcard patterns, registry paths, certificate paths, or filesystem paths that can escape the intended scope, match additional targets, or trigger destructive operations.
- Quoting that expands variables or subexpressions earlier than intended, especially across native-command, remoting, or nested-shell boundaries.

#### Parameters, Types, and Input Validation
- Parameters accepting external input without the type, validation attributes, or explicit checks needed before privileged, destructive, or security-sensitive use.
- A parameter declared for pipeline input but processed outside `process`, causing only the final item or no items to be handled as intended.
- Confusing `$null`, an empty collection, and a scalar after PowerShell's automatic enumeration, producing incorrect branching or skipped work.
- Relying on truthiness where valid values such as `0`, `''`, or `$false` must be distinguished from missing input.
- Unsafe casts or enum conversions that can throw after partial side effects have already occurred.
- Returning or accepting loosely shaped `PSCustomObject` values where callers rely on properties that are absent on reachable paths.

#### Errors and Native Process Status
- Expecting `try`/`catch` to handle a non-terminating cmdlet error without `-ErrorAction Stop` or an equivalent scoped `$ErrorActionPreference` policy.
- Setting `$ErrorActionPreference` globally or suppressing errors with `SilentlyContinue`/`Ignore` in a way that hides a required failure or changes unrelated caller behavior.
- Empty or overly broad `catch` blocks that replace failure with plausible output, continue after partial state changes, or discard actionable error context.
- Assuming a native command failure throws a PowerShell exception; inspect the supported version, `$PSNativeCommandUseErrorActionPreference`, `$ErrorActionPreference`, and any native-command wrapper before requiring an explicit `$LASTEXITCODE` check. Interpret exit codes using the invoked command's contract, not a blanket nonzero-is-failure rule.
- Checking `$?` after another command has already overwritten it, or treating stderr output alone as proof that a native command failed.
- Cleanup, lock release, location restoration, session teardown, or temporary-state restoration skipped on a reachable failure or cancellation path; recognize `finally`, PowerShell 7.3+ `clean` blocks, and other disposal mechanisms appropriate to the supported version and lifetime.

#### Pipeline and Output Semantics
- Helper expressions or cmdlets emitting unintended objects into a function's success stream and corrupting the function's documented return value.
- Assuming `return` suppresses objects emitted earlier in the function or script block.
- Accidental collection unrolling that changes a nested collection or single collection object into separate pipeline records; preserve shape explicitly when callers depend on it.
- Using formatting cmdlets such as `Format-Table` before data leaves a reusable function or module, converting structured objects into formatting instructions instead of returning data.
- Mixing host, information, warning, error, and success streams in a way that breaks redirection, automation, or structured consumers.
- Materializing an unbounded pipeline where memory exhaustion is reachable for expected input sizes. For array append costs, verify the supported version and workload: PowerShell 7.5 optimized array addition, so `+=` alone is not evidence of quadratic runtime.

#### State Changes, Idempotence, and Cleanup
- State-changing advanced functions that claim or require `-WhatIf`/`-Confirm` behavior but omit or bypass `SupportsShouldProcess` and `$PSCmdlet.ShouldProcess()`.
- Retry or rerun behavior that duplicates resources, appends configuration repeatedly, or corrupts state instead of converging on the requested result.
- Multiple related mutations that can leave a partially applied configuration after an intermediate failure without rollback, compensation, or a clearly documented partial-state contract.
- Scripts changing process-wide location, environment variables, preference variables, default parameter values, or module state without restoring them when used as reusable components.
- Resources, jobs, runspaces, sessions, subscriptions, streams, or temporary files that remain active after success, failure, or cancellation.
- Destructive wildcard operations whose resolved targets are not checked before mutation.

#### Credentials, Transport, and Sensitive Data
- Passwords, tokens, connection strings, private keys, or credentials embedded in source, command arguments, logs, exceptions, transcripts, or module manifests.
- Exposing plaintext credentials beyond a necessary API boundary or retaining/logging their plaintext representation. Conversion to `SecureString` alone is not a defect and does not erase the original plaintext; do not assume its contents are encrypted on non-Windows platforms.
- Accepting separate username/password strings where `PSCredential`, managed identity, or an established secret provider is required by the surrounding code.
- Disabling certificate validation, using `-SkipCertificateCheck`, weakening TLS, or suppressing host verification across a real trust boundary.
- Trusting execution policy as an authentication or authorization control; execution policy is not a security boundary.
- Logging full request headers, environment variables, process arguments, or remote-session configuration that may contain secrets.

#### Remoting, Jobs, and Concurrency
- Assuming local variables are available remotely without `Using:`, parameters, or an explicit argument list, or expanding them in the wrong session because of quoting.
- Depending on methods, live handles, or private-key material lost during out-of-process job or remoting serialization; verify the actual type because some types are rehydrated rather than returned as deserialized property bags.
- Starting jobs, thread jobs, runspaces, or remote commands without observing completion and failures when the result affects correctness.
- Shared mutable state accessed from thread jobs, parallel pipelines, event handlers, or runspaces without synchronization or an ownership invariant.
- Event subscriptions or background jobs surviving beyond their owner and continuing unexpected side effects.
- Remoting endpoints, authentication mechanisms, or credential delegation that broaden access beyond the intended hosts or users.

#### Modules, Scopes, and Manifests
- Module code leaking mutable variables, aliases, functions, drives, or preference changes into global scope unintentionally.
- Public commands missing from `FunctionsToExport`, or private helpers exported through wildcards when the manifest is expected to define a stable API.
- `RootModule`, `NestedModules`, `RequiredModules`, compatible editions, or PowerShell version constraints that do not match the module's actual dependencies and syntax.
- Module initialization performing irreversible side effects on import without an explicit module contract.
- Dot-sourcing or importing a path derived from untrusted input, the current directory, or an ambiguous module search path.
- Evaluating untrusted `.psd1` content as code instead of importing data safely, or adding values the actual loader rejects. Distinguish `Import-PowerShellDataFile` from module-manifest restricted language; supported manifest variables and expressions are not arbitrary code execution.

#### Version and Platform Compatibility
- Windows PowerShell 5.1-only and PowerShell 7-only syntax, cmdlets, parameters, or .NET APIs used without matching the project's declared support range.
- Windows-only providers, registry access, drive letters, path separators, environment-variable casing, or executable names used on code paths intended for macOS or Linux.
- Native-command quoting or argument-passing assumptions that differ across Windows PowerShell, modern PowerShell, and target operating systems; inspect PowerShell 7.3+ `$PSNativeCommandArgumentPassing` modes and legacy-command exceptions.
- Text encoding assumptions that corrupt non-ASCII input or output across Windows PowerShell and PowerShell 7 defaults.
- Platform checks performed after an incompatible command or type has already been resolved or invoked.

#### Test Correctness
- Pester mocks targeting a command name or module scope different from the one used by the code under test, allowing the real command to run or making the assertion meaningless.
- Assertions that cannot fail for the reported regression, or tests that exercise setup output instead of the function result.
- Tests sharing global variables, imported module state, environment changes, files, jobs, or sessions without isolation and cleanup.
- Destructive or privileged tests reaching real resources without an effective safety boundary. `TestDrive` isolates only file operations directed into it; mocks must intercept the actual call, and `-WhatIf` only protects operations that honor `ShouldProcess`.
- Tests depending on uncontrolled network, credentials, locale, installed modules, execution policy, or host state without declaring and isolating that dependency.

#### Official References
- [Command evaluation](https://learn.microsoft.com/en-us/powershell/scripting/learn/deep-dives/avoid-using-invoke-expression), [native-command preferences](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_preference_variables), and [execution policy](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_execution_policies).
- [Advanced function lifecycle](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_functions_advanced_methods), [return/output semantics](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_return), and [array addition](https://learn.microsoft.com/en-us/powershell/scripting/dev-cross-plat/performance/script-authoring-considerations#array-addition).
- [ShouldProcess](https://learn.microsoft.com/en-us/powershell/scripting/learn/deep-dives/everything-about-shouldprocess), [SecureString](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/convertto-securestring), and [remoting serialization](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_remote_variables).
- [Module manifests](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_module_manifests), [safe data import](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.utility/import-powershelldatafile), and [character encoding](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_character_encoding).
- [Pester mocking](https://pester.dev/docs/usage/mocking) and [TestDrive isolation](https://pester.dev/docs/usage/testdrive); use the documentation matching the project's Pester version.
