---
title: Agent Skill
sidebar:
  order: 1
---

把 OCR 注册为可调用的 skill，使 agent 框架能以正确的参数和发现核验流程
调用它——无需你在调用侧重新推导这些。

## 仓库里有什么

仓库在
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
提供 SKILL manifest，定义如何调用 OCR、核验发现、报告不确定性以及按需修复。
skill 目录还包含 GitHub PR 评审脚本；请安装完整目录，而不只是 SKILL.md。

## 安装

### 方式 1：`npx skills add`（推荐）

在希望 skill 可用的项目内运行：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

这从
[skills registry](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
安装 skill 目录并放入项目，使任何尊重 skills 约定的编码 agent 在下次调用时加载
它。重新运行该命令以更新 skill 到最新版本。

> **前置条件：** 原生模式需要在本地配置 OCR 模型提供商，请勿将凭证粘贴到对话中。
> 若缺少 `ocr`，skill 的故障排查说明包含通过
> `npm install -g @alibaba-group/open-code-review` 安装的步骤。
> 只有 GitHub PR 模式还需要 Python 3 和已认证的 `gh`。见[配置](../../configuration/)。

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

1. **提取业务上下文。** 检查评审目标（commit、分支、工作副本）并生成一个简短的
   `--background` 字符串。
2. **运行所请求的评审。** 本地评审直接调用
   `ocr review --audience agent --background "…"`，已提交的变更按需添加
   `--commit` 或 `--from`/`--to`，正常路径不先探测安装或连接状态。
   新的 commit 或区间评审默认添加 `--analysis`，未指定其他格式时使用 JSON。
   工作副本、预览、恢复、SARIF 输出，以及明确要求跳过分析时不添加分析。
   调用失败后先排查，再重试。
   GitHub PR 编号或 URL 则使用已安装的
   `python3 "$SKILL_DIR/scripts/review_pr.py" <id-or-url>` 脚本。
   编号使用当前检出目录的 `gh` 默认仓库，在 fork 中可能指向上游。
   报告中列出 PR URL、评审版本、快照是否仍为最新以及产物路径。
3. **核验与报告。** 对照评审代码检查每条发现，包括低严重程度的发现。
   分为 **Reproduced（已复现）** 和 **Uncertain（未确定）** 两组，保留 OCR 的严重程度及组内顺序。
   只有具体证据反驳发现时才省略，不能仅因不确定就丢弃。
   保留原始结果，报告 Review 覆盖范围和缺口。联合运行时单独呈现 Analysis Markdown，
   区分分析执行状态与模型对完整性的判断。
4. **按需修复。** 仅在用户要求修改时应用安全且已核实的修复，重点处理 critical、high 和 medium 发现。
   仅评审的请求不授权修改代码。PR 模式下保留快照，直到用户明确同意清理；清理后仍保留报告。

完整 prompt——包括核验要求、报告方式、PR 模式限制与注意事项——位于
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
所述工作流——包括调用失败后的排查，以及需要凭证时请用户在本地配置。

## 其他 agent 框架

任何有“注册外部 skill”接口的框架都能摄入 SKILL.md——它只是带 frontmatter 的
markdown。若你的框架期望不同 schema，markdown 正文仍可用作 prompt 模板。

## 另见

- [Command（Claude Code Plugin）](../claude-code/)——同一 skill 的
  slash-command 版本。
