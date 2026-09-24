> Favor precision over recall: only raise an issue when the rendered consequence or trust boundary is clear. ERB itself only substitutes embedded Ruby — HTML escaping is supplied by the host (Rails escapes `<%= %>` by default, bare ERB does not escape at all) — so distinguish the escaping the host actually applies from the validation or serialization required by the actual output context.

#### ERB Escaping and Output Contexts
- Unescaped output — `html_safe` / `raw` strings, or `<%= %>` reaching markup after escaping has been bypassed — that can receive untrusted HTML without prior, context-appropriate sanitization
- Escaped output (`<%= %>`) used inside JavaScript, CSS, JSON, URL, or event-handler syntax as though HTML escaping protected those grammars
- Do not report ordinary `<%= value %>` in an HTML text node solely for lacking an explicit escape helper; Rails ERB HTML-escapes that form by default
- Sanitized HTML rendered unescaped after transformations that invalidate the sanitizer's guarantee, such as concatenating new untrusted markup afterward

#### Attributes, URLs, and Dynamic Markup
- Attribute values from untrusted data that are HTML-escaped but not validated for the sink, especially `href`, `src`, `action`, `formaction`, `srcdoc`, `style`, and event-handler attributes
- Hash or keyword-argument attribute spreads of untrusted data that can introduce event handlers or override security-sensitive attributes; inspect attribute names and non-HTML sink semantics rather than conventional helper forwarding
- Boolean attributes whose values use strings such as `"false"` when presence still enables the HTML behavior, or conditional attributes that leave a control enabled, selected, or focusable on the wrong branch
- IDs, `name` values, fragment links, or ARIA references generated in loops without a stable uniqueness guarantee

#### Embedded Ruby and Server-to-Client Data
- Values interpolated into inline `<script>` blocks or event handlers without JavaScript-string or JSON-safe serialization; HTML escaping is not a substitute for JavaScript encoding
- JSON embedded in an inline script without neutralizing HTML parser terminators such as `</script>`, allowing data to end the script element or break parsing
- Secrets, session data, authorization material, internal-only fields, or unnecessarily large objects serialized into client-visible markup or scripts
- Values passed to `javascript_tag`, `content_security_policy`-relevant inline blocks, or `data-*` attributes consumed by client code without checking what the client does with them

#### Template Trust, Ruby Evaluation, and Side Effects
- Attacker-controlled template source or attacker-writable template files passed to the renderer; ERB templates execute embedded Ruby, so treating untrusted templates as presentation data creates a server-side template injection and code-execution boundary
- Request-controlled template or partial selection without a strict name allowlist and template-root boundary, allowing unintended files to be rendered
- Request, query, or body objects spread wholesale into render options or locals, allowing untrusted keys to alter layout, format, or handler selection; keep render options fixed and expose an explicit locals shape
- Embedded Ruby that performs database, network, filesystem, or shared-state mutation during rendering rather than in the controller or view-model layer
- Required locals or instance variables that can become empty output, omitted attributes, invalid identifiers, or broken URLs without an explicit default or branch; do not require defaults for optional display-only values
- Exceptions from property access or method calls on absent locals that can abort the whole render, especially inside shared layouts or error pages

#### Partials, Layouts, and Ruby Control Flow
- Relative `render` / `partial` paths that resolve against a different root than the author intended, or absolute paths that unintentionally depend on a different template root
- Locals passed to a partial that omit a value the partial reads, or an overly broad `locals` hash that leaks fields across component boundaries
- Layout or wrapper overrides that silently drop required metadata, scripts, security controls, fallback content, or accessibility structure
- Control flow whose ERB tag boundaries are misplaced, so a block that appears nested is emitted as a sibling (and vice versa) — particularly around `<% if %>` / `<% end %>` pairs split across partials
- `while` or recursive constructs with a reachable non-terminating path, or expensive expressions and method calls repeated inside large loops

#### Whitespace and Output Structure
- Whitespace-sensitive output where ERB tags emit stray newlines that concatenate words or tokens, or where `<%-` / `-%>` trimming changes which text nodes are produced. `<%-` and `-%>` trim surrounding whitespace only; they are not an unescaped-output form — output is always produced by `<%= %>`, and escaping is a property of the value, not of the tag's dash
- Literal HTML or plain-text blocks whose indentation looks like control-flow nesting but is emitted mostly unprocessed, producing a different structure than intended
- Conditional branches that omit required empty or error states, or that render controls without the data their handlers need

#### Accessibility
- Interactive non-button elements without equivalent keyboard activation, focusability, and semantics when a native element would provide them
- Inputs generated without an associated label, repeated form-control IDs, or labels/ARIA references that point to a different loop item
- Images with missing or misleading alternatives, and conditional content that changes visible state without updating accessible name, role, or `aria-*` state
- Layout or partial overrides that silently remove landmarks, heading structure, fallback text, focus management, or required attributes supplied by the parent

#### Performance and Review Scope
- Partials rendered per item inside a hot loop when the result is identical across iterations and can be hoisted or cached
- Expensive helpers, serialization, or data reshaping repeated per item when the result can be prepared once by the host application
- Report performance findings only when request frequency or collection size makes the cost evident; do not turn whitespace-trimming preferences or equivalent shorthand forms into blocking findings
