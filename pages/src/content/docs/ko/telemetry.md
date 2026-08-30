---
title: 텔레메트리
sidebar:
  order: 11
---

OCR은 **OpenTelemetry**를 일급으로 지원합니다. 리뷰를 한 번 돌릴 때마다 구조화된
스팬과 메트릭, 이벤트가 나옵니다. 컬렉터에 연결해 두면 "Agent가 어디에 시간을
썼는가", "어느 모델에 얼마가 들었는가", "이 실행은 왜 실패했는가"에 답하기에
충분한 데이터가 쌓입니다.

## 개요 {#overview}

텔레메트리는 **기본적으로 꺼져 있습니다**. 켜면 OCR이 다음을 내보냅니다.

- **스팬** — 파이프라인 수준의 스팬 세 가지(`review.run`, `diff.parse`,
  `subtask.execute.group.<group-key>`). 여기에 결정 지점 이벤트마다 짧게
  생겼다 사라지는 `event.*` 스팬이 하나씩 더 붙습니다.
- **메트릭** — 리뷰 소요 시간, 리뷰한 파일 수, 생성한 코멘트 수, LLM 요청·토큰·
  지연 시간, 도구 호출·지연 시간의 집계 카운트와 히스토그램.
- **이벤트** — `plan.skipped`, `token.threshold.exceeded`, `review.started`처럼
  스팬 안에서 일어나는 개별 사건.

익스포터는 두 가지를 지원합니다.

| 익스포터 | 언제 쓰나 |
|---|---|
| `console` | 개인 용도 / 디버깅. 스팬을 stdout에 보기 좋게 출력합니다. |
| `otlp` | 시스템 연동. OTLP를 지원하는 컬렉터라면 어디로든 보냅니다(Jaeger, Tempo, OTel Collector, Datadog Agent 등). |

## 텔레메트리 켜기 {#enabling-telemetry}

LLM 엔드포인트와 마찬가지로 텔레메트리도 영구 설정이나 환경 변수로 지정합니다.
둘이 충돌하면 환경 변수가 이깁니다.

### 설정 파일로 지정하기 {#config-file-approach}

```bash
ocr config set telemetry.enabled        true
ocr config set telemetry.exporter       otlp
ocr config set telemetry.otlp_endpoint  localhost:4317
ocr config set telemetry.content_logging false
```

`~/.opencodereview/config.json`에는 이렇게 남습니다.

```json
{
  "telemetry": {
    "enabled": true,
    "exporter": "otlp",
    "otlp_endpoint": "localhost:4317",
    "content_logging": false
  }
}
```

### 환경 변수로 지정하기 {#environment-variable-approach}

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # implies exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # default; http/protobuf and http/json
                                                    # are also supported
export OTEL_SERVICE_NAME=open-code-review-prod      # optional; default: open-code-review
export OCR_CONTENT_LOGGING=0                        # reserved / currently a no-op (see Content logging)
```

`OTEL_EXPORTER_OTLP_ENDPOINT`를 지정하면 `exporter=otlp`도 함께 강제됩니다.
`OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review`처럼 한 번만 돌릴 때 편합니다.

### 엔드포인트 형식 {#endpoint-format}

엔드포인트에는 `host:port`만 적어도 되고 스킴을 붙여도 됩니다. 스킴을 붙이면
전송 보안이 그에 따라 정해집니다. `http://`는 평문, `https://`는 TLS입니다.
스킴 없이 `host:port`만 적으면 TLS를 씁니다.

기본 포트는 프로토콜마다 다릅니다. gRPC는 `4317`, HTTP는 `4318`입니다.

```bash
# gRPC (default)
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# HTTP
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

HTTP 프로토콜에서는 엔드포인트가 **기준 URL**이고 그 뒤에 시그널별 경로가
붙습니다. 그래서 `http://localhost:4318`은 트레이스를
`http://localhost:4318/v1/traces`로 보냅니다. 기준 경로는 그대로 유지되므로
접두 경로 아래에서 서비스되는 백엔드에도 맞출 수 있습니다.

