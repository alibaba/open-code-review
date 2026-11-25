> Favor precision over recall: report a concrete rendering error or a demonstrated trust-boundary violation. ERB can generate any text format; distinguish Ruby's ERB from Rails' HTML template handler and inspect the output context.

#### ERB Escaping and Output Contexts
- Ruby's standard ERB substitutes `<%= expression %>` without automatic HTML escaping. Rails HTML templates escape ordinary unsafe strings, while `raw` and `html_safe` bypass that protection. Verify the host and value before applying either rule.
- Report untrusted values reaching HTML without appropriate escaping or sanitization. Do not require an additional helper for an ordinary Rails HTML text-node value already escaped by the handler, or flag trusted helper-generated markup solely because it is HTML-safe.
- HTML escaping does not serialize JavaScript, CSS, JSON, or event-handler expressions, and does not reject dangerous URL schemes. Trace the value to the actual sink rather than treating every `<%= %>` as HTML output.
- Check JSON embedded in a script element for HTML parser terminators such as `</script>`; JSON validity alone does not keep the surrounding script element intact.

#### Template Trust and Ruby Evaluation
- ERB evaluates Ruby code. Attacker-controlled template source is a code-execution boundary; ordinary untrusted locals passed to a fixed template are not themselves template source.
- Report request-controlled render options, template names, or partial paths when the actual host API allows unintended selection, file access, or disclosure. Do not assume a filesystem escape from a dynamic name alone.
- Check partial locals and controller-provided data for missing values, shadowing, or exposed secrets when a concrete failure or disclosure is reachable.

#### Whitespace and Output Structure
- `<% ... %>` executes Ruby without inserting its result; `<%= ... %>` inserts a result. `<%-` and `-%>` concern whitespace trimming under the handler's supported trim mode, not an unescaped-output tag as in EJS.
- Check conditionals, loops, and block boundaries for missing or duplicated markup, invalid nesting, and controls emitted on the wrong branch. Partials are compiled separately; do not assume an opening Ruby block in one template can be closed in another.
- Check trim-mode changes for joined tokens, removed separators, or unsupported syntax. Do not report whitespace preferences without a rendered consequence.
- Check partial/layout changes for missing required wrapper content, repeated form IDs, or labels that point to the wrong control.

#### References
- [Ruby ERB documentation](https://docs.ruby-lang.org/en/master/ERB.html): execution and expression tags, rendering bindings, and trim modes.
- [Rails output safety](https://guides.rubyonrails.org/active_support_core_extensions.html#output-safety): automatic escaping in HTML views, `raw`, and `html_safe`.
