# 네이티브 OCR 플러그인

[English](../native-plugins.md) | [简体中文](native-plugins.zh-CN.md) | [日本語](native-plugins.ja-JP.md) | [한국어](native-plugins.ko-KR.md) | [Русский](native-plugins.ru-RU.md)

OCR은 설치 프로그램, 레지스트리 및 런타임을 통해 리뷰 규칙(Review Rules)과 MCP 도구를 제공하는 플러그인을 호스팅할 수 있습니다. 이러한 패키지는 `ocr-plugin.json`을 매니페스트로 사용하며, OCR을 다른 코딩 에이전트에 통합하는 구성과는 독립적으로 동작합니다.

모듈 경계, 수명 주기 다이어그램 및 권장 검토 순서는 [네이티브 플러그인 아키텍처 가이드](../plugin-architecture.md)를 참조하십시오.

## 지원되는 설치 소스 및 URL 입력

OCR 플러그인을 처음 사용할 때 가장 자주 묻는 질문: **현재 어떤 소스가 지원되며, 입력으로 URL을 지원합니까?**

`ocr plugin install <source>`는 공개 GitHub URL, HTTPS 아카이브 URL, 로컬 디렉터리와 아카이브를 지원합니다. `ocr plugin add <source>`는 같은 패키지 형식을 사용하지만 프로젝트 소스에는 추가 제한이 있습니다.

- 로컬 소스는 대상 저장소 내부로 해석되어야 하며 정규화된 `./` 경로로 기록됩니다. 저장소 내부의 절대 경로는 허용되지만 외부 디렉터리나 아카이브는 거부됩니다.
- 프로젝트 HTTPS 소스는 쿼리 매개변수가 없는 영구 URL이어야 하며 빈 `?`도 허용되지 않습니다. 전역 `install`은 쿼리를 허용하지만 관리 출력에는 표시하지 않습니다.
- `add`의 `--repo`는 대상 프로젝트를 지정합니다. 셸의 현재 디렉터리가 다르면 소스에 절대 경로를 사용하세요.

### 지원되는 소스 유형

OCR은 4가지 주요 패키지 소스 유형을 지원합니다:

1. **공개 GitHub 리포지토리(URL)**
   - **리포지토리 루트**: `https://github.com/OWNER/REPO`
     리포지토리 루트에 위치한 플러그인을 설치합니다.
   - **브랜치 또는 태그(하위 디렉터리 지원)**: `https://github.com/OWNER/REPO/tree/REF/PATH`
     예: `https://github.com/example/monorepo/tree/v1.2.0/plugins/security-rules`
   - **커밋 SHA(불변 리비전)**: `https://github.com/OWNER/REPO/tree/FULL_40_CHAR_COMMIT_SHA/PATH`
     특정 40자리 커밋 SHA를 직접 가리킵니다.
   - *동작 방식*: 브랜치나 태그가 지정되면 OCR은 GitHub API를 통해 이를 정확한 40자리 커밋 SHA로 변환합니다(URL에 이미 40자리 SHA가 있으면 API 조회를 건너뜁니다). 그런 다음 `codeload.github.com`에서 해당 커밋의 tar.gz 아카이브를 격리된 임시 디렉터리로 스트리밍 다운로드하고 대상 플러그인 디렉터리를 추출합니다.

2. **원격 HTTPS 아카이브(URL)**
   - **HTTPS tar.gz 아카이브**: `https://example.com/team-review.tar.gz`
   - **HTTPS ZIP 아카이브**: `https://example.com/team-review.zip`
   - **릴리스 자산**: `https://github.com/OWNER/REPO/releases/download/v1.0.0/plugin.tar.gz`
   - *동작 방식*: HTTPS를 통해 다운로드하고 매직 바이트(gzip: `0x1f 0x8b`, ZIP: `PK`)를 검사하여 형식을 자동 감지합니다(URL에 확장자가 필수는 아님). 크기 제한 내에서 격리된 샌드박스로 압축을 풀고 유효성을 검증합니다.

