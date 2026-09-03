---
title: MCP 서버
sidebar:
  order: 10
---

OCR은 **Model Context Protocol(MCP) 클라이언트**로 동작할 수 있습니다. 외부
MCP 서버를 하나 이상 지정해 두면 그 서버가 제공하는 도구를 리뷰 Agent가 쓸 수
있게 됩니다. `file_read`나 `code_search` 같은 [내장 도구](../tools/)와 나란히
놓입니다.

## 언제 쓰나 {#when-to-use-it}

diff 바깥에 있는 맥락이 리뷰에 도움이 될 때 MCP 서버를 붙입니다.

- **이슈·티켓 조회** — 연결된 Jira나 GitHub 이슈를 Agent가 직접 가져와, 그
  변경이 요구 사항에 적힌 대로인지 확인하게 합니다.
- **문서·지식 베이스** — 사내 API 문서나 코딩 표준을 끌어와 코멘트가 실제 팀
  규칙을 근거로 삼게 합니다.
- **맞춤 분석** — 린터, 스키마 검증기, 의존성 검사기를 도구로 노출해 리뷰어가
  필요할 때 부르게 합니다.

저장소를 그냥 읽기만 하면 되는 경우라면 내장 도구로 충분합니다. MCP는 체크아웃
바깥까지 손을 뻗기 위한 것입니다.

## 설정 {#configuration}

#### 로컬 MCP 서버 추가하기 {#adding-a-local-mcp-server}

`ocr config set` 명령이 아래 필드를 대화 없이 기록합니다. 배열 필드(`args`,
`env`, `tools`)에는 JSON 배열 문자열을 넘깁니다.

```bash
# Minimal: just a command
ocr config set mcp_servers.docs.command npx

# Arguments
ocr config set mcp_servers.docs.args '["-y", "@acme/docs-mcp-server"]'

# Restrict which tools are exposed to the reviewer
ocr config set mcp_servers.docs.tools '["search_docs", "get_page"]'

# A setup command to run before the server starts
ocr config set mcp_servers.docs.setup "npm install -g @acme/docs-mcp-server"

# Environment variables (KEY=VALUE entries)
ocr config set mcp_servers.docs.env '["DOCS_TOKEN=secret", "DOCS_REGION=eu"]'
```

#### 원격 MCP 서버 추가하기 {#adding-a-remote-mcp-server}

**Streamable HTTP**를 지원하는 서버라면 `type`을 `remote`로 두고, 로컬 명령 대신
`url`을 지정합니다. `url`만 설정하는 것으로는 부족합니다. 기본 type이
`stdio`이기 때문입니다.

기존 연결을 덮어쓰지 않도록 새 서버 이름을 쓰세요.

```bash
ocr config set mcp_servers.search.type remote
ocr config set mcp_servers.search.url https://mcp.example.com/mcp
ocr config set mcp_servers.search.tools '["search", "fetch"]'
```

이 명령들은 연결을 사용자 설정에 저장합니다. 다음 리뷰에서 OCR이 서버에 접속해
`search`와 `fetch`를 내장 도구와 나란히 Agent에게 넘깁니다. 도구 허용 목록이
서버가 제공할 수 있는 나머지 도구를 리뷰 바깥에 남겨 둡니다. 이미 설정한 다른
서버와 리뷰 설정은 그대로입니다.

설정을 마치면 Agent는 리뷰 중에 매번 묻지 않고 이 도구들을 부릅니다. 도구
인자 — 검색어, 요청한 URL, Agent가 함께 실어 보내는 맥락 — 는 내 컴퓨터를 떠나
그 엔드포인트를 운영하는 쪽에 닿습니다. 사용자 설정이므로 저장소를 가로질러
적용됩니다. 외부 요청이 허용된 곳에서만 켜고, 요청에 비밀 값이나 비공개 코드,
내부 URL을 담지 마세요. 서드파티 서비스에 연결하기 전에 운영자의 개인정보
처리방침과 이용 약관을 확인하세요.

#### MCP 서버 제거하기 {#removing-an-mcp-server}

서버를 지울 때는 `unset`을 씁니다.

```bash
ocr config unset mcp_servers.docs
```

MCP 서버는 사용자 설정 파일(`~/.opencodereview/config.json`)의 `mcp_servers`
키 아래에 자리합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `type` | 문자열 | | 로컬 하위 프로세스는 `stdio`(기본값), Streamable HTTP는 `remote`. |
| `command` | 문자열 | `stdio`에 필수 | MCP 서버를 띄우는 실행 파일(예: `npx`, `uvx`, 절대 경로). |
| `args` | 문자열 배열 | | `command`에 넘길 인자(`stdio` 전용). |
| `url` | 문자열 | `remote`에 필수 | HTTP 또는 HTTPS MCP 엔드포인트. |
| `headers` | 객체 | | HTTP 헤더 이름과 문자열 값(`remote` 전용). 값은 연결 시점에 OCR의 환경에서 `$VAR` 또는 `${VAR}`를 펼칩니다. 빈 문자열로 펼쳐진 값은 빈 채로 보내지거나 무시되지 않고 **연결을 실패시킵니다**. 익명 접근이면 생략합니다. |
| `tools` | 문자열 배열 | | 등록할 도구 이름의 허용 목록. 비어 있으면 서버가 제공하는 모든 도구를 등록합니다. |
| `setup` | 문자열 | | 서버가 뜨기 전에 한 번 실행하는 셸 명령(`stdio` 전용, 예: 의존성 설치). 저장소 루트에서 5분 제한으로 돕니다. |
| `env` | 문자열 배열 | | 하위 프로세스에 넘길 `KEY=VALUE` 형태의 추가 환경 변수(`stdio` 전용). |

