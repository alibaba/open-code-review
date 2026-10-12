# Copilot SDK feasibility for OCR reviews

This note records the architecture check and experimental implementation for
[discussion #1271](https://github.com/alibaba/open-code-review/discussions/1271).
It records one completed, signed-in Copilot-backed review of a one-line fixture;
that result does not establish reliability for larger reviews.

## Checked state

- OCR base: `bccbc15f785269400735d5255540c231e6c02b6d` (`origin/main` at the
  start of this check). The isolated worktree had no pre-existing patch.
- SDK inspected and used: `github.com/github/copilot-sdk/go` `v1.0.14`.
- The first local probes used CLI `1.0.88` under the ignored `temp/` directory
  and made no authenticated Copilot request. The initial probe received **2**
  local model calls; the adapter test received exactly **1** per OCR request.
  The user subsequently installed CLI `1.0.88`, signed in, and authorized a
  real connectivity test and bounded reviews. Host-agent usage: unavailable.
- The local mock tests used CLI `1.0.88`, SDK `v1.0.14`, and an `httptest`
  Chat Completions server. The product dependency is now in `go.mod`.
- `make check`, `make test` (with race detection), and `make coverage` passed;
  total coverage was **90.6%** on the draft snapshot. Copilot CLI sign-in is separate from `gh auth`.

## Behavior that an integration must preserve

| Obligation and source | Scenario and control | Evidence and judgment | Still to check |
| --- | --- | --- | --- |
| Each `CompletionsWithCtx` call returns the model's text and tool requests to OCR; OCR executes the tools and records their results. See `internal/llm/client.go` (`LLMClient`) and `internal/llmloop/loop.go` (`Runner`). | **C1:** A Copilot session exposes its default tools and receives an OCR review prompt. **Control:** A text-only request returns a final assistant message. | GitHub documents that Copilot CLI owns the tool loop, executes tools, and may make several model calls per `session.send`. A direct `SendAndWait` adapter would skip OCR's per-round tool dispatch and budgets. The adapter locally returned at an OCR tool boundary, and a one-line signed-in review completed with one correct `code_comment`. **Tool routing and small-review completion confirmed.** | Repeated completion on larger reviews. |
| OCR passes complete message history, including assistant tool calls and tool results, on every round; its compression may replace that history. See `internal/llmloop/loop.go` (`addNextMessage`) and `internal/llmloop/compression.go` (`runCompression`). | **C2:** A review makes one tool call, OCR returns its result, then OCR compresses the transcript. **Control:** The same exchange without compression. | Ordinary tool rounds now keep one SDK session: a local mock received OCR's result as a native `tool` message on the second model request. When OCR replaces its history, the adapter starts a new session with the authoritative compressed transcript. **Native continuation confirmed locally; compression fallback quality unverified.** | Compare completed reviews and compression behavior against the signed-in service. |
| OCR review tools and their side effects stay under OCR's control; Copilot must not gain unrelated file or shell tools. See OCR's tool definitions and dispatch in `internal/llmloop/loop.go`. | **C3:** A model asks for a built-in shell or write tool. **Control:** It asks for an explicitly allowed OCR tool. | The adapter uses `ModeEmpty` and an explicit `AvailableTools` list. OCR supplies results through the SDK's pending-tool RPC, so OCR remains the sole tool executor. A local mock verified a custom tool whose name collides with a built-in name, and the prior live run routed 87 tool calls through OCR. **OCR routing confirmed locally; built-in denial remains untested live.** | Verify denied built-ins, cancellation, and error handling against the real service. |
| OCR can change the allowed tools for a request, including a final grace round that exposes only `code_comment` and `task_done`. See `internal/llmloop/loop.go` (`runGraceRound`). | **C4:** A review reaches the grace round after normal tools were available. **Control:** The same request using a fresh session with only grace tools. | The adapter keeps a native session while tools are unchanged, then restarts with OCR's restricted tool list when the grace round begins. A local mock saw only the grace tool on that model request. **Per-request tool restriction confirmed locally.** | Verify against a real signed-in Copilot account. |

GitHub's [agent-loop documentation](https://docs.github.com/en/copilot/how-tos/copilot-sdk/features/agent-loop)
defines who owns each turn. Its [authentication documentation](https://docs.github.com/en/copilot/how-tos/copilot-sdk/auth/authenticate)
provides supported signed-in-user and OAuth paths. The
[Go SDK](https://github.com/github/copilot-sdk/tree/main/go) exposes the
`ModeEmpty`, `AvailableTools`, declaration-only `Tool`, and pending-tool APIs
described above.

GitHub also publishes a
[manual tool resume sample](https://github.com/github/copilot-sdk/blob/main/go/samples/manual_tool_resume/main.go)
that responds to a pending tool call after resuming a session.

## Local probe result

The ignored probe used SDK `v1.0.14`, CLI `1.0.88`, `ModeEmpty`, one
declaration-only `lookup_probe` tool, and an `httptest` Chat Completions server.
The mock returned a tool call on its first request and final text on its
second. The probe waited for `external_tool.requested`, returned `READY`
through `HandlePendingToolCall`, and observed:

```text
pending tool: lookup_probe call-probe-1
first request tools: [lookup_probe only]
final: probe complete
model calls: 2
```

This establishes that the SDK can pause at a tool boundary and resume after
the caller supplies a result. A second local probe aborted at the first tool
boundary and observed exactly one model call. The current repository tests
also check two OCR calls in the same SDK session: the first requests
`file_read`, OCR supplies its result, and the second model request contains a
native `tool` message with that result. A separate test changes the tool
allowlist for a grace round and checks that only `task_done` is offered. A
name-collision test verifies that an OCR tool can replace a CLI built-in tool
without the CLI executing the built-in. These checks do not validate review
quality or the real Copilot service.

## Current decision

Do not expose a `copilot` model provider by wrapping `session.SendAndWait`.
It would report the final output of Copilot's agent loop where OCR expects one
model response, changing tool execution, review budgets, and trace ownership.

The adapter now keeps one `ModeEmpty` SDK session across ordinary OCR tool
rounds. Only OCR's requested tools are declared. The CLI pauses at a pending
custom-tool call; OCR executes it through its existing dispatcher and the next
OCR request returns the recorded result through `HandlePendingToolCall`. The
CLI then continues with native assistant/tool history. OCR's round, timeout,
aggregate token, and context gates remain in the Runner. The Runner closes the
SDK session when the review subtask ends.

When OCR changes the transcript through compression or changes the tool list
for its grace round, the adapter closes that session and starts a new one with
OCR's authoritative transcript. This fallback still serializes history as
JSON in a user message; quality at those boundaries remains unverified. The
SDK's experimental pending-tool RPC is now part of the implementation and
must be pinned and regression-tested. OCR's per-request `max_tokens` value has
no verified hard-equivalent on the signed-in Copilot path. A local mock did not
receive `max_tokens` or `max_completion_tokens` when a session model-capability
override was tried, so that override was not kept. SDK usage events provide
token counts when available; otherwise the adapter estimates usage. The
provider remains experimental because only a one-line signed-in review has
completed; larger reviews and compression quality remain unverified.

The first real-account `ocr llm test` attempt stopped before a session or model
request: the unbundled Go SDK runtime did not discover the installed CLI from
`PATH`. OCR now resolves an explicit CLI path, `COPILOT_CLI_PATH`, or `PATH` and
passes the result to the SDK. The installed CLI then passed the local mock
integration tests. A second, explicitly authorized `ocr llm test` with the
signed-in Copilot CLI 1.0.88 and model `auto` returned a text response and
`Connection test successful` in about 11 seconds. The command did not report
provider request or token counts.

Parallel tool calls were returned together in a local mock test, and
cancellation returned promptly to OCR; cancellation of the CLI's upstream HTTP
request was not established. The live connectivity check did not exercise OCR
tool calls. The review attempts below did exercise tool routing and memory
compression, but did not reach the final grace round or a completed review.

An authorized full-patch review attempt used low effort, one concurrent task,
a 30-second per-request timeout, and a 20,000-token aggregate budget. OCR's
grouping request used 804 reported tokens, then its conservative pre-dispatch
estimate for the single six-file group was 173,616 tokens. The budget gate
correctly skipped all six files before any review tool call, and the run exited
failed with `budget_exceeded=true`. This is not evidence of a completed review
or of a Copilot tool-round failure; a higher budget or narrower scope requires
a separate, explicitly bounded run.

The user increased the aggregate allowance to 500,000 OCR-reported tokens.
Further bounded attempts used one concurrent task and low effort:

| Attempt | Limits | OCR-reported tokens | OCR tool calls | Outcome |
| --- | --- | ---: | ---: | --- |
| Full patch | 500,000 aggregate, 8,000 per-group prompt ceiling | 726 | 0 | The six-file prompt exceeded the per-group ceiling before dispatch. |
| Full patch | 500,000 aggregate, 32,000 per-group ceiling, 30-second request and 1-minute task timeout | 13,665 | 5 | The task timed out after the first tool round. |
| Full patch | 500,000 aggregate, 32,000 per-group ceiling, 90-second request and 5-minute task timeout | 41,427 | 11 | The second model request timed out at 90 seconds. |
| Full patch | 440,000 aggregate, 32,000 per-group ceiling, 180-second request and 10-minute task timeout | 414,611 | 87 | Fourteen model/tool rounds completed; the fifteenth hit the group deadline. All six files remained failed(timeout), with no review comments. |

Those attempts together used 471,233 OCR-reported tokens, including the initial
20,000-budget attempt. A later one-line smoke review first stopped at OCR's
pre-dispatch gate because its 21,892-token group estimate exceeded a 20,000-token
run limit; it made no Copilot model request. With a 25,000-token limit, the same
review completed in 44 seconds, used 9,158 OCR-reported tokens, and produced
one `code_comment` correctly identifying `Add(a, b)` changing from `a + b` to
`a - b`. Its run manifest marked the only selected file complete with no failed
items. A pre-commit `ocr review --audience agent --background ...` attempt on
the draft snapshot used a 19,000-token limit and consumed 851 tokens for
grouping. Its first group estimate was 47,208 tokens, so OCR stopped dispatch
before reviewing any of the nine selected production files. This is an
incomplete automated review, not a zero-finding pass. OCR delegation supplied
file selection and rules for a separate host-agent self-review of all nine
production files; tests, dependencies, and documentation were checked separately.
The combined OCR-reported usage is 481,242 tokens. These counts may
include OCR estimates rather than actual Copilot billing or allowance units.
The live runs establish that the SDK bridge can use the signed-in account and
return tool calls for OCR to execute. The small fixture does **not** establish
reliable completion of a larger OCR review.
The single semantic group repeatedly searched and read code without calling
`task_done` or submitting a finding before its deadline. A larger scoped review
and an explicit quality comparison are needed before treating the provider as
production-ready.

## Timeout diagnosis after the bounded runs

| Obligation and source | Scenario and control | Evidence / counterevidence | Judgment | Required check status / unknowns |
| --- | --- | --- | --- | --- |
| A completed review group calls `task_done`; otherwise OCR records incomplete coverage (`internal/llmloop/loop.go`, `internal/agent/agent.go`). | **B1:** Review the six-file workspace group with Copilot `auto`. **Control:** The local mock adapter returns a scripted `task_done` when asked. | The live group returned 14 model responses containing 87 successful OCR tool calls, all searches or reads. There was no `code_comment` or `task_done`. Its fifteenth request met the ten-minute group deadline. A later one-line fixture completed with one correct `code_comment`, showing that the signed-in route can finish a small group. | **Small-review completion confirmed; cause of the larger review's non-convergence unresolved.** | A bounded larger review and a comparison against another provider remain unrun. |
| An explicit review `--max-tools 50` should set the tool-round ceiling to 50; zero uses the template default (`cmd/opencodereview/shared_flags.go`). | **B2:** Load the review template with overrides 0, 50, and 150. | The review template defaults to 100, and `loadCommonContext` applied an override only when it exceeded 100. A new local test reproduces this mismatch; the implementation now applies any positive override. `make check`, `make test`, and `make coverage` pass at 90.9%. The live run stopped after only 14 rounds, so this defect did not cause its timeout. | **Confirmed and fixed independent CLI defect.** | No further live model call was needed. |
| A replayed conversation must let the model use prior tool results and finish the review (`internal/llm/copilot_client.go`, `internal/llmloop/compression.go`). | **B3:** Inspect the live sequence across memory compression. **Control:** The adapter's local mock checks that tool-call and tool-result fields survive JSON serialization. | OCR compressed the history repeatedly, retaining a summary in the user prompt. The live model repeated the exact `copilot\|GitHub Copilot` search eight times and `session.On\|SessionEvent\|unsubscribe` five times; these regex-like strings lacked `use_perl_regexp: true`, so OCR's literal `git grep -F` could not match them. Compression summaries repeatedly said the review remained incomplete. At the time of that run, the adapter recreated a fresh SDK session each call and presented prior roles as JSON text, but the trace cannot isolate that choice from the model's tool selection or OCR's normal compression. | **Repeated unproductive calls confirmed; adapter-specific causality unresolved.** | Compare native session continuation with the JSON replay on the same fixed review case before claiming a repair. |

This diagnosis used the existing session JSONL and local repository code only.
It made zero additional authenticated Copilot requests. The first two review
attempts had 0 tool calls; later attempts had 5, 11, and 87, respectively.
Host-agent token usage is unavailable.