3. **로컬 디렉터리 경로(Path)**
   - 루트에 `ocr-plugin.json`이 있는 로컬 상대 또는 절대 경로:
     ```text
     ocr plugin install ./examples/ocr-plugins/review-checklist
     ocr plugin install /opt/company-plugins/code-style
     ```
   - 매니페스트 및 디렉터리 내용을 검증한 후 로컬 플러그인 저장소로 스냅샷을 복사합니다.

4. **로컬 아카이브 파일(Path)**
   - 로컬 `.tar.gz` 또는 `.zip` 파일:
     ```text
     ocr plugin install ./team-review.tar.gz
     ocr plugin install ./team-review.zip
     ```
   - 격리된 디렉터리에 압축을 풀고 무결성을 검증한 후 스냅샷을 저장합니다.

### 지원되지 않거나 금지된 소스

결정론적 실행, 재현 가능한 프로젝트 잠금 및 시스템 보안을 위해 다음은 **지원되지 않습니다**:

- **일반 HTTP URL(`http://`)**: 보안을 위해 HTTPS가 필수입니다. 안전하지 않은 HTTP 연결이나 HTTP로의 다운그레이드 리디렉션은 거부됩니다.
- **인증 정보 또는 프래그먼트가 포함된 URL**: 사용자 정보(예: `https://user:password@...`) 또는 URL 프래그먼트(`#...`)가 포함된 URL은 거부됩니다.
- **기존 마켓플레이스 카탈로그 / JSON 인덱스**: 카탈로그 검색 기능(`ID@market` 구문)은 패키지 직접 설치 방식으로 완전히 대체되었습니다.
- **순수 Git SSH 클론 URL(`git@github.com:...`)**: SSH를 통한 직접 Git 클론은 지원되지 않습니다. 공개 HTTPS URL이나 아카이브를 사용하십시오.
- **심볼릭 링크, 하드 링크, 디바이스 파일 및 소켓**: 디렉터리 탐색 및 권한 상승을 방지하기 위해 아카이브 내의 심볼릭 링크나 특수 파일은 압축 해제 시 엄격히 거부됩니다.
- **`@` 기호가 포함된 로컬 파일 이름**: 이전 마켓 ID와의 혼동을 방지하기 위해 `@`가 포함된 단순 입력은 거부됩니다. 파일 이름에 `@`가 포함된 경우 `./plugin@1.0.tar.gz`와 같이 명시적인 경로를 사용하십시오.

### 아카이브 및 리소스 제한

압축 해제 폭탄 및 리소스 고갈을 방지하기 위한 안전 제한:
- **매니페스트 최대 크기**: 1 MiB (`ocr-plugin.json`)
- **아카이브 다운로드 최대 크기**: 32 MiB
- **압축 해제 후 총 크기 제한**: 64 MiB
- **tar.gz 압축 해제 스트림 제한**: 72 MiB (헤더 및 메타데이터 포함)
- **패키지당 최대 파일 수**: 최대 4,096개 파일
- **프로젝트 구성 파일 제한**: 4 MiB (`plugins.json` 및 `plugins.lock.json`)

---

## 전체 CLI 서브커맨드 레퍼런스

OCR은 `ocr plugin` 아래에 **전역 플러그인 관리** 및 **프로젝트 수준 플러그인 관리** 명령어를 제공하며, 도구 확인을 위해 `ocr mcp list`를 지원합니다.

```text
ocr plugin <command> [flags]
```

### 전역 플러그인 관리 서브커맨드

전역 플러그인은 `~/.opencodereview/plugins`에 설치됩니다. 활성화되면 프로젝트별 구성이 없는 한 현재 사용자가 실행하는 모든 리포지토리의 리뷰 및 스캔에 자동으로 적용됩니다.

#### 1. `ocr plugin install <url|path>`
URL 또는 로컬 파일 경로에서 플러그인을 다운로드 및 검증하고 스냅샷을 생성하여 등록합니다.

