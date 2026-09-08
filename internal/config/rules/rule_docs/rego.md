#### Rego Review Principles
> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat authorization-bypass and data-exposure findings as blocking, and style or idiom suggestions as non-blocking. Ground findings in the Rego under review and observable caller, schema, and configuration evidence. Do not assume the runtime shape or provenance of `input` or `data`, the OPA/Regal version, or enforcement behavior (fail-open vs fail-closed) when that context is unavailable.

#### Obvious Typos or Spelling Errors
- Spelling errors in package names, rule names, function names, or `import` aliases at their declaration sites; do not report spelling errors at reference sites
- Typos in `deny`/`violation` message strings that affect readability of the policy decision output

#### Default Posture and Overly Broad Allow
- A missing `default allow := false` (or equivalent deny default) in an authorization policy, so no rule matching means undefined rather than an explicit deny the caller handles consistently
- An `allow` rule with no body, `if true`, or constraints that can never fail, granting access unconditionally
- A `deny`/`violation` set that the caller never consults (e.g. `allow` decided independently of `deny`), so listed violations do not actually block anything
- New `allow` rules added without checking they are intersected with, not unioned past, the existing `deny` rules on the same decision path

#### Undefined, Negation, and Unification
- `not` used where undefined was meant, or vice versa: `not x` succeeds when `x` is undefined, so `input.user == x` with an absent field behaves differently from an explicit `false`
- Unification (`=`) used where comparison (`==`) was intended, silently binding a variable instead of testing it — especially inside `with` overrides or function arguments
- A rule body that is undefined (no value) on a reachable path where the caller expects `false` or an empty set, flipping the downstream default
- Negation over a non-ground term (`not some_unbound_var == value`) whose scope is wider than intended

#### Unsafe Variables and Iteration
- Unbound or unsafe variables the compiler would reject being "fixed" by binding them to attacker-influenced `input` instead of a trusted value
- `some x in input.collection` iterating a field that may be absent or a scalar rather than an array/set/object, making the rule undefined instead of denying
- Partial iteration that checks only the first match (e.g. `input.roles[i] == "admin"` binding `i` once and succeeding) where every element needed checking; `every` or a comprehension with aggregation was required
- Object key iteration (`some k, v in input.map`) that assumes keys present without `object.get` defaults or presence checks

#### Input, Data, and Trust Boundaries
- Authorization-relevant `input` fields (roles, groups, `is_admin`, expiry) used as trusted facts when observable caller or policy code shows an attacker can supply them without validation; missing in-policy validation alone does not establish attacker control
- Authorization decisions based on `data` from a demonstrably untrusted bundle, upload, or refresh path; use available loading and deployment configuration to establish provenance, and do not flag an unknown source as unsafe merely because it is not pinned in the Rego file
- JWT or token claims used for authorization with evidence that required signature, expiry, issuer, or audience validation is missing or bypassed along the observed policy and caller path; validation may happen upstream, and its absence from this file alone is not a defect
- `with` overrides that demonstrably bypass a security constraint on the enforced decision path, such as replacing a validated identity with an attacker-controlled value; scoped overrides for policy composition or hypothetical queries are valid in production and are not findings by themselves

#### Non-Determinism and Functions
- Non-deterministic values or final decisions incorrectly reused across evaluations, with evidence that freshness or determinism requirements are violated; caching a compiled policy or using partial evaluation is not itself a defect, since calls such as `time.now_ns()` may remain in residual queries for runtime evaluation
- `http.send` settings that demonstrably disable required safeguards (e.g. `tls_insecure_skip_verify: true` or a zero timeout on a path that requires a bound), or an error path that changes the authorization decision incorrectly; omitted options retain OPA's defaults of a five-second timeout and TLS certificate verification
- Recommend HTTP response caching only for an established performance problem with an acceptable freshness contract; uncached requests are valid when decisions require fresh data
- A custom function that is undefined for some inputs (no return value on a reachable path) used as though it always returns a value
- `print()` or `trace()` calls left in committed policy, leaking evaluated values into logs

#### Version Drift and Structure
- `if`, `contains`, and `in` keywords used without the matching `import future.keywords` on toolchains that still require it, or the import retained as dead weight where the target version makes it unconditional — check the file's own imports before flagging
- `package` name shadowing an `import`ed path, so an unqualified rule reference resolves to a different package than the author meant
- `default` rules contradicting each other on the same name, or a `default` value of a different type than the rule body produces
- Duplicate rule names in the same package whose bodies union when the author meant override: multiple `allow if ...` definitions all contribute, none replaces another

#### Testing Correctness
- A `test_` rule that asserts nothing about the behavior it names (no `allow`/`deny`/`violation` check against concrete `input`/`data`)
- Tests using `with` to supply an `input` shape that production never sends, or covering only the allow path with no deny-path counterpart
- A test asserting only that evaluation did not error, without checking the decision value
