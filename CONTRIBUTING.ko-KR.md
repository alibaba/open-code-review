# OpenCodeReview 기여 가이드

OpenCodeReview에 기여해 주셔서 감사합니다. 오타 수정, bug report, 새 기능 구현 등 모든 기여는 프로젝트에 도움이 됩니다.

[English](CONTRIBUTING.md) | [简体中文版](CONTRIBUTING.zh-CN.md) | [日本語版](CONTRIBUTING.ja-JP.md) | 한국어

## Code of Conduct

이 프로젝트에 참여하는 모든 사람은 서로를 존중하고 포용적인 환경을 유지해야 합니다. 모든 interaction에서 친절하고 건설적인 태도를 유지해 주세요.

## 기여 방법

코드 작성 외에도 여러 방식으로 기여할 수 있습니다.

- **Bug report**: 문제가 있으면 재현 단계와 함께 issue를 열어 주세요.
- **Feature 제안**: 개선 아이디어가 있다면 [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)에서 논의를 시작하거나 [Feature Request](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml) issue를 열 수 있습니다.
- **문서 개선**: 오타 수정, 설명 보완, 예시 추가를 환영합니다. 문서 문제를 보고하려면 [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml)를 열어 주세요.
- **Pull request 리뷰**: 다른 contributor의 코드를 리뷰하는 것도 큰 도움이 됩니다.
- **코드 작성**: bug fix, feature 추가, performance 개선 등을 기여할 수 있습니다.

## 시작하기

### 전제 조건

