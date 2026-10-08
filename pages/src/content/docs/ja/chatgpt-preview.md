---
title: ChatGPT サインインのプレビュー
---

OpenCodeReview は OpenAI の公開 Responses API を通じて、対象となる ChatGPT プランの利用枠を使用できます。これはオープンソース CLI の任意のプレビュー機能で、OpenCodeReview が別途サブスクリプション料金を請求することはありません。アカウント、ワークスペース、地域、モデルの利用資格は OpenAI が決定します。

`chatgpt` provider は公開されている Sign in with ChatGPT フローを使います。 `ocr auth login`、`status`、`logout` は既定で Sign in with ChatGPT を使用します。明示的な `--provider chatgpt` も引き続き使用できます。

## サインインしてモデルを選択

```bash
ocr auth login --provider chatgpt
ocr auth status --provider chatgpt
ocr llm models
ocr config set provider chatgpt
ocr config set providers.chatgpt.model gpt-6-luna
ocr config set providers.chatgpt.extra_body '{"reasoning":{"effort":"low"}}'
ocr llm test
```

OAuth 登録では、OpenID Connect と OpenAI の次の scope をすべて要求します: `openid`（本人確認）、`profile` と `email`（アカウント情報）、`offline_access`（リフレッシュトークンによるオフライン更新）、`resource.invoke`（リソース呼び出し）、`chatgpt.tokens.use.direct`（ChatGPT プラン利用枠の直接使用）。初回登録では `agent_name_hint=OpenCodeReview` を送信します。コネクター権限は要求せず、Codex CLI のクライアント登録も再利用しません。この文書化されたフローに必要な scope のみを要求し、任意の追加権限は要求しません。

初回サインイン時に OpenAI がクライアント登録を発行し、そのアカウント用に保存します。アプリ名を初回登録時のヒントとして送信し、インストール間で同じ名前を使い、再サインインでは省略します。ローカルインストールを示す安定した不透明な `ext_agent_host_id` は毎回送信し、同じインストールでは同じ ID を使います。

認証ごとに新しい state、nonce、PKCE 値を生成します。コード交換前に callback の state を検証します。issuer の RSA JWKS で ID token の署名を検証し、issuer、audience、subject、有効期間、nonce を確認してから ID と付与 scope を信頼します。

モデル一覧には、現在のアカウントで表示可能な候補と表示名がサーバーの順序で表示されます。モデルを設定していない場合、provider は一覧の最初の表示モデルを選びます。明示的に設定したモデルまたは `--model` の slug は一覧になくてもそのまま OpenAI に送られます。アカウントで利用できるかどうかは OpenAI が判定します。この provider に固定モデル一覧や別名は組み込まれていません。

`extra_body` の推論 effort は API パラメーターです。別の review `--effort` フラグは OpenCodeReview のレビューラウンドとツール予算を制御します。

サインインではシステムブラウザーを開き、`127.0.0.1` のみで待ち受けます。`--no-browser` を指定すると一度限りのローカル URL を表示します。CLI は保存済み ID token hint を含む認証 URL を表示しません。このプレビューではデバイスコード認証をサポートしません。

サインインできても ChatGPT プラン権限がない場合、OpenCodeReview はアカウント ID を保存し、プラン利用が無効であることを表示します。現在保存されている登録で同意画面を要求するには、次を実行します:

```bash
ocr auth login --provider chatgpt --enable-plan
# 別の保存済み登録の場合:
ocr auth login --provider chatgpt --enable-plan --account <saved-client-id>
```

この明示的な操作は `prompt=consent`、完全な要求 scope と resource、および保存済みのクライアント ID とホスト ID を送信します。`--new-account` とは併用できません。通常の再ログインでは同意を強制しません。返された grant がプラン利用を許可するまで推論は無効です。代わりに API key provider を設定できます。OpenCodeReview が課金経路を自動で切り替えることはありません。

## アカウントとサインアウト

