---
title: MCP 서버
sidebar:
  order: 10
---

## 시작하기

`ocr mcp`를 실행하고 **Add server**를 선택합니다. 이미 설정된 서버는
**CONFIGURED SERVERS**, 추가·가져오기·전역 권한은 **MANAGEMENT ACTIONS**에 표시됩니다.
**Tab**으로 영역을 이동하고 **Enter**로 엽니다. 각 단계는 현재 화면을 교체합니다.
**Esc**는 이전 화면 또는 취소, **Ctrl-C**는 종료, **PgUp/PgDn**은 긴 내용 스크롤입니다.
목록을 여는 것만으로 서버에 연결하지 않습니다. 연결 상태는 마지막 검사 결과입니다.

1. 서버 이름과 `stdio` 또는 `remote`를 선택합니다.
2. stdio는 실행 파일, 개별 인수, 환경 변수 참조를 입력합니다.
   remote는 URL과 header 참조를 입력합니다.
3. 실행 명령 또는 주소와 위험 안내를 확인한 뒤 연결을 승인합니다.
4. 서버 도구 목록을 확인합니다. **Space**로 필요한 도구만 선택합니다.
5. 선택한 도구는 기본 `ask`이며 최종 확인 후 저장합니다.
   **Ctrl-B**로 이전 단계에 돌아갈 수 있습니다.
6. 연결하지 않고 보관하려면 **Save disabled (no connection)**을 선택합니다.
   비활성·도구 없음 상태로 저장됩니다.

서버의 **Tools**는 설정된 도구와 권한을 보여 주며, **Test connection**은 확인 후
도구를 다시 찾습니다. 새 도구는 자동 선택하지 않습니다. 변경된 정의를 수락하면
권한이 `ask`로 돌아갑니다. 도구 비활성화는 오프라인에서도 가능합니다.

## 연결 예시

이미 설치한 로컬 서버에 연결하려면:

```bash
ocr mcp add docs --type stdio --command /absolute/path/to/docs-mcp --yes
ocr mcp tools docs --enable search_docs --yes
ocr mcp enable docs --yes
```

원격 Streamable HTTP 서버에 연결하려면:

```bash
ocr mcp add search --type remote --url https://mcp.example.com/mcp --yes
ocr mcp tools search --enable search --yes
ocr mcp enable search --yes
```

명령의 도구 이름은 예시입니다. 서버가 제공하는 이름을 모르면 대화형 관리자를
사용하세요. 비대화형 add는 저장만 하며 시작하거나 연결하지 않습니다.
외부 서비스에 보내는 요청에는 비공개 코드와 내부 URL을 넣지 말고 운영자의
개인정보 처리방침을 확인하세요. 인증이 없는 서버는 header가 필요 없습니다.

## 가져오기

**Import (JSON / TOML)**에서 파일을 선택하거나 내용을 붙여 넣습니다.
Cursor JSON의 `mcpServers`와 Codex TOML의 `mcp_servers`를 지원합니다.
붙여 넣은 내용은 숨겨지며 **Ctrl-S**로 미리보기를 엽니다.

```bash
ocr mcp import ./mcp.json
ocr mcp import ./one-server.toml --yes
```

한 번에 연결 하나만 가져오며, 권한은 복사하지 않습니다. 기존 이름은 덮어쓰지 않습니다.
파일 제한은 1 MiB, 64개 서버이며 비대화형 입력에는 서버 하나와 `--yes`가 필요합니다.
OAuth, 사용자 지정 cwd와 지원하지 않는 필드는 거부합니다.
평문 env/header는 환경 변수 참조로 바뀝니다. 활성화 전에 **Edit connection**에서
검색을 승인하고 사용할 도구를 선택하세요.

## 두 단계 권한 검사

`ocr review`만 MCP를 사용합니다. `ocr scan`은 MCP를 로드하지 않습니다.
`tools`가 비어 있거나 없으면 **도구 0개**를 뜻합니다.

모델 표시 범위와 실행 승인은 독립적인 두 단계입니다. 전역·서버가 활성화되어 있고,
명시적으로 선택한 도구의 fingerprint가 일치해야 모델에 표시됩니다.
실제 실행 직전에도 Authorizer가 도구 신원, 권한, context를 다시 검사합니다.

거부, 취소, timeout, EOF 또는 입력 불가 시 `tools/call`을 보내지 않습니다.
`allow`는 승인 창만 생략하며 allowlist를 넓히지 않습니다.
서버 annotation은 검증되지 않은 정보이며 자동 승인에 사용하지 않습니다.

| 설정 | 의미 |
|---|---|
| 전역 `deny` | 모든 MCP 도구 거부 |
| 서버 `deny` | 해당 서버의 모든 도구 거부 |
| 도구 `deny` | 해당 도구 거부 |
| `inherit` | 상위 설정 사용 |
| `ask` | 실행 전 승인 필요 |
| `allow` | 선택·fingerprint·상위 정책 검사를 통과한 도구만 자동 실행 |

상위 `deny`는 하위 설정으로 덮어쓸 수 없습니다.
그 밖에는 도구 > 서버 > 전역 순서로 가장 구체적인 설정을 사용합니다.

실행 승인 선택지는 **Allow once**, **Allow this review**, **Deny once (default)**,
**Deny this review**입니다. 이번 review의 선택은 정확한 서버·도구에만 적용되며
다음 review나 설정 파일에 남지 않습니다. 영구 `allow`는 `ocr mcp permissions`에서 설정합니다.