인증이 필요한 원격 서버라면 그 서버의 안내에 따라 `headers`를 설정하세요.
`ocr config set`에 넘기는 JSON에 환경 변수 참조가 들어 있으면, OCR이 설정을
저장하기 전에 셸이 먼저 펼쳐 버리지 않도록 작은따옴표로 감싸세요. 익명 접근을
허용하는 서버라면 `headers`는 아예 필요 없습니다.

## 도구 걸러 내기 {#filtering-tools}

기본적으로 서버가 알리는 도구는 모두 등록됩니다. 서버가 리뷰에 필요한 것보다
많은 도구를 노출한다면 `tools`에 허용 목록을 지정하세요. 도구가 적고
날카로울수록 Agent가 흐트러지지 않고 토큰 비용도 줄어듭니다. 목록에 적었지만
서버가 실제로 제공하지 않는 이름은 경고와 함께 건너뜁니다. 그래서 오타는 조용히
묻히지 않고 stderr에 드러납니다.

## 이름 충돌 {#name-conflicts}

MCP 도구 이름은 내장 도구와 이름 공간 하나를 함께 씁니다. 서버가 알린 도구
이름이 **내장·예약** 도구(`file_read`, `code_search`, `task_done` 등)나 다른
MCP 서버가 이미 등록한 도구와 겹치면 OCR은 그 도구를 **건너뛰고** 경고를
남깁니다. 먼저 등록한 쪽이 이깁니다. 도구를 이렇게 잃지 않으려면 서버마다
겹치지 않는 도구 이름을 쓰세요.

## `setup` 명령 {#the-setup-command}

`setup`은 서버 하위 프로세스가 뜨기 전에 저장소 루트에서 한 번 실행됩니다.
필요할 때 서버를 설치하거나 빌드하는 데 쓰세요.

```json
"setup": "npm install -g @acme/docs-mcp-server"
```

제한 시간은 **5분**입니다. 0이 아닌 코드로 끝나면 OCR은 명령, 작업 디렉터리,
출력을 기록한 뒤 그 서버를 건너뛰고 리뷰를 이어 갑니다.

## 문제 해결 {#troubleshooting}

MCP 진단 메시지는 모두 **stderr**로 나가며 `[ocr]` 접두사가 붙습니다. 그래서
stdout의 `--format json` 출력을 더럽히지 않습니다.

- `Running setup for MCP server "x": …` — setup 명령이 실행 중입니다.
- `failed to start MCP server "x": …` — 하위 프로세스가 30초 초기화 제한 안에
  연결되지 않았거나, `command`가 `PATH`에 없습니다.
- `remote MCP server "x" has no URL configured, skipping` — `type`은 `remote`인데
  `url`이 비어 있습니다. `url`만 넣고 `type`을 빠뜨리는 경우의 뒷면입니다.
- `failed to connect to remote MCP server "x": …` — 엔드포인트가 30초 초기화 제한
  안에 연결되지 않았거나 도구 목록을 내주지 않았습니다. URL, 네트워크 접근,
  필요한 헤더를 확인하세요.
- `MCP server "x" header "h" expanded to empty value` — `headers`에 쓴 `$VAR`가
  OCR의 환경에 없습니다. 헤더가 없는 것으로 넘어가지 않고 연결이 실패합니다.
- `remote MCP server "x" returned HTTP 401 Unauthorized` — 토큰이나 헤더 설정을
  확인하세요.
- `remote MCP server "x" returned HTTP 403 Forbidden` — 자격 증명은 서버에
  닿았지만 필요한 권한이 없습니다.
- `tool "y" conflicts with built-in tool, skipping` — 서버 쪽 도구 이름을
  바꾸거나 `tools`에서 빼세요.
- `allowed tool "y" not found in server's tool list` — `tools`에 적은 이름이
  서버가 제공하는 것과 맞지 않습니다. 철자를 확인하세요.

띄우거나 연결하는 데 실패한 서버는 건너뜁니다. 리뷰는 그 서버의 도구 없이
이어집니다.

## 함께 보기 {#see-also}

- [도구](../tools/) — MCP 도구가 나란히 놓이는 내장 도구 여섯 가지.
- [설정](../configuration/) — 설정 파일 전체와 모든 키.
- [CLI 레퍼런스](../cli-reference/) — `ocr config`와 리뷰 플래그.