```bash
ocr auth login --provider chatgpt --new-account
ocr auth status --provider chatgpt
ocr auth select <saved-client-id> --provider chatgpt
ocr auth login --provider chatgpt --account <saved-client-id>
ocr auth logout --provider chatgpt --account <saved-client-id>
```

発行された各クライアント ID は検証済み issuer と subject に結び付けられます。同じメールアドレスを使う複数の登録も区別されます。再ログインでは同じクライアント ID を再利用します。コード交換で `invalid_grant` が返ると、コールバックで発行されたクライアント ID を使い、新しい state、nonce、PKCE で認証を一度だけ再開します。署名付き ID の検証が終わるまで、この保留中の登録は有効化されません。2 回目も失敗した場合はログインを停止し、既存資格情報を保持します。

資格情報データベースは `~/.opencodereview/auth/chatgpt/accounts.json` です。別の `host.json` には安定したインストールホスト ID を保存します。資格情報ファイルはアトミックに置き換えます。

Unix では資格情報ファイルに所有者のみ読み書き可能な `0600`、保存ディレクトリに `0700` の権限を設定します。Windows でのアクセス権はローカルユーザープロファイルから継承する ACL に依存し、OpenCodeReview は DACL を設定も検証もしません。Go の [`os.Chmod`](https://pkg.go.dev/os#Chmod) は Windows では読み取り専用属性だけを変更します。ACL で自分のアカウントにアクセスを制限したユーザープロファイルを使い、サインイン前に権限を確認し、共有または広くアクセス可能な保存場所を避けてください。ユーザープロファイル内のディレクトリであるだけでは、非公開のアクセスは保証されません。

保存内容の変更と refresh token のローテーションには OS ファイルロックを使いますが、ブラウザー認証はロックの外で実行するため、他のアカウントを利用できます。完了時に保存内容を読み直し、登録の revision を確認します。その登録のログアウト、refresh、または別のログイン完了が保留結果を無効にします。別アカウントの更新は保持されます。access token は期限前にローテーション refresh token で更新します。

ログアウトは更新可能なセッションの失効を試み、access、refresh、ID token を消去し、アカウント登録とホスト ID は保持します。ログアウト後の再ログインでは `id_token_hint` を送信しません。サーバー側の失効を確認できない場合、CLI はローカル token を消去したことを表示し、ChatGPT 設定からアプリを切断するよう案内します。

## 通信と利用状況

この provider は `https://api.openai.com/v1/responses` のみに送信します。エンドポイント、プロトコル、資格情報の上書きは拒否されます。`store: false` でストリーミングし、履歴全体を送信し、ローカル関数を `ocr` 名前空間にまとめ、後続ターン用に呼び出し ID と暗号化された推論内容を保持します。`response.completed` のみが成功を示します。出力開始後の利用制限エラーを含め、失敗、不完全、中断したストリームではリクエストを停止します。

プレビューのリクエストフィールドは OpenAI がサポートする範囲に制限されます。`extra_body` で指定できるのは `reasoning`、`text`、`prompt_cache_key` です。モデル、履歴、ツール、ストリーム方式、保存ポリシーは変更できません。応答では provider の生の利用量データを利用できます。既存の任意の生レビュー記録は認証ヘッダーを含めずにストリームを記録します。

**ChatGPT プランの利用:** 対象リクエストは選択したアカウントの ChatGPT プランを使用します。[ChatGPT 設定で利用状況を管理](https://chatgpt.com/settings/usage)。利用制限エラーはプラン全体またはアプリ固有の制限を示す場合があります。CLI はリセット時刻を推測しません。

現在の要件は OpenAI の[オープンソース SIWC 概要](https://developers.openai.com/siwc/token-sharing-open-source)、[サインイン仕様](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)、[プレビュー制限](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)を参照してください。

`ocr auth` と `ocr llm models` のコマンドは [CLI リファレンス](../cli-reference/)を、provider 設定は[設定ガイド](../configuration/)を参照してください。
