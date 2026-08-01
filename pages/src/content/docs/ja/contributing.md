---
title: コントリビュート
sidebar:
  order: 13
---

OCR は Apache-2.0 ライセンスのオープンソースプロジェクトです。バグ報告、ドキュメント修正、
コード貢献を歓迎します。本ページはクイックリファレンスです。正式版は
[`CONTRIBUTING.md`](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md) にあります。

## 貢献の方法

Go を書かなくても貢献できます。

- **バグ報告**——再現手順を添えて
  [GitHub issue](https://github.com/alibaba/open-code-review/issues/new/choose) を作成してください。
- **機能リクエスト**——
  [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)
  に投稿するか、feature-request issue を作成してください。
- **ドキュメント**——誤字、不足している例、リンク切れ——これらの PR は通常、最も早くマージされます。
- **他の PR のレビュー**——メンテナー以外からのコメントは、レビュアーの負担軽減に役立ちます。
- **コード**——バグ修正、パフォーマンス改善、新機能。

## ローカル開発環境のセットアップ

### 前提条件

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### ソースコードの取得

```bash
# GitHub で Fork し、次に:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # dist/opencodereview を書き出す
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> `upstream` remote は読み取り専用です。`origin`（あなたの fork）にプッシュし、そこから PR を出してください。

### ローカルビルドの実行

```bash
./dist/opencodereview review --preview
```

便宜のため、`dist/opencodereview` を指すシンボリックリンクを `~/bin/ocr-dev` に置いておくと、
任意のリポジトリで `ocr-dev` を呼び出せます。

### Make target

| Target | 作用 |
|---|---|
| `make build` | 現在のプラットフォーム向けにビルド → `dist/opencodereview`。 |
| `make build-darwin-amd64` | macOS Intel 向けのクロスコンパイル。 |
| `make build-darwin-arm64` | macOS Apple Silicon 向けのクロスコンパイル。 |
| `make build-linux-amd64` | Linux x86_64 向けのクロスコンパイル。 |
| `make build-linux-arm64` | Linux ARM64 向けのクロスコンパイル。 |
| `make build-windows-amd64` | Windows x86_64 向けのクロスコンパイル。 |
| `make build-windows-arm64` | Windows ARM64 向けのクロスコンパイル。 |
| `make build-all` | 6 つすべてのクロスコンパイルバイナリ（linux/darwin/windows × amd64/arm64）。 |
| `make sha256sum` | ビルド成果物の `sha256sum.txt` を生成。 |
| `make dist` | `clean → build-all → sha256sum`。CI が実行する内容。 |
| `make test` | race 検出を有効にしてテストを実行。 |
| `make clean` | `dist/` を削除。 |

## ブランチとコミットの規約

### ブランチのプレフィックス

| プレフィックス | 用途 |
|---|---|
| `feat/` | 新機能 |
| `fix/` | バグ修正 |
| `docs/` | ドキュメントのみ |
| `refactor/` | 挙動を変えないリファクタリング |
| `test/` | テストのみの変更 |
| `chore/` | ビルド / CI / ツール |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### コミットメッセージ

[Conventional Commits](https://www.conventionalcommits.org/) フォーマット:

```
<type>(<scope>): <short summary>

[optional body explaining the why]
```

例:

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(readme): clarify endpoint resolution priority
refactor(viewer): extract task-card rendering into helper
```

**PR タイトル**も同じフォーマットを使うと、生成される changelog に整然と表示されます。

## プロジェクト構成

```
open-code-review/
├── cmd/opencodereview/        # CLI エントリーポイント——引数解析、ディスパッチ
├── internal/
│   ├── agent/                 # レビューエージェントのロジック、サブエージェントのディスパッチ
│   ├── config/                # テンプレート、ルール、ホワイトリスト、埋め込み JSON
│   ├── diff/                  # Git diff の解析、3 つのモード
│   ├── gitcmd/                # Git サブプロセスランナー
│   ├── llm/                   # LLM client（Anthropic と OpenAI）、エンドポイント解決
│   ├── model/                 # データ構造（LlmComment、Diff……）
│   ├── pathutil/              # パスユーティリティ
│   ├── release/               # Release notes の生成
│   ├── session/               # JSONL セッションライター
│   ├── stdout/                # ミュート可能な stdout writer
│   ├── suggestdiff/           # 提案 diff のレンダリング
│   ├── telemetry/             # OpenTelemetry の設定 + ヘルパー
│   ├── tool/                  # ツールレジストリ + provider の実装
│   └── viewer/                # 埋め込み HTTP UI
├── pages/                     # WebUI マーケティングページ（独立した React app）
├── plugins/                   # Claude Code slash コマンド
├── extensions/                # エディタ拡張（VS Code）
├── examples/                  # CI レシピ（GitHub Actions、GitLab CI）
├── skills/                    # Agent SDK skill manifest
├── scripts/                   # NPM postinstall + クロスプラットフォームビルドスクリプト
├── npm/                       # 各プラットフォーム向け optional dependency パッケージ
└── bin/                       # NPM wrapper（Node）
```

ほとんどの貢献は `internal/agent/`、`internal/tool/`、`internal/llm/` に触れます。
`cmd/opencodereview/` の CLI レイヤーは意図的に薄く保たれています——引数を解析してから
agent パッケージへディスパッチします。

## コード品質チェック

PR を出す前に:

```bash
go fmt ./...
go vet ./...
make test       # race 有効、CI では push のたびに実行される
make build      # バイナリがビルドできることのスモークテスト
```

CI は push のたびに同じ一式を実行するため、予期せぬ結果になることはありません。

## 新しいツールの追加

ツールは 2 つの部分から成ります。

1. [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)
   内の **JSON 定義**: name、description、そして LLM が見る JSON-schema 引数。
2. `internal/tool/definitions.go` に登録される **Go provider**（実際の実装を含む）。

両方が揃って初めて、新しいツール名が機能します。既存の 6 つは[ツール](../tools/)にあり、
テンプレートとして使えます。

## 新しいルールパターンの追加

`internal/config/rules/system_rules.json` を編集して新しい glob をルールドキュメントにマッピングし、
`internal/config/rules/rule_docs/` 下に対応する markdown を追加します。ルールドキュメントは
パターンごとに 1 ファイルで保存されます（英語）。`language` 設定は system prompt に
「その言語で応答せよ」という指示を 1 行追加するだけです。rule-doc ファイルは切り替えません。

## PR のフロー

1. **大きな変更はまず issue を作成してください。** 事前に方向性を合わせておく方が、
   コードレビューで初めて相違に気づくよりも良いです。
2. **1 つの PR につき 1 つの論理的変更。** 無関係な修正が 2 つあるなら、PR を 2 つ出してください。
3. **テストを更新してください。** 挙動の変更にはテストのカバレッジが必要です——`make test` は必ず通ること。
4. **ドキュメントを更新してください。** 変更が引数、config key、レビューパイプラインに影響する場合は、
   本ドキュメントサイト（[`docs/`](https://github.com/alibaba/open-code-review) 内）と関連するインラインヘルプの
   両方を更新してください。
5. **PR テンプレートを記入してください。** メンテナーがレビューします。通常は数営業日以内です。

## コントリビューターライセンス契約（CLA）

本プロジェクトは Alibaba Open Source CLA を必要とします。初めて PR を出すと bot がリンクを貼ります——
電子署名してください（1 分程度）。以降の PR では再署名は不要です。

## 初めての貢献ですか？

[`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue)
または [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted)
のラベルが付いた issue を探してください。ほとんどは小規模で自己完結しており、issue の説明に
着手に十分なコンテキストがあります。

## 関連項目

- [アーキテクチャ](../architecture/)——`internal/agent/` を変更する前に必要なメンタルモデル。
- [ツール](../tools/)——既存のツールがどのようなものか。
- 完全な貢献ガイド:
  [CONTRIBUTING.md](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)
