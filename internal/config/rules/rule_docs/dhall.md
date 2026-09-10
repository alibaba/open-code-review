> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the Dhall under review; do not infer the contents of an imported file, the behavior of the remote endpoint behind a URL import, or the value an `env:` variable will hold at render time.

#### Obvious Typos or Spelling Errors
- Spelling errors in `let` binding names, record field names, and union alternative names at their declaration sites; do not report spelling errors at reference sites
- Typos in comment text that describe a schema's public interface and would mislead a consumer

#### Remote Imports and Integrity Hashes
- A remote import by URL with no trailing `sha256:<hex>` integrity hash — an unpinned dependency whose content can change under the project without any diff; `dhall freeze` adds the hash
- An integrity hash left unchanged while the import target visibly moved in the same diff (a new tag, commit, branch, or path). The cache is keyed by the hash, so a machine that already has it keeps serving the old content without fetching the URL, while a cold cache (CI, a new checkout) fails with a hash mismatch — the fix is re-running `dhall freeze`, never editing the hex by hand
- A real-looking secret committed as the fallback after `?` rather than an obvious placeholder such as `"password"` or `"disabled"`; reading a credential from `env:` is how it stays out of the file and is not itself a finding
- `env:VAR` without `as Text` where the fallback or use site shows a `Text` value (`env:HOST ? "127.0.0.1"`): without `as Text` the variable's value is parsed and type-checked as Dhall source, so `HOST=localhost` fails as an unbound variable. `env:PORT ? 8080` (a `Natural`) and `env:DHALL_PRELUDE ? https://...` (an import override) read Dhall on purpose and are fine
- Do not flag a bare relative import (`./types.dhall`, `../Prelude/package.dhall`) for lacking a hash — local imports are the overwhelming majority and pinning them is not the convention
- Do not flag the `missing sha256:<hex> ? ./local.dhall` idiom used throughout the Prelude; that is a content-addressed cache lookup with a working local fallback, not an unpinned import

#### Records
- `//` used to change one nested field (`deployment // { metadata = ObjectMeta::{ name = Some "web" } }`): `//` is a shallow merge, so the new `metadata` replaces the old one whole and its other fields (labels, namespace) silently reset or disappear; `deployment with metadata.name = Some "web"` updates only the nested field
- Do not report what the type checker already rejects: a missing required field or a misspelled field in `Type::{ ... }` completion, a `merge` that lacks a handler for a union alternative, or a `let` binding that refers to itself (Dhall `let` is not recursive, so that is an unbound variable)
