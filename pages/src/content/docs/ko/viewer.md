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
ocr viewer                       # start and open the browser
ocr viewer --addr :3000          # bind to all interfaces on port 3000
ocr viewer --addr 0.0.0.0:8080   # bind on all interfaces
ocr viewer --open=never          # just print the URL
ocr viewer --open=always         # force it when auto declines (piped output, WSL)
```

기본 주소는 `localhost:5483`입니다. 서버는 포그라운드를 잡고 있으며 `Ctrl+C`로
멈춥니다. 세션은 요청이 들어올 때마다 `~/.opencodereview/sessions/`에서 그때그때
훑습니다. 그래서 다른 터미널에서 돌고 있는 리뷰도 JSONL 파일이 생기는 즉시
목록에 나타납니다.

> **DNS 리바인딩 방어.** 뷰어는 `Host` 헤더를 루프백 허용 목록(`localhost`,
> `127.0.0.1`, `::1`)과 대조합니다. 바인드 호스트를 콕 집어 준 경우(예:
> `--addr 192.168.1.10:5483`)는 자동으로 목록에 더해지지만 **와일드카드**
> 바인드(`:3000`, `0.0.0.0`, `::`)는 그렇지 않습니다. 이때 LAN IP나 호스트
> 이름으로 UI에 접근하면 `forbidden host`가 돌아옵니다. 와일드카드 바인드를
> 열려면 `OCR_VIEWER_ALLOWED_HOSTS`에 허용할 호스트 이름을 쉼표로 이어
> 지정하세요(예: `OCR_VIEWER_ALLOWED_HOSTS=box.local,192.168.1.10`).

## 브라우저 열기 {#opening-the-browser}

서버가 수신을 시작하면 `ocr viewer`가 해당 URL을 기본 브라우저에서 엽니다. 이
동작은 `--open`으로 제어하며, 전역 `--color`와 같은 세 가지 값을 받습니다.

| 값 | 동작 |
|---|---|
| `auto`(기본값) | 동작할 가능성이 높을 때만 엽니다 — 아래 참조. |
| `always` | 조건 없이 엽니다. `auto`가 열지 않지만 실제로는 브라우저에 닿을 수 있는 경우 — 출력이 파이프될 때, 디스플레이가 없는 WSL 등 — 에 사용하세요. |
| `never` | URL만 출력하고 그 외에는 아무것도 하지 않습니다. |

`auto` 모드에서는 다음 경우에 브라우저를 열지 **않습니다**.

- stdout이 터미널이 아님 — 출력이 파이프되거나 리다이렉트됨
- `SSH_CONNECTION`이 설정되어 있고 **또한** 전달된 디스플레이가 없음 — 원격
  호스트에서 열 곳이 없음. `ssh -X` / `ssh -Y`는 `DISPLAY`를 설정하므로 억제되지
  않습니다
- Linux에서 `DISPLAY`와 `WAYLAND_DISPLAY`가 모두 비어 있음 — 디스플레이 서버 없음

그 이유는 ready 줄 끝에 덧붙습니다. 의도적으로 억제된 자동 열기가 고장난 것으로
보이는 일은 없습니다.

```
Viewer ready: http://localhost:5483 (browser not opened: no DISPLAY or WAYLAND_DISPLAY)
```

Unix에서는 `$BROWSER`를 먼저 시도합니다. 콜론으로 구분된 명령 목록이며, 각
항목은 URL을 뜻하는 `%s` 자리표시자를 포함하거나 URL을 마지막 인자로 받습니다.
그 외에는 플랫폼 기본 명령이 실행됩니다 — macOS는 `open`, Linux와 BSD는
`xdg-open`, Windows는 `rundll32`. 브라우저를 열지 못하면 stderr에 경고만 남기고
치명적으로 처리하지 않습니다. 어느 쪽이든 서버는 계속 서비스합니다.

## 페이지 네 개 {#four-pages}

뷰어에는 URL이 넷 있습니다.

| URL | 보이는 것 |
|---|---|
| `/` | 디스크에 세션이 남아 있는 저장소 목록. |
| `/r/{repo}` | 저장소 하나의 세션 목록. 최신순. |
| `/r/{repo}/{sessionID}` | 세션 하나의 상세 내용 전체. |
| `/r/{repo}/compare` | 저장소 하나의 세션 두 개를 비교. |

`{repo}`는 경로를 인코딩한 문자열입니다(구분자 `/`와 `\`는 `-`로, 콜론은 `_`로
바꿉니다. 디스크의 디렉터리 이름에 쓰는 인코딩과 같습니다). 보통은 직접
입력하지 않고 클릭해서 들어갑니다.

### `/` — 저장소 목록 {#repository-list}

세션이 하나라도 있는 저장소마다 저장소 경로, 전체 세션 수, 가장 최근 활동
시각이 보입니다.

### `/r/{repo}` — 저장소 하나의 세션 목록 {#r-repo-session-list-for-one-repo}

세션마다 ID(UUID), 브랜치 이름(OCR이 알아낼 수 있었던 경우), 리뷰 모드, 모델,
파일 수, 소요 시간, 시작 시각, 그리고 바로 이전(더 오래된) 세션으로 가는
`compare` 링크가 보입니다.

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

### `/r/{repo}/compare` — 세션 두 개 비교 {#r-repo-compare-compare-two-sessions}

`ocr session compare` 가 출력하는 것과 같은 네 갈래를 페이지로 보여 줍니다.
세션 목록에 **Compare** 열이 생겼고, 각 행은 바로 이전(더 오래된) 세션과의
비교로 연결됩니다. 따라서 맨 위 행은 직전 실행 이후 무엇이 달라졌는지를
보여 줍니다. 가장 오래된 행은 비교할 이전 실행이 없으므로 `-` 를 표시합니다.

지적 사항은 네 갈래로 나뉩니다:

| 갈래 | 의미 |
|---|---|
| New | 나중 실행만 보고했다. |
| Persisting | 두 실행 모두 보고했다. |
| Resolved | 이전 실행만 보고했고, 나중 실행이 그 파일을 실제로 검토했다. |
| Not reviewed | 이전 실행만 보고했고, 나중 실행은 그 파일을 아예 보지 않았다. 아무도 다시 확인하지 않았으므로 해결된 것이 아니다. |

페이지는 CLI 와 한 가지가 다릅니다. CLI 는 비어 있는 갈래를 건너뛰지만 페이지는
항상 네 갈래를 모두 표시합니다. `Resolved (0)` 자체가 답이고, 아무 말 없이
사라진 구획은 페이지가 깨진 것처럼 보이기 때문입니다.

실행 매니페스트보다 오래된 세션은 커버리지를 기록하지 않았으므로, 짝이 맞지
않는 지적은 모두 Not reviewed 가 아니라 Resolved 로 들어갑니다.

다른 조합을 비교하려면 질의 문자열을 직접 고치면 됩니다:
`/r/{repo}/compare?before=<더 오래된 세션 id>&after=<더 새로운 세션 id>`.
두 id 모두 URL 의 저장소에 속해 있어야 합니다.

두 실행의 검토 모드가 다르면 `ocr session compare` 와 같은 경고가 표시됩니다.
같은 파일들을 본 것이 아닐 수 있으므로 갈래를 그대로 비교할 수 없습니다.

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

## 리뷰 코멘트 {#review-comments}

작업 레인 아래에서 세션 페이지는 이번 리뷰가 만들어낸 모든 발견을
**코멘트 카드**로 파일별로 묶어 보여 줍니다. 코멘트 본문과, 있을 때는 기존
코드 / 제안 코드, 심각도 / 카테고리 배지가 표시됩니다. 필터 바의 칩으로
심각도나 카테고리별로 좁힐 수 있습니다.

### 고치면서 표시하기 {#marking-findings-as-you-fix-them}

카드마다 세 개의 버튼——**Fixed** / **Ignored** /
**Clear**——이 있어 코멘트 하나에 표시를 남깁니다.

- 표시는 코멘트당 하나뿐입니다. 하나를 설정하면 다른 표시를 대체하고,
  **Clear**로 해제합니다. 현재 상태는 카드의 색 배지로 보입니다.
- **Hide marked**(기본 켬, 브라우저별로 기억)은 남은 발견을 처리하는 동안
  표시된 카드를 화면에서 치워 둡니다. 도구 모음에 표시 수와 숨김 수가
  나타나며, 언제든 끄면 전체를 다시 볼 수 있습니다.
- **Clear all marks**는 세션 전체를 한 번에 되돌립니다.

표시는 뷰어 상태이지 리뷰 데이터가 아니며, 뷰어 자체는 읽기 전용으로
유지됩니다.

- 표시는 브라우저의 `localStorage`에 세션 페이지 단위로 저장됩니다. 세션
  JSONL 옆에는 아무것도 기록되지 않고, 뷰어는 쓰기 API를 아예 제공하지
  않습니다.
- 표시는 한 세션**이자 한 브라우저**에 속합니다. 다른 브라우저나 컴퓨터에서는
  표시 없는 세션이 보이고, 해당 사이트의 브라우저 저장소를 지우면 처음부터
  다시 시작합니다.
- 같은 변경을 다시 리뷰하면 새 세션이 만들어지며 표시 없이 시작합니다.

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

디스크를 비우려면 세션 파일을 통째로 지우세요. 뷰어는 다음 요청 때 색인을
다시 만듭니다.

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
