# ネイティブ OCR プラグイン

[English](../native-plugins.md) | [简体中文](native-plugins.zh-CN.md) | [日本語](native-plugins.ja-JP.md) | [한국어](native-plugins.ko-KR.md) | [Русский](native-plugins.ru-RU.md)

OCR は、インストーラー、レジストリ、およびランタイムを通じて、レビュー規則（Review Rules）および MCP ツールを提供するプラグインをホストできます。これらのパッケージは `ocr-plugin.json` をマニフェストとして使用し、OCR を他のコーディングエージェントに統合する設定とは独立しています。

モジュール境界、ライフサイクル図、推奨レビュー手順については、[ネイティブプラグインのアーキテクチャ設計](../plugin-architecture.md) を参照してください。

## サポートされているインストールソースと URL 入力

OCR プラグインの利用時によくある疑問：**どのようなソースがサポートされていますか？入力に URL は使用できますか？**

`ocr plugin install <source>` は公開 GitHub URL、HTTPS アーカイブ URL、ローカルディレクトリとアーカイブを受け付けます。`ocr plugin add <source>` は同じ形式を扱いますが、プロジェクトのソースには追加の制約があります。

- ローカルソースは対象リポジトリ内に解決され、正規化された `./` パスとして記録されます。リポジトリ内の絶対パスは使用できますが、外部のディレクトリやアーカイブは拒否されます。
- プロジェクトの HTTPS ソースはクエリパラメーターのない固定 URL に限られ、末尾の空の `?` も使用できません。グローバルの `install` はクエリを受け付けますが、管理出力には表示しません。
- `add` の `--repo` は対象プロジェクトを指定します。シェルの現在のディレクトリが異なる場合は、ソースに絶対パスを使用してください。

### サポートされているソースの種類

OCR は主に 4 種類のパッケージソースをサポートしています：

1. **パブリック GitHub リポジトリ（URL）**
   - **リポジトリルート**: `https://github.com/OWNER/REPO`
     リポジトリのルートにあるプラグインをインストールします。
   - **ブランチまたはタグ（サブディレクトリ指定可能）**: `https://github.com/OWNER/REPO/tree/REF/PATH`
     例: `https://github.com/example/monorepo/tree/v1.2.0/plugins/security-rules`
   - **コミット SHA（不変リビジョン指定）**: `https://github.com/OWNER/REPO/tree/FULL_40_CHAR_COMMIT_SHA/PATH`
     確定した 40 文字のコミット SHA を直接指定します。
   - *動作の仕組み*: ブランチ名やタグが指定された場合、OCR は GitHub API 経由で確定した 40 文字のコミット SHA に解決します（URL に既に 40 文字の SHA が含まれている場合は API 解決をスキップします）。その後、`codeload.github.com` から該当コミットの tar.gz アーカイブを隔離された一時ディレクトリにストリーミングダウンロードし、対象のプラグインディレクトリを展開します。

2. **リモート HTTPS アーカイブ（URL）**
   - **HTTPS tar.gz アーカイブ**: `https://example.com/team-review.tar.gz`
   - **HTTPS ZIP アーカイブ**: `https://example.com/team-review.zip`
   - **リリースアセット**: `https://github.com/OWNER/REPO/releases/download/v1.0.0/plugin.tar.gz`
   - *動作の仕組み*: HTTPS 経由でダウンロードし、マジックバイト（gzip は `0x1f 0x8b`、ZIP は `PK`）を検査して形式を自動判別します（URL に拡張子が含まれていなくても問題ありません）。サイズ制限内で隔離サンドボックスに展開し検証します。

3. **ローカルディレクトリパス（Path）**
   - ルートに `ocr-plugin.json` を含むローカルの相対または絶対パス：
     ```text
     ocr plugin install ./examples/ocr-plugins/review-checklist
     ocr plugin install /opt/company-plugins/code-style
     ```
   - OCR はマニフェストと内容を検証し、ローカルプラグインストアにスナップショットを複製します。

4. **ローカルアーカイブファイル（Path）**
   - ローカルの `.tar.gz` または `.zip` アーカイブ：
     ```text
     ocr plugin install ./team-review.tar.gz
     ocr plugin install ./team-review.zip
     ```
   - 隔離ディレクトリに展開・検証し、スナップショットを保存します。

