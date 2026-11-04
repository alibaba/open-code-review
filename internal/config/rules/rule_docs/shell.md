#### Shell Script Review Principles
> Favor precision over recall: report only defects that are demonstrable from the script itself. Establish the target shell from the shebang and the file extension before applying any rule -- a bashism in a `#!/bin/sh` script is a real portability defect, the same line in a `#!/usr/bin/env bash` script is not. `fish` is not POSIX and shares none of the quoting, `$@`, `[[ ]]`, or `set -e` semantics below; for `.fish` files apply only the shell-neutral sections (interpreter and strict mode intent, destructive operations, external input, temporary files, secrets, process handling) and say explicitly that POSIX quoting rules do not apply. ShellCheck findings that the script already suppresses with a justified `# shellcheck disable=` directive are not defects.

#### Obvious Typos or Spelling Errors
- Spelling errors in function names, variable names, or environment variable names at their definition site; do not report spelling errors at use sites, as these are determined by the definition
- Strings in log or error messages containing spelling errors that affect readability
- Do not report a name that differs from a conventional spelling only to satisfy a linter or an existing project convention

#### Interpreter and Strict Mode
- A script with no shebang, or a shebang that does not match the syntax used in the body; the kernel, not the user's shell, decides how the file is interpreted
- Bashisms in a script declared `#!/bin/sh`: `[[ ]]`, `local`, arrays, `+=`, `function`, process substitution, `source`, `$'...'`, or brace expansion. On Debian and Ubuntu `sh` is `dash`, so these fail at runtime rather than degrade
- Missing `set -e` / `set -u` / `set -o pipefail` in a script whose whole purpose is to abort on failure, such as a CI or deploy script. Do not demand them in scripts that deliberately continue after errors or that are sourced into an interactive shell
- `set -e` treated as total protection: it does not fire for a command in an `if`, `&&`, `||`, `!` context, or for a failure inside a pipeline without `pipefail`, and a function whose result is used as a condition is exempt for its whole body
- `set -u` missing while the script relies on `${VAR}` being defined, so a typo in a variable name silently expands to nothing instead of aborting
- `set -x` enabled unconditionally in a script that handles credentials, since the trace prints expanded argument values

#### Command Injection and Unquoted Expansions
- A variable expanded unquoted into a command position, an argument, or a `sh -c` / `eval` / `ssh` / `sudo` / `docker run` payload, where the value is attacker-influenced (CWE-78, CWE-88); the caller controls word splitting and globbing, so `--file=$path` with `path='x --exec=y'` becomes two arguments
- `eval`, or `sh -c` / `bash -c` on a string assembled from variables, where the constructed text is not a fixed literal
- Unquoted command substitution such as `` for f in $(ls) `` or `` grep $pattern $file ``; the result is word-split and glob-expanded, and a filename containing whitespace or `*` changes the command
- `$*` where `"$@"` is meant: `$*` joins the arguments with the first character of `IFS`, collapsing the argument boundaries the callee depends on
- Backticks instead of `$(...)` in a context where the nesting or escaping is ambiguous; report only when the substitution is nested or the inner text contains a backslash, since this is otherwise style
- `read` without `-r`, which consumes backslashes in the input line
- `IFS` reassigned without restoring it, or reassigned to a value that re-enables word splitting on data

#### Destructive and Irreversible Operations
- `rm -rf` on a variable, glob, or path that can be empty, unset, or `/` at runtime: `rm -rf "$dir"` with `dir` empty deletes nothing while `dir=/` deletes the machine, and `rm -rf $dir` unquoted lets word splitting and globbing turn one value into several paths the author never named (CWE-73)
- A path passed to `rm`, `mv`, `cp`, `chmod`, `chown`, `>` or `truncate` without `--` in front of it, so a value beginning with `-` is read as an option
- A `cd` whose failure is not checked before a destructive command runs, so the command executes in whatever directory the shell happened to start in; `cd "$dir" || exit 1` on its own line, or `cd "$dir" && ...`, is the fix, and a bare `cd` followed later by a destructive line is the defect
- A glob used for deletion or overwrite that can expand to nothing or to the current directory, such as an unquoted `rm -rf "$dir"/*`, or `rm -rf "$dir"/` with a trailing slash, where an empty expansion removes the wrong set of entries
- `find ... -delete` or `find ... -exec rm` whose `-path` / `-name` predicate is missing, is broader than intended, or does not anchor the search root
- Truncating or overwriting a file with `>` where the same file is a source of the command, so the shell empties it before the reader runs