- [Go 1.25+](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Setup

```bash
# 1. GitHub에서 repository를 fork합니다

# 2. fork를 clone합니다
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review

# 3. main repo에서 update를 가져오기 위해 upstream remote를 추가합니다
git remote add upstream https://github.com/alibaba/open-code-review.git

# 4. project를 build합니다
make build

# 5. test를 실행합니다
make test
```

모든 과정이 통과하면 기여를 시작할 준비가 된 것입니다.

> **참고:** `upstream` remote는 contributor에게 read-only입니다. main repository의 최신 변경을 가져오는 데 사용하며 직접 push할 수 없습니다. 모든 기여는 fork(`origin`)에 push한 뒤 Pull Request로 제출해야 합니다.

## 개발 workflow

### Branching

`main`에서 feature branch를 만듭니다.

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature-name
```

변경 유형을 나타내기 위해 prefix를 사용합니다.

| Prefix | Purpose |
|--------|---------|
| `feat/` | 새 기능 |
| `fix/` | bug fix |
| `docs/` | 문서 변경만 포함 |
| `refactor/` | 동작 변화 없는 code refactoring |
| `test/` | test 추가 또는 수정 |
| `chore/` | build, CI, tooling 변경 |

### Commit Messages

[Conventional Commits](https://www.conventionalcommits.org/) 형식을 따릅니다.

```text
<type>(<scope>): <short summary>

[optional body]
```

예시:

```text
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(README): update configuration examples
```

### Code Quality

변경을 제출하기 전에 모든 check가 통과하는지 확인하세요.

```bash
# Format and lint (Go standard tooling)
go fmt ./...
go vet ./...

# race detection과 함께 test 실행
make test

# build 성공 확인
make build
```

### Project Structure

```text
.
├── cmd/opencodereview/   # CLI entry point
├── internal/
│   ├── agent/            # Review agent logic
│   ├── config/           # Configuration management
│   ├── diff/             # Git diff parsing
│   ├── llm/              # LLM API client (Anthropic & OpenAI)
│   ├── model/            # Data models
│   ├── session/          # Review session management
│   ├── tool/             # Built-in tools (file_read, code_search, etc.)
│   ├── telemetry/        # OpenTelemetry integration
│   └── viewer/           # WebUI session viewer
├── pages/                # WebUI frontend
├── scripts/              # Build & install scripts
└── bin/                  # NPM wrapper
```

## 문서 기여

문서는 OpenCodeReview의 중요한 일부입니다. README, inline code comment, configuration example, 사용자에게 노출되는 모든 text의 개선을 환영합니다.

### 문서 기여에 해당하는 작업

- 오타, 문법 오류, 깨진 link 수정
- 혼란스러운 설명을 명확히 하거나 빠진 맥락 추가
- command나 configuration option의 사용 예시 추가
- feature 변경 이후 오래된 내용 update
- 지역화 문서 번역 또는 개선(`README.zh-CN.md`, `README.ja-JP.md`, `README.ko-KR.md`, `CONTRIBUTING.zh-CN.md`, `CONTRIBUTING.ja-JP.md`, `CONTRIBUTING.ko-KR.md`)

### 문서 workflow

1. 문제를 발견했지만 직접 수정할 계획이 없다면 [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml)를 열어 주세요.
2. 직접 수정하려면 repo를 fork하고 변경한 뒤 `docs/` branch prefix로 PR을 제출하세요. 예: `docs/fix-config-example`.
3. 문서만 변경하는 PR은 test 변경이 필요하지 않지만, 포함한 command와 code snippet이 정확한지 확인해 주세요.

### 문서 파일

| File | Purpose |
|------|---------|
| `README.md` | main project documentation (English) |
| `README.zh-CN.md` | Chinese translation |
| `README.ja-JP.md` | Japanese translation |
| `README.ko-KR.md` | Korean translation |
| `CONTRIBUTING.md` | contribution guide (English) |
| `CONTRIBUTING.zh-CN.md` | contribution guide (Chinese) |
| `CONTRIBUTING.ja-JP.md` | contribution guide (Japanese) |
| `CONTRIBUTING.ko-KR.md` | contribution guide (Korean) |

## 변경 제출

### Issue 열기

큰 변경을 시작하기 전에는 먼저 issue를 열어 접근 방식을 논의해 주세요. 이렇게 하면 중복 작업을 줄이고 기여 방향이 프로젝트와 맞는지 확인할 수 있습니다.

Bug를 보고할 때는 다음 정보를 포함해 주세요.

1. OpenCodeReview version(`ocr version`)
2. OS와 architecture
3. 재현 단계
4. 기대 동작과 실제 동작
5. 관련 log 또는 error message

### Pull Request 절차

1. **PR을 focused하게 유지**: 하나의 PR에는 하나의 논리적 변경만 포함하세요. 서로 독립적인 변경이 여러 개라면 별도 PR로 제출하세요.
2. **Test 작성**: 동작 변경에는 test를 추가하거나 update하세요.
3. **문서 update**: 사용자에게 보이는 동작이 바뀌면 관련 문서를 update하세요.
4. **CLA 서명**: 모든 contributor는 PR이 merge되기 전에 Contributor License Agreement에 서명해야 합니다.
5. **PR template 작성**: 변경 내용과 이유를 설명하세요.

### PR title format

commit message와 같은 Conventional Commits 형식을 사용합니다.

```text
feat(agent): add support for custom tool definitions
```

### Review process

- maintainer가 보통 며칠 내에 PR을 리뷰합니다.
- 변경 요청이 있을 수 있습니다. 이는 일반적이고 협업적인 과정입니다.
- 승인되면 maintainer가 PR을 merge합니다.

## Contributor License Agreement (CLA)

모든 contributor는 Alibaba Open Source Contributor License Agreement에 서명해야 합니다. 이는 프로젝트가 license terms에 따라 배포될 수 있도록 하기 위한 절차입니다.

첫 PR을 열면 CLA bot이 안내 comment를 남깁니다. link를 따라 전자 서명하면 되며 보통 1분 정도면 끝납니다.

## 처음 기여하는 분들

처음이라면 다음 label이 붙은 issue를 찾아보세요.

- [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue): 시작하기 좋은 작고 범위가 명확한 task
- [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted): community help를 환영하는 issue

시작하기 좋은 영역:

- error message와 CLI output 개선
- test가 부족한 code path의 test 작성
- 문서 개선

## Community

- **Bug Reports**: [GitHub Issues](https://github.com/alibaba/open-code-review/issues)
- **Feature Suggestions**: [GitHub Discussions (Ideas)](https://github.com/alibaba/open-code-review/discussions/categories/ideas) 또는 [Feature Request Issue](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml)
- **Questions & Help**: OpenCodeReview 사용과 관련한 질문은 [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions)에 올릴 수 있습니다.

## License

OpenCodeReview에 기여하면 해당 기여가 [Apache License 2.0](LICENSE)에 따라 license되는 것에 동의하는 것입니다.
