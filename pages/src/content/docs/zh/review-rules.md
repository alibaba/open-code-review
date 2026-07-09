---
title: 评审规则
sidebar:
  order: 7
---

规则告诉 OCR 评审每个文件时**应关注什么**。它们存放在三层的 JSON 文件中，
外加随二进制发布的一个内嵌系统默认规则。

## 优先级链

OCR 用一条**四层优先级链**解析规则。对每个文件路径，按序尝试各层；第一个匹配
的模式生效。

| 优先级 | 来源 | 路径 | 说明 |
|---|---|---|---|
| 1（最高） | `--rule` 参数 | 用户指定 | CLI 覆盖；只要提供就总是生效。 |
| 2 | 项目配置 | `<repoDir>/.opencodereview/rule.json` | 项目级规则——可安全提交。 |
| 3 | 全局配置 | `~/.opencodereview/rule.json` | 用户级偏好。 |
| 4（最低） | 系统默认 | 内嵌 `system_rules.json` | 覆盖常见语言的内置规则。 |

若更高优先级层的文件不存在，会被静默跳过——不是错误。因此从未添加
`.opencodereview/rule.json` 的项目会直接落到全局 / 系统层。

系统层**始终**存在（随二进制发布），因此总会解析出*某个*规则。

## 规则文件格式（层 1–3）

```json
{
  "include": ["src/**/*.{ts,tsx}", "src/**/*.go"],
  "exclude": ["**/*.test.ts", "**/generated/**"],
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "All exported handlers must validate request bodies before use."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags."
    }
  ]
}
```

三个独立字段：

- `include`——可选。glob 模式，用于*绕过*内置的默认排除模式（测试文件排除——见
  下文）。它不是白名单：不匹配任何 `include` 模式的文件仍会经过
  `unsupported_ext` 和 `default_path` 检查，可能仍被评审。
- `exclude`——可选。OCR 不予评审的文件 glob 模式。过滤中优先级最高。
- `rules`——`{path, rule}` 条目数组，按**声明顺序**求值。第一个 `path` glob
  匹配该文件的条目，决定 OCR 发给模型的 prompt。

### glob 能力

OCR 用 [`bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4)
做匹配：

- `*`——匹配除 `/` 外的任意字符。
- `**`——跨目录边界匹配（`src/**/*.go` 覆盖任意深度）。
- `{a,b,c}`——花括号展开。`*.{ts,tsx,js,jsx}` 展开为四个模式并依次匹配。
- `?`——匹配单个字符。
- `[abc]`——字符类。

> 模式匹配**不区分大小写**（匹配前文件路径会被小写化）。不确定时用
> `ocr rules check <path>` 确认。

## 文件如何被过滤

过滤是一个五重门算法，位于
[`internal/agent/preview.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/preview.go)。
对每个 diff，OCR 依次问：

1. **`binary`**——文件是二进制吗？排除。
2. **`user_exclude`**——路径匹配任何用户 `exclude` 模式吗？排除。
3. **`user_include`**——若用户定义了 `include`，路径匹配吗？若是，**立即保留**
   （绕过下面的 `unsupported_ext` 和 `default_path` 门）。
4. **`unsupported_ext`**——文件扩展名在
   [白名单](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)
   里吗？不在则排除。
5. **`default_path`**——路径匹配某个内置测试文件排除模式
   （`**/*_test.go`、`**/*.test.{js,jsx,ts,tsx}`、`**/*_spec.rb`……）吗？排除。

通过全部五重门的文件才发给 LLM。`deleted` 原因（不是门——它在 `Preview()` 中
单独计算）标记新路径为 `/dev/null` 的文件；没有新内容可评审。用
`ocr review --preview` 可在不花 token 的情况下打印此过滤结果。

### 默认路径排除