#### External Input and Downloads
- `curl` or `wget` piped directly into `sh` / `bash` / `python`, so whatever the remote host returns today executes with the script's privileges; the fix is to download, verify, then run
- A download whose integrity is never checked against a pinned checksum or signature, especially in an install, bootstrap, or CI script
- `curl -k` / `--insecure`, `wget --no-check-certificate`, or an `http://` URL for anything other than a deliberately local endpoint
- Remote content or a URL taken from an environment variable, a command-line argument, or an API response and used as the source of a download or the target of a redirect without an allowlist
- Extraction of an untrusted archive without bounding the output path or size, since archive entries can contain absolute paths or `../` segments

#### Temporary Files and Race Conditions
- A predictable temporary path built by hand, such as `/tmp/$name.$$` or `/tmp/out.txt`, in a script that more than one user or process can run; use `mktemp` (CWE-377)
- A check on a path followed by an action on it, where the path is in a shared directory an attacker can write to (CWE-367)
- A temporary file, directory, mount, lock, or background process with no `trap ... EXIT` cleanup, so an early `return`, `exit`, or signal leaks it
- `umask` never set in a script that creates files containing credentials or private keys

#### Paths, Files, and Traversal
- A path assembled by concatenating a user-supplied or environment-supplied base with a relative name and then read, written, or executed, without canonicalizing it first (CWE-22)
- Following a symlink where the script intended to operate on the link itself, or the reverse; `-L` / `-P` and `readlink -f` change which file is touched
- A script that trusts the current working directory or `$PWD` as an anchor without resolving it, so running it from a different directory silently targets the wrong tree
- `cp` / `mv` / `tar` on a path derived from input where a trailing slash or a missing `--` changes the meaning of the operation

#### Error Handling and Exit Codes
- A failing command whose status is discarded, then a later step that depends on its effect
- `$?` read after an intervening command, so the status belongs to the wrong command
- A pipeline whose first command fails while the last one succeeds, in a script with no `pipefail`
- `exit 0` or an unconditional success message on a path that can follow a failure
- `trap` handlers that swallow the signal without cleaning up, or that `exit 0` and so report success for a run that was interrupted
- An error message printed to stdout rather than stderr, or a message that omits the failing value, in a script whose output is consumed by another program

#### Secrets and Credentials
- A password, token, API key, private key, or connection string with an embedded credential written literally into the script (CWE-798)
- A secret passed as a command-line argument, since the full command line is readable by other processes on the host through `ps` and `/proc`
- A secret interpolated into a log line, an error message, or an echoed progress message
- A secret written to a file, a cache, or a CI artifact without restricted permissions
- A `curl -u user:password` or `git clone https://user:token@host/...` form that leaves the credential in shell history, process listings, and remote logs

#### Portability
- GNU-only flags in a script that claims to be portable: `sed -i` without a suffix argument, `readlink -f`, `grep -P`, `date -d`, `stat -c`, `xargs -r`, `head -c`; these fail or behave differently on BSD and macOS
- `echo -e` or `echo -n` for escape and newline handling, where `printf` is the portable form
- `local` in a `#!/bin/sh` script, or a function definition syntax the declared shell does not accept
- A dependency on a tool or version the script never checks for, where a missing tool turns into a confusing downstream failure rather than a clear message

#### Process, Signals, and Concurrency
- Background jobs started without recording their PIDs, or without a `wait` on the paths that matter, so a failure in the background is never observed
- A lock implemented with a plain existence check on a file instead of `flock` or an atomic create, which two concurrent runs can both pass
- A PID file trusted without verifying that the process is still the one that wrote it
- `kill` on a bare PID, or `kill -9` as the first attempt, where a graceful termination and a `wait` are what the script needs
- Two processes writing the same output file or appending to the same log without coordination

#### CI/CD and Container Contexts
- `sudo` inside a script that is documented to run unprivileged, or a step that assumes root without checking
- An unpinned tool, image tag, or `latest` reference in a script whose output other steps depend on being reproducible
- `docker run --privileged`, a `/var/run/docker.sock` mount, or a host-path mount that is broader than the step needs
- A CI step whose failure is masked by a trailing `|| true`, a piped `tee`, or a subshell whose exit status is dropped
- Credentials or tokens echoed into CI logs, including via `set -x` or a debug branch that runs on failure

#### Testing and Linting
- A shell script in a repository with a ShellCheck configuration that carries a new violation, where the script is not excluded and the suppression has no reason comment
- A test script that asserts nothing about the behavior it names, or that only checks that a command did not exit non-zero
- A test whose fixture directory or environment is not isolated per run, so a leftover temporary file makes the result order-dependent
