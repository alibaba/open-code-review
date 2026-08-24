> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat correctness, data-integrity, and unsafe-dynamic-code findings as blocking; treat naming, comment, and idiom suggestions as non-blocking. Review only the lines changed in this diff. MATLAB resolves most names at run time — do not infer the behavior of functions, classes, or validators defined outside the file under review.

#### Obvious Typos or Spelling Errors

- Spelling errors in function names, local function names, variable names, struct field names, or `arguments` block parameter names at their declaration sites; do not report spelling errors at reference sites, as these are determined by the declaration
- Typos in `error`/`warning`/`assert` message text, error identifiers, `fprintf`/`disp` log output, or the function description header that affect readability

#### File and Function Structure

- In a new file, or when this diff renames the leading function: a leading function name that does not match the file name
- Functions longer than roughly 200 lines that could be decomposed into local functions; report as non-blocking unless the length actively obscures a defect
- Nested functions used where a local function would do; nested functions share the parent workspace and should be reserved for cases that genuinely require shared access
- A helper called from exactly one parent function and placed in its own file instead of as a local function below the parent
- In a function added or substantially rewritten by this diff: missing `%%` section markers, or section markers without a description of what the section does, once the function is long enough to need structure
- Do not report file length alone, and do not report structure findings on files that were only touched incidentally

#### Argument Validation and Input Contracts

- In a new function, when the surrounding file already validates inputs and outputs via `arguments (Input)`/`arguments (Output)` blocks elsewhere: a function missing one of the two. Do not report this on an existing function being edited for an unrelated reason, or in a codebase that does not otherwise use this pattern
- An `arguments` block placed after executable code rather than immediately following the function description header
- Parameters declared with neither a size, a class, nor a validator function — an empty declaration validates nothing
- Missing size specification where the shape is known (`(:,1)`, `(1,1)`, `(:,:)`); missing class specification where the type is known (`double`, `logical`, `string`, `struct`)
- Class validation coerces rather than rejects when a conversion exists: a parameter declared `double` silently converts `logical`, integer, and `char` inputs (`'a'` becomes `97`). Where the caller must not be silently converted, add `mustBeA` or an equivalent validator
- Optional arguments handled via `nargin` branching, `exist("var","var")`, or `isempty` checks where a default value in the input `arguments` block would express the same contract declaratively
- A default value assigned in an `arguments (Output)` block; output blocks do not support defaults
- Do not report a missing validator when the size and class declarations already constrain the input adequately

#### Naming Conventions

- `i` or `j` used as a loop counter or any other variable; both are built-in functions for the imaginary unit, and shadowing them silently changes complex arithmetic elsewhere in the function
- Any built-in shadowed by a variable name — `length`, `size`, `sum`, `max`, `min`, `error`, `table`, `str`, `time`, `power`, `line` are the common offenders. Treat as blocking when the shadowed built-in is called later in the same scope
- Function or variable names not in `lowerCamelCase`; single-letter or cryptic names where a descriptive name is possible; abbreviations that are not established domain terms
- Logical variables not prefixed with `is`
- A variable reused within one function for a second purpose, or reassigned to a different class or array shape; this costs both readability and run time
- Do not report abbreviations that are standard in the domain and do not report established naming in surrounding untouched code

#### Comments and Documentation

- A function without a description header, or a header that restates the function name without saying what the function does, what it returns, and what the caller must guarantee
- Input or output variables described neither in the header nor as a trailing comment in the `arguments` blocks
- Non-obvious logic — index arithmetic, sign conventions, unit conversions, matrix assembly — left uncommented
- A comment that contradicts the code beside it; this is a correctness signal, not a style one, since one of the two is wrong
- Lines longer than 120 characters, or long expressions not broken across lines with `...` at logical boundaries
- Do not request comments on self-explanatory single-purpose lines, and do not report comment density in the abstract

#### Dead Code and Diff Hygiene