内置排除列表（见
[`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json)）
匹配测试文件模式：

- `**/*_test.go`
- `**/src/test/java/**/*.java`
- `**/src/test/**/*.kt`
- `**/*.test.{js,jsx,ts,tsx}`
- `**/*.spec.{js,jsx,ts,tsx}`
- `**/__tests__/**`
- `**/test/**/*_test.py`
- `**/tests/**/*_test.py`
- `**/*_test.py`
- `**/*_spec.rb`
- `**/spec/**/*_spec.rb`
- `**/*Test.java`
- `**/*Tests.java`
- `**/*_test.rs`
- `**/oh_modules/**`
- `**/*.test.ets`

噪声目录过滤（`vendor/`、`node_modules/`、`target/`……）发生在更早的阶段，位于
[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go)
的 diff 层，先于 per-file 过滤运行。

要**评审**一个匹配这些测试文件模式的文件，把它加入用户 `include` 列表——那会
覆盖 default-path 门。

## 每文件的规则解析

过滤决定某文件*将被*评审后，OCR 选择 agent 应遵循的规则文本：

1. 按声明顺序试 `--rule`（custom）层。
2. 按声明顺序试 `<repo>/.opencodereview/rule.json`。
3. 按声明顺序试 `~/.opencodereview/rule.json`。
4. 回退到内嵌系统规则层。

内嵌 `system_rules.json` 自带这些模式（按序）：

| 模式 | 规则文档 |
|---|---|
| `**/*.properties` | `properties.md`——i18n / 配置文件。 |
| `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md`——MyBatis 风格 mapper SQL。 |
| `**/pom.xml` | `pom_xml.md`——Maven 依赖。 |
| `**/build.gradle` | `build_gradle.md`——Gradle 依赖。 |
| `**/package.json` | `package_json.md`——NPM 依赖 / 脚本。 |
| `**/Cargo.toml` | `cargo_toml.md`——Rust manifest。 |
| `**/*.{json,json5}` | `json.md`——通用 JSON（也匹配 `.json5`）。 |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md`——GitHub Actions 工作流 YAML。 |
| `.github/**/*.{yaml,yml}` | `github_config.md`——其他 `.github` 配置 YAML。 |
| `**/*.{yaml,yml}` | `yaml.md` |
| `**/*.java` | `java.md` |
| `**/*.ets` | `arkts.md`——ArkTS / HarmonyOS。 |
| `**/*.{ts,js,tsx,jsx}` | `ts_js_tsx_jsx.md` |
| `**/*.{kt}` | `kotlin.md` |
| `**/*.rs` | `rust.md` |
| `**/*.{cpp,cc,hpp}` | `cpp.md` |
| `**/*.c` | `c.md` |
| *(fallback)* | `default.md` |

解析出的规则正文成为 plan 和 main task prompt 中 `{{system_rule}}` 占位符的内容。

## 查看哪条规则生效：`ocr rules check`

```bash
$ ocr rules check src/main/java/com/example/UserService.java
File: src/main/java/com/example/UserService.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
…contents of java.md…
────────────────────────────────────────
```

```bash
$ ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml
File: src/main/resources/mapper/UserMapper.xml
Source: Custom (--rule)
Pattern: **/*mapper*.xml
Rule:
────────────────────────────────────────
…contents of your custom rule…
────────────────────────────────────────
```

当某条规则未按预期生效时用它——它会显示生效的**层**与**模式**。

## 配方

### 项目级：强制编码规范

保存为 `<repo>/.opencodereview/rule.json` 并提交：

```json
{
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "Every public handler must `defer tx.Rollback()` immediately after starting a transaction."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, missing parameter binding, and unclosed XML tags."
    }
  ]
}
```

### 项目级：跳过生成代码，聚焦 src

```json
{
  "include": ["src/**/*.{ts,tsx,js,jsx}"],
  "exclude": ["**/*.gen.ts", "**/generated/**"]
}
```

设置 `include` 后，`src/` 内的文件即使本会被内置默认排除模式（如测试文件）剔除
也会被保留。`src/` 之外的文件仍走正常的 ext / default 检查——`include` 是绕过机制，
不是白名单。

### 按 PR 覆盖

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

同时绕过项目层与全局层——当单个 PR 需要完全不同的评审清单（如仅安全评审）时
很方便。

### 全局个人偏好

放到 `~/.opencodereview/rule.json`，你机器上每个仓库都会继承：

```json
{
  "rules": [
    {
      "path": "**/*.{ts,tsx,js,jsx}",
      "rule": "Always check for unhandled promise rejections; warn on `// eslint-disable` without a reason comment."
    }
  ]
}
```

## 另见

- [CLI 参考](../cli-reference/)——`ocr review --rule`、`--preview` 与 `ocr rules check`。
- [配置](../configuration/)——config 文件位置与分层解析链。
- [架构](../architecture/)——解析出的规则如何馈入 agent prompt。