```bash
# traces go to http://collector:3000/api/public/otel/v1/traces
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:3000/api/public/otel
```

gRPC에는 URL 경로가 없으므로 이 이야기는 HTTP 프로토콜에만 해당합니다.

## 무엇이 나가나 {#what-gets-exported}

### 스팬 {#spans}

리뷰 한 번의 전체 스팬 트리입니다.

```
review.run
├── diff.parse
├── event.review.started                   (decision-point event)
├── subtask.execute.group.<group-key1>
│   ├── event.plan.skipped                 (when changes are below both thresholds)
│   ├── event.plan.failed                  (when plan phase errored)
│   ├── event.token.threshold.exceeded     (when prompt > 80% of max_tokens)
│   ├── main.loop                          (one span per review round)
│   └── event.subtask.error                (when the subtask errored)
├── subtask.execute.group.<group-key2>
└── …
```

`subtask.execute.group.*` 스팬은 파일마다가 아니라 리뷰한 **그룹**마다 하나씩
나옵니다. 파일은 리뷰 전에 의미 단위로 묶이기 때문입니다. 그룹 키는 그룹의 파일
경로를 정렬해 쉼표로 이은 것입니다(파일이 하나뿐인 그룹이면 경로 하나).

LLM 왕복과 도구 실행은 별도 스팬으로 **나오지 않습니다**. 아래의 메트릭에만
잡힙니다. 결정 지점 이벤트는 현재 컨텍스트에 붙은 짧은 `event.<name>` 스팬으로
발생합니다.

스팬마다 쓸모 있는 속성이 실려 있습니다.

| 스팬 | 주요 속성 |
|---|---|
| `review.run` | `error`(실행이 실패했을 때 설정됨) |
| `diff.parse` | `files.changed`, `lines.inserted`, `lines.deleted` |
| `subtask.execute.group.<group-key>` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file` |
| `main.loop` | `group.label`, `round` |
| `event.review.started` | `file.count`, `review.count`, `repo.dir` |
| `event.plan.skipped` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file`, `threshold`, `threshold.group` |
| `event.plan.failed` | `group.label`, `message` |
| `event.token.threshold.exceeded` | `group.label`, `tokens`, `max_tokens`, `round` |
| `event.subtask.error` | `group.label`, `error` |

### 메트릭 {#metrics}

OCR은 OTel 미터로 수치 메트릭을 기록합니다. 컬렉터가 뒷단에서 집계하는 카운트와
히스토그램입니다.

| 메트릭 | 종류 | 단위 | 레이블 |
|---|---|---|---|
| `ocr.review.duration_seconds` | 히스토그램 | `s` | — |
| `ocr.files_reviewed_total` | 카운터 | — | — |
| `ocr.comments_generated_total` | 카운터 | — | — |
| `ocr.llm.requests_total` | 카운터 | — | `model`, `status`(`ok` / `error`) |
| `ocr.llm.request_duration_seconds` | 히스토그램 | `s` | `model` |
| `ocr.llm.tokens_used` | 카운터 | — | `model`, `type`(지금은 언제나 `total`) |
| `ocr.tool.calls_total` | 카운터 | — | `tool.name`, `status`(`ok` / `error`) |
| `ocr.tool.execution_duration_seconds` | 히스토그램 | `s` | `tool.name` |

### 이벤트 {#events}

이벤트는 결정 지점에서 짧게 생겼다 사라지는 `event.<name>` 스팬으로 발생합니다.
전체 목록입니다.

