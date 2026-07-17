---
title: 委托模式
sidebar:
  order: 5
---

OCR 负责确定性工程（文件筛选、规则解析），宿主 Agent 使用自身的 LLM 能力执行实际的代码审查。OCR 端无需配置 LLM。

## 何时使用委托模式

委托模式专为订阅制 AI 编码代理设计 — 如 Claude Code、Codex、Cursor、Open Code、Qoder 等。这些工具已内置 LLM 订阅额度，使用委托模式可以直接复用宿主 Agent 的订阅额度进行代码审查，无需额外配置模型或 API Key。

适用于以下场景：

1. 你的 AI 编码代理使用订阅制，希望复用已有额度进行代码审查 — 无需额外配置 API Key 或模型端点。
2. 你只需要 OCR 的工程脚手架 — 文件过滤、规则解析、排除逻辑 — 由宿主 Agent 负责所有 LLM 推理。
3. 你正在构建自定义 Agent 流水线，需要结构化输入（文件列表 + 规则）作为自身审查步骤的输入。

## 前置条件

需要安装 `ocr` CLI：

```bash
which ocr || npm install -g @alibaba-group/open-code-review
```

无需配置 LLM（`ocr config set …` 或环境变量）— 委托模式在 OCR 端不调用任何 LLM。

## 安装 Skill / Command

### Claude Code — Command

```bash
mkdir -p .claude/commands
curl -o .claude/commands/delegate-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

### 任意 Agent — Skill

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

或手动复制：

```bash
cp -R /path/to/open-code-review/skills/open-code-review-delegate ~/.claude/skills/
```

## 工作流程

### 第 1 步：Preview — 确定审查范围

```bash
ocr delegate preview [--from <ref> --to <ref>] [--commit <hash>] [--exclude <patterns>]
```

输出内容：

- **mode** — workspace / range / commit
- **ref 元数据** — from、to、commit、merge\_base
- **可审查文件列表** — 路径、状态、插入/删除行数
- **已排除文件** — 及排除原因

常见用法：

| 场景 | 命令 |
|------|------|
| 工作区变更 | `ocr delegate preview` |
| 分支对比 | `ocr delegate preview --from main --to feature` |
| 单次提交 | `ocr delegate preview -c abc123` |

### 第 2 步：获取文件规则

```bash
ocr delegate rule <path1> <path2> ...
```

传入第 1 步中的可审查文件路径。输出按规则内容分组 — 共享相同规则的文件归为一组，避免重复。

### 第 3 步：获取 diff

根据第 1 步的 mode/ref 信息，使用 git 直接获取：

**Range 模式**（有 merge\_base）：
```bash
git diff <merge_base>..<to> -- <path>
```

**Commit 模式**：
```bash
git show <commit> -- <path>
```

**Workspace 模式**：
```bash
git diff HEAD -- <path>        # 已跟踪文件
cat <path>                     # 新的未跟踪文件
```

### 第 4 步：审查每个文件

对每个可审查文件：

1. 获取其 diff（第 3 步）
2. 参照匹配的规则组（第 2 步）作为审查清单
3. 进行深入审查，按需探索上下文

### 第 5 步：报告

按严重程度分类：

- **Critical/High** — Bug、安全问题、数据丢失风险。始终报告。
- **Medium** — 性能问题、错误处理缺失。附带上下文报告。
- **Low** — 风格建议、细微改进。静默丢弃，除非确有价值。

## 子命令参考

| 命令 | 用途 |
|------|------|
| `ocr delegate preview` | 列出可审查文件 + mode/ref 元数据 |
| `ocr delegate rule <path...>` | 按内容分组解析审查规则 |

## 通用标志

| 标志 | 描述 |
|------|------|
| `--from <ref>` | Range 模式的源引用 |
| `--to <ref>` | Range 模式的目标引用 |
| `-c, --commit <hash>` | 单次提交模式 |
| `--repo <path>` | 仓库根目录（默认：cwd） |
| `--rule <path>` | 自定义 rule.json 路径 |
| `--exclude <patterns>` | 逗号分隔的排除模式 |
| `-b, --background <text>` | 业务上下文 |
| `-B, --background-file <path>` | 从 Markdown 文件读取业务上下文 |

## 与其他集成方式的对比

| 模式 | 谁调用 LLM？ | 适用场景 |
|------|-------------|----------|
| [Agent Skill](../agent-skill/) | OCR | Agent 调用 `ocr review`，OCR 驱动完整审查 |
| [Command（Claude Code）](../claude-code/) | OCR | Claude Code 中的斜杠命令，OCR 驱动审查 |
| **委托模式** | 宿主 Agent | OCR 提供脚手架，Agent 驱动审查 |

## 另请参阅

- [Agent Skill](../agent-skill/) — OCR 代表 Agent 驱动完整审查。
- [Command（Claude Code）](../claude-code/) — 斜杠命令风格，含自动修复。
