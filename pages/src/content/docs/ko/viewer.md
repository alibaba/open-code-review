---
title: 세션 뷰어
sidebar:
  order: 10
---

`ocr viewer`는 지난 리뷰 세션을 브라우저에서 보기 좋게 보여 주는 작은 내장 HTTP
서버입니다. 외부 의존성은 없습니다. 리뷰할 때마다 OCR이 디스크에 남기는 JSONL
파일을 그대로 읽습니다.

## 실행하기 {#launching}

```bash
ocr viewer                        # binds localhost:5483
ocr viewer --bind 0.0.0.0 -p 8080 # bind to all interfaces on port 8080
```

기본 주소는 `localhost:5483`입니다. 서버는 포그라운드를 잡고 있으며 `Ctrl+C`로
멈춥니다. 세션은 요청이 들어올 때마다 `~/.opencodereview/sessions/`에서 그때그때
훑습니다. 그래서 다른 터미널에서 돌고 있는 리뷰도 JSONL 파일이 생기는 즉시
목록에 나타납니다.

> **DNS 리바인딩 방어.** 뷰어는 `Host` 헤더를 루프백 허용 목록(`localhost`,
> `127.0.0.1`, `::1`)과 대조합니다. 바인드 호스트를 콕 집어 준 경우(예:
> `--bind 192.168.1.10 -p 5483`)는 자동으로 목록에 더해집니다. **와일드카드**
> 바인드(`--bind 0.0.0.0` 또는 `--bind ::`)는 모든 Host를 허용하며, 모든
> 인터페이스에 노출되므로 경고를 출력합니다. 와일드카드 바인드를 제한하려면
> `OCR_VIEWER_ALLOWED_HOSTS`에 허용할 호스트 이름을 쉼표로 이어
> 지정하세요(예: `OCR_VIEWER_ALLOWED_HOSTS=box.local,192.168.1.10`).

## 페이지 세 개 {#three-pages}

뷰어에는 URL이 셋 있습니다.

| URL | 보이는 것 |
|---|---|
| `/` | 디스크에 세션이 남아 있는 저장소 목록. |
| `/r/{repo}` | 저장소 하나의 세션 목록. 최신순. |
| `/r/{repo}/{sessionID}` | 세션 하나의 상세 내용 전체. |

