---
title: CI/CD
sidebar:
  order: 4
---

Pull Request나 Merge Request마다 OCR을 실행합니다. 업스트림 저장소는 그대로 복사해
설정만 하면 되는 파이프라인 두 벌을 제공합니다. 하나는 GitHub Actions용, 하나는
GitLab CI용입니다. 둘 다 [CLI 레퍼런스](../cli-reference/#json)에서 설명하는 핵심
명령을 감싸기만 한 얇은 래퍼입니다.

## CI/CD 연동은 어떻게 동작하나 {#how-ci-cd-integration-works}

이 페이지의 모든 레시피는 같은 흐름을 따릅니다. 아래 GitHub Actions와 GitLab CI
절은 그 흐름을 각각 구현한 것일 뿐입니다.

1. **PR / MR 이벤트에서 트리거.** 새 pull request, 갱신된 merge request, 또는 수동으로
   남긴 `/open-code-review` 코멘트가 작업을 시작합니다.
2. **러너에 `ocr` 설치.** 보통
   `npm install -g @alibaba-group/open-code-review`를 씁니다. 러너는 일회성이므로
   실행할 때마다 설치합니다.
3. **CI 시크릿으로 LLM 설정.** `ocr config set`으로 엔드포인트·토큰·모델을
   지정합니다. 러너에는 기댈 만한 `~/.opencodereview`가 남아 있지 않습니다.
4. **range 모드로 리뷰 실행.** 기계가 읽을 수 있는 형식으로 출력해 stdout이 깔끔한
   JSON 응답만 담게 합니다.

   ```bash
   ocr review \
     --from "origin/<base-branch>" \
     --to "origin/<head-branch>" \
     --format json \
     --audience agent
   ```

   `--format json`은 파싱 가능한 페이로드를 만들고, `--audience agent`는 진행 상황
   출력을 억제합니다. 모든 레시피가 소비하는 응답 구조는
   [JSON 출력](../cli-reference/#json)을 참고하세요.
5. **JSON 파싱** 후 `comments[]`를 순회합니다.
6. **코멘트를 PR / MR에 게시.** 각 플랫폼의 리뷰 API를 씁니다. 유효한 라인 정보가
   없는 항목(파일 단위 지적)은 인라인으로 달지 않고 요약 노트에 모읍니다. 인라인
   일괄 등록 API가 요청을 거부하면 게시 단계가 일반 요약 코멘트로 대체합니다.

여기에는 항상 두 종류의 자격 증명이 등장합니다. 하나는 OCR이 지적을 생성할 때 쓰는
**LLM 자격 증명**이고, 다른 하나는 게시 단계가 코멘트를 남길 때 쓰는 **PR/MR 쓰기
토큰**입니다. GitHub 레시피는 후자를 `GITHUB_TOKEN`으로 별도 설정 없이 얻습니다.
GitLab은 `GITLAB_API_TOKEN`을 명시적으로 두기를 권장하지만, fork MR에서는 내장
`CI_JOB_TOKEN`이 대체 수단으로 쓰입니다(`/discussions`로 디스커션을 남길 수
있습니다). 안정적으로 쓰려면 전용 토큰을 권장합니다.

## GitHub Actions {#github-actions}

업스트림 워크플로는
[`examples/github_actions/ocr-review.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/github_actions/ocr-review.yml)에
있습니다.

### 무엇을 하나 {#what-it-does}

- `pull_request_target`(`opened`) 이벤트와 본문이 `/open-code-review` 또는
  `@open-code-review`로 시작하는 `issue_comment` 이벤트에서 트리거합니다. 후자
  덕분에 리뷰어가 PR에 코멘트를 남겨 OCR을 다시 돌릴 수 있습니다.
  (`pull_request` 대신 `pull_request_target`을 쓰는 이유는 fork에서 올라온 PR에서도
  시크릿을 쓸 수 있게 하기 위해서입니다. OCR은 diff를 읽기만 하고 PR의 코드를
  실행하지 않습니다.)
- `npm install -g @alibaba-group/open-code-review`로 OCR을 설치하고,
  `ocr config set`으로 설정을 기록한 뒤, 브랜치 range 모드로 핵심 명령을 실행합니다.
- JSON 응답을 파싱해 각 지적을 GitHub Pull Request Review API로 인라인 리뷰 코멘트로
  게시합니다. 라인 정보가 없는 코멘트는 요약 본문에 모읍니다. 일괄 등록이 실패하면
  코멘트를 하나씩 게시하는 방식으로 대체하고, 통계를 요약 코멘트로 남깁니다.
- 실행 결과 문제가 없고 선택된 모든 항목이 성공적으로 검토된 경우, 게시 단계에서는 요약 댓글 대신 공식 **승인** 리뷰(상태가 `APPROVE`인 GitHub 풀 리퀘스트 리뷰)를 제출합니다. 아무것도 선택되지 않았거나, 코드 커버리지가 부분적이거나, 실행이 실패한 경우에는 승인이 이루어지지 않으며, 이러한 경우에는 여전히 요약 댓글이 제출됩니다. `approve_on_clean: 'false'` 액션 입력값을 사용하여 이 기능을 비활성화할 수 있습니다(기본적으로 비활성화됨).

### 설치 {#install}

워크플로 파일을 저장소에 넣으세요:

```bash
mkdir -p .github/workflows
curl -o .github/workflows/ocr-review.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/github_actions/ocr-review.yml
```

### 필요한 시크릿 {#required-secrets}

**Settings → Secrets and variables → Actions**에서 설정합니다.

| 시크릿 | 필수 | 설명 |
|---|---|---|
| `OCR_LLM_URL` | 예 | LLM API 엔드포인트(예: `https://api.openai.com/v1/chat/completions`). |
| `OCR_LLM_AUTH_TOKEN` | 예 | LLM API 인증 토큰. 이 CI 시크릿은 `ocr config set llm.auth_token`으로 전달됩니다. (OCR이 직접 읽는 환경 변수는 `OCR_LLM_AUTH_TOKEN`이 아니라 `OCR_LLM_TOKEN`입니다.) |
| `OCR_LLM_MODEL` | 아니요 | 모델 이름. 기본값이 없으므로 반드시 지정해야 합니다. |
| `OCR_LLM_USE_ANTHROPIC` | 아니요 | Anthropic Claude 모델을 쓸 때 `true`로 설정합니다. |

`GITHUB_TOKEN`은 자동으로 제공됩니다. 워크플로가 리뷰 코멘트를 남길 수 있도록
`pull-requests: write`를 선언해 두었습니다.

> 워크플로는 시작할 때
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`도
> 실행합니다. 이 필드를 지원하지 않는 LLM 프로바이더와의 호환을 위해 thinking 모드
> 요청을 끄는 설정입니다. 쓰는 프로바이더가 thinking 모드를 켠 채로 두어야 한다면
> 이 줄을 지우세요.

### 커스터마이즈 {#customization}

아래 내용은 모두 방금 복사한 워크플로 파일
(`.github/workflows/ocr-review.yml`)을 고치는 방법입니다.

#### 배경 맥락 {#background-context}

`--background`는 효과가 가장 큰 플래그입니다.
[모든 패턴에 적용되는 팁](../#tips-that-apply-to-every-pattern)을 참고하세요. PR
제목을 넘기면 됩니다(`feat(auth): add OAuth2 support`처럼 제목이 시맨틱 규약을 따를
때 특히 잘 맞습니다).

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

PR에서 제어할 수 있는 값은 `run:` 안에 `${{ }}`로 직접 넣지 말고 `env:`로
넘기세요. GitHub는 셸이 줄을 파싱하기 *전에* `${{ }}`를 텍스트로 치환하므로, 셸
메타문자가 들어간 PR 제목이나 브랜치 이름이 러너에서 그대로 실행될 수 있습니다.

#### 커스텀 규칙 {#custom-rules}

`--rule`로 프로젝트 전용 규칙 파일을 넘깁니다:

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

스키마는 [리뷰 규칙](../../review-rules/)을 참고하세요.

#### 동시 실행 수 {#concurrency}

기본적으로 파일 그룹당 하나씩, 서브 Agent 8개를 병렬로 돌립니다. 큰 PR에서 LLM
프로바이더의 요청 한도를 넘지 않으려면 값을 낮추세요:

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

#### 트리거 조건 {#trigger-pattern}

기본 워크플로는 PR **opened**와 `/open-code-review` 또는 `@open-code-review`로
시작하는 PR 코멘트에서 트리거합니다. 흔히 다음 두 가지를 조정합니다.

더 많은 PR 생명주기 이벤트에서 실행하기(예: 새 커밋이 푸시되면 다시 리뷰):

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
```

다른 코멘트 키워드 쓰기:

```yaml
if: |
  github.event_name == 'pull_request' ||
  (github.event_name == 'issue_comment'
    && github.event.issue.pull_request
    && startsWith(github.event.comment.body, '/review'))
```

`github.event.issue.pull_request` 검사는 그 코멘트가 일반 이슈가 아니라 PR에 달린
것임을 보장합니다.

#### OCR 버전 고정 {#pin-the-ocr-version}

기본 워크플로는 가장 최근에 배포된 버전을 설치합니다. 고정하려면:

```yaml
- name: Install OpenCodeReview
  run: npm install -g @alibaba-group/open-code-review@1.0.0
```

#### GitHub App 명의로 게시하기 {#post-under-a-github-app-identity}

기본적으로 리뷰 코멘트는 `github-actions[bot]` 이름으로 달립니다.
`OpenCodeReview Bot` 같은 브랜드 봇 명의로 남기려면 `GITHUB_TOKEN` 대신 GitHub App
설치 토큰을 쓰세요.

1. *Settings → Developer settings → GitHub Apps → New GitHub App*에서 **앱을
   만듭니다.** 웹훅은 이 용도에 필요 없으므로 끕니다. *Repository permissions*에서
   다음을 부여합니다.
   - **Pull requests**: Read and write
   - **Contents**: Read-only (diff를 가져오는 데 필요)
   - **Metadata**: Read-only (필수)

2. 앱 설정 페이지에서 **개인 키를 생성해** `.pem` 파일을 내려받습니다. 같은
   페이지에서 **App ID**도 확인해 둡니다.

3. OCR로 리뷰할 저장소에 **앱을 설치합니다.** 설치 후 URL에 Installation ID가
   나옵니다. 예를 들어 `https://github.com/settings/installations/12345`라면 ID는
   `12345`입니다.

4. *Settings → Secrets and variables → Actions*에서 **시크릿 세 개를 추가합니다.**

   | 시크릿 | 값 |
   |---|---|
   | `GITHUB_APP_ID` | App ID. |
   | `GITHUB_APP_PRIVATE_KEY` | `.pem` 파일의 전체 내용. `-----BEGIN RSA PRIVATE KEY-----`와 `-----END RSA PRIVATE KEY-----` 줄도 포함합니다. |
   | `GITHUB_APP_INSTALLATION_ID` | Installation ID. |

5. 코멘트 게시 단계에서 **토큰을 발급받아 사용합니다.**

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
         # ...기존 게시 스크립트...
   ```

이제 리뷰가 `github-actions[bot]`이 아니라 앱 이름으로 올라갑니다.

#### GitHub Code Scanning으로 결과 업로드 (SARIF) {#upload-findings-to-github-code-scanning-sarif}

`--format sarif`는
[SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
리포트를 stdout으로 출력합니다. 파일로 저장한 뒤 CodeQL `upload-sarif` 액션으로
업로드하면 결과가 **Security → Code scanning**에 나타납니다:

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

SARIF는 기계가 읽는 형식이므로 OCR은 stdout에 진행 상황을 출력하지 않고,
`results.sarif`에는 리포트만 담깁니다. `--preview`는 `--format sarif`를 지원하지
않습니다. 리포트를 만들려면 전체 리뷰를 실행하거나 `ocr scan`을 쓰세요.

### 문제 해결 {#troubleshooting}

| 증상 | 원인 / 해결 |
|---|---|
| `Cannot find merge-base` | 체크아웃 단계가 shallow clone을 썼는데, range 모드 리뷰에는 전체 히스토리가 필요합니다. 업스트림 워크플로는 `actions/checkout`에 `fetch-depth: 0`을 설정해 둡니다. 파일을 고치더라도 이 설정은 남겨 두세요. |
| `Failed to parse OCR output` | `OCR_LLM_URL`이나 `OCR_LLM_AUTH_TOKEN`이 없거나 잘못됐습니다. *Settings → Secrets and variables → Actions*에서 값을 다시 확인하세요. |
| 리뷰 코멘트가 엉뚱한 줄에 달림 | 대개 리뷰를 시작한 시점과 코멘트를 게시한 시점 사이에 diff가 밀린 경우입니다. 게시 스크립트가 일반 이슈 코멘트로 대체하므로 따로 할 일은 없습니다. |

> **참고.** `OCR_DEBUG` 환경 변수는 **아직 구현되어 있지 않습니다.**
> `OCR_DEBUG: "1"`을 설정해도 아무 효과가 없습니다. 나중에 연결될 경우를 대비해
> 여기 적어 둡니다. 지금 자세한 출력을 보려면 워크플로가
> `/tmp/ocr-result.json`과 `/tmp/ocr-stderr.log`에 남기는 원본 리뷰 JSON과 stderr를
> 확인하거나(아래 문제 해결 참고), `ocr review`를 로컬에서 실행하세요.

## GitLab CI {#gitlab-ci}

업스트림 파이프라인은
[`examples/gitlab_ci/.gitlab-ci.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/gitlab_ci/.gitlab-ci.yml)에
있습니다.

### 무엇을 하나 {#what-it-does}

- `merge_requests` 이벤트에서 트리거합니다(생성·수정·재오픈 등 모든 MR 이벤트).
- `node:20` 이미지에서 실행하며, OCR을 설치하고 `ocr config set`으로 설정한 뒤 MR
  diff 모드로 핵심 명령을 실행합니다.
- 인라인 Python 스크립트로 JSON 응답을 파싱해 각 지적을 GitLab 디스커션(diff 위
  인라인)으로 게시합니다. 위치를 정확히 잡기 위해 MR의 `versions` 엔드포인트로
  올바른 `base_sha` / `start_sha` / `head_sha`를 계산합니다. 인라인으로 달 수 없는
  코멘트는 일반 MR 노트로 대체하고, 마지막에 요약 노트를 남깁니다.

### 설치 {#install}

파이프라인 파일을 저장소 루트에 넣으세요:

```bash
curl -o .gitlab-ci.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/.gitlab-ci.yml
```

이미 `.gitlab-ci.yml`이 있고 그대로 두고 싶다면, 레시피를 다른 경로에 넣고
`include:`로 불러오세요:

```yaml
include:
  - local: 'ci/ocr-review.gitlab-ci.yml'
```

### 필요한 CI/CD 변수 {#required-ci-cd-variables}

**Settings → CI/CD → Variables**에서 설정합니다.

| 변수 | 필수 | 마스킹 | 설명 |
|---|---|---|---|
| `OCR_LLM_URL` | 예 | 아니요 | LLM API 엔드포인트 URL. |
| `OCR_LLM_AUTH_TOKEN` | 예 | 예 | API 인증 토큰. 이 CI 변수는 `ocr config set llm.auth_token`으로 전달됩니다. (OCR이 직접 읽는 환경 변수는 `OCR_LLM_AUTH_TOKEN`이 아니라 `OCR_LLM_TOKEN`입니다.) |
| `OCR_LLM_MODEL` | 아니요 | 아니요 | 모델 이름. 기본값이 없으므로 반드시 지정해야 합니다. |
| `GITLAB_API_TOKEN` | 아니요 | 예 | `api` 스코프를 가진 프로젝트 / 개인 / 그룹 액세스 토큰. 없으면 내장 `CI_JOB_TOKEN`이 대신 쓰입니다(예: fork MR). 안정적으로 쓰려면 전용 `GITLAB_API_TOKEN`을 권장합니다. |

> GitLab은 8자보다 짧은 변수를 거부하므로 파이프라인에서 `llm.use_anthropic`을
> `false`로 하드코딩해 두었습니다. Anthropic Claude 모델을 쓰려면 스크립트를 직접
> 고치세요.

> 파이프라인도 시작할 때
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`를
> 실행합니다. 이 필드를 지원하지 않는 LLM 프로바이더와의 호환을 위해 thinking 모드
> 요청을 끄는 설정입니다. 쓰는 프로바이더가 thinking 모드를 켠 채로 두어야 한다면
> 이 줄을 지우세요.

> **봇 이름을 빠르게 붙이는 팁.** 프로젝트 액세스 토큰과 그룹 액세스 토큰은 토큰의
> **이름**이 MR 디스커션 옆에 표시됩니다. 토큰 이름을 `OpenCodeReview Bot`으로
> 지으면 다른 설정 없이 리뷰어 이름에 브랜드를 입힐 수 있습니다.
> [서비스 계정 명의로 게시하기](#post-under-a-service-account-identity)에 적힌 더
> 견고한 서비스 계정 설정까지는 필요 없을 때 쓸 만합니다.

### 커스터마이즈 {#customization}

아래 내용은 모두 방금 복사한 `.gitlab-ci.yml`을 고치는 방법입니다.

#### 배경 맥락 {#background-context}

MR 제목을 `--background`로 넘기세요. `feat(auth): add OAuth2 support`처럼 제목이
시맨틱 규약을 따를 때 특히 유용합니다:

```yaml
script:
  - |
    ocr review \
      --background "$CI_MERGE_REQUEST_TITLE" \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}" \
      --format json --audience agent
```

#### 커스텀 규칙과 동시 실행 수 {#custom-rules-and-concurrency}

GitHub Actions 레시피와 같은 플래그를 씁니다. 프로젝트 전용 규칙 파일은 `--rule`로,
병렬 서브 Agent 수(기본값 8) 조절은 `--concurrency`로 합니다:

```yaml
script:
  - |
    ocr review --rule ./my-rules.json --concurrency 5 \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}"
