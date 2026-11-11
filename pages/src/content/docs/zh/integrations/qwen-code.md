---
title: Qwen Code
sidebar:
  order: 3
---

通过仓库现有的 Claude Code Marketplace 插件，在 Qwen Code 中使用 OCR。
安装程序会转换插件并加载评审命令。

以下步骤已在 Windows 上使用 Qwen Code 0.27.2 验证。独立发布的
[Qwen Code 1.0 Preview](https://github.com/QwenLM/qwen-code/releases/tag/v1.0.0-preview)
兼容性尚未验证。

## 前置条件

安装 Qwen Code 和 Git 2.41 或更高版本，然后安装 OCR CLI：

```bash
npm install -g @alibaba-group/open-code-review
```

## 安装

在终端运行：

```bash
qwen extensions install alibaba/open-code-review:open-code-review
qwen extensions list
```

通过 CLI 安装后，开启新的 Qwen Code 会话。该插件复用仓库中
[随附的命令提示词](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code/commands)。

## 运行评审

配置并测试 OCR 将调用的 LLM：

```bash
ocr config provider
ocr config model
ocr llm test
```

启用 Qwen Code 自带的 `/review` 时，用以下命令调用 OCR：

```text
/open-code-review.review
/open-code-review.review --from main --to feature
/open-code-review.review --commit abc123
```

Qwen Code 保留自带的 `/review`，为冲突的 OCR 命令添加 `open-code-review.` 前缀。
如果禁用了自带的评审 skill，OCR 可以使用 `/review`。在会话中运行 `/help`
查看实际加载的命令名称。

## 委托模式

要使用 Qwen Code 自身的模型和认证进行评审，运行：

```text
/delegate-review
/delegate-review --from main --to feature
/delegate-review --commit abc123
```

OCR 运行 `ocr delegate preview` 和 `ocr delegate rule`，负责选择文件和解析规则；
由 Qwen Code 执行评审。此模式需要 OCR CLI，无需单独配置 OCR 的 LLM。

如果其他命令已使用 `/delegate-review`，Qwen Code 会将 OCR 的命令注册为
`/open-code-review.delegate-review`。

两个随附命令都可能自动修复其采纳的问题。

## 市场发现

将本仓库添加为来源，即可在 Discover 页面浏览插件：

```bash
qwen extensions sources add alibaba/open-code-review
```

在 Qwen Code 中运行 `/extensions` 并打开 Discover。

## 另见

- [委托模式](../delegate/)
- [配置](../configuration/)
- [Qwen Code 扩展文档](https://qwenlm.github.io/qwen-code-docs/zh/users/extension/introduction/)
