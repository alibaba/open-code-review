---
title: 統合
sidebar:
  order: 12
---

OCR は CLI であり、プロセスを派生できるあらゆる環境と組み合わせられます。本セクションでは、
エージェント型ワークフローや CI に組み込むための主要な方法を、統合方式ごとに 1 ページずつ扱います。

## なぜこれらの統合なのか？

OCR の `--audience agent` モードは、別のエージェントから駆動されることを前提に設計されています。
stdout には JSON / 最終サマリーのみが流れ、進捗 UI はありません。これにより、次の 3 つの組み合わせ方が
自然に成り立ちます。

1. **Agent skill**——呼び出し側のエージェントが呼べる skill として OCR を登録します（Anthropic
   Agent SDK など）。
2. **Command（Claude Code plugin）**——パッケージ化されたコマンドをインストールし、
   `/open-code-review:review` で `ocr review` をエンドツーエンドに実行します。Claude-Code スタイルの
   コマンド規約をサポートする他のエージェントでも利用できます。
3. **Direct subprocess**——`subprocess.run` を呼べるあらゆるフレームワーク（LangChain ツール、
   カスタムシェル、CI ステップ）から直接 shell 経由で呼び出します。

これらは組み合わせて使えます。skill も plugin も、最終的には同じバイナリを呼び出します。

## モードを選ぶ

| 方式 | 最適なケース | ページ |
|---|---|---|
| Agent skill | Anthropic Agent SDK、または `SKILL.md` を消費する他のフレームワーク上で構築している。 | [Agent Skill](agent-skill/) |
| Command（Claude Code plugin） | Claude Code（または Claude-Code スタイルのコマンド規約を持つ任意のエージェント）を使っていて、`/open-code-review:review` に正しい動作をさせたい。 | [Command（Claude Code Plugin）](claude-code/) |
| Direct subprocess | カスタムスクリプト、LangChain ツール、あるいは Anthropic 以外のエージェントから OCR を呼び出す必要がある。 | [Direct Subprocess](subprocess/) |
| CI/CD | すべての PR や pre-commit のたびに OCR を実行したい。 | [CI/CD](ci/) |

## MCP はどうなのか？

OCR は現在、Model Context Protocol server を公開していません。想定している統合方式は
「エージェントが CLI を呼び出す」というもので、よりシンプルであり、MCP server が持ち込む
長時間稼働プロセスの問題を避けられます。エージェントプラットフォームがどうしても MCP を
必要とする場合は、CLI を薄い shim でラップしてください。単一の `review` ツールを公開する
30 行程度の Node スクリプトで十分です。

## すべてのモードに共通するヒント

- **呼び出し側が人間でない場合は、常に `--audience agent` を渡してください。** そうしないと、
  進捗行が解析対象の出力を汚染します。
- **PR / 要件のコンテキストがある場合は、常に `--background` を渡してください。** 品質が大きく
  向上し、コストはツール引数 1 つ分にすぎません。
- **CI では `--concurrency` を低めに設定してください**（`--concurrency 4`）。プロバイダーの
  レート制限に触れないようにするためです。デフォルトは 8 です。
- **CI では `--commit HEAD` よりも `--from origin/main --to HEAD` を優先してください。**
  merge-base 計算により、ブランチを切った後に `main` に取り込まれた無関係な変更を除外できます。
- **`OCR_LLM_TOKEN` を stdout/logs から遠ざけてください。** OCR はこれを出力しませんが、
  設定を誤った shell が漏らす可能性があります。CI の secret マスクを使ってください。

## 関連項目

- [CLI リファレンス](../cli-reference/)——review コマンドの各引数。
- [設定](../configuration/)——環境変数と config key。
- [クイックスタート](../quickstart/)——初回レビューのための最小構成。
