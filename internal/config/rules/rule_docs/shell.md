> Favor precision over recall: report only defects that can change behavior, lose data, or create a security boundary violation. Do not recommend stylistic rewrites or impose one shell dialect on another.

#### Shell Dialect and Execution Context
- Use the shebang and invocation context to identify the dialect before reporting syntax. Treat `#!/bin/sh` as POSIX shell even when `/bin/sh` happens to be Bash locally; Bash-only syntax such as arrays, `[[ ... ]]`, process substitution, `source`, and `set -o pipefail` is not portable there
- Treat a Bash shebang such as `#!/bin/bash` or `#!/usr/bin/env bash`, and files ending in `.bash`, as Bash. For `.sh` files without a shebang, do not report dialect-specific incompatibilities unless the caller or surrounding project establishes the interpreter
- Check that the shebang matches how the script is executed. A shebang has no effect when a caller explicitly runs `sh script.sh`

#### Expansion, Arguments, and Filenames
- Report unquoted parameter or command expansion when a value can contain whitespace, glob characters, or be empty and word splitting or pathname expansion would change the arguments. Do not flag expansions in contexts where splitting does not occur, including assignment values, arithmetic contexts, or Bash `[[ ... ]]`
- Preserve argument boundaries with `"$@"`; `$*`, unquoted `$@`, and storing argument lists in scalar strings can merge or split arguments. In Bash, use arrays when a command and its arguments must be assembled dynamically
- Do not claim that `[ "$value" = expected ]` is unsafe: quoting makes this a valid POSIX comparison. Recommend Bash `[[ ... ]]` only when the script is actually Bash and its pattern or regular-expression semantics are needed
- Check filename loops and command substitutions for newline, whitespace, glob, and leading-dash failures. Prefer null-delimited input where filenames may contain arbitrary bytes, and use `--` or an explicit path prefix when an operand could begin with `-`

#### Exit Status and Control Flow
- Check whether a failed `cd`, download, copy, deployment, or other prerequisite can let a destructive or externally visible command run in the wrong state. Handle the status at the point where continuing would be unsafe
- Do not require `set -e` universally. Its behavior is context-dependent; when it is used, check commands in conditions, pipelines, substitutions, and functions for cases where failures are ignored or cause an unintended exit
- In Bash, check pipeline status when an earlier stage can fail while the last command succeeds; use `pipefail` or explicit status handling when that failure matters. Do not recommend `pipefail` for POSIX sh
- Report state changes lost because a loop runs in a pipeline subshell when later code relies on those changes. Account for the target shell rather than assuming every pipeline has identical process semantics
- In Bash, a declaration command such as `local result=$(command)` can hide the substitution's status. Split declaration from assignment when the command's failure must be detected

#### Input, Injection, and Temporary Resources
- Treat command strings passed to `eval`, `sh -c`, or `bash -c` as injection sinks when any part can be influenced externally. Prefer direct argument passing and validate the rare cases that require generated shell code
- Use `read -r` when backslashes are data, and set `IFS` deliberately when leading or trailing whitespace must be preserved. Verify that EOF and a final line without a newline are handled when either can occur
- Report predictable temporary paths and unsafe creation in shared directories. Use `mktemp` or an equivalent atomic creation mechanism, restrictive permissions for sensitive data, and cleanup that also runs on failure or signals when required
- Check traps for quoting and timing mistakes: values expanded while installing a trap may differ from values at exit, and cleanup must not operate on an empty or unchecked path

#### Portability and External Commands
- For POSIX scripts, report Bash-only operators and features only when the executed path can reach them. For Bash scripts, check version-dependent features against any declared minimum Bash version
- Do not assume GNU and BSD variants accept the same flags or output formats. Report a portability problem only when the project targets an affected platform
- Check external-command output before using it as code, a path, or a numeric value, and distinguish an empty successful result from a failed command when the behavior depends on that difference
