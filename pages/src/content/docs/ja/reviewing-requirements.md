---
title: 要件に基づくレビュー
sidebar:
  order: 8
---

関連する受け入れ基準を OCR に渡すと、コードの不具合に加えて業務上の動作も確認できます。
推奨されるワークフローは、要件を簡潔なローカル Markdown ファイルにまとめ、
`--background-file` で渡す方法です。

## 変更ごとの要件ファイル

今回の変更に必要な情報を、`docs/requirements/checkout.md` などの簡潔なローカル
Markdown ファイルにまとめます。

```markdown
# Checkout requirements

Scope: src/checkout/**
Review changes against these criteria and cite the requirement ID in findings.

- PAY-1: Reject checkout when the cart is empty.
- PAY-2: Calculate the total on the server; never trust a client-supplied total.
- PAY-3: A repeated idempotency key must not create a second charge.

Out of scope: refunds and subscription billing.
```

OCR の設定が済み、ローカルに `main` がある状態で、リポジトリルートから実行します。

```bash
ocr review --from main --to HEAD --background-file docs/requirements/checkout.md
```

`--background-file` はレビュー全体にコンテキストを渡します。相対パスの基準は
**リポジトリルート**で、サブディレクトリから実行しても変わりません。絶対パスも
使用できます。`--background` も指定した場合はファイルが優先され、インラインの
テキストは無視されます。内容は簡潔にしてください。サニタイズ後の文字数が
2,000 を超えると警告され、8,000 を超えると拒否されます。入力ファイルは 1 MiB 以下で、
サニタイズ後も空でない必要があります。

## 社内ナレッジベースの要件

社内ナレッジベースを使う場合は、関連する要件、制約、受け入れ基準を簡潔なローカル
Markdown に書き出し、`--background-file` で渡します。background 本文にファイル名や
リモートリンクを書くだけでは、その文書やリンク先は自動的に読み込まれません。
基準そのものを含め、指摘内容を仕様やテストと照合してください。

## 現在の制限

OCR は要件を 1 回のレビューのコンテキストとして扱います。現在、レビュールールとは
独立して参照文書をモジュールやディレクトリに関連付ける専用の仕組みはありません。
パス別ルールは一致したファイルに適用するルール本文を選ぶものであり、参照資料を
添付する仕組みではないためです。ナレッジベースのユースケースが成熟すれば、
専用のサポートを別途設計できます。

## 関連項目

- [CLI リファレンス](../cli-reference/): `ocr review --background-file` とその他のレビューフラグ。
- [レビュールール](../review-rules/): ルールの階層、パス別ルール、ルールの解決。
