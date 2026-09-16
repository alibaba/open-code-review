# ocr-acp: ACP Adapter for OpenCodeReview

Independent ACP (Agent Client Protocol) adapter for OpenCodeReview CLI.

## Status

**Status as of 2026-09-15: phase 7 automated validation and phase 8 manual
client acceptance are complete. Phase 8 release preparation remains in progress.**
The author confirms full client acceptance in Zed and a VSCode ACP extension.
This records the tested installations, not every client version or platform.

| Evidence level | Current evidence | Remaining verification |
| --- | --- | --- |
| Implemented | ACP v1 stdio, command discovery, parser clarification/rejection, review/scan, cancellation, bounded output, formatted findings and safe navigation | Unsupported features listed below remain outside the first release |
| Automatically tested | Local macOS checks and protocol/lifecycle tests passed; Linux ACP CI passed at `aa7d4e7`, including cleanup regressions, race-enabled coverage (90% minimum), build, official Python SDK smoke with mock OCR and govulncheck | Each later revision needs its own CI result; see the linked run below for the verified revision |
| Manually accepted in real clients | Author-confirmed full acceptance in Zed and a VSCode ACP extension on 2026-09-15 | Exact client/extension versions and local patch details were not supplied for this record; other installations need their own compatibility checks |
| Real OCR/LLM quality | Individual real review runs have been observed | No controlled quality benchmark or complete client/provider matrix; mock tests do not establish review quality |

- ✓ Phase 1: CLI investigation and contract definition
- ✓ Phase 2: Overall design and architecture
- ✓ Phase 3: CLI Contract and mock OCR
- ✓ Phase 4: Command parsing and clarification
- ✓ Phase 5: OCR process orchestration and macOS/Linux lifecycle validation
- ✓ Phase 6: ACP stdio implementation and protocol validation
- ✓ Phase 7: Local regression testing and Linux ACP CI passed at `aa7d4e7`
- ◐ Phase 8: Full Zed and VSCode ACP client acceptance confirmed; versioned release, installation and released-OCR compatibility evidence remain to be finalized
- ⏳ Phase 9: Delivery review

### Phase 7 Acceptance

Local macOS verification at `1202d79` passed `make -C acp check`,
`make -C acp test` and `make -C acp coverage`. Both test targets enable race
detection; total statement coverage is 91.9%, above the 90% gate. Regression
tests cover inherited version-output waits, process-group cleanup after the
leader exits, brief helper output flushing, cancellation and long stderr lines.
The default suite does not enable the opt-in real OCR binary or real LLM tests.

