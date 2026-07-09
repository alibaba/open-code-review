---
title: 遥测
sidebar:
  order: 11
---

OCR 自带一流的 **OpenTelemetry** 支持。每次评审运行产出结构化的 span、
metric 和 event。接入 collector 后，这些数据足以回答“agent 把时间花在哪了？”、
“各模型成本如何？”、“这次运行为什么失败？”。

## 概览

遥测**默认关闭**。启用后，OCR 导出：

- **Span**——三个流水线级 span（`review.run`、`diff.parse`、
  `subtask.execute.<file>`）外加每个决策点事件一个短生命周期的 `event.*` span。
- **Metric**——评审时长、评审文件数、生成评论数、LLM 请求 / token / 延迟、
  工具调用 / 延迟的聚合计数与直方图。
- **Event**——span 内离散事件，如 `plan.skipped`、
  `token.threshold.exceeded`、`review.started`。

支持两种 exporter：

| Exporter | 何时使用 |
|---|---|
| `console` | 个人使用 / 调试。把 span 格式化打印到 stdout。 |
| `otlp` | 系统集成。发送到任何 OTLP 兼容 collector（Jaeger、Tempo、OTel Collector、Datadog Agent……）。 |

## 启用遥测

与 LLM 端点一样，遥测可通过持久化 config 或环境变量配置——冲突时环境变量优先。

### 配置文件方式

```bash
ocr config set telemetry.enabled        true
ocr config set telemetry.exporter       otlp
ocr config set telemetry.otlp_endpoint  localhost:4317
ocr config set telemetry.content_logging false
```

`~/.opencodereview/config.json` 中的结果：

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

### 环境变量方式

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # implies exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # default. NOTE: only grpc is currently
                                                    # implemented; http/protobuf and http/json
                                                    # are accepted but not yet wired up.
