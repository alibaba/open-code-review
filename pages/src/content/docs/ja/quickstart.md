---
title: クイックスタート
sidebar:
  order: 3
---

数分で初回のコードレビューを実行できます。

## 前提条件

- **Git ≥ 2.41**
- **Node.js ≥ 18**
- **LLM API key**

## ステップ 1 —— CLI をインストールする

```bash
npm install -g @alibaba-group/open-code-review
ocr version
```

> その他の方法は [インストール](../installation/) を参照してください。

## ステップ 2 —— LLM を設定する

```bash
ocr config provider
```

組み込みまたはカスタムの provider を選択し、API key を入力し、model を選び、すべてを設定ファイルに保存したうえで、`ocr llm test` を 1 回実行してエンドポイントを検証します。あとで model を切り替えるには：

```bash
ocr config model
```

### 代替方法：非インタラクティブコマンド

CI や TUI のない環境では、`ocr config set` で同じ設定に直接書き込みます。

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

## ステップ 3 —— 接続性をテストする

```bash
ocr llm test
```

`no valid LLM endpoint configured` のようなエラーが出た場合は、ステップ 2 の設定を再確認してください。401 / 403 は token が誤っているか期限切れであることを示します。

## ステップ 4 —— 初回のレビューを実行する

任意の Git リポジトリに移動して実行します。

```bash
cd path/to/your-repo

# ワークスペースモード —— staged + unstaged + untracked の変更をレビュー（デフォルト）
ocr review

# ブランチ区間 —— `main..feature-branch` をレビュー
ocr review --from main --to feature-branch

# 単一 commit —— その commit が導入した diff をレビュー
ocr review --commit abc123
```

> `ocr review` の完全な引数（並行数のチューニング、出力形式、audience モード、背景コンテキストなど）と、その他すべてのサブコマンドは [CLI リファレンス](../cli-reference/) を参照してください。

### 先に何がレビューされるか見てみたい場合

```bash
ocr review --preview              # ワークスペース
ocr review -c abc123 --preview    # commit
```

### システム向けの JSON 出力

`--audience agent` は人間向けの進捗 UI を抑制し、stdout を JSON / 最終サマリーだけにします —— 上流の agent や CI スクリプトが必要とするものです。

```bash
ocr review --format json --audience agent > review.json
```

## 関連項目

- [インストール](../installation/) —— すべてのインストール方法と OCR の状態ディレクトリ。
- [設定](../configuration/) —— 各環境変数、config key、組み込み provider。
- [CLI リファレンス](../cli-reference/) —— 各サブコマンド、引数、出力モード。
- [レビュールール](../review-rules/) —— レビュー内容をカスタマイズします。
- [インテグレーション](../integrations/) —— OCR を Claude Code、Agent skill、CI に組み込みます。
- [FAQ](../faq/) —— 既知のエラーと対策。
