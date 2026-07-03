---
title: Agent Skill
sidebar:
  order: 1
---

OCR を呼び出し可能な skill として登録することで、agent フレームワークが正しい引数、事前チェック、分類基準を用いて OCR を呼び出せるようになります。呼び出し側でこれらを改めて導き出す必要はありません。

## リポジトリに含まれるもの

リポジトリには
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
に SKILL マニフェストが用意されています。これは OCR を呼び出し可能な skill として宣言し、事前チェック、呼び出しワークフロー、コメントの分類基準（High/Medium/Low）を含みます。

## インストール

### 方法 1：`npx skills add`（推奨）

skill を使いたいプロジェクト内で実行します。

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

これは
[skills registry](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
からマニフェストを取得してプロジェクトに配置し、skills の規約を尊重するコーディング agent が次回の呼び出し時にそれをロードするようにします。skill を最新版に更新するには、このコマンドを再実行してください。

> **前提条件：** 初回実行時、バイナリが `PATH` 上に存在しない場合、skill は
> （`npm install -g @alibaba-group/open-code-review` を通じて）`ocr` CLI を自動的にインストールします——[skill が行うこと](#what-the-skill-does)を参照してください。ただし、LLM は事前に設定しておく**必要があります**。skill が代わりに行うことはできず、処理を止めて尋ねます。[設定](../../configuration/)を参照してください。

### 方法 2：手動コピー（システムレベル）

プロジェクトごとではなくグローバルに skill をインストールしたい場合は、フォルダを skills ディレクトリにコピーします。

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

これにより、マシン上のすべてのプロジェクトで skill が利用可能になります。

## skill が行うこと

SKILL.md は一つの prompt です。呼び出し側の agent がそれをロードすると、agent 自身が手順を実行します。完全な `/open-code-review`（または同等）のリクエストは、次のように展開されます。

1. **事前チェック。** `which ocr` を実行して CLI が `PATH` 上にあることを確認し、続いて `ocr llm test` で LLM に到達可能であることを確認します。
2. **CLI が無ければ自動インストール。** `which ocr` が "NOT INSTALLED" を報告した場合、agent は `npm install -g @alibaba-group/open-code-review` を実行して続行します。ユーザーへの確認は行いません——これは通常のセットアップ手順とみなされます。
3. **LLM 設定が無ければ止めて尋ねる。** `ocr llm test` が失敗した場合、agent は認証情報を*でっち上げません*。サポートされている 2 つの方法（環境変数または `ocr config set …`）をユーザーに提示し、API key の提供を待ちます。
4. **業務コンテキストの抽出。** レビュー対象（commit、ブランチ、作業コピー）を確認し、短い `--background` 文字列を生成します。
5. **レビューの実行。**
   `ocr review --audience agent --background "…" [--commit | --from/--to]`
   を呼び出します。ユーザーが作業コピー、特定の commit、ブランチ区間のいずれをレビューしたいかに応じて引数を選択します。
6. **分類とレポート。** SKILL.md の基準を用いて JSON コメントを **High** /
   **Medium** / **Low** に分類し（bug とセキュリティ問題は High、些細な指摘や誤検知と疑われるものは黙って破棄）、Markdown 形式のサマリーをレンダリングします。
7. **必要に応じて修正。** ユーザーが「レビュー**して**修正して」（または類似）と指示した場合、High/Medium 項目に対して安全な修正をインラインで適用します。そうでない場合はコードを変更する前に尋ねます。

完全な prompt——正確な分類基準、出力テンプレート、注意事項を含む——は
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
にあります。上記のいずれかを厳しくしたい場合（例えばデフォルトの動作を、修正前に常に尋ねるように変更するなど）は、ローカルのコピーを編集してください。

## Anthropic Agent SDK

SDK の init をインストール済みの skill パスに向けます。

```python
from anthropic_agent_sdk import Agent

agent = Agent(
    skill_paths=["/path/to/open-code-review/skills/open-code-review"],
)

agent.run("Review my staged changes — focus on race conditions.")
```

SDK は SKILL.md prompt をロードし、agent が[skill が行うこと](#what-the-skill-does)で述べられているワークフローを実行します——`npm install` のフォールバックや、LLM 設定が無いときに認証情報の入力を促す手順を含みます。

## その他の agent フレームワーク

「外部 skill を登録する」インターフェースを持つフレームワークであれば、SKILL.md を取り込めます——それは frontmatter 付きの markdown に過ぎません。フレームワークが異なるスキーマを期待する場合でも、markdown 本文は prompt テンプレートとして利用できます。

## 関連項目

- [Command（Claude Code Plugin）](../claude-code/)——同じ skill の slash-command 版。
- [Direct Subprocess](../subprocess/)——マニフェストを介さず、自分で CLI を呼び出す方法。