- **인자**: `<url|path>` (GitHub URL, HTTPS 아카이브 URL, 로컬 디렉터리 또는 로컬 아카이브).
- **플래그**:
  - `--enable`: 설치 즉시 플러그인을 활성화합니다.
  - `--dry-run`: 설치나 활성화 없이 패키지 내용을 다운로드/검사하고 미리보기를 출력합니다.
  - `--plugin <id>`: 하나의 소스에 여러 플러그인이 포함된 경우 설치할 플러그인 ID를 지정합니다.
  - `--json`: 구조화된 JSON 형식으로 출력합니다.
- **핵심 원칙**: 설치는 파일 복사 및 매니페스트 유효성 검사만 수행하며, **플러그인 코드를 실행하거나 설치 스크립트를 실행하지 않습니다**.

#### 2. `ocr plugin list`
전역에 등록된 플러그인을 `ID`, `VERSION`, `STATE`, `CAPABILITIES` 열로 표시합니다. 소스는 `ocr plugin show <id>` 또는 `ocr plugin list --json`으로 확인합니다. `ocr plugins list`도 동일하게 동작하며, `ocr plugins`만 실행하면 도움말을 표시합니다.

- **플래그**: `--json` (JSON 배열로 출력)

```text
ID                VERSION  STATE     CAPABILITIES
review-checklist  1.0.0    disabled  1 rule packs, 0 MCP servers
```

#### 3. `ocr plugin show <id>`
설치된 플러그인의 매니페스트, 스냅샷 디렉터리, 기여 규칙, MCP 서버 및 호스트 요구 사항을 확인합니다.

- **인자**: `<id>` (플러그인 ID)
- **플래그**: `--json`
- **민감 정보 마스킹**: 자격 증명 유출을 방지하기 위해 MCP 명령 인자, 환경 변수, 인증 헤더 및 URL 쿼리 파라미터는 출력에서 자동으로 제외됩니다.

#### 4. `ocr plugin update <id>`
설치 시 기록된 원래 소스에서 플러그인을 다시 가져와 업데이트하며 활성화 상태를 유지합니다.

- **인자**: `<id>` (플러그인 ID)
- **플래그**:
  - `--source <url|path>`: 기록된 소스를 변경합니다. `install`과 동일하게 HTTPS/GitHub URL, 로컬 디렉터리, ZIP/tar.gz 아카이브를 지원합니다. 패키지 ID는 설치된 플러그인 ID와 일치해야 합니다.
  - `--accept-permissions`: 추가되거나 변경된 MCP 실행 권한을 명시적으로 수락합니다.
  - `--json`: JSON 결과 출력.
- **실행 권한 변경 감지**: 업데이트 시 **새로운 MCP 서버가 추가되거나 기존 서버의 명령어, 인자, 환경 변수, 전송 방식이 변경된 경우**, OCR은 업데이트를 중단하고 오류(`PLUGIN_EXECUTION_CHANGED`)를 발생시킵니다. 변경 사항을 확인한 후 `--accept-permissions` 플래그를 붙여 실행해야 합니다:
  ```sh
  ocr plugin update <id> --accept-permissions
  ```

다운로드 토큰이 만료되었거나 패키지 주소가 변경된 경우 새 소스를 지정할 수 있습니다:

```sh
ocr plugin update my-plugin --source "https://plugins.example.com/download.php?token=NEW_TOKEN"
```

업데이트에 성공하면 이후 `update <id>`는 쿼리 파라미터를 포함한 새 소스를 재사용하며 활성화/비활성화 상태를 유지합니다. 다운로드, 검증 또는 권한 승인에 실패하면 기존 패키지와 소스 기록을 변경하지 않습니다. 권한 승인이 필요하면 `--source`와 `--accept-permissions`를 함께 지정하여 다시 실행하세요.

