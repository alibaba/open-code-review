---
title: CI/CD
sidebar:
  order: 4
---

在每个 Pull Request 或 Merge Request 上运行 OCR。上游仓库提供两条现成流水线，
你复制并配置即可——一条 GitHub Actions，一条 GitLab CI。两者都是
[Direct Subprocess](../subprocess/)中核心命令的薄包装。

## CI/CD 集成如何工作

本页每条配方都遵循同一模式——下面的 GitHub Actions 与 GitLab CI 章节只是它的
具体实现：

1. **在 PR / MR 事件上触发。** 新建 pull request、更新的 merge request，或手动
   `/open-code-review` 评论触发作业。
2. **在 runner 中安装 `ocr`**，通常是
   `npm install -g @alibaba-group/open-code-review`。runner 是临时的，因此每次
   运行都发生。
3. **从 CI secret 经 `ocr config set` 配置 LLM**（端点、token、model）。没有持久
   的 `~/.opencodereview` 可回退。
4. **以区间模式运行评审**，输出机器可读，使 stdout 是干净的 JSON 外壳：

   ```bash
   ocr review \
     --from "origin/<base-branch>" \
     --to "origin/<head-branch>" \
     --format json \
     --audience agent
   ```

   `--format json` 给出可解析载荷；`--audience agent` 屏蔽进度行。每条配方消费的
   外壳见 [JSON 结构](../subprocess/#json-shape)。
5. **解析 JSON** 并遍历 `comments[]`。
6. **通过 provider 的 review API 把评论回贴到 PR / MR。** 无有效行信息的条目
   （文件级发现）合并到摘要备注而非内联张贴；若内联批量 API 拒绝请求，张贴步骤也
   回退为普通摘要评论。

始终涉及两类凭据：OCR 用来生成发现的 **LLM 凭据**，以及张贴步骤用来回贴评论的
**PR/MR 写 token**。GitHub 配方通过 `GITHUB_TOKEN` 自动提供后者；GitLab 建议显式
配置 `GITLAB_API_TOKEN`，但对 fork MR 会回退使用内置 `CI_JOB_TOKEN`（它可通过
`/discussions` 发起讨论）——为可靠性推荐使用专用 token。

## GitHub Actions

上游工作流位于
[`examples/github_actions/ocr-review.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/github_actions/ocr-review.yml)。

### 它做什么

- 在 `pull_request_target`（`opened`）**和** `issue_comment` 事件上触发，后者正文
  以 `/open-code-review` 或 `@open-code-review` 开头——后者让评审者通过在 PR 上
   评论按需重跑 OCR。（用 `pull_request_target` 而非 `pull_request`，使即便从
   fork 提交的 PR 也能用上 secret；OCR 只读 diff，不执行 PR 中的代码。）
- 通过 `npm install -g @alibaba-group/open-code-review` 安装 OCR，用
  `ocr config set` 写配置，再以分支区间模式运行核心命令。
- 解析 JSON 外壳并通过 GitHub Pull Request Review API 把每条发现作为内联评审评论
  张贴。无行信息的评论合并到摘要正文。若批量提交失败，回退为逐条张贴，并在摘要
  评论中呈现统计。

### 安装

把工作流放进你的仓库：

```bash
mkdir -p .github/workflows
curl -o .github/workflows/ocr-review.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/github_actions/ocr-review.yml
```

### 必需 secret

在 **Settings → Secrets and variables → Actions** 下设置：

| Secret | 必需 | 说明 |
|---|---|---|
| `OCR_LLM_URL` | 是 | LLM API 端点（如 `https://api.openai.com/v1/chat/completions`）。 |
| `OCR_LLM_AUTH_TOKEN` | 是 | LLM API 的认证 token。此 CI secret 传给 `ocr config set llm.auth_token`。（OCR 的直接环境变量是 `OCR_LLM_TOKEN`，不是 `OCR_LLM_AUTH_TOKEN`。） |
| `OCR_LLM_MODEL` | 否 | 模型名。无默认——必须显式设置。 |
| `OCR_LLM_USE_ANTHROPIC` | 否 | Anthropic Claude 模型设为 `true`。 |

`GITHUB_TOKEN` 自动提供；工作流声明 `pull-requests: write` 以便张贴评审评论。

> 工作流启动时还会运行
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`，
> 为不支持该字段的 LLM provider 关闭 thinking-mode 请求。若你的 provider 需保留
> thinking-mode，删除该行。

### 定制

以下都是对你刚复制的工作流文件
（`.github/workflows/ocr-review.yml`）的编辑。

#### 背景上下文

`--background` 是效果最显著的单一参数——见
[适用于所有模式的提示](../#tips-that-apply-to-every-pattern)。
传入 PR 标题（当标题遵循 `feat(auth): add OAuth2 support` 这样的语义约定时，效果
更好）：

```yaml
- name: Run OCR review
  run: |
    ocr review \
      --background "${{ github.event.pull_request.title }}" \
      --from "origin/${{ github.base_ref }}" \
      --to "origin/${{ github.head_ref }}" \
      --format json --audience agent
```

#### 自定义规则

用 `--rule` 传入项目专属规则文件：

```yaml
- name: Run OCR review
  run: |
    ocr review --rule ./my-rules.json \
      --from "origin/${{ github.base_ref }}" \
      --to "origin/${{ github.head_ref }}"
```

schema 见[评审规则](../../review-rules/)。

#### 并发

默认 8 个并行 per-file 子 agent。大 PR 上调低，以免触发 LLM provider 速率限制：

```yaml
- name: Run OCR review
  run: |
    ocr review --concurrency 5 \
      --from "origin/${{ github.base_ref }}" \
      --to "origin/${{ github.head_ref }}"
```

#### 触发模式

默认工作流在 PR **opened** 时以及以 `/open-code-review` 或
`@open-code-review` 开头的 PR 评论时触发。两种常见调整：

在更多 PR 生命周期事件上运行（如推送新 commit 时复审）：

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
```

使用不同评论关键字：

```yaml
if: |
  github.event_name == 'pull_request' ||
  (github.event_name == 'issue_comment'
    && github.event.issue.pull_request
    && startsWith(github.event.comment.body, '/review'))
```

`github.event.issue.pull_request` 检查确保评论在 PR 上而非普通 issue 上。

#### 固定 OCR 版本

默认工作流安装最新发布版本。固定：

```yaml
- name: Install OpenCodeReview
  run: npm install -g @alibaba-group/open-code-review@1.0.0
```

#### 以 GitHub App 身份发布

默认评审评论来自 `github-actions[bot]`。要以 `OpenCodeReview Bot` 这类自定义品牌的 bot 发布，把 `GITHUB_TOKEN` 换成 GitHub App installation token。

1. 在 *Settings → Developer settings → GitHub Apps → New GitHub App* **创建
   app**。禁用 webhook（此用例不需要）。在 *Repository permissions* 授予：
   - **Pull requests**：Read and write
   - **Contents**：Read-only（用于取 diff）
   - **Metadata**：Read-only（必需）

2. 从 app 设置页**生成私钥**并下载 `.pem` 文件。记下同页的 **App ID**。

3. 把 app **安装**到你想 OCR 评审的仓库。Installation ID 出现在安装后 URL 中，
   如 `https://github.com/settings/installations/12345` → ID 为 `12345`。

4. 在 *Settings → Secrets and variables → Actions* 下**添加三个 secret**：

   | Secret | 值 |
   |---|---|
   | `GITHUB_APP_ID` | App ID。 |
   | `GITHUB_APP_PRIVATE_KEY` | `.pem` 文件全部内容，含 `-----BEGIN RSA PRIVATE KEY-----` 与 `-----END RSA PRIVATE KEY-----` 行。 |
   | `GITHUB_APP_INSTALLATION_ID` | Installation ID。 |

5. 在评论张贴步骤中**生成并使用 token**：

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

评审现在会以你 app 的名字而非 `github-actions[bot]` 发布。

### 故障排查

| 症状 | 原因 / 修复 |
|---|---|
| `Cannot find merge-base` | checkout 步骤用了浅克隆，但区间模式评审需要完整历史。上游工作流在 `actions/checkout` 上设 `fetch-depth: 0`——编辑文件时保留该设置。 |
| `Failed to parse OCR output` | `OCR_LLM_URL` 或 `OCR_LLM_AUTH_TOKEN` 缺失或错误。在 *Settings → Secrets and variables → Actions* 下复查值。 |
| 评审评论落到错误行 | 通常意味着评审开始到评论张贴之间 diff 发生了偏移。张贴脚本此时回退为普通 issue 评论——无需处理。 |

> **注意。** `OCR_DEBUG` 环境变量目前在 OCR 中**未实现**——设置
> `OCR_DEBUG: "1"` 无效。此处记录以备将来接入。当前若需详细输出，可检查工作流写
> 到 `/tmp/ocr-result.json` 和 `/tmp/ocr-stderr.log` 的原始评审 JSON 和 stderr
> （见下方故障排查），或本地运行 `ocr review`。

## GitLab CI

上游流水线位于
[`examples/gitlab_ci/.gitlab-ci.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/gitlab_ci/.gitlab-ci.yml)。

### 它做什么

- 在 `merge_requests` 事件上触发（所有 MR 事件——创建、更新、重开）。
- 在 `node:20` 镜像中运行，安装 OCR，通过 `ocr config set` 配置，再以 MR diff 模式
   运行核心命令。
- 用内联 Python 脚本解析 JSON 外壳，把每条发现作为 GitLab Discussion（在 diff
  上内联）张贴，用 MR 的 `versions` 端点计算正确的 `base_sha` / `start_sha` /
  `head_sha` 以精确定位。对无法内联张贴的评论回退为普通 MR note，并以摘要 note
  收尾。

### 安装

把流水线放进仓库根：

```bash
curl -o .gitlab-ci.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/.gitlab-ci.yml
```

若已有 `.gitlab-ci.yml` 并想保留，把配方放到其他路径并用 `include:`
引入：

```yaml
include:
  - local: 'ci/ocr-review.gitlab-ci.yml'
```

### 必需 CI/CD 变量

在 **Settings → CI/CD → Variables** 下设置：

| 变量 | 必需 | 掩码 | 说明 |
|---|---|---|---|
| `OCR_LLM_URL` | 是 | 否 | LLM API 端点 URL。 |
| `OCR_LLM_AUTH_TOKEN` | 是 | 是 | API 认证 token。此 CI 变量传给 `ocr config set llm.auth_token`。（OCR 的直接环境变量是 `OCR_LLM_TOKEN`，不是 `OCR_LLM_AUTH_TOKEN`。） |
| `OCR_LLM_MODEL` | 否 | 否 | 模型名。无默认——必须显式设置。 |
| `GITLAB_API_TOKEN` | 否 | 是 | 带 `api` scope 的 project / personal / group access token。可选——缺失时回退使用内置 `CI_JOB_TOKEN`（如对 fork MR）。为可靠性推荐专用 `GITLAB_API_TOKEN`。 |

> GitLab 拒绝短于 8 字符的变量，因此流水线中 `llm.use_anthropic` 硬编码为
> `false`。要用 Anthropic Claude 模型，直接编辑脚本。

> 流水线启动时还会运行
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`，
> 为不支持该字段的 LLM provider 关闭 thinking-mode 请求。若你的 provider 需保留
> thinking-mode，删除该行。

> **快速 bot 命名提示。** 对 Project Access Token 和 Group Access Token，
> token 的**名字**会出现在 MR 讨论旁。把 token 命名为 `OpenCodeReview Bot`，
   > 即可让评审讨论带上品牌名，无需额外设置——当你不需要
   > [以服务账号身份发布](#post-under-a-service-account-identity)中记录的更持久
   > 服务账号设置时很方便。

### 定制

以下都是对你刚复制的 `.gitlab-ci.yml` 的编辑。

#### 背景上下文

把 MR 标题传给 `--background`——当标题遵循 `feat(auth): add OAuth2 support`
这样的语义约定时，效果更好：

```yaml
script:
  - |
    ocr review \
      --background "$CI_MERGE_REQUEST_TITLE" \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}" \
      --format json --audience agent
```

#### 自定义规则与并发

与 GitHub Actions 配方相同的参数——`--rule` 传项目专属规则文件，
`--concurrency` 限制并行子 agent（默认 8）：

```yaml
script:
  - |
    ocr review --rule ./my-rules.json --concurrency 5 \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}"
```

规则 schema 见[评审规则](../../review-rules/)。

#### 固定 OCR 版本

```yaml
script:
  - npm install -g @alibaba-group/open-code-review@1.0.0
```

#### 避免每次推送都复审

`only: [merge_requests]` 在**每次** MR 更新时触发，对长生命周期 MR 会消耗大量
LLM token。GitLab 无原生“仅在创建时”事件，因此推荐模式是运行评审前检测已有
OCR note，若有则跳过。把 `ocr review` 调用替换为 Python wrapper：

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

if any("OpenCodeReview" in n.get("body", "") for n in notes):
    print("OCR already reviewed this MR. Skipping to save tokens.")
    sys.exit(0)

# ...otherwise call `ocr review ...` as usual and write the JSON to
# the file the posting step expects.
```

要在此之后强制复审，从 MR 删除之前的 OCR note——下次流水线运行会看不到 OCR
note，便会继续。

#### 自托管 GitLab

无需改代码。张贴脚本读 `CI_SERVER_URL`（GitLab 在每个 runner 上自动设置），
因此开箱即可与你自己的实例通信。只需确保 `GITLAB_API_TOKEN` 由你的自托管实例签发，
而非 `gitlab.com`。

#### 以服务账号身份发布

默认评审讨论出现在 `GITLAB_API_TOKEN` 所属用户名下。改用项目级服务账号，即可获得
`OpenCodeReview Bot` 这类自定义品牌的 bot 身份。

1. 在 *Project → Settings → Service Accounts → New service account* **创建服务
   账号**。你选的名字（如 `OpenCodeReview Bot`）会出现在 MR 讨论旁。

2. 在 *Settings → Members → Invite member* **邀请它到项目**。搜索服务账号名并
   分配 `Developer` 或 `Maintainer`——两者都有张贴讨论所需权限。

3. 在 *Settings → Service Accounts →（该账号）→ Add new token* **签发 access
   token**。所需 scope：`api`。立即复制 token——GitLab 只显示一次。

4. 在 *Settings → CI/CD → Variables* **替换 token 值**——用服务账号的 token
   替换现有 `GITLAB_API_TOKEN` 值（变量名保持不变）。

讨论现在以服务账号名而非最初创建 token 的用户名发布。

### 故障排查

| 症状 | 原因 / 修复 |
|---|---|
| `Cannot find merge-base` | runner 用了浅克隆。上游流水线设 `GIT_DEPTH: 0` 强制完整克隆——编辑文件时保留该设置。 |
| 张贴时 `API error 403` | `GITLAB_API_TOKEN` 缺 `api` scope、不是项目成员，或——自托管时——由不同实例签发。以 `api` scope 重签并在 *Settings → CI/CD → Variables* 下重新添加。 |
| `Failed to parse OCR output` | `OCR_LLM_URL` 或 `OCR_LLM_AUTH_TOKEN` 错误。在 *Settings → CI/CD → Variables* 下复查值。 |
| 内联评论落到错误行 | GitLab 内联讨论要求精确 SHA 匹配；张贴脚本取 `versions` 元数据以得到正确的 `base_sha` / `start_sha` / `head_sha`。若某条发现仍无法锚定，回退为普通 MR note。 |

流水线把原始评审 JSON 写到 `/tmp/ocr-result.json`，stderr 写到
`/tmp/ocr-stderr.log`。可在 debug 步骤中 cat 它们，检查 OCR 返回了什么：

```yaml
script:
  - cat /tmp/ocr-result.json
  - cat /tmp/ocr-stderr.log
```

## 另见

- [Direct Subprocess](../subprocess/)——两条流水线消费的 JSON 结构，从头写
  CI 脚本时有用。
- [配置](../../configuration/)——OCR 接受的每个环境变量与 config key。