```

규칙 스키마는 [리뷰 규칙](../../review-rules/)을 참고하세요.

#### OCR 버전 고정 {#pin-the-ocr-version}

```yaml
script:
  - npm install -g @alibaba-group/open-code-review@1.0.0
```

#### 푸시할 때마다 다시 리뷰하지 않기 {#avoid-re-reviewing-on-every-push}

`only: [merge_requests]`는 **모든** MR 갱신에서 트리거하므로, 오래 열려 있는 MR에서는
LLM 토큰을 많이 쓰게 됩니다. GitLab에는 "생성 시에만" 같은 이벤트가 없으므로, 리뷰를
실행하기 전에 기존 OCR 노트가 있는지 확인하고 있으면 빠져나오는 방식을 권장합니다.
`ocr review` 호출을 다음 Python 래퍼로 바꾸세요:

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

# ...노트가 없으면 평소처럼 `ocr review ...`를 호출하고, 게시 단계가 기대하는
# 파일에 JSON을 씁니다.
```

이 상태에서 다시 리뷰하고 싶다면 MR에서 기존 OCR 노트를 지우세요. 다음 파이프라인
실행에서 OCR 노트가 보이지 않으면 리뷰가 진행됩니다.

#### 자체 호스팅 GitLab {#self-hosted-gitlab}

