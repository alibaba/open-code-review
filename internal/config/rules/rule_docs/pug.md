> Favor precision over recall: only raise an issue when the rendered consequence or trust boundary is clear. Pug evaluates JavaScript and compiles to HTML, so distinguish Pug's built-in HTML escaping from the additional validation or serialization required by the actual output context.

#### Pug Escaping and Output Contexts
- Unescaped string interpolation (`!{value}`) or unescaped buffered code (`!= value`) that can receive untrusted HTML without prior, context-appropriate sanitization
- Escaped interpolation (`#{value}`) or buffered code (`= value`) used inside JavaScript, CSS, JSON, URL, or event-handler syntax as though HTML escaping protected those grammars
- Do not report ordinary `#{value}` or `= value` in an HTML text node solely for lacking an explicit escape helper; Pug HTML-escapes those forms by default
- Sanitized HTML rendered through `!{}` or `!=` after transformations that invalidate the sanitizer's guarantee, such as concatenating new untrusted markup afterward

#### Attributes, URLs, and Dynamic Markup
- Attribute values from untrusted data that are HTML-escaped but not validated for the sink, especially `href`, `src`, `action`, `formaction`, `srcdoc`, `style`, and event-handler attributes
- Direct `&attributes(object)` spreads of untrusted objects: Pug does not automatically escape values in a general attribute spread, and attacker-controlled keys can introduce event handlers or override security-sensitive attributes
- Do not flag conventional mixin forwarding solely because it uses `&attributes(attributes)` or `!= attributes.name`; values in the implicit mixin `attributes` object are already escaped by Pug. Inspect the original caller, attribute names, and non-HTML sink semantics instead
- Boolean attributes whose values use strings such as `"false"` when presence still enables the HTML behavior, or conditional attributes that leave a control enabled, selected, or focusable on the wrong branch
- IDs, `name` values, fragment links, or ARIA references generated in loops without a stable uniqueness guarantee

#### Embedded JavaScript and Server-to-Client Data
- Values interpolated into `script.` blocks or inline handlers without JavaScript-string or JSON-safe serialization; HTML escaping is not a substitute for JavaScript encoding
- `!{JSON.stringify(data)}` embedded in an inline script without neutralizing HTML parser terminators such as `</script>` (and relevant line-separator characters for the supported JavaScript target), allowing data to end the script element or break parsing
- Secrets, session data, authorization material, internal-only fields, or unnecessarily large objects serialized into client-visible markup or scripts
- Client-compiled templates (`compileClient` / `compileFileClient`) that can render attacker-controlled raw output before the caller inserts the result with `innerHTML` or an equivalent HTML sink
- Server-only assumptions embedded in client-compiled Pug, including access to unavailable globals, modules, filesystem state, or locals the browser never receives

#### Template Trust, JavaScript, Missing Data, and Side Effects
- Attacker-controlled template source passed to `compile` / `render`, or attacker-writable files passed to `compileFile` / `renderFile`; Pug templates can execute embedded JavaScript, so treating untrusted templates as presentation data creates a server-side template injection and code-execution boundary
- Request-controlled template file selection without a strict name allowlist and template-root boundary, allowing unintended templates or paths to be rendered even though `include` and `extends` directives inside a trusted template are static
- Request, query, or body objects spread wholesale into Pug render/compiler options, allowing untrusted keys to alter options such as `filename`, `basedir`, `filters`, `globals`, `cache`, or `pretty` as well as locals; keep compiler options fixed, expose an explicit locals shape, and verify the project's Pug version before claiming exploitability
- Unbuffered code (`-` or an unbuffered code block) that performs database, network, filesystem, shared-state mutation, or other externally observable side effects during rendering
- Required locals that can become empty output, omitted attributes, invalid identifiers, or broken URLs without an explicit default or branch; do not require defaults for optional display-only values
- Exceptions from property access or function calls on absent locals that can abort the whole render, especially inside shared layouts or error pages
- Business logic, authorization decisions, or non-trivial data shaping implemented in the template instead of the controller/view-model layer

#### Includes, Inheritance, Mixins, and Filters
- Relative `include` or `extends` paths used without the `filename` context needed for correct resolution, or absolute paths that unintentionally depend on a different `basedir`
- Non-Pug includes that insert raw HTML, JavaScript, CSS, or secrets when the author appears to expect Pug parsing or escaping
- Child templates that replace, append, or prepend the wrong named block and thereby drop required metadata, scripts, security controls, fallback content, or accessibility structure
- Content or unbuffered code placed at the top level of an extending template even though child templates may only define named blocks and mixins there
- Mixins invoked with arguments in the wrong order, implicit caller context, or an overly broad attribute object, producing missing data or leaking fields across component boundaries
- Filters treated as runtime sanitizers or as processors for dynamic locals even though Pug filters run at compile time and cannot support dynamic content or options

#### Indentation, Control Flow, and Whitespace
- Indentation changes that move an element into the wrong parent, loop, conditional, or block, changing form ownership, DOM structure, visibility, or execution scope
- `while` loops with a reachable non-terminating path, or expensive expressions and function calls repeated inside large `each` loops
- `each ... else` or conditional branches that omit required empty/error states, duplicate IDs, or render controls without the data needed by their handlers
- Inline tags and text whose Pug whitespace rules concatenate words or tokens, or reliance on trailing spaces that formatters and editors can remove
- Literal HTML or plain-text blocks whose indentation looks like Pug nesting but is emitted mostly unprocessed, producing a different structure than intended

#### Accessibility
- Interactive non-button elements without equivalent keyboard activation, focusability, and semantics when a native element would provide them
- Inputs generated without an associated label, repeated form-control IDs, or labels/ARIA references that point to a different loop item
- Images with missing or misleading alternatives, and conditional content that changes visible state without updating accessible name, role, or `aria-*` state
- Inheritance or mixin overrides that silently remove landmarks, heading structure, fallback text, focus management, or required attributes supplied by the parent

#### Performance and Review Scope
- Templates compiled on every request or repeatedly inside a hot loop when they are static and the host can precompile or cache them with a stable `filename` key
- Expensive helpers, mixins, serialization, or data reshaping repeated per item when the result can be prepared once by the host application
- Report performance findings only when request frequency, collection size, or repeated compilation is evident; do not turn indentation preferences or equivalent shorthand forms into blocking findings
