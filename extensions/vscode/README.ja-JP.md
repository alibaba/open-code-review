<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | 日本語 | <a href="README.ko-KR.md">한국어</a> | <a href="README.ru-RU.md">Русский</a>
</p>

# Open Code Review（VS Code 拡張機能）

[`open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review)（`ocr`）CLI を基盤にした VS Code のコードレビュー拡張機能です。Preact WebView でプロトタイプの操作感を再現し、AI コードレビューをエディターに取り込みます。サイドバーからレビューを開始し、ログをリアルタイムに表示し、各コメントをエディター内でそのまま適用／無視／誤検出として扱えます。サイドバーとは双方向に同期します。

---

## 機能

- **3 つのレビューモード**：ワークスペースの変更、ブランチ比較（`--from` / `--to`）、単一コミット（`--commit`）。
- **レビュー対象ファイルのプレビュー**：現在の Git 状態から変更ファイルを一覧表示し、クリックするとネイティブの diff ビューで差分を確認できます。
- **カスタムレビュープロンプト**：今回のレビューに `--background` のヒントを任意で追加できます。
- **ストリーミングログ**：レビュー中は CLI の出力をリアルタイムに表示し、いつでもキャンセルできます。
- **結果表示と双方向同期**：完了するとサイドバーにコメントカードが並び、同時にエディター内へ CommentThread が描画されます。適用／無視／誤検出の操作は両側で同期します。
- **空・キャンセル・失敗の各状態**：問題なし、ユーザーによるキャンセル、CLI の失敗それぞれに専用ビューを用意しています（失敗は再試行でき、CLI が返した実際のエラーを表示します）。
- **設定の管理**：LLM プロバイダーの設定を拡張機能内で表示・編集できます（保存は `ocr config set` 経由）。
- **モデル切り替えと接続テスト**：ステータスバーからモデルを切り替え、LLM への接続をテストできます。

---

## 前提条件

1. `ocr` CLI をグローバルにインストールします：

   ```bash
   npm i -g @alibaba-group/open-code-review
   ```

2. 利用する LLM（エンドポイント、API キー、モデル）を設定します。CLI で直接設定するか、拡張機能の設定ビューで入力します：

   ```bash
   ocr config set llm.url https://api.anthropic.com/v1/messages
   ocr config set llm.auth_token sk-...
   ocr config set llm.model claude-opus-4-6
   ocr config set llm.use_anthropic true
   ```

   設定は `~/.opencodereview/config.json` に書き込まれます。

---

## 開発

### 環境

- Node.js 18 以上、パッケージマネージャーは **Yarn**（リポジトリに `yarn.lock` を同梱）。
- VS Code 1.74 以上。
- グローバルに利用できる `ocr` CLI（上記「前提条件」を参照）。この拡張機能は実質的に `ocr` の GUI フロントエンドです。

### 開発環境の起動

```bash
cd extensions/vscode
yarn install      # 依存関係をインストール
yarn watch        # ウォッチモードの開発ビルド（変更のたびに out/ を再ビルドするため推奨）
```

次に VS Code で `extensions/vscode` フォルダーを開き、**F5** を押して Extension Development Host を起動します（デバッグ構成は `.vscode/launch.json` に用意されています）。新しいウィンドウで Git の変更があるプロジェクトを開くと、アクティビティバーに Open Code Review のアイコンが表示され、レビューを開始できます。

> コードを編集した後：WebView の変更は開発ホストのウィンドウで **サイドバーを開き直す**必要があります（`Developer: Reload Webviews` の実行でも可）。Extension Host の変更は **デバッグセッションを再起動する**必要があります（デバッグツールバーの ⟳ ボタン、またはホストのウィンドウで `Cmd+R`）。

### スクリプト

```bash
yarn compile      # 一度だけの開発ビルド（webpack development）
yarn watch        # ウォッチモードの開発ビルド
yarn build        # 本番ビルド（webpack production。パッケージング前に自動実行）
yarn test         # Jest のユニットテストを実行
yarn lint         # ESLint
yarn package      # 配布用 .vsix を生成（「リリースパッケージの作成」を参照）
```

### デバッグのポイント

- **双方向メッセージング**：WebView と Extension Host は `postMessage` で通信し、メッセージ型は `extensions/frontend/src/shared/messages.ts` にあります。両側とも `dispatch` / `handle` を通るため、デバッグはそこから始めるとよいでしょう。
- **CLI の呼び出し**：`ocr` のサブコマンドはすべて `src/extension/services/CliService.ts` の `child_process.spawn` で実行します。`runRaw` は CLI の終了コードが 0 以外の場合に reject し、stderr の `Error:` テキストを含めるため、「レビュー失敗／接続失敗」の切り分けに役立ちます。
- **設定の読み書き**：`ConfigService` は `~/.opencodereview/config.json` を読み、書き込みは `ocr config set` に委譲します。WebView 側のフィールドは camelCase（例：`useAnthropic`）、ディスク／CLI 側は snake_case（例：`use_anthropic`）で、変換は `src/extension/services/configParse.ts` にあります。

---

## ビルド

### 成果物のみをコンパイル

```bash
yarn build        # 本番ビルド（webpack production）
```

成果物：`out/extension.js`（Extension Host）+ `out/webview.js`（WebView SPA）。

### リリースパッケージ（.vsix）の作成

```bash
yarn package      # = vsce package --no-yarn
```

このコマンドは次の処理を行います：

1. `vscode:prepublish` を起動し、`yarn build` の本番ビルドを実行します；
2. `.vscodeignore` に従ってソース、テスト、開発用ファイルを除外します；
3. カレントディレクトリに `open-code-review-vscode-<version>.vsix` を生成します。

> パッケージングツールは `@vscode/vsce` で、devDependency としてインストールされているためグローバルインストールやネットワークからのダウンロードは不要です。`--no-yarn` は vsce 既定の npm 依存ツリー検査をスキップします（本プロジェクトは Yarn を使用）。

リリースパッケージには実行に必要なものだけが含まれます：`package.json`、`README.md`、`resources/icon.svg`、`out/extension.js`、`out/webview.js`。

### ローカルでのインストール／確認

```bash
code --install-extension open-code-review-vscode-<version>.vsix
```

または VS Code で：拡張機能パネル → 右上の `⋯` → **Install from VSIX…** → 生成した `.vsix` ファイルを選択します。

> Marketplace へ公開する場合は `vsce publish` を使用します（publisher アカウントと PAT が必要）。日常的な配布には上記の `.vsix` で十分です。

---

## アーキテクチャ

**Monolithic WebView + Thin Extension Host** という設計です：

- **WebView** は別ビルドの Preact SPA で、共有フロントエンド `extensions/frontend/` からビルドされます（IntelliJ IDEA プラグインとも共用）。
- **Extension Host** 層は薄く、CLI 呼び出し、ファイルシステム、Git 操作、エディターコメントのみを担当します。
- 両者は `postMessage` で通信し、`extensions/frontend/src/shared/` の共有 TypeScript 型で型安全を保ちます。

```
src/
└── extension/          Extension Host（Node.js）：services / providers / commands
```

WebView と共有型は、共有フロントエンド `extensions/frontend/` からビルドされます（IntelliJ IDEA プラグインとも共用）。

---

## ライセンス

Apache-2.0
