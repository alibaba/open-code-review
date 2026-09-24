---
title: CI/CD
sidebar:
  order: 4
---

在每个 Pull Request 或 Merge Request 上运行 OCR。上游仓库提供两条现成流水线，
你复制并配置即可——一条 GitHub Actions，一条 GitLab CI。两者都是
[CLI 参考](../cli-reference/#json)中记录的核心命令的薄包装。

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
   外壳见 [JSON 输出](../cli-reference/#json)。
5. **解析 JSON** 并遍历 `comments[]`。
6. **通过 provider 的 review API 把评论回贴到 PR / MR。** 无有效行信息的条目
   （文件级发现）合并到摘要备注而非内联张贴；若内联批量 API 拒绝请求，张贴步骤也
   回退为普通摘要评论。

始终涉及两类凭据：OCR 用来生成发现的 **LLM 凭据**，以及张贴步骤用来回贴评论的
**PR/MR 写 token**。GitHub 配方通过 `GITHUB_TOKEN` 自动提供后者；GitLab 建议显式
配置 `GITLAB_API_TOKEN`，但对 fork MR 会回退使用内置 `CI_JOB_TOKEN`（它可通过
`/discussions` 发起讨论）——为可靠性推荐使用专用 token。

## 可选 CI 门禁

两种集成都支持在发布评审结果后运行 `ocr gate`，门禁**默认关闭**。通过 GitHub 的
`ocr_version` 参数或 GitLab 的 `OCR_VERSION` 变量选择包含 `ocr gate` 的 CLI
版本。启用门禁后，集成会在调用模型前检查该命令是否可用。

| 决策 | 含义 | 退出码 |
|---|---|---|
| `pass` | 所选文件均有完整评审记录，且所有已启用的检查通过。 | `0` |
| `fail` | 发现达到或超过所配置严重度阈值的问题。 | `1` |
| `inconclusive` | 缺少必要的评审记录，或无法确认所选文件已完成评审、提交版本匹配、评论工具调用成功。 | `1` |

评审前，集成将 merge base 和 head 解析为不可变的 commit ID，并将相同 ID 传给
评审和门禁。门禁会检查原始评审结果中的全部问题，包括汇总到摘要或在发布时去重的
问题。因预算限制中断、存在标为豁免的文件、未选中任何文件、缺少评审记录、
已记录的 `code_comment` 调用失败或 manifest 版本不受支持时，门禁均无法通过。

作业会先尝试发布评审结果并运行门禁，保留评审结果和诊断信息。评审执行、发布和
门禁均成功后，作业才通过。行内评论发布失败会使作业失败，即使相关问题已汇总到
摘要；最终摘要也必须确认发布。

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

### Action 参数

上游工作流通过 `uses: alibaba/open-code-review@main` 把评审委托给可复用的
composite action
（[`action.yml`](https://github.com/alibaba/open-code-review/blob/main/action.yml)）。
除上述凭据外，以下 input 用于调节评审本身——在 action 步骤的 `with:` 下传入：

| Input | 默认值 | 说明 |
|---|---|---|
| `effort` | `''` | 传给 `ocr review --effort` 的评审强度预设：`low`、`medium` 或 `high`（不区分大小写）。留空则沿用 CLI 默认值（已配置的值，否则为 medium）。需要 OCR v1.10.0 或更新版本；在更旧版本上 action 会提前以明确报错失败。 |
| `max_tokens_budget` | `''` | 传给 `ocr review --max-tokens-budget` 的 token 总量上限（输入 + 输出）。留空或 `'0'` 表示不限。每次 LLM 轮次前都会检查：已超出上限的子任务会获得最后一轮来提交发现，不再派发新的子任务，超出预算和被跳过的文件记为 `failed(budget)`，已产生的部分结果仍会发布，评审以 0 退出。 |
| `llm_reasoning_effort` | `''` | 面向支持 `reasoning_effort` 请求字段的模型（如 GLM-5.x、OpenAI reasoning 模型）的推理深度：`minimal`、`low`、`medium`、`high`、`max`（不区分大小写）。经 `llm_extra_body` 合并进请求体，因此所有已发布的 CLI 版本均可使用；`llm_extra_body` 中显式的 `reasoning_effort` 键优先于此 input。留空（默认）则不发送。仅适用于 OpenAI 兼容协议——Anthropic API 会拒绝未知请求体字段，action 在该协议下会快速失败；Anthropic 的 thinking 控制请改用 `llm_extra_body` 中的显式键。 |
| `stream_progress` | `'false'` | 设为 `'true'` 时，在工作流日志中实时显示 `[ocr]` 进度。stderr 仍会写入文件，供产物上传与评论发布使用。 |
| `gate` | `'false'` | 启用共享 CI 门禁。接受 `true` 或 `false`，不区分大小写。 |
| `fail_on_severity` | `''` | 需要 `gate: 'true'`。发现的问题达到或超过指定严重度时，作业失败。可选 `critical`、`high`、`medium` 或 `low`；留空关闭严重度检查，忽略大小写和首尾空白。 |

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

完整 input 列表见
[`action.yml`](https://github.com/alibaba/open-code-review/blob/main/action.yml)——
包括张贴模式（`sticky_summary`、`incremental`）、severity/类别路由与跨 push
检查点等。

### 启用门禁

在现有 action 步骤的 `with:` 下添加：

```yaml
gate: 'true'
fail_on_severity: high
```

门禁模式每次评审完整的 `merge-base..head` 区间，并关闭检查点读写，即使已启用
`checkpoint_range`。多次更新的 PR 因此可能消耗更多 token。评论路由和增量发布仍可使用。

`gate_exit_code` 输出记录门禁命令的退出码，关闭门禁或未执行到该步骤时为空。
`upload_artifacts: 'true'`（默认）时，可从运行的 **Artifacts** 下载
`ocr-result.json`、`ocr-stderr.log`，以及门禁运行时的 `ocr-gate.json` 和
`ocr-gate-stderr.log`。每次 action 调用使用独立的临时目录。

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

把 PR 可控的值通过 `env:` 传入，不要把 `${{ }}` 直接插值进 `run:`。GitHub 在
shell 解析该行 *之前* 就已把 `${{ }}` 做了文本替换，因此包含 shell 元字符的 PR
标题或分支名会在你的 runner 上被执行。

#### 自定义规则

用 `--rule` 传入项目专属规则文件：

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

schema 见[评审规则](../../review-rules/)。

#### 并发

默认 8 个并行子 agent——每个子任务一个。大 PR 上调低，以免触发 LLM provider 速率限制：

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

#### 将发现上传到 GitHub Code Scanning（SARIF）

`--format sarif` 会把
[SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
报告写入 stdout。把它重定向到文件，并用 CodeQL 的 `upload-sarif`
action 上传，让发现出现在 **Security → Code scanning** 下：

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

SARIF 是机器可读格式，因此 OCR 会在 stdout 上抑制进度行，`results.sarif`
只包含报告本身。`--preview` 不支持 `--format sarif`——请运行完整的
review（或 `ocr scan`）来生成报告。

### 故障排查

| 症状 | 原因 / 修复 |
|---|---|
| `Cannot find merge-base` | checkout 步骤用了浅克隆，但区间模式评审需要完整历史。上游工作流在 `actions/checkout` 上设 `fetch-depth: 0`——编辑文件时保留该设置。 |
| `Failed to parse OCR output` | `OCR_LLM_URL` 或 `OCR_LLM_AUTH_TOKEN` 缺失或错误。在 *Settings → Secrets and variables → Actions* 下复查值。 |
| 评审评论落到错误行 | PR head 或 diff 可能在评审期间发生变化。无法作为行内评论发布的问题会写入摘要。启用 `gate: 'true'` 时，行内评论发布失败仍会使作业失败。核对评审时的 head 和评论位置后重新运行。 |

> 在本次运行的产物中查看 `ocr-result.json` 和 `ocr-stderr.log`。门禁诊断信息位于
> `ocr-gate.json` 和 `ocr-gate-stderr.log`，stderr 也会显示在 "Run OpenCodeReview"
> 步骤日志中。

## GitLab CI

上游流水线位于
[`examples/gitlab_ci/.gitlab-ci.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/gitlab_ci/.gitlab-ci.yml)。

### 它做什么

- 在 `merge_requests` 事件上触发（所有 MR 事件——创建、更新、重开）。
- 在 `node:20` 镜像中运行，安装 OCR，通过 `ocr config set` 配置，再以 MR diff 模式
   运行核心命令。
- 用 `post_review.py` 解析 JSON 结果，把每个问题作为 GitLab Discussion（在 diff
  上内联）张贴，用 MR 的 `versions` 端点计算正确的 `base_sha` / `start_sha` /
  `head_sha` 以精确定位。对无法内联张贴的评论回退为普通 MR note，并以摘要 note
  收尾。

### 安装

把流水线和发布脚本放进仓库根目录：

```bash
curl -o .gitlab-ci.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/.gitlab-ci.yml
curl -o post_review.py \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/post_review.py
```

若已有 `.gitlab-ci.yml` 并想保留，把配方放到其他路径并用 `include:`
引入：

```yaml
include:
  - local: 'ci/ocr-review.gitlab-ci.yml'
```

将 `post_review.py` 保留在仓库根目录，或同步修改流水线中的脚本路径。

### 必需 CI/CD 变量

在 **Settings → CI/CD → Variables** 下设置：

| 变量 | 必需 | 掩码 | 说明 |
|---|---|---|---|
| `OCR_LLM_URL` | 是 | 否 | LLM API 端点 URL。 |
| `OCR_LLM_AUTH_TOKEN` | 是 | 是 | API 认证 token。此 CI 变量传给 `ocr config set llm.auth_token`。（OCR 的直接环境变量是 `OCR_LLM_TOKEN`，不是 `OCR_LLM_AUTH_TOKEN`。） |
| `OCR_LLM_MODEL` | 否 | 否 | 模型名。无默认——必须显式设置。 |
| `GITLAB_API_TOKEN` | 否 | 是 | 带 `api` scope 的 project / personal / group access token。可选——缺失时回退使用内置 `CI_JOB_TOKEN`（如对 fork MR）。为可靠性推荐专用 `GITLAB_API_TOKEN`。 |

> GitLab 的 8 字符下限适用于
> [Masked 变量](https://docs.gitlab.com/ci/variables/#mask-a-cicd-variable)。
> `OCR_GATE` 等非敏感配置值可设为可见（Visible）变量。流水线将 `llm.use_anthropic`
> 设为 `false`；使用 Anthropic Claude 模型时修改该行。

> 流水线启动时还会运行
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`，
> 为不支持该字段的 LLM provider 关闭 thinking-mode 请求。若你的 provider 需保留
> thinking-mode，删除该行。

> **快速 bot 命名提示。** 对 Project Access Token 和 Group Access Token，
> token 的**名字**会出现在 MR 讨论旁。把 token 命名为 `OpenCodeReview Bot`，
   > 即可让评审讨论带上品牌名，无需额外设置——当你不需要
   > [以服务账号身份发布](#post-under-a-service-account-identity)中记录的更持久
   > 服务账号设置时很方便。

### 启用门禁

在作业中添加以下变量，或在 CI/CD 设置中将其设为可见（Visible）变量：

```yaml
variables:
  OCR_GATE: 'true'
  OCR_FAIL_ON_SEVERITY: high
```

`OCR_GATE` 默认为 `false`，接受 `true` 或 `false`，不区分大小写。启用门禁时，
`OCR_FAIL_ON_SEVERITY` 接受 `critical`、`high`、`medium` 或 `low`，忽略大小写和
首尾空白；留空关闭严重度检查。关闭门禁时，该变量沿用发布脚本原有的严重度策略。
门禁模式需要对当前 MR 区间进行一次新的完整评审。

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
`--concurrency` 限制并行子 agent（默认 8，每个子任务一个）：

```yaml
script:
  - |
    ocr review --rule ./my-rules.json --concurrency 5 \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}"
```

规则 schema 见[评审规则](../../review-rules/)。

#### 固定 OCR 版本

将 `OCR_VERSION` 设为要安装的 npm 版本。门禁模式需要包含 `ocr gate` 的版本。

```yaml
variables:
  OCR_VERSION: '<version>'
```

#### 避免每次推送都复审

设置 `OCR_GATE=true` 时，每次 MR 更新都需进行新的完整评审，供门禁评估当前区间。
设置 `OCR_GATE=false` 时，可以检测 MR 中已有的 OCR 评论，跳过评审以节省 token。
后续变更将在下一次请求评审时检查。以下 Python 脚本可替代原有的 `ocr review`
调用，仅在门禁关闭且已有 OCR 评论时跳过评审：

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

流水线通过 `when: always` 保留项目内的产物：`.ocr/ocr-result.json`、
`.ocr/ocr-stderr.log`、`.ocr/ocr-gate.json` 和 `.ocr/ocr-gate-stderr.log`。
关闭门禁或未执行到该步骤时，门禁文件为空。发布统计通过 `.ocr/ocr-stats.env`
dotenv 报告提供。可在调试步骤中查看这些文件：

```yaml
script:
  - cat .ocr/ocr-result.json
  - cat .ocr/ocr-stderr.log
  - cat .ocr/ocr-gate.json
  - cat .ocr/ocr-gate-stderr.log
```

## 另见

- [CLI 参考](../cli-reference/#json)——两条流水线消费的 JSON 结构，从头写
  CI 脚本时有用。
- [配置](../../configuration/)——OCR 接受的每个环境变量与 config key。
