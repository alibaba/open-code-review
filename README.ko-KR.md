<div align="center">
  <a href="https://alibaba.github.io/open-code-review/">
    <img src="imgs/logo-core.svg" alt="OpenCodeReview logo" width="180" />
  </a>
  <h1>OpenCodeReview</h1>
</div>

<p align="center">
  <a href="https://trendshift.io/repositories/41087" target="_blank">
    <img src="https://trendshift.io/api/badge/trendshift/repositories/41087/weekly?language=Go" alt="alibaba%2Fopen-code-review | Trendshift" style="width: 320px; height: 70px;" width="320" height="70" />
  </a>
</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://goreportcard.com/report/github.com/alibaba/open-code-review"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://deepwiki.com/alibaba/open-code-review"><img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg" /></a>
  <a href="https://www.bestpractices.dev/projects/13328"><img alt="OpenSSF Best Practices" src="https://www.bestpractices.dev/projects/13328/badge" /></a>
</p>
<p align="center">
  <a href="#supported-platforms"><img alt="Windows" src="https://img.shields.io/badge/Windows-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="macOS" src="https://img.shields.io/badge/macOS-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="Linux" src="https://img.shields.io/badge/Linux-supported-blue.svg" /></a>
  <a href="#supported-agents"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Codex" src="https://img.shields.io/badge/Codex-supported-blueviolet.svg" /></a>
</p>
<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja-JP.md">日本語</a> | 한국어 | <a href="README.ru-RU.md">Русский</a>
</p>

---

## Open Code Review란?

Open Code Review는 AI 기반 코드 리뷰 CLI 도구입니다. Alibaba Group의 내부 공식 AI 코드 리뷰 어시스턴트에서 시작했으며, 지난 2년 동안 수만 명의 개발자에게 제공되어 수백만 건의 코드 결함을 찾아냈습니다. 대규모 환경에서 충분히 검증한 뒤 커뮤니티를 위해 오픈 소스 프로젝트로 공개했습니다. 모델 endpoint만 설정하면 바로 사용할 수 있습니다.

이 도구는 Git diff를 읽고, 변경 파일을 tool-use 기능을 가진 agent를 통해 설정 가능한 LLM으로 전달한 뒤, 라인 단위 위치 정보가 포함된 구조화된 리뷰 코멘트를 생성합니다. agent는 전체 파일 내용 읽기, 코드베이스 검색, 다른 변경 파일 확인 등을 통해 맥락을 확보하고 표면적인 diff 피드백이 아닌 깊이 있는 리뷰를 수행할 수 있습니다.

![Highlights](imgs/highlights-en.png)

## 벤치마크

> 범용 Agent(Claude Code)와 비교할 때, Open Code Review는 동일한 기반 모델에서 유의미하게 높은 **정밀도(Precision)**와 **F1 점수**를 달성하며, 토큰 소비량은 **약 1/9** 수준이고 리뷰 속도도 더 빠릅니다. 다만 재현율(Recall)은 범용 Agent보다 낮습니다 — 이는 노이즈를 줄이고 정밀도를 우선하는 설계적 트레이드오프입니다.

실제 코드 리뷰 기반 벤치마크. **50**개 인기 오픈소스 저장소에서 **200**개 실제 Pull Request를 엄선하고, **10**개 프로그래밍 언어를 커버 — 80명 이상의 시니어 엔지니어가 교차 검증(**1,505**개 어노테이션된 결함).

| 지표 | 측정 내용 | 중요한 이유 |
|------|-----------|-------------|
| **F1** | 정밀도와 재현율의 조화 평균 | 리뷰 품질을 나타내는 최적의 단일 지표 |
| **정밀도 (Precision)** | 보고된 이슈 중 실제 결함 비율 | 높을수록 확인할 오탐이 적음 |
| **재현율 (Recall)** | 실제 결함 중 발견된 비율 | 높을수록 놓치는 이슈가 적음 |
| **평균 시간 (Avg Time)** | 리뷰당 실제 소요 시간 | CI 파이프라인 대기 시간에 영향 |
| **평균 토큰 (Avg Token)** | 리뷰당 총 토큰 소비량 | API 비용에 직접 영향 |

![Benchmark](imgs/benchmark-en.png)

## 왜 Open Code Review인가?

### 범용 Agent의 문제

Claude Code Skills 같은 범용 agent로 코드 리뷰를 해봤다면 다음 문제를 경험했을 수 있습니다.

