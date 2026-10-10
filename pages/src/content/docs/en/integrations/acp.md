---
title: ACP
sidebar:
  order: 6
---

**Experimental feature:** The ACP adapter is currently unstable and is only supported when compiled locally from source. GitHub Releases and the OCR npm package do not provide a prebuilt ACP binary. Update the source and rebuild when upgrading.

## Prerequisites {#prerequisites}

- Install the [OCR CLI](../../installation/), configure the [review model](../../configuration/), and check the connection with `ocr llm test`.
- Prepare a source checkout containing `acp/`, Git, and Go 1.23+ to build the adapter. Building the OCR CLI from source requires Go 1.25.5+.
- Use a client that supports ACP v1 stdio and custom agent launch commands. macOS and Linux have automated test coverage; Windows and riscv64 require separate verification.

The adapter and OCR CLI are built separately. The instructions below use a local build and do not depend on a preinstalled ACP binary from the OCR npm package.

## Build and connect your client {#build-and-connect-your-client}

Run from the repository root:

```bash
make -C acp build
command -v ocr
```

The build output is `dist/ocr-acp`. Put the absolute path printed by `command -v ocr` in `--ocr-binary` below.

Add the following configuration to the client's Agent settings. Replace every `/absolute/path` placeholder and the parser model values with your own settings:

```json
{
  "agent_servers": {
    "OpenCodeReview": {
      "type": "custom",
      "command": "/absolute/path/open-code-review/dist/ocr-acp",
      "args": [
        "--ocr-binary",
        "/absolute/path/ocr"
      ],
      "env": {
        "OCR_ACP_PARSER_PROVIDER": "anthropic",
        "OCR_ACP_PARSER_MODEL": "deepseek-flash",
        "OCR_ACP_PARSER_BASE_URL": "https://your-gateway.example",
        "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
      }
    }
  }
}
```

Field names vary by client; consult its ACP configuration guide.

## Run a review or scan {#run-a-review-or-scan}

Send the following in an OpenCodeReview conversation:

| Request | Command |
| --- | --- |
| Review staged, unstaged, and untracked changes | `/review` |
| Run at most one review round | `/review --effort low` |
| Review the latest checked-out commit | `/review --commit HEAD` |
| Compare two existing refs | `/review --from main --to HEAD` |
| Scan a directory | `/scan --path internal/agent` |
| Scan multiple paths | `/scan --path internal/agent,internal/config` |

Review requires the working directory to be a Git repository and resolves result paths from its root. Scan uses the session working directory as its root.

`--effort low` runs at most one review round, `medium` at most two, and `high` at most three. A review can stop early when a round finds nothing new. Omitting the option uses OCR's configured value. Fewer rounds can miss findings, so choose the tradeoff you need.

In clients that support ACP command suggestions, type `/` to see available commands and options.

If the client attaches local file links, send them with `/scan` and omit `--path`. File links cannot be combined with `/review` or an explicit scan path; unsupported or ambiguous links are rejected instead of expanding the scan scope.

The adapter accepts only the documented CLI options:

- Review: `--commit`, `--from`, `--to`, `--effort`, `--no-filter`, `--background`, `--background-file`.
- Scan: `--path`, `--batch`, `--no-plan`, `--no-dedup`, `--no-summary`, `--background`.

`/review` does not support `--staged`. Conversation commands also reject output options such as `--format` and `--audience`, and override options such as `--repo`, `--model`, and `--provider`. Configure the review model through OCR itself.

## Enable natural-language requests {#enable-natural-language-requests}

The parsing model converts natural-language requests into review or scan commands, or asks for clarification when information is missing. It is configured separately from OCR's review model; the client's own model selection does not configure either model.

Add these settings to the Agent's `env` object and replace the model and key placeholders:

```json
{
  "OCR_ACP_PARSER_PROVIDER": "openai",
  "OCR_ACP_PARSER_MODEL": "YOUR_TOOL_CALLING_MODEL",
  "OCR_ACP_PARSER_BASE_URL": "https://your-gateway.example",
  "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
}
```

This example uses an OpenAI-compatible Chat Completions protocol. Set the provider to `anthropic` for the Anthropic Messages protocol. For a compatible gateway, set `OCR_ACP_PARSER_BASE_URL` to its API base URL.

Base URL path rules depend on the protocol:

| Protocol | `OCR_ACP_PARSER_BASE_URL` example | Actual request URL |
| --- | --- | --- |
| `openai` | `https://gateway.example/v1` | `https://gateway.example/v1/chat/completions` |
| `anthropic` | `https://gateway.example` | `https://gateway.example/v1/messages` |

You may also provide a complete URL ending in `/chat/completions` or `/v1/messages`; the adapter will not append the suffix again. For Anthropic, a base URL ending in `/v1` produces `/v1/v1/messages`; use the path required by the gateway.

