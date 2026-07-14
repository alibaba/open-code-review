<div align="center">
  <a href="https://alibaba.github.io/open-code-review/">
    <img src="imgs/logo-core.svg" alt="OpenCodeReview logo" width="180" />
  </a>
  <h1>OpenCodeReview</h1>
</div>

<p align="center">
  <a href="https://trendshift.io/repositories/41087" target="_blank">
    <img src="https://trendshift.io/api/badge/trendshift/repositories/41087/weekly?language=Go" alt="alibaba%2Fopen-code-review | Trendshift" style="width: 320px; height: 70px;" width="320" height="70" />
  </a>
</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://deepwiki.com/alibaba/open-code-review"><img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg" /></a>
  <a href="https://www.bestpractices.dev/projects/13328"><img alt="OpenSSF Best Practices" src="https://img.shields.io/badge/OpenSSF-Silver-4C566A?style=flat-square" /></a>
</p>
<p align="center">
  <a href="#supported-platforms"><img alt="Windows" src="https://img.shields.io/badge/Windows-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="macOS" src="https://img.shields.io/badge/macOS-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="Linux" src="https://img.shields.io/badge/Linux-supported-blue.svg" /></a>
  <a href="#supported-agents"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Codex" src="https://img.shields.io/badge/Codex-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Cursor" src="https://img.shields.io/badge/Cursor-supported-blueviolet.svg" /></a>
</p>
<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | 日本語 | <a href="README.ko-KR.md">한국어</a> | <a href="README.ru-RU.md">Русский</a>
</p>

---

## Open Code Reviewとは？

Open Code ReviewはAIを活用したコードレビューCLIツールです。もともとはAlibaba Group社内の公式AIコードレビューアシスタントとして誕生し、過去2年間で数万人の開発者にサービスを提供し、数百万件のコード欠陥を発見してきました。大規模な環境で徹底的に検証された後、コミュニティ向けのオープンソースプロジェクトとして公開されました。モデルのエンドポイントを設定するだけで使い始められます。

Gitのdiffを読み取り、変更されたファイルをツール利用機能を持つエージェント経由で設定可能なLLMに送信し、行レベルの精度で構造化されたレビューコメントを生成します。エージェントはファイル全体の内容を読み取り、コードベースを検索し、コンテキストのために他の変更ファイルを参照し、深いレビューを生成できます — 単なる表面的なdiffへのフィードバックではありません。diffレビュー以外にも、`ocr scan` はファイル全体をレビューできます。不慣れなコードベースの監査や、意味のあるdiffがないディレクトリの検査に便利です。

