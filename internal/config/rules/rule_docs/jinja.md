> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the host application's Jinja environment or data trust boundary is not visible.

#### Jinja Escaping Boundaries
- Untrusted values passed through `|safe`, wrapped in `Markup`, or rendered inside `{% autoescape false %}` without prior context-appropriate sanitization
- HTML-escaped values inserted into JavaScript, CSS, URL, or event-handler contexts as though HTML escaping protected those grammars
- `|tojson` output placed in a double-quoted HTML attribute; Jinja documents that this requires single-quoted attributes or additional escaping
- Interpolated values in unquoted HTML attributes, where escaping alone does not establish an attribute boundary
- Do not report every `{{ value }}` as unescaped: autoescaping is configured by the host environment and may be selected from the template filename

#### Template and Expression Injection
- User-controlled template source, expression fragments, filter names, or macro bodies evaluated as Jinja rather than passed as data
- Dynamic `{% include %}`, `{% import %}`, or `{% extends %}` targets derived from request data without an explicit allowlist
- Use of attribute traversal or the `attr` filter to reach sensitive host objects exposed to an untrusted template
- Treat `SandboxedEnvironment` as defense in depth, not permission to expose secrets or powerful application objects to attacker-authored templates

#### Undefined Values and Defaults
- Required values silently rendered as empty strings under the default `Undefined` behavior, especially in identifiers, URLs, authorization decisions, or generated configuration
- `|default(..., true)` used where valid falsey values such as `0`, `False`, or an empty collection must remain distinct from missing data
- Deep attribute chains whose intermediate value may be undefined, unless the environment clearly uses `ChainableUndefined` or a guard establishes the value
- Do not require `StrictUndefined` in every application; report the concrete silent-failure path instead

#### Includes, Imports, and Macros
- Macros depending on ambient variables even though imported templates do not receive the caller's context by default
- `{% include %}` unintentionally inheriting sensitive caller context when `without context` is required
- Macro calls where missing arguments become `Undefined`, positional arguments are misordered, or unintended extra arguments are captured through `varargs` or `kwargs`
- Child templates overriding a block but omitting `{{ super() }}` when the parent block contains required structure or security metadata

#### Output Correctness
- Whitespace-control markers (`{%-`, `-%}`, `{{-`, `-}}`) that concatenate tokens, lines, HTML attributes, YAML scalars, or generated source unexpectedly
- Conditions that emit only one half of a required delimiter, tag, quote, or structured-output field
- Loop metadata such as `loop.index` and `loop.index0` mixed in the same external identifier or pagination calculation
- Reusing a loop variable after the loop as though assignments inside loops escaped Jinja's loop scope

#### Performance and Side Effects
- Host functions, database-backed properties, or network lookups invoked repeatedly inside a loop when the result can be prepared once by the application
- Repeated sorting, grouping, filtering, or serialization of the same collection within one render
- Includes or imports selected per item in a large loop when a fixed macro or precomputed view model would avoid repeated loader work
