---
title: 기여하기
sidebar:
  order: 13
---

OCR은 Apache-2.0 라이선스로 공개된 오픈 소스입니다. 버그 신고, 문서 수정, 코드
기여 모두 환영합니다. 이 페이지는 요약본이고, 정본은
[`CONTRIBUTING.md`](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)에
있습니다.

## 기여하는 방법 {#ways-to-contribute}

Go를 쓰지 않아도 도울 수 있습니다.

- **버그 신고** — 재현 절차를 담아
  [GitHub 이슈](https://github.com/alibaba/open-code-review/issues/new/choose)를
  열어 주세요.
- **기능 제안** —
  [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)에
  글을 올리거나 기능 제안 이슈를 열어 주세요.
- **문서** — 오타 수정, 빠진 예제, 깨진 링크. 이런 PR이 대개 가장 빨리
  머지됩니다.
- **다른 PR 리뷰** — 메인테이너가 아닌 분들의 코멘트가 리뷰 부담을 덜어 줍니다.
- **코드** — 버그 수정, 성능 개선, 새 기능.

## 로컬 개발 환경 준비 {#local-development-setup}

### 사전 요구 사항 {#prerequisites}

- [Go 1.25 이상](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### 소스 내려받기 {#getting-the-source}

```bash
# Fork on GitHub, then:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # writes dist/opencodereview
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> `upstream` 리모트는 읽기 전용입니다. `origin`(여러분의 fork)으로 푸시하고
> 거기서 PR을 여세요.

### 로컬 빌드 실행하기 {#running-your-local-build}

```bash
./dist/opencodereview review --preview
```

편하게 쓰려면 `dist/opencodereview`를 가리키는 심볼릭 링크를 `~/bin/ocr-dev`에
걸어 두세요. 어느 저장소에서든 `ocr-dev`로 부를 수 있습니다.

### Make 타깃 {#make-targets}

| 타깃 | 하는 일 |
|---|---|
| `make build` | 현재 플랫폼용으로 빌드해 `dist/opencodereview`에 씁니다. |
| `make build-darwin-amd64` | macOS Intel용 크로스 컴파일. |
| `make build-darwin-arm64` | macOS Apple Silicon용 크로스 컴파일. |
| `make build-linux-amd64` | Linux x86_64용 크로스 컴파일. |
| `make build-linux-arm64` | Linux ARM64용 크로스 컴파일. |
| `make build-windows-amd64` | Windows x86_64용 크로스 컴파일. |
| `make build-windows-arm64` | Windows ARM64용 크로스 컴파일. |
| `make build-all` | 크로스 컴파일 바이너리 여섯 개 전부(linux/darwin/windows × amd64/arm64). |
| `make sha256sum` | 빌드 산출물의 `sha256sum.txt`를 만듭니다. |
| `make dist` | `clean → build-all → sha256sum`. CI가 돌리는 것입니다. |
| `make test` | 레이스 디텍터를 켜고 테스트를 돌립니다. |
| `make clean` | `dist/`를 지웁니다. |

## 브랜치와 커밋 규칙 {#branching-and-commit-conventions}

### 브랜치 접두사 {#branch-prefixes}

| 접두사 | 용도 |
|---|---|
| `feat/` | 새 기능 |
| `fix/` | 버그 수정 |
| `docs/` | 문서만 |
| `refactor/` | 동작 변화 없는 리팩터링 |
| `test/` | 테스트만 |
| `chore/` | 빌드 / CI / 도구 |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### 커밋 메시지 {#commit-messages}

[Conventional Commits](https://www.conventionalcommits.org/) 형식을 씁니다.

```
<type>(<scope>): <short summary>

[optional body explaining the why]
```

예시:

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(readme): clarify endpoint resolution priority
refactor(viewer): extract task-card rendering into helper
```

**PR 제목**에도 같은 형식을 씁니다. 그래야 생성되는 변경 로그에 깔끔하게
들어갑니다.

## 프로젝트 구조 {#project-layout}

```
open-code-review/
├── cmd/opencodereview/        # CLI 진입점 — 플래그 파싱, 디스패치
├── internal/
│   ├── agent/                 # 리뷰 Agent 로직, 서브 Agent 디스패치
│   ├── config/                # 템플릿, 규칙, 허용 목록, 내장 JSON
│   ├── diff/                  # Git diff 파싱, 세 가지 모드
│   ├── gitcmd/                # Git 하위 프로세스 실행기
│   ├── llm/                   # LLM 클라이언트(Anthropic, OpenAI), 엔드포인트 해석기
│   ├── model/                 # 데이터 구조체(LlmComment, Diff 등)
│   ├── pathutil/              # 경로 유틸리티
│   ├── release/               # 릴리스 노트 생성
│   ├── session/               # JSONL 세션 기록기
│   ├── stdout/                # 조용히 만들 수 있는 stdout 라이터
│   ├── suggestdiff/           # 제안 diff 렌더링
│   ├── telemetry/             # OpenTelemetry 설정과 헬퍼
│   ├── tool/                  # 도구 레지스트리와 프로바이더 구현
│   └── viewer/                # 내장 HTTP UI
├── pages/                     # WebUI 소개 페이지(별도 React 앱)
├── plugins/                   # Claude Code 슬래시 명령
├── extensions/                # 에디터 확장(VS Code)
├── examples/                  # CI 레시피(GitHub Actions, GitLab CI)
├── skills/                    # Agent SDK skill 매니페스트
├── scripts/                   # NPM postinstall과 크로스 빌드 스크립트
├── npm/                       # 플랫폼별 optional dependency 패키지
└── bin/                       # NPM 래퍼(Node)
```

기여는 대개 `internal/agent/`, `internal/tool/`, `internal/llm/`을 건드립니다.
`cmd/opencodereview/`의 CLI 표면은 일부러 얇게 두었습니다. 플래그를 파싱한 뒤
agent 패키지로 넘기는 것이 전부입니다.

## 라이선스 헤더 {#license-headers}

모든 소스 파일(`.go`, `.sh`, `.js`, `.mjs`, `.ts`, `.tsx`)에는 SPDX 라이선스
헤더가 있어야 합니다. 파일을 새로 만들었다면 다음을 실행하세요.

```bash
make license-add
```

필요한 헤더가 자동으로 붙습니다. 헤더가 빠진 PR은 CI가 막습니다.

## 코드 품질 검사 {#code-quality-checks}

PR을 열기 전에:

```bash
make check      # format, lint, and verify license headers
make test       # race-enabled, runs in CI on every push
make build      # smoke test the binary builds
```

CI도 푸시할 때마다 같은 것을 돌립니다. 놀랄 일은 없습니다.

## 새 도구 추가하기 {#adding-new-tools}

도구는 두 부분으로 이뤄집니다.

1. [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)의
   **JSON 정의** — LLM이 보게 될 이름, 설명, JSON 스키마 매개변수입니다.
2. `internal/tool/definitions.go`에 등록하는 **Go 프로바이더** — 실제 구현입니다.

새 도구 이름이 동작하려면 둘 다 있어야 합니다. 기존 여섯 개는
[도구](../tools/)에서 볼 수 있으니 본보기로 삼으세요.

## 새 규칙 패턴 추가하기 {#adding-new-rule-patterns}

`internal/config/rules/system_rules.json`을 고쳐 새 glob을 규칙 문서에 연결하고,
그에 맞는 마크다운을 `internal/config/rules/rule_docs/` 아래에 추가하세요. 규칙
문서는 패턴당 파일 하나이며 영문입니다. `language` 설정은 해당 언어로 답하라는
지시문을 시스템 프롬프트에 덧붙일 뿐, 규칙 문서 파일을 갈아 끼우지는 않습니다.

## PR 절차 {#pr-process}

1. **큰 변경은 이슈부터 여세요.** 접근 방식을 먼저 맞추는 편이, 코드 리뷰에서야
   어긋난 걸 알게 되는 것보다 낫습니다.
2. **PR 하나에 논리적 변경 하나.** 서로 무관한 수정이 두 개라면 PR도 두 개로
   나눠 주세요.
3. **테스트를 갱신하세요.** 동작이 바뀌면 테스트도 따라와야 하며 `make test`가
   통과해야 합니다.
4. **문서를 갱신하세요.** 플래그나 설정 키, 리뷰 파이프라인에 영향이 있는
   변경이라면 이 문서 사이트([`docs/`](https://github.com/alibaba/open-code-review))와
   관련 인라인 도움말을 함께 고쳐 주세요.
5. **PR 템플릿을 채우세요.** 메인테이너가 검토하며, 보통 영업일 며칠 안에
   답이 갑니다.

## PR 리뷰를 앞당기는 요령 {#tips-for-faster-pr-reviews}

PR이 빨리 리뷰되고 머지되길 바라시나요? 다음이 도움이 됩니다.

- **CLA에 일찍 서명하세요** — 처음 기여하는 분들이 CLA 봇의 코멘트를 놓쳐 막히는
  일이 잦습니다. 봇이 안내하는 즉시 기여자 라이선스 동의서에 서명하세요. 서명
  없이는 PR을 머지할 수 없습니다.
- **CI 검사를 모두 통과시키세요** — 검사가 실패한 PR은 리뷰하지 않습니다. 푸시
  전에 `make test`와 `make build`를 로컬에서 돌려 문제를 미리 잡으세요.
- **변경은 좁고 작게 유지하세요** — 한 가지를 제대로 하는 PR이 이것저것 섞인 PR
  보다 훨씬 리뷰하기 쉽습니다. 작은 PR일수록 빨리 리뷰되고 여러 차례 고쳐 쓸 일도
  줄어듭니다.
- **설명을 명확하고 정확하게 쓰세요** — *무엇을* 바꿨고 *왜* 바꿨는지 적어
  주세요. 설명은 실제 diff와 맞아야 합니다. 둘이 어긋나면 리뷰어의 신뢰를
  잃습니다. 개발 도중 범위가 달라졌다면 리뷰를 요청하기 전에 설명을 고치세요.
- **동작이 바뀌면 테스트를 함께 넣으세요** — 테스트 없는 새 기능이나 버그 수정은
  의문을 남깁니다. 테스트는 정확성을 보여 주고, 의도한 동작이 무엇인지 리뷰어가
  이해하도록 돕습니다.
- **기존 코드의 방식을 따르세요** — 주변 코드의 스타일과 이름 규칙, 구조에
  맞추세요. 일관성은 리뷰어의 부담을 줄이고 스타일만 지적하는 코멘트를 없애
  줍니다.
- **피드백에 빠르게 답하세요** — 리뷰어가 수정을 요청하면 빨리 반영해 리뷰
  주기를 짧게 유지하세요. 동의하지 않는다면 코멘트를 무시하지 말고 이유를 설명해
  주세요.

## 기여자 라이선스 동의서(CLA) {#contributor-license-agreement-cla}

이 프로젝트는 Alibaba 오픈 소스 CLA를 요구합니다. 처음 PR을 열면 봇이 링크를
남깁니다. 전자 서명하면 되고 1분이면 끝납니다. 이후의 PR에서는 다시 서명하지
않아도 됩니다.

## 처음 기여하시나요? {#first-contribution}

[`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue)나
[`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted)
라벨이 붙은 이슈를 찾아보세요. 대부분 작고 독립적이며, 이슈 설명만으로 시작하기에
충분한 맥락이 담겨 있습니다.

## 함께 보기 {#see-also}

- [아키텍처](../architecture/) — `internal/agent/`를 건드리기 전에 갖춰야 할
  머릿속 그림.
- [도구](../tools/) — 기존 도구가 어떻게 생겼는지.
- 전체 기여 가이드:
  [CONTRIBUTING.md](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)
