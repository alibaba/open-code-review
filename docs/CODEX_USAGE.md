# 在 Codex / Codex CLI 中使用这次重构

这次重构把 `Open Code Review` 切成了两层：

- Codex 负责理解需求、选择评审范围、判断问题、决定是否修复。
- `ocr` 负责提供确定性的 diff、上下文、校验、报告和扫描能力，不再在 Codex 路径里调用外部 LLM。

这意味着在 Codex 或 Codex CLI 中使用时，不需要配置 OCR 的 LLM provider 或 API key。

## 1. 安装和启用

如果你在本地仓库里开发或使用 fork，直接在仓库根目录执行：

```bash
codex plugin marketplace add .
codex
/plugins
```

如果是远程仓库，也可以替换成对应的仓库地址：

```bash
codex plugin marketplace add alibaba/open-code-review
codex
/plugins
```

进入插件列表后，启用 `Open Code Review`，然后新开一个 Codex 会话。

## 2. 在 Codex 里怎么用

启用后，直接用自然语言描述目标即可：

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review this commit
@Open Code Review review and fix high-confidence issues
```

Codex 会自动走新的主导路径：

1. 先调用 `ocr codex prepare` 生成 review bundle。
2. 再由 Codex 读取 bundle、补充上下文并完成判断。
3. 需要时再调用 `validate-comments` 和 `report`。
4. 只有你明确要求修复时，才会修改工作区文件。

## 3. 在 Codex CLI 里怎么用

Codex CLI 的使用方式和上面一致。核心是先启用插件，再用 `@Open Code Review` 明确发起评审。

如果你想先看目标范围，可以先让 Codex 只做准备和预览；如果目标较大，可以让它分片处理。

## 4. 常用 `ocr codex` 命令

手动调试或排查时，可以直接跑这些命令：

```bash
# 工作区
ocr codex prepare --format json

# 分支 / PR
ocr codex prepare --from <base> --to <head> --format json

# 指定提交
ocr codex prepare --commit <sha> --format json

# 大型变更分片
ocr codex prepare --split --format json

# 全量扫描
ocr codex prepare --scan --path internal --format json
```

如果你手上已经有 bundle 和评论结果，可以继续做校验和报告：

```bash
ocr codex validate-comments --bundle /tmp/bundle.json --comments /tmp/comments.json --output /tmp/validation.json
ocr codex report --bundle /tmp/bundle.json --comments /tmp/comments.json --validation /tmp/validation.json --format markdown
```

如果需要补证据，可以用 target-aware context：

```bash
ocr codex context read --bundle /tmp/bundle.json --path internal/example.go
ocr codex context find --bundle /tmp/bundle.json --query ResolveTarget
ocr codex context diff --bundle /tmp/bundle.json --path internal/example.go
ocr codex context search --bundle /tmp/bundle.json --query example
```

## 5. 会话和记录

如果你希望同一轮评审的准备、校验、报告都关联到同一个会话，可以显式传 `--session-id`：

```bash
ocr codex prepare --session-id review-20260630 --format json
ocr codex validate-comments --session-id review-20260630 --bundle /tmp/bundle.json --comments /tmp/comments.json --output /tmp/validation.json
ocr codex report --session-id review-20260630 --bundle /tmp/bundle.json --comments /tmp/comments.json --validation /tmp/validation.json --format markdown
```

这只会在你显式指定时写入会话记录，不会默认污染工作区。

## 6. 使用边界

- Codex 路径不需要 OCR provider。
- `ocr review` 和 `ocr scan` 仍然保留给明确想走原生 OCR 外部 LLM 流程的用户。
- 默认只读，只有明确要求修复时才修改文件。
- 不要把 `ocr codex` 当成独立的智能体，它只是 Codex 的确定性数据面和工具面。