- **불완전한 커버리지**: 큰 changeset에서는 일부 파일만 선택적으로 리뷰하고 중요한 파일을 놓치기 쉽습니다.
- **위치 드리프트**: 지적된 문제가 실제 코드 위치와 맞지 않거나 라인 번호와 파일 참조가 어긋나는 일이 자주 발생합니다.
- **불안정한 품질**: 자연어 기반 Skill은 디버깅이 어렵고, 작은 prompt 변화에도 리뷰 품질이 크게 흔들릴 수 있습니다.

근본 원인은 순수 언어 중심 아키텍처가 리뷰 프로세스에 강한 제약을 제공하지 못한다는 점입니다.

### 핵심 설계: 결정적 엔지니어링과 Agent의 하이브리드

Open Code Review의 핵심 철학은 결정적 엔지니어링과 agent를 결합해 각자가 가장 잘하는 일을 맡기는 것입니다.

**결정적 엔지니어링: 강한 제약**

반드시 정확해야 하는 리뷰 단계는 언어 모델이 아니라 엔지니어링 로직이 보장합니다.

- **정확한 파일 선택**: 어떤 파일을 리뷰하고 어떤 파일을 필터링할지 결정해 중요한 변경이 누락되지 않도록 합니다.
- **스마트 파일 번들링**: 관련 파일을 하나의 리뷰 단위로 묶습니다. 예를 들어 `message_en.properties`와 `message_zh.properties`를 함께 묶습니다. 각 번들은 독립된 context를 가진 sub-agent로 실행되며, 대규모 changeset에서도 안정적인 divide-and-conquer 전략과 동시 리뷰를 지원합니다.
- **세밀한 rule 매칭**: 각 파일의 특성에 맞는 리뷰 rule을 매칭해 모델의 주의를 집중시키고 정보 노이즈를 줄입니다. 순수 자연어 기반 rule 안내보다 template engine 기반 rule 매칭이 더 안정적이고 예측 가능합니다.
- **외부 위치 지정 및 reflection 모듈**: 독립적인 comment positioning과 comment reflection 모듈이 AI 피드백의 위치 정확도와 내용 정확도를 체계적으로 개선합니다.

**Agent: 동적 의사결정**

agent의 강점은 동적 판단과 동적 context 검색이 중요한 지점에 집중됩니다.

- **시나리오 최적화 prompt**: 코드 리뷰에 깊이 최적화된 prompt template으로 효과를 높이고 token 사용량을 줄입니다.
- **시나리오 최적화 toolset**: 대규모 production 데이터의 tool-call trace를 분석해 도출했습니다. 호출 빈도 분포, tool별 반복률, 신규 tool이 전체 call chain에 미치는 영향 등을 반영해 범용 agent toolkit보다 코드 리뷰에 더 안정적이고 예측 가능한 전용 toolset을 제공합니다.

## 사용 방법

### CLI

#### 설치

**NPM 사용(권장)**

```bash
npm install -g @alibaba-group/open-code-review
```

설치 후 `ocr` 명령을 전역에서 사용할 수 있습니다.

**GitHub Release 사용**

