---
title: Agent Skill
sidebar:
  order: 1
---

OCR を呼び出し可能な skill として登録することで、agent フレームワークが正しい引数と指摘の検証手順を用いて OCR を呼び出せるようになります。呼び出し側でこれらを改めて導き出す必要はありません。

## リポジトリに含まれるもの

リポジトリには
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
に SKILL マニフェストが用意されています。OCR の呼び出し、指摘の検証、不確実性の報告、依頼に応じた修正の手順を定義しています。skill ディレクトリには GitHub PR レビュー用ヘルパーも含まれるため、SKILL.md だけでなくディレクトリ全体をインストールしてください。

## インストール

### 方法 1：`npx skills add`（推奨）

skill を使いたいプロジェクト内で実行します。

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

これは
[skills registry](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
から skill ディレクトリを取得してプロジェクトに配置し、skills の規約を尊重するコーディング agent が次回の呼び出し時にそれをロードするようにします。skill を最新版に更新するには、このコマンドを再実行してください。

> **前提条件：** ネイティブモードでは OCR のモデルプロバイダーをローカルで設定する必要があります。認証情報を会話に貼り付けないでください。`ocr` がない場合のトラブルシューティングには、`npm install -g @alibaba-group/open-code-review` によるインストール手順があります。GitHub PR モードのみ、Python 3 と認証済みの `gh` も必要です。[設定](../../configuration/)を参照してください。

### 方法 2：手動コピー（システムレベル）

プロジェクトごとではなくグローバルに skill をインストールしたい場合は、フォルダを skills ディレクトリにコピーします。

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

これにより、マシン上のすべてのプロジェクトで skill が利用可能になります。

## skill が行うこと

SKILL.md は一つの prompt です。呼び出し側の agent がそれをロードすると、agent 自身が手順を実行します。完全な `/open-code-review`（または同等）のリクエストは、次のように展開されます。

1. **業務コンテキストの抽出。** レビュー対象（commit、ブランチ、作業コピー）を確認し、短い `--background` 文字列を生成します。
2. **依頼されたレビューの実行。** ローカルのレビューでは `ocr review --audience agent --background "…"` を直接実行し、コミット済みの変更には `--commit` または `--from`/`--to` を追加します。通常はインストールや接続の事前確認を行いません。新しい commit・区間レビューには既定で `--analysis` を追加し、別の形式を指定されなければ JSON を使います。作業コピー、プレビュー、再開、SARIF、分析を省略する明示的な依頼では分析を追加しません。失敗したら原因を調べてから再試行します。
   GitHub PR の番号または URL には、インストール済みの `python3 "$SKILL_DIR/scripts/review_pr.py" <id-or-url>` を使います。番号はチェックアウトの `gh` 既定リポジトリを選び、fork では上流を指す場合があります。PR URL、レビューしたリビジョン、スナップショットの鮮度、成果物のパスを報告します。
3. **検証と報告。** 低重要度も含め、すべての指摘をレビュー対象のコードと照合します。**Reproduced（再現済み）** と **Uncertain（未確定）** に分け、OCR の重要度と各グループ内の順序を保ちます。具体的な反証がある場合のみ指摘を省略し、不確実という理由だけでは破棄しません。元の結果を保持し、Review のカバレッジと未確認範囲も報告します。併用時は Analysis Markdown を別に表示し、実行状態とモデルによる完全性の評価を区別します。
4. **依頼に応じた修正。** 編集を依頼された場合のみ、安全かつ検証済みの修正を行い、critical、high、medium の指摘を優先します。レビューのみの依頼はコード変更を許可しません。PR モードのスナップショットは、ユーザーが明示的に削除に同意するまで保持します。削除後もレポートは残ります。

完全な prompt——検証要件、報告方法、PR モードの制限、注意事項を含む——は
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

SDK は SKILL.md prompt をロードし、agent が[skill が行うこと](#what-the-skill-does)のワークフローを実行します。呼び出しの失敗を調査し、認証情報が必要ならユーザーにローカルでの設定を依頼します。

## その他の agent フレームワーク

「外部 skill を登録する」インターフェースを持つフレームワークであれば、SKILL.md を取り込めます——それは frontmatter 付きの markdown に過ぎません。フレームワークが異なるスキーマを期待する場合でも、markdown 本文は prompt テンプレートとして利用できます。

## 関連項目

- [Command（Claude Code Plugin）](../claude-code/)——同じ skill の slash-command 版。