## CI와 timeout

TTY가 아니거나 CI 환경이면 `ask` 도구는 모델에서 숨겨집니다. 명시적으로 선택하고
fingerprint가 일치하는 `allow` 도구만 자동 실행할 수 있습니다. 실패 시 fail closed입니다.

`mcp.approval_timeout_seconds`의 기본값은 60초이며 범위는 1–600초입니다.

```bash
ocr mcp permissions --timeout 120
ocr config set mcp.approval_timeout_seconds 120
```

승인 대기열과 입력 모두 제한 시간을 적용합니다. timeout은 허용을 의미하지 않습니다.

## 도구 검색과 신원

검색은 프로토콜 초기화와 페이지별 `tools/list`만 수행합니다.
`tools/call` 실행이나 모델 등록은 하지 않습니다.
최대 64페이지·512도구, 설명 8 KiB, schema 256 KiB·깊이 64, 전체 목록 4 MiB,
실행 결과 1 MiB 제한이 있습니다. 중복 이름·cursor·잘못된 schema는 거부합니다.

모델 별칭은 `mcp__<server-slug>__<tool-slug>__<16-hex>` 형식입니다.
이름 충돌이 있으면 해당 review의 모든 MCP 등록을 중단하여 부분 노출을 막습니다.
실행 중 도구 목록은 고정되며 변경된 정의는 다음 검색에서 수동 승인해야 합니다.

## 설정과 마이그레이션

전역 키는 `mcp.version`(1), `mcp.enabled`, `mcp.default_permission`,
`mcp.approval_timeout_seconds`입니다. 서버는 `mcp_servers`에 저장되며
`default_permission`, `tools`, `tool_permissions`, `tool_definition_sha256`,
`allow_insecure_http`를 사용합니다.

일반 review는 legacy 설정을 자동 변경하지 않습니다. fingerprint 없는 기존 도구는
대화형 `ask`만 가능하고 CI 자동 실행은 불가능합니다.
기존 `setup`은 실행하지 않으며 관리자가 저장할 때 제거합니다. 서버는 직접 설치하세요.
마이그레이션 후 구버전 OCR로 설정을 편집하지 마세요.

`OCR_CONFIG_PATH`로 읽기·쓰기·review의 설정 파일을 함께 지정할 수 있습니다.
저장은 같은 디렉터리의 0600 임시 파일을 fsync 후 원자적으로 교체합니다.
취소·검색 실패는 저장하지 않습니다. 동시에 다른 프로세스가 수정하면 저장을 거부하므로
관리자를 다시 열고 변경을 적용하세요. 구버전과 수동 편집기는 이 잠금을 따르지 않습니다.

`0600` 권한은 Unix 계열 시스템에 적용됩니다. Windows는 Unix 권한 비트 대신
디렉터리에서 상속한 ACL을 사용하므로 설정 디렉터리를 본인 계정만 접근할 수 있도록 보호하세요.

## 자격 증명과 로그

stdio는 부모 프로세스의 전체 환경을 상속하지 않습니다. 필요한 변수는
`${ENV_NAME}`으로 명시합니다. header 값도 환경 변수 참조를 사용하며 토큰을
명령 인수나 URL에 넣지 마세요. 비밀 값은 로그·오류·결과에서 마스킹합니다.
외부 주소는 HTTPS가 기본이며 다른 HTTP 주소는 `allow_insecure_http`와 확인이 필요합니다.
교차 출처 redirect와 HTTPS에서 HTTP로의 redirect는 거부합니다.

MCP 도구가 모델에 노출된 review에서는 `OCR_RAW_LOGGING` 원문 저장을 중단합니다.
민감한 도구 호출의 native payload와 설명·사고 내용은 저장용 기록에서 제외하며
모델이 다음 호출에 사용하는 원래 데이터는 유지합니다.

도구 검색만 하더라도 로컬 프로세스 시작이나 원격 접속 자체에는 부작용이 있을 수 있습니다.
이 기능은 운영체제 sandbox가 아닙니다.

## 명령

```text
ocr mcp
ocr mcp add [name]
ocr mcp import [file] [--yes]
ocr mcp list [--json]
ocr mcp show <name> [--json]
ocr mcp edit <name>
ocr mcp discover <name> [--json] [--yes]
ocr mcp tools <name> [--enable TOOL ...] [--disable TOOL ...] [--yes]
ocr mcp permissions [name]
ocr mcp enable <name>
ocr mcp disable <name>
ocr mcp remove <name> [--yes]
```

`list/show`는 접속하지 않고 민감한 연결 값도 출력하지 않습니다.
삭제 확인의 기본값은 No이며 비대화형에서는 `--yes`가 필요합니다.

## 문제 해결

- `needs-review`: Tools에서 정의를 확인하고 다시 승인합니다.
- CI에서 도구 없음: `ask`, 비활성 상태, allowlist와 fingerprint를 확인합니다.
- 연결 실패: 실행 파일·주소·환경 변수와 서버의 MCP 지원을 확인합니다.
- 결과 초과: 쿼리를 좁혀 1 MiB 이하로 요청합니다.
- 서버 `isError`: 성공으로 기록하지 않고 실제 도구 실패로 보고합니다.
- 이름 충돌: 구성 키나 도구 이름을 조정합니다.
