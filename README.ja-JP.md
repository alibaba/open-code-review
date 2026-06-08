<p align="center">
  <a href="https://alibaba.github.io/open-code-review/">
    <img src="imgs/logo.svg" alt="OpenCodeReview logo" width="240" height="240">
  </a>
</p>
<p align="center">オープンソースのAIコードレビューエージェント。</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://goreportcard.com/report/github.com/alibaba/open-code-review"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
</p>
<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | 日本語 | <a href="README.ko-KR.md">한국어</a>
</p>

---

## Open Code Reviewとは？

Open Code ReviewはAIを活用したコードレビューCLIツールです。もともとはAlibaba Group社内の公式AIコードレビューアシスタントとして誕生し、過去2年間で数万人の開発者にサービスを提供し、数百万件のコード欠陥を発見してきました。大規模な環境で徹底的に検証された後、コミュニティ向けのオープンソースプロジェクトとして公開されました。モデルのエンドポイントを設定するだけで使い始められます。

Gitのdiffを読み取り、変更されたファイルをツール利用機能を持つエージェント経由で設定可能なLLMに送信し、行レベルの精度で構造化されたレビューコメントを生成します。エージェントはファイル全体の内容を読み取り、コードベースを検索し、コンテキストのために他の変更ファイルを参照し、深いレビューを生成できます — 単なる表面的なdiffへのフィードバックではありません。

![Highlights](imgs/highlights-en.png)

## なぜOpen Code Reviewなのか？

### 汎用エージェントの問題点

Claude CodeのSkillsのような汎用エージェントをコードレビューに使ったことがあれば、次のような課題に直面したことがあるはずです：

- **不完全なカバレッジ** — 大きな変更セットでは、エージェントが「手を抜き」、一部のファイルだけを選択的にレビューして他を見落としがちです。
- **位置のずれ** — 報告された問題が実際のコード位置と一致せず、行番号やファイル参照がターゲットからずれることが頻繁にあります。
- **不安定な品質** — 自然言語駆動のSkillsはデバッグが難しく、わずかなプロンプトの違いでレビュー品質が大きく変動します。

根本原因は、純粋に言語駆動のアーキテクチャにはレビュープロセスに対するハードな制約が欠けていることです。

### コア設計: 決定論的エンジニアリング × エージェントのハイブリッド

Open Code Reviewのコア哲学は、決定論的エンジニアリングとエージェントを組み合わせ、それぞれが得意とする領域を担当させることです。

**決定論的エンジニアリング — ハードな制約**

*絶対に間違えてはならない*レビューステップについては、言語モデルではなくエンジニアリングロジックが正しさを保証します：

- **正確なファイル選択** — どのファイルをレビューし、どのファイルをフィルタリングすべきかを正確に決定し、重要な変更が見落とされないようにします。
- **スマートなファイルバンドル** — 関連するファイルを単一のレビューユニットにグループ化します（例：`message_en.properties`と`message_zh.properties`はまとめてバンドルされます）。各バンドルは分離されたコンテキストを持つサブエージェントとして実行されます — 非常に大きな変更セットでも安定する分割統治戦略で、自然に並行レビューもサポートします。
- **きめ細かなルールマッチング** — 各ファイルの特性に応じてレビュールールをマッチングし、モデルの注意を鋭く集中させ、情報ノイズを発生源から排除します。純粋に言語駆動のルール誘導と比べて、テンプレートエンジンベースのルールマッチングはより安定的で予測可能です。
- **外部の位置特定・リフレクションモジュール** — 独立したコメント位置特定モジュールとコメントリフレクションモジュールにより、AIフィードバックの位置精度と内容精度の両方を体系的に向上させます。

**エージェント — 動的な意思決定**

エージェントの強みは、最も重要な領域 — 動的な意思決定と動的なコンテキスト取得 — に集中させています：

- **シナリオに最適化されたプロンプト** — コードレビュー向けに深く最適化されたプロンプトテンプレートにより、効果を高めつつトークン消費を削減します。
- **シナリオに最適化されたツールセット** — 大規模な本番データにおけるツール呼び出しトレースの詳細な分析 — 呼び出し頻度の分布、ツールごとの繰り返し率、新しいツールが呼び出しチェーン全体に与える影響など — から抽出された、汎用エージェントツールキットよりもコードレビューにおいて安定的で予測可能な専用ツールセットです。

## 使い方

### CLI

#### インストール

**NPM経由（推奨）**

```bash
npm install -g @alibaba-group/open-code-review
```

インストール後、`ocr`コマンドがグローバルに利用可能になります。