코드를 고칠 필요가 없습니다. 게시 스크립트가 `CI_SERVER_URL`(GitLab이 모든 러너에
자동으로 설정합니다)을 읽으므로 자체 인스턴스와 그대로 통신합니다. `GITLAB_API_TOKEN`을
`gitlab.com`이 아니라 자체 호스팅 인스턴스에서 발급했는지만 확인하세요.

#### 서비스 계정 명의로 게시하기 {#post-under-a-service-account-identity}

기본적으로 리뷰 디스커션은 `GITLAB_API_TOKEN`을 소유한 사용자 이름으로 올라갑니다.
`OpenCodeReview Bot` 같은 브랜드 봇 명의로 남기려면 프로젝트 범위의 서비스 계정으로
바꾸세요.

1. *Project → Settings → Service Accounts → New service account*에서 **서비스 계정을
   만듭니다.** 여기서 정한 이름(예: `OpenCodeReview Bot`)이 MR 디스커션 옆에
   표시됩니다.

2. *Settings → Members → Invite member*에서 **프로젝트에 초대합니다.** 서비스 계정
   이름으로 검색해 `Developer`나 `Maintainer`를 부여하세요. 둘 다 디스커션을 남길
   권한이 있습니다.

3. *Settings → Service Accounts → (해당 계정) → Add new token*에서 **액세스 토큰을
   발급합니다.** 필요한 스코프는 `api`입니다. GitLab은 토큰을 한 번만 보여 주므로
   즉시 복사해 두세요.