| 이벤트 | 뜻 |
|---|---|
| `review.started` | diff를 다 읽어 리뷰할 파일이 몇 개인지 알게 됐습니다. |
| `no.files.changed` | diff를 풀어 보니 파일이 하나도 없었습니다. |
| `plan.skipped` | 그룹이 plan 임계값 둘 다에 못 미쳤습니다. 가장 큰 파일의 변경이 `PLAN_MODE_LINE_THRESHOLD`보다 적고, (파일이 2개 이상인 그룹이라면) 합계도 `PLAN_MODE_GROUP_LINE_THRESHOLD`보다 적은 경우입니다. |
| `plan.failed` | plan 단계에서 오류가 나 main 루프가 계획 없이 돌았습니다. |
| `token.threshold.exceeded` | 프롬프트 토큰이 `MAX_TOKENS`(입력 상한)의 80%를 넘어 그룹을 건너뛰었습니다. |
| `subtask.error` | 그룹별 서브태스크에서 오류가 났습니다. 스팬 상태 `Error`와 함께 나옵니다. |

리뷰 품질이 나빠지고 있다는 신호를 사용자가 알아차리기 한참 전에 잡아내는 알림
기준으로 쓰세요.

## 콘텐츠 로깅 {#content-logging}

텔레메트리는 LLM 트래픽의 **모양**(횟수, 소요 시간, 상태)만 내보낼 뿐 실제
프롬프트나 응답은 **절대** 내보내지 않습니다. OCR은 LLM 메시지 내용을 스팬이나
이벤트에 붙이려는 시도를 아예 하지 않습니다. 프로세스 밖으로 나가는 것은 위에
정리한 메트릭·이벤트 스키마뿐입니다.

`content_logging` 설정 키(그리고 이를 덮어쓰는 `OCR_CONTENT_LOGGING=1` 환경
변수)는 설정 계층에 배선만 돼 있을 뿐 지금은 프롬프트 내용을 내보내는 어떤
경로도 제어하지 **않습니다**. 예약된 플래그로 여기시면 됩니다.

LLM에 무엇을 보내고 무엇을 받았는지 들여다봐야 한다면
[세션 뷰어](../viewer/)가 읽는 로컬 JSONL 기록을 쓰세요. 이 기록은 전부
`~/.opencodereview/` 아래 디스크에만 있고 컬렉터로는 결코 보내지 않습니다.

## 레시피 {#recipes}

### 로컬 디버깅용 console 익스포터 {#console-exporter-for-local-debugging}

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

스팬이 사람이 읽을 수 있는 형태로 stdout에 출력됩니다. 오래 걸린 실행이라면
`less`로 넘겨 읽으세요.

### OTel Collector + Tempo + Prometheus {#otel-collector-with-tempo-prometheus}

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols: { grpc: { endpoint: 0.0.0.0:4317 } }

exporters:
  otlp/tempo:
    endpoint: tempo:4317
    tls: { insecure: true }
  prometheus:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    traces:  { receivers: [otlp], exporters: [otlp/tempo] }
    metrics: { receivers: [otlp], exporters: [prometheus] }
```

그다음 셸에서:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

Tempo를 열어 `service.name=open-code-review`로 검색한 뒤 아무 트레이스나 누르면
전체 스팬 트리가 보입니다.

### Datadog {#datadog}

Datadog Agent의 OTLP 리시버는 기본적으로 OTLP/gRPC로 통신합니다.

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

스팬은 APM에서 이 서비스 이름으로, LLM 메트릭은 Metrics에서 위의 레이블과 함께
보입니다.

### CI에서 돌리고 결과는 대시보드에서 보기 {#ci-run-results-in-your-dashboard}

파이프라인 단계에 환경 변수를 넣어 줍니다.

```yaml
- name: Code review
  env:
    OCR_LLM_URL: ${{ secrets.OCR_LLM_URL }}
    OCR_LLM_TOKEN: ${{ secrets.OCR_LLM_TOKEN }}
    OCR_LLM_MODEL: claude-opus-4-6
    OCR_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_COLLECTOR_URL }}
    OTEL_SERVICE_NAME: open-code-review-ci
  run: ocr review --from origin/main --to HEAD --audience agent
