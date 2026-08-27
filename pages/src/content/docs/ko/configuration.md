---
title: 설정
sidebar:
  order: 5
---

설정 파일은 `~/.opencodereview/config.json`에 있습니다. 편집하는 방법은 세 가지입니다:

- **대화형 TUI** — `ocr config provider` / `ocr config model`. 메뉴가 안내합니다.
- **커맨드라인** — `ocr config set <key> <value>`. 스크립트와 CI에 적합합니다.
- **직접 편집(권장하지 않음)** — JSON 파일을 직접 수정합니다(다음 `ocr config set` 기록 때 다시 포맷됩니다).

## 모델 설정

### 권장: 대화형 설정

```bash
ocr config provider
```

내장 프로바이더나 커스텀 프로바이더를 고르고 API 키를 입력한 뒤 모델을 선택하면, 명령이 모든 값을 설정 파일에 저장하고 `ocr llm test`를 한 번 실행해 엔드포인트를 검증합니다. 나중에 모델을 바꾸려면:

```bash
ocr config model
```

### 비대화형 설정 (CI / TUI가 없는 환경)

`ocr config set`으로 같은 설정 파일에 기록합니다:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### 내장 프로바이더

다음 프로바이더는 Base URL과 프로토콜이 미리 설정된 채 OCR에 내장되어 있습니다. 선택한 뒤 API 키만 채우면 됩니다. `providers.<name>.api_key`가 비어 있으면 OCR은 해당 환경 변수로 대체합니다.

| 이름 | 프로토콜 | Base URL | API 키 환경 변수 |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `bedrock` | anthropic-bedrock | `aws_region`에서 결정 | — (AWS 자격 증명 체인) |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `openai-responses` | openai-responses | `https://api.openai.com/v1` | `OPENAI_RESPONSES_API_KEY` |
| `gemini` | openai | `https://generativelanguage.googleapis.com/v1beta/openai` | `GEMINI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `kimi-global` | openai | `https://api.moonshot.ai/v1` | `MOONSHOT_GLOBAL_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow`  | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |
| `xai` | openai | `https://api.x.ai/v1` | `XAI_API_KEY` |

### 내장 프로바이더의 Base URL 재정의

모든 내장 프로바이더에는 미리 설정된 Base URL이 있습니다(위 표 참고). 내장 프로바이더를 다른 엔드포인트로 보내려면 `providers.<name>.url`을 설정합니다(예: 자체 호스팅 LiteLLM 게이트웨이는 미리 설정된 기본값 `http://localhost:4000/v1`에 있는 경우가 드뭅니다):

```bash
ocr config set provider                   litellm
ocr config set model                      openai/gpt-5.4
ocr config set providers.litellm.api_key  "$LITELLM_API_KEY"
ocr config set providers.litellm.url      https://gateway.internal:8000/v1
```

설정한 `url`이 미리 설정된 Base URL보다 우선합니다. `providers.<name>.url`이 비어 있으면(또는 지우면) OCR은 기본값으로 돌아가므로, 엔드포인트가 다를 때만 설정하면 됩니다.

### AWS Bedrock

`bedrock`은 `anthropic`과 같은 Messages API를 사용하지만, 요청에 API 키를 싣는 대신 표준 AWS 자격 증명 체인으로 SigV4 서명을 하며 호스트는 리전이 결정합니다. 설정할 `api_key`가 없고, 서명을 대신할 키도 받지 않습니다:

```bash
ocr config set provider                      bedrock
ocr config set model                         us.anthropic.claude-sonnet-4-6
ocr config set providers.bedrock.aws_region  us-west-2
ocr config set providers.bedrock.aws_profile example-profile
```

| 필드 | 의미 |
|---|---|
| `providers.bedrock.aws_region` | 요청을 처리할 `bedrock-runtime` 호스트의 리전. 비어 있으면 `AWS_REGION`이나 활성 프로필로 대체합니다. |
| `providers.bedrock.aws_profile` | 자격 증명을 가져올 명명 프로필. 비어 있으면 `AWS_PROFILE`이나 환경의 자격 증명 체인으로 대체합니다. |

두 필드 모두 선택 사항이며, 비워 두면 다른 AWS 도구와 마찬가지로 표준 체인이 결정합니다. 값을 고정해 두면 `AWS_PROFILE`을 먼저 내보내지 않아도 실행을 재현할 수 있는데, 기본 프로필이 다른 CI 러너에서 특히 유용합니다.

