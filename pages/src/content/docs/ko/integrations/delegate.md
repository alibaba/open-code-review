---
title: 위임 모드
sidebar:
  order: 5
---

위임 모드(Delegation Mode)에서는 OCR이 결정적인 엔지니어링(파일 선택, 규칙 해석)을
맡고, 실제 코드 리뷰는 호스트 Agent가 자신의 LLM으로 수행합니다. OCR 쪽에는 LLM
엔드포인트가 필요 없습니다.

## 언제 쓰나 {#when-to-use-delegation-mode}

위임 모드는 구독형 AI 코딩 Agent를 염두에 두고 만들었습니다. Claude Code, Codex,
Cursor, Open Code, Qoder처럼 이미 호스트 Agent에 LLM 구독이 묶여 있는 환경입니다.
OCR용으로 모델 엔드포인트를 따로 설정하는 대신, 호스트 Agent가 쓰던 구독 할당량을
그대로 리뷰에 씁니다.

다음과 같을 때 위임 모드를 쓰세요.

1. AI 코딩 Agent를 구독 요금제로 쓰고 있고, 그 할당량을 코드 리뷰에도 쓰고 싶을 때.
   API 키나 모델 설정을 따로 하지 않아도 됩니다.
2. OCR의 엔지니어링 뼈대(파일 필터링, 규칙 해석, 제외 처리)만 쓰고 LLM 추론은 전부
   호스트 Agent에 맡기고 싶을 때.
3. 자체 Agent 파이프라인을 만들면서 리뷰 단계에 구조화된 입력(파일 목록 + 규칙)이
   필요할 때.

## 사전 요구 사항 {#prerequisites}

`ocr` CLI가 설치되어 있어야 합니다:

```bash
which ocr || npm install -g @alibaba-group/open-code-review
```

LLM 설정(`ocr config set …`이나 환경 변수)은 필요 없습니다. 위임 모드에서는 OCR이
LLM을 호출하지 않습니다.

## 스킬 / 명령 설치 {#install-the-skill-command}

### Claude Code — 명령 {#claude-code-command}

```bash
mkdir -p .claude/commands
curl -o .claude/commands/delegate-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

### 모든 Agent — 스킬 {#any-agent-skill}

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

매니페스트를 직접 복사해도 됩니다:

```bash
cp -R /path/to/open-code-review/skills/open-code-review-delegate ~/.claude/skills/
```

## 워크플로 {#workflow}

### 1단계: 미리 보기 — 리뷰 대상 확인 {#step-1-preview-determine-what-to-review}

```bash
ocr delegate preview [--from <ref> --to <ref>] [--commit <hash>] [--exclude <patterns>]
```

출력 내용:

- **mode** — workspace / range / commit
- **ref 메타데이터** — from, to, commit, merge\_base
- **리뷰 대상 파일 목록** — 경로, 상태, 추가/삭제 라인 수
- **제외된 파일** — 제외 사유 포함

자주 쓰는 호출:

| 상황 | 명령 |
|----------|---------|
| 워크스페이스 변경 | `ocr delegate preview` |
| 브랜치 비교 | `ocr delegate preview --from main --to feature` |
| 단일 커밋 | `ocr delegate preview -c abc123` |

### 2단계: 파일별 규칙 가져오기 {#step-2-get-rules-for-files}

```bash
ocr delegate rule <path1> <path2> ...
```

1단계에서 얻은 리뷰 대상 경로를 넘깁니다. 출력은 규칙 내용별로 묶여 나오므로, 같은
규칙을 쓰는 파일은 한 그룹에 모여 중복이 없습니다.

### 3단계: diff 가져오기 {#step-3-get-diffs}

1단계에서 얻은 모드와 ref 정보를 바탕으로 git을 직접 씁니다.

**range 모드**(merge\_base가 제공됨):
```bash
git diff <merge_base>..<to> -- <path>
```

**commit 모드**:
```bash
git show <commit> -- <path>
```

**workspace 모드**:
```bash
git diff HEAD -- <path>        # 추적 중인 파일
cat <path>                     # 새로 추가된 추적되지 않은 파일
```

### 4단계: 파일별 리뷰 {#step-4-review-each-file}

리뷰 대상 파일마다 다음을 수행합니다.

1. 해당 파일의 diff를 가져옵니다(3단계).
2. 대응하는 Rule Group(2단계)을 리뷰 체크리스트로 삼습니다.
3. 필요한 만큼 맥락을 탐색하며 꼼꼼히 리뷰합니다.

### 5단계: 보고 {#step-5-report}

발견한 문제를 심각도로 분류합니다.

- **Critical/High** — 버그, 보안 이슈, 데이터 유실 위험. 항상 보고합니다.
- **Medium** — 성능 문제, 오류 처리 누락. 맥락과 함께 보고합니다.
- **Low** — 스타일 지적, 사소한 제안. 분명히 가치 있는 경우가 아니면 조용히 버립니다.

## 하위 명령 레퍼런스 {#sub-commands-reference}

| 명령 | 용도 |
|---------|---------|
| `ocr delegate preview` | 리뷰 대상 파일과 모드/ref 메타데이터를 나열 |
| `ocr delegate rule <path...>` | 리뷰 규칙을 내용별로 묶어 해석 |

## 공통 플래그 {#shared-flags}

| 플래그 | 설명 |
|------|-------------|
| `--from <ref>` | range 모드의 원본 ref |
| `--to <ref>` | range 모드의 대상 ref |
| `-c, --commit <hash>` | 단일 커밋 모드 |
| `--repo <path>` | 저장소 루트(기본값: 현재 디렉터리) |
| `--rule <path>` | 커스텀 rule.json 경로 |
| `--exclude <patterns>` | 쉼표로 구분한 제외 패턴 |
| `-b, --background <text>` | 비즈니스 맥락 |
| `-B, --background-file <path>` | Markdown 파일에서 읽는 비즈니스 맥락(`-b`보다 우선) |

## 다른 연동 방식과 비교 {#comparison-with-other-integration-modes}

| 방식 | LLM을 호출하는 주체 | 용도 |
|------|-------------------|----------|
| [Agent Skill](../agent-skill/) | OCR | Agent가 `ocr review`를 호출하고, OCR이 리뷰 전체를 주도 |
| [Command (Claude Code)](../claude-code/) | OCR | Claude Code의 슬래시 명령. OCR이 리뷰를 주도 |
| **위임 모드** | 호스트 Agent | OCR은 뼈대만 제공하고, Agent가 리뷰를 주도 |

## 관련 문서 {#see-also}

- [Agent Skill](../agent-skill/) — Agent를 대신해 OCR이 리뷰 전체를 수행합니다.
- [Command (Claude Code)](../claude-code/) — 자동 수정이 기본인 슬래시 명령
  형태입니다.