### サポートされていない・禁止されているソース

決定論的な実行、再現可能なプロジェクトロック、およびシステムの安全性を維持するため、以下は**サポートされていません**：

- **平文 HTTP URL (`http://`)**: セキュリティ保護のため HTTPS が必須です。安全でない HTTP 接続や HTTP へのダウングレードリダイレクトは拒否されます。
- **認証情報やフラグメントが含まれる URL**: userinfo（例: `https://user:password@...`）や URL フラグメント（`#...`）を含む URL は拒否されます。
- **従来のマーケットプレイスカタログ / JSON インデックス**: カタログ検索機能（`ID@market` 構文）は廃止され、直接パッケージや URL の指定方式に統合されました。
- **生の Git SSH クローン URL (`git@github.com:...`)**: SSH 経由の直接 Git クローンはサポートされていません。パブリック HTTPS URL またはアーカイブを使用してください。
- **シンボリックリンク、ハードリンク、デバイスファイル**: ディレクトリトラバーサルや権限昇格を防ぐため、アーカイブ内のシンボリックリンクや特殊ファイルは展開時に厳格に拒否されます。
- **`@` を含むローカルファイル名**: 従来のマーケット ID との混同を防ぐため、裸の `@` を含む入力は拒否されます。ファイル名に `@` が含まれる場合は `./plugin@1.0.tar.gz` のように明示的なパスを指定してください。

### アーカイブおよびリソースの制限

展開爆弾やリソース枯渇を防ぐための上限：
- **マニフェスト最大サイズ**: 1 MiB (`ocr-plugin.json`)
- **アーカイブダウンロード最大サイズ**: 32 MiB
- **展開後合計サイズ上限**: 64 MiB
- **tar.gz 展開ストリーム上限**: 72 MiB（ヘッダー・メタデータを含む）
- **最大ファイル数**: 1 パッケージあたり最大 4,096 ファイル
- **プロジェクト設定ファイル上限**: 4 MiB (`plugins.json` および `plugins.lock.json`)

---

## 完全な CLI サブコマンドリファレンス

OCR は `ocr plugin` の下に、**グローバルプラグイン管理** と **プロジェクト単位のプラグイン管理** のコマンドを提供しており、ツールの確認には `ocr mcp list` を使用します。

```text
ocr plugin <command> [flags]
```

### グローバルプラグイン管理コマンド

グローバルプラグインは `~/.opencodereview/plugins` に配置されます。有効化されると、プロジェクト固有の設定がない限り、該当ユーザーが実行するすべてのレビューやスキャンに自動的に適用されます。

#### 1. `ocr plugin install <url|path>`
URL またはローカルパスからプラグインをインストール・登録し、スナップショットを作成します。

- **引数**: `<url|path>`（GitHub URL、HTTPS アーカイブ URL、ローカルディレクトリまたはアーカイブ）。
- **フラグ**:
  - `--enable`: インストール直後にプラグインを有効化します。
  - `--dry-run`: インストールや有効化を行わずにパッケージ内容をダウンロード・検査しプレビューを表示します。
  - `--plugin <id>`: 単一のソース内に複数のプラグインが存在する場合に ID を指定します。
  - `--json`: 構造化 JSON で出力します。
- **安全規則**: インストールはファイルのコピーとマニフェストの検証のみを行い、**プラグインコードの実行、インストールスクリプトの実行、パッケージマネージャーによる依存関係のビルドなどは一切行いません**。

#### 2. `ocr plugin list`
グローバルに登録されたプラグインを `ID`、`VERSION`、`STATE`、`CAPABILITIES` 列で表示します。ソースは `ocr plugin show <id>` または `ocr plugin list --json` で確認できます。`ocr plugins list` も同じ動作です。`ocr plugins` 単独ではヘルプを表示します。

- **フラグ**: `--json`（JSON 配列で出力）

```text
ID                VERSION  STATE     CAPABILITIES
review-checklist  1.0.0    disabled  1 rule packs, 0 MCP servers
```

