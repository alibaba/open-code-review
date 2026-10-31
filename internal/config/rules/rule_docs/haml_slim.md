> Favor precision over recall: report a concrete rendering error or a demonstrated trust-boundary violation. Check the installed engine version, host integration, and effective escaping options before judging output safety.

#### HAML and Slim Escaping and Output Contexts
- Haml `!=` and Slim `==` bypass HTML escaping. Report untrusted data reaching these forms without context-appropriate sanitization; do not flag trusted partial or layout output solely because it uses a raw-output form.
- Haml `=` depends on `escape_html`; current Haml enables it by default, but applications and older versions can differ. Slim escapes ordinary `=` output by default unless `disable_escape` or a supported HTML-safe value bypasses it. Trace the actual configuration and value rather than assuming all output is escaped or all output is unsafe.
- Do not require an extra escape helper for ordinary text-node interpolation when the effective engine configuration already escapes that value. Missing configuration evidence alone is not proof of XSS; establish untrusted input and an unsafe output path before reporting.
- HTML escaping does not validate URL schemes or serialize JavaScript, CSS, JSON, or event-handler expressions. Check the actual sink, including script data containing `</script>` and dynamic attribute names that can introduce handlers.
- Treat `html_safe` / `raw` as trust assertions, not sanitizers. Report them when untrusted content can reach executable markup.

#### Template Trust and Ruby Evaluation
- Haml and Slim evaluate embedded Ruby. Attacker-controlled template source is a code-execution boundary, unlike ordinary data passed as locals to a trusted template.
- Report request-controlled template or partial selection only when it can select unintended content under the actual renderer and loader configuration.
- Trace missing or shadowed locals, helper failures, and side effects to a concrete render failure, information leak, or unintended operation.

#### Indentation and Whitespace Semantics
- Both engines derive nesting from indentation. Check changes that move markup across a parent, conditional, loop, or Ruby block, especially when form ownership or control visibility changes.
- Review Haml whitespace removal and Slim whitespace modifiers for joined words, missing separators, or changed text nodes. Do not report equivalent indentation or shorthand preferences without a rendered consequence.
- Check partial/layout changes for omitted required markup, incorrect locals, and repeated form IDs or broken label references; trusted helper forwarding alone is not a defect.

#### References
- [Haml reference](https://haml.info/docs/yardoc/file.REFERENCE.html): Ruby evaluation, escaping, indentation, whitespace, and Rails options. [Current engine defaults](https://github.com/haml/haml/blob/main/lib/haml/engine.rb) define `escape_html: true`; verify the version used by the project.
- [Slim documentation](https://github.com/slim-template/slim#readme): output forms, interpolation, indentation, whitespace modifiers, `disable_escape`, and `use_html_safe`.
- [Rails output safety](https://guides.rubyonrails.org/active_support_core_extensions.html#output-safety): HTML-safe strings bypass ordinary escaping without being sanitized.