| Environment variable | Startup flag | Purpose |
| --- | --- | --- |
| `OCR_ACP_PARSER_PROVIDER` | `--parser-provider` | `openai` or `anthropic` |
| `OCR_ACP_PARSER_MODEL` | `--parser-model` | Model with tool-calling support |
| `OCR_ACP_PARSER_BASE_URL` | `--parser-base-url` | Optional API base URL override |
| `OCR_ACP_PARSER_API_KEY` | None | Parser model key, supplied only through the environment |

The supported parser protocols are `openai` and `anthropic`; `openai-responses` and `anthropic-bedrock` are not supported.

Anthropic parser requests explicitly disable Thinking so that the required `submit_intent` tool call works. This does not change Thinking settings for the OCR review model.

After configuring the parser, start a new conversation and try:

```text
Review the changes in my current workspace.
Review the latest checked-out commit.
Scan internal/agent for potential issues.
```

When a request is ambiguous, answer the clarification before execution. The model is instructed to ask and explain in your language; fixed validation messages and report labels remain in English. Slash commands continue to work when the parser is unavailable. Incomplete parser configuration produces a clear startup error.

The phrase “latest checked-out commit” maps to `/review --commit HEAD`. A session keeps one pending clarification request. Compatible known fields can accumulate across clarifications, and an explicit new value replaces the old one; fields from another review type are not mixed in. Completion or cancellation clears the state, which is not restored across sessions.

## Read results and cancel work {#read-results-and-cancel-work}

Progress uses the OCR CLI's existing logs and final JSON. It shows the latest reported activity and total elapsed time, without inferring file-group stages, stage durations, or model-wait time.

A single **OCR review** or **OCR scan** entry shows progress and locally measured runtime. Expand it to see **Command:**, the working directory, and bounded plain-text logs. Diagnostics remain in the details and do not replace the progress title; commands and logs are not repeated in the conversation body. The client controls the initial expansion state.

The final report first shows completion status, file and finding counts, and the severity distribution. Each finding has a numbered heading with severity and file location, followed by its category, explanation, and a language-tagged code block. Suggested fixes are clearly marked as unapplied. Findings appear only once. Valid locations link to files; missing files, invalid line numbers, and paths outside the working root remain plain text. Exact rendering and click behavior depend on the client version.

Partial-failure details and budget-limit notices are retained. The report footer shows available cumulative tokens and OCR-reported runtime. Execution details also show OCR's tool-call count and input, output, and cached tokens when available. Missing data is omitted. The live timer measures local execution and may differ from OCR's reported runtime; token totals include all model requests, not only newly generated answer tokens.

Use the client's cancel control to stop a request. The adapter cancels the task and waits for managed processes to clean up. To limit the whole turn, including parsing, add `"--turn-timeout", "10m"` to the Agent's `args` array. The default is `0` (no whole-turn limit); parser requests have their own 15-second default. After a timeout the adapter asks you to retry.

## Troubleshooting {#troubleshooting}

| Symptom | Check |
| --- | --- |
| Agent cannot start | Check both executable paths and permissions. Run `ocr llm test` for OCR configuration; remove incomplete parser settings or provide all required values. |
| Slash commands work but natural language does not | Configure the separate `OCR_ACP_PARSER_*` environment variables and check the provider, model, key, and gateway URL. |
| Natural-language parsing times out | Parsing has a separate 15-second default limit and OCR has not started yet. Use the diagnostic request phase, HTTP status, and provider response to locate the cause; increasing `--turn-timeout` does not extend the parser limit. Retry or use `/review` and `/scan` to bypass parsing. |
| Review fails or returns partial results | Expand the OCR review/scan details for diagnostics and read the final failure message. Check OCR model configuration and provider availability. |
| A task takes too long | Cancel it, inspect progress details, or set a whole-turn timeout. For review, `--effort low` reduces the number of rounds. |
| Rebuilding still shows old behavior | Check the configured binary path and start a new OpenCodeReview conversation. Restart the Agent or client if it reuses an old process. |

For connection errors, inspect the client's ACP logs and the adapter's stderr diagnostics. Remove keys and private source code before sharing logs.

## Upgrade and support boundaries {#upgrade-and-support-boundaries}

After updating the source, run `make -C acp build` again and start a new OpenCodeReview conversation. Running processes and old messages do not load the update. Session restoration is not supported; a new conversation does not retain an earlier clarification request.

The adapter currently uses local stdio transport. HTTP, multiple workspace roots, automatic file edits, and image or audio requests are not supported. Other ACP clients require separate compatibility checks; passing protocol tests does not verify every client's interface behavior.

## See also {#see-also}

- [Configuration](../../configuration/) — OCR review model and provider settings.
- [CLI Reference](../../cli-reference/) — details for review and scan options.
- [ACP Introduction](https://agentclientprotocol.com/get-started/introduction) — the official Agent Client Protocol introduction.