export OTEL_SERVICE_NAME=open-code-review-prod      # optional; default: open-code-review
export OCR_CONTENT_LOGGING=0                        # reserved / currently a no-op (see Content logging)
```

设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 也会强制 `exporter=otlp`——适合一次性的
`OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review` 运行。

## 导出什么

### Span

一次评审的完整 span 树：

```
review.run
├── diff.parse
├── event.review.started                   (decision-point event)
├── subtask.execute.<file1>
│   ├── event.plan.skipped                 (when changes are below threshold)
│   ├── event.plan.failed                  (when plan phase errored)
│   ├── event.token.threshold.exceeded     (when prompt > 80% of max_tokens)
│   └── event.subtask.error                (when the subtask errored)
├── subtask.execute.<file2>
└── …
```

LLM 往返和工具执行**不**作为单独 span 发出——它们只出现在 metric（见下）中。
决策点事件作为短生命周期的 `event.<name>` span 附着到当前 context。

每个 span 携带有用属性：

| Span | 关键属性 |
|---|---|
| `review.run` | `error`（运行失败时设置） |
| `diff.parse` | `files.changed`、`lines.inserted`、`lines.deleted` |
| `subtask.execute.<file>` | `file.path`、`lines.changed`、`lines.inserted`、`lines.deleted` |
| `event.review.started` | `file.count`、`review.count`、`repo.dir` |
| `event.plan.skipped` | `file.path`、`lines.changed`、`threshold` |
| `event.plan.failed` | `file.path`、`message` |
| `event.token.threshold.exceeded` | `file.path`、`tokens`、`max_tokens` |
| `event.subtask.error` | `file.path`、`error` |

### Metric

OCR 通过 OTel meter 记录数值 metric——计数与直方图，由 collector 在下游聚合：

| Metric | 类型 | 单位 | 标签 |
|---|---|---|---|
| `ocr.review.duration_seconds` | histogram | `s` | — |
| `ocr.files_reviewed_total` | counter | — | — |
| `ocr.comments_generated_total` | counter | — | — |
| `ocr.llm.requests_total` | counter | — | `model`、`status`（`ok` / `error`） |
| `ocr.llm.request_duration_seconds` | histogram | `s` | `model` |
| `ocr.llm.tokens_used` | counter | — | `model`、`type`（当前总是 `total`） |
| `ocr.tool.calls_total` | counter | — | `tool.name`、`status`（`ok` / `error`） |
| `ocr.tool.execution_duration_seconds` | histogram | `s` | `tool.name` |

### Event

事件在决策点作为短生命周期的 `event.<name>` span 触发。完整列表：

| 事件 | 含义 |
|---|---|
| `review.started` | diff 已加载；我们知道将评审多少文件。 |
| `no.files.changed` | diff 解析出零文件。 |
| `plan.skipped` | 某文件低于 `PLAN_MODE_LINE_THRESHOLD`。 |
| `plan.failed` | plan 阶段出错；main 循环无 plan 运行。 |
| `token.threshold.exceeded` | 初始 prompt token > `MAX_TOKENS` 的 80 %；文件被跳过。 |
| `subtask.error` | 某 per-file 子任务出错——以 `Error` span 状态发出。 |

借此可在用户察觉之前，及早发现评审质量退化并告警。

## 内容日志

遥测导出 LLM 流量的**形状**（计数、时长、状态），但**绝不**导出实际 prompt 或
响应。OCR 不尝试把 LLM 消息内容附加到 span 或 event——离开进程的数据仅限于上面
记录的 metric / event schema，别无其他。

`content_logging` config key（和 `OCR_CONTENT_LOGGING=1` 环境覆盖）已接入配置层，
但目前**不**控制任何发出 prompt 内容的代码路径。请将该标志视为保留位。

如需检查发给 LLM 或从 LLM 返回的内容，使用[会话查看器](../viewer/)读取的本地
JSONL 转录。它们完全存在于 `~/.opencodereview/` 下的磁盘上，绝不发往 collector。

## 配方

### 本地调试用 console exporter

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

span 以人类可读形式打印到 stdout。可通过管道传给 `less` 查看长运行输出。

### OTel Collector + Tempo + Prometheus

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

然后在 shell 中：

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

打开 Tempo → 按 `service.name=open-code-review` 搜索 → 点击任意 trace 看完整
span 树。

### Datadog

Datadog Agent 的 OTLP receiver 默认使用 OTLP/gRPC：

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

span 以该 service name 出现在 APM 下；LLM metric 带上述标签出现在 Metrics 下。

### CI 运行，结果进入仪表盘

在流水线步骤中注入环境变量：

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

`OTEL_SERVICE_NAME` 可把 CI trace 与人工开发运行的 trace 区分开。

## 解析优先级

OCR 构建最终遥测配置时：

1. 默认（`enabled=false`、`exporter=console`、无 endpoint）。
2. `~/.opencodereview/config.json` 的 `telemetry.*` key。
3. 环境变量（最高优先级，**覆盖**文件）。

因此你可以在 config 中保留 `telemetry.enabled=false`，按运行用
`OCR_ENABLE_TELEMETRY=1` 开启。

## 采样与开销

OCR 导出**一切**。没有采样配置；OTel 的采样是 collector 的责任。对一次典型评审
运行：

- 1 个 `review.run` span + 1 个 `diff.parse` span + 每个被评审文件 1 个
  `subtask.execute.<file>` span + 每个决策点事件 1 个短生命周期的 `event.*` span。
- 10 文件的 PR 总共约 15–25 个 span。LLM 往返和工具调用增加 metric 计数但不创建
  额外 span。

导出是**批量且异步**的——遥测不阻塞评审循环。若 collector 不可达，OCR 记录警告
并继续；评审仍会产出正常输出。

## 故障排查

| 症状 | 可能原因 |
|---|---|
| 什么都没导出 | `OCR_ENABLE_TELEMETRY` / `telemetry.enabled` 未设置。默认**关闭**。 |
| OTLP 本地可用，生产失败 | OCR 当前仅实现 OTLP/gRPC——`OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`（或 `http/json`）被接受但未接入，切换也无济于事。请验证 endpoint 以及 collector 是否在监听 gRPC。 |
| span 显示但无 metric | 一些 collector 默认只启用 traces pipeline；在配置中加 `metrics` pipeline。 |
| span 中缺 prompt | OCR 绝不把 prompt 内容附加到遥测——见[内容日志](#content-logging)。改用[会话查看器](../viewer/)检查转录。 |

## 另见

- [配置](../configuration/)——`telemetry.*` 命名空间的完整 key 参考。
- [架构](../architecture/)——每个 span 实际度量什么。
- [OpenTelemetry 文档](https://opentelemetry.io/docs/)——collector 设置与 exporter。