`{repo}`는 경로를 인코딩한 문자열입니다(구분자 `/`와 `\`는 `-`로, 콜론은 `_`로
바꿉니다. 디스크의 디렉터리 이름에 쓰는 인코딩과 같습니다). 보통은 직접
입력하지 않고 클릭해서 들어갑니다.

### `/` — 저장소 목록 {#repository-list}

세션이 하나라도 있는 저장소마다 저장소 경로, 전체 세션 수, 가장 최근 활동
시각이 보입니다.

### `/r/{repo}` — 저장소 하나의 세션 목록 {#r-repo-session-list-for-one-repo}

세션마다 ID(UUID), 브랜치 이름(OCR이 알아낼 수 있었던 경우), 리뷰 모드, 모델,
파일 수, 소요 시간, 시작 시각이 보입니다.

### `/r/{repo}/{sessionID}` — 세션 상세 {#r-repo-sessionid-session-detail}

재미있는 쪽은 상세 페이지입니다. 여기에는 다음이 나옵니다.

1. **헤더** — diff 범위, 모델, 브랜치, 전체 토큰, 실행 시간.
2. **파일 그룹** — 리뷰한 그룹마다 블록이 하나씩 있습니다. 파일은 리뷰 전에
   의미 단위로 묶이므로 한 블록이 연관된 파일 여럿을 아우를 수 있고, 제목은 그
   그룹의 파일 경로입니다. 각 그룹 안에는 "작업 종류" 레인이 다섯 가지
   있습니다.

| 작업 종류 | 언제 나타나나 |
|---|---|
| `plan_task` | plan 단계가 돌았을 때(가장 큰 파일이 `PLAN_MODE_LINE_THRESHOLD` 이상이거나, 파일 2개 이상의 합이 `PLAN_MODE_GROUP_LINE_THRESHOLD` 이상). |
| `main_task` | 모든 그룹. main 리뷰 루프이며 리뷰 라운드마다 한 번씩 돕니다. |
| `review_filter_task` | 이 그룹에 대해 리뷰 후 코멘트 필터링 패스가 돌았을 때. |
| `memory_compression_task` | active+compress 구역이 예산의 60% / 80%를 넘었을 때. |
| `re_location_task` | `code_comment`의 위치를 짚지 못해 재배치 대안이 돌았을 때. |

각 레인은 **작업 카드**가 가로로 늘어선 띠입니다. 카드 하나가 LLM 왕복 한
번에 해당합니다. 카드는 작업 종류별로 색이 달라서 어느 단계가 실행의 대부분을
차지했는지 한눈에 보입니다.

## 작업 카드에 담긴 것 {#what-s-in-a-task-card}

작업 카드를 누르면 펼쳐집니다. 카드마다 다음이 있습니다.

- **헤더 줄** — 요청 번호, 모델 배지, 토큰 배지(`P:` 프롬프트 / `C:` 완성,
  캐시가 있으면 `CR:` / `CW:` 캐시 읽기·쓰기), 소요 시간 배지, 그리고 그
  라운드가 실패했다면 오류 배지.
- **Response** — assistant의 원본 응답. 추론 / `thinking` 블록이 있으면 함께
  나옵니다.
- **Tool calls** — 도구 호출마다 인자와 돌아온 결과(접었다 펼 수 있습니다).

모델에 보낸 메시지 목록 전체와 그때 쓸 수 있었던 도구 정의는 카드 UI에
**나오지 않습니다**. 필요하다면 JSONL 기록을 직접 들여다보세요
(`llm_request` 레코드마다 있는 `messages` 필드입니다).

## 활용 사례 {#use-cases}

뷰어는 세 가지 흐름을 염두에 두고 만들었습니다.

### "모델이 왜 저렇게 말했지?" {#why-did-the-model-say-that}

터미널 출력에서 코멘트를 하나 고르고, 뷰어에서 그 파일이 속한 그룹을 찾아
`main_task` 레인을 따라 내려갑니다. **Tool calls**에 찾던 `code_comment`가
들어 있는 카드가 그 코멘트를 만든 라운드입니다. 카드의 Response에 모델의 추론이
보입니다. 모델에 실제로 전달된 프롬프트와 컨텍스트가 궁금하다면 JSONL 기록에서
그 요청 번호의 `llm_request` 레코드를 열어 `messages` 필드를 보세요.

### "이 파일은 왜 아무 말이 없지?" {#why-was-this-file-silent}

**코멘트가 없는** 파일은 모델이 *일부러* `task_done`을 불렀을 때에만 리뷰가 잘
끝난 것입니다. 레인에 도구 호출은 있는데 `code_comment`가 없다면 의도한 깨끗한
리뷰입니다. 레인이 오류 카드로 끝났다면 침묵을 가장한 실패이니 경고로 끌어
올려야 합니다.

### "압축이 무엇을 남기고 무엇을 버렸지?" {#what-did-compression-keep-drop}

`memory_compression_task` 레인에 압축 라운드가 모두 보입니다. 그 안의 Response
창에 압축 결과 요약이 있고, 입력으로 들어간 압축 구역의 XML 렌더링 결과는 해당
라운드의 `llm_request` `messages`(JSONL 기록)에 있습니다. "모델이 앞의 맥락을
잊어버렸다"는 문제를 파고들 때 유용합니다. 압축이 그 대목을 버렸는지 직접 확인할
수 있습니다.

## 디스크 저장 구조 {#storage-layout-on-disk}

뷰어가 읽는 위치는 다음과 같습니다.

```
~/.opencodereview/sessions/
└── <path-encoded-repo-path>/
    └── <session-id>.jsonl
```

JSONL 파일의 한 줄이 이벤트 하나입니다.

```json
{"type": "llm_request", "filePath": "src/foo.go", "taskType": "main_task", "request_no": 1, "messages": [{"role": "user", "content": "Review this diff…"}], "timestamp": "2026-06-02T10:15:23Z"}
{"type": "llm_response", "filePath": "src/foo.go", "taskType": "main_task", "model": "claude-sonnet-4-6", "content": "Found 2 issues…", "duration_ms": 8421, "usage": {"prompt_tokens": 12450, "completion_tokens": 320}}
{"type": "tool_call", "filePath": "src/foo.go", "tool_name": "file_read", "arguments": "{\"file_path\":\"src/foo.go\",\"start_line\":1,\"end_line\":50}", "result": "File: src/foo.go (Total lines: 220)\nIS_TRUNCATED: false\nLINE_RANGE: 1-50\n1|package foo…", "ok": true, "duration_ms": 14}
```

`filePath`에는 **그룹 키**가 들어갑니다. 파일 하나짜리 그룹이면 경로 하나이고,
여러 파일을 함께 리뷰했다면 그룹의 경로를 정렬해 쉼표로 이은 것입니다.

줄은 덧붙이기만 합니다. JSONL이 중간에 끊겨 있다면 세션이 도중에 죽었다는
뜻이며 뷰어는 남아 있는 만큼만 그려 줍니다.

디스크를 비우려면 세션 파일을 통째로 지우세요. 뷰어는 다음 요청 때 색인을 다시
만듭니다.

## 개인정보 {#privacy}

JSONL 기록에는 LLM에 보내고 받은 **모든 것**이 들어 있습니다. diff에 담겼던
코드도 예외가 아닙니다. 이 기록은 전부 여러분 컴퓨터의
`~/.opencodereview/` 안에만 있습니다. OCR은 어디에도 올리지 않습니다.

오래 보관하고 싶지 않은 코드가 리뷰에 섞인다면 방법은 둘입니다.

- 세션 파일을 주기적으로 지우거나,
- CI에서 `--audience agent --format json` 출력을 임시 파이프로 흘려보내고
  `HOME`을 임시 디렉터리로 잡아 JSONL이 아예 남지 않게 합니다.

OpenTelemetry 익스포터는 별개의 문제입니다. 내보내는 트레이스에 프롬프트 내용이
섞이지 않게 하는 방법은 [텔레메트리](../telemetry/)를 참고하세요.

## 뷰어가 답이 아닐 때 {#when-the-viewer-is-not-the-right-tool}

- 프로그램으로 후처리할 때(CI, 대시보드)는
  `ocr review --format json --audience agent`를 쓰세요. 뷰어는 기계가 아니라
  사람이 보라고 그립니다.
- 여러 세션을 한꺼번에 뒤지고 싶다면 JSONL 파일에 `jq`를 직접 쓰세요. UI에는
  아직 검색창이 없습니다.

## 함께 보기 {#see-also}

- [아키텍처](../architecture/) — 그 다섯 가지 작업 종류가 내부에서 실제로 하는
  일.
- [도구](../tools/) — `main_task` 카드에서 보게 될 도구 호출들.
