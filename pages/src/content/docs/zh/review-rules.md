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

以下是内嵌 `system_rules.json` 的部分模式，按相对匹配顺序排列：

| 模式 | 规则文档 |
|---|---|
| `**/*.properties` | `properties.md`——i18n / 配置文件。 |
| `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md`——MyBatis 风格 mapper SQL。 |
| `**/pom.xml` | `pom_xml.md`——Maven 依赖。 |
| `**/build.gradle` | `build_gradle.md`——Gradle 依赖。 |
| `**/package.json` | `package_json.md`——NPM 依赖 / 脚本。 |
| `**/Cargo.toml` | `cargo_toml.md`——Rust manifest。 |
| `**/composer.json` | `composer_json.md`——Composer 依赖、自动加载、脚本、插件和包配置。 |
| `**/*.{json,json5}` | `json.md`——通用 JSON（也匹配 `.json5`）。 |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md`——GitHub Actions 工作流 YAML。 |
| `.github/**/*.{yaml,yml}` | `github_config.md`——其他 `.github` 配置 YAML。 |
| `**/*.{yaml,yml}` | `yaml.md` |
| `**/*.java` | `java.md` |
| `**/*.go` | `go.md`——Go 源代码。 |
| `**/*.{ftl,ftlh,ftlx}` | `freemarker.md`——FreeMarker 模板（SSTI / XSS / null 处理）。 |
| `**/*.ets` | `arkts.md`——ArkTS / HarmonyOS。 |
| `**/*.astro` | `astro.md`——Astro 组件与 islands。 |
| `**/*.{ts,js,tsx,jsx}` | `ts_js_tsx_jsx.md` |
| `**/*.{kt}` | `kotlin.md` |
| `**/*.rs` | `rust.md` |
| `**/*.R` | `r.md` |
| `**/*.{cpp,cc,hpp}` | `cpp.md` |
| `**/*.c` | `c.md` |
| `**/*.{py,ipynb}` | `python.md`——Python 源代码。 |
| `**/*.{php,phtml}` | `php.md`——PHP 源代码和 PHP 模板。 |
| `**/*.proto` | `protobuf.md`——Protocol Buffers 线协议兼容性。 |
| `**/*.po` | `po.md`——gettext 翻译源目录。 |
| `**/*.pot` | `pot.md`——gettext 模板文件。 |
| `**/*.{graphql,gql}` | `graphql.md`——GraphQL schema 与操作。 |
| `**/*.prisma` | `prisma.md`——Prisma schema。 |
| `**/*.jl` | `julia.md`——Julia 源代码。 |
| `**/*.{tf,hcl,tfvars}` | `terraform.md`——Terraform / HCL。 |
| `**/*.bicep` | `bicep.md`——Bicep（Azure）模板。 |
| `**/*.elm` | `elm.md` - Elm 源代码。 |
| `**/*.{jsonnet,libsonnet}` | `jsonnet.md`——Jsonnet 配置模板与库。 |
| `**/*.thrift` | `thrift.md`——Apache Thrift IDL 线协议兼容性。 |
| `**/*.capnp` | `capnp.md`——Cap'n Proto schema 线协议兼容性。 |
| `**/*.m` | 默认使用 `matlab.md`——见下方[针对 `.m` 文件的内容嗅探](#针对-m-文件的内容嗅探)。 |
| *(fallback)* | `default.md` |

解析出的规则正文成为 plan 和 main task prompt 中 `{{system_rule}}` 占位符的内容。

### 针对 `.m` 文件的内容嗅探

`.m` 被两种互不相关的语言共用——MATLAB 和 Objective-C——仅凭路径匹配无法区分。在回退到 `**/*.m` 匹配的 `matlab.md` 之前，OCR 会窥探文件的**首个非空行**：

| 首行内容 | 使用的规则文档 |
|---|---|
| `#import`、`#include`、`#pragma`、`#if`（也覆盖 `#ifdef`/`#ifndef`）、`#define`、`@import`、`@interface`、`@implementation`、`@class`、`@protocol`、`//` 或 `/*` | `objc.md` |
| 其他任何情况（包括没有内容可嗅探，例如文件已被删除） | `matlab.md` |

C 风格的注释起始符（`//` 或 `/*`）本身就是可靠的 ObjC 信号：MATLAB 的注释以 `%` 开头，而 `.m` 文件在 MATLAB 中不能以 `/` 合法开头，因此两者不会混淆。这一点很重要，因为 Xcode 的文件模板首行是 `//` 横幅注释，而大多数实际项目会先放置许可证头——真正的 `#import` 很少出现在第一行。

刻意没有把匹配范围扩大到单独的 `#`：Octave 同样使用 `.m` 扩展名，并把 `#` 当作注释符号，扩大匹配会把一个真正的 Octave/MATLAB 文件误判为 Objective-C。

内容是**在被审查的 ref 上**读取的，而不是从你的工作区读取：`ocr review --from/--to` 通过 `git show <to>:<path>` 读取，`--commit` 通过 `git show <commit>:<path>` 读取，因此即使该 ref 未被检出，嗅探结果依然正确。工作区审查、`ocr scan` 以及 `ocr rules check` 没有 ref，直接读取工作区——这正是它们所审查的对象。如果文件完全无法读取，解析将回退到 `matlab.md`。

`objc.md` 目前是通用 `default.md` 检查清单的副本——它是 OCR 源码中的占位文件（`internal/config/rules/rule_docs/objc.md`），留给日后有人补充真正的 Objective-C 专属指南；由于它通过 `go:embed` 编译进二进制文件，修改它需要从源码重新构建 OCR，而不是直接编辑磁盘上的文件。如果你现在就需要 Objective-C 专属指南而不想重新构建，可以使用项目级的 [`.opencodereview/rule.json`](#规则文件格式-层-1-3) 条目匹配你的 `.m` 路径（例如 `ios/**/*.m`）——项目规则的检查优先于系统层，因此无论嗅探结果如何都会优先生效。

`ocr rules check` 会通过单独的 `Note:` 行报告嗅探是否生效——`Pattern` 始终保持为匹配到的原始 glob：

```bash
$ ocr rules check ios/ViewController.m
File: ios/ViewController.m
Source: System built-in
Pattern: **/*.m
Note:    rule selected by file content (objc), not by path alone
Rule:
────────────────────────────────────────
…contents of objc.md…
────────────────────────────────────────
```

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