- Code that can never execute: statements after `return`, `error`, `break`, or `continue`; branches whose condition is a constant; `if false` blocks
- Variables assigned but never read, outputs computed but never returned, and input parameters never used — unless the signature is fixed by a callback or interface contract
- Large commented-out blocks with no note explaining why they are being preserved
- Whitespace-only or reindentation-only changes to lines the author did not otherwise modify; these create avoidable merge conflicts
- Legacy code rewritten purely to conform to the styleguide, with no functional change in the same hunk
- `%#ok<...>` suppressions of Code Analyzer warnings without an adjacent comment explaining why the warning is being ignored

#### Indexing, Shapes, and Implicit Expansion

- `for k = v` where `v` is a vector variable: the loop iterates over the *columns* of `v`, so a column vector yields exactly one iteration with the whole vector bound to `k`. Use `for k = 1:numel(v)` or transpose explicitly
- Reduction functions called without an explicit dimension (`sum(A)`, `max(A)`, `any(A)`, `mean(A)`) where `A` may be a single row at run time; MATLAB switches to row-wise behavior for row vectors. Pass the dimension: `sum(A,1)`
- Binary operations on operands with mismatched dimensions that silently broadcast under implicit expansion instead of erroring — for example `A + b` where `b` was intended to be conformable but is a row vector
- `&` or `|` inside an `if` condition with array operands, where `&&`/`||` with a scalar condition was intended; `if` requires *all* elements true, so this fails silently on mixed arrays
- `==` used to compare arrays of possibly different sizes in a condition; use `isequal`
- Indexing, `max`/`min`, or `x(1)`/`x(end)` on a container that can legitimately be empty upstream
- Repeated `find` calls where logical indexing would express the same filter, especially where several filters are combined
- Do not report shape assumptions that an `arguments` block size specification has already guaranteed

#### Numeric Correctness

- Floating-point values compared with `==` or `~=`, particularly convergence checks and tolerance comparisons; use an explicit tolerance or `ismembertol`
- `NaN` handling assumed rather than checked: `NaN == NaN` is false, `sum` propagates `NaN` while `max`/`min` skip it by default, and `isnan` is the only reliable test
- Integer-class arithmetic treated as C-like: MATLAB integer division *rounds to nearest* (`int32(5)/int32(2)` is `3`) and overflow *saturates* at `intmax` rather than wrapping. Use `idivide` with an explicit rounding mode where truncation is intended
- `single` and `double` mixed in one expression; the result silently degrades to `single`
- `'` (complex-conjugate transpose) used where `.'` (plain transpose) was intended on complex data such as phasors, impedance matrices, or admittance matrices — a defect that is invisible on real-valued test data
- Matrix operators used where element-wise was intended, or the reverse: `*` vs `.*`, `/` vs `./`, `^` vs `.^`
- `inv(A)*b` instead of `A\b`; the explicit inverse is slower and less accurate. `inv()` applied to a sparse matrix additionally destroys sparsity and can exhaust memory on network-sized systems
- Division by a quantity that can legitimately be zero (an out-of-service branch, a zero base value, an empty aggregate) without a guard
- Do not flag numerical style where the surrounding code documents a deliberate choice

#### Error Handling, Assertions, and Logging

- A condition that will inevitably lead to a downstream failure left unchecked; assert explicitly at the point where the assumption is made
- `assert` called without an error identifier, or with an identifier that does not follow `function_name:ErrorCondition`
- `try` blocks with an empty `catch`, a `catch` that only rethrows without context, or a `catch` that omits `disp(getReport(ME))`
- `try` blocks wrapping substantially more code than the one call that can actually fail, obscuring where the error originates
- A `catch` that swallows an error and continues with a partially computed result, so the caller sees plausible but wrong output
- Log output that reports nothing actionable, or that omits the project's expected context (timestamp, function name); a long-running function that produces no summary output at all on completion
- Do not report missing logging in small pure helper functions

#### State, Scope, and Lifetime