#### 3. `ocr plugin show <id>`
指定したプラグインの詳細情報（スナップショットディレクトリ、ルール一覧、提供される MCP サーバー、ホスト互換性要件など）を確認します。

- **引数**: `<id>`（プラグイン ID）
- **フラグ**: `--json`
- **情報の難読化・保護**: 認証情報の漏洩を防ぐため、MCP コマンドの引数、環境変数、認証ヘッダー、URL クエリパラメータは表示時に自動的に除外されます。

#### 4. `ocr plugin update <id>`
インストール時に記録された元のソースからプラグインを再取得して更新します（有効/無効状態は維持されます）。

- **引数**: `<id>`（プラグイン ID）
- **フラグ**:
  - `--source <url|path>`: 記録されたソースを変更します。`install` と同じ HTTPS/GitHub URL、ローカルディレクトリ、ZIP/tar.gz アーカイブに対応します。パッケージの ID はインストール済みプラグインと一致する必要があります。
  - `--accept-permissions`: 新規追加または変更された MCP 実行権限を明示的に承認します。
  - `--json`: 更新結果を JSON で出力します。
- **実行権限の変更検知**: 更新によって**新しい MCP サーバーが追加された場合**、または**既存サーバーのコマンド、引数、環境変数、トランスポートが変更された場合**、OCR は更新を中断してエラー（`PLUGIN_EXECUTION_CHANGED`）を返します。内容を確認後、`--accept-permissions` を付与して明示的に実行する必要があります：
  ```sh
  ocr plugin update <id> --accept-permissions
  ```

ダウンロードトークンが失効した場合やパッケージの配布先が変わった場合は、新しいソースを指定できます：

```sh
ocr plugin update my-plugin --source "https://plugins.example.com/download.php?token=NEW_TOKEN"
```

更新が成功すると、以降の `update <id>` はクエリパラメータを含む新しいソースを再利用し、有効/無効状態を維持します。ダウンロード、検証、権限の承認に失敗した場合、以前のパッケージとソースは変更されません。承認が必要な場合は、`--source` と `--accept-permissions` の両方を付けて再実行してください。

HTTPS ダウンロード URL はクエリパラメータを含めることができ、応答が有効なプラグインアーカイブなら拡張子は不要です。完全な URL はローカルに保存されます。管理画面の出力ではクエリパラメータが非表示になりますが、保存時に暗号化されるわけではありません。このコマンドはグローバルにインストールしたプラグインを更新します。プロジェクト依存関係には `plugin add` と `plugin lock` を使います。

#### 5. `ocr plugin enable <id>`
プラグインを有効化します。レビュー規則と MCP ツールがその後の `ocr review` や `ocr scan` に読み込まれます。

#### 6. `ocr plugin disable <id>`
プラグインを無効化します。パッケージは残りますが、実行時に読み込まれなくなります。

#### 7. `ocr plugin remove <id>`
プラグインの登録を解除して削除します。現在実行中のレビューセッションは既存のスナップショットを引き続き使用できます。

---

<a id="project-plugins"></a>

### プロジェクト単位のプラグイン管理コマンド

チーム開発や CI/CD 環境では、全員が完全に同一のプラグインバージョンを使用し、明示的に実行権限を管理できる再現性が不可欠です。プロジェクトプラグインはリポジトリルートの `.opencodereview/plugins.json`（宣言）と `.opencodereview/plugins.lock.json`（ロック）で管理されます。

リポジトリ内にプロジェクトプラグインが宣言されている場合、OCR は**プロジェクト専用モード**となり、グローバルプラグインを無視してロックされたプラグインのみを読み込みます。

#### 1. `ocr plugin add <source>`
プロジェクトにプラグイン依存関係を追加します。ソースを解決し、宣言ファイルおよび現在のプラットフォーム向けロックファイルを更新します。

- **フラグ**:
  - `--plugin <id>`: 複数プラグインを含むソースから特定の ID を選択。
  - `--repo <path>`: プロジェクトルートのパス。

#### 2. `ocr plugin lock`
宣言ファイルに基づいて、現在の OS/アーキテクチャ（`GOOS/GOARCH`）向けに依存関係を解決し、ロックファイルを生成または更新します。

#### 3. `ocr plugin sync`
ロックファイルで指定された厳密な成果物スナップショットをインストールします。

