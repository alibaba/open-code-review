<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja-JP.md">日本語</a> | 한국어 | <a href="README.ru-RU.md">Русский</a>
</p>

# Open Code Review (VS Code 확장)

[`open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review) (`ocr`) CLI를 기반으로 만든 VS Code 코드 검토 확장입니다. Preact WebView로 프로토타입의 사용 경험을 재현하고 AI 코드 검토를 에디터로 가져옵니다. 사이드바에서 검토를 시작하고 로그를 실시간으로 확인하며, 각 댓글을 에디터 안에서 바로 적용/무시/오탐 처리할 수 있습니다. 사이드바와는 양방향으로 동기화됩니다.

---

## 기능

- **세 가지 검토 모드**: 워크스페이스 변경 사항, 브랜치 비교(`--from` / `--to`), 단일 커밋(`--commit`).
- **검토 대상 파일 미리 보기**: 현재 Git 상태에서 변경된 파일을 나열하고, 파일을 클릭하면 기본 diff 뷰에서 변경 내용을 확인할 수 있습니다.
- **사용자 지정 검토 프롬프트**: 이번 검토에 `--background` 힌트를 선택적으로 추가할 수 있습니다.
- **스트리밍 로그**: 검토 중에는 CLI 출력을 실시간으로 보여 주며 언제든 취소할 수 있습니다.
- **결과 표시 + 양방향 동기화**: 완료되면 사이드바에 댓글 카드가, 에디터에는 CommentThread가 나타납니다. 적용/무시/오탐 작업은 양쪽에서 동기화됩니다.
- **비어 있음 / 취소 / 실패 상태**: 문제 없음, 사용자 취소, CLI 실패에 각각 전용 화면이 있습니다(실패는 다시 시도할 수 있고 CLI가 반환한 실제 오류를 보여 줍니다).
- **설정 관리**: LLM 프로바이더 설정을 확장 안에서 보고 편집할 수 있습니다(저장은 `ocr config set`).
- **모델 전환 / 연결 테스트**: 상태 표시줄에서 모델을 전환하고 LLM 연결을 테스트합니다.

---

## 사전 요구 사항

1. `ocr` CLI를 전역으로 설치합니다:

   ```bash
   npm i -g @alibaba-group/open-code-review
   ```

2. 사용할 LLM(엔드포인트, API 키, 모델)을 설정합니다. CLI에서 직접 설정하거나 확장의 설정 화면에서 입력합니다:

   ```bash
   ocr config set llm.url https://api.anthropic.com/v1/messages
   ocr config set llm.auth_token sk-...
   ocr config set llm.model claude-opus-4-6
   ocr config set llm.use_anthropic true
   ```

   설정은 `~/.opencodereview/config.json`에 기록됩니다.

---

## 개발

### 환경

- Node.js 18 이상, 패키지 관리자는 **Yarn**(저장소에 `yarn.lock` 포함).
- VS Code 1.74 이상.
- 전역에서 사용할 수 있는 `ocr` CLI(위 "사전 요구 사항" 참고). 이 확장은 사실상 `ocr`의 GUI 프런트엔드입니다.

### 개발 환경 시작

```bash
cd extensions/vscode
yarn install      # 의존성 설치
yarn watch        # 워치 모드 개발 빌드(변경 시 out/를 다시 빌드하므로 권장)
```

그다음 VS Code에서 `extensions/vscode` 폴더를 열고 **F5**를 눌러 Extension Development Host를 실행합니다(디버그 구성은 `.vscode/launch.json`에 있습니다). 새 창에서 Git 변경 사항이 있는 프로젝트를 열면 활동 표시줄에 Open Code Review 아이콘이 나타나고 검토를 시작할 수 있습니다.

> 코드를 수정한 뒤: WebView 변경은 개발 호스트 창에서 **사이드바를 다시 열어야** 반영됩니다(`Developer: Reload Webviews` 실행도 가능). Extension Host 변경은 **디버그 세션을 다시 시작해야** 합니다(디버그 도구 모음의 ⟳ 버튼 또는 호스트 창에서 `Cmd+R`).

### 스크립트

```bash
yarn compile      # 1회성 개발 빌드(webpack development)
yarn watch        # 워치 모드 개발 빌드
yarn build        # 프로덕션 빌드(webpack production, 패키징 전에 자동 실행)
yarn test         # Jest 단위 테스트 실행
yarn lint         # ESLint
yarn package      # 배포용 .vsix 생성("릴리스 패키지 만들기" 참고)
```

### 디버깅 참고 사항

- **양방향 메시징**: WebView와 Extension Host는 `postMessage`로 통신하며 메시지 타입은 `extensions/frontend/src/shared/messages.ts`에 있습니다. 양쪽 모두 `dispatch` / `handle`을 거치므로 디버깅은 여기서 시작하세요.
- **CLI 호출**: 모든 `ocr` 하위 명령은 `src/extension/services/CliService.ts`의 `child_process.spawn`으로 실행됩니다. `runRaw`는 CLI 종료 코드가 0이 아니면 reject하고 stderr의 `Error:` 텍스트를 포함하므로 "검토 실패/연결 실패"를 진단하는 데 도움이 됩니다.
- **설정 읽기/쓰기**: `ConfigService`는 `~/.opencodereview/config.json`을 읽고 쓰기는 `ocr config set`에 위임합니다. WebView 필드는 camelCase(예: `useAnthropic`), 디스크/CLI 쪽은 snake_case(예: `use_anthropic`)이며 변환은 `src/extension/services/configParse.ts`에 있습니다.

---

## 빌드

### 산출물만 컴파일

```bash
yarn build        # 프로덕션 빌드(webpack production)
```

산출물: `out/extension.js`(Extension Host) + `out/webview.js`(WebView SPA).

### 릴리스 패키지(.vsix) 만들기

```bash
yarn package      # = vsce package --no-yarn
```

이 명령은 다음을 수행합니다:

1. `vscode:prepublish`를 실행해 `yarn build` 프로덕션 빌드를 수행합니다;
2. `.vscodeignore`에 따라 소스, 테스트, 개발 파일을 제외합니다;
3. 현재 디렉터리에 `open-code-review-vscode-<version>.vsix`를 생성합니다.

> 패키징 도구는 devDependency로 설치되는 `@vscode/vsce`이므로 전역 설치나 네트워크 다운로드가 필요 없습니다. `--no-yarn`은 vsce의 기본 npm 의존성 트리 검사를 건너뜁니다(이 프로젝트는 Yarn 사용).

릴리스 패키지에는 실행에 필요한 파일만 포함됩니다: `package.json`, `README.md`, `resources/icon.svg`, `out/extension.js`, `out/webview.js`.

### 로컬 설치 / 확인

```bash
code --install-extension open-code-review-vscode-<version>.vsix
```

또는 VS Code에서: 확장 패널 → 오른쪽 위 `⋯` → **Install from VSIX…** → 생성한 `.vsix` 파일을 선택합니다.

> Marketplace에 게시하려면 `vsce publish`를 사용합니다(publisher 계정과 PAT 필요). 일상적인 배포에는 위 `.vsix`로 충분합니다.

---

## 아키텍처

**Monolithic WebView + Thin Extension Host** 구조입니다:

- **WebView**는 별도로 빌드되는 Preact SPA로, 공유 프런트엔드 `extensions/frontend/`에서 빌드됩니다(IntelliJ IDEA 플러그인과도 공유).
- **Extension Host** 계층은 얇게 유지하며 CLI 호출, 파일 시스템, Git 작업, 에디터 댓글만 담당합니다.
- 두 계층은 `postMessage`로 통신하고 `extensions/frontend/src/shared/`의 공유 TypeScript 타입으로 타입 안전성을 확보합니다.

```
src/
└── extension/          Extension Host(Node.js): services / providers / commands
```

WebView와 공유 타입은 공유 프런트엔드 `extensions/frontend/`에서 빌드됩니다(IntelliJ IDEA 플러그인과도 공유).

---

## License

Apache-2.0
