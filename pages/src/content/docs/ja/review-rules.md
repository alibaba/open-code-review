---
title: レビュールール
sidebar:
  order: 7
---

ルールは、各ファイルをレビューする際に OCR が**何に注目すべきか**を伝えます。ルールは 3 層の JSON ファイルに格納され、加えてバイナリに同梱される埋め込みのシステムデフォルトルールがあります。

## 優先順位チェーン

OCR は**4 層の優先順位チェーン**でルールを解決します。各ファイルパスについて、層を順に試し、最初に一致したパターンが有効になります。

| 優先順位 | 出所 | パス | 説明 |
|---|---|---|---|
| 1（最高） | `--rule` 引数 | ユーザー指定 | CLI による上書き。指定されている限り常に有効になります。 |
| 2 | プロジェクト設定 | `<repoDir>/.opencodereview/rule.json` | プロジェクトレベルのルール。安全に commit できます。 |
| 3 | グローバル設定 | `~/.opencodereview/rule.json` | ユーザーレベルの好み。 |
| 4（最低） | システムデフォルト | 埋め込み `system_rules.json` | 一般的な言語をカバーする組み込みルール。 |

より高い優先順位の層のファイルが存在しない場合は静かにスキップされます。エラーではありません。したがって `.opencodereview/rule.json` を一度も追加していないプロジェクトは、そのままグローバル / システム層に落ちます。

システム層は**常に**存在するため（バイナリに同梱）、必ず*何らかの*ルールが解決されます。

## ルールファイル形式（層 1〜3） {#rule-file-format-layers-1-3}

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

3 つの独立したフィールドがあります:

- `include`: 任意。組み込みのデフォルト除外パターン（テストファイルの除外。下記参照）を*バイパス*するための glob パターンです。ホワイトリストではありません。どの `include` パターンにも一致しないファイルも、依然として `unsupported_ext` と `default_path` のチェックを通過し、レビューされる可能性があります。
- `exclude`: 任意。OCR がレビューしないファイルの glob パターンです。フィルタリングで最も優先されます。
- `rules`: `{path, rule}` エントリの配列で、**宣言順**に評価されます。そのファイルに最初に一致した `path` glob のエントリが、OCR がモデルに送る prompt を決定します。

### glob の機能

OCR は [`bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4) でマッチングを行います:

- `*`: `/` 以外の任意の文字に一致します。
- `**`: ディレクトリ境界をまたいで一致します（`src/**/*.go` は任意の深さをカバー）。
- `{a,b,c}`: 波括弧の展開。`*.{ts,tsx,js,jsx}` は 4 つのパターンに展開され、順に一致が試されます。
- `?`: 単一の文字に一致します。
- `[abc]`: 文字クラス。

> パターンマッチングは**大文字小文字を区別しません**（マッチング前にファイルパスは小文字化されます）。確信が持てないときは `ocr rules check <path>` で確認してください。

## ファイルがどのようにフィルタリングされるか

フィルタリングは 5 段階のゲートアルゴリズムで、[`internal/agent/preview.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/preview.go) にあります。各 diff について、OCR は順に次を問います:

1. **`binary`**: ファイルはバイナリか？ 除外します。
2. **`user_exclude`**: パスがいずれかのユーザー `exclude` パターンに一致するか？ 除外します。
3. **`user_include`**: ユーザーが `include` を定義している場合、パスは一致するか？ 一致するなら**即座に保持**します（下記の `unsupported_ext` と `default_path` のゲートをバイパス）。
4. **`unsupported_ext`**: ファイルの拡張子は[ホワイトリスト](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)にあるか？ なければ除外します。
5. **`default_path`**: パスがいずれかの組み込みテストファイル除外パターン（`**/*_test.go`、`**/*.test.{js,jsx,ts,tsx}`、`**/*_spec.rb`……）に一致するか？ 除外します。

5 つのゲートをすべて通過したファイルだけが LLM に送られます。`deleted` の理由（これはゲートではなく、`Preview()` の中で個別に計算されます）は、新しいパスが `/dev/null` であるファイルを示します。レビューすべき新しい内容がありません。`ocr review --preview` を使えば、token を消費せずにこのフィルタリング結果を出力できます。

### デフォルトパスの除外

