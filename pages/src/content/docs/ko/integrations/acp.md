---
title: ACP
sidebar:
  order: 6
---

ACP 호환 클라이언트의 대화에서 OCR 리뷰와 스캔을 실행할 수 있습니다. `ocr-acp` 어댑터는
Agent Client Protocol(ACP)을 통해 에디터와 로컬 OCR CLI를 연결하고, 대화에
진행 상황, 발견 사항, 파일 위치를 표시합니다. 수정안을 제안하지만 파일을 직접
편집하지는 않습니다.

ACP는 에디터와 Agent를 연결하는 개방형 프로토콜입니다. 클라이언트는 어댑터를 별도의
로컬 프로세스로 시작하고 stdio를 통해 JSON-RPC 메시지를 교환합니다. 어댑터는
요청을 검증하고 `ocr review` 또는 `ocr scan`을 실행한 뒤 OCR의 구조화된 출력을
클라이언트 업데이트로 변환합니다. OCR의 리뷰 엔진과 모델 설정은 어댑터와 독립적입니다.

## 사전 요구 사항 {#prerequisites}

- [OCR CLI](../../installation/)를 설치하고 [리뷰 모델](../../configuration/)을
  설정하세요. `ocr llm test`로 연결을 확인할 수 있습니다.
- 어댑터를 빌드하려면 `acp/` 디렉터리가 포함된 소스 체크아웃, Git, Go 1.23 이상이
  필요합니다. 상위 프로젝트의 OCR CLI를 빌드하려면 Go 1.25.5 이상이 필요합니다.
