> Favor precision over recall: only raise an issue when the rendered consequence or trust boundary is clear. EJS, Liquid and Nunjucks all embed a host language and compile to HTML, but their escaping contracts differ: EJS `<%= %>` and Nunjucks `{{ }}` HTML-escape by default, while Liquid emits output raw unless an `escape` filter is applied. Distinguish each engine's built-in escaping from the validation or serialization required by the actual output context.

#### Escaping and Output Contexts
- EJS unescaped output (`<%- %>`) or Nunjucks raw output (`| safe`) that can receive untrusted HTML without prior, context-appropriate sanitization
- Liquid `{{ value }}` emitting untrusted text into an HTML context without the `escape` filter; Liquid does not HTML-escape output automatically, so an explicit escape filter is the only HTML-safe form
- EJS escaped output (`<%= %>`) or Nunjucks `{{ value }}` used inside JavaScript, CSS, JSON, URL, or event-handler syntax as though HTML escaping protected those grammars
- Do not report ordinary EJS `<%= %>` or Nunjucks `{{ value }}` in an HTML text node solely for lacking an explicit escape helper; both HTML-escape those forms by default. Liquid is the exception — it has no default escaping, so report the missing `escape` filter rather than treating it as a false positive
- Sanitized HTML rendered unescaped after transformations that invalidate the sanitizer's guarantee, such as concatenating new untrusted markup afterward
- Nunjucks `autoescape` turned off, or an environment registered without autoescaping enabled, without a documented reason scoped to trusted input only

#### Attributes, URLs, and Dynamic Markup
- Attribute values from untrusted data that are HTML-escaped but not validated for the sink, especially `href`, `src`, `action`, `formaction`, `srcdoc`, `style`, and event-handler attributes
- EJS attribute construction by string concatenation, or object spreads of untrusted data that can introduce event handlers or override security-sensitive attributes; inspect attribute names and non-HTML sink semantics rather than conventional helper forwarding
- Liquid `{{ }}` used inside an attribute whose value is then interpreted as a URL by client code, without scheme validation — Liquid output is unescaped to begin with, and even in engines that do HTML-escape, `javascript:` and `data:` survive HTML escaping because escaping does not constrain URL schemes
- Boolean attributes whose values use strings such as `"false"` when presence still enables the HTML behavior, or conditional attributes that leave a control enabled, selected, or focusable on the wrong branch
- IDs, `name` values, fragment links, or ARIA references generated in loops without a stable uniqueness guarantee

#### Embedded Scripts and Server-to-Client Data
- Values interpolated into inline `<script>` blocks or event handlers without JavaScript-string or JSON-safe serialization; HTML escaping is not a substitute for JavaScript encoding
- JSON embedded in an inline script without neutralizing HTML parser terminators such as `</script>`, allowing data to end the script element or break parsing
- Secrets, session data, authorization material, internal-only fields, or unnecessarily large objects serialized into client-visible markup or scripts
- Client-side assumptions in a server-rendered template, including access to browser globals the server never has, or server-only state the browser never receives

#### Template Trust, Evaluation, and Side Effects
- EJS: attacker-controlled template source or attacker-writable template files passed to `render`/`renderFile`; EJS templates execute embedded JavaScript, so treating untrusted templates as presentation data creates a server-side template injection and code-execution boundary
- EJS: request-controlled template selection without a strict name allowlist and template-root boundary, allowing unintended files to be rendered
- EJS: request, query, or body objects spread wholesale into render options or locals, allowing untrusted keys to alter `filename`, `root`, or locals; keep render options fixed and expose an explicit locals shape
- Liquid: template source or partial names selected from untrusted input; Liquid is deliberately restricted, but an attacker-controlled template name still reaches the filesystem and can expose unintended content
- Nunjucks: templates loaded from an untrusted or user-writable path, or a loader configured to read outside the intended template root
- Embedded host code that performs database, network, filesystem, or shared-state mutation during rendering rather than in the controller or view-model layer

#### Missing Data, Partials, and Layouts
- Required locals that can become empty output, omitted attributes, invalid identifiers, or broken URLs without an explicit default or branch; do not require defaults for optional display-only values
- Exceptions from property access or method calls on absent locals that can abort the whole render, especially inside shared layouts or error pages
- EJS `include`/partial paths that resolve against a different root than intended, or locals passed to a partial that omit a value it reads
- Liquid `include`/`render` with an object whose shape is not fixed, or an overly broad locals hash that leaks fields across component boundaries
- Nunjucks `include`/`extends`/`import` whose relative path resolves differently than the author intended, or `{% import %` pulling in a macro file from outside the template root
- Layout or wrapper overrides that silently drop required metadata, scripts, security controls, fallback content, or accessibility structure

#### Control Flow, Filters, and Macros
- Liquid filters treated as runtime sanitizers or as validators for a non-HTML sink; Liquid filters transform display text and do not make a value safe for a URL, attribute, or script context — and only `escape` (not a filter in general) makes a value safe for HTML
- Nunjucks custom filters registered without `autoescape` awareness, so a filter returning untrusted markup bypasses the engine's escaping
- Nunjucks macros that emit unescaped markup (`| safe`) for values the caller controls, or that build attributes by concatenation
- Loops or conditionals whose tag boundaries are misplaced, so a block that appears nested is emitted as a sibling (and vice versa) — particularly around EJS `<% } %>`, Nunjucks `{% endif %}`, or Liquid `{% endif %}` pairs split across partials
- `while` or recursive constructs with a reachable non-terminating path, or expensive expressions and function calls repeated inside large loops
- Business logic, authorization decisions, or non-trivial data shaping implemented in the template instead of the controller/view-model layer

#### Accessibility
- Interactive non-button elements without equivalent keyboard activation, focusability, and semantics when a native element would provide them
- Inputs generated without an associated label, repeated form-control IDs, or labels/ARIA references that point to a different loop item
- Images with missing or misleading alternatives, and conditional content that changes visible state without updating accessible name, role, or `aria-*` state
- Layout or partial overrides that silently remove landmarks, heading structure, fallback text, focus management, or required attributes supplied by the parent

#### Performance and Review Scope
- Templates compiled or re-read on every request when they are static and the host can cache them with a stable key
- Partials rendered per item inside a hot loop when the result is identical across iterations and can be hoisted or cached
- Expensive helpers, filters, serialization, or data reshaping repeated per item when the result can be prepared once by the host application
- Report performance findings only when request frequency or collection size makes the cost evident; do not turn whitespace or formatting preferences into blocking findings
