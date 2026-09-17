# Starlark Code Review Guide

> Guidance for AI reviewers. Starlark is a deterministic, hermetic configuration
> language (a dialect of Python) used primarily by Bazel. It is **not** Python:
> the syntax is a strict subset, but the semantics differ, and many Python
> features are intentionally absent. Unless noted, this guide assumes Bazel's
> Starlark dialect.

## Obvious Typos or Spelling Errors

- Spelling errors in variable, function, provider, rule, attribute, and target
  names **at their declaration sites**; do not report spelling errors at call
  sites.
- Spelling errors in `fail()` or `print()` messages, comments, and docstrings
  that affect readability.
- Typos in `load()` symbol names and label strings (these can cause load or
  analysis failures).
- Misspelled attribute names in rule or macro calls (for example `sorce` instead
  of `srcs`), because they are usually a runtime error or are silently ignored.

## Determinism and Purity

Starlark is designed to be deterministic and side-effect free. Review code that
could break hermeticity.

**Key checks:**

- No reliance on wall-clock time, randomness, host filesystem state,
  environment variables, or network access in ordinary `.bzl` or `BUILD` code.
- Rule implementations must produce the same result for the same inputs and
  `ctx`.
- Repository rules may perform network or filesystem access, but their result
  must still be deterministic for a given set of inputs.
- No `print()` in production code (see Error Reporting).

## Recursion and Termination

**Key checks:**

- No direct or mutual recursion; Starlark rejects recursion at runtime.
- No `while` loops; Starlark has only `for` loops over finite sequences.
- Starlark `for` loops always terminate because they iterate over finite
  collections; `break` and `continue` cannot make a loop non-terminating, so
  flag excessive iteration cost instead (see Depsets and Efficiency).
- Avoid deeply nested comprehensions that make termination or cost hard to
  reason about.

## Frozen Values and Mutation

Starlark favors immutability. Only `list` and `dict` are mutable, and only
inside the evaluation context that created them. Bazel 8.1 also added a
mutable core `set` type, so `set` values are subject to the same rules.

**Key checks:**

- Do not mutate a value loaded from another `.bzl` file, a rule attribute, or a
  value returned by a rule or provider; those values are frozen.
- Do not mutate a collection while iterating over it (a dynamic error).
- Avoid mutable default arguments; defaults are evaluated once and shared
  across calls.
- Do not rely on object identity; Starlark has no `is` operator.

**Example:**

```python
# Bad: mutates a frozen value
# other.bzl
load(":defs.bzl", "registry")

def use():
    registry.append("x")  # runtime error: registry is frozen

# Good: the function owns the mutable value; loaded module globals are frozen
# after evaluation and cannot be mutated from another file
def register(name):
    registry = []
    registry.append(name)
    return registry
```

```python
# Bad: mutable default argument is shared between calls
def add(name, names=[]):
    names.append(name)
    return names

# Good: use None and create the list inside
def add(name, names=None):
    names = names or []
    names.append(name)
    return names
```

## Safe Data Handling (Python Differences)

**Key checks:**

- Ordered comparisons (`<`, `<=`, `>`, `>=`) are defined only within a single
  value type; `==` and `!=` may compare across types.
- Strings are not iterable; use `s.elems()` to iterate over characters.
- Dictionary literals cannot contain duplicate keys.
- Dictionary iteration order is deterministic.
- There is no implicit string concatenation; use `+`.
- In Bazel, `int` is limited to 32-bit signed values and overflow is an error.
- Use `struct()` instead of `class`; use `provider()` for rule outputs.
- Define named functions with `def`; Bazel's dialect has no `lambda`.
- There is no `import`; use `load()`, placed at the top of the file.
- There is no `global` or `nonlocal`.

**Example:**

```python
# Bad: Python-style exception handling does not exist
def lookup(data, key):
    try:
        return data[key]
    except KeyError:
        return None

# Good: check first and fail with a clear message
def lookup(data, key):
    if key not in data:
        fail("missing key %r in %r" % (key, data))
    return data[key]
```

## Depsets and Efficiency

**Key checks:**

- Aggregate transitive information with `depset`; do not flatten transitive
  depsets with `to_list()` inside a loop, which is O(N^2) over the dependency
  graph.
- Avoid `depset.to_list()` except for debugging.
- Build a depset in a single call; do not construct depsets inside a loop.
- Use `ctx.actions.args()` for command lines instead of flattening depsets or
  building giant strings.
- Pass depsets directly as action inputs; do not convert them to lists.

**Example:**

```python
# Bad: repeatedly flattening depsets with to_list() is O(N^2)
files = []
for dep in ctx.attr.deps:
    files += dep[MyProvider].files.to_list()

# Good: keep the depsets nested and merge once
files = depset(
    direct = ctx.files.srcs,
    transitive = [dep[MyProvider].files for dep in ctx.attr.deps],
)
```

## Error Reporting

**Key checks:**

- Use `fail()` to report validation errors and unrecoverable conditions.
- `print()` is for debugging only; it must not appear in production code, or it
  must be gated behind a hardcoded `DEBUG` flag.
- Error messages should name the offending value and the expected condition.
- Do not silently swallow errors, and do not return `None` where the caller
  expects a value without documenting that behavior.

## Loads, Symbols, and Encapsulation

**Key checks:**

- Use `load()` (not `import`), with all loads at the top of the file.
- Minimize exported symbols per `.bzl` file; mark file-private top-level
  symbols with a leading `_`.
- Remove unused loads, variables, and parameters.
- Prefer rules over macros; macros expand before graph analysis and hide the
  real build graph.
- Flag macro-generated helper targets that are not prefixed with the main
  target's `name` (for example `name_bar` or `_name_bar`).
- Flag helper targets that lack `visibility = ["//visibility:private"]` or a
  `manual` tag; without `manual`, wildcard patterns such as `:all` and `:...`
  expand them.
- Keep `BUILD` files simple and explicit (DAMP over DRY); avoid top-level list
  comprehensions and shared `deps` variables.

## Naming Conventions

**Requirements:**

- Variables, functions, rules, attributes, and targets use `snake_case`.
- Constants use `UPPER_SNAKE_CASE`.
- File-private top-level symbols start with `_`.
- Rule and macro implementation functions are named `_<name>_impl`.
- Rule names are nouns describing the produced artifact; common suffixes
  include `_library`, `_binary`, `_test`, and `_import`.
- Common attributes follow standard names and types: `srcs` is a `label_list`
  (typically source files), `deps` is a `label_list` (typically *not* files),
  `data` is a `label_list` (runtime/test data), and `runtime_deps` is a
  `label_list` for dependencies not needed at compile time.
- A macro's `name` parameter is first, and generated targets are prefixed with
  `name` plus `_`.
- Call rules and macros with keyword arguments, spaces around `=`, and `name`
  first.

## Documentation

- Add a docstring at the top of each `.bzl` file and for each public function.
- Document rules, aspects, providers, and attributes using the `doc` argument.
- Rules pass data through well-defined providers; flag provider fields that
  are undeclared or undocumented.
- Use comments to explain *why*, not what the code does.

## Formatting

- Formatting must match `buildifier`; do not nitpick formatting it would
  produce.
- Prefer double quotes for strings.
- Use four-space indentation.
- Use a single blank line between top-level definitions.

## Do Not Report

- Code that already avoids Python-isms correctly, such as using `==` instead of
  `is`, or `fail()` instead of `try`/`except`.
- Formatting that `buildifier` is responsible for.
- Spelling errors at call sites rather than declaration sites.
- The absence of features Starlark intentionally omits (`class`, `while`,
  `import`, recursion, and so on).
