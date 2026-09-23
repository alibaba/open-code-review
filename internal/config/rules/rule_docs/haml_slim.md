> Favor precision over recall: only raise an issue when the rendered consequence or trust boundary is clear. HAML and Slim are Ruby template languages that evaluate embedded Ruby, so distinguish their built-in HTML escaping from the validation or serialization required by the actual output context.

#### HAML and Slim Escaping and Output Contexts
- Unescaped interpolation (`!=` in HAML, `==` in Slim) or `html_safe` / `raw` output that can receive untrusted HTML without prior, context-appropriate sanitization
- Escaped interpolation (`=` in HAML, `=` in Slim) used inside JavaScript, CSS, JSON, URL, or event-handler syntax as though HTML escaping protected those grammars
- Do not report ordinary `=` / `#{value}` in an HTML text node solely for lacking an explicit escape helper; both languages HTML-escape those forms by default
- Sanitized HTML rendered unescaped after transformations that invalidate the sanitizer's guarantee, such as concatenating new untrusted markup afterward

#### Attributes, URLs, and Dynamic Markup
- Attribute values from untrusted data that are HTML-escaped but not validated for the sink, especially `href`, `src`, `action`, `formaction`, `srcdoc`, `style`, and event-handler attributes
- Splat or hash attribute spreads of untrusted data that can introduce event handlers or override security-sensitive attributes; inspect attribute names and non-HTML sink semantics rather than conventional helper forwarding
- Boolean attributes whose values use strings such as `"false"` when presence still enables the HTML behavior, or conditional attributes that leave a control enabled, selected, or focusable on the wrong branch
- IDs, `name` values, fragment links, or ARIA references generated in loops without a stable uniqueness guarantee

#### Embedded Ruby and Server-to-Client Data
- Values interpolated into inline `:javascript` blocks or event handlers without JavaScript-string or JSON-safe serialization; HTML escaping is not a substitute for JavaScript encoding
- JSON embedded in an inline script without neutralizing HTML parser terminators such as `</script>`, allowing data to end the script element or break parsing
- Secrets, session data, authorization material, internal-only fields, or unnecessarily large objects serialized into client-visible markup or scripts
- Client-side assumptions in a server-rendered template, including access to browser globals the server never has, or server-only state the browser never receives

#### Template Trust, Ruby Evaluation, and Side Effects
- Attacker-controlled template source or attacker-writable template files passed to the renderer; these templates execute embedded Ruby, so treating untrusted templates as presentation data creates a server-side template injection and code-execution boundary
- Request-controlled template or partial selection without a strict name allowlist and template-root boundary, allowing unintended files to be rendered
- Request, query, or body objects spread wholesale into render options, allowing untrusted keys to alter locals, layout, or format selection; keep render options fixed and expose an explicit locals shape
- Embedded Ruby that performs database, network, filesystem, or shared-state mutation during rendering rather than in the controller or view-model layer
- Required locals that can become empty output, omitted attributes, invalid identifiers, or broken URLs without an explicit default or branch; do not require defaults for optional display-only values
- Exceptions from property access or method calls on absent locals that can abort the whole render, especially inside shared layouts or error pages

#### Partials, Layouts, and Ruby Control Flow
- Relative `render` / `partial` paths that resolve against a different root than the author intended, or absolute paths that unintentionally depend on a different template root
- Locals passed to a partial that omit a value the partial reads, or an overly broad `locals` hash that leaks fields across component boundaries
- Layout or wrapper overrides that silently drop required metadata, scripts, security controls, fallback content, or accessibility structure
- Loops or conditionals whose HAML/Slim whitespace control changes which elements are emitted, so a block that appears nested is rendered as a sibling (and vice versa)
- `while` or recursive constructs with a reachable non-terminating path, or expensive expressions and method calls repeated inside large loops

#### Indentation and Whitespace Semantics
- Indentation changes that move an element into the wrong parent, loop, conditional, or block, changing form ownership, DOM structure, visibility, or execution scope; HAML and Slim both derive structure from indentation, so a reindent is a semantic change
- Inline tags and text whose whitespace rules concatenate words or tokens, or reliance on trailing spaces that formatters and editors can remove
- Literal HTML or plain-text blocks whose indentation looks like template nesting but is emitted mostly unprocessed, producing a different structure than intended

#### Accessibility
- Interactive non-button elements without equivalent keyboard activation, focusability, and semantics when a native element would provide them
- Inputs generated without an associated label, repeated form-control IDs, or labels/ARIA references that point to a different loop item
- Images with missing or misleading alternatives, and conditional content that changes visible state without updating accessible name, role, or `aria-*` state
- Layout or partial overrides that silently remove landmarks, heading structure, fallback text, focus management, or required attributes supplied by the parent

#### Performance and Review Scope
- Partials rendered per item inside a hot loop when the result is identical across iterations and can be hoisted or cached
- Expensive helpers, serialization, or data reshaping repeated per item when the result can be prepared once by the host application
- Report performance findings only when request frequency or collection size makes the cost evident; do not turn indentation preferences or equivalent shorthand forms into blocking findings