HTTPS 다운로드 URL에는 쿼리 파라미터를 포함할 수 있으며, 응답이 유효한 플러그인 아카이브라면 확장자가 없어도 됩니다. 전체 URL은 로컬에 저장됩니다. 쿼리 파라미터는 관리 출력에서 숨겨지지만 저장 시 암호화되지는 않습니다. 이 명령은 전역 설치를 업데이트하며, 프로젝트 의존성은 `plugin add`와 `plugin lock`으로 관리합니다.

#### 5. `ocr plugin enable <id>`
지정된 플러그인을 활성화합니다. 기여된 리뷰 규칙과 MCP 도구가 후속 `ocr review` 및 `ocr scan` 실행에 로드됩니다.

#### 6. `ocr plugin disable <id>`
지정된 플러그인을 비활성화합니다. 패키지는 로컬에 유지되지만 리뷰 실행 시 로드되지 않습니다.

#### 7. `ocr plugin remove <id>`
플러그인 등록을 해제하고 삭제합니다. 현재 진행 중인 리뷰 세션은 기존 스냅샷을 계속 사용합니다.

---

<a id="project-plugins"></a>

### 프로젝트 수준 플러그인 관리 서브커맨드

팀 환경이나 CI/CD 파이프라인에서는 모든 참가자가 완전히 동일한 플러그인 버전을 사용하고 실행 권한을 명시적으로 제어할 수 있어야 합니다. 프로젝트 플러그인은 리포지토리 루트의 `.opencodereview/plugins.json`(선언) 및 `.opencodereview/plugins.lock.json`(잠금)을 통해 관리됩니다.

리포지토리에 프로젝트 플러그인이 선언된 경우, OCR은 **프로젝트 전용 모드**로 전환되어 전역 플러그인을 완전히 무시하고 잠긴 플러그인만 로드합니다.

#### 1. `ocr plugin add <source>`
현재 프로젝트에 플러그인 의존성을 추가합니다. 소스를 확인하고 선언 파일과 현재 플랫폼 잠금 파일을 업데이트합니다.

- **플래그**:
  - `--plugin <id>`: 다중 플러그인 소스에서 특정 ID 선택.
  - `--repo <path>`: 프로젝트 루트 경로.

#### 2. `ocr plugin lock`
선언 파일을 기반으로 현재 OS/아키텍처(`GOOS/GOARCH`)에 대한 모든 종속성을 확인하고 잠금 파일을 생성하거나 업데이트합니다.

#### 3. `ocr plugin sync`
잠금 파일에 지정된 정확한 아티팩트 스냅샷을 설치합니다.

- **플래그**:
  - `--frozen`: **필수 항목**. 선언과 잠금 파일이 정확히 일치해야 하며 새 버전 확인을 방지합니다.
  - `--offline`: 로컬 캐시 스냅샷 또는 리포지토리 내부 소스만 사용합니다.
  - `--repo <path>`: 프로젝트 루트 경로.
- **안전성**: 아티팩트를 복사할 뿐 **플러그인 코드를 실행하거나 실행 권한을 임의로 부여하지 않습니다**.

#### 4. `ocr plugin trust`
현재 머신에서 현재 프로젝트 잠금을 승인합니다. 원격 엔드포인트를 포함한 모든 MCP 서버에 승인이 필요합니다. 규칙만 제공하는 패키지에는 실행 승인이 필요하지 않습니다.

- **플래그**:
  - `--revoke`: 로컬 실행 권한을 취소합니다.
  - `--repo <path>`: 프로젝트 루트 경로.

승인 기록은 저장소 밖의 `~/.opencodereview/plugins/projects/<project-hash>/trust-<lock-id>`에 저장되며 정규 프로젝트 경로, 잠금 내용, 호스트 플랫폼에 결합됩니다. 잠금이 바뀌면 다시 승인해야 합니다. `add`, `lock`, `sync`는 승인하지 않습니다. 철회하려면 `ocr plugin trust --revoke`를 사용하세요.