모델 식별자는 계정 **과** 리전 단위로 정해지므로 OCR이 제공하는 목록은 닫힌 집합이 아니라 출발점입니다. 목록에 없더라도 계정에서 유효한 추론 프로필 ID나 애플리케이션 추론 프로필 ARN이면 그대로 받습니다. 계정에서 무엇을 쓸 수 있는지는 `aws bedrock list-inference-profiles --region <region>`으로 확인하세요. 최신 계열에서는 `-v1:0` 같은 버전 접미사가 유효하지 않습니다.

bedrock에는 설정된 URL이 없고 호스트를 리전이 결정하므로, `ocr llm test`는 URL 대신 리전과 프로필을 표시합니다:

```
Source: provider:bedrock
Region: us-east-1
Profile: example-profile
Model:  claude-sonnet-5
✓ Connection test successful
```

Bedrock은 `llm.protocol`이나 `OCR_LLM_PROTOCOL`로는 사용할 수 **없습니다**. 이 블록은 URL 하나와 토큰 하나를 기술하는 구조라 리전이나 프로필을 담을 자리가 없고, bedrock은 이 블록이 담는 두 값 중 어느 것도 쓰지 않습니다. 그래서 이 조합은 받아들인 뒤 무시하는 대신 거부합니다.

### 커스텀 프로바이더

위 표에 없는 프로바이더 이름은 커스텀으로 취급하며 최소한 `url`과 `protocol`을 지정해야 합니다(`protocol`은 `anthropic`, `openai`, `openai-responses`, `anthropic-bedrock` 중 하나):

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

프로바이더나 모델이 OpenAI Responses API(`/v1/responses`)를 요구하면 `openai-responses`를 사용합니다:

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

`anthropic-bedrock` 프로토콜을 쓰는 커스텀 프로바이더는 호스트를 리전이 결정하므로 `url`이 필요 없고, 내장 프로바이더와 같은 AWS 필드를 받습니다. 두 번째 리전이나 프로필에 별도 항목을 주려면 이렇게 합니다:

```bash
ocr config set provider                                bedrock-eu
ocr config set custom_providers.bedrock-eu.protocol    anthropic-bedrock
ocr config set custom_providers.bedrock-eu.aws_region  eu-west-1
ocr config set custom_providers.bedrock-eu.aws_profile eu-profile
ocr config set custom_providers.bedrock-eu.model       eu.anthropic.claude-sonnet-4-6
```

`url`은 API Base URL이든 전체 `/responses` 엔드포인트든 상관없습니다. OCR이 어느 쪽이든 정규화합니다.

