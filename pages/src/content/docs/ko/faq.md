---
title: FAQ
sidebar:
  order: 14
---

자주 만나는 오류와 뜻밖의 동작, "원래 이런 건가?" 싶은 질문을 모았습니다. 찾는
문제가 여기 없다면 실행한 절차와 전체 출력을 담아
[GitHub 이슈](https://github.com/alibaba/open-code-review/issues)를 열어 주세요.

## 설정과 시작 {#configuration-startup}

### `no valid LLM endpoint configured` {#no-valid-llm-endpoint-configured}

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

OCR이 엔드포인트 해석 체인을 끝까지
돌았는데도([설정](../configuration/#reuse-existing-environment-variables))
`(URL, token, model)` 세 값이 다 갖춰진 조합을 찾지 못했다는 뜻입니다. 다음 중
하나를 택하세요.

- `ocr config set llm.url …` / `llm.auth_token …` / `llm.model …`을 실행해
  `~/.opencodereview/config.json`을 채우거나,
- `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL`을 export 하거나,
- 이미 Claude Code를 쓰고 있다면 `ANTHROPIC_BASE_URL` /
  `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_MODEL`을 export 하세요.

그런 다음 `ocr llm test`로 연결을 확인하고 리뷰를 다시 돌리세요.

### `ocr llm test`가 엉뚱한 출처를 가리킬 때 {#ocr-llm-test-shows-the-wrong-source}

OCR은 세 값이 다 갖춰진 **첫 번째** 조합을 씁니다. 마지막이 아닙니다. 그래서
설정 파일에 llm.* 키 세 개가 모두 있으면 환경 변수는 무시됩니다. 환경 변수가
이기게 하려면 설정 키를 지우거나(파일을 `rm` 하거나 직접 unset)
`ocr config set`으로 새 값으로 바꾸세요.

### `ocr llm test`에서 401 / 403이 날 때 {#401-403-from-ocr-llm-test}

토큰에 권한이 없거나, 만료됐거나, 벤더가 다릅니다. Anthropic과 OpenAI는 인증
헤더도 URL 모양도 다릅니다. `llm.use_anthropic`이 지금 가리키는 URL과 맞는지
확인하세요.

- Anthropic: URL이 `/v1/messages`로 끝나고 `use_anthropic=true`.
- OpenAI 및 OpenAI 호환: URL이 `/v1/chat/completions`로 끝나고
  `use_anthropic=false`.

### `not a git repository` {#not-a-git-repository}

`ocr review`는 현재 디렉터리를 대상으로 `git diff`를(추적되지 않는 파일에는
`git ls-files`를) 실행합니다. Git 작업 트리 안이 아니면 곧바로 종료합니다.
저장소로 `cd` 하거나 `--repo /path/to/repo`를 넘기세요.

### "No tool calls parsed" (로컬 모델 / Ollama) {#no-tool-calls-parsed-local-models-ollama}

```
[ocr] No tool calls parsed for src/foo.go, retrying...
[ocr] Max tool requests reached for src/foo.go.
```

리뷰마다 `No tool calls parsed` 재시도만 반복하다 "Max tool requests reached"로
끝나고 코멘트가 하나도 없다면, 문제는 설정이 아니라 모델입니다. OCR은 리뷰를
전적으로 도구 호출로 진행하므로 **모델이 네이티브 도구 호출(function calling)을
지원해야 합니다**. 텍스트 출력이나 `<think>` 블록 안에서 도구 호출을 *말로 흉내*
내기만 하는 모델은 프롬프트를 아무리 손봐도 OCR과 함께 쓸 수 없습니다.
`deepseek-r1`이 대표적인 예입니다. `qwen3`처럼 네이티브 도구 지원이 있는 모델은
잘 동작합니다. Ollama라면 tools 지원 태그가 붙은 모델 중에서 고르세요.
<https://ollama.com/search?c=tools>

OCR을 끼우지 않고 로컬 모델을 직접 확인해 볼 수도 있습니다.

```bash
curl http://127.0.0.1:11434/v1/chat/completions -H "Content-Type: application/json" -d '{
  "model": "qwen3:32b",
  "messages": [{"role": "user", "content": "The code below has a bug, use the report_bug tool to report it.\n\nfunc add(a, b int) int {\n  return a - b\n}"}],
  "tools": [{"type": "function", "function": {"name": "report_bug", "description": "Report a bug in the code",
    "parameters": {"type": "object", "properties": {"line": {"type": "integer"}, "description": {"type": "string"}}, "required": ["description"]}}}]
}'
```

통과: 응답에 `report_bug`를 지목하는 구조화된 `tool_calls` 배열이 들어 있습니다.
실패: "호출"이 `content` 안에 텍스트로 나옵니다.

도구를 지원하는 모델인데 로컬 하드웨어에서 응답이 느린 것이라면 모델을 바꾸지
말고 LLM 제한 시간을 늘리세요. [제한 시간](../configuration/#timeouts)을
참고하세요.

## 필터링과 규칙 {#filtering-rules}

### 제 파일이 리뷰되지 않습니다 {#my-file-isn-t-being-reviewed}

`ocr review --preview`를 돌려 보세요(LLM 비용이 들지 않습니다). 후보 파일마다
남긴 **이유** 또는 버린 **이유**가 함께 나옵니다.

```
src/foo.go              modified
src/foo_test.go         modified  (excluded: user_exclude)
node_modules/lib.js     added     (excluded: default_path)
imgs/logo.png           binary    (excluded: unsupported_ext)
```

제외 사유 다섯 가지는
[파일 필터](../review-rules/#how-files-are-filtered)의 관문과 짝을 이룹니다.

| 사유 | 해결 |
|---|---|
| `binary` | 할 일이 없습니다. 바이너리 파일에는 리뷰할 텍스트가 없습니다. |
| `user_exclude` | `exclude` 목록에서 해당 패턴을 빼세요. |
| `unsupported_ext` | 확장자를 `include` 목록에 넣어 허용 목록 관문을 건너뛰세요. |
| `default_path` | 파일을 `include`에 넣으세요. 내장 테스트 파일 제외 패턴을 덮어씁니다. |
| `deleted` | 할 일이 없습니다. 리뷰할 새 내용이 없습니다. |

### 제가 만든 규칙이 안 걸립니다 {#my-custom-rule-isn-t-firing}

`ocr rules check <file-path>`를 돌려 보세요. 어느 **계층**의 어떤
**glob 패턴**이 걸렸는지 처음부터 끝까지 보여 줍니다.

```
File: src/api/UserHandler.go
Source: Project (.opencodereview/rule.json)
Pattern: src/api/**/*.go
Rule: …
```

계층이 예상과 다르다면(프로젝트 규칙을 기대했는데 "System built-in"이 나오는
등) 대개 **선언 순서** 때문입니다. 먼저 걸리는 패턴이 이깁니다. 더 구체적인
규칙을 `rules` 배열 앞쪽으로 옮기거나 glob을 고치세요.

### 중괄호 확장이 동작하지 않습니다 {#brace-expansion-isn-t-working}

`bmatcuk/doublestar/v4`는 `{ts,tsx}` 같은 중괄호를 지원합니다. 걸리지 않는다면
공백이 섞였는지 확인하세요. `{ts, tsx}`처럼 공백이 들어가면 `tsx`에 조용히
걸리지 않습니다.

## 리뷰 {#reviews}

### 코멘트가 하나도 없는 파일, 정말 리뷰된 건가요? {#a-file-shows-zero-comments-was-it-actually-reviewed}

[세션 뷰어](../viewer/)(`ocr viewer`)를 열어 해당 세션을 찾고, 그 파일이 속한
그룹의 `main_task` 레인을 보세요(그룹 키는 파일 경로이므로 혼자 리뷰된 파일은
자기 경로 아래에 나옵니다).

- 도구 호출이 있고 `task_done`으로 끝났다면 → 깨끗하게 리뷰된 것입니다.
- 도구 호출은 있는데 루프 도중에 끝났다면 → 오류 카드가 있는지 찾아보세요.
- `main_task` 카드가 아예 없다면 → 리뷰 전에 걸러진 것입니다. 위의
  [필터링과 규칙](#filtering-rules)을 참고하세요.

### 코멘트에 `start_line: 0`, `end_line: 0`이 찍힙니다 {#comments-have-startline-0-and-endline-0}

OCR이 코멘트를 diff의 정확한 줄에 붙이지 못했다는 뜻입니다. 흔한 원인은 둘입니다.

- 모델이 `existing_code`를 diff에서 그대로 옮기지 않고 조금 바꿔 썼습니다.
  그러지 말라고 일러 두었지만 가끔 그럽니다.
- diff의 서식이 특이해서(CRLF, 탭과 공백 혼용) 슬라이딩 윈도 매칭이
  깨졌습니다.

코멘트 자체는 여전히 유효합니다. 자동으로 위치를 잡지 못했을 뿐입니다. Agent
연동 대부분(SKILL, Claude Code 플러그인)은 `existing_code` 필드를 읽어 파일에서
그 자리를 직접 찾아냅니다.

### 토큰 임계값 초과 {#token-threshold-exceeded}

```
[ocr] WARNING: prompt tokens (240000) exceed 80% of max_tokens(200000) [round 1] for group "src/big.sql"
```

그 그룹의 첫 프롬프트(규칙 + diff + 변경 파일 목록)가 모델이 답하기도 전에 이미
`MAX_TOKENS = 200000`의 80%를 넘겼다는 뜻입니다. OCR은 그 그룹을 건너뛰고 계속
갑니다. JSON 모드에서는 `warnings`에도 나옵니다.

`MAX_TOKENS`는 **프롬프트** 상한일 뿐입니다. 모델의 출력은
`MAX_COMPLETION_TOKENS`(`16384`)가 따로 제한하므로 이 경고는 언제나 입력 크기
때문에 납니다.

이렇게 줄일 수 있습니다.

- 자동 생성된 파일이라면 `exclude` 목록에 넣으세요.
- 큰 리팩터링은 작은 커밋 여러 개로 나누세요.
- 작은 커밋이 이어진다면 워크스페이스 모드로 한꺼번에 리뷰하지 말고 `--commit`
  모드를 쓰세요.

### 파일은 작은데 plan 단계가 한참 걸립니다 {#plan-phase-took-forever-and-the-file-is-small}

먼저 `ocr review --preview`를 돌려 보세요. plan 단계는 두 임계값 중 **하나만**
넘어도 돕니다.

- 그룹에서 가장 큰 파일의 변경이 `PLAN_MODE_LINE_THRESHOLD`(기본 **50**)줄
  이상이거나,
- 그룹에 파일이 2개 이상이고 그 `lines.changed` 합이
  `PLAN_MODE_GROUP_LINE_THRESHOLD`(기본 **100**)에 닿는 경우입니다.

그래서 작은 파일이라도 합치면 양이 되는 다른 변경과 함께 묶였다면 plan 패스를
거칠 수 있습니다. 의도한 동작입니다. 크거나 넓게 퍼진 diff는 계획 단계에서
이득을 봅니다. 한 번만 건너뛰고 싶다면 diff를 더 작게 만들어 돌리거나, 내장
템플릿을 잠시 고치세요(고급 사용자용이며 `--tools`를 덮어써야 합니다).

### "Max tool requests reached" {#max-tool-requests-reached}

```
[ocr] Max tool requests reached for src/foo.go.
```

모델이 `task_done`을 부르지 않은 채 도구 호출 라운드 100회
(`MAX_TOOL_REQUEST_TIMES`)를 다 썼다는 뜻입니다. 그때까지 나온 코멘트는 그대로
모아 출력합니다. 이 일이 대부분의 그룹에서 벌어진다면 원인은 대개
다음 중 하나입니다.

- 모델이 "끝나면 `task_done`을 부르라"는 지시를 잘 따르지 못합니다. 더 강한
  모델로 바꾸세요(예: Claude Opus).
- 어떤 도구가 계속 오류를 내고 모델이 계속 재시도합니다. 세션 JSONL을 보세요.
  같은 도구 결과가 반복된다면 그것이 원인입니다.
- 그룹이 정말로 크거나 맥락이 많아 100라운드로는 모자랍니다.
  `--max-tools <n>`으로 상한을 올리세요(예: `--max-tools 150`). 이 플래그는
  상한을 *올리기만* 합니다. 템플릿 기본값 `100`보다 낮은 값은 무시되고,
  1~49는 `50`으로 올려 맞추며, `0`은 템플릿 기본값을 그대로 씁니다.
- 모델이 네이티브 도구 호출을 아예 지원하지 않습니다(로컬 모델에서 흔합니다).
  ["No tool calls parsed"(로컬 모델 / Ollama)](#no-tool-calls-parsed-local-models-ollama)를
  참고하세요.

### 서브 Agent 일부가 실패했는데 종료 코드가 0입니다 {#some-sub-agents-fail-the-run-still-exits-0}

의도한 동작입니다. OCR은 그룹별 실패를 격리해 그룹 하나가 잘못됐다고 파일
20개짜리 리뷰가 통째로 죽지 않게 합니다. *하나라도* 성공했다면 전체 종료 코드는
`0`입니다. 완전히 실패한 실행(성공한 서브 Agent가 0개)만 0이 아닌 코드로
끝납니다. 어느 그룹이 실패했는지는 JSON 모드의 `warnings` 배열이나 텍스트 모드의
stderr에서 확인하세요.

### CI 실행이 로컬보다 훨씬 느립니다 {#ci-run-is-much-slower-than-local}

의심할 것은 둘입니다.

- **모델 요청 한도** — 한도에 걸리면 LLM 클라이언트가 물러섰다가 재시도합니다.
  애초에 한도에 닿지 않도록 `--concurrency`를 낮추세요(예: `4`).
- **콜드 캐시** — 프로바이더가 프롬프트 캐싱을 지원한다면 배포 직후 첫 실행은
  그 이득을 보지 못합니다. 같은 구간의 이후 실행은 더 빠릅니다.

## 출력과 연동 {#output-integration}

### `--audience agent`인데도 진행 상황이 찍힙니다 {#audience-agent-still-has-progress-lines}

**stderr**를 보고 있는 것은 아닌지 확인하세요. 진행 메시지는 경우에 따라
stderr로 나갑니다(경고, 오류). `--audience agent`가 보장하는 깨끗한 stdout은
*파서가 읽기 좋다*는 뜻입니다. 전부 없애려면
`ocr review --audience agent 2>/dev/null`처럼 리다이렉트하세요.

### JSON 출력이 `{ "files_reviewed": 0, "comments": [] }`입니다 {#json-output-is-filesreviewed-0-comments}

워크스페이스에 대상 파일이 없었다는 뜻입니다. 의도한 모양입니다. 이렇게 명시해야
호출하는 쪽이 "리뷰할 것이 없었다"와 "리뷰한 파일에서 지적을 찾지 못했다"를
구분할 수 있습니다. 코멘트가 0건인 평범한 리뷰라면 대신 빈 배열 `[]`이 나옵니다.

### 세션 JSONL은 어디에 있나요? {#where-do-session-jsonls-live}

```
~/.opencodereview/sessions/<path-encoded-repo-path>/<session-id>.jsonl
```

저장소 경로는 `/`와 `\`를 `-`로, `:`를 `_`로 바꿔 인코딩합니다(예:
`/Users/foo/my-repo` → `Users-foo-my-repo`). 세션은 `ocr viewer`로 둘러보세요.
기록을 지우려면 디렉터리를 지우면 됩니다. OCR은 다음 실행 때 인코딩된 경로를
다시 만듭니다.

## 성능과 비용 {#performance-cost}

### 토큰이 어디에 얼마나 쓰였는지 어떻게 알 수 있나요? {#how-can-i-tell-what-tokens-cost-what}

텔레메트리를 켜세요.

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review
```

LLM 호출에는 별도 스팬이 생기지 않고 메트릭으로 기록됩니다.
`ocr.llm.tokens_used`(카운터, 레이블 `model` + `type`),
`ocr.llm.requests_total`(카운터, 레이블 `model` + `status`),
`ocr.llm.request_duration_seconds`(히스토그램, 레이블 `model`)를 보세요. console
익스포터는 이 집계를 그 자리에 출력합니다. 대시보드로 보려면 OTLP 익스포터로
바꿔 메트릭 스택으로 보내세요. [텔레메트리](../telemetry/)를 참고하세요.

### 리뷰 비용이 왜 이렇게 비싼가요? {#why-are-my-reviews-so-expensive}

흔히 쓰는 조절 수단입니다.

- effort 프리셋이 그룹마다 리뷰를 몇 라운드 돌릴지 정합니다. `low` = 1,
  `medium`(기본값) = 2, `high` = 3입니다. 비용은 대체로 라운드 수에 비례하므로
  싸게 돌리고 싶다면 `--effort low`가 가장 큰 수단이고 `--effort high`가 가장
  비쌉니다.
- plan 단계는 가장 큰 파일이 50줄 이상인 그룹, 또는 파일 2개 이상의 합이 100줄
  이상인 그룹에서 켜집니다. 그룹마다 LLM 호출이 한 번 더 듭니다. 임계값을
  낮추면 비용이 줄고, 올리면 작은 PR이 빨라집니다.
- `MAX_TOOL_REQUEST_TIMES = 100`은 넉넉한 값입니다. 라운드를 다 쓰는 모델은 3
  라운드에 끝내는 모델보다 대화가 길어져(토큰이 늘어) 비쌉니다. 강한 모델일수록
  대체로 빨리 끝냅니다. 반대로 "max tool requests reached"를 피하려고
  `--max-tools`로 상한을 올렸다면 그룹당 비용도 대략 그에 비례해 늘어난다고
  보시면 됩니다.
- 메모리 압축도 LLM 호출입니다. 긴 서브태스크는 리뷰 라운드에 더해 압축 라운드
  비용도 냅니다.
- 의미 기반 그룹화는 실행마다 작은 LLM 호출을 하나 더합니다. 파일 메타데이터
  (경로, 상태, 추가·삭제 줄 수)만 볼 뿐 diff 내용은 보지 않으므로 저렴하고,
  연관된 파일을 파일마다 따로가 아니라 함께 리뷰하게 해 대개 값어치를 합니다.

### LLM 호출을 어떻게 줄이나요? {#how-do-i-reduce-llm-calls}

- `include` 목록을 두어 관심 없는 파일은 OCR이 리뷰하지 않게 하세요.
- 계정에 버스트 요금제가 걸려 있다면 `--concurrency`를 낮추세요.
- `--background`를 넘기세요. 맥락을 앞에서 잘 주면 모델이 `file_read` /
  `code_search` 왕복 없이 끝내기도 합니다.

## 개인정보와 보안 {#privacy-security}

### OCR이 제 코드를 어딘가로 보내나요? {#does-ocr-send-my-code-anywhere}

OCR은 여러분의 **diff**(그리고 읽기 도구가 가져온 코드 조각)를 여러분이 설정한
LLM 엔드포인트로 보냅니다. 그 밖에는 아무것도 컴퓨터를 떠나지 않습니다. 세션
JSONL과 규칙 파일은 로컬에만 있습니다.

텔레메트리를 켰다면, `content_logging` 플래그는 설정 계층에 배선만 돼 있고 지금은
어떤 코드 경로도 제어하지 **않습니다**. 값이 무엇이든 프롬프트와 응답 내용은
컬렉터로 내보내지 않습니다. 예약된 플래그로 여기시고 운영 환경에서는 `false`로
두세요. 자세한 내용은 [텔레메트리](../telemetry/#content-logging)를 참고하세요.

### LLM에 보내기 전에 비밀 값을 가릴 수 있나요? {#can-i-redact-secrets-before-they-re-sent-to-the-llm}

내장 기능은 없습니다. 권장하는 방식은 이렇습니다.

1. 비밀 값을 저장소에 커밋하지 마세요(늘 하는 이야기입니다).
2. 민감한 정보가 들어 있다고 알려진 파일은 `exclude`에 넣으세요.
3. `git diff --no-textconv` 필터나 커밋 전 가리기를 써서 비밀 값이 diff에
   섞이지 않게 하세요.

"가리기 규칙" 기능은 로드맵에 있습니다.
[이슈 트래커](https://github.com/alibaba/open-code-review/issues)에서 진행 상황을
지켜보세요.

## 기타 {#misc}

### 변경 로그는 어디에 있나요? {#where-s-the-changelog}

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)에
있습니다. 릴리스마다 Conventional Commits에서 생성한 노트가 붙습니다.

### Git이 아닌 버전 관리 시스템도 지원하나요? {#does-ocr-support-non-git-vcs}

지원하지 않습니다. diff 프로바이더가 `git`을 직접 실행합니다. SVN이나 Mercurial
등을 쓰려면 프로바이더를 새로 만들어야 합니다. Hg 지원 이슈는
[여기](https://github.com/alibaba/open-code-review/issues)에 열려 있습니다.

### 바이너리 이름은 `opencodereview`인데 CLI는 왜 `ocr`인가요? {#why-is-the-binary-called-opencodereview-but-the-cli-is-ocr}

릴리스에 올라가는 정적 바이너리는 프로젝트 이름을 따 `opencodereview`이고, NPM
래퍼는 쓰기 편하도록 `ocr`이라는 이름으로 설치합니다. 소스에서 빌드하면
`dist/opencodereview`가 나옵니다. `$PATH`에 `ocr`로 복사해 두세요.

### 어떻게 제거하나요? {#how-do-i-uninstall}

```bash
npm uninstall -g @alibaba-group/open-code-review        # NPM install
sudo rm /usr/local/bin/ocr                              # binary install
rm -rf ~/.opencodereview                                # all state
```

OCR은 `~/.opencodereview` 바깥에 아무것도 쓰지 않습니다(NPM으로 내려받는
바이너리는 예외). 그러니 이 디렉터리를 지우면 기록과 설정, 사용자별 규칙이 모두
사라집니다.

## 함께 보기 {#see-also}

- [설정](../configuration/) — LLM 엔드포인트 해석과 설정 키.
- [리뷰 규칙](../review-rules/) — 파일 필터와 규칙 해석 체인.
- [세션 뷰어](../viewer/) — 지난 리뷰 세션 들여다보기.
- [텔레메트리](../telemetry/) — 토큰 사용량과 LLM 메트릭.