#### 5. `ocr plugin project`
선언과 잠금의 일치를 검증하고 플랫폼 항목을 포함한 잠금 내용을 표시합니다. 설치 및 승인 상태는 표시하지 않습니다. 설치와 승인은 `ocr plugin preflight --json`으로, MCP 구성과 승인 상태는 `ocr mcp list --json`으로 확인하세요.

#### 6. `ocr plugin preflight`
리뷰 실행 전 또는 CI 파이프라인에서 사전 검증을 수행합니다. 잠긴 모든 패키지가 설치되었는지, 체크섬이 일치하는지, 호스트 버전 요구 사항을 충족하는지, 실행 신뢰가 부여되었는지 확인합니다.

- **플래그**:
  - `--fingerprint`: 검증된 잠금 및 플랫폼의 결정론적 지문 해시를 계산 및 출력합니다(증분 리뷰 캐싱용).
  - `--repo <path>`: 프로젝트 루트 경로.
  - `--json`: JSON 형식 출력.

---

### 통합 MCP 도구 확인 서브커맨드

#### `ocr mcp list`
사용자 설정(`~/.opencodereview/config.json`), 전역 플러그인, 프로젝트 플러그인에서 제공되는 모든 MCP 서버를 통합하여 읽기 전용으로 보여줍니다.

```sh
ocr mcp list
ocr mcp list --repo /path/to/project --json
```

- **출력 항목**: 서버 이름(`plugin/PLUGIN_ID/SERVER` 형식), 제공 소스(`user_config`, `global_plugin`, `project_plugin`), 전송 방식(`stdio`, `remote`), 구성 상태 및 인증 상태.
- **안전성**: 설정과 매니페스트만 읽으며, **프로세스를 실행하거나 외부 네트워크에 연결하지 않고 LLM 키에도 접근하지 않습니다**.

| 상태 | 의미 |
| --- | --- |
| `configured` | 서버가 선택 및 승인되어 리뷰 시 호출 가능 |
| `disabled` | 전역 플러그인이 현재 비활성화됨 |
| `incompatible` | 플러그인이 요구하는 최소 OCR 버전 미달 |
| `authorization_required` | 설치되었으나 `ocr plugin trust`를 통한 실행 승인이 이루어지지 않음 |
| `server_name_conflict` | 플러그인 서버 이름이 수동 설정 서버 이름과 충돌함 |

---

## 일반적인 운영 워크플로

### 전역 플러그인 운영 워크플로

```sh
# 1. 설치 전 드라이런 미리보기
ocr plugin install https://github.com/example/team-rules --dry-run

# 2. 설치 및 즉시 활성화
ocr plugin install https://github.com/example/team-rules --enable

# 3. 제공 기능 확인
ocr plugin show team-rules
ocr mcp list

# 4. 리뷰 실행 (규칙 및 도구가 자동 적용됨)
ocr review

# 5. 업데이트 시 권한 확인
ocr plugin update team-rules --accept-permissions
```

### 프로젝트 플러그인 & CI 워크플로

```sh
# 1. 의존성 추가 및 잠금 파일 커밋
ocr plugin add https://github.com/example/security-pack
git add .opencodereview/
git commit -m "chore: 보안 검토 플러그인 추가"

# 2. 팀원 또는 CI 환경에서 정확한 버전 동기화
ocr plugin sync --frozen

# 3. 패키지 확인 및 로컬 실행 권한 부여
ocr plugin project
ocr plugin trust

# 4. 사전 검증
ocr plugin preflight

# 5. 리뷰 실행
ocr review
```

---

### GitHub Actions

기존 OCR Action 단계의 `with` 입력에 다음을 추가하세요.

```yaml
project_plugins: 'true'
allow_plugin_exec: 'false'
```

