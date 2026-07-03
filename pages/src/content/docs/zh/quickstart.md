---
title: 快速开始
sidebar:
  order: 3
---

几分钟内跑通第一次代码评审。

## 前置条件

- **Git ≥ 2.41**
- **Node.js ≥ 18**
- **LLM API key**

## 第 1 步 —— 安装 CLI

```bash
npm install -g @alibaba-group/open-code-review
ocr version
```

> 更多方式见 [安装](../installation/)。

## 第 2 步 —— 配置 LLM

```bash
ocr config provider
```

它会让你选择一个内置或自定义 provider、填入 API key、挑选 model，保存到配置文件后自动运行一次 `ocr llm test` 验证端点。之后想换模型：

```bash
ocr config model
```

### 备选:非交互命令

在 CI 或无 TUI 的环境里,用 `ocr config set` 直接写入同一份配置:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

## 第 3 步 —— 测试连通性

```bash
ocr llm test
```

如果报出 `no valid LLM endpoint configured` 这类错误,请重新检查第 2 步的配置。 401 / 403 表示 token 错误或已过期。

## 第 4 步 —— 运行第一次评审

进入任意 Git 仓库并运行：

```bash
cd path/to/your-repo

# 工作区模式 —— 评审 staged + unstaged + untracked 变更（默认）
ocr review

# 分支区间 —— 评审 `main..feature-branch`
ocr review --from main --to feature-branch

# 单个 commit —— 评审该 commit 引入的 diff
ocr review --commit abc123
```

> `ocr review` 的完整参数（并发调优、输出格式、audience模式、背景上下文等）及其他所有子命令见 [CLI 参考](../cli-reference/)。

### 想先看看会评审什么？

```bash
ocr review --preview              # 工作区
ocr review -c abc123 --preview    # commit
```

### 面向系统的 JSON 输出

`--audience agent` 屏蔽人性化的进度 UI,让 stdout 只剩 JSON / 最终摘要 —— 正是上游 agent 或 CI 脚本所需。

```bash
ocr review --format json --audience agent > review.json
```

## 另见

- [安装](../installation/) —— 全部安装方式与 OCR 的状态目录。
- [配置](../configuration/) —— 每个环境变量、config key 与内置 provider。
- [CLI 参考](../cli-reference/) —— 每个子命令、参数与输出模式。
- [评审规则](../review-rules/) —— 自定义评审内容。
- [集成](../integrations/) —— 把 OCR 嵌入 Claude Code、Agent skill 或 CI。
- [FAQ](../faq/) —— 已知错误与对策。