- Any use of `global`; the only tolerable exception is a logical feature flag such as a debug level, and even then it should be questioned in review
- `persistent` variables without a documented reset path; a stale cache surviving into the next calculation is a silent-wrong-answer defect
- `clear all`, `clear classes`, `close all`, or `clc` inside a function
- `warning("off", ...)` set without restoring the previous state, leaving warnings suppressed for the rest of the session; capture and restore the state.
- `assignin`, `evalin`, or `inputname` reaching into a caller's workspace
- Runtime `cd`, `addpath`, or `rmpath`
- Runtime introspection in a hot path: `exist`, `which`, `whos`, `dbstack`

#### Data Types and Containers

- `char` used for text where `string` would work; `char` breaks on ragged concatenation (`['asdf';'asd']` errors) and lacks `+` concatenation
- `cell` arrays used for homogeneous or tabular data where a `table`, a numeric matrix, or a struct array fits; each cell carries roughly 120 bytes of overhead
- `cell2mat` where `vertcat(c{:,1})` or `horzcat` would do the same job far more cheaply
- `struct("field", someCell)` — a cell value argument creates a *struct array*, not a struct holding a cell
- Dynamic field names built from data (`s.(name)`) where a `table`, `dictionary`, or `containers.Map` would express the lookup, and where a malformed name would error at run time
- `unique`, `sort`, or `setdiff` applied where the original row order matters, without `"stable"`
- Do not report container choice in code that is demonstrably not on a hot path and is already clear

#### Performance and Preallocation

Confirm the code is on a hot path and that the data scale justifies the finding before flagging:

- Arrays grown inside a loop (`x(end+1) = ...`, `x = [x; new]`, `s(end+1).f = ...`) where the final size is known or boundable; preallocate instead
- Preallocation that no longer matches the final size after subsequent edits — an oversized preallocation leaves trailing zeros that silently enter the result
- Loop-invariant work inside a loop: repeated `ismember` against the same set, repeated struct field lookups, repeated table indexing, repeated file access
- Small element-wise loops that vectorize cleanly
- A variable that changes class or shape mid-function instead of a new variable being introduced
- `parfor` used before the serial version has been profiled; `parfor` without a `numThreads` argument, which makes debugging inside called functions impossible
- Loop-carried dependencies, order-dependent output, or shared mutable state inside `parfor`; results that depend on iteration order are a correctness defect, not a performance note
- Random number generation inside `parfor` without an explicit reproducible stream, where results must be repeatable
- Do not raise micro-optimizations; clear code that is slower is explicitly preferred to fast code that is hard to follow

#### File and Data I/O

- `load` or `save` without an explicit variable list; an unrestricted `load` can silently overwrite existing workspace variables
- `load` called without capturing the output struct inside a function
- `exist("name","file")` or `exist("name","dir")` instead of `isfile` / `isfolder`
- `xlsread` or `xlswrite` in new code; use `readtable`/`writetable`, `readmatrix`/`writematrix`, or `readcell`/`writecell`
- `fopen` without a guaranteed `fclose` on every exit path, including the error path; prefer `onCleanup`
- Paths assembled by string concatenation with hard-coded separators instead of `fullfile`; hard-coded absolute paths or drive letters

#### Unsafe Dynamic Code

- `eval`, `evalc`, or `feval` on a string assembled from data, file contents, or user input; this is arbitrary code execution
- `str2num` on any externally sourced value — it evaluates its argument; use `str2double`
- `system`, `dos`, or `unix` invoked with a command string built from unvalidated input
- File paths taken from external data and used without validation, allowing traversal outside the intended directory
- Credentials, tokens, or connection strings hard-coded in source or written to the log

#### Compatibility and Code Analyzer

- Toolbox-dependent functions used in code that is expected to run without that toolbox license
- Remaining Code Analyzer warnings in the changed lines
- Do not report a compatibility concern without naming the introducing release — an unverified claim here is worse than silence