- ACP v1 stdio와 사용자 지정 Agent 실행 명령을 지원하는 클라이언트를 사용하세요. macOS arm64에서는 로컬
  자동화 테스트를 수행했습니다. Linux의 `aa7d4e7`은 [ACP CI](https://github.com/alibaba/open-code-review/actions/runs/34986645165)를
  통과했으며, 프로세스 정리, race 검사, 공식 Python ACP SDK smoke 테스트를 포함합니다. Windows는 현재 지원하지 않습니다.

어댑터는 OCR CLI와 별도로 빌드합니다. 아래 안내는 로컬 빌드를 사용하며,
OCR npm 패키지에 ACP 바이너리가 미리 설치되어 있다고 가정하지 않습니다.

## 빌드 및 클라이언트 연결 {#build-and-connect-your-client}

저장소 루트에서 실행하세요:

```bash
make -C acp build
command -v ocr
```

빌드하면 `dist/ocr-acp`가 생성됩니다. `ocr`의 경로로 출력된 절대 경로를 아래의
`--ocr-binary` 값으로 사용하세요.

클라이언트에 `OpenCodeReview` 사용자 지정 ACP Agent를 등록하고 ACP v1 stdio
전송을 선택한 뒤 다음 실행 명령을 설정하세요:

```bash
/absolute/path/open-code-review/dist/ocr-acp --ocr-binary /absolute/path/ocr
```

두 실행 파일 모두 절대 경로를 지정하고 프로젝트의 작업 디렉터리를 선택하세요.
필드 이름은 클라이언트마다 다르므로 해당 ACP 설정 안내를 참고하세요.
새 OpenCodeReview 대화에서 별도의 파싱 모델 없이 슬래시 명령을 사용할 수 있습니다.
OCR 자체의 리뷰 모델은 여전히 필요합니다.

## 리뷰 및 스캔 실행 {#run-a-review-or-scan}

OpenCodeReview 대화에서 다음 명령을 보내세요:

| 요청 | 명령 |
| --- | --- |
| 스테이징된 변경, 스테이징되지 않은 변경, 추적되지 않는 변경 리뷰 | `/review` |
| 리뷰를 최대 한 라운드 실행 | `/review --effort low` |
| 현재 체크아웃한 최신 커밋 리뷰 | `/review --commit HEAD` |
| 기존 참조 두 개 비교 | `/review --from main --to HEAD` |
| 디렉터리 스캔 | `/scan --path internal/agent` |
| 여러 경로 스캔 | `/scan --path internal/agent,internal/config` |

예시의 참조와 경로는 프로젝트에 있는 값으로 바꾸세요. 리뷰에는 Git 저장소가
필요하며, 발견 사항의 경로는 저장소 루트를 기준으로 해석합니다. 스캔은 세션의 작업
디렉터리를 사용합니다. `--effort low`는 최대 한 라운드, `medium`은 최대 두 라운드,
`high`는 최대 세 라운드의 리뷰를 실행합니다. 새로운 발견 사항이 없으면 일찍 종료할 수 있습니다.
생략하면 OCR 설정을 유지합니다.
라운드 수를 줄이면 문제를 놓칠 수 있으므로 속도와 품질을 고려해 선택하세요.

ACP 명령 제안을 지원하는 클라이언트에서 `/`를 입력하면 명령과 인수 힌트를
볼 수 있습니다. 동적 브랜치 및 경로 자동 완성은 제공하지 않습니다. 클라이언트가
로컬 파일 링크를 첨부한다면 `/scan`과 함께 보내고 `--path`는 생략하세요.
링크를 `/review` 또는 명시적인 스캔 경로와 함께 사용할 수는 없습니다.
지원하지 않거나 모호한 링크는 거부하며 스캔 범위를 자동으로 넓히지 않습니다.

어댑터가 허용하는 CLI 옵션은 정해져 있으며, 임의의 옵션을 그대로 전달할 수는 없습니다:

- 리뷰: `--commit`, `--from`, `--to`, `--effort`, `--no-filter`,
  `--background`, `--background-file`.
- 스캔: `--path`, `--batch`, `--no-plan`, `--no-dedup`, `--no-summary`,
  `--background`.

전체 변경 사항을 리뷰하려면 `/review`를 사용하세요. `--staged`는 지원하지 않습니다.
대화 명령에서는 `--format`, `--audience` 같은 출력 옵션과 `--repo`, `--model`,
`--provider` 같은 재정의 옵션을 사용할 수 없습니다. 리뷰 모델은 OCR 자체 설정을
통해 구성하세요.

## 자연어 요청 활성화 {#enable-natural-language-requests}

파싱 모델은 자연어 요청을 리뷰 또는 스캔 명령으로 변환하거나, 불명확한 부분을
질문합니다. OCR의 리뷰 모델과 별도로 설정해야 합니다. 클라이언트 자체의 모델을 선택해도
이 두 모델이 설정되지는 않습니다.

Agent의 `env` 객체에 다음 환경 설정을 추가하고, 모델과 키 자리표시자를 사용 중인
제공자의 값으로 바꾸세요:

```json
{
  "OCR_ACP_PARSER_PROVIDER": "openai",
  "OCR_ACP_PARSER_MODEL": "YOUR_TOOL_CALLING_MODEL",
  "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
}
```

이 예시는 OpenAI 호환 Chat Completions 프로토콜을 사용합니다. Anthropic Messages
프로토콜은 `anthropic`을 사용하세요. 호환 게이트웨이를 쓴다면
`OCR_ACP_PARSER_BASE_URL`에 API 기본 URL도 설정하세요. 자격 증명은 로컬 설정에
보관하거나 Agent 프로세스의 환경 변수로 전달하세요. 공유 프로젝트 설정에 넣어
커밋하지 마세요. GUI 앱은 터미널에서 내보낸 환경 변수를 상속하지 않을 수 있습니다.

기본 URL의 경로 규칙은 프로토콜마다 다릅니다:

| 프로토콜 | `OCR_ACP_PARSER_BASE_URL` 예시 | 실제 요청 주소 |
| --- | --- | --- |
| `openai` | `https://gateway.example/v1` | `https://gateway.example/v1/chat/completions` |
| `anthropic` | `https://gateway.example` | `https://gateway.example/v1/messages` |

각각 `/chat/completions` 또는 `/v1/messages`로 끝나는 전체 엔드포인트도 사용할 수 있으며,
이 경우 해당 접미사를 다시 붙이지 않습니다. Anthropic 기본 URL이 `/v1`로 끝나면
`/v1/v1/messages`가 되므로 게이트웨이의 실제 API 경로에 맞춰 지정하세요.

| 환경 변수 | 시작 플래그 | 용도 |
| --- | --- | --- |
| `OCR_ACP_PARSER_PROVIDER` | `--parser-provider` | `openai` 또는 `anthropic` |
| `OCR_ACP_PARSER_MODEL` | `--parser-model` | 도구 호출을 지원하는 모델 |
| `OCR_ACP_PARSER_BASE_URL` | `--parser-base-url` | API 기본 URL 재정의(선택 사항) |
| `OCR_ACP_PARSER_API_KEY` | 없음 | 파싱 API 키. 환경 변수로만 제공 |

시작 플래그가 대응하는 환경 변수보다 우선합니다. 파서는 OCR 설정이나 `OCR_LLM_*`
변수를 읽지 않습니다. 지원하는 프로토콜은 `openai`와 `anthropic`이며,
여기서는 `openai-responses`와 `anthropic-bedrock`을 지원하지 않습니다.

Anthropic 파싱 요청은 필수 `submit_intent` 도구 호출을 위해 Thinking을 명시적으로
비활성화합니다. OCR 리뷰 모델의 Thinking 설정은 변경하지 않습니다.

파서를 설정한 뒤 새 대화를 시작하고 다음과 같이 요청해 보세요:

```text
작업 디렉터리의 변경 사항을 리뷰해 주세요.
현재 체크아웃한 최신 커밋을 리뷰해 주세요.
internal/agent를 스캔해서 잠재적인 버그를 찾아 주세요.
```

요청이 모호하면 실행 전에 확인 질문에 답하세요. 모델은 질문과 안내를 사용자의 언어에
맞추도록 지시받습니다. 고정된 검증 메시지와 보고서 라벨은 영어로 유지됩니다.
파싱 모델을 사용할 수 없어도 슬래시 명령은 작동합니다. 파서 설정이 불완전하면
시작 시 명시적인 오류가 발생합니다.

“현재 체크아웃한 최신 커밋”은 `/review --commit HEAD`에 해당합니다. 세션은 확인을 기다리는
요청 하나만 유지하며, 여러 호환 가능한 필드를 저장할 수 있습니다. 연속된 확인 질문에서는
알려진 값을 유지하고 명시적인 새 값으로 이전 값을 덮어씁니다. 서로 다른 리뷰 유형의 필드는
섞지 않습니다. 완료 또는 취소 후 상태를 지우며 다른 세션에서 복원하지 않습니다.

## 결과 확인 및 취소 {#read-results-and-cancel-work}

**Command:** 블록에는 실행 중인 명령이 표시됩니다. **OCR progress**에는 작업
디렉터리와 길이가 제한된 일반 텍스트 로그의 마지막 부분이 표시됩니다. 펼치면 상세
내용을 확인할 수 있습니다. 로그는 이 항목 안에서 갱신되며, 대화에 반복해서 추가되지
않습니다. 처음에 펼칠지 접을지는 클라이언트가 결정합니다.

발견 사항은 최종 메시지에 한 번만 표시되며 심각도, 범주, 설명, 제안 코드가 포함됩니다.
별도의 발견 사항 카드나 이동 버튼은 생성하지 않습니다. 유효한 위치 정보가 있는
기존 파일은 본문의 링크에서 열 수 있습니다. 파일이 없거나 행이 범위를 벗어나거나 경로가
작업 루트 밖에 있으면, 이동 기능 없이 텍스트만 유지합니다. 표시와 클릭 동작은
클라이언트 버전에 따라 달라집니다.

최종 결과에는 제공 가능한 요약, 부분 실패 상세 정보, 총 토큰 수, OCR이 보고한
경과 시간이 포함됩니다. 토큰 합계는 모든 모델 요청을 포함하며, 새로 생성된 답변
토큰만 세는 값이 아닙니다.

실행 중인 요청은 클라이언트의 취소 기능으로 중지하세요. 어댑터는 작업을 취소하고
관리 대상 프로세스의 정리가 끝날 때까지 기다립니다. 파싱을 포함한 전체 턴에 시간
제한을 두려면 Agent의 `args` 배열에 `"--turn-timeout", "10m"`을 추가하세요.
기본값은 `0`이며, 전체 턴 제한 시간을 비활성화합니다. 파싱에는 별도로 기본 15초의
제한이 있습니다. 시간 초과 시 어댑터가 재시도 안내를 표시합니다.

## 문제 해결 {#troubleshooting}

| 증상 | 확인할 사항 |
| --- | --- |
| Agent가 시작되지 않음 | 실행 파일 두 개의 절대 경로와 실행 권한을 확인하세요. `ocr llm test`로 OCR 설정을 확인하고, 불완전한 파서 설정을 제거하거나 필수 값을 모두 제공하세요. |
| 슬래시 명령은 되지만 자연어는 안 됨 | 별도의 `OCR_ACP_PARSER_*` 환경을 설정하세요. 제공자, 모델, 키, 게이트웨이 URL을 확인하세요. |
| 자연어 파싱 시간 초과 | 기본 파싱 제한은 15초이며, 이 단계에서는 OCR이 아직 시작되지 않았습니다. 진단의 요청 단계, HTTP 상태, 제공자 응답으로 원인을 확인하세요. `--turn-timeout`을 늘려도 파싱 자체의 제한은 연장되지 않습니다. 재시도하거나 `/review`, `/scan`으로 파싱 모델을 건너뛸 수 있습니다. |
| 명령이나 플래그가 거부됨 | 위의 지원 옵션을 사용하세요. `/review --path` 대신 `/scan --path`를 사용하고, 커밋 또는 범위 리뷰에는 기존 Git 참조를 지정하세요. |
| 리뷰가 실패하거나 일부 결과만 반환됨 | OCR progress를 펼쳐 진단 정보를 확인하고 최종 결과의 실패 상세 정보를 읽으세요. OCR의 모델 설정과 제공자 가용성을 확인하세요. |
| 결과 위치를 클릭할 수 없음 | 파일이 작업 루트 안에 존재하고 행 범위가 유효한지 확인하세요. 텍스트만 표시하는 대체 동작은 의도된 것입니다. |
| 작업이 너무 오래 걸림 | 취소하거나 진행 상황의 상세 정보를 확인하거나 전체 턴 제한 시간을 설정하세요. 리뷰에서는 `--effort low`로 라운드 수를 줄일 수 있습니다. |
| 다시 빌드해도 이전 동작이 유지됨 | 설정된 바이너리 경로를 확인하고 새 Agent 대화를 시작하세요. 이전 프로세스를 계속 재사용한다면 Agent 또는 클라이언트를 다시 시작하세요. |

연결에 문제가 있으면 클라이언트의 ACP 로그와 어댑터의 stderr를 확인하세요.
공유하기 전에 자격 증명과 비공개 소스 정보를 제거하세요.

## 업그레이드 및 지원 범위 {#upgrade-and-support-boundaries}

소스 체크아웃을 업데이트하고 `make -C acp build`를 다시 실행한 뒤 새
OpenCodeReview 대화를 시작하세요. 실행 중인 프로세스와 이전 메시지는 다시 로드되지
않습니다. 세션 복원은 지원하지 않습니다. 새 대화는 답변을 기다리던 확인 질문의
상태를 이어받지 않고 시작합니다.

어댑터는 현재 로컬 stdio 전송을 사용합니다. HTTP 전송, 추가 작업 공간 루트,
자동 파일 편집, 이미지 또는 오디오 프롬프트는 지원하지 않습니다. 다른 ACP
클라이언트는 각각 호환성을 확인해야 합니다. 프로토콜 테스트가 통과했다고 모든
클라이언트의 화면 동작까지 확인된 것은 아닙니다.

## 관련 문서 {#see-also}

- [설정](../../configuration/) — OCR의 리뷰 모델 및 제공자 설정.
- [CLI 레퍼런스](../../cli-reference/) — 리뷰 및 스캔 옵션 상세 정보.
- [ACP 소개](https://agentclientprotocol.com/get-started/introduction) — Agent Client Protocol 공식 소개.
