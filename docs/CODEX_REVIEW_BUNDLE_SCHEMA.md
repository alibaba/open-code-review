# Codex Review Bundle 协议

本文档固定 `codex-review-bundle/v1`、`codex-review-comments/v1` 与
`codex-review-manifest/v1` 的规范语义。JSON Schema 的可执行副本位于
`internal/reviewbundle/schemas/`。

## 哈希规范

所有摘要均使用 SHA-256，小写十六进制，并带 `sha256:` 前缀。多个字段组合时，依次写入每个字段的 8 字节大端长度和原始字节，禁止仅用分隔符拼接。这样可以避免字段边界歧义。

`diff_sha256` 覆盖 provider 返回顺序中的路径、旧路径和原始 patch。`content_sha256` 覆盖 provider 解析出的目标文件内容；删除文件覆盖空内容。规则摘要覆盖 `source`、`pattern` 与完整规则文本。

`bundle_id` 覆盖以下规范 JSON，计算时 `bundle_id` 自身为空，且不包含时间戳：

- 已解析 target；
- workspace state（如适用）；
- summary；
- 去重后的 rules；
- files；
- contract 中除 `bundle_size_bytes` 外的约束。

## Target 语义

- `workspace`：目标是当前 `HEAD` 加 staged、unstaged、untracked 改动；`workspace_state` 必须存在。
- `range`：base 是 `merge-base(from,to)`，head 是 `to^{commit}`。
- `commit`：base 是 `commit^`，head 是 `commit^{commit}`。
- `scan`：文件内容来自指定目录的当前快照；Git 仓库和普通目录均可使用。

所有 ref 必须经过 `rev-parse --verify --end-of-options <ref>^{commit}`，以拒绝选项注入和不存在的提交。

workspace state 分别记录当前 `HEAD`、staged diff、unstaged diff和按路径排序的 untracked 文件内容摘要。未跟踪符号链接只摘要链接文本，不跟随到仓库外。

## 行号与评论

评论行号使用一基新文件行号 `one_based_new_file`。普通行级评论要求 `start_line >= 1` 且 `end_line >= start_line`；文件级评论显式设置 `file_level_comment=true`，并使用 0/0 行范围。

`priority` 只允许 `high`、`medium`、`low`。`category` 只允许 `bug`、`security`、`performance`、`concurrency`、`maintainability`、`test`。

## 大小与失败

默认 bundle 上限为 4 MiB。编码结果超限时必须返回 `bundle_too_large`，不得静默截断 patch、规则或文件。

## Scan manifest

全文件 scan 输出 `codex-review-manifest/v1`。manifest 记录全局目标摘要、分组策略、
批次大小、估算 token、partial 状态、明确跳过的文件及原因，并按确定顺序内嵌
`codex-review-bundle/v1` 分片。每个文件只允许出现在一个分片中。

scan 分片使用 `target.mode=scan`，`files[].content` 保存全文件证据，
`content_sha256` 保存内容摘要，`patch` 为空。`none`、`by-language` 和
`by-directory` 分组复用原生 scan 实现。文件大小或 token 预算超限时必须出现在
`skipped_files`，且 manifest 标记 `partial=true`；不得把跳过文件计入已评审范围。

大型 diff 使用 `ocr agent prepare --split` 生成同一 manifest 协议，
`batch_strategy=diff`；每个文件只进入一个满足大小上限的分片。单文件本身超过上限
时仍返回 `bundle_too_large`，不得截断。

context 命令读取 manifest 时使用 `--bundle-index` 选择分片。评论校验和报告
根据 `comments.bundle_id` 自动选择 manifest 中对应的分片。

Phase 2 校验器保留以下错误码：

- `invalid_schema`
- `bundle_id_mismatch`
- `stale_bundle`
- `unknown_path`
- `path_escape`
- `invalid_line_range`
- `outside_changed_hunk`
- `existing_code_mismatch`
- `ambiguous_existing_code`
- `bundle_too_large`

## 安全边界

Phase 1 的 prepare 只读取 Git、规则和目标文件。默认仅向 stdout 输出；只有显式 `--output` 才写用户指定的 bundle 文件。该路径不得创建 OCR LLM client、读取模型凭据、生成评审结论、修改源码或执行 commit。
