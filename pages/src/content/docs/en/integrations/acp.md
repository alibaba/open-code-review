---
title: ACP
sidebar:
  order: 6
---

Run OCR reviews and scans from an ACP-compatible client conversation. The `ocr-acp`
adapter connects the editor to your local OCR CLI through Agent Client
Protocol (ACP), showing progress, findings and file locations in the conversation.
It reports suggested fixes; it does not edit your files.

ACP is an open protocol for connecting editors to agents. The client starts the
adapter as a separate local process and exchanges JSON-RPC messages over stdio.
The adapter validates each request, runs `ocr review` or `ocr scan`, and converts
OCR's structured output into client updates. OCR's review engine and model
configuration remain independent of the adapter.

## Prerequisites {#prerequisites}

- Install the [OCR CLI](../../installation/) and configure its
  [review model](../../configuration/). Check it with `ocr llm test`.
- Use a source checkout containing the `acp/` directory, Git and Go 1.23+
  to build the adapter. Building the parent OCR CLI requires Go 1.25.5+.
- Use a client that supports ACP v1 over stdio and custom agent commands. macOS arm64 has local
  automated coverage. Linux ACP CI passed at `aa7d4e7`, including process-cleanup
  regressions, race-enabled coverage and the official Python SDK smoke test
  ([verification run](https://github.com/alibaba/open-code-review/actions/runs/34986645165)).
  Windows is currently unsupported.

The adapter is built separately from the OCR CLI. The instructions below use
a local build, not a preinstalled ACP binary from the OCR npm package.

## Build and connect your client {#build-and-connect-your-client}

From the repository root:

```bash
make -C acp build
command -v ocr
```

The build creates `dist/ocr-acp`. Use the absolute path printed for `ocr`
as the `--ocr-binary` value below.

Register a custom ACP agent in your client. Set its name to `OpenCodeReview`,
choose ACP v1 stdio transport, and configure this launch command:

```bash
/absolute/path/open-code-review/dist/ocr-acp --ocr-binary /absolute/path/ocr
```

Use absolute paths for both executables and select your project's working
directory. Configuration field names vary by client; use its ACP setup guide.
Start a new OpenCodeReview conversation. Slash commands work without a parsing
model; OCR still needs its own review model.

## Run a review or scan {#run-a-review-or-scan}

Send these commands in the OpenCodeReview conversation:

| Request | Command |
| --- | --- |
| Review staged, unstaged and untracked changes | `/review` |
| Run at most one review round | `/review --effort low` |
| Review the latest commit in the current checkout | `/review --commit HEAD` |
| Compare two existing refs | `/review --from main --to HEAD` |
| Scan a directory | `/scan --path internal/agent` |
| Scan multiple paths | `/scan --path internal/agent,internal/config` |

Replace example refs and paths with ones in your project. Review requires a
Git repository and resolves findings against its root; scan uses the session's
working directory. `--effort low` allows at most one review round, `medium` two
and `high` three. A group stops early when a round adds no new findings.
Omitting it preserves OCR's configuration. Fewer rounds may miss
findings; lower effort is an optional speed/quality tradeoff.

Type `/` to discover the available commands and argument hints in clients that
show ACP command suggestions. Dynamic branch and path completion is not provided.
If the client attaches local file links, send them with `/scan` and omit
`--path`. File links cannot be combined with `/review` or explicit scan paths;
unsupported or ambiguous links are rejected instead of expanding the scan.

The adapter accepts a defined set of CLI options, not arbitrary passthrough:

- Review: `--commit`, `--from`, `--to`, `--effort`, `--no-filter`,
  `--background`, `--background-file`.
- Scan: `--path`, `--batch`, `--no-plan`, `--no-dedup`, `--no-summary`,
  `--background`.

Use `/review` for the whole change set; `--staged` is not supported. Output
options such as `--format` and `--audience`, and overrides such as `--repo`,
`--model` and `--provider`, are not accepted in conversation commands.
Configure the review model through OCR itself.

## Enable natural-language requests {#enable-natural-language-requests}

A parsing model converts natural-language requests into review/scan commands
or asks for clarification. It is configured separately from OCR's review
model; the client's own model selection does not configure either one.

Add the following environment settings to the agent's `env` object, replacing
the model and key placeholders with your provider's values:

```json
{
  "OCR_ACP_PARSER_PROVIDER": "openai",
  "OCR_ACP_PARSER_MODEL": "YOUR_TOOL_CALLING_MODEL",
  "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
}
```

This example uses the OpenAI-compatible Chat Completions protocol. Use
`anthropic` for the Anthropic Messages protocol. For a compatible gateway,
also set `OCR_ACP_PARSER_BASE_URL` to its API base URL. Keep credentials in
local settings or supply them through the agent process environment; do not
commit them in shared project settings. A graphical app may not inherit the
environment exported in your terminal.

Base URL conventions differ by protocol:

| Protocol | Example base URL | Request endpoint |
| --- | --- | --- |
| `openai` | `https://gateway.example/v1` | `https://gateway.example/v1/chat/completions` |
| `anthropic` | `https://gateway.example` | `https://gateway.example/v1/messages` |

A full endpoint ending in `/chat/completions` or `/v1/messages`, respectively,
is also accepted. For Anthropic, a base ending only in `/v1` would produce
`/v1/v1/messages`; use the correct base or full endpoint for your gateway.
These URLs are placeholders.

| Environment variable | Startup flag | Purpose |
| --- | --- | --- |
| `OCR_ACP_PARSER_PROVIDER` | `--parser-provider` | `openai` or `anthropic` |
| `OCR_ACP_PARSER_MODEL` | `--parser-model` | Model supporting tool calls |
| `OCR_ACP_PARSER_BASE_URL` | `--parser-base-url` | Optional API base URL override |
| `OCR_ACP_PARSER_API_KEY` | None | Parsing API key; environment only |

Startup flags take precedence over matching environment variables. The parser
does not read OCR's configuration or `OCR_LLM_*` variables. Its supported
protocols are `openai` and `anthropic`; `openai-responses` and
`anthropic-bedrock` are not supported here.

Anthropic parsing requests explicitly disable Thinking to allow the required
`submit_intent` tool call. This does not change OCR's review-model Thinking.

Start a new conversation after configuring the parser, then try:

```text
Review the changes in my working directory.
Review the latest commit in the current checkout.
Scan internal/agent for potential bugs.
```

When the request is ambiguous, answer the clarification before execution.
The session retains one pending request with multiple compatible fields across
clarifications; new explicit values override previous ones. Switching review
type or between review and scan does not carry incompatible fields forward.
Completion or cancellation clears the request; it is not conversation memory.
The model is instructed to match your language for questions and guidance;
fixed validation messages and report labels remain English. If the parsing
model is unavailable, slash commands still work. An incomplete parser
configuration causes an explicit startup error.

## Read results and cancel work {#read-results-and-cancel-work}

The **Command:** block shows the command being run. **OCR progress** contains
the working directory and a bounded tail of plain-text logs; expand it for
details. Log updates stay in this entry instead of repeating in the conversation.
The client controls whether the entry starts expanded or collapsed.

Findings appear once in the final message, with severity, category, explanation
and suggested code; no separate finding cards or navigation buttons are emitted.
Existing files with valid locations can be opened from links in the result; missing files,
out-of-range lines and paths outside the working root retain their text without
navigation. Rendering and click behavior depend on the client version.

The final result includes available summary, partial-failure details, total
tokens and OCR-reported elapsed time. Token totals cover all model requests;
they are not a count of newly generated answer tokens.

Use the client's cancel control to stop a running request. The adapter cancels
the work and waits for managed process cleanup. To cap the whole turn, including
parsing, add `"--turn-timeout", "10m"` to the agent's `args` array. The default
is `0`, which disables the whole-turn timeout; parsing has its own 15-second
default limit. On timeout, the adapter reports a retry message.

## Troubleshooting {#troubleshooting}

| Symptom | What to check |
| --- | --- |
| Agent does not start | Check both absolute executable paths and execute permissions. Run `ocr llm test` for OCR configuration; remove incomplete parser settings or supply all required values. |
| Slash commands work, natural language does not | Configure the separate `OCR_ACP_PARSER_*` environment. Check provider, model, key and gateway URL. |
| Natural-language parsing times out before OCR starts | Parsing has a separate default 15-second limit; increasing `--turn-timeout` does not extend it, and there is no parser-timeout startup flag. Inspect the reported request phase, elapsed time, HTTP status and provider response, or use `/review` or `/scan` to bypass parsing. A timeout alone does not establish a network failure. |
| Command or flag is rejected | Use the supported options above. `/review --path` should be `/scan --path`; use existing Git refs for commit/range review. |
| Review fails or returns partial results | Expand OCR progress for diagnostics and read the final failure details. Check OCR's model configuration and provider availability. |
| A result location is not clickable | Check that the file exists inside the operation's root and that its line range is valid. Text-only fallback is intentional. |
| A task takes too long | Cancel it, inspect the progress details, or set a whole-turn timeout. For review, `--effort low` reduces the number of rounds. |
| Old behavior remains after rebuilding | Check the configured binary path and start a new agent conversation. Restart the agent/client if it keeps reusing the old process. |

Inspect your client's ACP logs and the adapter's stderr diagnostics. Remove
credentials and private source details before sharing logs.

## Upgrade and support boundaries {#upgrade-and-support-boundaries}

Update your source checkout, run `make -C acp build` again, and start a new
OpenCodeReview conversation. Active processes and old messages do not reload.
Session restoration is not supported; a new conversation starts without pending
clarification state.

The adapter currently uses local stdio transport. HTTP transport, additional
workspace roots, automatic file edits and image/audio prompts are not supported.
Other ACP clients require their own compatibility checks; successful protocol
tests do not establish every client's interface behavior.

## See also {#see-also}

- [Configuration](../../configuration/) — OCR's review model and provider settings.
- [CLI Reference](../../cli-reference/) — review and scan option details.
- [ACP Introduction](https://agentclientprotocol.com/get-started/introduction) — official introduction to Agent Client Protocol.