組み込みの除外リスト（[`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json) を参照）は、テストファイルのパターンに一致します:

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

ノイズディレクトリのフィルタリング（`vendor/`、`node_modules/`、`target/`……）は、より早い段階、[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go) の diff 層で発生し、ファイルごとのフィルタリングより先に実行されます。

これらのテストファイルパターンに一致するファイルを**レビューする**には、それをユーザー `include` リストに追加してください。それが default-path ゲートを上書きします。

## ファイルごとのルール解決

フィルタリングによってあるファイルが*レビューされる*と決まったあと、OCR は agent が従うべきルールテキストを選びます:

1. 宣言順に `--rule`（custom）層を試します。
2. 宣言順に `<repo>/.opencodereview/rule.json` を試します。
3. 宣言順に `~/.opencodereview/rule.json` を試します。
4. 埋め込みのシステムルール層にフォールバックします。

埋め込みの `system_rules.json` から主なパターンを相対的なマッチ順で示します:

| パターン | ルールドキュメント |
|---|---|
| `**/*.properties` | `properties.md`: i18n / 設定ファイル。 |
| `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md`: MyBatis 形式の mapper SQL。 |
| `**/pom.xml` | `pom_xml.md`: Maven 依存関係。 |
| `**/build.gradle` | `build_gradle.md`: Gradle 依存関係。 |
| `**/package.json` | `package_json.md`: NPM 依存関係 / スクリプト。 |
| `**/Cargo.toml` | `cargo_toml.md`: Rust manifest。 |
| `**/composer.json` | `composer_json.md`: Composer の依存関係、自動読み込み、スクリプト、プラグイン、パッケージ設定。 |
| `**/*.{json,json5}` | `json.md`: 汎用 JSON（`.json5` にも一致）。 |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md`: GitHub Actions ワークフロー YAML。 |
| `.github/**/*.{yaml,yml}` | `github_config.md`: その他の `.github` 設定 YAML。 |
| `**/*.{yaml,yml}` | `yaml.md` |
| `**/*.java` | `java.md` |
| `**/*.go` | `go.md`: Go ソースコード。 |
| `**/*.{ftl,ftlh,ftlx}` | `freemarker.md`: FreeMarker テンプレート（SSTI / XSS / null 処理）。 |
| `**/*.ets` | `arkts.md`: ArkTS / HarmonyOS。 |
| `**/*.astro` | `astro.md`: Astro コンポーネントと islands。 |
| `**/*.{ts,js,tsx,jsx}` | `ts_js_tsx_jsx.md` |
| `**/*.{kt}` | `kotlin.md` |
| `**/*.rs` | `rust.md` |
| `**/*.R` | `r.md` |
| `**/*.{cpp,cc,hpp}` | `cpp.md` |
| `**/*.c` | `c.md` |
| `**/*.{py,ipynb}` | `python.md`: Python ソースコード。 |
| `**/*.{php,phtml}` | `php.md`: PHP ソースと PHP テンプレート。 |
| `**/*.proto` | `protobuf.md`: Protocol Buffers のワイヤ互換性。 |
| `**/*.po` | `po.md`: gettext 翻訳ソースカタログ。 |
| `**/*.pot` | `pot.md`: gettext テンプレートファイル。 |
| `**/*.{graphql,gql}` | `graphql.md`: GraphQL スキーマと操作。 |
| `**/*.prisma` | `prisma.md`: Prisma スキーマ。 |
| `**/*.jl` | `julia.md`: Julia ソースコード。 |
| `**/*.{tf,hcl,tfvars}` | `terraform.md`: Terraform / HCL。 |
| `**/*.bicep` | `bicep.md`: Bicep（Azure）テンプレート。 |
| `**/*.elm` | `elm.md` - Elm ソースコード。 |
| `**/*.{jsonnet,libsonnet}` | `jsonnet.md`: Jsonnet の設定テンプレートとライブラリ。 |
| `**/*.thrift` | `thrift.md`: Apache Thrift IDL のワイヤ互換性。 |
| `**/*.capnp` | `capnp.md`: Cap'n Proto スキーマのワイヤ互換性。 |
| `**/*.{glsl,hlsl,wgsl,metal}` | `shader.md` - GLSL、HLSL、WGSL、Metal シェーダー。 |
| `**/*.m` | `matlab.md`（または[コンテンツスニッフィング](#content-sniffing-for-m-files)により `objc.md`） |
| `**/*.sol` | `solidity.md`: Solidity スマートコントラクト。 |
| `**/*.vy` | `vyper.md`: Vyper スマートコントラクト。 |
| *(fallback)* | `default.md` |

解決されたルール本文は、plan および main task prompt 内の `{{system_rule}}` プレースホルダーの内容になります。

### `.m` ファイルのコンテンツスニッフィング {#content-sniffing-for-m-files}

`.m` は MATLAB と Objective-C で共有されています。OCR はファイルの先頭の空でない
行を覗き見して区別します: Objective-C らしい内容（例: `#import`、
`@implementation`、C スタイルコメント）であれば `matlab.md` の代わりに `objc.md`
を使用します。コンテンツを読み取れない場合は `matlab.md` にフォールバックします。

> **安定性に関する注意。** スニッフィングのヒューリスティックは OCR のバージョン間
> で変更される可能性があります。確定的な `.m` ルーティングが必要な場合は、`.m`
> パスに明示的なプロジェクトレベルのルールを設定してください——プロジェクトルールは
> 常にシステム層より優先されます。

## どのルールが有効かを確認する: `ocr rules check`

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

あるルールが期待どおりに有効にならないときに使用してください。有効な**層**と**パターン**を表示します。

## レシピ

### プロジェクトレベル: コーディング規約を強制する

`<repo>/.opencodereview/rule.json` として保存し、commit します:

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

### プロジェクトレベル: 生成コードをスキップし、src に集中する

```json
{
  "include": ["src/**/*.{ts,tsx,js,jsx}"],
  "exclude": ["**/*.gen.ts", "**/generated/**"]
}
```

`include` を設定すると、`src/` 内のファイルは、本来は組み込みのデフォルト除外パターン（テストファイルなど）で除外されるものであっても保持されます。`src/` 以外のファイルは依然として通常の ext / default チェックを通ります。`include` はバイパスの仕組みであり、ホワイトリストではありません。

### PR ごとの上書き

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

プロジェクト層とグローバル層の両方を同時にバイパスします。単一の PR が完全に異なるレビューチェックリスト（例: セキュリティレビューのみ）を必要とするときに便利です。

### グローバルな個人設定

`~/.opencodereview/rule.json` に置くと、自分のマシン上のすべてのリポジトリが継承します:

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

## 関連項目

- [CLI リファレンス](../cli-reference/): `ocr review --rule`、`--preview`、`ocr rules check`。
- [設定](../configuration/): config ファイルの場所と階層的な解決チェーン。
- [アーキテクチャ](../architecture/): 解決されたルールがどのように agent prompt に供給されるか。
