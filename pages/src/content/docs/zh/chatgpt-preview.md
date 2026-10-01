---
title: 使用 ChatGPT 登录预览版
---

OpenCodeReview 可通过 OpenAI 公开 Responses API 使用符合条件的 ChatGPT 套餐额度。此功能是开源 CLI 中的可选预览版，OpenCodeReview 不另收订阅费。账户、工作区、地区和模型资格由 OpenAI 决定。

`chatgpt` provider 使用公开的 Sign in with ChatGPT 流程。 `ocr auth login`、`status` 和 `logout` 默认使用 Sign in with ChatGPT。仍支持显式指定 `--provider chatgpt`。

## 登录并选择模型

```bash
ocr auth login --provider chatgpt
ocr auth status --provider chatgpt
ocr llm models
ocr config set provider chatgpt
ocr config set providers.chatgpt.model gpt-6-luna
ocr config set providers.chatgpt.extra_body '{"reasoning":{"effort":"low"}}'
ocr llm test
```

OAuth 注册请求完整的 OpenID Connect 与 OpenAI 范围：`openid`（身份验证）、`profile` 和 `email`（账户资料）、`offline_access`（刷新令牌以便离线续期）、`resource.invoke`（调用资源）及 `chatgpt.tokens.use.direct`（直接使用 ChatGPT 套餐额度）。首次注册会发送 `agent_name_hint=OpenCodeReview`。请求不包含连接器授权，也不会复用 Codex CLI 客户端注册。仅请求这些文档流程所需的范围，不请求任意额外权限。

首次登录时由 OpenAI 签发客户端注册，并为该账户保留。应用在首次注册时发送真实名称作为提示，并在不同安装间使用相同名称；返回登录时省略此提示。每次登录都会发送此本地安装对应的稳定、不透明 `ext_agent_host_id`；同一安装始终使用相同主机 ID。

每次授权都会生成新的 state、nonce 和 PKCE 值。交换授权码前会验证回调 state。使用 issuer 提供的 RSA JWKS 验证 ID token 签名，并在信任账户身份和授权范围前检查 issuer、audience、subject、有效期和 nonce。

模型目录按服务端顺序显示当前账户可见的建议和名称。未设置模型时，provider 选择目录中第一个可见模型。显式设置的模型或 `--model` 参数会原样发送，即使目录中没有该模型；OpenAI 决定当前账户是否可用。OpenCodeReview 不为此 provider 编译固定模型列表或别名。

`extra_body` 中的推理强度是 API 参数。单独的 review `--effort` 参数控制 OpenCodeReview 的审查轮次和工具预算。

登录会打开系统浏览器，并且只监听 `127.0.0.1`。使用 `--no-browser` 可打印一次性本地 URL。CLI 不会打印包含已保存 ID token 提示的授权 URL。此预览版不支持设备码登录。

如果登录成功但未获得 ChatGPT 套餐权限，OpenCodeReview 会保留账户身份并报告套餐用量已禁用。可针对当前保存的注册请求同意页：

```bash
ocr auth login --provider chatgpt --enable-plan
# 对另一项已保存注册：
ocr auth login --provider chatgpt --enable-plan --account <saved-client-id>
```

此显式操作会发送 `prompt=consent`、完整请求范围和资源，以及已保存的客户端和主机 ID。它不能与 `--new-account` 同时使用。普通的返回用户登录不会强制重新同意。只有返回授权允许套餐使用后才会启用推理。也可以配置 API key provider；OpenCodeReview 不会自动切换计费方式。

## 账户与退出登录

```bash
ocr auth login --provider chatgpt --new-account
ocr auth status --provider chatgpt
ocr auth select <saved-client-id> --provider chatgpt
ocr auth login --provider chatgpt --account <saved-client-id>
ocr auth logout --provider chatgpt --account <saved-client-id>
```

每个签发的客户端 ID 都与已验证的 issuer 和 subject 关联，即使多个注册使用相同邮箱也彼此区分。返回登录会复用该客户端 ID。授权码交换出现 `invalid_grant` 后，登录会用回调签发的客户端 ID、全新的 state、nonce 和 PKCE 重新开始一次授权。在签名身份验证通过之前，不会启用此待处理注册。第二次失败会停止登录并保留先前凭证。

凭证数据库位于 `~/.opencodereview/auth/chatgpt/accounts.json`。独立的 `host.json` 保存稳定的安装主机 ID。凭证文件采用原子替换。

在 Unix 上，凭证文件使用仅所有者可读写的 `0600` 权限，存储目录使用 `0700`。在 Windows 上，访问权限取决于从本地用户配置文件目录继承的 ACL；OpenCodeReview 不设置或验证 DACL。Go 的 [`os.Chmod`](https://pkg.go.dev/os#Chmod) 在 Windows 上只改变只读属性。请使用 ACL 将访问限制为本人账户的用户配置文件目录，在登录前检查权限，并避免共享或允许广泛访问的存储位置。目录位于用户配置文件下并不保证私密访问。

存储变更和刷新令牌轮换使用操作系统文件锁；浏览器授权在锁外运行，以便其他账户继续使用。完成时会重新读取存储，并检查原注册版本。该注册上的退出、刷新或其他已完成登录会使待处理结果失效；其他账户的更新会保留。访问令牌会在过期前使用轮换刷新令牌续期。

退出登录会尝试撤销可续期会话、清除访问令牌、刷新令牌和 ID token，并保留账户注册及主机 ID。退出后的后续登录不会发送 `id_token_hint`。如果无法确认远端撤销，CLI 会说明本地令牌已清除，并提示在 ChatGPT 设置中断开应用。

## 传输与用量

此 provider 只向 `https://api.openai.com/v1/responses` 发送请求。端点、协议和凭证覆盖均不接受。请求使用流式传输及 `store: false`，发送完整历史记录，将本地函数归入 `ocr` 命名空间，并为后续轮次保留调用 ID 和加密推理内容。只有 `response.completed` 算作成功。失败、不完整或中断的流都会停止请求，包括输出开始后收到的用量限制错误。

预览版请求字段限制为 OpenAI 支持的子集。`extra_body` 仅接受 `reasoning`、`text` 和 `prompt_cache_key`；不能替换模型、历史记录、工具、流模式或存储策略。响应仍提供 provider 原始用量数据；已有的选择性原始审查记录会保存数据流，但不包含凭证请求头。

**使用 ChatGPT 套餐：**符合条件的请求使用所选账户的 ChatGPT 套餐。[在 ChatGPT 设置中管理用量](https://chatgpt.com/settings/usage)。用量限制错误可能来自套餐整体或应用级限制；CLI 不推断重置时间。

请参阅 OpenAI 的[开源 SIWC 概览](https://developers.openai.com/siwc/token-sharing-open-source)、[登录规范](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)和[预览版限制](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)，了解当前要求。

命令详见 [CLI 参考](../cli-reference/)中的 `ocr auth` 和 `ocr llm models`，provider 设置请参阅[配置](../configuration/)。