- **フラグ**:
  - `--frozen`: **必須**。宣言とロックファイルが完全に一致していることを要求し、新しいバージョンの解決を防ぎます。
  - `--offline`: オフラインモード。ローカルキャッシュまたはリポジトリ内ソースのみを使用します。
  - `--repo <path>`: プロジェクトルートのパス。
- **安全性**: スナップショットの配置のみを行い、**プラグインコードを実行したり実行権限を勝手に付与したりしません**。

#### 4. `ocr plugin trust`
このマシンで現在のプロジェクトロックを承認します。リモートエンドポイントを含むすべての MCP サーバーに承認が必要です。ルールのみのパッケージには実行承認は不要です。

- **フラグ**:
  - `--revoke`: 実行権限を取り消します。
  - `--repo <path>`: プロジェクトルートのパス。

承認記録はリポジトリ外の `~/.opencodereview/plugins/projects/<project-hash>/trust-<lock-id>` に保存され、正規化されたプロジェクトパス、ロック内容、ホストプラットフォームに結び付けられます。ロックが変わると再承認が必要です。`add`、`lock`、`sync` は承認を付与しません。取り消すには `ocr plugin trust --revoke` を使用します。

#### 5. `ocr plugin project`
宣言とロックの一致を検証し、プラットフォームエントリーを含むロック内容を表示します。インストール状態や承認状態は表示しません。検証には `ocr plugin preflight --json`、MCP の構成と承認状態の確認には `ocr mcp list --json` を使用してください。

#### 6. `ocr plugin preflight`
レビュー実行前または CI パイプライン内で事前検証を行います。ロックされた全パッケージがインストールされ、チェックサムが一致し、ホストバージョンが満たされ、実行信頼が付与されているかを確認します。

- **フラグ**:
  - `--fingerprint`: 検証済みのロックおよびプラットフォームの決定論的な指紋ハッシュを計算・出力します（差分レビューのキャッシュ判定用）。
  - `--repo <path>`: プロジェクトルートのパス。
  - `--json`: JSON 形式で出力。

---

### 統合 MCP インベントリ確認コマンド

#### `ocr mcp list`
ユーザー設定（`~/.opencodereview/config.json`）、グローバルプラグイン、プロジェクトプラグインから提供される全 MCP サーバーを横断的に一覧表示します。

```sh
ocr mcp list
ocr mcp list --repo /path/to/project --json
```

- **出力項目**: サーバー名（`plugin/PLUGIN_ID/SERVER` 形式）、提供元（`user_config`, `global_plugin`, `project_plugin`）、通信方式（`stdio`, `remote`）、状態、承認ステータス。
- **安全性**: 設定とマニフェストの読み取りのみを行い、**プロセスの起動、セットアップスクリプトの実行、ネットワーク接続の確立、LLM 認証情報の取得は一切行いません**。

| 状態 | 意味 |
| --- | --- |
| `configured` | 選択および承認済みであり、レビューで呼び出し可能 |
| `disabled` | グローバルプラグインが無効化されている |
| `incompatible` | 要求される OCR バージョンを満たしていない |
| `authorization_required` | インストール済みだが `ocr plugin trust` による実行承認が行われていない |
| `server_name_conflict` | 手動設定とプラグインサーバーの名前が衝突している |

---

## 典型的な運用ワークフロー

### グローバルプラグインの運用例

```sh
# 1. インストール前のドライラン
ocr plugin install https://github.com/example/team-rules --dry-run

# 2. インストールして有効化
ocr plugin install https://github.com/example/team-rules --enable

# 3. 提供機能の確認
ocr plugin show team-rules
ocr mcp list

# 4. レビューの実行（ルールとツールが自動適用されます）
ocr review

# 5. 更新時の権限確認
ocr plugin update team-rules --accept-permissions
```

### プロジェクトプラグイン & CI の運用例

```sh
# 1. 依存関係の追加とロックファイルのコミット
ocr plugin add https://github.com/example/security-pack
git add .opencodereview/
git commit -m "chore: セキュリティレビュープラグインを追加"

# 2. チームメンバーや CI 環境での完全同期
ocr plugin sync --frozen

# 3. パッケージの確認とローカル実行承認
ocr plugin project
ocr plugin trust

# 4. 事前検証
ocr plugin preflight

# 5. レビュー実行
ocr review
```