詳細は[公式サイト](https://alibaba.github.io/open-code-review/)をご覧ください。

![Highlights](imgs/highlights-ja.png)

## ベンチマーク

> 汎用エージェント（Claude Code）と比較して、Open Code Reviewは同じ基盤モデルで有意に高い**精度（Precision）**と**F1スコア**を達成し、トークン消費量は**約1/9**にとどまり、レビューもより高速です。ただし、リコール（Recall）は汎用エージェントより低くなります——これはノイズを抑え精度を優先する設計上のトレードオフです。

実際のコードレビューに基づくベンチマーク。**50**の人気オープンソースリポジトリから**200**の実際のPull Requestを厳選し、**10**のプログラミング言語をカバー——80人以上のシニアエンジニアによるクロスバリデーション（**1,505**件のアノテーション済み欠陥）。

| 指標 | 測定内容 | 重要性 |
|------|----------|--------|
| **F1** | 精度とリコールの調和平均 | レビュー品質を示す最良の単一指標 |
| **精度 (Precision)** | 報告された問題のうち実際の欠陥の割合 | 高い = 確認すべき偽陽性が少ない |
| **リコール (Recall)** | 実際の欠陥のうち発見された割合 | 高い = 見逃しが少ない |
| **平均時間 (Avg Time)** | レビューあたりの実時間 | CIパイプラインの待機時間に影響 |
| **平均トークン (Avg Token)** | レビューあたりの総トークン消費量 | APIコストに直接影響 |

![Benchmark](imgs/benchmark-ja.png)

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

### 前提条件

- **Git >= 2.41** — Open Code Review は diff 生成、コード検索、リポジトリ操作に Git を利用します。

### CLI

#### インストール

**NPM経由（推奨）**

```bash
npm install -g @alibaba-group/open-code-review
```

インストール後、`ocr`コマンドがグローバルに利用可能になります。

**更新**

NPM でインストールした場合は、手動で最新バージョンへ更新できます：

```bash
npm install -g @alibaba-group/open-code-review@latest
```

NPM インストール版の `ocr` は、既定でバックグラウンドで新しいバージョンを確認し、自動的に更新します。自動更新を無効にするには、`OCR_NO_UPDATE=1` を設定してください。

インストールスクリプトまたは手動ダウンロードしたバイナリでインストールした場合は、同じインストール/ダウンロードコマンドを再実行すると、ローカルのバイナリを最新リリースに置き換えられます。特定のリリースタグに固定する必要がある場合は `OCR_VERSION` を使います。

**GitHub Releaseから**

1 つのコマンドで、お使いの OS / アーキテクチャ向けの最新バイナリをインストールできます（macOS / Linux）：

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

このスクリプトは適切なリリースバイナリを選択し、SHA-256 チェックサムを検証して、`ocr` として `/usr/local/bin` にインストールします。インストール先は `OCR_INSTALL_DIR` で、リリースバージョンは `OCR_VERSION` で上書きできます：

```bash
OCR_INSTALL_DIR="$HOME/.local/bin" OCR_VERSION=v1.3.13 \
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh)"
```

<details>
<summary>手動ダウンロード（Windows を含む全プラットフォーム）</summary>

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)からお使いのプラットフォーム向けのバイナリをダウンロードします：

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

</details>

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

OCRは統一された**プロバイダー（Provider）**システムでLLM設定を管理します。多数の主要プロバイダーが組み込まれており、プライベートデプロイメントやその他の互換エンドポイントに接続するためのカスタムプロバイダーの追加もサポートしています。設定は`~/.opencodereview/config.json`に保存されます。

**オプションA: 対話的セットアップ（推奨）**

```bash
ocr config provider          # ビルトインプロバイダーを選択またはカスタムプロバイダーを追加
ocr config model             # アクティブなプロバイダーのモデルを選択
```

![Provider setup](imgs/providers.jpg)

対話的UIがプロバイダーの選択、APIキーの入力、モデル設定をガイドし、完了後に自動的に接続テストを行います。

`ocr llm providers`を実行すると、すべてのビルトインプロバイダーを確認できます。ビルトインプロバイダーにはAPI URLとプロトコルがプリセットされているため、APIキーを提供するだけで使用できます。対応する環境変数（例：`ANTHROPIC_API_KEY`、`OPENAI_API_KEY`）が設定済みの場合、APIキーは自動的に読み取られます。

**カスタムプロバイダー**も対話的UIから追加できます — プロバイダー名、API URL、プロトコルタイプ（`anthropic`または`openai`）、APIキーを入力します。

**オプションB: CLIセットアップ（CI/CDなど非対話環境向け）**

`ocr config set`コマンドでプロバイダー設定を直接書き込みます。スクリプトや自動化に適しています。

ビルトインプロバイダーを使用する場合：

```bash
ocr config set provider anthropic
ocr config set providers.anthropic.api_key your-api-key-here
ocr config set providers.anthropic.model claude-sonnet-4-6
```

カスタムプロバイダーを使用する場合（プライベートゲートウェイやその他の互換エンドポイント）：

```bash
ocr config set provider my-gateway
ocr config set custom_providers.my-gateway.url https://my-llm-gateway.internal/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.api_key your-api-key-here
ocr config set custom_providers.my-gateway.model gpt-4o
```

> カスタムプロバイダーでは`url`と`protocol`が必須です。サポートされるプロトコル：`anthropic`、`openai`、`openai-responses`。

オプション設定：

