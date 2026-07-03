---
title: クイックスタート
sidebar:
  order: 3
---

OCR をインストールし、Anthropic Messages API または OpenAI Chat Completions API
に対応した任意の LLM に接続して、初回のコードレビューを実行しましょう。

## 前提条件

- 動作する **Git** のインストール——OCR は Git をサブプロセスとして駆動し diff を読み取ります。
- Anthropic または OpenAI 互換の LLM の **API key**。
- 以下のいずれか：
  - **Node.js ≥ 18**（推奨。最低サポートは Node 14——NPM 経由でインストール）。
  - または `curl` + `chmod` だけで静的バイナリを `$PATH` に配置。
  - またはソースからビルドしたい場合は **Go ≥ 1.25**。

## ステップ 1——CLI をインストールする

### 方法 A：NPM（推奨）

```bash
npm install -g @alibaba-group/open-code-review
```

NPM パッケージは小さな wrapper をインストールし、インストール時に（postinstall hook を通じて）お使いの
OS / アーキテクチャ向けの正しいバイナリをダウンロードします。実行時にバイナリが存在しない場合、wrapper はエラーを出し、
ダウンロードは行いません。インストール後、グローバルな `ocr` コマンドが利用できます。

```bash
ocr --version
```

### 方法 B：GitHub Release バイナリ

