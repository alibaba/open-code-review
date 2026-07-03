---
title: FAQ
sidebar:
  order: 14
---

常见错误、意外与“这应该这样吗？”的问题。若你的问题不在此处，开一个带运行步骤
与完整输出的 [GitHub issue](https://github.com/alibaba/open-code-review/issues)。

## 配置与启动

### `no valid LLM endpoint configured`

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

OCR 走完了四来源解析链（[配置](../configuration/#endpoint-resolution)）但没
找到完整的 `(URL, token, model)` 三元组。要么：

- 运行 `ocr config set llm.url …` / `llm.auth_token …` / `llm.model …` 填充
  `~/.opencodereview/config.json`，**或**
- 导出 `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL`，**或**
- 若你已在用 Claude Code，导出 `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` /
  `ANTHROPIC_MODEL`。

然后 `ocr llm test` 验证连通性再重试评审。

### `ocr llm test` 显示错误的来源

OCR 取**第一个**完整三元组，而非最后一个。因此若配置文件已有全部三个 llm.*
key，环境变量会被忽略。要让环境变量生效，删除配置 key（删除文件或手动 unset）或
用 `ocr config set` 切换到新值。

### `ocr llm test` 返回 401 / 403

token 缺少 scope、已过期或厂商不匹配。Anthropic 与 OpenAI 用不同的 auth header 与
URL 格式——确保 `llm.use_anthropic` 与你指向的 URL 相匹配：

- Anthropic：URL 以 `/v1/messages` 结尾，`use_anthropic=true`。
- OpenAI / OpenAI 兼容：URL 以 `/v1/chat/completions` 结尾，
  `use_anthropic=false`。

### `not a git repository`

`ocr review` 对当前目录运行 `git diff`（以及对 untracked 文件的 `git ls-files`）。
若你不在 Git 工作树内，它会提前退出。要么 `cd` 进仓库，要么传 `--repo /path/to/repo`。

## 过滤与规则

### 我的文件没被评审

运行 `ocr review --preview`（无 LLM 成本）。输出列出每个候选文件及其被保留或
丢弃的**原因**：

```
src/foo.go              modified
src/foo_test.go         modified  (excluded: user_exclude)
node_modules/lib.js     added     (excluded: default_path)
imgs/logo.png           binary    (excluded: unsupported_ext)
```

五种排除原因对应[文件过滤](../review-rules/#how-files-are-filtered)中的门：

| 原因 | 修复 |
|---|---|
| `binary` | 无需处理——二进制文件无可评审文本。 |
| `user_exclude` | 从你的 `exclude` 列表移除该模式。 |
| `unsupported_ext` | 把扩展名加入你的 `include` 列表以绕过白名单门。 |
| `default_path` | 把文件加入 `include`——那会覆盖内置测试文件排除模式。 |
| `deleted` | 无需处理——没有新内容可评审。 |

### 我的自定义规则没触发

运行 `ocr rules check <file-path>`。它会完整打印匹配的**层**与 **glob 模式**：

```
File: src/api/UserHandler.go
Source: Project (.opencodereview/rule.json)
Pattern: src/api/**/*.go
Rule: …
```

若层不对（如期望项目规则却显示 "System built-in"），多半是**声明顺序**问题——
首条匹配模式生效。把更具体的规则在 `rules` 数组里前移，或修正 glob。

### 花括号展开不工作

`bmatcuk/doublestar/v4` 支持 `{ts,tsx}` 花括号。若不匹配，检查多余空格——
`{ts, tsx}` 带空格会静默地无法匹配 `tsx`。

## 评审

### 某文件显示零评论——它真的被评审了吗？

打开[会话查看器](../viewer/)（`ocr viewer`），找到会话，看该文件的
`main_task` 泳道：

- 有工具调用 + 以 `task_done` 结束 → 干净评审。
- 有工具调用 + 循环中途结束 → 找错误卡片。
- 完全没有 `main_task` 卡片 → 文件评审前被过滤；见上方[过滤与规则](#filtering--rules)。

### 评论的 `start_line: 0` 和 `end_line: 0`

OCR 无法把评论锚定到 diff 中的精确行。两个常见原因：

- 模型改写了 `existing_code` 而非从 diff 原样复制。模型被告知不要这样做，但偶尔
  仍会如此。
- diff 有异常格式（CRLF、tab/空格混用）破坏了滑动窗口匹配。

评论仍是真实的——只是没被自动放置。多数 agent 集成（SKILL、Claude Code
plugin）读 `existing_code` 字段并自行在文件中定位。

### Token threshold exceeded

```
[ocr] WARNING: prompt tokens (94000) exceed 80% of max_tokens(58888) for src/big.sql
```

该文件的初始 prompt（规则 + diff + change-files 列表）在模型能响应之前就已超过
`MAX_TOKENS = 58888` 的 80 %。OCR 跳过该文件并继续——JSON 模式下你也会在
`warnings` 中看到。

缓解：

- 若是自动生成的，把文件加入 `exclude` 列表。
- 把大重构拆成更小的 commit。
- 对一系列小 commit 用 `--commit` 模式，而非一次性工作区模式评审。

### plan 阶段花了很久而文件很小

先运行 `ocr review --preview`。若文件的 `lines.changed` 超过
`PLAN_MODE_LINE_THRESHOLD`（默认 **50**），plan 阶段会运行。这是有意为之——大
diff 能从 plan 中受益。要为单次评审跳过它，用更小 diff 运行，或临时编辑内嵌模板
（高级；需覆盖 `--tools`）。

### "Max tool requests reached"

```
[ocr] Max tool requests reached for src/foo.go.
```

模型花了 30（`MAX_TOOL_REQUEST_TIMES`）轮工具调用却没调 `task_done`。到那时为
止发出的评论仍被收集并渲染。若多数文件都这样，问题通常是：

- 模型不擅长遵循“完成后调 `task_done`”指令。换更强模型（如 Claude Opus）。
- 某工具持续报错而模型持续重试。看会话 JSONL——若同一工具结果重复，即是原因。
- 文件确实大或上下文重，30 轮不够。用 `--max-tools <n>` 调高或调低
  （如 `--max-tools 40` 更多，`--max-tools 15` 更少）。1–9 会被上调到 10；
  `0`（默认）用模板默认 30。

### 一些子 agent 失败；运行仍以 0 退出

有意为之。OCR 隔离 per-file 失败，使一个有问题的文件不会拖垮 20 文件的评审。只要*有*
成功的，聚合退出码就是 `0`；仅当完全失败（零成功子 agent）才非零退出。查看 JSON
模式的 `warnings` 数组或文本模式的 stderr，看哪些文件失败了。

### CI 运行比本地慢得多

两个常见原因：

- **模型速率限制**——限流下 LLM client 退避并重试。调低 `--concurrency`
  （如 `4`）以免一开始就触限。
- **冷缓存**——若 provider 支持 prompt 缓存，部署后首次运行无法受益。同一窗口内
  后续运行更快。

## 输出与集成

### `--audience agent` 仍有进度行

确认你看到的不是 **stderr**。进度消息偶尔会到 stderr（警告、错误）。`--audience
agent` 保证的干净 stdout 是*对解析器友好的*——要屏蔽一切，重定向：
`ocr review --audience agent 2>/dev/null`。

### JSON 输出是 `{ "files_reviewed": 0, "comments": [] }`

工作区没有合格文件。这是有意为之——显式形状让调用方区分“无可评审内容”与“已评审
文件中无发现”。零评论的正常评审产出的是普通空数组 `[]`。

### 会话 JSONL 在哪？

```
~/.opencodereview/sessions/<path-encoded-repo-path>/<session-id>.jsonl
```

仓库路径通过把 `/` 和 `\` 替换为 `-`、`:` 替换为 `_` 编码
（如 `/Users/foo/my-repo` → `Users-foo-my-repo`）。用 `ocr viewer` 浏览会话。
删除该目录清除历史；OCR 在下次运行时重新生成编码路径。

## 性能与成本

### 怎么知道哪些 token 花了多少？

启用遥测：

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review
```

LLM 调用没有自己的 span——它们记为 metric。关注 `ocr.llm.tokens_used`
（counter，标 `model` + `type`）、`ocr.llm.requests_total`（counter，标 `model`
+ `status`）、`ocr.llm.request_duration_seconds`（histogram，标 `model`）。
console exporter 会内联打印这些聚合。如需仪表盘，切换到 OTLP exporter 并发到你的
metrics 体系——见[遥测](../telemetry/)。

### 为什么我的评审这么贵？

常见因素：

- 文件 ≥ 50 行时 plan 阶段开启。它每文件多一次 LLM 调用。降低阈值可减少成本；升高
  阈值可提升小 PR 的速度。
- `MAX_TOOL_REQUEST_TIMES = 30` 很宽松。用满轮数的模型会产出比 3 轮就完成的模型
  更长（更多 token）的对话。更强模型倾向于更快完成。反过来，若你为应对 "max tool
  requests reached" 用 `--max-tools` 调高，预期每文件成本大致线性增长。
- 记忆压缩本身是一次 LLM 调用。较长的子任务除评审轮外，还要为压缩轮付费。

### 如何减少 LLM 调用？

- 添加 `include` 列表，使 OCR 不评审你不关心的文件。
- 若你的账户有 burst-mode 计价，调低 `--concurrency`。
- 传 `--background`——更充分的前期上下文有时能让模型无需 `file_read` /
  `code_search` 往返即可完成。

## 隐私与安全

### OCR 会把我的代码发到别处吗？

OCR 把你的 **diff**（及可选 read-tool 片段）发到你配置的 LLM 端点。其余任何内容都不
离开你的机器——会话 JSONL 与规则文件仅存于本地。

若启用遥测，`content_logging` 标志已接入配置层但目前**不**控制任何代码路径——
无论该标志值如何，prompt 与响应内容绝不导出到你的 collector。请视为保留位。生产
环境保持 `false`。详情见[遥测](../telemetry/#content-logging)。

### 我能在发给 LLM 前脱敏 secret 吗？

非内置功能。推荐工作流：

1. 不要把 secret 提交到仓库（常规规则）。
2. 把已知含敏感信息的文件加入 `exclude`。
3. 用 `git diff --no-textconv` 过滤器或 pre-commit 脱敏，使 secret 不进入 diff。

“脱敏规则”功能在路线图上；关注
[issue 跟踪器](https://github.com/alibaba/open-code-review/issues)。

## 杂项

### changelog 在哪？

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)
——每个 release 都附带从 Conventional Commits 生成的 notes。

### OCR 支持非 Git VCS 吗？

不支持。diff provider 通过 shell 调用 `git`。SVN / Mercurial 等需要新的 provider；Hg 支持的
issue 已在[此](https://github.com/alibaba/open-code-review/issues)开放。

### 为什么二进制叫 `opencodereview` 而 CLI 是 `ocr`？

release 中发布的静态二进制以项目命名（`opencodereview`）；NPM wrapper 为了便于使用而安装为 `ocr`。从源码构建得到 `dist/opencodereview`——复制为 `$PATH` 上的
`ocr`。

### 如何卸载？

```bash
npm uninstall -g @alibaba-group/open-code-review        # NPM install
sudo rm /usr/local/bin/ocr                              # binary install
rm -rf ~/.opencodereview                                # all state
```

OCR 不在 `~/.opencodereview` 之外写入（NPM 下载二进制除外），因此删除该目录即可
清除历史、配置与每用户规则。

## 另见

- [配置](../configuration/)——LLM 端点解析与 config key。
- [评审规则](../review-rules/)——文件过滤器与规则解析链。
- [会话查看器](../viewer/)——查看历史评审会话。
- [遥测](../telemetry/)——token 用量与 LLM 指标。
