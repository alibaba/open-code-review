---
title: GitHub Copilot CLI
sidebar:
  order: 3
---

OCR を、他のコーディングエージェントと同じポータブルな [Agent Skill](../agent-skill/) を通じて
[GitHub Copilot CLI](https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli)
内でエンドツーエンドに実行します——diff をレビューし、発見を分類し、採用すべきものを修正します。この統合はコピーするだけで使える skill です——フックをインストールせず、必須の指示を追加せず、リポジトリの動作を変更しません。

## リポジトリに含まれるもの

ワークフローは agent に依存しない skill
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
にあります。Copilot CLI のプラグインマニフェスト
[`plugins/open-code-review/plugin.json`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/plugin.json)
（Agent Plugins 1.0）が、同梱の skill を `copilot plugin install` 用にまとめます。
Copilot CLI は標準的な場所からも skill を読み込むため、skill ディレクトリをコピーする方法も引き続き使えます。

## インストール

プラグインをインストールします（推奨）：

```bash
copilot plugin install alibaba/open-code-review:plugins/open-code-review
```

`open-code-review` と `open-code-review-delegate` の skill がインストールされます。削除するには `copilot plugin uninstall open-code-review` を実行します。

Copilot CLI のインストーラーは現在、直接のプラグインインストール（repo/path/URL）が非推奨であり、marketplace 経由のインストールが推奨される、と警告しています。将来 marketplace のエントリにより `plugin@open-code-review` が推奨形式になる可能性があります。

### 手動の代替：skill をコピーする

skill を Copilot CLI が検出するディレクトリにコピーします。**プロジェクトレベル**
（リポジトリにコミットし、チームで共有）：

```bash
mkdir -p .github/skills/open-code-review
curl -o .github/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

**ユーザーレベル**（マシン上のすべてのプロジェクトで利用可能）：

```bash
mkdir -p ~/.copilot/skills/open-code-review
curl -o ~/.copilot/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

Copilot CLI はプロジェクト内の `.agents/skills/` と `.claude/skills/` も検出します。同じ名前のプロジェクト skill は個人用のものより優先されます。セッション内で `/skills list` を実行して（セッション外では
`copilot skill list`）、何がロードされているかを確認してください。手動でコピーした skill は `/skills reload` または新しいセッションの後に反映されます。インストールしたプラグインは、再インストール（`copilot plugin install` をもう一度実行）または `copilot plugin update` で更新します。

## 使い方

skill を `/open-code-review` で明示的に指定するか、タスクを記述するだけです——Copilot がその説明から skill を選択します：

```
/open-code-review
/open-code-review review this PR against main
/open-code-review focus on race conditions in commit abc123
```

| あなたの発言 | skill が実行する内容 |
| --- | --- |
| 「変更をレビューして」 | `ocr review --audience agent`（staged、unstaged、untracked） |
| 「この PR をレビューして」 | `ocr review --from main --to <branch>` |
| 「commit abc123 をレビューして」 | `ocr review --commit abc123` |

skill は独自に設定されたモデルで `ocr` を駆動し、重要度別にグループ化された発見を報告します。`ocr` 用に別の LLM を設定したくない場合は、[デリゲーションモード](../delegate/)をお試しください——ホスト agent（Copilot）がモデルを提供し、`ocr` はファイルの選択とルールの解決のみを行います。

### 任意：レビューを習慣にする

レビューのステップをオンデマンドではなく自動にするには、`.github/copilot-instructions.md` または `AGENTS.md` にルールを 1 つ追加します——どちらか 1 つのファイルを選んでください。Copilot は指示ファイルを優先順位なしで結合するためです：

```markdown
Before reporting a coding task complete, run `ocr review --audience agent`.
Fix all `critical` and `high` findings first.
```

このようなルールをインストールするかどうかは各プロジェクトの判断です。このリポジトリでは強制していません。

## ヘッドレス実行

```bash
copilot -p "Review the current branch with /open-code-review and report the findings." \
  --allow-tool='shell(ocr:*)'
```

- `--allow-tool='shell(ocr:*)'` が事前承認するのは `ocr` のシェルコマンドのみです——ファイル編集を許可するものではありません。非対話（`-p`）セッションでは、承認が必要な操作は一時停止ではなく拒否されます。対話セッションでは代わりに承認のために一時停止します。Copilot に修正を適用させるには、対話的に実行してください（または許可リストを意図的に編集ツールまで広げてください）。
- 信頼されていないフォルダでは、非対話セッションでプロジェクトレベルの skill（`.github/skills/`）がスキップされることがあります。Copilot CLI v1.0.107 以降には、単一セッションでそのフォルダを信頼する `--trust-folder` があります。
- Copilot のサンドボックスが有効な場合、`ocr` は LLM エンドポイントへのネットワークアクセスと、`~/.opencodereview/` への読み取りおよび書き込みアクセスを必要とします（セッション状態を保存するため）。
- `--output-format json` が出力するのは Copilot の JSONL セッションイベントです。レビュー結果そのものは `ocr` の JSON で、`ocr review --format json --audience agent` を明示的に実行して取得します。

決定的な CI ゲートが必要な場合は、agent を介さずに `ocr` を直接呼び出してください——
[CI/CD](../cicd/)を参照してください。

## 注意事項

- **フックは意図的にありません。** Copilot CLI は `preToolUse` フックで `git commit` や
  `git push` をゲートできますが、OCR はフックを提供していません。レビューは LLM を実行し（遅く、コントリビューターが持っていない可能性のある認証情報を必要とします）、レビュー対象のワークスペースは作成中の commit ではなく、フェイルオープンするフックは保証できない強制を宣伝することになるためです。こうしたトレードオフを受け入れる場合にのみ、独自のフックを追加してください。
- **プロンプトファイルは IDE 専用です。** Copilot CLI は
  `.github/prompts/*.prompt.md` を読みません。対応しているのは skill です。
- **発見は終了コードを変更しません。** `ocr` は問題を発見した場合でも、レビューが完了すれば 0 で終了します——`--format json` の重要度でゲートし、終了ステータスでは決してゲートしないでください。

## トラブルシューティング

- **skill が一覧に表示されない** — パス（`~/.copilot/skills/` または
  `<repo>/.github/skills/`）を確認してから、`/skills reload` を実行するか新しい
  セッションを開始してください。
- **`ocr: command not found`** — `npm i -g @alibaba-group/open-code-review` で
  インストールします。
- **レビューが LLM エラーで失敗する** — プロバイダーを設定し、`ocr llm test` で
  テストします。[設定](../configuration/)を参照してください。
- **Copilot が `ocr` の承認を繰り返し求める** — `--allow-tool` で
  `shell(ocr:*)` を許可するか、プロンプトで永続的に承認してください。