---

### GitHub Actions

既存の OCR Action ステップの `with` に次の入力を追加します。

```yaml
project_plugins: 'true'
allow_plugin_exec: 'false'
```

`plugin sync --frozen` と `preflight --fingerprint` を含む Action と CLI のバージョンを固定してください。古いリリースはこの手順に対応していません。Action はモデル認証情報を設定する前にロックされたパッケージをインストールします。リモート接続を含む MCP 宣言を確認した後にのみ `allow_plugin_exec: 'true'` を設定します。これはその実行の承認であり、ロック自体は実行を許可しません。

ルール、宣言、ロック、リポジトリ内のパッケージは信頼されたポリシーのチェックアウトに必要です。既定では PR の base commit を使い、コメントからの実行では PR API で取得します。`policy_sha` で別の信頼されたコミットの完全な SHA を指定できます。PR head は差分のみに使用し、プラグインポリシーには使用しません。

ロックには runner の OS/アーキテクチャが必要です。各対象プラットフォームで `ocr plugin lock` を実行し、結果をコミットしてください。宣言が同じなら他のプラットフォームの項目は保持されますが、宣言変更時は無効になります。`.gitattributes` の `eol=lf` などでローカルパッケージのバイト列をチェックアウト間で統一してください。

各実行は新しい専用の OCR ホームを使い、内容と承認を検証します。ネイティブプログラムは runner の権限で動き、モデル認証情報を含む環境変数にアクセスできます。共有キャッシュから承認記録を復元しないでください。ポリシー、プラグイン識別情報、実行許可が変わると増分レビューのチェックポイントは無効になります。

