---
title: Sign in with ChatGPT preview
---

OpenCodeReview can use eligible ChatGPT plan usage through OpenAI's public Responses API. This optional preview is available in the open-source CLI. OpenCodeReview does not charge a separate subscription for it. Account, workspace, region and model eligibility are determined by OpenAI.

The `chatgpt` provider uses the documented Sign in with ChatGPT flow. Sign in with ChatGPT is the default for `ocr auth login`, `status` and `logout`. Explicit `--provider chatgpt` remains supported.

OAuth requests the full documented OpenID Connect and OpenAI scope set: `openid` verifies identity; `profile` and `email` provide account claims; `offline_access` permits refresh-token renewal while you are away; `resource.invoke` authorizes resource invocation; and `chatgpt.tokens.use.direct` permits direct use of ChatGPT plan capacity. The initial dynamic registration includes `agent_name_hint=OpenCodeReview`. The flow requests no connector grants and does not reuse the Codex CLI client registration. It requests only these documented scopes, not arbitrary additional permissions.

The client registration is issued by OpenAI during first sign-in and is retained for that account. The app sends its actual name as the initial registration hint, uses the same name across installations, then omits that hint on returning sign-in. It also sends a stable, opaque `ext_agent_host_id` for this local installation on every sign-in. The same installation keeps the same host ID.

Authorization uses fresh state, nonce and PKCE values. The callback state is checked before code exchange. The ID token signature is verified with the issuer's RSA JWKS; issuer, audience, subject, lifetime and nonce are validated before the returned identity or granted scopes are trusted.

## Sign in and select a model

```bash
ocr auth login --provider chatgpt
ocr auth status --provider chatgpt
ocr llm models
ocr config set provider chatgpt
ocr config set providers.chatgpt.model gpt-6-luna
ocr config set providers.chatgpt.extra_body '{"reasoning":{"effort":"low"}}'
ocr llm test
```

The catalog command shows account-specific suggestions and display names in server order. Without a configured model, the provider selects the first list-visible model. An explicitly configured or `--model` slug is sent unchanged to OpenAI, even if it is absent from the catalog. OpenAI determines whether the selected account can use that model. OpenCodeReview has no compiled model restrictions or aliases for this provider.

The reasoning effort in `extra_body` is an API parameter. The separate review `--effort` flag controls OpenCodeReview's review-round and tool budgets.

Sign-in opens the system browser and listens only on `127.0.0.1`. Use `--no-browser` to print a one-time local URL instead. The CLI never prints an authorization URL containing a retained ID-token hint. Device-code authentication is not supported for this preview.

This preview is intended for local use by one person with an eligible ChatGPT account. Do not use it for pooled or hosted resale, shared CI with personal credentials, or as a fallback to API billing. OpenAI determines account and usage eligibility; this documentation makes no promise of savings or availability.

If sign-in succeeds without ChatGPT plan permission, OpenCodeReview retains the account identity and reports that plan usage is disabled. Request a consent screen for the active retained registration with:

```bash
ocr auth login --provider chatgpt --enable-plan
# For another retained registration:
ocr auth login --provider chatgpt --enable-plan --account <saved-client-id>
```

This explicit action sends `prompt=consent`, the full requested scopes and resource, and the saved client and host IDs. It cannot be combined with `--new-account`. Ordinary returning sign-in does not force consent. Inference remains disabled until the returned grant permits plan usage. You can instead configure an API-key provider; OpenCodeReview does not switch billing paths automatically.

## Accounts and sign-out

```bash
ocr auth login --provider chatgpt --new-account
ocr auth status --provider chatgpt
ocr auth select <saved-client-id> --provider chatgpt
ocr auth login --provider chatgpt --account <saved-client-id>
ocr auth logout --provider chatgpt --account <saved-client-id>
```

Each issued client ID remains associated with its verified issuer and subject, even if multiple registrations share an email. A returning sign-in reuses that client ID. After a code-exchange `invalid_grant`, sign-in restarts authorization once with the callback-issued client ID and fresh state, nonce and PKCE. This pending registration is never activated before signed identity validation. A second failure stops sign-in and leaves previous credentials intact.

The credential database is `~/.opencodereview/auth/chatgpt/accounts.json`. The separate `host.json` stores a stable installation host ID. Credential files are replaced atomically.

On Unix, credential files use owner-only `0600` permissions and the store directory uses `0700`. On Windows, access depends on ACLs inherited from the local user profile; OpenCodeReview does not establish or verify a DACL. Go's [`os.Chmod`](https://pkg.go.dev/os#Chmod) changes only the read-only attribute on Windows. Use a user profile whose ACLs restrict access to your account, check those permissions before signing in, and avoid shared or broadly accessible storage locations. A directory under the user profile is not a guarantee of private access.

Store mutations and refresh rotation share an operating-system file lock, but browser authorization runs outside it so other accounts remain usable. Completion reloads the store and checks the retained registration's revision. Logout, refresh or another completed login on that registration invalidates the pending result. Unrelated account updates are preserved. Access tokens are refreshed before expiry with rotating refresh tokens.

Logout attempts to revoke the renewable session, clears access, refresh and ID tokens, and retains the account registration and host ID. Later sign-in omits `id_token_hint` after logout. If remote revocation cannot be confirmed, the CLI reports that local tokens were cleared and directs you to disconnect the app in ChatGPT settings.

## Transport and usage

This provider sends only to `https://api.openai.com/v1/responses`. Endpoint, protocol and credential overrides are rejected. It uses streaming with `store: false`, sends full history, groups local functions in the `ocr` namespace, and preserves call IDs and encrypted reasoning for subsequent turns. Only `response.completed` counts as success. Failed, incomplete and interrupted streams stop the request, including usage-limit errors received after output starts.

Preview request fields are constrained to OpenAI's supported subset. `extra_body` accepts `reasoning`, `text` and `prompt_cache_key`; it cannot replace the model, history, tools, stream mode or storage policy. Raw provider usage remains available on the response, and the existing opt-in raw review capture records the stream without credential headers.

**Using ChatGPT plan:** eligible requests use the selected account's ChatGPT plan. [Manage usage in ChatGPT settings](https://chatgpt.com/settings/usage). A usage-limit error may reflect a plan-wide or app-specific limit; the CLI does not infer a reset time.

See OpenAI's [SIWC open-source overview](https://developers.openai.com/siwc/token-sharing-open-source), [sign-in contract](https://developers.openai.com/siwc/token-sharing-open-source/sign-in) and [preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations) for current requirements.

See [CLI Reference](../cli-reference/) for the `ocr auth` and `ocr llm models` commands, and [Configuration](../configuration/) for provider settings.
