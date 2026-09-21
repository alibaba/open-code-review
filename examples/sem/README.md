# Optional structural code context with Sem

[Sem](https://github.com/Ataraxy-Labs/sem) exposes entity-level code context,
structural diffs, and dependency analysis over MCP. Install the Sem executable
separately and verify `sem mcp --help` before enabling this example.

Compatibility: OCR's Go MCP SDK can probe `server/discover` before initializing.
Sem must answer that unsupported method without closing the connection. The
installed Sem 0.22.1 failed this check during development; use a build containing
the [discovery-probe compatibility fix](https://github.com/Ataraxy-Labs/sem/pull/490)
and pass its absolute path as `command`.
Do not treat this example as validated against the current published release.

Merge `config.json` into `~/.opencodereview/config.json`; preserve existing
providers, settings, and MCP servers. OCR launches Sem in the repository being
reviewed, so no fixed repository path or remote service is configured.

The four-tool allowlist avoids exposing unrelated tools. Use `sem_context`
with an entity name and a bounded `token_budget` for structural questions.
Keep OCR's built-in file reads and code search for unsupported syntax, textual
matches, and incomplete structural coverage. A dependency graph is not proof
of behavioral correctness or of complete dynamic dispatch resolution.

Cloud graph access is disabled in this example. Local cache files may be
created. Inspect OCR's stderr to confirm connection/tool registration: OCR can
continue its review if an MCP connection fails. This configuration does not
establish improved review quality, lower token use, or faster reviews; measure
those against the same model and changesets before claiming a benefit.

Remove the integration with `ocr config unset mcp_servers.sem`.

To test the connection using OCR's real MCP client without calling a model,
run from this repository's root:

```sh
go run ./examples/sem --repo /absolute/path/to/checkout --entity SomeFunction
```