[Linux ACP CI run 34986645165](https://github.com/alibaba/open-code-review/actions/runs/34986645165)
passed at `aa7d4e7`, which includes both `36812e5` and `1202d79` and their
version-probe and post-exit process-group cleanup regressions. The run includes
quality checks, race-enabled coverage, the adapter build, the official Python
SDK smoke test and govulncheck. This evidence applies to that revision;
subsequent changes require their own validation. Manual Zed and VSCode ACP
client acceptance is recorded separately above.

## Project Structure

```
acp/
├── cmd/ocr-acp/          # Independent stdio server and shutdown owner
├── internal/
│   ├── adapter/          # SDK boundary, sessions, protocol and result mapping
│   ├── contract/         # CLI contract and intent structures (phase 3)
│   ├── intent/           # prompt -> intent / clarify / reject (phase 4)
│   ├── llmresolve/       # OCR LLM config resolution + protocol client (phase 4)
│   └── orchestrator/     # OCR process lifecycle and result decoding (phase 5)
├── testdata/
│   └── mock-ocr/         # Test double for OCR CLI (phase 3)
├── Makefile              # Build, test, and quality checks
└── go.mod                # Independent Go module
```

`cmd/ocr-acp` starts the independent ACP server. It exposes `/review` and
`/scan` through ACP `available_commands_update` and emits findings once in the
final message. Safe locations become Markdown file links, resolved against the
Git root for review and the session cwd for scan; other locations remain text.

## Quick Start

The Test, Quality Checks and Build commands below run from `acp/`.
From the repository root, use `make -C acp <target>`.

### Test

```bash
make test
make coverage
```

`make coverage` fails if total statement coverage drops below `COVERAGE_MIN`
(default 90%). Override per-run with `make coverage COVERAGE_MIN=95`.

### Quality Checks

```bash
make check
make english-check
```

`make check` fails on unformatted files, on `go vet` findings, and on
`staticcheck` findings. It looks for `staticcheck` on `PATH` first and then in
`$(go env GOPATH)/bin`, where `go install` puts it, so a local install is
found without editing `PATH`. When the binary is genuinely absent it prints a
notice and skips it — it never reports a pass for a check it did not run.

A present-but-useless `staticcheck` is also a failure, not a pass: if it
reports `matched no packages` it analyzed nothing, which usually happens when
the binary is newer than the active Go toolchain. `staticcheck` 0.8.x needs a
Go >= 1.26 toolchain while this module declares `go 1.23`, so CI pins a
`golang:1.26.6` container for exactly that reason.

### CI

`acp/` is a nested Go module, and the root `ci.yml` resolves its package list
with `go list ./...`, which does not descend into a nested module. Nothing in
this directory is built or tested by the workflows at the repository root.

`.github/workflows/acp-ci.yml` covers it instead — the same arrangement
`pages-ci.yml` uses for `pages/`. It runs the license, formatting, vet,
staticcheck, coverage and govulncheck gates in a `golang:1.26.6` container,
builds the adapter and runs the pinned official Python SDK smoke test with
mock OCR. No real review-provider credentials are required.
Its `pull_request` trigger deliberately has no `branches:` filter, so it also
runs for pull requests into an integration branch rather than only `main`.

Because `make check` skips a missing staticcheck, the workflow asserts
`staticcheck -version` succeeds before calling it, and `make check` itself now
fails if the analyser reports `matched no packages`.

### Build

```bash
make build
./../dist/ocr-acp --ocr-binary /path/to/ocr
```

The server speaks ACP v1 over stdio. Configure natural-language parsing with
`--parser-provider`, `--parser-model`, `--parser-base-url` and the
`OCR_ACP_PARSER_API_KEY` environment variable. `--turn-timeout` bounds a
complete turn. Windows currently exits with `platform_unsupported`.

### ACP Client Configuration and Upgrades

Build with `make -C acp build` from the repository root. Register a custom
agent named `OpenCodeReview` in an ACP v1 stdio client with these launch values
(replace both absolute paths). This is a launch configuration example, not a
complete settings file: field names and nesting depend on the client.

```json
{
  "command": "/absolute/path/open-code-review/dist/ocr-acp",
  "args": ["--ocr-binary", "/absolute/path/ocr"],
  "env": {}
}
```

This minimal configuration supports slash commands without a parser model.
For natural language, supply the separate parser configuration described below
to the agent process through flags or its environment. A graphical app may not
inherit your terminal environment. Do not put API keys in CLI arguments or
commit credentials in shared settings. Configure the review model through OCR's
own configuration; the client's model selector does not configure the parser
or OCR. Select your project's working directory in the client and follow its
ACP configuration guide for the enclosing settings structure.

After rebuilding or upgrading, ensure the configured command points to the
new binary, then start a **new OpenCodeReview session**. Existing processes and
old messages do not reload automatically. If the client reuses an older agent process,
restart the agent/client after allowing active work to finish or cancelling it.
Session restoration is not supported; a new session has no pending clarification
state from the previous one.

### Current Conversation Display

- A single **Command:** code block appears above the process entry, showing
  the actual OCR binary and shell-quoted argv. The runner executes argv directly,
  not through a shell. Copying the command requires the working directory shown
  in the details and the appropriate local OCR configuration.
- **OCR progress** is a generic ACP tool entry. Folding support and expansion
  state belong to the client. The title shows a bounded,
  single-line plain-text latest activity while running and the final outcome
  after cleanup.
- Expand the entry for the working directory and plain-text process logs.
  Updates replace the retained tail (up to 32 KiB of log text, with an omission
  notice). Raw logs are not repeated in the conversation body. Individual
  `file_read`/`code_search` log lines are not additional ACP tool executions.
- Findings use separated severity/category headings, a **Location:** citation,
  and language-tagged existing/suggested code blocks. Valid in-root locations
  retain file links; invalid locations remain non-clickable. Findings appear
  only in the final conversation message, without separate finding tool cards
  or navigation buttons. Markdown and link rendering depend on the client version.
- The final result retains partial/failure information, summary, total tokens
  and OCR-reported elapsed time. Tokens are cumulative across model requests,
  not a count of newly generated answer tokens; input/output/cache breakdown
  and a live elapsed-time display are not implemented.

For a faster development check, use `/review --effort low` (at most one review round).
`medium` allows up to two rounds and `high` up to three. A group stops early
when a round adds no new findings. Omitting the flag preserves OCR's
configuration, whose fallback is `medium`; the adapter does not lower it.
Lower effort may miss findings and does not guarantee a proportional speedup.

### Platform Support

| Platform/client | Implementation | Validation/support boundary |
| --- | --- | --- |
| macOS arm64 | Process groups and cancellable stdio | Local automated tests and SDK smoke passed |
| Linux x86_64 | Unix process-group/stdio implementation | ACP CI passed at `aa7d4e7`, including version-probe and post-exit process-group cleanup regressions |
| Windows | Explicit unsupported paths | Job Objects and cancellable stdio are not implemented; do not advertise Windows support |
| Other OS/architectures | No release commitment | No runtime acceptance recorded |
| Zed / VSCode ACP extension | Standard ACP stdio interface | Full manual acceptance confirmed by the author on 2026-09-15 for the tested installations; this does not establish Windows support |
| Paseo / JetBrains / other ACP clients | Standard ACP stdio interface | Not covered by a recorded graphical-client acceptance run |

## CLI Contract

`internal/contract` builds OCR CLI argument vectors and models the JSON the
CLI writes back.

### Intent to arguments

`BuildReviewArgs` and `BuildScanArgs` turn an intent into an argv slice. They
always append `--format json --audience human --color never` last, so the
integration contract holds regardless of what a caller passes in `Extra`.

### Extra flag whitelist

`Extra` is not a free-form passthrough. Every element is validated against a
per-command whitelist derived from the OCR CLI's own flag registrations:

| Command | Forwardable via `Extra` |
| --- | --- |
| `review` | `--effort`, `--no-filter`, `--background`, `--background-file` |
| `scan` | `--batch`, `--no-plan`, `--no-dedup`, `--no-summary`, `--background` |

The lists are asymmetric because the CLI registers different flags on each
command — `--effort` is rejected on `scan`, `--batch` on `review`. Enum flags
(`--effort`, `--batch`) also have their values checked, because the CLI
silently falls back to a default for unknown values rather than erroring.

Everything else is rejected, including the integration flags the adapter owns
(`--format`, `--audience`, `--color`, `--output`, `--repo`, `--rule`,
`--tools`, `--resume`), the runtime overrides (`--model`, `--provider`),
`--preview`, and any bare word that is not a flag.

### Result types

`ReviewResult`, `ScanResult`, `Summary`, `Comment` and `Manifest` mirror the
real `ocr --format json` envelope (`jsonOutput` plus `model.LlmComment`):
`summary` is an object, a finding carries `content`/`start_line`/`end_line`,
and `start_line == end_line == 0` means a file-level comment. Per-comment
`thinking` is deliberately not modeled, so reasoning cannot leak by accident.
`Comment.Severity` and the `status` string are open sets: the observed values
are declared as `Status*` constants, but consumers must tolerate unknown ones.

## Parsing and Clarification

`internal/intent` turns one `session/prompt` text into exactly one of three
outcomes, and never starts a process:

- a runnable `contract.ReviewIntent` / `contract.ScanIntent`;
- a clarification question (one pending request with multiple known fields);
- a rejection with an actionable hint.

Routing is deliberate:

- A leading `/` selects the deterministic parser. `/review`, `/scan` and
  their flags are an exact grammar, so they keep working while the parsing LLM
  is down. Unknown commands (`/foo`) and out-of-scope flags (`--staged`,
  `--repo`, `--format`, `--ocr-binary`) are rejected, never forwarded to
  the model.
- Everything else goes to the LLM through a single `submit_intent` tool call.
  There is no keyword table, regex or template for natural language, and no
  deterministic fallback: a failed, timed-out or unsupported call becomes a
  clarification or a rejection.
- Every proposed value is validated before it becomes argv. Refs are checked
  with `git rev-parse --verify <ref>^{commit}`; paths and extra flags go through
  the phase 3 `BuildReviewArgs` / `BuildScanArgs`, which own the whitelist.
  Adapter-owned flags are absent from the tool schema, so prompt injection
  cannot reach them.

Compatible fields survive successive clarifications; explicitly supplied values
override earlier ones. Switching review type or between review and scan does
not carry incompatible fields forward. Completion or cancellation clears the
pending request. This is short-lived clarification state, not conversation memory.

The parsing timeout defaults to 15s and is cancellable through the context.
It applies before OCR starts. Increasing `--turn-timeout` does not extend this
separate parser deadline; there is no parser-timeout startup flag. On failure,
inspect the reported request phase, elapsed time, HTTP status and provider
response. A phase identifies where the request stopped, not necessarily the
root cause; a timeout alone does not prove a network failure. Use a slash
command to bypass parsing, for example `/review --commit HEAD` to review the
latest commit in the current checkout.

The parsing model is instructed to write `question`, `reason` and `hint` in the
current user's language, honoring an explicit response-language request.
Commands, flags, refs, paths and protocol fields are not translated. This is a
model instruction, not a deterministic translation guarantee: tests cover the
instruction and preservation of localized responses, not every provider's
language adherence. Fixed configuration/validation/timeout messages and report
labels remain English. Anthropic intent parsing disables Thinking to support
the forced `submit_intent` call; OCR's review-model Thinking is unchanged.

### Parsing LLM configuration

The parsing LLM is configured independently from the review LLM, through
`--parser-provider`, `--parser-model` and `--parser-base-url` (or
`OCR_ACP_PARSER_PROVIDER`, `OCR_ACP_PARSER_MODEL`, `OCR_ACP_PARSER_BASE_URL`);
the API key is environment-only (`OCR_ACP_PARSER_API_KEY`). Flags win over
the environment.

Base URL conventions depend on the protocol:

| Protocol | Example base URL | Request endpoint |
| --- | --- | --- |
| `openai` | `https://gateway.example/v1` | `https://gateway.example/v1/chat/completions` |
| `anthropic` | `https://gateway.example` | `https://gateway.example/v1/messages` |

A full URL ending in `/chat/completions` or `/v1/messages`, respectively, is
also accepted without appending that suffix again. An Anthropic base ending
only in `/v1` would produce `/v1/v1/messages`; use the gateway's correct base
or full endpoint. These are placeholder URLs, not provider recommendations.

With no parser configuration, slash commands remain usable and natural
language requests receive an actionable refusal. Partial or invalid parser
configuration fails startup explicitly; it never silently disables the parser.

## Protocol and Resource Boundaries

The adapter uses ACP Go SDK v0.13.5 for JSON-RPC framing, validation and types.
SDK imports stay in `internal/adapter`; `contract`, `intent` and `orchestrator`
remain protocol-independent. This compact boundary combines the protocol,
server and mapping directories proposed by the design, without extra layers.
The connection uses the SDK's base dispatcher so a busy session can reject a
second prompt without the SDK automatically cancelling the first.

- `session/new` requires an existing absolute cwd and announces `review` and
  `scan` through standard `available_commands_update` with argument hints.
  The connection writes the session response before publishing its command
  notification, without requiring a first prompt or a fixed delay. The pinned
  SDK's serialized frame writer preserves this order; clients still own their
  session-registration and native completion UI scheduling. Descriptions include
  examples; argument hints do not provide dynamic branch or path completion.
- Review resolves locations against the nearest Git root; non-Git review is
  refused. Scan resolves locations against the session cwd. Additional roots
  are not advertised; prompts cannot override `--repo`.
- Existing in-root regular files receive Markdown file links in the final message.
  Line ranges are checked against the file; `0/0` means file-level. Missing,
  escaping, invalid and symlink-outside paths retain text without navigation.
- Text blocks are separated by newlines. Local `file:` ResourceLinks may
  select files for `/scan` when no explicit `--path` is present. Links with
  review or explicit scan paths are refused instead of silently changing
  scope. Link-only prompts ask for `/scan`; resend the links with that command.
  Remote URLs, directories, outside files and unsupported content are refused.
  Resources are not fetched or read into LLM context. Prompt content is bounded
  to 128 blocks and 64 KiB, with an 8 KiB URI limit.
- Results retain severity, category, suggestions, statistics and partial-review
  diagnostics. The final message presents the summary before findings, with no
  separate finding tool cards or navigation buttons; `thinking` is never
  modeled or transmitted.
- Cancel is idempotent and clears clarification state. EOF, SIGINT and SIGTERM
  cancel tasks and wait for managed process cleanup. Stalled stdio writes use
  pollable OS pipes with a two-second deadline, rather than relying on context
  cancellation to interrupt synchronous writes.
- Timeout sends a visible retry message and returns `end_turn` with
  `_meta.ocr.kind=timed_out`. ACP v1 has no dedicated timeout stop reason.
- Greetings, unsupported requests and invalid options send explanatory text
  and finish with `end_turn` plus `_meta.ocr.kind=rejected`, without starting
  OCR. They do not use the `refusal` stop reason: Zed removes the user prompt
  and subsequent response on that reason, hiding the guidance.

HTTP transport, persistent sessions, additional directories, client-managed terminal APIs,
file mutation and image/audio/embedded-resource prompts are not supported.

### Official Python Client Smoke Test

Run from the repository root (requires `uv`, Git and Go):

```bash
make -C acp build
uv run --no-project --managed-python --python 3.12.13 --with-requirements acp/testdata/python-smoke-requirements.txt acp/testdata/python-client-smoke.py
```

Python is pinned to 3.12.13. The generated requirements file pins all SDK
dependencies and records distribution hashes; its header contains the
regeneration command. Update the `.in` file when changing the SDK version.
The script refuses optimized Python execution because its checks use assertions.
CI keys the uv cache on the Python version and requirements file and allows
five minutes for interpreter provisioning, dependency installation and testing.

The official Python ACP SDK starts the independent adapter over OS stdio
pipes. The test verifies initialization, session creation, command discovery,
review and scan findings from a nested directory, severity/suggestions,
ResourceLink target selection, Command-before-progress ordering, live activity
before completion, plain-text logs confined to tool details, repeated
cancellation and graceful EOF exit. Go wire tests separately assert the
session-response-before-command-notification ordering.
OCR itself is a deterministic subprocess fixture: this proves cross-SDK
protocol interoperability, not real OCR/LLM output quality or graphical-client
rendering. This smoke test passed in
[Linux ACP CI run 34986645165](https://github.com/alibaba/open-code-review/actions/runs/34986645165)
at `aa7d4e7`, alongside the lifecycle regressions and govulncheck. Automated
SDK evidence is separate from the author-confirmed Zed and VSCode ACP client
acceptance recorded above.

`internal/llmresolve` deliberately does not read the OCR config file,
`OCR_LLM_*`/`ANTHROPIC_*`, shell rc files or `api_key_cmd`. The review LLM
stays owned by the OCR CLI, so an OCR upgrade never forces the adapter to copy
or drift with OCR's provider system. Only `anthropic` and `openai` are
supported; `openai-responses` and `anthropic-bedrock` fail with an
actionable error instead of silently degrading. `Endpoint.Summary()` names
the source, protocol, model and URL for reproducible logs, and never prints the
token.

## Mock OCR

The `testdata/mock-ocr` program simulates OCR CLI behavior for testing without
real LLM calls.

### Build Mock

```bash
go build -o testdata/mock-ocr/mock-ocr ./testdata/mock-ocr/
```

### Scenarios

`-scenario` defaults to `success-review`, so running the binary bare works.

All scenarios emit the real CLI JSON envelope (`status`, `message`, `summary`
object, `comments` with `content`/`start_line`/`end_line`, `manifest`), not the
adapter's DTO.

- `success-review`: Normal review with findings (default)
- `success-scan`: Normal scan with findings (status `success`, no manifest)
- `partial`: Partial results with a failed coverage entry (timeout simulation)
- `empty-comments`: No findings; emits `"comments":[]`
- `stderr-pollution`: Noise and a second JSON document on stderr alongside a
  valid stdout document
- `invalid-json`: Malformed JSON output
- `non-zero-exit`: `status: failed`, `"comments":null`, a failed manifest and
  exit code 1
- `block-for-cancel`: Prints `READY` to stderr, blocks, then on SIGINT/SIGTERM
  emits a failed document whose `manifest.run_failure.classification` and
  `coverage.failed[].classification` are `cancelled`, and exits 1 to mirror the
  real CLI (which has no dedicated cancellation exit code)
- `spawn-child`: forks a blocking child in the same process group, writes its
  PID to `-child-pid-file`, and waits until that child is ready. It is used to
  verify cancellation reaps managed descendants without timing-based tests

`-delay` pauses before producing output. If a signal arrives during the delay
the mock exits 130 without writing a result.

### Example

```bash
# Normal review
./testdata/mock-ocr/mock-ocr -scenario success-review

# With delay
./testdata/mock-ocr/mock-ocr -scenario success-review -delay 500ms

# Test cancellation
./testdata/mock-ocr/mock-ocr -scenario block-for-cancel
```

### Contract tests

`internal/contract/mock_contract_test.go` builds the mock and decodes its real
stdout into the contract types, asserting the decoded field values (content,
line numbers, summary, manifest classifications) rather than just counts.

`internal/contract/real_cli_test.go` adds two more layers:

- deterministic fixtures of the real CLI shape (summary object, unknown
  fields, `thinking`) that fail if the DTO drifts from the documented output;
- `TestRealOCRBinaryScanMultiplePaths`, an opt-in integration test. Set
  `OCR_BINARY` to run the adapter's own argv against a real binary:
  `OCR_BINARY=../dist/opencodereview make test` from `acp/`. It pins the
  `--path` comma-joining rule end to end (a repeated `--path` would silently
  scan only the last path).

## Dependencies

- Go 1.23+ to build this module (its `go` directive), but the `staticcheck`
  version CI pins (0.8.x) needs a Go >= 1.26 toolchain, so run `make check` on
  Go 1.26+ or expect its staticcheck step to fail fast rather than pass
  vacuously. The parent OCR module declares `go 1.25.5`; aligning the three is
  still pending.
- OCR CLI (external dependency, not imported as library)
- ACP Go SDK v0.13.5

## License

Apache-2.0 (same as parent OCR project)
