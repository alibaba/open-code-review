---
title: Qwen Code
sidebar:
  order: 3
---

リポジトリにある Claude Code Marketplace プラグインを使い、Qwen Code で OCR を
実行します。インストーラーがプラグインを変換し、レビューコマンドを読み込みます。

以下の手順は Windows 上の Qwen Code 0.27.2 で検証済みです。別途公開された
[Qwen Code 1.0 Preview](https://github.com/QwenLM/qwen-code/releases/tag/v1.0.0-preview)
との互換性は未検証です。

## 前提条件

Qwen Code と Git 2.41 以降をインストールし、OCR CLI を導入します。

```bash
npm install -g @alibaba-group/open-code-review
```

## インストール

ターミナルで実行します。

```bash
qwen extensions install alibaba/open-code-review:open-code-review
qwen extensions list
```

CLI でインストールした後、新しい Qwen Code セッションを開始してください。
プラグインは[同梱のコマンドプロンプト](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code/commands)を再利用します。

## レビューの実行

OCR が呼び出す LLM を設定し、接続を確認します。

```bash
ocr config provider
ocr config model
ocr llm test
```

Qwen Code の組み込み `/review` が有効な場合、次のコマンドで OCR を呼び出します。

```text
/open-code-review.review
/open-code-review.review --from main --to feature
/open-code-review.review --commit abc123
```

Qwen Code は組み込みの `/review` を保持し、競合する OCR コマンドに
`open-code-review.` を付けます。組み込みのレビュースキルが無効なら、
OCR は `/review` を使用できます。セッション内の `/help` で実際のコマンド名を
確認してください。

## 委譲モード

Qwen Code 自身のモデルと認証でレビューするには、次を実行します。

```text
/delegate-review
/delegate-review --from main --to feature
/delegate-review --commit abc123
```

OCR は `ocr delegate preview` と `ocr delegate rule` でファイルの選択とルールの
解決を行い、Qwen Code がレビューします。OCR CLI は必要ですが、OCR 側で LLM を
別途設定する必要はありません。

別のコマンドが `/delegate-review` を使用している場合、OCR のコマンドは
`/open-code-review.delegate-review` として登録されます。

どちらの同梱コマンドも、採用した指摘に対して自動的に修正を適用する場合があります。

## マーケットプレイスでの検索

このリポジトリをソースとして追加すると、Discover タブでプラグインを参照できます。

```bash
qwen extensions sources add alibaba/open-code-review
```

Qwen Code 内で `/extensions` を実行し、Discover を開きます。

## 関連情報

- [委譲モード](../delegate/)
- [設定](../configuration/)
- [Qwen Code 拡張機能のドキュメント](https://qwenlm.github.io/qwen-code-docs/en/users/extension/introduction/)
