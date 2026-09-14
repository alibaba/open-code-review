> Favor precision over recall: only raise an issue when the defect is observable in the template and the relevant data or trust boundary is clear. Handlebars and Mustache are implemented by many runtimes, so do not assume optional features or host configuration without evidence.

#### Handlebars/Mustache Escaping Boundaries
- Raw interpolations (`{{{value}}}` or `{{& value}}`) that can receive untrusted content without prior context-appropriate sanitization
- Double-braced values placed in JavaScript, CSS, URL, or event-handler contexts as though HTML escaping protected those grammars
- Interpolated values in unquoted HTML attributes, where escaping alone does not establish a safe attribute boundary
- Values inserted into URL-bearing attributes without validating the allowed scheme and destination
- Do not report ordinary `{{value}}` output in an HTML text node solely for lacking an explicit escape helper; both Handlebars and Mustache escape that form by default

#### Template and Partial Selection
- Dynamic Handlebars partials such as `{{> (lookup . "partialName")}}`, or Mustache dynamic names such as `{{>*partialName}}`, selected from untrusted data without an explicit allowlist
- Partial or parent names assembled from request-controlled values when they can expose unintended registered templates
- Recursive partial chains with no data-dependent termination condition
- Mustache lambdas or Handlebars helpers used to reinterpret data as template source when the input can be attacker-controlled

#### Missing Values and Helper Resolution
- Required values that can silently render as an empty string, especially inside identifiers, URLs, form names, generated configuration, or security-sensitive attributes
- Misspelled paths, helpers, or partial names when the surrounding template establishes the intended name and the mismatch would render empty output or fail at runtime
- Handlebars helper/data collisions where `{{name}}` invokes a helper but the template intends the current context property; use `{{./name}}` or `{{this.name}}` when disambiguation is required
- Unquoted helper arguments intended as literal strings, causing Handlebars to resolve them as paths instead
- Do not require a default for every optional display value; report concrete output or control-flow failures

#### Sections, Blocks, and Context
- Mismatched opening and closing section names, or conditionals that emit only one half of a required tag, delimiter, quote, or structured-output field
- References inside `each`, `with`, or Mustache sections that use the wrong context depth and therefore read from the current item instead of the parent or root
- Nested loops that use `@index`, `@key`, or `../` from the wrong level; prefer block parameters when they make the intended binding explicit
- Handlebars `if` checks where a valid numeric zero must be rendered but is treated as empty without `includeZero=true`
- Section and inverted-section pairs that can both be skipped or can duplicate fallback output because they test different paths

#### Partials and Inheritance
- Partials invoked with the current context when they require an explicit context or hash argument, leading to missing or shadowed values
- Sensitive caller data inherited by a partial that should receive a narrow explicit context
- Partial-block or Mustache inheritance overrides that omit required wrapper markup, accessibility attributes, or security metadata supplied by the parent
- Optional Mustache features such as lambdas, dynamic names, delimiter changes, or inheritance used when the project's target implementation does not support them

#### Structured Output and Whitespace
- HTML escaping used as a substitute for JSON, YAML, JavaScript, CSS, shell, or SQL serialization; require a format-specific serializer or helper when values can contain syntax characters
- Handlebars whitespace control (`~`) or Mustache standalone-line behavior that joins tokens, removes required separators, or changes indentation-sensitive output
- Partials inserted into indentation-sensitive output at a depth that produces invalid YAML, source code, or configuration

#### Performance and Side Effects
- Expensive helpers, lambdas, lookups, or partial selection repeated inside large sections when the result can be prepared once by the host application
- Helpers or lambdas with observable side effects invoked from branches that may render multiple times