명령 한 번으로 사용 중인 OS/아키텍처에 맞는 최신 binary를 설치합니다 (macOS / Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

이 스크립트는 알맞은 릴리스 binary를 선택하고 SHA-256 체크섬을 검증한 뒤 `ocr`로 `/usr/local/bin`에 설치합니다. 설치 위치는 `OCR_INSTALL_DIR`로, 릴리스 버전은 `OCR_VERSION`으로 재정의할 수 있습니다:

```bash
OCR_INSTALL_DIR="$HOME/.local/bin" OCR_VERSION=v1.3.13 \
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh)"
```

<details>
<summary>수동 다운로드 (Windows 포함 모든 플랫폼)</summary>

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)에서 사용 중인 플랫폼의 binary를 다운로드합니다.

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (x86_64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (ARM64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (x86_64): ocr.exe를 PATH에 포함된 디렉터리로 이동하세요
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64): ocr.exe를 PATH에 포함된 디렉터리로 이동하세요
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

</details>

**소스에서 빌드**

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### Quick Start

**1. LLM 설정**

**코드 리뷰를 실행하기 전에 반드시 LLM을 설정해야 합니다.**

**Option A: 대화형 설정 (권장)**

```bash
ocr config provider          # built-in provider 선택 또는 custom provider 추가
ocr config model             # 활성 provider의 model 선택
```

![Provider setup](imgs/providers.jpg)

**Option B: 수동 설정**

```bash
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token your-api-key-here
ocr config set llm.model claude-opus-4-6
ocr config set llm.use_anthropic true
```

config는 `~/.opencodereview/config.json`에 저장됩니다.

**`auth_header` (선택사항):** Anthropic 사용 시 API key를 전송할 HTTP header를 제어합니다. 생략하면 기본값은 `authorization` (Bearer token)입니다. 표준 `sk-ant-*` API key를 사용하는 경우 `x-api-key`로 설정해야 합니다:

```bash
ocr config set llm.auth_header x-api-key
```

지원되는 값: `x-api-key`, `authorization` (별칭: `bearer`). 그 외 값은 오류로 처리됩니다.

**Option C: 환경 변수(가장 높은 우선순위)**

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

Claude Code 환경 변수(`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`)와도 호환되며, `~/.zshrc` / `~/.bashrc`의 export도 파싱합니다.

> **CC-Switch 사용자 참고**: [CC-Switch](https://github.com/farion1231/cc-switch)를 [routing service](https://www.ccswitch.io/en/docs?section=proxy&item=service)와 함께 사용한다면, 추가 설정 없이 `llm.url`을 CC-Switch proxy 주소로 지정할 수 있습니다.
> - **Claude** provider: `llm.url`을 `http://127.0.0.1:15721`로 설정
> - **Codex** provider: `llm.url`을 `http://127.0.0.1:15721/v1`로 설정
> - provider 설정에 맞게 `llm.model` 설정
> - `llm.auth_token`은 아무 값이나 사용할 수 있음
> - `extra_body` 설정은 그대로 적용됨

**2. 연결 테스트**

```bash
ocr llm test
```

**3. 리뷰 실행**

```bash
cd your-project

# Workspace mode: staged, unstaged, untracked 변경을 모두 리뷰
ocr review

# Branch range: 두 ref 비교
ocr review --from main --to feature-branch

# 단일 commit
ocr review --commit abc123
```

### Coding Agent와 통합

OCR은 AI coding agent에 slash command로 자연스럽게 통합할 수 있으며, agent workflow 안에서 바로 코드 리뷰를 실행할 수 있습니다.

#### Option 1: Skill로 설치

`npx`로 OCR skill을 프로젝트에 설치합니다.

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

이 명령은 [skills registry](skills/open-code-review/SKILL.md)의 `open-code-review` skill을 설치합니다. 이 skill은 coding agent가 `ocr`을 호출해 코드 리뷰를 수행하고, issue를 우선순위별로 분류하며, 필요한 경우 fix를 적용하는 방법을 알려줍니다.

#### Option 2: Claude Code Plugin으로 설치

[Claude Code](https://docs.anthropic.com/en/docs/claude-code)에서는 Claude Code 안에서 다음 명령으로 command plugin을 설치합니다.

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

이렇게 하면 OCR을 실행하고 issue를 자동으로 필터링 및 수정하는 `/open-code-review:review` slash command가 등록됩니다.

#### Option 3: Codex Plugin으로 설치

local Codex에서는 이 repository에서 Open Code Review plugin을 설치합니다.

```bash
codex plugin marketplace add alibaba/open-code-review
codex
/plugins
```

local checkout이나 fork에서는 다음을 사용할 수 있습니다.

```bash
codex plugin marketplace add .
codex
/plugins
```

`Open Code Review`를 설치하고 활성화한 뒤, 새 Codex thread를 시작해 명시적으로 호출합니다.

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

이 plugin은 local OCR CLI를 실행하는 Codex skill을 등록합니다.

```bash
ocr review --audience agent
```

이 통합은 OCR의 내부 LLM backend를 변경하지 않으며 Codex용 OpenAI Responses API endpoint 설정을 요구하지 않습니다. OCR 자체는 CLI 설정 섹션에 설명된 대로 `ocr` CLI 설치와 설정이 필요합니다.

한국어 가이드: [`plugins/open-code-review/CODEX.ko-KR.md`](plugins/open-code-review/CODEX.ko-KR.md)

#### Option 4: Command 파일 직접 복사

package manager 없이 빠르게 설정하려면 command 파일을 복사해 Claude Code에서 `/open-code-review` slash command를 사용할 수 있습니다.

**Project-level**(git으로 팀과 공유):

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

**User-level**(여러 프로젝트에서 개인 전역 사용):

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

> **전제 조건**: 모든 통합 방식은 `ocr` CLI가 설치되어 있고 LLM이 설정되어 있어야 합니다. 위의 [설치](#설치)와 [LLM 설정](#1-llm-설정)을 참고하세요.

### CI/CD 통합

OCR은 CI/CD pipeline에 통합해 Merge Request / Pull Request 코드 리뷰를 자동화할 수 있습니다.

CI 통합의 핵심 명령:

```bash
ocr review \
  --from "origin/main" \
  --to "origin/feature-branch" \
  --format json
```

`--format json` flag는 CI script에서 파싱하기 좋은 machine-readable 결과를 출력합니다.

통합 예시는 [`examples/`](./examples/) 디렉터리를 참고하세요.

- [`github_actions/`](./examples/github_actions/): GitHub Actions 통합 예시
- [`gitlab_ci/`](./examples/gitlab_ci/): GitLab CI 통합 예시

## Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `ocr review` | `ocr r` | 코드 리뷰 시작 |
| `ocr rules check <file>` | - | 파일 경로에 적용될 리뷰 rule 미리보기 |
| `ocr config provider` | - | 대화형 provider 설정 (built-in, custom, 수동) |
| `ocr config model` | - | 활성 provider의 대화형 model 선택 |
| `ocr config set <key> <value>` | - | config 값 설정 |
| `ocr config unset custom_providers.<name>` | - | custom provider 삭제 |
| `ocr llm test` | - | LLM 연결 테스트 |
| `ocr llm providers` | - | built-in LLM provider 목록 표시 |
| `ocr viewer` | `ocr v` | `localhost:5483`에서 WebUI session viewer 실행 |
| `ocr version` | - | version 정보 표시 |

### `ocr review` Flags

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--repo` | - | current dir | Git repository root |
| `--from` | - | - | Source ref 예: `main` |
| `--to` | - | - | Target ref 예: `feature-branch` |
| `--commit` | `-c` | - | 리뷰할 단일 commit |
| `--preview` | `-p` | `false` | LLM 실행 없이 리뷰 대상 파일 미리보기 |
| `--format` | `-f` | `text` | Output format: `text` 또는 `json` |
| `--concurrency` | - | `8` | 최대 동시 파일 리뷰 수 |
| `--timeout` | - | `10` | 동시 task timeout(분) |
| `--audience` | - | `human` | `human`(progress 표시) 또는 `agent`(summary only) |
| `--background` | `-b` | - | 리뷰를 위한 선택적 요구사항/비즈니스 컨텍스트. `--commit` 사용 시 미지정이면 commit message에서 자동 추출 |
| `--model` | - | - | 이번 리뷰에서 LLM model 선택 또는 override |
| `--rule` | - | - | custom JSON review rules 경로 |
| `--max-tools` | - | built-in | 파일별 최대 tool call round. template default보다 클 때만 적용 |
| `--max-git-procs` | - | built-in | 최대 동시 git subprocess 수 |
| `--tools` | - | - | custom JSON tools config 경로 |

## Examples

```bash
# 대화형 provider 및 model 설정
ocr config provider
ocr config model
ocr llm providers

# custom provider 삭제
ocr config unset custom_providers.my-gateway

# 리뷰 대상 파일 미리보기(LLM call 없음)
ocr review --preview
ocr review -c abc123 -p

# default 설정으로 workspace 변경 리뷰
ocr review

# 더 높은 concurrency로 branch diff 리뷰
ocr review --from main --to my-feature --concurrency 4

# 특정 commit을 verbose JSON output으로 리뷰
ocr review --commit abc123 --format json --audience agent

# 이번 리뷰에서 model 선택 또는 override
ocr review --model claude-opus-4-6
ocr review --commit abc123 --model claude-sonnet-4-6

# 요구사항 컨텍스트를 제공하여 더 정확한 리뷰 수행
ocr review --background "로그인 API에 rate limiting 추가"

# custom review rules 사용
ocr review --rule /path/to/my-rules.json

# 파일에 적용될 rule 미리보기
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# browser에서 review session history 보기
ocr viewer
ocr viewer --addr :3000
```

### Viewer 보안

viewer는 session JSONL 내용(LLM request messages와 responses)을 HTTP로 제공합니다. 모든 request에 대해 Host header allowlist를 적용합니다. loopback 이름(`localhost`, `127.0.0.0/8`, `::1`)과 실제 bind host는 항상 허용됩니다. wildcard bind(`--addr :3000`, `--addr 0.0.0.0:3000`)와 다른 non-loopback hostname은 `OCR_VIEWER_ALLOWED_HOSTS` 환경 변수에 comma-separated 값으로 추가해야 합니다.

```bash
OCR_VIEWER_ALLOWED_HOSTS=review.internal,ocr.lan ocr viewer --addr :3000
```

이 설정은 local viewer를 대상으로 하는 DNS rebinding 공격을 차단합니다.

## Review Rules

OCR은 네 계층의 priority chain으로 review rule을 해석합니다. 각 계층은 first-match-wins 방식입니다. 파일 경로가 pattern에 match되면 해당 rule을 사용하고, 아니면 다음 계층으로 넘어갑니다.

| Priority | Source | Path | Description |
|----------|--------|------|-------------|
| 1 (highest) | `--rule` flag | User-specified path | CLI explicit override |
| 2 | Project config | `<repoDir>/.opencodereview/rule.json` | project별 rule, git commit 가능 |
| 3 | Global config | `~/.opencodereview/rule.json` | user-wide 개인 선호 |
| 4 (lowest) | System default | Embedded `system_rules.json` | 일반 language와 file type을 다루는 built-in rule |

### Rule File Format

모든 계층은 같은 JSON format을 공유합니다.

```json
{
  "rules": [
    {
      "path": "force-api/**/*.java",
      "rule": "All new methods must validate required parameters for null values"
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags"
    }
  ]
}
```

- `path`는 `**` recursive matching과 `{java,kt}` brace expansion을 지원합니다.
- 각 계층 안에서는 rule이 선언 순서대로 평가되며 첫 번째 match가 선택됩니다.
- rule file이 없으면 조용히 건너뜁니다.

## Configuration Reference

Config file: `~/.opencodereview/config.json`

| Key | Type | Example |
|-----|------|---------|
| `provider` | string | `anthropic` \| `openai` \| `dashscope` \| `deepseek` \| `z-ai` |
| `providers.<name>.api_key` | string | Provider별 API key |
| `providers.<name>.url` | string | Provider base URL override |
| `providers.<name>.protocol` | string | `anthropic` \| `openai` |
| `providers.<name>.model` | string | Provider의 model 이름 |
| `providers.<name>.models` | array | 대화형 선택에 사용할 optional provider model 목록 |
| `providers.<name>.auth_header` | string | `x-api-key` \| `authorization` |
| `custom_providers.<name>.*` | — | optional `models`를 포함한 `providers.<name>.*`과 동일한 필드 |
| `llm.url` | string | `https://api.openai.com/v1/chat/completions` |
| `llm.auth_token` | string | `sk-xxxxxxx` |
| `llm.auth_header` | string | Anthropic only: `x-api-key` \| `authorization` |
| `llm.model` | string | `claude-opus-4-6` |
| `llm.use_anthropic` | boolean | `true` \| `false` |
| `language` | string | 임의의 언어 이름, 예: `English`, `Chinese` (기본값: `English`) |
| `telemetry.enabled` | boolean | `true` \| `false` |
| `telemetry.exporter` | string | `console` \| `otlp` |
| `telemetry.otlp_endpoint` | string | OTLP collector address |
| `telemetry.content_logging` | boolean | telemetry에 prompt 포함 여부 |

환경 변수는 config file보다 우선합니다.

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `OCR_LLM_URL` | LLM API endpoint URL |
| `OCR_LLM_TOKEN` | API key / auth token |
| `OCR_LLM_AUTH_HEADER` | Anthropic auth header (`x-api-key` 또는 `authorization`) |
| `OCR_LLM_MODEL` | Model name |
| `OCR_USE_ANTHROPIC` | `true` = Anthropic, `false` = OpenAI |

## Telemetry

관측성을 위한 OpenTelemetry 통합(spans, metrics)입니다. 기본값은 disabled입니다.

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

exported data에 LLM prompt와 response를 포함하려면 `telemetry.content_logging`을 설정합니다.

## Contributing

개발 환경 설정, coding guideline, pull request 제출 방법은 [CONTRIBUTING.ko-KR.md](CONTRIBUTING.ko-KR.md)를 참고하세요.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=alibaba/open-code-review&type=Date)](https://star-history.com/#alibaba/open-code-review&Date)

## License

[Apache-2.0](LICENSE) Copyright 2026 Alibaba