CI 統合テストは実際の Git コミットと新しくビルドした OCR バイナリを使い、Action のポリシー解決スクリプトと準備用 shell ステップを実行します。fork PR とコメントイベントを模擬し、base バージョンのルールのインストールと MCP 認可を検証し、PR head のチェックアウトや差し替えられた宣言・ロックを拒否します。実際の GitHub API やモデル認証情報は不要です。[テストコマンド](../../examples/github_actions/README.md#testing-project-plugin-policy)を参照してください。

詳細は [Action の設定例](../../examples/github_actions/README.md#project-plugins-in-ci) を参照してください。

---

## パッケージの作成仕様

ディレクトリまたは tar.gz のルートに `ocr-plugin.json` を配置します：

```text
my-plugin/
├── ocr-plugin.json
├── rules.json
├── checks.md
└── server.mjs
```

マニフェスト例：

```json
{
  "schema_version": 1,
  "id": "team-review",
  "name": "Team Review",
  "version": "1.0.0",
  "description": "チーム特有のレビュー規則とリポジトリコンテキストツール",
  "rules": ["rules.json"],
  "mcp_servers": {
    "context": {
      "type": "stdio",
      "command": "node",
      "args": ["${plugin_dir}/server.mjs"],
      "tools": ["team_repository_context"]
    }
  },
  "requires": {
    "ocr_min_version": "1.2.0",
    "features": ["rules", "mcp"]
  },
  "metadata": {
    "homepage": "https://example.com/team-review"
  }
}
```

### ルールパックのファイル

マニフェストの `rules` 配列には相対 `.json` パスを指定します。各 JSON ファイルは OCR の形式に従う `rules` 配列を持ち、各ルールからパッケージ内の Markdown を参照できます。パックに `include` や `exclude` は指定できません。プラグインのルールは既存の設定に追加されます。

```json
{"rules":[{"path":"**/*","rule":"checks.md"}]}
```

ルール JSON は各 512 KiB までで、参照はパッケージ内に限定されます。[review-checklist の例](../../examples/ocr-plugins/review-checklist) を参照してください。

### マニフェストのフィールド

- `schema_version` は `1` にします。未知の操作フィールドや重複 JSON キーは拒否されます。
- `id` は最大 64 文字の小文字、数字、ドット、アンダースコア、ハイフンで、先頭と末尾は英数字にします。Windows の予約デバイス名は使用できません。
- `name` と `version` は空にできません。少なくともルールパックか MCP サーバーを宣言してください。
- `rules` は相対 `.json` パスです。形式は上記のルールパックの説明を参照してください。
- `mcp_servers` の `stdio`（既定）は `command`、`args`、`env`（`KEY=VALUE`）、`tools` 許可リストを使います。`remote` は HTTPS の `url`、`headers`、`tools` を使います。ローカル開発の例外は以下を参照してください。

リモート MCP URL には HTTPS が必要です。HTTP はローカル開発に限り、`localhost` または `127.0.0.1`、`[::1]` などのループバック IP リテラルで利用できます。プライベート LAN アドレスやワイルドカードの待受アドレスを使う HTTP エンドポイントは拒否されます。リダイレクトでは元のスキーム、ホスト、ポートを維持する必要があり、認証情報が HTTP への降格や別オリジンへの転送に伴って送信されることを防ぎます。プラグインのマニフェストと手動 MCP 設定からの接続の両方に適用されます。既存の非ループバック HTTP エンドポイントは HTTPS に移行してください。パッケージのダウンロードには、ローカルホストでも HTTPS が必要です。

以前にインストールした HTTP MCP エンドポイントを持つグローバルプラグインは、引き続き `plugin list`、`disable`、`remove`、`update` で管理できます。有効な非ループバック HTTP プラグインは、レビュー用にプラグインを読み込む前に無効化するか、HTTPS 対応パッケージに更新してください。新規インストール、再有効化、プロジェクトの sync/preflight では引き続き転送ポリシーを検証します。更新で MCP URL が変わる場合は `--accept-permissions` が必要です。

### プレースホルダーのサポート

`command`, `args`, `env` では以下のプレースホルダーが使用可能です：
- `${plugin_dir}`: インストール先スナップショットディレクトリの絶対パス。
- `${os}`: OS 名（`linux`, `darwin`, `windows`）。
- `${arch}`: CPU アーキテクチャ（`amd64`, `arm64`）。
- `${exe}`: Windows の場合は `.exe`、Unix 系の場合は空文字列。

### 配布時のパッケージング

Linux/macOS で実行可能ファイルやスクリプトをパッケージ化する際は、アーカイブ作成前に実行権限を付与してください（例: `chmod +x bin/server`）。OCR は tar.gz 展開時にファイルの実行権限ビットを自動的に保持します：

```sh
tar -czf team-review-1.0.0.tar.gz -C my-plugin .
```

---

## セキュリティモデルと整合性保証

1. **インストール時のコード非実行**: `install`, `sync`, `list`, `show`, `dry-run`, `lock`, `preflight` はプラグインコードを実行しません。
2. **決定論的な整合性ハッシュ**:
   - `archive_sha256`: アーカイブファイルの厳密な SHA-256。
   - `content_sha256`: `ocr-package-tree-v1` による全ファイルの決定論的ツリーハッシュ。
3. **実行権限変更の安全ガード**: グローバルプラグイン更新時に MCP 実行設定の変更を検知した場合、`--accept-permissions` による明示的な承認を要求します。
4. **プロジェクト信頼の完全隔離**: 実行信頼はリポジトリ外に保管され、新たにクローンしたプロジェクトが勝手にローカル権限を行使することはありません。
5. **CI ベースコミットのポリシー保護**: GitHub Actions では、プラグイン宣言とロックは常に PR のベースコミット（信頼できる親コミット）から読み込まれ、フォーク元の不正なコードから保護されます。

<a id="compatibility-and-host-requirements"></a>

## 互換性とホスト要件

`requires` は省略可能です。`ocr_min_version` は `1.2.0` のような完全なセマンティックバージョンが必要です。比較可能なバージョンを持たない `dev` ビルドはこの条件を満たせません。`features` は必須機能を列挙し、このバージョンでは `rules` と `mcp` をサポートします。未対応の要件は実行前に拒否されます。必要のないバージョン制約は省略してください。

説明用のメタデータは `metadata` に記述します。未知の操作フィールドや重複 JSON キーは拒否されます。旧マーケットのメタデータは移行用に読み込めますが、カタログからのインストールは利用できません。
