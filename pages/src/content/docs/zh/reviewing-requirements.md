---
title: 结合需求进行评审
sidebar:
  order: 8
---

把验收标准提供给 OCR，让它在检查代码缺陷的同时核对业务行为。推荐做法是把需求
保存为本地 Markdown 文件，并用 `--background-file` 传入。

## 单次变更的需求文件

针对单次变更，可以先保存一份精简的本地 Markdown 文件，例如
`docs/requirements/checkout.md`：

```markdown
# Checkout requirements

Scope: src/checkout/**
Review changes against these criteria and cite the requirement ID in findings.

- PAY-1: Reject checkout when the cart is empty.
- PAY-2: Calculate the total on the server; never trust a client-supplied total.
- PAY-3: A repeated idempotency key must not create a second charge.

Out of scope: refunds and subscription billing.
```

配置好 OCR 并确保本地存在 `main` 分支后，在仓库根目录运行：

```bash
ocr review --from main --to HEAD --background-file docs/requirements/checkout.md
```

`--background-file` 为整个评审提供上下文。相对路径以**仓库根目录**为基准，
即使你在子目录中运行 OCR 也是如此；也可以使用绝对路径。如果同时传入 `--background`，
文件内容优先，内联文本会被忽略。请保持内容精简：清理后的内容超过 2,000 个字符时
会收到警告，超过 8,000 个字符则会被拒绝。输入文件不能超过 1 MiB，且清理后不能为空。

## 来自内部知识库的需求

对于内部知识库，请将相关需求、约束和验收标准导出为精简的本地 Markdown 文件，
再用 `--background-file` 传入。仅在 background 正文中提到文件名或附上远程链接，
不会自动加载该文档或继续读取其中的链接。请提供实际标准，并结合规格与测试核实评审发现。

## 当前限制

OCR 只把需求作为单次评审的上下文。目前没有独立于评审规则之外、将参考文档与模块或
目录关联的专门机制：路径规则决定匹配文件使用哪份规则文本，因此不适合用来附加
参考资料。如果知识库的使用场景逐渐成熟，可以另行设计专门的支持。

## 另见

- [CLI 参考](../cli-reference/)——`ocr review --background-file` 及其他评审参数。
- [评审规则](../review-rules/)——规则层级、路径规则与规则解析。
