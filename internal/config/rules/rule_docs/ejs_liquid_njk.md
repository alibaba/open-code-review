> Favor precision over recall: report a concrete rendering error or a demonstrated trust-boundary violation. EJS executes JavaScript, Liquid uses a restricted template language, and Nunjucks has its own tags and expressions; do not apply one execution or escaping model to all three.

#### EJS, Liquid and Nunjucks Escaping and Output Contexts
- EJS `<%= value %>` uses the configured escape function (HTML escaping by default); `<%- value %>` emits raw output. Report untrusted HTML reaching raw output, but do not flag `<%- include(...) %>` solely for outputting a trusted partial's rendered HTML. Inspect the partial and its data flow.
- Liquid does not automatically HTML-escape ordinary `{{ value }}` output. For untrusted HTML text, look for `escape`, `escape_once`, appropriate upstream escaping, or sanitization; a missing literal `escape` filter alone does not prove a vulnerability. `safe` is a Nunjucks filter, not a standard Liquid escape bypass.
- Nunjucks enables autoescaping by default, but `autoescape: false`, `safe`, or a `SafeString` can bypass it. Trace the effective environment and value before exempting an interpolation from review. A custom filter returning an ordinary string is not automatically an escape bypass.
- Do not require a redundant escape helper for an HTML text-node value already escaped by EJS or Nunjucks under the effective configuration. Do not assume that exemption applies to Liquid or to values marked safe.
- HTML escaping does not validate URL schemes or serialize JavaScript, CSS, JSON, or event-handler expressions. Check the actual output context and script-element terminators such as `</script>`.
- Report sanitization bypasses when later concatenation or transformation demonstrably introduces untrusted markup into the emitted value.

#### Template Trust, Evaluation, and Side Effects
- EJS templates execute JavaScript; Nunjucks explicitly does not sandbox user-defined templates. Report attacker-controlled source compiled or rendered by these engines, separately from ordinary locals passed to a fixed trusted template.
- EJS render options must not be populated wholesale from request data when that gives the caller control of compilation or template selection.
- Liquid is designed for user-editable templates with restricted evaluation. Do not equate untrusted Liquid source with arbitrary host-code execution. Inspect exposed objects, custom tags/filters, and the configured filesystem for a concrete disclosure or execution path.
- Dynamic include, import, or layout names require evidence that the configured loader can select unintended content; do not infer arbitrary file access from a dynamic name alone.

#### Missing Data, Partials, and Control Flow
- Check required locals and partial arguments for concrete missing-data failures, shadowing, or information leaks. Liquid `render` isolates locally created variables; do not assume the same scope behavior as `include`.
- Check EJS braces, Liquid block tags, and Nunjucks block tags for missing or duplicated output, malformed markup, and controls emitted on the wrong branch.
- Review custom filters and macros for the actual return type and escaping behavior; marking an untrusted result safe can bypass Nunjucks escaping.
- Report repeated expensive render work only when the data size or request path demonstrates a material cost; do not turn formatting, optional display defaults, or ordinary template logic into findings.

#### References
- [EJS documentation](https://ejs.co/docs/): output tags, includes, escaping options, and the render-input security warning.
- [Liquid introduction](https://github.com/Shopify/liquid#introduction), [escape](https://shopify.github.io/liquid/filters/escape/), [escape_once](https://shopify.github.io/liquid/filters/escape_once/), and [render](https://shopify.dev/docs/api/liquid/tags/render): restricted templates, explicit escaping, and isolated partial scope.
- [Nunjucks API](https://mozilla.github.io/nunjucks/api.html): autoescaping defaults and the user-defined-template warning; [templating documentation](https://mozilla.github.io/nunjucks/templating.html#safe): the `safe` filter.
