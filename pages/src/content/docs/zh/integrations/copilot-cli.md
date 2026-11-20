---
title: GitHub Copilot CLI
sidebar:
  order: 3
---

借助与其他编码 agent 相同的可移植 [Agent Skill](../agent-skill/)，在
[GitHub Copilot CLI](https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli)
中端到端运行 OCR：评审 diff、分类发现，并修复值得采纳的问题。该集成是一个
复制即用的 skill——它不安装 hooks、不添加必需的指令文件，也不改变仓库行为。

## 仓库里有什么

工作流位于与 agent 无关的 skill 中：
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)。
一个 Copilot CLI 插件清单
[`plugins/open-code-review/plugin.json`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/plugin.json)
（Agent Plugins 1.0）将随附的 skill 打包，供 `copilot plugin install` 使用。
Copilot CLI 仍会从标准位置读取 skill，因此手动复制该 skill 目录同样可行。

## 安装

安装插件（推荐）：

```bash
copilot plugin install alibaba/open-code-review:plugins/open-code-review
```

这会安装 `open-code-review` 和 `open-code-review-delegate` 两个 skill。用
`copilot plugin uninstall open-code-review` 卸载。

Copilot CLI 的安装程序目前会警告：直接安装插件（repo/path/URL）已被弃用，建议
改用 marketplace 安装；将来 marketplace 条目可能让 `plugin@open-code-review`
成为推荐写法。

### 手动替代方案：复制 skill

把 skill 复制到 Copilot CLI 能发现的目录中。**项目级**（提交到仓库，让团队共享）：

```bash
mkdir -p .github/skills/open-code-review
curl -o .github/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

**用户级**（机器上每个项目都可用）：

```bash
mkdir -p ~/.copilot/skills/open-code-review
curl -o ~/.copilot/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

Copilot CLI 还会发现项目中的 `.agents/skills/` 和 `.claude/skills/`；同名时项目级
skill 优先于个人级。在会话内用 `/skills list` 查看已加载的 skill（在会话外用
`copilot skill list`）。手动复制的 skill 在 `/skills reload` 或开启新会话后生效；
已安装的插件则通过重新安装（再次执行 `copilot plugin install`）或
`copilot plugin update` 来更新。

## 使用

显式通过 `/open-code-review` 提及该 skill，或直接描述任务——Copilot 会根据
skill 的描述自动选用：

```
/open-code-review
/open-code-review review this PR against main
/open-code-review focus on race conditions in commit abc123
```

| 你说 | skill 会运行什么 |
| --- | --- |
| "review my changes" | `ocr review --audience agent` (staged, unstaged, untracked) |
| "review this PR" | `ocr review --from main --to <branch>` |
| "review commit abc123" | `ocr review --commit abc123` |

该 skill 用自身配置的模型驱动 `ocr`，并按严重程度分组报告发现。如果你不想为
`ocr` 单独配置 LLM，可以试试[委托模式](../delegate/)——由宿主 agent（Copilot）
提供模型，`ocr` 只负责选择文件和解析规则。

### 可选：让评审成为习惯

要让评审步骤从按需触发变为自动执行，请在 `.github/copilot-instructions.md` 或
`AGENTS.md` 中添加一条规则——只选其中一个文件，因为 Copilot 会合并多个指令文件
且没有优先级顺序：

```markdown
Before reporting a coding task complete, run `ocr review --audience agent`.
Fix all `critical` and `high` findings first.
```

是否安装这样一条规则由每个项目自行决定；本仓库不强制要求。

## 无头运行

```bash
copilot -p "Review the current branch with /open-code-review and report the findings." \
  --allow-tool='shell(ocr:*)'
```

- `--allow-tool='shell(ocr:*)'` 仅预先批准 `ocr` 的 shell 命令——并不授权编辑文件。
  在非交互式（`-p`）会话中，需要审批的操作会被拒绝而非暂停等待；交互式会话则会
  暂停等待审批。若要让 Copilot 应用修复，请以交互方式运行（或有意识地扩展允许
  列表以包含编辑工具）。
- 在不受信任的文件夹中，非交互式会话可能会跳过项目级 skill（`.github/skills/`）；
  Copilot CLI v1.0.107+ 提供 `--trust-folder`，可为单次会话信任该文件夹。
- 启用 Copilot 沙箱后，`ocr` 需要访问你的 LLM 端点的网络权限，以及
  `~/.opencodereview/` 的读写权限（它会在此保存会话状态）。
- `--output-format json` 输出的是 Copilot 的 JSONL 会话事件；评审结果本身是 `ocr`
  的 JSON，需显式运行 `ocr review --format json --audience agent` 获得。

若需要确定性的 CI 门禁，请跳过 agent，直接调用 `ocr`——参见 [CI/CD](../cicd/)。

## 注意事项

- **刻意不提供 hooks。** Copilot CLI 可以通过 `preToolUse` hooks 拦截
  `git commit` 或 `git push`，但 OCR 不提供这样的 hook：评审要运行 LLM（速度慢，
  且需要贡献者未必拥有的凭证），它评审的工作区并不是正在提交的内容，而失败即
  放行的 hook 宣称的强制力无法真正保证。只有在你能接受这些权衡时才添加自己的
  hook。
- **prompt 文件仅限 IDE。** Copilot CLI 不读取 `.github/prompts/*.prompt.md`；
  skill 才是受支持的接入方式。
- **发现不影响退出码。** 即使发现问题，`ocr` 在完成评审时仍以 0 退出——请根据
  `--format json` 中的严重程度做门禁，绝不要依赖退出状态。

## 故障排查

- **skill 未列出** — 确认路径（`~/.copilot/skills/` 或 `<repo>/.github/skills/`），
  然后运行 `/skills reload` 或开启新会话。
- **`ocr: command not found`** — 用
  `npm i -g @alibaba-group/open-code-review` 安装。
- **评审因 LLM 错误失败** — 配置一个提供方，并用 `ocr llm test` 测试；参见
  [配置](../configuration/)。
- **Copilot 反复请求批准 `ocr`** — 通过 `--allow-tool` 允许 `shell(ocr:*)`，
  或在提示处永久批准。
