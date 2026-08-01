#### Obvious Typos or Spelling Errors
- Spelling errors in macro names, assigned variable names, or user-facing text at their declaration sites; do not report at reference sites
- Typos in `<#assign>`/`<#macro>`/`<#function>` names that surface only at render time (`InvalidReferenceException` / macro not found), or that are silently masked by `!` defaults and `??` guards

#### Output Escaping and XSS
- Interpolations (`${...}`) that reach HTML without escaping: flag only when auto-escaping is not already active — i.e. the template lacks `<#ftl output_format="HTML">` (FreeMarker 2.3.24+) AND does not use the `.ftlh`/`.ftlx` extension (which auto-activate HTML/XML escaping via `recognize_standard_file_extensions`, on by default since 2.3.24) — and the value is not passed through `?html`/`?url`/`?js_string` appropriate to its sink (HTML body, attribute, URL, JS, CSS)
- Explicit `?no_esc` or `<#noautoesc>` on values that carry user-controlled data — treat as a high-risk escape hatch; flag unless the source is clearly trusted or already sanitized
- Escaping with the wrong context builtin (e.g. `?html` for a value placed inside a URL or inline `<script>`)
- Do not report missing `?html` when auto-escaping is active for the file's output format and no override disables it

#### Template Injection / RCE (SSTI)
- User-controlled data concatenated into template source, or templates whose name/body derives from request input (`<#include>`, `<#import>`, `.get_optional_template(userValue)`) — enables server-side template injection
- Use of the `?new()` builtin to instantiate `TemplateModel` classes, especially `freemarker.template.utility.Execute` or `ObjectConstructor` — arbitrary code execution; flag unless the class is a vetted internal type
- `?api` / `?eval` on untrusted input, or exposing raw `Class`/`ClassLoader`/`ProcessBuilder`-reachable objects into the data model
- Templates authored from untrusted input without a restricted `TemplateClassResolver` (e.g. `SAFER_RESOLVER`) — call it out as a hardening gap

#### Null and Missing-Value Handling
- Interpolations or directive arguments on possibly-absent values without `!` (default) or `??` (existence) — missing values raise `InvalidReferenceException` at render time
- Overuse of a bare `!` that masks genuinely-required data with a silent empty string; prefer an explicit default (`value!"fallback"`) or an `<#if value??>` guard where absence is meaningful
- `!` precedence mistakes in expressions (`a.b.c!` guards only the last step); confirm the intended nullable segment

#### Logic-in-Template Smells
- Business logic, data-access, or non-trivial computation embedded in templates that belongs in the controller/model layer
- Deeply nested `<#if>`/`<#list>` or duplicated conditional blocks that indicate the view is doing the model's job
- `<#assign>` used to build state that should have been prepared before rendering

#### Macro and Include Hygiene
- `<#include>` where `<#import>` (namespaced) is intended, causing global-namespace pollution or accidental variable shadowing
- Macros/functions defined but never called, or duplicated across templates instead of shared via a common library template
- Relative template names passed to `<#include>`/`.get_optional_template` without `?absolute_template_name` when resolution context is ambiguous
- Missing-template failures not handled (`.get_optional_template(...).exists`) where the include is optional

#### Internationalization and Locale-Sensitive Formatting
- Numbers, dates, times, and currency emitted with locale-default formatting where a fixed machine format is required (e.g. `?string`/`?c` for numbers in URLs, JSON, or IDs) — `?c` (computer format) prevents locale-dependent thousands separators corrupting non-display output
- Hard-coded user-facing strings that should come from a localized message/resource bundle
- Date/number output relying on an implicit locale/timezone without confirming the render environment sets them intentionally
