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

#### MCP 서버 추가하기 {#adding-an-mcp-server}

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

#### MCP 서버 제거하기 {#removing-an-mcp-server}

서버를 지울 때는 `unset`을 씁니다.

```bash
ocr config unset mcp_servers.docs
```

MCP 서버는 사용자 설정 파일(`~/.opencodereview/config.json`)의 `mcp_servers`
키 아래에 자리합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `command` | 문자열 | ✓ | MCP 서버를 띄우는 실행 파일(예: `npx`, `uvx`, 절대 경로). |
| `args` | 문자열 배열 | | `command`에 넘길 인자. |
| `tools` | 문자열 배열 | | 등록할 도구 이름의 허용 목록. 비어 있으면 서버가 제공하는 모든 도구를 등록합니다. |
| `setup` | 문자열 | | 서버가 뜨기 전에 한 번 실행하는 셸 명령(예: 의존성 설치). 저장소 루트에서 5분 제한으로 돕니다. |
| `env` | 문자열 배열 | | `KEY=VALUE` 형태의 추가 환경 변수. |

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
- `tool "y" conflicts with built-in tool, skipping` — 서버 쪽 도구 이름을
  바꾸거나 `tools`에서 빼세요.
- `allowed tool "y" not found in server's tool list` — `tools`에 적은 이름이
  서버가 제공하는 것과 맞지 않습니다. 철자를 확인하세요.

## 함께 보기 {#see-also}

- [도구](../tools/) — MCP 도구가 나란히 놓이는 내장 도구 여섯 가지.
- [설정](../configuration/) — 설정 파일 전체와 모든 키.
- [CLI 레퍼런스](../cli-reference/) — `ocr config`와 리뷰 플래그.