`plugin sync --frozen`과 `preflight --fingerprint`를 지원하는 Action 및 CLI 버전을 고정하세요. 이전 릴리스에서는 이 흐름을 지원하지 않습니다. Action은 모델 자격 증명을 설정하기 전에 잠긴 패키지를 설치합니다. 원격 엔드포인트를 포함한 MCP 선언을 확인한 뒤에만 `allow_plugin_exec: 'true'`를 설정하세요. 해당 실행에만 권한을 부여하며 잠금 파일 자체는 실행 권한이 아닙니다.

규칙, 선언, 잠금 파일 및 저장소 내부 패키지는 신뢰된 정책 체크아웃에 있어야 합니다. 기본 정책은 PR의 base commit이며 댓글 트리거에서는 PR API로 조회합니다. `policy_sha`로 다른 신뢰된 커밋의 전체 SHA를 지정할 수 있습니다. PR head는 검토할 diff만 제공하며 플러그인 정책에는 사용되지 않습니다.

잠금에는 runner의 OS/아키텍처가 포함되어야 합니다. 각 대상 플랫폼에서 `ocr plugin lock`을 실행한 뒤 결과를 커밋하세요. 선언이 같으면 다른 플랫폼 항목이 유지되지만 선언이 바뀌면 무효화됩니다. `.gitattributes`의 `eol=lf` 등으로 체크아웃 간 로컬 패키지 바이트를 동일하게 유지하세요.

각 실행은 새 전용 OCR 홈에서 패키지 내용과 승인을 검증합니다. 네이티브 프로그램은 runner 권한으로 실행되며 모델 자격 증명을 포함한 환경에 접근할 수 있습니다. 공유 캐시에서 로컬 승인 기록을 복원하지 마세요. 정책, 플러그인 식별 정보 또는 실행 권한이 바뀌면 증분 리뷰 체크포인트가 무효화됩니다.