Ollama로 서빙하는 로컬 모델도 로컬 OpenAI 호환 엔드포인트를 가리키는 커스텀 프로바이더일 뿐입니다:

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama는 API 키를 무시하지만 커스텀 프로바이더에는 비어 있지 않은 `api_key`가 필요하므로(커스텀 프로바이더에는 환경 변수 대체가 없음) 아무 자리 표시자 값이나 설정하세요. 모델 자체는 네이티브 도구 호출을 지원해야 합니다. 모델을 고르기 전에 FAQ의 ["No tool calls parsed" (local models / Ollama)](../faq/#no-tool-calls-parsed-local-models-ollama)를 참고하세요.

### 타임아웃

LLM 요청마다 HTTP 타임아웃이 있으며 기본값은 **300초**입니다. 느린 로컬 모델(또는 큰 파일)에는 더 필요할 수 있습니다. 범위가 좁은 것부터 세 가지 설정이 있습니다:

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec` — 프로바이더별, 초 단위.
- `llm.timeout_sec` — 레거시 `llm` 섹션용, 초 단위.
- `OCR_LLM_TIMEOUT` 환경 변수 — 정수 초. 모든 해석 경로에서 설정 파일 값보다 우선합니다.

`timeout_sec` 키는 `ocr config set`이 지원하지 않으므로 `~/.opencodereview/config.json`을 직접 편집합니다:

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

### 명령으로 API 키 가져오기

키를 설정 파일에 저장하는 대신 `api_key_cmd`가 실행 시점에 비밀 관리자(1Password, `pass`, `gopass` 등)에서 가져옵니다. 앞뒤 공백을 제거한 한 줄짜리 stdout이 키가 됩니다. 레거시 `llm` 블록에서는 같은 옵션을 `auth_token_cmd`로 사용할 수 있습니다.

```bash
ocr config set providers.anthropic.api_key_cmd "op read op://dev/anthropic/api-key"
```

OS 키링도 이미 설치된 도구를 통해 같은 방식으로 동작하므로, 키가 `config.json`이 아니라 Keychain이나 Secret Service에 남습니다:

```bash
# macOS Keychain
ocr config set providers.anthropic.api_key_cmd \
  "security find-generic-password -s ocr-anthropic -w"

# Linux (Secret Service: GNOME Keyring, KWallet 등)
ocr config set providers.anthropic.api_key_cmd \
  "secret-tool lookup service ocr-anthropic"
```

우선순위: 정적 `api_key`가 항상 이깁니다(둘 다 설정하면 명령은 무시되고 경고가 출력됩니다). 그다음 `api_key_cmd`가 실행되며, 둘 다 없을 때만 OCR이 프로바이더의 환경 변수로 대체합니다.

명령은 `ocr` 실행마다 한 번 돌고 반드시 성공해야 합니다. 0이 아닌 종료 코드, 빈 출력, 여러 줄 출력, 64KiB 초과 출력은 하드 에러입니다(OCR은 조용히 대체하지 않습니다). 프롬프트에 응답하는 시간을 포함해 60초 안에 끝나야 합니다. 명령은 터미널의 stdin과 stderr를 물려받으므로 대화형 프롬프트(pinentry, Touch ID)가 표시되고 응답할 수 있습니다. 명령이 stdout 파이프를 잡고 있는 백그라운드 데몬(`gpg-agent`, 최초 실행 시의 `op` 데몬)을 남기면 자격 증명은 도착하지만 `ocr` 실행마다 그 파이프가 닫히길 기다리며 5초씩 멈춥니다. 데몬의 출력을 리다이렉트(`>/dev/null 2>&1`)하면 대기가 사라집니다.

Windows에서는 명령이 `sh`가 아니라 `cmd.exe`로 실행되므로 한쪽용으로 작성한 명령이 다른 쪽에서 그대로 돌지 않는 경우가 많습니다. `%VAR%`와 `^`는 `cmd.exe` 메타 문자이고, `$VAR` 확장과 `\` 이스케이프는 거기서 적용되지 않습니다. 따옴표로 감싼 인자는 그대로 전달되므로 `op read "op://Private/My Vault/api-key"`는 그대로 동작합니다.

값이 셸 명령으로 실행되므로 `config.json`은 신뢰된 입력입니다. 소유자를 본인으로 유지하고 다른 사용자가 쓸 수 없게 하세요(OCR은 `0600` 권한으로 기록합니다).

### 추가 재시도 상태 코드

일부 LLM 프로바이더는 일시적 오류에 비표준 4xx 상태 코드를 사용합니다. 예를 들어 레이트 리밋에 `403`이나 `400`을 반환하기도 합니다. `retry_codes`를 설정하면 OCR이 기존 SDK 재시도 메커니즘으로 이런 요청을 재시도합니다.

`retry_codes`는 정수 배열이며 `llm.retry_codes` 또는 `custom_providers.<name>.retry_codes`로 설정할 수 있습니다. `ocr config set`을 쓸 때는 코드를 쉼표로 구분해 전달합니다:

```bash
ocr config set llm.retry_codes 403,400
ocr config set custom_providers.my-gateway.retry_codes 403,400
```

4xx HTTP 상태 코드만 허용됩니다. `408`, `409`, `429`는 SDK가 이미 재시도합니다. 설정 파일에서 읽을 때 이런 중복 코드는 무시되며, `ocr config set`으로 전달하면 OCR이 경고를 출력하고 저장 값에서 제외합니다. 5xx 응답은 모두 SDK가 이미 재시도하므로 `retry_codes`에 추가할 수 없습니다.

### 프롬프트 상한

`max_tokens`는 리뷰 단위 하나에 대한 **프롬프트**(입력) 상한입니다. 그 단위는 `ocr review`에서는 파일 그룹, `ocr scan`에서는 파일 하나입니다. 내장 템플릿의 기본값은 `ocr review` 200,000토큰, `ocr scan` 58,888토큰입니다. 컨텍스트 윈도가 다른 모델에서는 `max_tokens`를 저장해 바꿉니다:

```bash
ocr config set max_tokens 400000
```

이 설정은 `ocr review`와 `ocr scan` 모두에 적용됩니다. 저장된 설정을 바꾸지 않고 일회성으로 재정의하려면 `--max-tokens`를 사용합니다:

```bash
ocr review --max-tokens 400000
ocr scan --max-tokens 400000
```

실행별 플래그가 `max_tokens`보다 우선하고, 둘 다 없으면 OCR은 내장 작업 템플릿 기본값을 사용합니다. 이 상한은 모델의 **출력** 상한(`MAX_COMPLETION_TOKENS`, 두 템플릿 모두 `16384`)이나 실행 전체 토큰 사용량을 제한하는 `--max-tokens-budget`과는 별개입니다. `ocr config unset max_tokens`로 내장 기본값을 복원합니다.

### 리뷰 강도(effort)

`effort`는 파일 그룹마다 리뷰를 몇 라운드 돌릴지 정합니다. `low` = 1라운드, `medium`(기본값) = 2라운드, `high` = 3라운드입니다. 라운드가 늘어나면 더 많은 문제를 찾지만 비용도 그만큼 늘어납니다.

```bash
ocr config set effort high
ocr config unset effort      # 기본값 medium으로 복귀
```

`--effort low|medium|high`는 한 번의 실행에 한해 저장된 값을 재정의합니다.

### 연결 검증

```bash
ocr llm test
```

### 기존 환경 변수 재사용

Claude Code의 `ANTHROPIC_*` 또는 OCR 자체의 `OCR_LLM_*` 환경 변수를 이미 설정해 두었다면 OCR이 자동으로 인식합니다. 설정 파일이 필요 없습니다.

### CC-Switch 사용

[CC-Switch](https://github.com/farion1231/cc-switch)를 [라우팅 서비스](https://www.ccswitch.io/en/docs?section=proxy&item=service)와 함께 사용 중이라면, 프로바이더 `url`을 로컬 프록시로 향하게 하면 됩니다. 다른 설정은 필요 없습니다:

```bash
# Claude (Anthropic 호환)
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / OpenAI 호환 — 해당 프로바이더의 url 키를 설정
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

`api_key`는 아무 값이어도 됩니다. `extra_body`(및 다른 프로바이더별 필드)는 평소처럼 적용됩니다.

### 벤더 고유 필드 전송

일부 프로바이더는 비표준 요청 필드를 요구합니다(Bedrock 스타일의 `thinking` 등). 소스를 고치지 않고 보내려면 모든 요청에 병합되는 `extra_body`를 사용합니다:

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

### 프롬프트 캐싱을 위한 세션 어피니티

OCR은 LLM 대화마다 리뷰 세션과 그 안의 작업 범위로 한정된 프롬프트 캐시 어피니티 키(`<session-id>-<task-type>-<scope-hash>`)를 만듭니다. 프롬프트 캐시는 접두사로 매칭되므로, 대화별 키는 실행 전체를 핫 키 하나에 고정하는 대신 늘어나는 각 대화(파일 하나의 리뷰 도구 루프 등)를 일관된 캐시 노드에 붙잡아 둡니다. 세션 ID 접두사 덕분에 프로바이더 쪽 캐시 로그를 `ocr session` 기록과 대조할 수 있습니다.

사용하려면 프로바이더가 키를 기대하는 자리의 `extra_headers`나 `extra_body` 값에 `{ocr_session_key}` 템플릿 변수를 넣습니다. OCR이 요청마다 해당 대화의 키로 치환하며, 넣지 않으면 아무것도 보내지 않습니다:

```bash
# OpenAI 스타일 요청 본문 필드로 전달(예: prompt_cache_key)
ocr config set providers.openai.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'

# HTTP 헤더로 전달(예: x-session-affinity)
ocr config set custom_providers.my-gateway.extra_headers "x-session-affinity={ocr_session_key}"
```

## 리뷰 언어 설정

`language`는 리뷰 코멘트를 어떤 언어로 쓸지 정합니다. 비어 있으면 기본값은 영어입니다:

```bash
ocr config set language 中文
ocr config set language English
```

## 관련 문서

- [빠른 시작](../quickstart/) — 최소 설정과 첫 리뷰.
- [CLI 레퍼런스](../cli-reference/) — review 명령이 받는 모든 플래그.
