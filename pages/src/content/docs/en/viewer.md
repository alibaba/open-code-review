---
title: Session Viewer
sidebar:
  order: 10
---

`ocr viewer` is a small embedded HTTP server that renders past review
sessions in a browser-friendly UI. No external dependencies — sessions
are read directly from the JSONL files OCR writes to disk during every
review.

## Launching

```bash
ocr viewer                  # binds localhost:5483
ocr viewer --addr :3000     # bind to all interfaces on port 3000
ocr viewer --addr 0.0.0.0:8080   # bind on all interfaces
```

The default address is `localhost:5483`. The server holds the foreground
— `Ctrl+C` stops it. Sessions are scanned lazily from
`~/.opencodereview/sessions/` on each request, so a review running in
another terminal shows up the moment its JSONL file appears.

> **DNS-rebinding protection.** The viewer checks the `Host` header
> against a loopback allowlist (`localhost`, `127.0.0.1`, `::1`). A
> concrete bind host (e.g. `--addr 192.168.1.10:5483`) is added
> automatically, but **wildcard** binds (`:3000`, `0.0.0.0`, `::`) are
> not — reaching the UI from a LAN IP or hostname then returns
> `forbidden host`. To expose a wildcard bind, set
> `OCR_VIEWER_ALLOWED_HOSTS` to a comma-separated list of allowed
> hostnames (e.g. `OCR_VIEWER_ALLOWED_HOSTS=box.local,192.168.1.10`).

## Three pages

The viewer has three URLs:

| URL | What you see |
|---|---|
| `/` | List of all repositories that have sessions on disk. |
| `/r/{repo}` | List of sessions for one repository, newest first. |
| `/r/{repo}/{sessionID}` | Full detail for a single session. |

`{repo}` is a path-encoded string (separators `/` and `\` replaced with
`-`, colons replaced with `_` — the same encoding used to name the
on-disk directories). You don't usually type this — you click through.

### `/` — Repository list

For each repo with at least one session you see the repo path, the
total session count, and the most recent activity timestamp.

### `/r/{repo}` — Session list for one repo

For each session: ID (a UUID), branch name (when OCR was able to
detect it), review mode, model, file count, duration, and a started-at
timestamp.

### `/r/{repo}/{sessionID}` — Session detail

The detail page is the interesting one. It shows:

1. **Header** — diff range, model, branch, total tokens, run duration.
2. **File group** — one block per reviewed file. Inside each file, five
   "task type" lanes:

| Task type | When it appears |
|---|---|
| `plan_task` | The plan phase ran (file ≥ `PLAN_MODE_LINE_THRESHOLD`). |
| `main_task` | Every file. The main review loop. |
| `review_filter_task` | The post-review comment-filtering pass ran for this file. |
| `memory_compression_task` | The active+compress zone exceeded 60 % / 80 % budget. |
| `re_location_task` | A `code_comment` couldn't be anchored, fallback re-location ran. |

Each lane is a horizontal strip of **task cards** — one per LLM round
trip. Cards are coloured by task type so you can see at a glance which
phases dominated the run.

## What's in a task card

Click a task card to expand. Each card has:

- a **header row** — request number, model badge, a token badge
  (`P:` prompt / `C:` completion, plus `CR:` / `CW:` cache read/write
  when present), a duration badge, and an error badge when the round
  failed;
- **Response** — the raw assistant response, including any reasoning /
  `thinking` blocks;
- **Tool calls** — each tool invocation with arguments + the result that
  was returned (collapsible).

The full message list sent to the model and the in-scope tool
definitions are **not** rendered in the card UI; if you need them,
inspect the JSONL transcript directly (the `messages` field on each
`llm_request` record).

## Use cases

The viewer is designed around three workflows:

### "Why did the model say that?"

Open a comment in your terminal output, locate the file in the viewer,
and walk down its `main_task` lane. The card whose **tool calls**
include the `code_comment` you care about is the round that produced
it. The card's Response shows the model's reasoning; for the exact
prompt + context the model was sent, open the `llm_request` record for
that request number in the JSONL transcript (its `messages` field).

### "Why was this file silent?"

A file with **no comments** is a successful review only if the model
*deliberately* called `task_done`. If the lane shows tool calls but no
`code_comment`, that's an intentional clean review. If the lane ends in
an error card, it's a failure dressed up as silence — surface it as a
warning.

### "What did compression keep / drop?"

The `memory_compression_task` lane shows every compression round.
Inside, the Response pane has the resulting summary; the rendered XML
of the compress zone that was fed in lives in the round's
`llm_request` `messages` in the JSONL transcript. Useful when debugging
a "the model forgot earlier context" complaint — you can see whether
compression dropped the relevant detail.

## Storage layout on disk

The viewer reads from:

```
~/.opencodereview/sessions/
└── <path-encoded-repo-path>/
    └── <session-id>.jsonl
```

Each line in the JSONL file is one event:

```json
{"type": "llm_request", "filePath": "src/foo.go", "taskType": "main_task", "request_no": 1, "messages": [{"role": "user", "content": "Review this diff…"}], "timestamp": "2026-06-02T10:15:23Z"}
{"type": "llm_response", "filePath": "src/foo.go", "taskType": "main_task", "model": "claude-sonnet-4-6", "content": "Found 2 issues…", "duration_ms": 8421, "usage": {"prompt_tokens": 12450, "completion_tokens": 320}}
{"type": "tool_call", "filePath": "src/foo.go", "tool_name": "file_read", "arguments": "{\"file_path\":\"src/foo.go\",\"start_line\":1,\"end_line\":50}", "result": "File: src/foo.go (Total lines: 220)\nIS_TRUNCATED: false\nLINE_RANGE: 1-50\n1|package foo…", "ok": true, "duration_ms": 14}
```

Lines are append-only — a partial JSONL means a session was killed
mid-run, and the viewer renders what it has.

To free disk space, delete entire session files; the viewer regenerates
its index on the next request.

## Privacy

The JSONL transcripts contain **everything** sent to and received from
the LLM, including any code that was in the diff. They live entirely on
your machine inside `~/.opencodereview/`. OCR does not upload them
anywhere.

If your reviews include code you wouldn't want stored long-term,
either:

- delete the session files periodically, or
- redirect `--audience agent --format json` output to a transient pipe
  in CI and run with a temporary `HOME` so the JSONL never persists.

The OpenTelemetry exporter is a separate concern — see
[Telemetry](../telemetry/) for how to keep prompt content out of
exported traces.

## When the viewer is not the right tool

- For programmatic post-processing (CI, dashboards), use
  `ocr review --format json --audience agent`. The viewer renders for
  humans, not machines.
- For grepping across many sessions, use `jq` on the JSONL files
  directly. There's no search box in the UI yet.

## See Also

- [Architecture](../architecture/) — what those five task types
  actually do under the hood.
- [Tools](../tools/) — the tool calls you'll see in `main_task`
  cards.
