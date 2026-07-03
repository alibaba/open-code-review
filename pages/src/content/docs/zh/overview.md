---
title: 概览
sidebar:
  order: 2
---

## 什么是 Open Code Review？

Open Code Review（简称 **OCR**，区别于光学字符识别 Optical Character
Recognition）是一个 AI 驱动的代码评审 CLI，以
[`@alibaba-group/open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review)
NPM 包和独立的 Go 二进制形式发布。CLI 二进制名为 `ocr`。

只需一条命令（`ocr review`），它会：

1. 解析 Git diff——工作区、分支区间或单个 commit。
2. 结合系统默认规则与用户规则对变更文件进行过滤。
3. 为每个变更文件并行启动一个 **per-file 子 agent**。
4. 每个子 agent 运行一个 LLM 工具调用循环；对于较大的 diff，可选地先执行
   **plan 阶段**。
5. 模型调用 `code_comment` 记录发现，可选地调用 `file_read`、
   `code_search`、`file_find`、`file_read_diff` 收集上下文，完成后调用
   `task_done`。
6. OCR 将每条评论解析到精确的行号，对未能精确匹配的评论运行可选的重新定位
   流程，并打印（或以 JSON 输出）最终列表。

## 通用 agent 的问题

如果你用过通用编码 agent（Claude Code 的 Skill、Cursor、Cline 等）做代码
评审，很可能遇到过：

- **覆盖不全**——在较大的变更集上，agent 会悄悄偷工减料，只评审部分文件。
- **位置漂移**——评论与它所指的代码对不上；行号和文件路径偏离目标。
- **质量不稳定**——自然语言 Skill 难以调试，输出质量随 prompt 的微小改动
  而波动。

根本原因：纯语言驱动的架构缺乏对评审流程的 **硬约束**。

## 核心设计：确定性工程 × agent

OCR 的核心理念是把 **确定性工程** 与 **agent** 结合——各自做自己最擅长的事。

### 确定性工程——硬约束

对于那些 *绝不能出错* 的步骤，由工程逻辑（而非模型）保证正确性：

- **精确的文件选择**——一个[五重门过滤](../review-rules/#how-files-are-filtered)
  决定到底评审哪些文件，并提供显式的 `include`/`exclude` 控制。
- **智能文件打包**——相关文件（如 `message_en.properties` 与
  `message_zh.properties`）可以合并为一个评审单元。每个包作为独立上下文交给
  子 agent 运行——分而治之，在超大变更集上依然稳定，并天然支持并发评审。
- **细粒度规则匹配**——评审规则按文件路径匹配，首条匹配生效，让模型的注意力
  高度聚焦并消除噪声。基于模板的匹配比纯语言驱动的规则引导更稳定。
- **外部定位与反思模块**——独立的评论定位
  （[`internal/diff/relocation.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/relocation.go)）
  与重新定位流程，系统地提升位置与内容的准确性。

### Agent——动态决策

agent 的优势集中在最关键的地方：

- **场景化调优的 prompt**——针对代码评审场景深度调优的 prompt 模板，在降低 token
  消耗的同时提升效果（见
  [`internal/config/template/task_template.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/template/task_template.json)）。
- **场景化调优的工具集**——从大规模生产数据的工具调用 trace 分析中提炼而来
  （调用频次分布、单工具重复率、每个工具对整体调用链的影响）。最终得到一套
  专用 [六工具](../tools/) 集，比通用 agent 工具包更稳定、更可预测。

## 另见

- [快速开始](../quickstart/)——安装并完成首次评审。
- [架构](../architecture/)——agent 循环、plan 阶段与记忆压缩。
- [CLI 参考](../cli-reference/)——每个参数与子命令。
- [集成](../integrations/)——从 Claude Code 或任意 agent 调用 OCR。