**GitHub Releaseから**

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)から最新のバイナリをダウンロードします：

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (x86_64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (ARM64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (x86_64) — ocr.exe を PATH の通ったディレクトリに移動してください
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64) — ocr.exe を PATH の通ったディレクトリに移動してください
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

**ソースから**

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### クイックスタート

**1. LLMの設定**

**コードレビューの前に必ずLLMを設定する必要があります。**

```bash
# オプションA: 対話的な設定
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token your-api-key-here
ocr config set llm.auth_header x-api-key
ocr config set llm.model claude-opus-4-6
ocr config set llm.use_anthropic true

# オプションB: 環境変数（最優先）
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_AUTH_HEADER=x-api-key
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

設定は`~/.opencodereview/config.json`に保存されます。

Anthropicでは、標準の`sk-ant-*` APIキーは`x-api-key`を使用し、OAuthトークンは`authorization`を使用します。`llm.auth_header` / `OCR_LLM_AUTH_HEADER`を省略した場合、OCRは既存の`authorization` bearer-token動作を維持します。

また、Claude Codeの環境変数（`ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`）とも互換性があり、`~/.zshrc` / `~/.bashrc`からこれらのexportをパースします。

> **CC-Switchユーザー向けの注意**: [CC-Switch](https://github.com/farion1231/cc-switch)を[ルーティングサービス](https://www.ccswitch.io/en/docs?section=proxy&item=service)有効で使用している場合、追加設定なしで`llm.url`をCC-Switchのプロキシアドレスに向けることができます：
> - **Claude**プロバイダーの場合: `llm.url`を`http://127.0.0.1:15721`に設定
> - **CodeX**プロバイダーの場合: `llm.url`を`http://127.0.0.1:15721/v1`に設定
> - `llm.model`はプロバイダー設定に応じて設定
> - `llm.auth_token`は任意の値で構いません
> - `extra_body`設定は引き続き有効です

**2. 疎通テスト**

```bash
ocr llm test
```

**3. レビュー**

```bash
cd your-project

# ワークスペースモード — ステージ済み・未ステージ・未追跡のすべての変更をレビュー
ocr review

# ブランチ範囲 — 2つのrefを比較
ocr review --from main --to feature-branch

# 単一コミット
ocr review --commit abc123
```

### コーディングエージェントとの統合

OCRはスラッシュコマンドとしてAIコーディングエージェントにシームレスに統合でき、エージェントのワークフロー内で直接コードレビューが可能になります。

#### オプション1: Skillとしてインストール

`npx`を使ってOCRスキルをプロジェクトにインストールします：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

これにより、[skillsレジストリ](skills/open-code-review/SKILL.md)から`open-code-review`スキルがインストールされ、コーディングエージェントにコードレビューのための`ocr`の呼び出し方、優先度による問題の分類、必要に応じた修正の適用を教えます。

#### オプション2: Claude Codeプラグインとしてインストール

[Claude Code](https://docs.anthropic.com/en/docs/claude-code)の場合、Claude Code内で以下のコマンドを実行してコマンドプラグインをインストールします：

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

これにより`/open-code-review:review`スラッシュコマンドが登録され、OCRを実行して問題を自動的にフィルタリング・修正します。

#### オプション3: Codexプラグインとしてインストール

ローカルCodexでは、このリポジトリからOpen Code Reviewプラグインをインストールできます：

```bash
codex plugin marketplace add alibaba/open-code-review
codex
/plugins
```

ローカルcheckoutまたはforkでは、次を使用できます：

```bash
codex plugin marketplace add .
codex
/plugins
```

`Open Code Review`をインストールして有効化した後、新しいCodex threadを開始して明示的に呼び出します：

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

これにより、ローカルOCR CLIを実行するCodex skillが登録されます：

```bash
ocr review --audience agent
```

この統合はOCRの内部LLM backendを変更せず、Codex用のOpenAI Responses API endpoint設定も必要ありません。OCR自体には、CLI setupセクションで説明されている`ocr` CLIのインストールと設定が引き続き必要です。

韓国語ガイド：[`plugins/open-code-review/CODEX.ko-KR.md`](plugins/open-code-review/CODEX.ko-KR.md)

#### オプション4: コマンドファイルを直接コピー

パッケージマネージャーを使わずに素早くセットアップしたい場合は、コマンドファイルをコピーするだけでClaude Codeで`/open-code-review`スラッシュコマンドを使えるようになります。

**プロジェクトレベル**（gitでチームと共有）：

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

**ユーザーレベル**（全プロジェクトで個人用にグローバル利用）：

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

> **前提条件**: すべての統合方法において、`ocr` CLIのインストールとLLMの設定が必要です。上記の[インストール](#インストール)と[LLMの設定](#1-llmの設定)を参照してください。

### CI/CD統合

OCRをCI/CDパイプラインに統合して、Merge Request / Pull Requestのコードレビューを自動化できます。

CI統合のコアコマンド：

```bash
ocr review \
  --from "origin/main" \
  --to "origin/feature-branch" \
  --format json
```

`--format json`フラグは、CIスクリプトでのパースに適した機械可読な結果を出力します。

統合例は[`examples/`](./examples/)ディレクトリを参照してください：

- [`github_actions/`](./examples/github_actions/) — GitHub Actions統合の例
- [`gitlab_ci/`](./examples/gitlab_ci/) — GitLab CI統合の例

## コマンド

| コマンド | エイリアス | 説明 |
|---------|-------|-------------|
| `ocr review` | `ocr r` | コードレビューを開始 |
| `ocr rules check <file>` | — | ファイルパスに適用されるレビュールールをプレビュー |
| `ocr config set <key> <value>` | — | 設定値をセット |
| `ocr llm test` | — | LLMの疎通テスト |
| `ocr viewer` | `ocr v` | `localhost:5483`でWebUIセッションビューアーを起動 |
| `ocr version` | — | バージョン情報を表示 |

### `ocr review`のフラグ

| フラグ | 短縮形 | デフォルト | 説明 |
|------|-----------|---------|-------------|
| `--repo` | — | カレントディレクトリ | Gitリポジトリのルート |
| `--from` | — | — | ソースref（例：`main`） |
| `--to` | — | — | ターゲットref（例：`feature-branch`） |
| `--commit` | `-c` | — | レビュー対象の単一コミット |
| `--preview` | `-p` | `false` | LLMを実行せずにレビュー対象ファイルをプレビュー |
| `--format` | `-f` | `text` | 出力形式：`text`または`json` |
| `--concurrency` | — | `8` | ファイルレビューの最大同時実行数 |
| `--timeout` | — | `10` | 同時実行タスクのタイムアウト（分） |
| `--audience` | — | `human` | `human`（進捗を表示）または`agent`（サマリーのみ） |
| `--rule` | — | — | カスタムJSONレビュールールへのパス |
| `--max-tools` | — | 組み込み値 | ファイルごとのツール呼び出しラウンドの上限。テンプレートのデフォルトより大きい場合のみ有効 |
| `--max-git-procs` | — | 組み込み値 | gitサブプロセスの最大同時実行数 |
| `--tools` | — | — | カスタムJSONツール設定へのパス |

## 例

```bash
# レビュー対象ファイルをプレビュー（LLM呼び出しなし）
ocr review --preview
ocr review -c abc123 -p

# デフォルト設定でワークスペースの変更をレビュー
ocr review

# 高めの同時実行数でブランチのdiffをレビュー
ocr review --from main --to my-feature --concurrency 4

# 特定のコミットを詳細なJSON出力でレビュー
ocr review --commit abc123 --format json --audience agent

# カスタムレビュールールを使用
ocr review --rule /path/to/my-rules.json

# ファイルに適用されるルールをプレビュー
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# ブラウザでレビューセッション履歴を表示
ocr viewer
ocr viewer --addr :3000
```

### ビューアーのセキュリティ

ビューアーはセッションのJSONLコンテンツ（LLMリクエストメッセージとレスポンス）をHTTPで配信します。すべてのリクエストに対してHostヘッダーの許可リストを強制します：ループバック名（`localhost`、`127.0.0.0/8`、`::1`）と実際のバインドホストは常に許可されます。ワイルドカードバインド（`--addr :3000`、`--addr 0.0.0.0:3000`）やその他の非ループバックのホスト名は、環境変数`OCR_VIEWER_ALLOWED_HOSTS`（カンマ区切り）で追加する必要があります：

```bash
OCR_VIEWER_ALLOWED_HOSTS=review.internal,ocr.lan ocr viewer --addr :3000
```

これにより、ローカルビューアーに対するDNSリバインディング攻撃をブロックします。

## レビュールール

OCRは4層の優先度チェーンを使ってレビュールールを解決します。各層はファーストマッチ優先です：ファイルパスがパターンにマッチすればそのルールが使われ、マッチしなければ次の層にフォールスルーします。

| 優先度 | ソース | パス | 説明 |
|----------|--------|------|-------------|
| 1（最高） | `--rule`フラグ | ユーザー指定パス | CLIによる明示的なオーバーライド |
| 2 | プロジェクト設定 | `<repoDir>/.opencodereview/rule.json` | プロジェクトごとのルール。gitにコミット可能 |
| 3 | グローバル設定 | `~/.opencodereview/rule.json` | ユーザー全体の個人設定 |
| 4（最低） | システムデフォルト | 組み込みの`system_rules.json` | 一般的な言語とファイルタイプをカバーする組み込みルール |

### ルールファイルの形式

第1〜3層は同じJSON形式を共有します：

```json
{
  "rules": [
    {
      "path": "force-api/**/*.java",
      "rule": "All new methods must validate required parameters for null values"
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags"
    }
  ]
}
```

- `path`は`**`による再帰マッチと`{java,kt}`のブレース展開をサポートします。
- 各層の中では、ルールは宣言順に評価されます — 最初にマッチしたものが採用されます。
- ルールファイルが存在しない場合は、何も出力せずスキップされます。

### パスフィルタリング

ルールファイルでは `include` と `exclude` フィールドも使用でき、どのファイルをレビュー対象にするかを制御できます：

```json
{
  "rules": [
    {"path": "**/*.java", "rule": "null安全性をチェック"}
  ],
  "include": ["src/main/**/*.java", "lib/**/*.kt"],
  "exclude": ["**/generated/**", "vendor/**"]
}
```

**フィルタ判定の優先度（高い順）：**

| ステップ | 条件 | 結果 |
|------|-----------|--------|
| 1 | ファイルがバイナリ | 除外 |
| 2 | パスがユーザーの`exclude`パターンにマッチ | 除外 |
| 3 | ファイル拡張子がサポートリストにない | 除外 |
| 4 | `include`が設定されており、パスがマッチ | **レビュー対象**（ステップ5をスキップ） |
| 5 | パスが組み込みデフォルト除外パターン（テストファイル等）にマッチ | 除外 |
| 6 | 上記のいずれにも該当しない | レビュー対象 |

**動作ロジック：**

- `include`と`exclude`はレビュールールと同じ優先度チェーン（`--rule` > プロジェクト設定 > グローバル設定）に従います。**include/excludeが設定されている最も高い優先度の層**が一括で適用され、層を跨いだマージは行われません。
- `exclude`は常に`include`より優先されます — 両方にマッチするファイルは除外されます。
- `include`は**組み込みデフォルト除外パターンをバイパスする**ためのものであり（例：テストファイル）、排他的な許可リストではありません — `include`パターンにマッチしないファイルも通常通りデフォルトフィルタチェックに進みます。
- パターン構文：`**`再帰マッチ、`*`単一セグメントマッチ、`{a,b}`ブレース展開をサポート。マッチングは大文字小文字を区別しません。

**組み込みデフォルト除外パターン**（テストファイル等をフィルタ — `include`でオーバーライド可能）：

```
**/*_test.go, **/*Test.java, **/*Tests.java, **/*_test.rs,
**/*.test.{js,jsx,ts,tsx}, **/*.spec.{js,jsx,ts,tsx}, **/__tests__/**,
**/src/test/java/**/*.java, **/src/test/**/*.kt,
**/test/**/*_test.py, **/tests/**/*_test.py, **/*_test.py,
**/*_spec.rb, **/spec/**/*_spec.rb, **/oh_modules/**
```

## 設定リファレンス

設定ファイル：`~/.opencodereview/config.json`

| キー | 型 | 例 |
|-----|------|---------|
| `llm.url` | string | `https://api.openai.com/v1/chat/completions` |
| `llm.auth_token` | string | `sk-xxxxxxx` |
| `llm.auth_header` | string | Anthropicのみ：`x-api-key` \| `authorization` |
| `llm.model` | string | `claude-opus-4-6` |
| `llm.use_anthropic` | boolean | `true` \| `false` |
| `language` | string | `English` \| `Chinese`（デフォルト：Chinese） |
| `telemetry.enabled` | boolean | `true` \| `false` |
| `telemetry.exporter` | string | `console` \| `otlp` |
| `telemetry.otlp_endpoint` | string | OTLPコレクターのアドレス |
| `telemetry.content_logging` | boolean | テレメトリーにプロンプトを含める |

環境変数は設定ファイルより優先されます。

### 環境変数

| 変数 | 用途 |
|----------|---------|
| `OCR_LLM_URL` | LLM APIエンドポイントURL |
| `OCR_LLM_TOKEN` | APIキー / 認証トークン |
| `OCR_LLM_AUTH_HEADER` | Anthropic認証ヘッダー（`x-api-key`または`authorization`） |
| `OCR_LLM_MODEL` | モデル名 |
| `OCR_USE_ANTHROPIC` | `true` = Anthropic、`false` = OpenAI |


## テレメトリー

可観測性（スパン、メトリクス）のためのOpenTelemetry統合。デフォルトでは無効です。

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

エクスポートデータにLLMのプロンプトとレスポンスを含めるには、`telemetry.content_logging`を設定してください。

## コントリビューション

開発環境のセットアップ、コーディングガイドライン、プルリクエストの提出方法については[CONTRIBUTING.md](CONTRIBUTING.md)を参照してください。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=alibaba/open-code-review&type=Date)](https://star-history.com/#alibaba/open-code-review&Date)

## ライセンス

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