| キー | 説明 |
|------|------|
| `providers.<name>.auth_header` | 認証ヘッダー：`x-api-key`または`authorization`（デフォルト：`authorization`） |
| `providers.<name>.extra_body` | リクエストボディにマージされるカスタムJSONフィールド |
| `providers.<name>.extra_headers` | カンマ区切りの `key=value` ペアで、各リクエストに追加されるカスタムHTTPヘッダー |
| `providers.<name>.models` | 対話的選択用のモデルリスト |

**`extra_headers`（オプション）：** すべてのLLM APIリクエストにカスタムHTTPヘッダーを追加します。プロキシ、ゲートウェイ、追加ヘッダーを必要とするエンタープライズエンドポイント（組織ID、トレースIDなど）に便利です。形式はカンマ区切りの `key=value` ペアです。カンマを含む値はダブルクォートで囲んでください：

```bash
ocr config set llm.extra_headers "X-Org-ID=org-123,X-Forwarded-For=\"1.2.3.4,5.6.7.8\""
```

プロバイダーごとに追加ヘッダーを設定することもできます：

```bash
ocr config set providers.anthropic.extra_headers "X-Org-ID=org-123"
```

**環境変数（最優先）**

環境変数は設定ファイルの設定を上書きします。設定ファイルの書き込みが不便なCI/CDシナリオに適しています：

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

OpenAI Responses API（GPT-5.x / o-シリーズモデル）を使うには、`OCR_USE_ANTHROPIC` の代わりに `OCR_LLM_PROTOCOL` を設定してください:

```bash
export OCR_LLM_URL=https://api.openai.com/v1
export OCR_LLM_TOKEN=your-openai-key
export OCR_LLM_MODEL=gpt-5.4
export OCR_LLM_PROTOCOL=openai-responses
```

`OCR_LLM_PROTOCOL` は `anthropic`、`openai`、`openai-responses`を受け付け、`OCR_USE_ANTHROPIC` と同時に設定した場合は優先されます。

Claude Codeの環境変数（`ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`）とも互換性があり、`~/.zshrc` / `~/.bashrc`からこれらのexportをパースします。

