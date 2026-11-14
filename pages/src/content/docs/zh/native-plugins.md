---
title: 原生插件
sidebar:
  order: 8
---

原生插件将额外评审规则打包在本地目录中。OCR 会将目录复制为用户级快照；
启用后，插件规则适用于所有仓库中的 `ocr review` 和 `ocr scan`。
包内文件按数据读取。

## 安装与检查

在 OCR 源码仓库中安装自带示例：

```bash
ocr plugin install ./examples/ocr-plugins/review-checklist
ocr plugin list
ocr plugin show review-checklist
ocr plugin enable review-checklist
ocr rules check src/main.go
```

安装后默认禁用。添加 `--enable` 可在安装时启用。
`ocr rules check` 会显示每个匹配插件的 ID、版本、规则包路径、匹配模式，
以及合并后的规则文本。所有插件子命令均支持 `--json`，例如
`ocr plugin list --json`。

启用前请检查规则文本，它会进入模型的评审提示词。
安装源必须是包含 `ocr-plugin.json` 的本地目录。
不接受 URL、GitHub 仓库引用、ZIP 或 tar.gz 压缩包。

## 包格式

示例包含三个文件：

```text
review-checklist/
  ocr-plugin.json
  rules.json
  checklist.md
```

`ocr-plugin.json`：

```json
{
  "schema_version": 1,
  "id": "review-checklist",
  "name": "Review Checklist",
  "version": "1.0.0",
  "description": "Additional review checks for error handling and resource ownership.",
  "rules": ["rules.json"],
  "requires": {"features": ["rules"]}
}
```

| 字段 | 约束 |
|---|---|
| `schema_version` | 必填，值为 `1`。 |
| `id` | 必填，1–64 个小写 ASCII 字母、数字、`.`、`_`、`-`，首尾为字母或数字；禁止 Windows 保留名称。 |
| `name`、`version` | 必填，非空白字符串。 |
| `description` | 可选，显示在 `ocr plugin show` 中。 |
| `rules` | 必填，非空的有序列表，列出包内相对路径的 `.json` 文件，路径不能重复。 |
| `requires.ocr_min_version` | 可选，完整语义版本，例如 `1.0.0`，与 OCR 构建版本比较。 |
| `requires.features` | 可选，所需能力列表，当前支持 `rules`。 |
| `metadata` | 可选 JSON 对象，作为不解释执行的数据保留。 |

清单中的未知字段和重复 JSON 键会被拒绝。安装、启用及加载已启用插件时会检查兼容性。
`dev` 构建无法满足 `ocr_min_version`；用于开发构建的包可省略该要求。

`rules.json`：

```json
{
  "rules": [
    {
      "path": "**/*.{go,js,mjs,ts,tsx,py}",
      "rule": "checklist.md"
    }
  ]
}
```

`checklist.md` 包含评审指引：

```text
Check error propagation, cleanup, and resource ownership.
Report only concrete failure paths in the changed code.
Verify the relevant caller and explain the trigger and user-visible consequence.
```

`rule` 也可直接填写文本。Markdown 引用相对于规则包文件解析，且必须位于包内。
路径使用正斜杠。规则包用于提供评审规则；`include` 和 `exclude` 应在项目或
用户规则中配置。插件中非空的 `include` 或 `exclude` 列表会被拒绝。

## 规则如何组合

OCR 先沿现有优先级解析基础规则：`--rule`、项目、全局、系统。
随后按插件 ID 升序、再按清单中的 `rules` 顺序追加已启用插件的规则。
每个规则包仅采用第一条匹配项，因此多个匹配规则包可同时为同一文件添加指引。

插件规则始终追加，包内 `merge_system_rule` 不生效。评审文件仍由现有过滤规则决定。
模式、文件过滤和基础优先级详见[评审规则](../review-rules/)。

## 禁用、替换与移除

```bash
ocr plugin disable review-checklist
ocr plugin remove review-checklist
ocr plugin install ./examples/ocr-plugins/review-checklist --enable
```

替换插件前需先移除已安装的 ID。修改源目录后，重新安装才会生效。
状态记录在 `~/.opencodereview/plugins/state.json`，快照位于 `plugins/packages/`。
移除操作注销插件，并保留快照供正在运行的评审使用。

恢复评审时，配置标识包含已启用插件的清单和包内容。
重新安装相同内容会保持该标识；内容改变后，旧插件配置下的结果将不再复用。

包大小上限为 64 MiB，最多 4,096 个条目（包括目录）。清单上限为 1 MiB；
每个规则包 JSON 在加载时限制为 512 KiB。符号链接、特殊文件、仅大小写不同的
冲突名称、Windows 保留文件名以及逃逸包目录的路径均会被拒绝。
请保留快照原样，通过修改源目录并重新安装来更新规则。

命令汇总见 [CLI 参考](../cli-reference/#ocr-plugin)。
