---
title: Direct Subprocess
sidebar:
  order: 3
---

通过 shell 调用 `ocr` 并解析 JSON。这是最低层的集成路径——本站其他方式最终都归结
于它。[Agent Skill](../agent-skill/) 与 [Command](../claude-code/) 方式是告诉
调用方 agent 去做这件事的 prompt 模板；[CI/CD](../ci/) 配方是从脚本做同样事情的
GitHub Actions 和 GitLab CI 流水线——不涉及编排 agent，只有子进程调用、JSON 解析、
把评论回贴到 PR / MR。当你从自定义脚本、LangChain 工具或任何其他尚未覆盖的
框架调用 OCR 时，直接用本页。

## Bash

```bash
result=$(ocr review --format json --audience agent)
status=$(echo "$result" | jq -r '.status')
total=$(echo "$result" | jq '.comments | length')
echo "Status: $status — $total comments"
echo "$result" | jq -r '.comments[] | "\(.path):\(.start_line) — \(.content)"'
```

## Python

```python
import json, subprocess

proc = subprocess.run(
    ["ocr", "review", "--format", "json", "--audience", "agent",
     "--from", "origin/main", "--to", "HEAD",
     "--background", pr_description],
    capture_output=True, text=True, check=True,
)
data = json.loads(proc.stdout)
for c in data["comments"]:
    if c["start_line"] > 0:
        post_line_comment(c["path"], c["start_line"], c["content"])
```

## JSON 结构

OCR 发出单个顶层**对象**（不是裸数组）。下面是一个带一条发现的完整 `success`
外壳：

```json
{
  "status": "success",
  "summary": {
    "files_reviewed": 1,
    "comments": 1,
    "total_tokens": 12770,
    "input_tokens": 12450,
    "output_tokens": 320,
    "elapsed": "9s"
  },
  "comments": [
    {
      "path": "internal/cache/store.go",
      "content": "Concurrent map access without a lock — wrap reads and writes with `sync.RWMutex` to avoid a race on the shared cache.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "func (s *Store) Get(k string) string {\n    return s.m[k]\n}",
      "suggestion_code": "func (s *Store) Get(k string) string {\n    s.mu.RLock()\n    defer s.mu.RUnlock()\n    return s.m[k]\n}",
      "thinking": "The struct exposes `m map[string]string` without a guarding mutex, and Get/Set are called from concurrent request handlers."
    }
  ]
}
```

### 顶层字段

| 字段 | 类型 | 总是存在 | 说明 |
|---|---|---|---|
| `status` | string | 是 | `success`、`completed_with_warnings`、`completed_with_errors`、`skipped` 之一。 |
| `message` | string | 否 | 简短人类可读摘要。在空运行或跳过时设置，如 `"No comments generated. Looks good to me."`。 |
| `summary` | object | 否 | 运行聚合。完成运行时存在；`skipped` 时省略。字段见下。 |
| `comments` | array | 是 | 可能为空。每条评论 schema 见下。 |
| `warnings` | array | 否 | 仅当一个或多个子 agent 失败或被跳过时存在。schema 见下。 |

### summary 结构（`summary`）

| 字段 | 类型 | 说明 |
|---|---|---|
| `files_reviewed` | int | 通过所有过滤并发给模型的文件数。 |
| `comments` | int | 跨所有文件发出的评论总数（与 `comments.length` 一致）。 |
| `total_tokens` | int | 运行中每次 LLM 调用的 prompt + completion token 之和。 |
| `input_tokens` | int | 各次 LLM 调用的 prompt token（含缓存读 token）。 |
| `output_tokens` | int | 各次 LLM 调用的 completion token（含缓存写 token）。 |
| `cache_read_tokens` | int | 各次 LLM 调用的缓存读 token 总数。为零时省略（`omitempty`）。 |
| `cache_write_tokens` | int | 各次 LLM 调用的缓存写 token 总数。为零时省略（`omitempty`）。 |
| `elapsed` | string | 挂钟时长，取整到整秒，由 Go 的 `time.Duration.String()` 格式化（如 `"1m12s"`）。 |

### 每条评论字段（`comments[]`）

| 字段 | 类型 | 总是存在 | 说明 |
|---|---|---|---|
| `path` | string | 是 | 仓库相对文件路径。 |
| `content` | string | 是 | 评审评论，Markdown。 |
| `start_line` | int | 是 | 受影响范围的首行。值 `< 1` 表示评论无行锚点（文件级）——应把这些合并到摘要中，而非尝试内联张贴。 |
| `end_line` | int | 是 | 受影响范围的末行。单行评论时与 `start_line` 相等。 |
| `existing_code` | string | 否 | 要被替换的原始代码片段。对于无 diff 的建议性评论则省略。 |
| `suggestion_code` | string | 否 | `existing_code` 的提议替换。存在时总是与 `existing_code` 配对。 |
| `thinking` | string | 否 | 模型推理轨迹。对分级 / 调试有用；展示给用户前可安全丢弃。 |

### warnings 结构（`warnings[]`）

一个跳过或部分文件失败的运行形如：

```json
{
  "status": "completed_with_errors",
  "message": "Some files could not be reviewed due to errors.",
  "comments": [],
  "warnings": [
    {
      "file": "src/very_long_file.go",
      "message": "diff size exceeds 80% of MAX_TOKENS; skipped",
      "type": "token_threshold_exceeded"
    },
    {
      "file": "src/broken.py",
      "message": "sub-agent failed: context deadline exceeded",
      "type": "subtask_error"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `file` | string | 触发警告的文件的仓库相对路径。 |
| `message` | string | 简短人类可读描述。 |
| `type` | string | 用于过滤的稳定类型。当前发出：`subtask_error`（子 agent 运行失败）和 `token_threshold_exceeded`（diff 对模型来说过大）。 |

当 `warnings` 含至少一个 `subtask_error` 时，`status` 为
`completed_with_errors`；否则为 `completed_with_warnings`。

### 无 severity / priority 字段

OCR **不**发出 `severity` 或 `priority` 字段。你在 [Agent Skill](../agent-skill/)
和 [Command](../claude-code/) 文档中看到的 High/Medium/Low 分级是调用方 agent
收到原始评论后添加的——不要尝试 `jq '.comments[].severity'`，它不存在。

## 空结果处理

**没有合格文件**的工作区通过 `status` 报告，以便调用方区分“无变更”与“无发现”：

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "comments": []
}
```

断定“全部干净”之前，始终检查 `status == "skipped"`。

## 另见

- [CI/CD](../ci/)——在子进程调用之上构建的现成 GitHub Actions 与 pre-commit
  配方。
- [Agent Skill](../agent-skill/)——当调用方是 Anthropic SDK agent 而非普通
  脚本时。
