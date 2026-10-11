---
title: デリゲーションモード
sidebar:
  order: 5
---

OCR が確定的エンジニアリング（ファイル選択、ルール解決）を担当し、ホストエージェントが自身の LLM 能力を使って実際のコードレビューを行います。OCR 側に LLM エンドポイントは不要です。

## デリゲーションモードを使うタイミング

デリゲーションモードは、サブスクリプション型の AI コーディングエージェント向けに設計されています — Claude Code、Codex、Cursor、Open Code、Qoder など。これらのツールには LLM サブスクリプションが組み込まれており、デリゲーションモードを使えばホストエージェントの既存サブスクリプション枠をそのままコードレビューに活用できます。追加のモデル設定や API キーは不要です。

以下の場合に使用してください：

1. AI コーディングエージェントがサブスクリプション制で、その枠をコードレビューに再利用したい場合 — 追加の API キーやモデル設定は不要。
2. OCR のエンジニアリング機能のみ（ファイルフィルタリング、ルール解決、除外ロジック）を利用し、LLM 推論はホストエージェントに任せたい場合。
3. 構造化された入力（ファイルリスト＋ルール）を独自のレビューステップに必要とするカスタムエージェントパイプラインを構築している場合。

## 前提条件

`ocr` CLI がインストールされている必要があります：

```bash
which ocr || npm install -g @alibaba-group/open-code-review
```

LLM 設定（`ocr config set …` や環境変数）は不要です — デリゲーションモードは OCR 側で LLM を呼び出しません。

## Skill / Command のインストール

### Claude Code — Command

```bash
mkdir -p .claude/commands
curl -o .claude/commands/delegate-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

### 任意のエージェント — Skill

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

または手動コピー：

```bash
cp -R /path/to/open-code-review/skills/open-code-review-delegate ~/.claude/skills/
```

## ワークフロー

### ステップ 1：Preview — レビュー対象の決定

```bash
ocr delegate preview [--from <ref> --to <ref>] [--commit <hash>] [--exclude <patterns>]
```

出力内容：

- **mode** — workspace / range / commit
- **ref メタデータ** — from、to、commit、merge\_base
- **レビュー可能ファイルリスト** — パス、ステータス、挿入/削除行数
- **除外ファイル** — 除外理由付き

よく使うパターン：

| シナリオ | コマンド |
|----------|---------|
| ワークスペースの変更 | `ocr delegate preview` |
| ブランチ比較 | `ocr delegate preview --from main --to feature` |
| 単一コミット | `ocr delegate preview -c abc123` |

### ステップ 2：ファイルのルール取得

```bash
ocr delegate rule <path1> <path2> ...
```

ステップ 1 のレビュー可能パスを渡します。出力はルール内容でグループ化されます — 同じルールを共有するファイルは1つのグループにまとめられ、重複を避けます。

### ステップ 3：diff の取得

ステップ 1 のモードと参照情報に応じて diff コマンドを選びます。レビュー中のすべての Git 呼び出しで `git --no-pager` を使います。

以下の diff コマンドは外部 diff プログラム、テキスト変換、カラー出力を無効にして、プレーンテキストのパッチを取得します。

**Range モード**（merge\_base あり）：
```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color <merge_base>..<to> -- "<path>"
```

**Commit モード**：
```bash
git --no-pager show --no-ext-diff --no-textconv --no-color <commit> -- "<path>"
```

**Workspace モード**：
```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color HEAD -- "<path>"        # 追跡ファイル
cat "<path>"                     # 新規未追跡ファイル
```

コンテキストを取得するには：

```bash
git --no-pager log --no-color --oneline -- "<path>"
git --no-pager blame --no-textconv -- "<path>"
git --no-pager show --no-ext-diff --no-textconv --no-color "<ref>:<path>"
```

大きな diff では、選択したモードのコマンドに `--output="<absolute-diff-file>"` を追加します。リポジトリ外の一意な絶対パスを選び、親ディレクトリを作成します。ファイル読み取りツールでも同じパスを使います：

```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color --output="<absolute-diff-file>" <merge_base>..<to> -- "<path>"
```

Git が終了コード 0 で終了したら、ファイル全体を分割して読み取り、レビューを終えてから削除します。コマンドが失敗またはタイムアウトした場合は、再試行するか、エラーを記録して `skipped` にします。出力が予期せず空の場合は、preview を再実行して照合します。

### ステップ 4：各ファイルのレビュー

レビュー可能な各ファイルについて：

1. diff を取得（ステップ 3）
2. 対応するルールグループ（ステップ 2）をレビューチェックリストとして参照
3. コンテキスト探索を必要に応じて行い、徹底的にレビュー

preview の各 `(path, status)` を `reviewed` または `skipped` として記録し、スキップした項目には理由を付けます。レポートには `total_files`、`reviewed_files`、`skipped_files`、`coverage_rate` を含めます。

### ステップ 5：レポート

重要度で分類：

- **Critical/High** — バグ、セキュリティ問題、データ損失リスク。常に報告。
- **Medium** — パフォーマンスの懸念、エラーハンドリングの欠落。コンテキスト付きで報告。
- **Low** — スタイルの提案、軽微な改善。明確に価値がない限り静かに破棄。

## サブコマンドリファレンス

| コマンド | 目的 |
|----------|------|
| `ocr delegate preview` | レビュー可能ファイル＋mode/ref メタデータの一覧 |
| `ocr delegate rule <path...>` | 内容別にグループ化されたレビュールールの解決 |

## 共通フラグ

| フラグ | 説明 |
|--------|------|
| `--from <ref>` | Range モードのソース参照 |
| `--to <ref>` | Range モードのターゲット参照 |
| `-c, --commit <hash>` | 単一コミットモード |
| `--repo <path>` | リポジトリルート（デフォルト：cwd） |
| `--rule <path>` | カスタム rule.json パス |
| `--exclude <patterns>` | カンマ区切りの除外パターン |
| `-b, --background <text>` | ビジネスコンテキスト |
| `-B, --background-file <path>` | Markdown ファイルからビジネスコンテキストを読み込み（`-b` より優先） |

## 他の統合モードとの比較

| モード | LLM を呼ぶのは？ | ユースケース |
|--------|-----------------|-------------|
| [Agent Skill](../agent-skill/) | OCR | Agent が `ocr review` を呼び出し、OCR が完全なレビューを駆動 |
| [Command（Claude Code）](../claude-code/) | OCR | Claude Code のスラッシュコマンド、OCR がレビューを駆動 |
| **デリゲーションモード** | ホストエージェント | OCR がスキャフォールディングを提供、Agent がレビューを駆動 |

## 関連項目

- [Agent Skill](../agent-skill/) — OCR がエージェントに代わって完全なレビューを駆動。
- [Command（Claude Code）](../claude-code/) — スラッシュコマンド形式、自動修正付き。