```

`OTEL_SERVICE_NAME`이 CI 트레이스와 개발자가 직접 돌린 트레이스를 갈라 줍니다.

## 해석 우선순위 {#resolution-priority}

OCR이 최종 텔레메트리 설정을 만들 때의 순서입니다.

1. 기본값(`enabled=false`, `exporter=console`, 엔드포인트 없음).
2. `~/.opencodereview/config.json`의 `telemetry.*` 키.
3. 환경 변수(가장 높은 우선순위이며 파일을 **덮어씁니다**).

그래서 설정에는 `telemetry.enabled=false`를 남겨 두었다가 필요할 때만
`OCR_ENABLE_TELEMETRY=1`로 실행 단위로 켤 수 있습니다.

## 샘플링과 부담 {#sampling-and-overhead}

OCR은 **전부** 내보냅니다. 샘플링 설정은 없으며 OTel 샘플링은 컬렉터가 맡을
몫입니다. 보통의 리뷰 한 번이면 이 정도가 나옵니다.

- `review.run` 스팬 1개 + `diff.parse` 스팬 1개 + 리뷰한 그룹마다
  `subtask.execute.group.<group-key>` 스팬 1개(그 아래 `plan.execute` /
  `main.loop` / `review_filter.execute` 자식 스팬 포함) + 결정 지점 이벤트마다
  짧은 `event.*` 스팬 1개.
- 파일 10개짜리 PR이면 대략 15~25개 스팬입니다. 그룹화가 파일을 잘 묶으면 더
  적고, effort 프리셋이 리뷰 라운드를 더 돌리면 더 많습니다. LLM 왕복과 도구
  호출은 메트릭 카운터만 올릴 뿐 스팬을 더 만들지는 않습니다.

내보내기는 **묶음으로 비동기** 처리되므로 텔레메트리가 리뷰 루프를 막지
않습니다. 컬렉터에 닿지 못하면 OCR은 경고를 남기고 계속 갑니다. 리뷰는 평소대로
결과를 냅니다.

## 문제 해결 {#troubleshooting}

| 증상 | 유력한 원인 |
|---|---|
| 아무것도 안 나감 | `OCR_ENABLE_TELEMETRY` / `telemetry.enabled`가 설정돼 있지 않습니다. 기본값은 **꺼짐**입니다. |
| 로컬에서는 되는데 운영에서 실패 | 프로토콜이 컬렉터와 맞는지 확인하세요. 기본값은 gRPC인데, 관리형 백엔드 상당수는 HTTP OTLP만 받습니다. `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`를 지정하고 HTTP 포트(`4317`이 아니라 `4318`)를 쓰세요. |
| 아무것도 도착하지 않는데 오류도 없음 | 익스포터는 필요할 때 만들어지므로 엔드포인트가 틀렸어도 시작 시점이 아니라 내보내는 시점에 조용히 실패합니다. 컬렉터의 액세스 로그에서 어떤 경로로 요청이 갔는지 확인하세요. HTTP 프로토콜에서는 엔드포인트가 기준 URL이고 시그널 경로가 뒤에 붙으므로 `http://host:4318`은 `http://host:4318/v1/traces`로 POST 합니다. |
| 스팬은 보이는데 메트릭이 없음 | 컬렉터에 따라 기본적으로 트레이스 파이프라인만 켜져 있습니다. 설정에 `metrics` 파이프라인을 더하세요. |
| 스팬에 프롬프트가 없음 | OCR은 프롬프트 내용을 텔레메트리에 붙이지 않습니다. [콘텐츠 로깅](#content-logging)을 참고하세요. 대신 [세션 뷰어](../viewer/)로 기록을 들여다보세요. |

## 함께 보기 {#see-also}

- [설정](../configuration/) — `telemetry.*` 이름 공간의 전체 키 레퍼런스.
- [아키텍처](../architecture/) — 각 스팬이 실제로 재는 것.
- [OpenTelemetry 문서](https://opentelemetry.io/docs/) — 컬렉터 구성과
  익스포터.
