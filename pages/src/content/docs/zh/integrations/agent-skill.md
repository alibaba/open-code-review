---
title: Agent Skill
sidebar:
  order: 1
---

把 OCR 注册为可调用的 skill，使 agent 框架能以正确的参数、前置检查与分级标准
调用它——无需你在调用侧重新推导这些。

## 仓库里有什么

仓库在
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
提供 SKILL manifest。它把 OCR 声明为可调用 skill，含前置检查、调用工作流与
评论分级标准（High/Medium/Low）。

## 安装

### 方式 1：`npx skills add`（推荐）

在希望 skill 可用的项目内运行：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

这从
[skills registry](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
拉取 manifest 并放入项目，使任何尊重 skills 约定的编码 agent 在下次调用时加载
它。重新运行该命令以更新 skill 到最新版本。

> **前置条件：** 首次运行时 skill 会自行安装 `ocr` CLI
> （通过 `npm install -g @alibaba-group/open-code-review`），前提是二进制不在
> `PATH` 上——见[skill 做什么](#what-the-skill-does)。你**确实**需要预先配置好
> LLM；skill 无法替你完成，会停下来询问。见[配置](../../configuration/)。

### 方式 2：手动复制（系统级）

若想全局安装 skill 而非按项目，把文件夹复制进你的 skills 目录：

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

这使 skill 在机器上每个项目可用。

## skill 做什么

SKILL.md 是一个 prompt：当调用方 agent 加载它时，由 agent 自身执行步骤。一次
完整的 `/open-code-review`（或等价）请求流程如下展开：

1. **前置检查。** 运行 `which ocr` 确认 CLI 在 `PATH` 上，再 `ocr llm test`
   确认 LLM 可达。
2. **CLI 缺失则自动安装。** 若 `which ocr` 报告 "NOT INSTALLED"，agent 运行
   `npm install -g @alibaba-group/open-code-review` 并继续。不提示用户——这被视为
   常规设置步骤。
3. **无 LLM 配置则停下询问。** 若 `ocr llm test` 失败，agent *不会* 编造凭证。
   它向用户展示两种受支持的方式（环境变量或 `ocr config set …`）并等待用户提供
   API key。
4. **提取业务上下文。** 检查评审目标（commit、分支、工作副本）并生成一个简短的
   `--background` 字符串。
5. **运行评审。** 调用
   `ocr review --audience agent --background "…" [--commit | --from/--to]`，
   根据用户是要评审工作副本、特定 commit 还是分支区间来选择参数。
6. **分类与报告。** 用 SKILL.md 中的标准把 JSON 评论分为 **High** /
   **Medium** / **Low**（bug 与安全问题为 High；吹毛求疵与疑似误报被静默丢弃），
   再渲染 Markdown 摘要。
7. **按需修复。** 若用户说“评审**并**修复”（或类似），对 High/Medium 项内联
   应用安全修复；否则修改代码前先询问。

完整 prompt——包括确切分级标准、输出模板与注意事项——位于
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)。
如想收紧上述任一项（比如把默认行为改为修复前总先询问），编辑你本地副本。

## Anthropic Agent SDK

把你的 SDK init 指向已安装的 skill 路径：

```python
from anthropic_agent_sdk import Agent

agent = Agent(
    skill_paths=["/path/to/open-code-review/skills/open-code-review"],
)

agent.run("Review my staged changes — focus on race conditions.")
```

SDK 加载 SKILL.md prompt，由 agent 执行[skill 做什么](#what-the-skill-does)中
所述工作流——包括 `npm install` 回退与无 LLM 配置时提示输入凭证的步骤。

## 其他 agent 框架

任何有“注册外部 skill”接口的框架都能摄入 SKILL.md——它只是带 frontmatter 的
markdown。若你的框架期望不同 schema，markdown 正文仍可用作 prompt 模板。

## 另见

- [Command（Claude Code Plugin）](../claude-code/)——同一 skill 的
  slash-command 版本。
- [Direct Subprocess](../subprocess/)——绕过 manifest，自行调用 CLI。
