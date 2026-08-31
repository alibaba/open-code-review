---
title: Telemetry
sidebar:
  order: 11
---

OCR ships with first-class **OpenTelemetry** support. Every review run
produces structured spans, metrics, and events. Wired up to a collector,
the data is enough to answer "what did the agent spend time on?",
"which models cost what?", and "why did this run fail?".

## Overview

Telemetry is **off by default**. Once enabled, OCR exports:

- **Spans** — three pipeline-level spans (`review.run`, `diff.parse`,
  `subtask.execute.group.<group-key>`) plus one short-lived `event.*`
  span per decision-point event.
- **Metrics** — aggregated counts and histograms for review duration,
  files reviewed, comments generated, LLM requests / tokens / latency,
  and tool calls / latency.
- **Events** — discrete in-span events like `plan.skipped`,
  `token.threshold.exceeded`, `review.started`.

Two exporters are supported:

| Exporter | When to use |
|---|---|
| `console` | Personal use / debugging. Pretty-prints spans to stdout. |
| `otlp` | System integration. Sends to any OTLP-compatible collector (Jaeger, Tempo, OTel Collector, Datadog Agent, …). |

## Enabling telemetry

Like the LLM endpoint, telemetry is configured by either persistent
config or environment variables — env wins on conflict.

### Config-file approach

```bash
ocr config set telemetry.enabled        true
ocr config set telemetry.exporter       otlp
ocr config set telemetry.otlp_endpoint  localhost:4317
ocr config set telemetry.content_logging false
```

The result in `~/.opencodereview/config.json`:

```json
{
  "telemetry": {
    "enabled": true,
    "exporter": "otlp",
    "otlp_endpoint": "localhost:4317",
    "content_logging": false
  }
}
```

### Environment-variable approach

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # implies exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # default; http/protobuf and http/json
                                                    # are also supported
export OTEL_SERVICE_NAME=open-code-review-prod      # optional; default: open-code-review
export OCR_CONTENT_LOGGING=0                        # reserved / currently a no-op (see Content logging)
```

Setting `OTEL_EXPORTER_OTLP_ENDPOINT` also forces `exporter=otlp` —
useful for one-off `OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review` runs.

### Endpoint format

The endpoint may be a bare `host:port` or carry a scheme. With a scheme, the
scheme decides the transport security: `http://` is plaintext, `https://` uses
TLS. A bare `host:port` uses TLS.

Note that the default port differs by protocol — `4317` for gRPC, `4318` for
HTTP:

```bash
# gRPC (default)
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# HTTP
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

For the HTTP protocols the endpoint is a **base URL**, and the per-signal path
is appended to it — so `http://localhost:4318` sends traces to
`http://localhost:4318/v1/traces`. A base path is preserved, which is what
backends served under a prefix require:

```bash
# traces go to http://collector:3000/api/public/otel/v1/traces
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:3000/api/public/otel
```

gRPC has no URL path, so this applies to the HTTP protocols only.

## What gets exported

### Spans

The full span tree for a review:

```
review.run
├── diff.parse
├── event.review.started                   (decision-point event)
├── subtask.execute.group.<group-key1>
│   ├── event.plan.skipped                 (when changes are below both thresholds)
│   ├── event.plan.failed                  (when plan phase errored)
│   ├── event.token.threshold.exceeded     (when prompt > 80% of max_tokens)
│   ├── main.loop                          (one span per review round)
│   └── event.subtask.error                (when the subtask errored)
├── subtask.execute.group.<group-key2>
└── …
```

One `subtask.execute.group.*` span is emitted per reviewed **group**, not
per file — files are bundled semantically before review. The group key is
the group's file paths, sorted and comma-joined (a single path when the
group holds one file).

LLM round trips and tool executions are **not** emitted as separate
spans — they show up only in metrics (see below). Decision-point events
fire as short-lived `event.<name>` spans attached to the current
context.

Each span carries useful attributes:

| Span | Key attributes |
|---|---|
| `review.run` | `error` (set when the run failed) |
| `diff.parse` | `files.changed`, `lines.inserted`, `lines.deleted` |
| `subtask.execute.group.<group-key>` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file` |
| `main.loop` | `group.label`, `round` |
| `event.review.started` | `file.count`, `review.count`, `repo.dir` |
| `event.plan.skipped` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file`, `threshold`, `threshold.group` |
| `event.plan.failed` | `group.label`, `message` |
| `event.token.threshold.exceeded` | `group.label`, `tokens`, `max_tokens`, `round` |
| `event.subtask.error` | `group.label`, `error` |

### Metrics

OCR records numeric metrics via the OTel meter — counts and histograms
the collector aggregates downstream:

| Metric | Type | Unit | Labels |
|---|---|---|---|
| `ocr.review.duration_seconds` | histogram | `s` | — |
| `ocr.files_reviewed_total` | counter | — | — |
| `ocr.comments_generated_total` | counter | — | — |
| `ocr.llm.requests_total` | counter | — | `model`, `status` (`ok` / `error`) |
| `ocr.llm.request_duration_seconds` | histogram | `s` | `model` |
| `ocr.llm.tokens_used` | counter | — | `model`, `type` (currently always `total`) |
| `ocr.tool.calls_total` | counter | — | `tool.name`, `status` (`ok` / `error`) |
| `ocr.tool.execution_duration_seconds` | histogram | `s` | `tool.name` |

### Events

Events fire as short-lived `event.<name>` spans at decision points.
The full list:

| Event | Meaning |
|---|---|
| `review.started` | Diffs loaded; we know how many files we'll review. |
| `no.files.changed` | The diff resolved to zero files. |
| `plan.skipped` | A group was below both plan thresholds: its largest file changed fewer than `PLAN_MODE_LINE_THRESHOLD` lines, and (for 2+ file groups) the total was below `PLAN_MODE_GROUP_LINE_THRESHOLD`. |
| `plan.failed` | The plan phase errored; main loop ran without a plan. |
| `token.threshold.exceeded` | Prompt tokens > 80 % of `MAX_TOKENS` (the input ceiling); group skipped. |
| `subtask.error` | A per-group subtask errored — emitted with `Error` span status. |

Use these to alert on degraded review quality long before a user
notices.

## Content logging

Telemetry exports the **shape** of LLM traffic (counts, durations,
statuses) but **never** the actual prompts or responses. OCR makes no
attempt to attach LLM message content to spans or events — the data
that leaves the process is the metric / event schema documented above
and nothing else.

The `content_logging` config key (and `OCR_CONTENT_LOGGING=1` env
override) is plumbed through the config layer but currently does **not**
gate any code path that emits prompt content. Treat the flag as
reserved.

If you need to inspect what was sent to or returned from the LLM, use
the local JSONL transcripts that the [Session Viewer](../viewer/)
reads. Those live entirely on disk under `~/.opencodereview/` and are
never shipped to the collector.

For deeper debugging, set `OCR_RAW_LOGGING=1` to capture the raw
request and response bodies of every LLM call in
`~/.opencodereview/raw/` — more detailed debugging information than
the session transcripts. Off by default. With streaming enabled, the
capture reads the entire response body before handing it on to the
rest of the processing. Capture redacts request headers but records
request and response bodies as-is.

## Recipes

### Console exporter for local debugging

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

Spans print to stdout in human-readable form. Pipe through `less` to
read a long run.

### OTel Collector with Tempo + Prometheus

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols: { grpc: { endpoint: 0.0.0.0:4317 } }

exporters:
  otlp/tempo:
    endpoint: tempo:4317
    tls: { insecure: true }
  prometheus:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    traces:  { receivers: [otlp], exporters: [otlp/tempo] }
    metrics: { receivers: [otlp], exporters: [prometheus] }
```

Then in your shell:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

Open Tempo → search by `service.name=open-code-review` → click any
trace to see the full span tree.

### Datadog

The Datadog Agent's OTLP receiver speaks OTLP/gRPC by default:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

Spans show up under APM with the service name; LLM metrics show up
under Metrics with the labels above.

### CI run, results in your dashboard

Inject the env in your pipeline step:

```yaml
- name: Code review
  env:
    OCR_LLM_URL: ${{ secrets.OCR_LLM_URL }}
    OCR_LLM_TOKEN: ${{ secrets.OCR_LLM_TOKEN }}
    OCR_LLM_MODEL: claude-opus-4-6
    OCR_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_COLLECTOR_URL }}
    OTEL_SERVICE_NAME: open-code-review-ci
  run: ocr review --from origin/main --to HEAD --audience agent
```

The `OTEL_SERVICE_NAME` separates CI traces from human dev runs.

## Resolution priority

When OCR builds the final telemetry config:

1. Defaults (`enabled=false`, `exporter=console`, no endpoint).
2. `telemetry.*` keys from `~/.opencodereview/config.json`.
3. Environment variables (highest priority, **overrides** the file).

So you can leave `telemetry.enabled=false` in the config and flip it
per-run with `OCR_ENABLE_TELEMETRY=1`.

## Sampling and overhead

OCR exports **everything**. There is no sampling configuration; OTel's
sampling is the responsibility of your collector. For a typical review
run that's:

- 1 `review.run` span + 1 `diff.parse` span + 1
  `subtask.execute.group.<group-key>` span per reviewed group (plus its
  `plan.execute` / `main.loop` / `review_filter.execute` children) + 1
  short-lived `event.*` span per decision-point event.
- A 10-file PR produces ~15–25 spans total — fewer when grouping bundles
  files together, more when the effort preset runs extra review rounds.
  LLM round trips and tool calls add to the metric counters but do not
  create extra spans.

The export is **batched and asynchronous** — telemetry doesn't block
the review loop. If the collector is unreachable, OCR logs a warning
and continues; the review still produces its normal output.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Nothing exported | `OCR_ENABLE_TELEMETRY` / `telemetry.enabled` is unset. The default is **off**. |
| OTLP works locally, fails in prod | Check that the protocol matches the collector. The default is gRPC, and many managed backends accept OTLP over HTTP only — set `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf` and use the HTTP port (`4318`, not `4317`). |
| Nothing arrives, no error | The exporter is created lazily, so a wrong endpoint fails silently at export time rather than at startup. Check the collector's access log for the path being requested: for the HTTP protocols the endpoint is a base URL and the signal path is appended, so `http://host:4318` posts to `http://host:4318/v1/traces`. |
| Spans show but no metrics | Some collectors only enable the traces pipeline by default; add a `metrics` pipeline in the config. |
| Prompts missing from spans | OCR never attaches prompt content to telemetry — see [Content logging](#content-logging). Inspect transcripts via [Session Viewer](../viewer/) instead. |

## See Also

- [Configuration](../configuration/) — full key reference for the
  `telemetry.*` namespace.
- [Architecture](../architecture/) — what each span actually
  measures.
- [OpenTelemetry docs](https://opentelemetry.io/docs/) — collector
  setup and exporters.