자세한 설정은 [Action 예제](../../examples/github_actions/README.md#project-plugins-in-ci)를 참조하세요.

---

## 플러그인 패키지 작성 사양

디렉터리 또는 tar.gz 아카이브의 루트에 `ocr-plugin.json`을 배치합니다:

```text
my-plugin/
├── ocr-plugin.json
├── rules.json
├── checks.md
└── server.mjs
```

매니페스트 예시:

```json
{
  "schema_version": 1,
  "id": "team-review",
  "name": "Team Review",
  "version": "1.0.0",
  "description": "팀별 검토 규칙 및 리포지토리 컨텍스트 도구",
  "rules": ["rules.json"],
  "mcp_servers": {
    "context": {
      "type": "stdio",
      "command": "node",
      "args": ["${plugin_dir}/server.mjs"],
      "tools": ["team_repository_context"]
    }
  },
  "requires": {
    "ocr_min_version": "1.2.0",
    "features": ["rules", "mcp"]
  },
  "metadata": {
    "homepage": "https://example.com/team-review"
  }
}
```

### 규칙 팩 파일

매니페스트의 `rules` 배열은 상대 `.json` 경로를 사용합니다. 각 JSON 파일에는 OCR 형식의 `rules` 배열이 있으며 개별 규칙에서 패키지 내부 Markdown을 참조할 수 있습니다. 규칙 팩에 `include` 또는 `exclude`를 설정할 수 없습니다. 플러그인 규칙은 기존 규칙 구성에 추가됩니다.

```json
{"rules":[{"path":"**/*","rule":"checks.md"}]}
```

규칙 JSON 파일은 각각 최대 512 KiB이며 참조 경로는 패키지 내부에 있어야 합니다. [review-checklist 예제](../../examples/ocr-plugins/review-checklist)를 참고하세요.

### 매니페스트 필드

- `schema_version`은 `1`이어야 합니다. 알 수 없는 동작 필드와 중복 JSON 키는 거부됩니다.
- `id`는 소문자, 숫자, 점, 밑줄, 하이픈으로 최대 64자이며 처음과 끝은 영숫자여야 합니다. Windows 예약 장치 이름은 사용할 수 없습니다.
- `name`과 `version`은 비어 있을 수 없습니다. 하나 이상의 규칙 팩이나 MCP 서버를 선언해야 합니다.
- `rules`는 상대 `.json` 경로입니다. 형식은 위의 규칙 팩 설명을 참고하세요.
- `mcp_servers`의 `stdio`(기본)는 `command`, `args`, `env`(`KEY=VALUE`), `tools` 허용 목록을 사용합니다. `remote`는 HTTP/HTTPS `url`, `headers`, `tools`를 사용합니다. 자격 증명을 전송할 때는 HTTPS를 사용하세요. 패키지 다운로드에는 별도로 HTTPS가 필수입니다.

### 플레이스홀더 지원

`command`, `args`, `env`에서는 다음 플레이스홀더를 지원합니다:
- `${plugin_dir}`: 설치된 패키지 스냅샷 디렉터리의 절대 경로.
- `${os}`: 운영체제 이름(`linux`, `darwin`, `windows`).
- `${arch}`: 아키텍처 이름(`amd64`, `arm64`).
- `${exe}`: Windows에서는 `.exe`, 유닉스 계열에서는 빈 문자열.

### 배포용 패키징

Linux/macOS에서 실행 가능한 스크립트나 바이너리를 패키징할 때는 아카이브 생성 전에 실행 권한을 설정하십시오(`chmod +x bin/server`). OCR은 tar.gz 압축 해제 시 파일의 실행 권한 비트를 자동으로 유지합니다:

```sh
tar -czf team-review-1.0.0.tar.gz -C my-plugin .
```

---

## 보안 모델 및 무결성 보장

1. **설치 시 무코드 실행**: `install`, `sync`, `list`, `show`, `dry-run`, `lock`, `preflight`는 플러그인 코드를 실행하지 않습니다.
2. **결정론적 무결성 다이제스트**:
   - `archive_sha256`: 아카이브 파일의 SHA-256.
   - `content_sha256`: `ocr-package-tree-v1` 알고리즘을 사용한 결정론적 콘텐츠 해시.
3. **실행 권한 변경 안전 게이트**: 전역 플러그인 업데이트 시 MCP 실행 설정 변경이 감지되면 `--accept-permissions`를 통한 명시적 확인이 필요합니다.
4. **프로젝트 신뢰 격리**: 실행 신뢰는 리포지토리 외부에 보관되며 새로 클론한 프로젝트가 로컬 권한을 무단으로 행사할 수 없습니다.
5. **CI 베이스 커밋 보호**: GitHub Actions에서 플러그인 선언과 잠금은 항상 PR의 베이스 커밋(신뢰할 수 있는 부모 커밋)에서 로드되어 안전합니다.

<a id="compatibility-and-host-requirements"></a>

## 호환성과 호스트 요구 사항

`requires`는 선택 사항입니다. `ocr_min_version`은 `1.2.0`처럼 완전한 시맨틱 버전이어야 합니다. 비교 가능한 버전이 없는 `dev` 빌드는 이 요구 사항을 충족할 수 없습니다. `features`는 필수 기능이며 이 버전은 `rules`, `mcp`, `stdio-v1`를 지원합니다. 지원되지 않는 요구 사항은 실행 전에 거부됩니다. 필요하지 않은 버전 제약은 생략하세요.

설명용 메타데이터는 `metadata`에 넣습니다. 알 수 없는 동작 필드와 중복 JSON 키는 거부됩니다. 이전 마켓 메타데이터는 마이그레이션용으로 읽을 수 있지만 카탈로그 설치는 지원하지 않습니다.

워크플로는 규칙 및 MCP와 함께 제공할 수 있습니다. 프로젝트 워크플로 승인도 잠금에 결합됩니다. 전역 업데이트가 워크플로 실행 선언을 추가하거나 변경하면 `--accept-permissions`가 필요합니다. 설치나 활성화만으로 선택되지 않으며 실행하려면 `--review-plugin ID`를 명시해야 합니다. [리뷰 워크플로 플러그인](review-workflow-plugins.ko-KR.md)을 참고하세요.
