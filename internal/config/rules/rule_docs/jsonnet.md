> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Review only what is observable in the Jsonnet under review; do not infer the contents of libraries imported from outside the diff, the values supplied for external variables or top-level arguments, or how the rendered output is consumed downstream. Do not flag formatting that `jsonnetfmt` would silently fix.

#### Late Binding: self, $, and super
- `$` used where the enclosing object was meant. `$` is pinned to the outermost object of the *file it is written in*, so a library field that reaches for `$._config` resolves against the library's own root, not the caller's tree, the moment the object is merged into someone else's configuration
- A field reading `self.x` from inside a nested object where `self` has already rebound to that nested object rather than the one holding `x`. The established fix is capturing the intended scope once at the top of the object — `local this = self,` / `local defaults = self,` — and referring to `this.x`; flag a nested `self` reference where such a capture already exists in the same object and was clearly meant to be used
- `super.f` referenced in an object that is not the right operand of a `+` in any reachable composition, which is a runtime error rather than a silent default
- A `local` shadowing a field name that later code refers to unqualified, so a change to the field no longer affects the reference

#### Object Composition and Overrides
- `+` between two objects merges only the top level: a field present in both is taken wholesale from the right operand, and its nested contents are replaced rather than merged. Flag a nested field written with `:` where the surrounding overrides use `+:` and the intent is clearly to add to the inherited value, not to discard it
- `f+: v` evaluates to `super.f + v`, so the operator's meaning follows the type: objects merge one level, arrays concatenate, strings concatenate as text. An override that means "replace this list" written as `+:` silently appends instead, producing duplicate containers, volumes, or arguments
- `f+:` where `f` does not exist in the inherited object — a renamed or misspelled field — silently defines a new field instead of overriding anything, and nothing reads it
- An override applied to an object that is not on the right-hand side of the composition it was written for, so the later operand wins and the override is dropped from the output

#### Hidden Fields and Rendered Output
- A `::` field the rendered manifest is expected to contain. Hidden fields are absent from the output with no error, so the omission surfaces only where the artifact is applied
- A `:` field holding an internal helper, a partially built template, a raw function argument, or a credential, which leaks verbatim into the rendered YAML/JSON
- `:::` used to force visibility on a field that the library deliberately hid, without a stated reason
- The required-argument idiom `x:: error 'must provide x'`: the error only fires when something reads `x`, so an override that misspells the field name leaves the default in place and the failure appears far from its cause, or not at all if nothing reads it
- Null versus omission when the target is a Kubernetes manifest: an explicit `field: null` is a delete/reset in a strategic-merge patch, while an omitted field inherits the server default. `std.prune` and an explicit `null` are different requests, not stylistic variants — flag one substituted for the other

#### Imports and External Inputs
- `import` used on a file that is not Jsonnet, where `importstr` (raw text) or `importbin` (raw bytes) was meant, and the reverse: `importstr` on a Jsonnet file, yielding source text rather than a value
- Imports resolve at compile time against the `-J`/jpath search path, so a file added earlier in that path shadows the intended one and changes the output with no diagnostic. Flag an import whose relative path reaches into a vendored tree directly rather than through the library's documented entry point
- `std.extVar('name')` or a top-level argument read without a documented default or any validation — output then depends on state that is invisible in the file
- External variable values arrive as strings; flag one compared to a number, used in arithmetic, or treated as an object without `std.parseInt`/`std.parseJson`/`std.parseYaml`
- `std.extVar` inside a computed field name (`[if std.extVar('x') then 'k']`), where an unset or falsy value changes which keys exist in the output at all

#### Termination and Manifestation
- A recursive function or self-referential object with no argument that provably shrinks toward its base case. Evaluation is lazy, so an infinite structure is built without complaint and only exhausts the stack when a consumer forces it
- `std.manifestYamlDoc` quotes keys by default; `quote_keys=false` is what produces unquoted YAML keys. Flag its output being fed to a consumer that requires plain keys, and flag the result — a string — being re-parsed or indexed as if it were structured data
- A manifested string interpolated into a CLI flag, a ConfigMap entry, or an annotation where indentation, a multi-document `---` separator, or a non-string scalar changes how the receiver parses it
- `std.toString` or `std.manifestJson` used as the input to a hash, a checksum annotation, or an equality check, where field ordering or the representation of numbers is not guaranteed to be stable across evaluator versions
- `assert` used to validate an input at a point that is never forced, so the check silently never runs
