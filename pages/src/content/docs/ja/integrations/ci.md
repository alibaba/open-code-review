---
title: CI/CD
sidebar:
  order: 4
---

すべての Pull Request または Merge Request で OCR を実行します。上流リポジトリは、コピーして設定するだけのすぐ使える 2 つのパイプラインを提供しています——1 つは GitHub Actions、もう 1 つは GitLab CI です。どちらも
[CLI リファレンス](../cli-reference/#json)に記載されている中核コマンドの薄いラッパーです。

## CI/CD 統合の仕組み

本ページの各レシピは同じパターンに従います——以下の GitHub Actions と GitLab CI のセクションは、その具体的な実装に過ぎません。

1. **PR / MR イベントでトリガー。** 新しい pull request、更新された merge request、または手動の
   `/open-code-review` コメントがジョブをトリガーします。
2. **runner に `ocr` をインストール**します。通常は
   `npm install -g @alibaba-group/open-code-review` です。runner は一時的なため、これは実行のたびに発生します。
3. **CI secret から `ocr config set` 経由で LLM を設定**します（エンドポイント、token、model）。フォールバックできる永続的な `~/.opencodereview` はありません。
4. **区間モードでレビューを実行**し、機械可読な出力を得ることで、stdout がクリーンな JSON の外殻になるようにします。

   ```bash
   ocr review \
     --from "origin/<base-branch>" \
     --to "origin/<head-branch>" \
     --format json \
     --audience agent
   ```

   `--format json` は解析可能なペイロードを提供し、`--audience agent` は進捗行を抑制します。各レシピが消費する外殻は [JSON 出力](../cli-reference/#json)を参照してください。
5. **JSON を解析**し、`comments[]` を反復処理します。
6. **プロバイダーの review API を通じてコメントを PR / MR に貼り戻します。** 有効な行情報を持たない項目（ファイルレベルの発見）はインラインで貼り付けるのではなくサマリーの注記にまとめられます。インラインの一括 API がリクエストを拒否した場合、貼り付け手順も通常のサマリーコメントにフォールバックします。

常に 2 種類の認証情報が関わります。OCR が発見を生成するために使う **LLM 認証情報**と、貼り付け手順がコメントを貼り戻すために使う **PR/MR 書き込み token** です。GitHub のレシピは `GITHUB_TOKEN` を通じて後者を自動的に提供します。GitLab では `GITLAB_API_TOKEN` を明示的に設定することを推奨しますが、fork MR に対しては組み込みの `CI_JOB_TOKEN` にフォールバックします（これは `/discussions` を通じてディスカッションを開始できます）——信頼性のためには専用の token の使用を推奨します。

## オプションの CI ゲート

両方の統合で、レビューの投稿後に [`ocr gate`](../cli-reference/#ocr-gate) を実行できます。
ゲートは既定で無効です。使用するには、`ocr gate` を含む OCR ビルドを GitHub の
`ocr_version` または GitLab の `OCR_VERSION` で指定します。ゲートを有効にすると、
モデルを呼び出す前にコマンドが利用可能か確認します。

| 判定 | 意味 | 終了コード |
|---|---|---|
| `pass` | 選択した全ファイルのレビュー完了を確認でき、有効にしたチェックがすべて成功した。 | `0` |
| `fail` | 指摘の重要度が設定したしきい値以上だった。 | `1` |
| `inconclusive` | カバレッジ、対象リビジョン、コメントの投稿など、判定に必要な情報が不足している。 | `1` |

レビュー前に merge base と head のコミット ID を確定し、レビューとゲートに同じ ID を
渡します。ゲートは元の JSON に含まれるすべての指摘を評価し、サマリーに振り分けた指摘や
重複投稿を省いた指摘も含みます。予算による中止や `waived` 項目、`code_comment` の
失敗がある場合、ゲートは通過できません。対象ファイルが 0 件の場合や、manifest の
バージョンが未対応の場合も同様です。

ジョブの成功には、レビュー実行と投稿の成功、およびゲートの `pass` 判定が必要です。
結果の投稿とゲート判定を試みてから、ジョブの最終状態を決めます。インライン投稿に
失敗してサマリーへフォールバックした場合、ジョブは失敗として扱います。
最終サマリーの投稿確認も必要です。

## GitHub Actions

上流のワークフローは
[`examples/github_actions/ocr-review.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/github_actions/ocr-review.yml)
にあります。

### 何をするか

- `pull_request_target`（`opened`）**および** `issue_comment` イベント（本文が
  `/open-code-review` または `@open-code-review` で始まるもの）でトリガーします。後者はレビュアーが PR にコメントすることで OCR をオンデマンドで再実行できるようにします。（`pull_request` ではなく `pull_request_target` を使うことで、fork から提出された PR でも secret を利用できます。OCR は diff を読むだけで、PR 内のコードは実行しません。）
- `npm install -g @alibaba-group/open-code-review` で OCR をインストールし、`ocr config set` で設定を書き込み、ブランチ区間モードで中核コマンドを実行します。
- JSON の外殻を解析し、GitHub Pull Request Review API を通じて各発見をインラインのレビューコメントとして貼り付けます。行情報を持たないコメントはサマリー本文にまとめられます。一括送信が失敗した場合は 1 件ずつの貼り付けにフォールバックし、統計をサマリーコメントに表示します。

### インストール

ワークフローをリポジトリに配置します。

```bash
mkdir -p .github/workflows
curl -o .github/workflows/ocr-review.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/github_actions/ocr-review.yml
```

### 必須の secret

**Settings → Secrets and variables → Actions** で設定します。

| Secret | 必須 | 説明 |
|---|---|---|
| `OCR_LLM_URL` | はい | LLM API エンドポイント（例：`https://api.openai.com/v1/chat/completions`）。 |
| `OCR_LLM_AUTH_TOKEN` | はい | LLM API の認証 token。この CI secret は `ocr config set llm.auth_token` に渡されます。（OCR の直接の環境変数は `OCR_LLM_TOKEN` であり、`OCR_LLM_AUTH_TOKEN` ではありません。） |
| `OCR_LLM_MODEL` | いいえ | モデル名。デフォルトはありません——明示的に設定する必要があります。 |
| `OCR_LLM_USE_ANTHROPIC` | いいえ | Anthropic Claude モデルの場合は `true` に設定します。 |

`GITHUB_TOKEN` は自動的に提供されます。ワークフローはレビューコメントを貼り付けるために `pull-requests: write` を宣言しています。

> ワークフロー起動時には
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`
> も実行され、このフィールドをサポートしない LLM プロバイダー向けに thinking-mode リクエストをオフにします。プロバイダーが thinking-mode を維持する必要がある場合は、その行を削除してください。

### アクションの入力

上流のワークフローは、`uses: alibaba/open-code-review@main` を通じてレビューを再利用可能なコンポジットアクション（[`action.yml`](https://github.com/alibaba/open-code-review/blob/main/action.yml)）に委譲しています。上記の認証情報に加えて、以下の入力でレビュー自体を調整できます——アクションステップの `with:` に指定してください。

| 入力 | デフォルト | 説明 |
|---|---|---|
| `effort` | `''` | `ocr review --effort` に渡すレビュー強度プリセット：`low`、`medium`、`high`（大文字小文字を区別しません）。空の場合は CLI のデフォルト（設定済みの値、なければ medium）を使います。OCR v1.10.0 以降が必要で、それより古いバージョンではアクションが明確なエラーで早期に失敗します。 |
| `max_tokens_budget` | `''` | `ocr review --max-tokens-budget` に渡すトークン総量（入力 + 出力）の上限。空または `'0'` は無制限です。LLM の各ラウンドの前に確認されます: すでに上限を超えたサブタスクには発見を提出するための最終ラウンドが 1 回与えられ、以降のサブタスクはディスパッチされず、予算超過およびスキップされたファイルは `failed(budget)` として報告され、部分的な結果は引き続き公開され、レビューは 0 で終了します。 |
| `llm_reasoning_effort` | `''` | `reasoning_effort` リクエストフィールドを調整できるモデル（GLM-5.x、OpenAI reasoning モデルなど）の推論深度：`minimal`、`low`、`medium`、`high`、`max`（大文字小文字を区別しません）。`llm_extra_body` 経由でリクエストボディにマージされるため、公開済みのすべての CLI バージョンで動作します。`llm_extra_body` 内の明示的な `reasoning_effort` キーがこの入力より優先されます。空（デフォルト）の場合は送信しません。OpenAI 互換プロトコル専用です——Anthropic API は未知のボディフィールドを拒否するため、そのプロトコルではアクションが即座に失敗します。Anthropic の thinking 制御には `llm_extra_body` の明示的なキーを使ってください。 |
| `stream_progress` | `'false'` | `'true'` にすると、レビューが終了するまで沈黙する代わりに、`[ocr]` の進捗行をワークフローログへライブで流します（stderr の human audience）。表示のみの切り替えで、stderr は引き続きファイルにキャプチャされ、アーティファクトとコメント投稿に使われます。 |
| `gate` | `'false'` | 共有の CI ゲートを有効にします。`true` / `false` は大文字小文字を区別しません。 |
| `fail_on_severity` | `''` | `critical`、`high`、`medium`、`low` の指定値以上の指摘があると失敗します。`gate: 'true'` が必要です。空の場合は重要度の検査を省略し、カバレッジと投稿の要件を検査します。値の大文字小文字と前後の空白は無視します。 |

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
    effort: high
    max_tokens_budget: '10000000'
    llm_reasoning_effort: low
    stream_progress: 'true'
```

入力の完全な一覧は
[`action.yml`](https://github.com/alibaba/open-code-review/blob/main/action.yml)
を参照してください——投稿モード（`sticky_summary`、`incremental`）、重要度/カテゴリのルーティング、プッシュをまたぐチェックポイントなどを含みます。

### ゲートの有効化

既存のアクションステップの `with:` に追加します。

```yaml
with:
  gate: 'true'
  fail_on_severity: high
```

有効時は merge base から head までの全範囲をレビューします。`checkpoint_range` の
設定にかかわらず、チェックポイントの読み取りと更新を省略します。長期間開いている
PR では同じ範囲を再レビューするため、トークン消費が増える場合があります。
コメントの振り分けと、重複コメントを省く投稿は引き続き使用できます。

`gate_exit_code` はゲートコマンドの終了コードです。ゲートが無効、またはその段階に
到達しなかった場合は空です。
`upload_artifacts: 'true'`（既定値）では、`ocr-result.json` と `ocr-stderr.log` を保存します。
ゲートを実行した場合は `ocr-gate.json` と `ocr-gate-stderr.log` も含まれます。
各アクション呼び出しは独立した一時ディレクトリを使用します。

### カスタマイズ

以下はすべて、あなたがコピーしたばかりのワークフローファイル
（`.github/workflows/ocr-review.yml`）への編集です。

#### 背景コンテキスト

`--background` は最も効果の大きい単一の引数です——[すべてのモードに適用されるヒント](../#tips-that-apply-to-every-pattern)を参照してください。PR タイトルを渡します（タイトルが `feat(auth): add OAuth2 support` のようなセマンティックな規約に従っている場合、より効果的です）。

```yaml
- name: Run OCR review
  env:
    PR_TITLE: ${{ github.event.pull_request.title }}
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review \
      --background "$PR_TITLE" \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF" \
      --format json --audience agent
```

PR で制御可能な値は `${{ }}` を `run:` に直接展開するのではなく、`env:` 経由で渡してください。GitHub は `${{ }}` を shell が行を解析する *前に* テキストとして置換するため、shell のメタ文字を含む PR タイトルやブランチ名が runner 上で実行されてしまいます。

#### カスタムルール

`--rule` でプロジェクト固有のルールファイルを渡します。

```yaml
- name: Run OCR review
  env:
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review --rule ./my-rules.json \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF"
```

スキーマは[レビュールール](../../review-rules/)を参照してください。

#### 並行数

デフォルトは 8 つの並行サブ agent で、サブタスクごとに 1 つです。大きな PR では、LLM プロバイダーのレート制限に抵触しないよう下げてください。

```yaml
- name: Run OCR review
  env:
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review --concurrency 5 \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF"
```

#### トリガーモード

デフォルトのワークフローは、PR が **opened** されたとき、および `/open-code-review` または
`@open-code-review` で始まる PR コメントのときにトリガーします。よくある 2 つの調整があります。

より多くの PR ライフサイクルイベントで実行する（例：新しい commit がプッシュされたときに再レビュー）：

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
```

異なるコメントキーワードを使う：

```yaml
if: |
  github.event_name == 'pull_request' ||
  (github.event_name == 'issue_comment'
    && github.event.issue.pull_request
    && startsWith(github.event.comment.body, '/review'))
```

`github.event.issue.pull_request` のチェックは、コメントが通常の issue ではなく PR 上のものであることを保証します。

#### OCR のバージョン固定

デフォルトのワークフローは最新のリリース版をインストールします。固定するには：

```yaml
- name: Install OpenCodeReview
  run: npm install -g @alibaba-group/open-code-review@1.0.0
```

#### GitHub App として投稿する

デフォルトではレビューコメントは `github-actions[bot]` から投稿されます。`OpenCodeReview Bot` のようなカスタムブランドの bot として投稿するには、`GITHUB_TOKEN` を GitHub App の installation token に置き換えます。

1. *Settings → Developer settings → GitHub Apps → New GitHub App* で **app を作成**します。webhook は無効にします（このユースケースでは不要）。*Repository permissions* で次を付与します。
   - **Pull requests**：Read and write
   - **Contents**：Read-only（diff の取得用）
   - **Metadata**：Read-only（必須）

2. app 設定ページから**秘密鍵を生成**し、`.pem` ファイルをダウンロードします。同じページの **App ID** を控えておきます。

3. app を OCR にレビューさせたいリポジトリに**インストール**します。Installation ID はインストール後の URL に現れます。例：`https://github.com/settings/installations/12345` → ID は `12345`。

4. *Settings → Secrets and variables → Actions* で**3 つの secret を追加**します。

   | Secret | 値 |
   |---|---|
   | `GITHUB_APP_ID` | App ID。 |
   | `GITHUB_APP_PRIVATE_KEY` | `.pem` ファイルの全内容。`-----BEGIN RSA PRIVATE KEY-----` と `-----END RSA PRIVATE KEY-----` の行を含みます。 |
   | `GITHUB_APP_INSTALLATION_ID` | Installation ID。 |

5. コメント貼り付け手順で **token を生成して使用**します。

   ```yaml
   - name: Get GitHub App Token
     id: app-token
     uses: actions/create-github-app-token@v1
     with:
       app-id: ${{ secrets.GITHUB_APP_ID }}
       private-key: ${{ secrets.GITHUB_APP_PRIVATE_KEY }}

   - name: Post review comments to PR
     uses: actions/github-script@v7
     with:
       github-token: ${{ steps.app-token.outputs.token }}
       script: |
         # ...existing post script...
   ```

レビューは `github-actions[bot]` ではなく、あなたの app の名前で投稿されるようになります。

#### GitHub Code Scanning に指摘をアップロードする（SARIF）

`--format sarif` は
[SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
レポートを stdout に書き出します。ファイルにリダイレクトし、CodeQL の
`upload-sarif` アクションでアップロードすると、指摘が
**Security → Code scanning** に表示されます：

```yaml
- name: Run OCR review
  env:
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF" \
      --format sarif --audience agent > results.sarif

- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

SARIF は機械可読な形式なので、OCR は stdout 上の進捗行を抑制し、
`results.sarif` にはレポートだけが含まれます。`--preview` は
`--format sarif` に対応していません。レポートを生成するには、完全な
review（または `ocr scan`）を実行してください。

### トラブルシューティング

| 症状 | 原因 / 修正 |
|---|---|
| `Cannot find merge-base` | checkout 手順が浅いクローンを使っていますが、区間モードのレビューには完全な履歴が必要です。上流のワークフローは `actions/checkout` に `fetch-depth: 0` を設定しています——ファイルを編集する際はこの設定を保持してください。 |
| `Failed to parse OCR output` | `OCR_LLM_URL` または `OCR_LLM_AUTH_TOKEN` が欠落しているか誤っています。*Settings → Secrets and variables → Actions* で値を再確認してください。 |
| レビューコメントが誤った行に付く | レビュー中に PR の head や diff が変わった可能性があります。インライン投稿できない指摘はサマリーコメントに含まれます。`gate: 'true'` の場合、インライン投稿の失敗が残るとジョブは失敗します。レビュー対象の head と diff の位置を確認して再実行してください。 |

診断には、アップロードされた `ocr-result.json` と `ocr-stderr.log` を確認してください。
ゲートを実行した場合は `ocr-gate.json` と `ocr-gate-stderr.log` も含まれます。

## GitLab CI

上流のパイプラインは
[`examples/gitlab_ci/.gitlab-ci.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/gitlab_ci/.gitlab-ci.yml)
にあります。

### 何をするか

- `merge_requests` イベント（作成、更新、再オープンといったすべての MR イベント）でトリガーします。
- `node:20` イメージで実行し、OCR をインストールし、`ocr config set` で設定し、MR diff モードで中核コマンドを実行します。
- `post_review.py` で JSON 形式のレビュー結果を解析し、各指摘を GitLab Discussion として diff 上に投稿します。MR の `versions` エンドポイントを使って正しい `base_sha` / `start_sha` /
  `head_sha` を計算し、正確に位置決めします。インラインで投稿できないコメントは通常の MR note にフォールバックし、最後にサマリー note で締めくくります。

### インストール

パイプラインと投稿スクリプトをリポジトリのルートに配置します。

```bash
curl -o .gitlab-ci.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/.gitlab-ci.yml
curl -o post_review.py \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/post_review.py
```

すでに `.gitlab-ci.yml` があり、それを保持したい場合は、レシピを別のパスに配置して `include:`
で取り込みます。

```yaml
include:
  - local: 'ci/ocr-review.gitlab-ci.yml'
```

`post_review.py` はリポジトリのルートに置きます。別の場所に置く場合はパイプライン内のパスを更新してください。

### 必須の CI/CD 変数

**Settings → CI/CD → Variables** で設定します。

| 変数 | 必須 | マスク | 説明 |
|---|---|---|---|
| `OCR_LLM_URL` | はい | いいえ | LLM API エンドポイント URL。 |
| `OCR_LLM_AUTH_TOKEN` | はい | はい | API 認証 token。この CI 変数は `ocr config set llm.auth_token` に渡されます。（OCR の直接の環境変数は `OCR_LLM_TOKEN` であり、`OCR_LLM_AUTH_TOKEN` ではありません。） |
| `OCR_LLM_MODEL` | いいえ | いいえ | モデル名。デフォルトはありません——明示的に設定する必要があります。 |
| `GITLAB_API_TOKEN` | いいえ | はい | `api` scope を持つ project / personal / group access token。オプションです——欠落時は組み込みの `CI_JOB_TOKEN` にフォールバックします（fork MR など）。信頼性のためには専用の `GITLAB_API_TOKEN` を推奨します。 |

> GitLab の 8 文字以上という要件はマスクされた変数に適用されます。`OCR_GATE` などの
> ポリシー値はマスクなしの変数で設定します。この例の `llm.use_anthropic` は
> `false` です。Anthropic Claude モデルを使う場合は、その設定行を編集してください。

> パイプライン起動時には
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`
> も実行され、このフィールドをサポートしない LLM プロバイダー向けに thinking-mode リクエストをオフにします。プロバイダーが thinking-mode を維持する必要がある場合は、その行を削除してください。

> **手軽な bot 命名のヒント。** Project Access Token と Group Access Token では、
> token の**名前**が MR ディスカッションの横に表示されます。token を `OpenCodeReview Bot` と命名すれば、追加設定なしでレビューディスカッションにブランド名を付けられます——[サービスアカウント名義で投稿する](#post-under-a-service-account-identity)に記載のより永続的なサービスアカウント設定が不要なときに便利です。

### ゲートの有効化

```yaml
variables:
  OCR_GATE: 'true'
  OCR_FAIL_ON_SEVERITY: high
```

この値はマスクなしの変数として設定します。`OCR_GATE` の既定値は `false` で、
`true` / `false` の大文字小文字を区別しません。重要度は `critical`、`high`、`medium`、
`low` を指定でき、前後の空白と大文字小文字は無視します。空の場合は重要度を検査しません。
無効な値はインストールやモデル呼び出しの前にエラーになります。ゲートが無効な場合、
`OCR_FAIL_ON_SEVERITY` は既存の投稿スクリプトによる判定を維持します。

### カスタマイズ

以下はすべて、あなたがコピーしたばかりの `.gitlab-ci.yml` への編集です。

#### 背景コンテキスト

MR タイトルを `--background` に渡します——タイトルが `feat(auth): add OAuth2 support`
のようなセマンティックな規約に従っている場合、より効果的です。

```yaml
script:
  - |
    ocr review \
      --background "$CI_MERGE_REQUEST_TITLE" \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}" \
      --format json --audience agent
```

#### カスタムルールと並行数

GitHub Actions のレシピと同じ引数です——`--rule` でプロジェクト固有のルールファイルを渡し、
`--concurrency` で並行サブ agent を制限します（デフォルトは 8、サブタスクごとに 1 つ）。

```yaml
script:
  - |
    ocr review --rule ./my-rules.json --concurrency 5 \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}"
```

ルールのスキーマは[レビュールール](../../review-rules/)を参照してください。

#### OCR のバージョン固定

インストールする npm バージョンを `OCR_VERSION` で指定します。ゲートを有効にする場合は、`ocr gate` を含むバージョンが必要です。

```yaml
variables:
  OCR_VERSION: '<version>'
```

#### プッシュのたびの再レビューを避ける

`only: [merge_requests]` は MR が更新されるたびに実行されます。`OCR_GATE=false` で
コストを抑える場合は、既存の OCR note を検出してレビューを省略できます。
`OCR_GATE=true` では、MR が更新されるたびに現在の変更範囲全体を新たにレビューします。
次の例では、ゲートが無効な場合だけ既存 note によるスキップを適用します。
この省略を使うと、その後の変更は次のレビューを実行するまで未レビューのままになります。

```python
import json, os, sys, urllib.request

GITLAB_URL = os.environ.get("CI_SERVER_URL", "https://gitlab.com")
PROJECT_ID = os.environ["CI_PROJECT_ID"]
MR_IID     = os.environ["CI_MERGE_REQUEST_IID"]
API_TOKEN  = os.environ["GITLAB_API_TOKEN"]

url = (
    f"{GITLAB_URL}/api/v4/projects/{PROJECT_ID}"
    f"/merge_requests/{MR_IID}/notes?per_page=100"
)
req = urllib.request.Request(url, headers={"PRIVATE-TOKEN": API_TOKEN})
with urllib.request.urlopen(req) as resp:
    notes = json.loads(resp.read().decode())

if (
    os.environ.get("OCR_GATE", "false").lower() == "false"
    and any("OpenCodeReview" in n.get("body", "") for n in notes)
):
    print("OCR already reviewed this MR. Skipping to save tokens.")
    sys.exit(0)

# ...otherwise call `ocr review ...` as usual and write the JSON to
# the file the posting step expects.
```

この後に再レビューを強制するには、MR から以前の OCR note を削除してください——次のパイプライン実行では OCR note が見当たらなくなり、処理を続行します。

#### セルフホスト GitLab

コードの変更は不要です。貼り付けスクリプトは `CI_SERVER_URL`（GitLab が各 runner で自動的に設定します）を読むため、そのままで自分のインスタンスと通信できます。`GITLAB_API_TOKEN` が `gitlab.com` ではなく、あなたのセルフホストインスタンスによって発行されていることだけ確認してください。

#### サービスアカウント名義で投稿する

デフォルトではレビューディスカッションは `GITLAB_API_TOKEN` が属するユーザー名で表示されます。プロジェクトレベルのサービスアカウントに切り替えると、`OpenCodeReview Bot` のようなカスタムブランドの bot 名義が得られます。

1. *Project → Settings → Service Accounts → New service account* で**サービスアカウントを作成**します。選んだ名前（例：`OpenCodeReview Bot`）が MR ディスカッションの横に表示されます。

2. *Settings → Members → Invite member* で**プロジェクトに招待**します。サービスアカウント名を検索し、`Developer` または `Maintainer` を割り当てます——どちらもディスカッションの投稿に必要な権限を持ちます。

3. *Settings → Service Accounts →（該当アカウント）→ Add new token* で **access token を発行**します。必要な scope は `api` です。token はすぐにコピーしてください——GitLab は一度しか表示しません。

4. *Settings → CI/CD → Variables* で **token の値を置き換え**ます——既存の `GITLAB_API_TOKEN` の値をサービスアカウントの token で置き換えます（変数名は変えません）。

ディスカッションは、最初に token を作成したユーザー名ではなく、サービスアカウント名で投稿されるようになります。

### トラブルシューティング

| 症状 | 原因 / 修正 |
|---|---|
| `Cannot find merge-base` | runner が浅いクローンを使っています。上流のパイプラインは `GIT_DEPTH: 0` を設定して完全なクローンを強制します——ファイルを編集する際はこの設定を保持してください。 |
| 投稿時の `API error 403` | `GITLAB_API_TOKEN` に `api` scope が無い、プロジェクトのメンバーでない、または——セルフホストの場合——別のインスタンスによって発行されています。`api` scope で再発行し、*Settings → CI/CD → Variables* で再登録してください。 |
| `Failed to parse OCR output` | `OCR_LLM_URL` または `OCR_LLM_AUTH_TOKEN` が誤っています。*Settings → CI/CD → Variables* で値を再確認してください。 |
| インラインコメントが誤った行に付く | GitLab のインラインディスカッションは正確な SHA の一致を要求します。貼り付けスクリプトは `versions` メタデータを取得して正しい `base_sha` / `start_sha` / `head_sha` を得ます。それでも発見をアンカーできない場合は、通常の MR note にフォールバックします。 |

パイプラインは `when: always` でプロジェクト相対のアーティファクトを保存します。
`.ocr/ocr-result.json`、`.ocr/ocr-stderr.log`、`.ocr/ocr-gate.json`、
`.ocr/ocr-gate-stderr.log` が対象です。ゲートが無効、またはその段階に到達しなかった場合、
ゲートのファイルは空です。投稿統計は `.ocr/ocr-stats.env` の dotenv レポートで確認できます。
デバッグ手順でこれらのファイルを確認してください。

```yaml
script:
  - cat .ocr/ocr-result.json
  - cat .ocr/ocr-stderr.log
  - cat .ocr/ocr-gate.json
  - cat .ocr/ocr-gate-stderr.log
```

## 関連項目

- [CLI リファレンス](../cli-reference/#json)——2 つのパイプラインが消費する JSON の構造。ゼロから CI スクリプトを書くときに役立ちます。
- [設定](../../configuration/)——OCR が受け付けるすべての環境変数と config key。