> **CC-Switchユーザー向けの注意**: [CC-Switch](https://github.com/farion1231/cc-switch)を[ルーティングサービス](https://www.ccswitch.io/en/docs?section=proxy&item=service)有効で使用している場合、プロバイダーの`url`をCC-Switchのプロキシアドレスに向けることで、追加設定なしで利用できます：
> - **Claude**プロバイダーの場合：`providers.anthropic.url`を`http://127.0.0.1:15721`に設定
> - **Codex**プロバイダーの場合：対応するプロバイダーの`url`を`http://127.0.0.1:15721/v1`に設定
> - `api_key`は任意の値で構いません。`extra_body`設定は引き続き有効です

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

# 中断した範囲または単一 commit レビューを再開
ocr session list
ocr review --from main --to feature-branch --resume <session-id>

# フルファイルスキャン — diffではなくファイル全体をレビュー（git履歴不要）
ocr scan                          # リポジトリ全体をスキャン
ocr scan --path internal/agent    # ディレクトリまたは特定のファイルをスキャン
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

#### オプション4: Cursorプラグインとしてインストール

[Cursor](https://www.cursor.com/)では、このリポジトリからOpen Code Reviewプラグインをインストールできます：

```
cursor-plugin marketplace add alibaba/open-code-review
```

手動でmarketplaceを追加することもできます。Cursorで`/plugins`を開き、`Open Code Review`を検索してインストールしてください。

ローカルcheckoutまたはforkの場合：

```
cursor-plugin marketplace add .
```

インストール後、Cursorで次のように呼び出します：

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

これにより、ローカルOCR CLIを実行するCursor skillが登録されます：

```bash
ocr review --audience agent
```

この統合はOCRの内部LLM backendを変更しません。OCR自体には、CLI setupセクションで説明されている`ocr` CLIのインストールと設定が引き続き必要です。

#### オプション5: コマンドファイルを直接コピー

パッケージマネージャーを使わずに素早くセットアップしたい場合は、コマンドファイルをコピーするだけでClaude Codeで`/open-code-review`スラッシュコマンドを使えるようになります。

**プロジェクトレベル**（gitでチームと共有）：

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

**ユーザーレベル**（全プロジェクトで個人用にグローバル利用）：

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
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

各指摘には2つの構造化フィールドが付与され、CI統合はコメント本文を再パースせずに並べ替え・グループ化・フィルタリング・ビルドのゲート判定を行えます：

| フィールド | 許可される値 | 説明 |
|-----------|-------------|------|
| `category` | `bug`、`security`、`performance`、`maintainability`、`test`、`style`、`documentation`、`other` | 指摘が属するカテゴリ。 |
| `severity` | `critical`、`high`、`medium`、`low` | 指摘の重要度。 |

JSON出力ではこの2つのフィールドは`content`や`start_line`などと同じ階層に並びます。ターミナルでは、コメントの前にインラインの`[category · severity]`バッジとして表示され、重要度に応じて色分けされます。

統合例は[`examples/`](./examples/)ディレクトリを参照してください：

- [`github_actions/`](./examples/github_actions/) — GitHub Actions統合の例
- [`gitlab_ci/`](./examples/gitlab_ci/) — GitLab CI統合の例
- [`gitflic_ci/`](./examples/gitflic_ci/) — GitFlic CI統合の例

#### GitHub Action

GitHub 向けに、本リポジトリはリポジトリルートにすぐ使える composite Action（[`action.yml`](./action.yml)）を同梱しています。自分で `ocr review` をスクリプト化する代わりに、これを直接参照するだけで、checkout、OCR のインストール、レビューの実行、インラインコメントとサマリーコメントの投稿、アーティファクトのアップロード、再試行・冪等性までの全パイプラインを処理できます：

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

再現性を高めるため、バージョンタグまたはコミット SHA に固定してください。完全なワークフローデモ、inputs/outputs の全一覧、コメント投稿モード（スティッキーサマリー、非破壊的なインクリメンタル投稿）については [`examples/github_actions/`](./examples/github_actions/) ディレクトリを参照してください。

## コマンド

| コマンド | エイリアス | 説明 |
|---------|-------|-------------|
| `ocr review` | `ocr r` | diffベースのコードレビューを開始 |
| `ocr scan` | `ocr s` | ファイル全体をレビュー（diff不要） |
| `ocr rules check <file>` | — | ファイルパスに適用されるレビュールールをプレビュー |
| `ocr config provider` | — | 対話的プロバイダーセットアップ（ビルトイン、カスタム、手動） |
| `ocr config model` | — | アクティブなプロバイダーの対話的モデル選択 |
| `ocr config set <key> <value>` | — | 設定値をセット |
| `ocr config unset custom_providers.<name>` | — | カスタムプロバイダーを削除 |
| `ocr llm test` | — | LLMの疎通テスト |
| `ocr llm providers` | — | ビルトインLLMプロバイダーを一覧表示 |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | 保存済みレビューセッションを一覧表示 |
| `ocr session show <id>` | `ocr sessions show <id>` | 1つのセッションとファイル単位のチェックポイントを表示 |
| `ocr viewer` | `ocr v` | `localhost:5483`でWebUIセッションビューアーを起動 |
| `ocr version` | — | バージョン情報を表示 |

### `ocr review`のフラグ

| フラグ | 短縮形 | デフォルト | 説明 |
|------|-----------|---------|-------------|
| `--repo` | — | カレントディレクトリ | Gitリポジトリのルート |
| `--from` | — | — | ソースref（例：`main`） |
| `--to` | — | — | ターゲットref（例：`feature-branch`） |
| `--commit` | `-c` | — | レビュー対象の単一コミット |
| `--exclude` | — | — | カンマ区切りのgitignoreスタイルパターンでスキップ対象を指定；rule.jsonのexcludesとマージ |
| `--preview` | `-p` | `false` | LLMを実行せずにレビュー対象ファイルをプレビュー |
| `--resume` | — | — | 以前の互換性のある範囲または単一 commit レビューセッションから再開 |
| `--format` | `-f` | `text` | 出力形式：`text`または`json` |
| `--concurrency` | — | `8` | ファイルレビューの最大同時実行数 |
| `--timeout` | — | `10` | 同時実行タスクのタイムアウト（分） |
| `--audience` | — | `human` | `human`（進捗を表示）または`agent`（サマリーのみ） |
| `--background` | `-b` | — | レビューのための任意の要件/ビジネスコンテキスト。`--commit`使用時に未指定の場合、コミットメッセージから自動取得 |
| `--background-file` | `-B` | — | Markdownファイルから読み込む任意の要件/ビジネスコンテキスト。`--background`と併用した場合はインラインの値が先に配置されます |
| `--model` | — | — | このレビューでLLMモデルを選択または上書き |
| `--rule` | — | — | カスタムJSONレビュールールへのパス |
| `--max-tools` | — | 組み込み値 | ファイルごとのツール呼び出しラウンドの上限。テンプレートのデフォルトより大きい場合のみ有効 |
| `--max-git-procs` | — | 組み込み値 | gitサブプロセスの最大同時実行数 |
| `--tools` | — | — | カスタムJSONツール設定へのパス |

#### 再開可能なレビューとセッション

すべての `ocr review` 実行は、`~/.opencodereview/sessions/` 配下にローカル
セッションログを保存します。正常終了したテキスト出力はレビュー結果に集中し、session ID
は表示しません。保存済みセッションは `ocr session list/show` で確認でき、
`--format json` では機械可読出力に `session_id` が含まれます。範囲または単一 commit
レビューが中断された場合は、保存済みセッションを一覧表示し、同じレビュー対象に一致するセッションから再開します:

```bash
ocr session list
ocr session show <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

再開は意図的に厳密です。範囲レビューと単一 commit レビューのみ対応し、ワークスペースレビューは再開できません。
現在の `--from/--to` または `--commit` は保存済みセッションと一致する必要があります。`--preview` と `--resume` は併用できません。

`--format json` を使用すると、再開した実行には次が含まれます:

- `session_id` — 現在の実行の session ID
- `resume.resumed_from` — 再開元の session ID
- `resume.reused_files` — 保存済みチェックポイントから再利用したファイル数
- `resume.rerun_files` — 現在の実行で再レビューしたファイル数

### `ocr session`のフラグ

| コマンド | フラグ | デフォルト | 説明 |
|---------|------|---------|------|
| `ocr session list` | `--repo` | カレントディレクトリ | 一覧表示するセッションのリポジトリ |
| `ocr session list` | `--json` | `false` | セッション概要をJSONで出力 |
| `ocr session list` | `--limit` | `20` | 一覧表示するセッション数の上限。`0` は無制限 |
| `ocr session show <id>` | `--repo` | カレントディレクトリ | 確認するセッションのリポジトリ |
| `ocr session show <id>` | `--json` | `false` | セッションメタデータとファイル単位の項目をJSONで出力 |

### `ocr scan`のフラグ

`ocr scan` はdiffではなくファイル全体をレビューします — 不慣れなコードベースの監査、マイグレーション前のスキャン、意味のあるdiffがないディレクトリなどに有用です。非gitディレクトリでも動作します（`.gitignore` を尊重するファイルシステムウォークにフォールバック）。

| フラグ | 短縮形 | デフォルト | 説明 |
|------|-----------|---------|-------------|
| `--path` | — | リポジトリ全体 | カンマ区切りのスキャン対象ディレクトリ/ファイル |
| `--exclude` | — | — | カンマ区切りのgitignoreスタイルパターンでスキップ対象を指定；rule.jsonのexcludesとマージ |
| `--preview` | `-p` | `false` | LLMを実行せずにスキャン対象ファイルを一覧表示 |
| `--max-tokens-budget` | — | `0`（無制限） | トークン使用量の上限；超過するとディスパッチを停止 |
| `--no-plan` | — | `false` | ファイルごとのプランニング前処理をスキップ |
| `--no-dedup` | — | `false` | バッチごとの類似コメント重複排除をスキップ |
| `--no-summary` | — | `false` | プロジェクトレベルのサマリーをスキップ |
| `--batch` | — | `by-language` | バッチ戦略：`none`、`by-language`、または `by-directory` |
| `--format` | `-f` | `text` | 出力形式：`text` または `json`（JSONには `project_summary` フィールドを含む） |
| `--concurrency` | — | `8` | 最大同時ファイルスキャン数 |
| `--rule` | — | — | カスタムJSONレビュールールへのパス |
| `--repo` | — | カレントディレクトリ | スキャン対象のリポジトリまたはディレクトリルート |

各実行前に、`ocr scan` はおおまかなトークンコスト見積もりを表示します。`--preview` でまずファイルリストを確認し、`--max-tokens-budget` で大規模リポジトリの支出を制限できます。

## 例

```bash
# 対話的プロバイダーとモデルのセットアップ
ocr config provider
ocr config model
ocr llm providers

# カスタムプロバイダーを削除
ocr config unset custom_providers.my-gateway

# レビュー対象ファイルをプレビュー（LLM呼び出しなし）
ocr review --preview
ocr review -c abc123 -p

# デフォルト設定でワークスペースの変更をレビュー
ocr review

# 高めの同時実行数でブランチのdiffをレビュー
ocr review --from main --to my-feature --concurrency 4

# 特定のコミットを詳細なJSON出力でレビュー
ocr review --commit abc123 --format json --audience agent

# 中断した範囲または単一 commit レビューを再開
ocr session list
ocr session show <session-id>
ocr review --from main --to my-feature --resume <session-id>
ocr review --commit abc123 --resume <session-id>

# このレビューでモデルを選択またはオーバーライド
ocr review --model claude-opus-4-6
ocr review --commit abc123 --model claude-sonnet-4-6

# 要件コンテキストを提供してより的確なレビューを実施
ocr review --background "ログインAPIにレート制限を追加"

# Markdownファイルから要件コンテキストを提供
ocr review --background-file ./docs/my_business_context.md

# インラインのコンテキストとローカルのコンテキストファイルを組み合わせる（両方が使用されます）
ocr review --background "認証に注目" --background-file ./docs/my_business_context.md

# カスタムレビュールールを使用
ocr review --rule /path/to/my-rules.json

# ファイルに適用されるルールをプレビュー
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# フルファイルスキャン：まずファイルリストをプレビュー（LLM呼び出しなし）
ocr scan --preview

# リポジトリ全体をスキャン、支出を約500kトークンに制限
ocr scan --max-tokens-budget 500000

# サブディレクトリをスキャン、生成ファイル/テストファイルをスキップ
ocr scan --path internal --exclude '**/*_test.go,**/generated/**'

# 非gitディレクトリをJSON出力でスキャン（project_summaryを含む）
ocr scan --repo /path/to/plain/dir --format json

# 最速スキャン：プランニング、重複排除、プロジェクトサマリーをスキップ
ocr scan --no-plan --no-dedup --no-summary

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
      "rule": "All new methods must validate required parameters for null values",
      "merge_system_rule": true
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags"
    }
  ]
}
```

- `path`は`**`による再帰マッチと`{java,kt}`のブレース展開をサポートします。
- `merge_system_rule`は任意です。`true`の場合、一致した組み込みシステムルールがこのユーザールールとマージされます。
- 各層の中では、ルールは宣言順に評価されます — 最初にマッチしたものが採用されます。
- ルールファイルが存在しない場合は、何も出力せずスキップされます。

**`rule` フィールドはインラインコンテンツとファイルパスの両方をサポートします。** システムは次の順序で自動判別します：

1. 値に改行が含まれる → **インラインコンテンツ**（複数行ルールがファイルパスと見なされることはありません）。
2. 値が単一行で、スペースを含まず、`.md` / `.txt` / `.markdown` で終わる → **ファイルパス**。
   - 絶対パス（`/` で始まる）はそのまま使用されます。
   - 相対パスはプロジェクトルートで解決されます。パストラバーサル（例: `../../etc/passwd.md`）はブロックされます。見つからない場合は `[WARN]` を出力し、ルールはクリアされます（インラインへのフォールバックなし）。
   - ファイルはバリデーションを通過する必要があります：ホワイトリスト拡張子、≤ 512 KB、シンボリックリンク解決後のターゲットもホワイトリスト拡張子であること。バリデーションに失敗した場合、ルールはクリアされます。
3. それ以外 → **インラインコンテンツ**。

```json
{
  "rules": [
    {
      "path": "**/*mapper*.xml",
      "rule": "docs/sql-rules.md"
    },
    {
      "path": "**/*.java",
      "rule": "Always check for null safety and resource leaks"
    },
    {
      "path": "**/*.go",
      "rule": "shared/go-concurrency.md"
    },
    {
      "path": "**/*.py",
      "rule": "/Users/me/team-rules/python.md"
    }
  ]
}
```

- `docs/sql-rules.md` — 相対パス、`<project>/docs/sql-rules.md` から読み込み。
- `Always check for null safety…` — インライン文字列、そのまま使用。
- `shared/go-concurrency.md` — 相対パス、同様に解決。
- `/Users/me/team-rules/python.md` — 絶対パス、そのまま使用。

> 絶対パスはプロジェクト外のファイルにアクセスできますが、これは意図的な設計です。`rule.json` はメンテナが作成する信頼された入力のためです。共有ルールを共通パス（例：`/opt/company-rules/`）に置くことで、各プロジェクトへのコピーが不要になります。

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
| `provider` | string | `anthropic` \| `openai` \| `dashscope` \| `deepseek` \| `z-ai` |
| `providers.<name>.api_key` | string | プロバイダー固有のAPIキー |
| `providers.<name>.url` | string | プロバイダーのベースURLオーバーライド |
| `providers.<name>.protocol` | string | `anthropic` \| `openai` \| `openai-responses` |
| `providers.<name>.model` | string | プロバイダーのモデル名 |
| `providers.<name>.models` | array | 対話的選択に使う任意のプロバイダーモデル一覧 |
| `providers.<name>.auth_header` | string | `x-api-key` \| `authorization` |
| `providers.<name>.extra_body` | object | すべてのリクエストボディにマージされるJSONオブジェクト |
| `providers.<name>.timeout_sec` | integer | リクエストごとのHTTPタイムアウト（秒）、デフォルト `300` |
| `providers.<name>.extra_headers` | string | カンマ区切りの `key=value` HTTPヘッダー |
| `custom_providers.<name>.*` | — | 任意の`models`を含む`providers.<name>.*`と同じフィールド |
| `llm.url` | string | `https://api.openai.com/v1/chat/completions` |
| `llm.auth_token` | string | `sk-xxxxxxx` |
| `llm.auth_header` | string | Anthropicのみ：`x-api-key` \| `authorization` |
| `llm.extra_body` | object | すべてのリクエストボディにマージされるJSONオブジェクト |
| `llm.timeout_sec` | integer | リクエストごとのHTTPタイムアウト（秒）、デフォルト `300` |
| `llm.extra_headers` | string | カンマ区切りの `key=value` HTTPヘッダー |
| `llm.model` | string | `claude-opus-4-6` |
| `llm.protocol` | string | `anthropic` \| `openai` \| `openai-responses`；`llm.use_anthropic` より優先 |
| `llm.use_anthropic` | boolean | `true` \| `false`（レガシー；`llm.protocol` を推奨） |
| `mcp_servers.<name>.command` | string | MCPサーバーを起動するコマンド |
| `mcp_servers.<name>.args` | array | MCPサーバーのコマンドライン引数 |
| `mcp_servers.<name>.env` | array | 環境変数（`KEY=VALUE`形式） |
| `mcp_servers.<name>.tools` | array | 許可するツール名（空の場合はすべてのツール） |
| `mcp_servers.<name>.setup` | string | サーバー起動前に実行するセットアップコマンド |
| `language` | string | 任意の言語名、例：`English`、`Chinese`（デフォルト：`English`） |
| `telemetry.enabled` | boolean | `true` \| `false` |
| `telemetry.exporter` | string | `console` \| `otlp` |
| `telemetry.otlp_endpoint` | string | OTLPコレクターのアドレス |
| `telemetry.content_logging` | boolean | テレメトリーにプロンプトを含める |

