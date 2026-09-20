> Favor precision over recall: report concrete rendering failures or trust-boundary violations supported by the template and its callers. Check the project's Twig version, extensions, loader, `strict_variables`, and autoescape configuration before assuming behavior. Twig can generate formats other than HTML.

#### Twig Escaping and Output Contexts
- `|raw`, `{% autoescape false %}`, or custom filters/functions marked safe that let untrusted values reach HTML without context-appropriate sanitization
- HTML escaping used for JavaScript, CSS, or URL syntax, or interpolated values in unquoted HTML attributes; choose escaping for the actual output context and validate URL schemes separately
- JSON inserted into an inline script without neutralizing HTML script terminators; verify the serializer's options and output before claiming that `json_encode|raw` is unsafe
- Do not flag ordinary `{{ value }}` in HTML text or quoted text attributes when HTML autoescaping is active; do not demand an additional `|e` for already escaped output
- Macro and `parent()` results treated as if the caller will escape them again: these return safe markup, so inspect escaping at the point where untrusted values enter that markup

#### Template Source and Selection
- Untrusted values compiled as template source through `template_from_string` or the host's `createTemplate`; passing data to a fixed template is not itself server-side template injection
- Request-controlled names in `include`, `extends`, `embed`, or `source()` that can select unintended loader-visible templates; verify the loader boundary instead of assuming arbitrary filesystem access
- User-authored templates evaluated without the intended sandbox restrictions, or allowed custom functions/methods that expose privileged operations; autoescaping does not sandbox template execution

#### Undefined Values and Defaults
- Misspelled or absent required variables that produce broken URLs, identifiers, or form values when `strict_variables` is disabled, or abort rendering when it is enabled
- `value|default(true)` replacing a meaningful `false` value: `default` treats `false` as empty, while `value ?? true` preserves it; report only when the fallback changes intended behavior
- Guards or defaults applied after evaluating an unsafe argument or method call, leaving that earlier evaluation able to fail
- Do not require fallback values for optional display content or infer that every missing variable throws without checking `strict_variables`

#### Includes, Inheritance, and Macros
- Includes relying on caller variables removed by `only`, or `include()` calls using `with_context: false` without passing required data; conversely, inherited context exposing sensitive values to a template that should receive a narrow input
- `ignore missing` hiding the loss of a required fragment, or an ordered fallback template list selecting an existing but inappropriate template
- Macros reading caller-local variables without receiving them as arguments; macros do not inherit the current template's variable context
- Imported macro names shadowing a built-in function, or imports assumed to be available in included/child templates without an import there
- Child block overrides dropping required parent content when replacement was unintended, or recursive include/macro chains with no reachable termination condition

#### Control Flow, Whitespace, and Rendering Cost
- Variables first declared inside a `for` loop used after the loop as though they were available there; initialize an accumulator outside the loop when it must survive the loop's scope
- Nested loops using the wrong `loop` metadata, or conditionals and whitespace control (`-` / `~`) that remove required separators or change structured output
- Expensive getters, custom functions, or filters repeated inside large loops, especially when attribute access triggers lazy database queries; require evidence of repeated work before reporting a performance issue
- Side-effecting methods invoked during rendering, or secrets and server-only data emitted into client-visible markup or scripts