4. *Settings → CI/CD → Variables*에서 **토큰 값을 교체합니다.** 기존
   `GITLAB_API_TOKEN` 값을 서비스 계정 토큰으로 바꾸되 변수 이름은 그대로 둡니다.

이제 디스커션이 원래 토큰을 만든 사용자가 아니라 서비스 계정 이름으로 올라갑니다.

### 문제 해결 {#troubleshooting}

| 증상 | 원인 / 해결 |
|---|---|
| `Cannot find merge-base` | 러너가 shallow clone을 썼습니다. 업스트림 파이프라인은 전체 클론을 강제하도록 `GIT_DEPTH: 0`을 설정해 둡니다. 파일을 고치더라도 이 설정은 남겨 두세요. |
| 게시할 때 `API error 403` | `GITLAB_API_TOKEN`에 `api` 스코프가 없거나, 프로젝트 멤버가 아니거나, 자체 호스팅 환경에서 다른 인스턴스가 발급한 토큰입니다. `api` 스코프로 다시 발급해 *Settings → CI/CD → Variables*에 등록하세요. |
| `Failed to parse OCR output` | `OCR_LLM_URL`이나 `OCR_LLM_AUTH_TOKEN`이 잘못됐습니다. *Settings → CI/CD → Variables*에서 값을 다시 확인하세요. |
| 인라인 코멘트가 엉뚱한 줄에 달림 | GitLab은 인라인 디스커션에 정확한 SHA 일치를 요구하므로, 게시 스크립트가 `versions` 메타데이터를 가져와 올바른 `base_sha` / `start_sha` / `head_sha`를 씁니다. 그래도 위치를 잡지 못한 지적은 일반 MR 노트로 대체됩니다. |

파이프라인은 원본 리뷰 JSON을 `/tmp/ocr-result.json`에, stderr를
`/tmp/ocr-stderr.log`에 남깁니다. OCR이 무엇을 반환했는지 보려면 디버그 단계에서
두 파일을 출력하세요:

```yaml
script:
  - cat /tmp/ocr-result.json
  - cat /tmp/ocr-stderr.log
```

## 관련 문서 {#see-also}

- [CLI 레퍼런스](../cli-reference/#json) — 두 파이프라인이 소비하는 JSON 출력
  구조입니다. CI 스크립트를 처음부터 직접 작성할 때 유용합니다.
- [설정](../../configuration/) — OCR이 인식하는 모든 환경 변수와 설정 키입니다.