環境変数は設定ファイルより優先されます。

### MCPサーバー

Open Code Reviewは[Model Context Protocol (MCP)](https://modelcontextprotocol.io/)サーバーをサポートしており、レビューエージェントがstdioトランスポートを介してコードレビュー中に外部ツールを使用できます。

CLIからMCPサーバーを設定します：

```bash
# MCPサーバーを追加
ocr config set mcp_servers.<name>.command <command>
ocr config set mcp_servers.<name>.args '["arg1","arg2"]'
ocr config set mcp_servers.<name>.env '["KEY=VALUE"]'
ocr config set mcp_servers.<name>.tools '["tool_name"]'
ocr config set mcp_servers.<name>.setup '<setup command>'

# MCPサーバーを削除
ocr config unset mcp_servers.<name>
```

| フィールド | 必須 | 説明 |
|-----------|------|------|
| `command` | はい | MCPサーバーを起動する実行コマンド |
| `args` | いいえ | サーバーに渡すコマンドライン引数 |
| `env` | いいえ | 環境変数（`KEY=VALUE`形式） |
| `tools` | いいえ | 許可するツール名。空の場合、サーバーのすべてのツールが利用可能 |
| `setup` | いいえ | サーバー起動前に実行するシェルコマンド（例：インデックスの構築） |

> **注意：** MCPツールの名前が組み込みツールと競合する場合、そのツールは警告付きでスキップされます。`setup`コマンドのタイムアウトは5分です。

**例：[CodeGraph](https://github.com/nicholasgasior/codegraph)を追加してコード構造分析を強化**

```bash
ocr config set mcp_servers.codegraph.command codegraph
ocr config set mcp_servers.codegraph.args '["serve","--mcp"]'
ocr config set mcp_servers.codegraph.tools '["codegraph_explore"]'
ocr config set mcp_servers.codegraph.setup 'codegraph init && codegraph index'
```

### 環境変数

| 変数 | 用途 |
|----------|---------|
| `OCR_LLM_URL` | LLM APIエンドポイントURL |
| `OCR_LLM_TOKEN` | APIキー / 認証トークン |
| `OCR_LLM_AUTH_HEADER` | Anthropic認証ヘッダー（`x-api-key`または`authorization`） |
| `OCR_LLM_EXTRA_HEADERS` | カンマ区切りの `key=value` HTTPヘッダー |
| `OCR_LLM_MODEL` | モデル名 |
| `OCR_LLM_PROTOCOL` | プロトコル：`anthropic` \| `openai` \| `openai-responses`；`OCR_USE_ANTHROPIC` より優先 |
| `OCR_LLM_TIMEOUT` | リクエストごとのHTTPタイムアウト（秒）、設定ファイルの `timeout_sec` を上書き |
| `OCR_USE_ANTHROPIC` | `true` = Anthropic、`false` = OpenAI Chat Completions（レガシー；`OCR_LLM_PROTOCOL` を推奨） |

## テレメトリー

可観測性（スパン、メトリクス）のためのOpenTelemetry統合。デフォルトでは無効です。

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

エクスポートデータにLLMのプロンプトとレスポンスを含めるには、`telemetry.content_logging`を設定してください。

**プロトコル選択：** 環境変数 `OTEL_EXPORTER_OTLP_PROTOCOL` でエクスポートプロトコルを選択できます：

| 値 | トランスポート | 説明 |
|---|---|---|
| `grpc`（デフォルト） | gRPC | デフォルトポート 4317 |
| `http/protobuf` | HTTP | デフォルトポート 4318 |

**Endpoint 形式：** `telemetry.otlp_endpoint` は `host:port` または `http://host:port` 形式のベースURLを指定します。パスを含める必要はありません。SDKが [OTLP仕様](https://opentelemetry.io/docs/specs/otlp/#otlphttp-request)に従いシグナルパス（例：`/v1/traces`）を自動的に付加します。

## コントリビューション

このプロジェクトは、貢献してくださるすべての方々のおかげで成り立っています。開発環境のセットアップ、コーディングガイドライン、プルリクエストの提出方法については[CONTRIBUTING.md](CONTRIBUTING.md)を参照してください。

<a href="https://github.com/alibaba/open-code-review/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=alibaba/open-code-review" />
</a>

## ライセンス

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