[releases ページ](https://github.com/alibaba/open-code-review/releases)
から対応プラットフォームのバイナリを選び、`$PATH` に配置します。

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux x86_64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux ARM64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (AMD64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

### 方法 C：ソースからビルドする

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

> 各インストール方法の詳細は [インストール](../installation/) を参照してください。NPM wrapper が
> プラットフォームバイナリをどのように解決するかも含まれています。

## ステップ 2——LLM を設定する

完全な LLM エンドポイント（URL + token + model）を解決できるまで、OCR はレビューの実行を拒否します。
以下の優先順位で 4 つの来源を探索します。

1. `~/.opencodereview/config.json`
2. OCR 専用の環境変数（`OCR_LLM_*`）
3. Claude Code の環境変数（`ANTHROPIC_*`）
4. シェルの rc ファイル（`~/.zshrc`、`~/.bashrc`、`~/.bash_profile`、
   `~/.profile`）から解析された `export ANTHROPIC_*` 行

### 最速の経路：`ocr config set`

```bash
ocr config set llm.url           https://api.anthropic.com/v1/messages
ocr config set llm.auth_token    sk-ant-xxxxxxxxxx
ocr config set llm.model         claude-opus-4-6
ocr config set llm.use_anthropic true
```

これらの値は `~/.opencodereview/config.json` に永続化されます。

### 代替方法：環境変数

優先度が最も高く、ディスクに設定ファイルを残したくない CI / コンテナに適しています。

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=sk-ant-xxxxxxxxxx
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true   # デフォルトは true。false にすると OpenAI プロトコルを使用
```

### すでに Claude Code を使っている場合

OCR は Claude Code が使用するのと同じ変数群を自動的に読み取るため、追加の設定は不要です。

```bash
export ANTHROPIC_BASE_URL=https://api.anthropic.com
export ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxxxxxxx
export ANTHROPIC_MODEL=claude-opus-4-6
```

`ANTHROPIC_BASE_URL` にバージョン付きのパスがない場合、OCR は自動的に
`/v1/messages` を追加します。

### OpenAI 互換エンドポイントを使う場合

`llm.use_anthropic` を `false` に設定します（または `OCR_USE_ANTHROPIC=false`）。

```bash
ocr config set llm.url           https://api.openai.com/v1/chat/completions
ocr config set llm.auth_token    sk-xxxxxxxxxx
ocr config set llm.model         gpt-4o
ocr config set llm.use_anthropic false
```

> 完全な key のリファレンスは [設定](../configuration/) を参照してください。ベンダー固有のリクエストフィールド
> 用の `llm.extra_body` や、レビューコメントの言語を切り替える `language` も含まれています。

## ステップ 3——接続性をテストする

```bash
ocr llm test
```

期待される出力（モデル名は異なります）：

```
Source: OCR config file
URL:    https://api.anthropic.com/v1/messages
Model:  claude-opus-4-6
Hello! …
```

代わりに `no valid LLM endpoint configured` のようなエラーが出た場合は、上記の
設定 key を再確認してください。401 / 403 は token が誤っているか期限切れであることを示します。

## ステップ 4——初回のレビューを実行する

任意の Git リポジトリに移動して実行します。

```bash
cd path/to/your-repo

# ワークスペースモード——staged + unstaged + untracked の変更をレビュー（デフォルト）
ocr review

# ブランチ区間——`main..feature-branch` をレビュー
ocr review --from main --to feature-branch

# 単一 commit——その commit が導入した diff をレビュー
ocr review --commit abc123
```

進捗情報が継続的に出力され、最後に各ファイルに 1 つ以上のレビューコメントが表示されます。

> ワークスペースモードには **untracked** ファイルが含まれます。すでにステージされた内容だけをレビューしたい場合は、まず
> `git add` で選択的にステージしてください。

> 上記の 3 つは基本的な使い方です。`ocr review` の完全な引数（並行数のチューニング、出力形式、
> audience モード、背景コンテキストなど）と、その他すべてのサブコマンド（`config`、`rules`、
> `llm test`、`viewer`）は [CLI リファレンス](../cli-reference/) を参照してください。

### 先に *何が* レビューされるか見てみたい場合

```bash
ocr review --preview         # ワークスペース
ocr review -c abc123 -p      # commit
```

`--preview` は各フィルタステップを実行しますが LLM は一切呼び出さないため、token を消費しません。ファイルリストと
各ファイルのステータス（`added` / `modified` / `deleted` / `renamed` / `binary`）を出力し、
除外されたファイルについてはその理由（`binary`、`unsupported_ext`、`default_path`、
`user_exclude`、`deleted`）も示します。

### ツール向けの JSON 出力

```bash
ocr review --format json --audience agent > review.json
```

- `--format json` は機械可読なコメント配列を出力します。各コメントには `path`、`content`、
  `start_line`、`end_line`、`existing_code`、`suggestion_code`、およびオプションの
  `thinking` が含まれます。
- `--audience agent` は人間向けの進捗 UI を抑制し、stdout を JSON / 最終
  サマリーだけにします——上流の agent や CI スクリプトが必要とするものです。

## ステップ 5——結果を確認する

各コメントには以下が含まれます。

| フィールド | 意味 |
|---|---|
| `path` | そのコメントが対象とするファイル。 |
| `content` | レビューコメント本体。設定された `language` を使用します。 |
| `start_line` / `end_line` | ファイルの **新しい** バージョンにおける行範囲。両方が `0` の場合は OCR がコメントを正確に配置できなかったことを意味します——問題は本物ですが、正確な位置は自分で特定する必要があります。 |
| `existing_code` | コメントが指す diff の断片。内部的に行解決に使われます。`start_line` が `0` のときに役立ちます。 |
| `suggestion_code` | オプションの修正断片。 |
| `thinking` | オプションのモデルの推論。一部のモデルにのみ存在します。 |

## ステップ 6——過去のセッションを閲覧する

各レビューは JSONL トランスクリプトとして
`~/.opencodereview/sessions/...` に永続化されます。ローカルの Web UI でそれらを閲覧できます。

```bash
ocr viewer            # http://localhost:5483
ocr viewer --addr :3000
```

> UI の完全な紹介は [セッションビューア](../viewer/) を参照してください。

## 関連項目

- [CLI リファレンス](../cli-reference/)——各サブコマンド、引数、出力モード。
- [レビュールール](../review-rules/)——レビュー内容をカスタマイズします。
- [インテグレーション](../integrations/)——OCR を Claude Code、Agent skill、CI に組み込みます。
- [テレメトリ](../telemetry/)——OTLP 経由で trace と metrics を送信します。
- [FAQ](../faq/)——既知のエラーと対策。
