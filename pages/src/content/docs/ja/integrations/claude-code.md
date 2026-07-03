---
title: Command（Claude Code Plugin）
sidebar:
  order: 2
---

パッケージ化されたコマンドをインストールすることで、OCR を [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
内でエンドツーエンドに実行できます——diff をレビューし、発見を分類し、採用すべき修正を自動的に適用します。

## リポジトリに含まれるもの

リポジトリには
[`plugins/open-code-review/`](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review)
配下に Claude Code plugin が用意されています。コマンドの prompt 本体は
[`plugins/open-code-review/commands/review.md`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/commands/review.md)
にあり、以下で述べるワークフローの正式な拠り所です。

## インストール

### 方法 1：plugin marketplace（推奨）

**Claude Code 内で**次の 2 つのコマンドを実行します。

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

これにより `/open-code-review:review` slash コマンドが登録され、`/plugin` を通じて更新可能な状態が保たれます。

### 方法 2：コマンドファイルを直接コピー

plugin marketplace をスキップしたい場合は、コマンドファイルを直接 `.claude/commands/` に配置します。これは `/open-code-review`（`:review` サフィックス無し）として登録されます。

**プロジェクトレベル**（リポジトリにコミットし、チームで共有）：

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

**ユーザーレベル**（マシン上のすべてのプロジェクトで利用可能）：

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

### コマンドをサポートするその他の agent

コマンドファイルは単一の frontmatter フィールドを持つ純粋な markdown です——Claude Code 固有の内容は一切含まれていません。あなたの agent が同様の **command** 規約（ディレクトリから呼び出し可能なコマンドとしてロードされる markdown prompt）をサポートしている場合、上記のファイルコピー方法がインストール経路になります。`open-code-review.md` を agent がコマンドを読み込むディレクトリに配置し、agent のコマンド呼び出し方法に従って呼び出してください。prompt 本文は agent に依存しません——モデルに対して、どの `ocr` 引数を選び、出力をどのように分類するかを伝えるだけです。

> **前提条件：** 初回実行時、バイナリが `PATH` 上に存在しない場合、コマンドは
> （`npm install -g @alibaba-group/open-code-review` を通じて）`ocr` CLI を自動的にインストールします。ただし、LLM は事前に設定しておく**必要があります**——`ocr llm test` が接続できない場合、コマンドは失敗します。[設定](../../configuration/)を参照してください。

## 使い方

Claude Code でコマンドを名前で呼び出します。plugin marketplace 経由でインストールした場合は `/open-code-review:review` を、ファイルを直接コピーした場合は `/open-code-review` を使います。

```
/open-code-review:review
/open-code-review:review review this PR against main
/open-code-review:review focus on race conditions in commit abc123
```

prompt はあなたのリクエストを解析し、正しい `ocr review` 引数を選択します。引数なし → 作業領域モード（staged + unstaged + untracked）、commit の言及 → `--commit`、ブランチ区間の言及 →
`--from` / `--to`。OCR の引数を直接透過的に渡すこともできます
（例：`/open-code-review:review --commit abc123` や `--from main --to feature`）。

## コマンドが行うこと

コマンドの prompt はとても短く、3 ステップです。

1. **レビューの実行。** あなたのリクエストから推論した引数を用いて `ocr review --audience agent`
   を呼び出します（要件コンテキストが記述されている場合はオプションの `--background` を追加）。`ocr` バイナリが `PATH` 上に無い場合、コマンドは `npm i -g @alibaba-group/open-code-review` を通じて自動インストールして続行します。出力は 5 分のタイムアウト内で取得されます。
2. **フィルタリングと評価。** 各コメントを **High** / **Medium** / **Low** に分類します。低信頼度（誤検知の疑い、些細な指摘、コンテキスト不足）のコメントは黙って破棄され、その他は表示されます。
3. **修正。** 採用すべき High/Medium 項目に対して修正を自動的に適用します。
   [Agent Skill](../agent-skill/) と異なり、このコマンドは**デフォルトで自動修正します**——「レビューして片付ける」ワークフローには適した選択であり、「diff を見せてほしい」ワークフローには向きません。

コマンドがコードを変更する前に尋ねるようにしたい場合や、分類基準を厳しくしたい場合は、ローカルの prompt コピーを編集してください。Claude Code は呼び出しのたびにコマンドを読み直すため、再起動は不要です。

## 関連項目

- [Agent Skill](../agent-skill/)——SDK レベルの同等物。同じ底層の CLI で、デフォルト値が異なります（修正前に尋ねる）。
- [Direct Subprocess](../subprocess/)——slash コマンドを介さず、自分で CLI を呼び出す方法。
