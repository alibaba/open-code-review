<div align="center">
  <a href="https://open-codereview.ai">
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

詳細は[公式サイト](https://open-codereview.ai)をご覧ください。

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

Windows（PowerShell 5.1+）では：

```powershell
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
```

このスクリプトは適切な Windows リリースバイナリを選択し、SHA-256 チェックサムを検証して、`ocr.exe` として `%LOCALAPPDATA%\Programs\ocr` にインストールします。インストール先は `OCR_INSTALL_DIR` で、リリースバージョンは `OCR_VERSION` で上書きできます：

```powershell
$env:OCR_INSTALL_DIR = "$env:USERPROFILE\bin"
$env:OCR_VERSION = "v1.3.13"
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
```

リモートスクリプトをシェルに直接パイプすると、インターネット上のコードが実行されます。先にダウンロードして内容を確認してから実行することを推奨します：

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh -o install.sh
less install.sh && sh install.sh
```

```powershell
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 -OutFile install.ps1
notepad install.ps1   # 確認後: .\install.ps1
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

# デリゲートモード — AI コーディングエージェントが自らレビューを実行
# OCR はファイル選択とルール解決を担当。LLM 設定不要
ocr delegate preview
ocr delegate rule src/main.go src/handler.go
```

### コーディングエージェントとの統合

OCRはスラッシュコマンドとしてAIコーディングエージェントにシームレスに統合でき、エージェントのワークフロー内で直接コードレビューが可能になります。

#### オプション1: Skillとしてインストール

`npx`を使ってOCRスキルをプロジェクトにインストールします：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

これにより、[skillsレジストリ](skills/open-code-review/SKILL.md)から`open-code-review`スキルがインストールされ、コーディングエージェントにコードレビューのための`ocr`の呼び出し方、優先度による問題の分類、必要に応じた修正の適用を教えます。

**デリゲートモード** — コーディングエージェント自身がレビューを実行する場合（OCR はファイル選択とルール解決のみを担当、OCR 側の LLM 設定不要）：

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

詳細は [skills/open-code-review-delegate/SKILL.md](skills/open-code-review-delegate/SKILL.md) を参照。

#### オプション2: Claude Codeプラグインとしてインストール

[Claude Code](https://docs.anthropic.com/en/docs/claude-code)の場合、Claude Code内で以下のコマンドを実行してコマンドプラグインをインストールします：

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

これにより`/open-code-review:review`スラッシュコマンドが登録され、OCRを実行して問題を自動的にフィルタリング・修正します。また、`/open-code-review:delegate-review` デリゲートモードコマンドも提供されます（エージェントが自身の能力でレビューを実行し、OCR はファイル選択とルール解決を担当）。

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

デリゲートモード（OCR 側の LLM 設定不要）：

```bash
# プロジェクトレベル
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review-delegate.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md

# ユーザーレベル
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review-delegate.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

> **前提条件**：すべての統合方法には `ocr` CLI のインストールが必要です。標準モードではさらに LLM の設定が必要です — 上記の[インストール](#インストール)と[LLM の設定](#1-llm-の設定)を参照。デリゲートモードでは OCR 側の LLM 設定は**不要**です。

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
- [`gerrit_ci/`](./examples/gerrit_ci/) — Gerrit (Jenkins / Gerrit Trigger) 統合の例

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

## ドキュメント

完全なドキュメントは **[open-codereview.ai/docs](https://open-codereview.ai/docs)** にあります：

- [クイックスタート](https://open-codereview.ai/docs/quickstart) — インストールして最初のレビューを実行
- [インストール](https://open-codereview.ai/docs/installation) — すべてのプラットフォームとパッケージマネージャー
- [CLI リファレンス](https://open-codereview.ai/docs/cli-reference) — すべてのコマンドとフラグ
- [レビュールール](https://open-codereview.ai/docs/review-rules) — ルールの優先順位チェーン、ファイル形式、パスフィルタリング
- [設定](https://open-codereview.ai/docs/configuration) — 設定キーと環境変数
- [MCP サーバー](https://open-codereview.ai/docs/mcp) — 外部ツールでレビューエージェントを拡張
- [コーディングエージェント連携](https://open-codereview.ai/docs/claude-code) — Claude Code、Agent Skill、委譲モード
- [CI/CD 連携](https://open-codereview.ai/docs/cicd) — パイプラインでレビューを実行
- [アーキテクチャ](https://open-codereview.ai/docs/architecture) · [ツール](https://open-codereview.ai/docs/tools) · [セッションビューアー](https://open-codereview.ai/docs/viewer) · [テレメトリー](https://open-codereview.ai/docs/telemetry) · [FAQ](https://open-codereview.ai/docs/faq)

## コマンド

OCR は `review`、`scan`、`delegate`、`config`、`llm`、`session`、`viewer` などのコマンドを提供します。コマンドの完全な一覧とすべてのフラグ（再開可能なレビューや `ocr scan` / `ocr delegate` の全オプションを含む）については、**[CLI リファレンス](https://open-codereview.ai/docs/cli-reference)** を参照してください。

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

# デリゲートモード — AI エージェントがレビューを実行（LLM 設定不要）
ocr delegate preview
ocr delegate preview --from main --to feature-branch
ocr delegate preview --commit abc123
ocr delegate rule internal/handler.go internal/service.go cmd/main.go

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

OCR は 4 層の優先順位チェーン（`--rule` フラグ > プロジェクト設定 > グローバル設定 > 組み込みデフォルト）でレビュールールを解決し、インラインまたはファイルベースのルール、`**` グロブマッチング、`include` / `exclude` のパスフィルタリングをサポートします。ルールファイルの完全な形式とフィルタリングの意味については、**[レビュールール](https://open-codereview.ai/docs/review-rules)** を参照してください。

## 設定リファレンス

設定は `~/.opencodereview/config.json` にあり、環境変数で上書きできます。プロバイダー、モデル、MCP サーバー、言語、テレメトリーをカバーします。設定キーの完全なリファレンス、環境変数、MCP サーバーのセットアップについては、**[設定](https://open-codereview.ai/docs/configuration)** と **[MCP サーバー](https://open-codereview.ai/docs/mcp)** を参照してください。

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
