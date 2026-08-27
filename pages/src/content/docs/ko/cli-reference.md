---
title: CLI 레퍼런스
sidebar:
  order: 6
---

모든 `ocr` 하위 명령과 플래그, 종료 동작을 정리한 레퍼런스입니다.

## 전역 사용법 {#global-usage}

```text
OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  session, sessions  List and inspect saved review sessions
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr review --background "Focus on auth"                                           Review with inline context
  ocr review -B ./docs/requirements.md                                              Review with context file
  ocr config provider                      Interactive provider setup
  ocr config model                         Interactive model selection
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr llm providers                        List built-in providers
  ocr session list                         List saved review sessions
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.
Use "ocr session -h" for more information about session inspection.

GitHub: https://github.com/alibaba/open-code-review
```

## 전역 플래그 {#global-flags}

모든 명령에서 쓸 수 있으며, 하위 명령 앞뒤 어디에 붙여도 됩니다
(`ocr --color=never review`와 `ocr review --color=never`는 같습니다).

| 플래그 | 기본값 | 하는 일 |
|---|---|---|
| `--color <auto\|always\|never>` | `auto` | ANSI 색을 언제 출력할지 정합니다. `auto`는 stdout이 터미널일 때만 색을 입히므로, 파이프나 리다이렉트로 넘기면 일반 텍스트가 나옵니다. `always`는 파이프를 거쳐도 색을 유지합니다(`\| less -R`에 유용). |

stdout이 터미널이 아니면 텍스트 출력에 색이 들어가지 않으므로 그대로 파이프로
넘길 수 있습니다:

```bash
ocr review --commit HEAD | gh issue comment 123 --body-file -
```

`TERM=dumb`으로도 색을 끌 수 있습니다.

## 명령 요약 {#command-summary}

| 명령 | 별칭 | 하는 일 |
|---|---|---|
| `ocr review` | `ocr r` | 코드 리뷰를 실행하고 코멘트를 출력합니다. |
| `ocr scan` | `ocr s` | Git diff 없이 파일 전체를 스캔합니다. |
| `ocr rules check <file>` | — | 주어진 파일 경로에 어떤 규칙이 적용되는지, 그 규칙이 어디서 왔는지 보여줍니다. |
| `ocr config set <key> <value>` | — | 설정값을 `~/.opencodereview/config.json`에 저장합니다. |
| `ocr config unset custom_providers.<name>` | — | 커스텀 프로바이더를 삭제합니다(활성 상태였다면 `provider`/`model`도 함께 지웁니다). |
| `ocr config provider` | — | 대화형 프로바이더 설정 TUI입니다. |
| `ocr config model` | — | 대화형 모델 선택 TUI입니다. |
| `ocr llm test` | — | 설정된 엔드포인트를 확인하려고 작은 채팅 요청을 보냅니다. |
| `ocr llm providers` | — | 내장 LLM 프로바이더를 모두 나열합니다. |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | 저장된 리뷰 세션을 나열합니다. |
| `ocr session show <id>` | `ocr sessions show <id>` | 세션 하나와 파일별 체크포인트를 살펴봅니다. |
| `ocr session comments <id>` | `ocr sessions comments <id>` | 세션에 기록된 리뷰 코멘트를 출력합니다. |
| `ocr session compare <before> <after>` | `ocr session diff <before> <after>` | 두 세션의 지적을 비교합니다: 새로 생긴 것, 남아 있는 것, 해결된 것, 리뷰하지 않은 것. |
| `ocr viewer` | — | 지난 리뷰 세션을 볼 수 있는 로컬 웹 UI를 띄웁니다(`localhost:5483`). |
| `ocr version` | — | 버전, 커밋, 플랫폼, 빌드 날짜, GitHub URL을 출력합니다. |

`ocr`과 `ocr -h`는 최상위 사용법을 출력합니다. 각 하위 명령도 `-h` / `--help`를
받습니다.

## `ocr review` {#ocr-review}

핵심 명령입니다. Git diff를 해석하고, 변경된 파일을 의미 단위로 묶고, 그룹마다 서브
Agent를 하나씩 붙여 리뷰 코멘트를 모아 출력합니다.

### 명령 형식 {#synopsis}

```text
ocr review [flags]
ocr r      [flags]   (alias)
```

플래그를 하나도 주지 않으면 **워크스페이스 모드**로 동작합니다. 현재 디렉터리가 속한
저장소의 스테이징된 변경, 스테이징되지 않은 변경, 추적되지 않은 파일을 모두
리뷰합니다.

### 플래그 {#flags}

| 플래그 | 단축 | 기본값 | 설명 |
|---|---|---|---|
| `--repo <path>` | — | 현재 디렉터리 | Git 저장소 루트. |
| `--from <ref>` | — | — | diff를 시작할 원본 ref(예: `main`). |
| `--to <ref>` | — | — | diff가 끝나는 대상 ref(예: `feature-branch`). 지정하면 OCR이 `merge-base(from, to)..to`를 계산합니다. |
| `--commit <sha>` | `-c` | — | 리뷰할 단일 커밋(부모 커밋과의 diff). |
| `--preview` | `-p` | `false` | 필터 파이프라인만 돌리고 LLM은 호출하지 않습니다. 파일 목록과 제외 사유를 출력합니다. `--format json`은 지원하지만 `--format sarif`는 지원하지 않습니다(미리 보기에는 내보낼 완료된 지적이 없습니다). |
| `--no-filter` | — | `false` | 리뷰 코멘트를 모두 남기고 그룹 단위 `REVIEW_FILTER_TASK` LLM 후처리 호출을 건너뜁니다. |
| `--resume <session-id>` | — | — | 호환되는 이전 range 또는 commit 리뷰 세션에서 이어서 실행합니다. |
| `--format <fmt>` | `-f` | `text` | `text`(사람이 읽는 형식), `json`(기계가 읽는 코멘트 배열), `sarif`(GitHub Code Scanning용 SARIF 2.1.0 리포트). |
| `--output <path>` | `-o` | stdout | 리뷰 결과를 UTF-8 파일로 씁니다(`-`는 stdout). 첫 쓰기 시점에 파일을 만들므로 실패한 실행은 기존 파일을 건드리지 않습니다. text 형식에서는 ANSI 색 코드를 자동으로 제거합니다. |
| `--audience <who>` | — | `human` | `human`은 진행 상황을 흘려보냅니다(`--format`이 `json`/`sarif`이면 stderr로 보내 stdout이 파싱 가능한 문서 하나로 유지됩니다). `agent`는 진행 상황을 아예 끄고 최종 요약이나 JSON만 출력합니다. |
| `--background <text>` | `-b` | — | plan과 main 프롬프트에 넣을 요구사항 또는 비즈니스 맥락(선택). |
| `--background-file <path>` | `-B` | — | 리뷰 배경으로 쓸 Markdown 파일 경로. `--background`와 함께 지정하면 이쪽이 우선합니다. |
| `--exclude <patterns>` | — | — | 제외할 gitignore 형식 패턴(쉼표 구분). `rule.json`의 `excludes` 항목과 합쳐집니다. |
| `--concurrency <n>` | — | `8` | 병렬로 리뷰할 파일 그룹의 최대 개수. |
| `--timeout <minutes>` | — | `10` | 그룹당 제한 시간. `0`이면 타임아웃을 끕니다. effort 프리셋이 리뷰 라운드를 두 번 이상 돌리면 자동으로 50% 늘어납니다. |
| `--effort <level>` | — | `medium` | 리뷰 강도 프리셋: `low`(라운드 1회), `medium`(2회), `high`(3회). 라운드를 늘리면 놓치는 지적이 줄지만 비용도 그만큼 늘어납니다. 이 실행에 한해 저장된 `effort` 설정을 덮어씁니다. |
| `--rule <path>` | — | — | 커스텀 JSON 리뷰 규칙 파일 경로. 프로젝트 수준과 전역 `rule.json`을 덮어씁니다. |
| `--max-tools <n>` | — | 템플릿 기본값 | 그룹당 최대 도구 호출 라운드 수. `0`이면 템플릿 기본값(`100`)을 쓰고, 1~49는 `50`으로 올려 맞춥니다. 이 플래그는 상한을 *올리기만* 합니다. 템플릿 기본값보다 낮은 값은 무시됩니다. |
| `--max-tokens <n>` | — | 설정 또는 템플릿 기본값 | 그룹당 프롬프트(입력) 토큰 상한이며 템플릿 기본값은 `200000`입니다. 이 실행에 한해 저장된 `max_tokens` 설정을 덮어씁니다. 출력 상한은 바뀌지 않습니다. `MAX_COMPLETION_TOKENS`를 참고하세요. |
| `--max-tokens-budget <n>` | — | `0`(무제한) | 리뷰 전체의 입력+출력 토큰 사용량을 제한합니다. 예산을 넘기면 작업 전달을 멈추지만 그때까지의 결과는 그대로 내보냅니다. |
| `--provider <name>` | — | — | 이 실행에 쓸 프로바이더를 고릅니다. `providers`와 `custom_providers` 양쪽의 이름을 모두 받습니다. |
| `--model <name>` | — | — | 이 실행에 한해 해석된 LLM 모델을 덮어씁니다(예: `claude-opus-4-6`). |
| `--max-git-procs <n>` | — | `16` | 동시에 띄울 git 서브프로세스의 최대 개수. |
| `--tools <path>` | — | 내장 | 커스텀 JSON 도구 설정 파일 경로. 내장 도구 정의를 덮어씁니다. |

> 모드 플래그는 함께 쓸 수 없습니다. `--from`/`--to`, `--commit`, 아무것도 주지
> 않기(워크스페이스 모드) 중 하나만 고르세요. 섞어 쓰면 오류로 중단됩니다.
> `--resume`은 range와 commit 리뷰만 지원하며 `--preview`와 함께 쓸 수 없습니다.

### 실행 단위 LLM 선택 {#per-run-llm-selection}

`review`와 `scan` 모두 `--provider`와 `--model`을 받습니다. 이 값은 해당 실행에만
적용되며 저장된 설정을 바꾸지 않습니다:

```bash
ocr review --provider anthropic --model claude-opus-4-6 --format json
ocr scan --provider openai --model gpt-5.4 --format json
```

`--provider`를 명시하면 일반적인 소스 해석에 앞서 `providers`나 `custom_providers`에
저장된 항목을 고릅니다. `--provider`가 없으면 OCR은 기존 소스 순서를 그대로
따릅니다. 저장된 설정, 완전한 `OCR_LLM_*` 환경 변수 설정, 완전한 Claude Code 환경
변수 설정, 그다음 셸 rc 파일 순입니다. `--model`은 어떤 소스가 선택되든 그 안에서
모델만 덮어쓰며, 소스 순서 자체는 바꾸지 않습니다. 조건을 다 갖추지 못한 방식은
섞이지 않고 그대로 다음으로 넘어갑니다. 내장 프로바이더를 골랐다면 자격 증명은
여전히 해당 프로바이더가 지원하는 환경 변수에서 올 수 있습니다.

### 모드 {#modes}

#### 워크스페이스 모드(기본값) {#workspace-mode-default}

```bash
ocr review
```

OCR은 git 명령 두 개로 작업 트리의 변경을 모읍니다.

- 추적 중인 변경은 `git diff HEAD`로 가져옵니다(스테이징된 변경과 그렇지 않은 변경을
  `HEAD` 기준으로 합칩니다. 결과가 비어 있으면 `git diff --staged`로 넘어갑니다).
- 추적되지 않은 파일은 `git ls-files --others --exclude-standard`로 찾아 디스크에서
  읽고, 파일 전체가 추가된 것으로 다룹니다.

커밋 직전에 보통 원하는 동작입니다. 범위를 좁히고 싶다면 필요한 것만 스테이징하세요.

#### range 모드 {#range-mode}

```bash
ocr review --from main --to feature-branch
```

OCR이 `merge-base(main, feature-branch)..feature-branch`를 계산하므로, 브랜치를 딴 뒤
`main`에 들어온 무관한 변경은 빠지고 그 기능 브랜치가 *만들어 낸* diff만 보입니다.

#### commit 모드 {#commit-mode}

```bash
ocr review --commit abc123
ocr review -c abc123
```

`git show abc123`이 만드는 diff, 즉 그 커밋 하나가 도입한 변경을 리뷰합니다.

### 중단된 리뷰 이어서 하기 {#resuming-interrupted-reviews}

`ocr review`는 실행할 때마다 세션 로그를 `~/.opencodereview/sessions/` 아래에
남깁니다. 텍스트 출력이 정상적으로 끝나면 리뷰 결과에만 집중하도록 세션 ID를 찍지
않습니다. 저장된 세션을 찾으려면 `ocr session list/show`를 쓰거나, `--format json`으로
기계가 읽는 출력에 `session_id`를 포함시키세요. range나 commit 리뷰가 중간에 끊겼다면
저장된 세션을 나열한 뒤 같은 리뷰 대상을 가진 세션에서 이어서 실행합니다:

```bash
ocr session list
ocr session show <session-id>
ocr session comments <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

이어서 하기는 의도적으로 엄격합니다. 이어받은 실행이 원래 실행과 똑같은 대상을 리뷰할
때만 체크포인트를 재사용합니다.

- 워크스페이스 리뷰는 이어서 할 수 없습니다.
- 리뷰 모드가 같아야 합니다. range 세션을 commit 리뷰로 이어받을 수 없습니다.
- 해석된 입력이 같아야 합니다. ref를 *어떻게 적었는지*는 따지지 않습니다. `abc1234`와
  `abc1234def`는 같은 커밋을 가리킵니다. 다만 같은 ref가 지금은 다른 diff로
  해석되거나, 규칙이나 필터가 선택 파일 집합을 바꿨다면 일부만 재사용하지 않고 이어서
  하기 자체를 거부합니다.
- 프로바이더나 모델을 바꾸려면 `--provider` / `--model`로 명시해야 합니다. 설정이나
  환경 변수를 통해 들어온 변경은 거부합니다.
- 원래 실행에 run manifest가 있어야 하며, 입력은 이 manifest와 대조해 검증합니다. 파일
  전달이 시작된 뒤에는 Ctrl-C를 눌러도 리뷰가 정상적으로 종료되면서 manifest를
  남기므로, 완료된 체크포인트는 계속 이어받을 수 있습니다. 정상 종료 전에 강제로 죽은
  프로세스와 run manifest보다 오래된 세션에는 manifest가 없습니다.
- 원래 실행의 manifest가 처리를 마친 파일만 재사용합니다. manifest에 없는
  체크포인트나 읽을 수 없는 체크포인트는 그 파일의 체크포인트만 잃을 뿐이며, 해당
  파일을 다시 리뷰하는 것으로 끝납니다.
- `--preview`와 `--resume`은 함께 쓸 수 없습니다.

이어서 하기가 거부되면 아무것도 남지 않습니다. 세션도, manifest도, LLM 호출도
없습니다.

### 출력 {#output}

#### 텍스트(기본값, `--audience human`) {#text-default-audience-human}

리뷰가 진행되는 동안 진행 상황이 흘러나오고, 이어서 코멘트마다 블록이 하나씩
출력됩니다(흐린 유니코드 선으로 그린 `path:start-end` 머리글, 100칸으로 줄바꿈한
코멘트 본문, 그리고 제안이 있으면 색을 입힌 인라인 diff). 마지막에 실행 요약이
stdout에 남습니다:

```
[ocr] 17 file(s) changed, reviewing 9 in /path/to/repo
[ocr] Skipping image.png — filtered by path/extension rules
[ocr]   ▶ file_read "src/foo.go"
[ocr]   ✔ file_read (12ms)
[ocr] Plan completed for src/foo.go
…

─── src/foo.go:42-47 ───
Concurrent map access without a lock — wrap with sync.RWMutex.

- m[k] = v
+ mu.Lock(); defer mu.Unlock(); m[k] = v

…
[ocr] Summary: 9 file(s) reviewed, 14 comment(s), ~21344 token(s) used (input: ~18012, output: ~3332), 1m12s elapsed
```

#### 텍스트(agent, `--audience agent`) {#text-agent-audience-agent}

코멘트 출력은 같지만, 내부의 끌 수 있는 stdout writer로 진행 상황만 억제합니다
([`internal/stdout`](https://github.com/alibaba/open-code-review/blob/main/internal/stdout/stdout.go)).
CI에서 쓰거나 다른 Agent로 파이프할 때 이 모드를 쓰세요.

#### JSON {#json}

```bash
ocr review --format json --audience agent
```

문서는 언제나 stdout에 단독으로 나갑니다. 기본값인 `--audience human`에서는 리뷰가
도는 동안 `[ocr]` 진행 상황이 **stderr**로 흘러가므로, 오래 걸리는 실행을 지켜보면서도
stdout을 그대로 파서에 넘길 수 있습니다:

```bash
ocr review --format json > result.json   # 진행 상황은 터미널에 계속 보입니다
ocr review --format json | jq .summary   # stdout은 JSON 문서 하나입니다
```

진행 상황을 아예 없애려면 `--audience agent`를, 셸에서 버리려면 `2>/dev/null`을
쓰세요.

```json
{
  "status": "success",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "summary": {
    "files_reviewed": 9,
    "comments": 1,
    "total_tokens": 21344,
    "input_tokens": 18012,
    "output_tokens": 3332,
    "elapsed": "1m12s"
  },
  "comments": [
    {
      "path": "src/foo.go",
      "content": "Concurrent map access without a lock — wrap with sync.RWMutex.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "m[k] = v",
      "suggestion_code": "mu.Lock(); defer mu.Unlock(); m[k] = v",
      "thinking": "Looking at line 42, the map …"
    }
  ]
}
```

최상위 필드:

| 필드 | 설명 |
|---|---|
| `status` | `success`, `completed_with_warnings`, `completed_with_errors`, `skipped` 중 하나입니다. |
| `llm` | 해석된 LLM 정보입니다. 정규화한 `model`은 항상 있고, `provider`는 이름이 있는 설정된 프로바이더일 때만 나옵니다. |
| `message` | 선택. 사람이 읽는 요약입니다(예: `"No comments generated. Looks good to me."`). |
| `summary` | 선택. 실행 집계입니다: `files_reviewed`, `comments`, `total_tokens`, `input_tokens`, `output_tokens`, `cache_read_tokens`(omitempty), `cache_write_tokens`(omitempty), `elapsed`. `skipped` 실행에서는 나오지 않습니다. |
| `comments` | 항상 있으며 비어 있을 수 있습니다. 코멘트별 필드는 위 예시와 같습니다. |
| `warnings` | 선택. 서브 Agent가 하나 이상 실패했을 때 나오며, 각 항목이 해당 파일과 오류를 설명합니다. |
| `session_id` | 선택. 세션을 남긴 실행에 나옵니다. 호환되는 range나 commit 리뷰를 다시 시도할 때 `ocr review --resume <session-id>`에 넘기세요. |
| `resume` | 선택. 이어서 한 실행에 나오며 `resumed_from`, `reused_files`, `rerun_files`, `previous_model`, `current_model`을 담습니다. |

리뷰 대상 파일이 하나도 없으면 JSON 모드는 대신 `skipped` 응답을 내보냅니다. 호출한
쪽에서 "변경 없음"과 "지적 없음"을 구분할 수 있습니다:

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "comments": []
}
```

### 종료 코드 {#exit-codes}

| 코드 | 뜻 |
|---|---|
| `0` | 리뷰가 끝났습니다(코멘트가 0건일 수도, 치명적이지 않은 경고가 있을 수도 있습니다). |
| `1` | 치명적 오류입니다. 잘못된 플래그, LLM 엔드포인트 해석 실패, 그룹별 서브 Agent 전멸 등이며 오류 내용은 stderr에 출력됩니다. |

치명적이지 않은 경고(서브 Agent 하나 실패, 파일이 토큰 한계 초과 등)는 실행 중간에
출력되고, JSON 모드에서는 `warnings` 배열에 담깁니다.

## `ocr scan` {#ocr-scan}

Git diff 없이 파일 전체를 리뷰합니다. 각 파일의 현재 내용을 작업 트리에서 읽어 LLM에
보냅니다. 익숙하지 않은 코드베이스를 감사하거나 의미 있는 diff가 없는 디렉터리를 볼 때
유용합니다.

```text
ocr scan [flags]
ocr s      [flags]   (alias)
```

`--path`를 주지 않으면 저장소 전체를 스캔합니다.

### 플래그 {#flags}

| 플래그 | 단축 | 기본값 | 설명 |
|---|---|---|---|
| `--path <list>` | - | 저장소 전체 | 스캔할 저장소 기준 상대 디렉터리나 파일(쉼표 구분). 예: `internal/agent`, `internal/llm/client.go`. |
| `--exclude <patterns>` | - | - | 건너뛸 gitignore 형식 패턴(쉼표 구분). 예: `**/generated/*,*.pb.go`. `rule.json`의 excludes와 합쳐집니다. |
| `--output <path>` | `-o` | stdout | 스캔 결과를 UTF-8 파일로 씁니다(`-`는 stdout). 첫 쓰기 시점에 파일을 만들므로 실패한 실행은 기존 파일을 건드리지 않습니다. text 형식에서는 ANSI 색 코드를 자동으로 제거합니다. |
| `--preview` | `-p` | `false` | LLM을 호출하지 않고 파일을 나열하고 필터링합니다. 파일 목록, 리뷰 대상과 제외 대상 개수, 전체 라인 수, 파일별 제외 사유를 출력합니다. `--format json`은 지원하지만 `--format sarif`는 지원하지 않습니다. |

```bash
ocr scan --preview                              # 무엇을 스캔할지 확인
ocr scan --path internal/agent                  # 디렉터리 하나만 스캔
ocr scan --path internal/agent,internal/llm/client.go
ocr scan --exclude '**/generated/*,*.pb.go'
```

전체 플래그 목록은 `ocr scan -h`로 확인하세요.

## `ocr session` {#ocr-session}

`~/.opencodereview/sessions/` 아래에 저장된 로컬 리뷰 세션 로그를 나열하고 살펴봅니다.
세션 ID를 찾거나, 파일별 체크포인트 상태를 확인하거나, 중단된 range·commit 리뷰를
이어서 할 때 씁니다.

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls        List recent review sessions for the current repo
  show <id>       Show one session's metadata and per-file items
  comments <id>   Show the review comments recorded in one session
```

### `ocr session list` {#ocr-session-list}

```bash
ocr session list
ocr session list --limit 50
ocr session list --json
```

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `--repo <path>` | 현재 디렉터리 | 세션을 나열할 저장소. |
| `--json` | `false` | 세션 요약을 JSON으로 출력합니다. |
| `--limit <n>` | `20` | 나열할 세션 수를 제한합니다. `0`이면 제한이 없습니다. |

### `ocr session show` {#ocr-session-show}

이어서 한 실행이라면 어떤 실행을 이어받았는지, 그 과정에서 프로바이더나 모델이 바뀌었다면
무엇에서 무엇으로 바뀌었는지도 함께 출력합니다.

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `--repo <path>` | 현재 디렉터리 | 살펴볼 세션이 속한 저장소. |
| `--json` | `false` | 세션 메타데이터와 파일별 항목을 JSON으로 출력합니다. |

### `ocr session comments` {#ocr-session-comments}

세션에 저장된 리뷰 코멘트를 모두 출력하며, `ocr review`의 터미널 출력과 같은
형식으로 보여줍니다(경로, 라인 범위, 심각도 배지, 제안 diff).

```bash
ocr session comments <session-id>
ocr session comments --json <session-id>
ocr session comments --severity high <session-id>
ocr session comments --severity critical,high --category bug,security <session-id>
```

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `--repo <path>` | 현재 디렉터리 | 살펴볼 세션이 속한 저장소. |
| `--json` | `false` | 코멘트를 JSON 배열로 출력합니다. |
| `--severity <list>` | 전체 | 포함할 심각도(쉼표 구분): `critical`, `high`, `medium`, `low`. |
| `--category <list>` | 전체 | 포함할 분류(쉼표 구분). 예: `bug`, `security`. |

### `ocr session compare` {#ocr-session-compare}

두 세션의 지적을 네 갈래로 묶습니다. **새로 생긴 것**(뒤 세션에만 있음), **남아 있는
것**(양쪽에 있음), **해결된 것**(앞 세션에만 있음), **리뷰하지 않은 것**(앞 세션에는
있지만 뒤 세션이 아예 보지 않은 파일이라 해결된 것으로 세지 않음)입니다.

지적은 라인 번호가 아니라 경로와 분류, 문제가 된 코드 조각으로 대조합니다. 그래서
파일 안에서 위치만 밀린 지적은 여전히 남아 있는 것으로 잡힙니다.

```bash
ocr session compare <before-session-id> <after-session-id>
ocr session diff <before-session-id> <after-session-id>
ocr session compare --json <before-session-id> <after-session-id>
```

두 세션은 같은 저장소에 속해야 하며, 그렇지 않으면 명령이 실패합니다. 리뷰 모드가
다를 때는 stderr에 경고만 출력하므로 `--json` 출력은 그대로 파이프할 수 있습니다.

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `--repo <path>` | 현재 디렉터리 | 비교할 세션이 속한 저장소. |
| `--json` | `false` | 비교 결과를 JSON으로 출력합니다(`new`, `persisting`, `resolved`, `not_reviewed`). |

## `ocr rules` {#ocr-rules}

규칙을 들여다보는 명령입니다. 하위 명령은 하나뿐입니다:

```text
ocr rules check [flags] <file-path>

Flags:
  --repo <path>    Git repository root (default: current dir)
  --rule <path>    Path to a custom rule JSON file
```

주어진 파일 경로를 받으면 OCR은 다음을 수행합니다.

1. 네 겹의 규칙 사슬(커스텀 → 프로젝트 → 전역 → 시스템)을 따라갑니다.
2. 처음 일치하는 규칙을 고릅니다.
3. **출처 계층**, 일치한 **glob 패턴**, 해석된 **규칙 내용**을 출력합니다.

```bash
$ ocr rules check src/main/java/com/example/Foo.java
File: src/main/java/com/example/Foo.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
<contents of internal/config/rules/rule_docs/java.md>
────────────────────────────────────────
```

"내 커스텀 규칙이 왜 안 걸리지?"를 파헤칠 때 유용합니다. 우선순위 전체는
[리뷰 규칙](../review-rules/)을 참고하세요.

## `ocr config` {#ocr-config}

키를 `~/.opencodereview/config.json`에 저장하고 대화형 설정 TUI를 제공합니다. 하위
명령은 네 가지입니다:

```text
ocr config set <key> <value>
ocr config unset <key>                     Clear a saved key
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`** — 설정값 하나를 대화 없이 기록합니다. `effort`는 `low` / `medium` /
  `high`를 받아 모든 실행의 기본 리뷰 강도를 정하며, `--effort`가 실행 단위로 이를
  덮어씁니다.
- **`unset`** — 저장된 키를 지웁니다. `provider`, `max_tokens`, `effort`,
  `custom_providers.<name>`, `mcp_servers.<name>`을 지원합니다. `effort`를 지우면
  기본값인 `medium` 프리셋으로 돌아갑니다. 지운 프로바이더가 활성 상태였다면
  `provider`와 `model`도 함께 지워지므로, `ocr config provider`로 새로 고르세요.
- **`provider`** — 대화형 프로바이더 설정 TUI를 띄웁니다(추가 인자 없음. 대화 없이
  설정하려면 `ocr config set provider <name>`을 쓰세요).
- **`model`** — 대화형 모델 선택 TUI를 띄웁니다(추가 인자 없음. 대화 없이 설정하려면
  `ocr config set model <name>`을 쓰세요).

키 전체 목록과 스키마, 예시는 [설정](../configuration/)을 참고하세요.

## `ocr llm` {#ocr-llm}

LLM 유틸리티 명령입니다. 하위 명령은 두 가지입니다:

```text
ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers
```

### `ocr llm test` {#ocr-llm-test}

```text
ocr llm test
```

`ocr review`와 똑같은 방식으로 LLM 엔드포인트를 해석한 뒤,
[`internal/config/testconnection/task.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/testconnection/task.json)에
담긴 정해진 채팅 요청을 한 번 보내고 다음을 출력합니다:

```
Source: <which strategy was used>
URL:    <endpoint URL>
Model:  <effective model>
<the model's reply>
✓ Connection test successful
```

0이 아닌 종료 코드는 엔드포인트 설정이 덜 됐거나 요청이 실패했다는(네트워크·인증·모델
오류) 뜻입니다. 둘 중 무엇인지는 오류 메시지가 알려 줍니다.

### `ocr llm providers` {#ocr-llm-providers}

```text
ocr llm providers
```

내장 LLM 프로바이더를 세 열짜리 표로 나열합니다:

```
Built-in providers:
  NAME        PROTOCOL    BASE URL
  ----        --------    --------
  anthropic   anthropic   https://api.anthropic.com
  …
```

이어서 `ocr config provider`로 대화형 설정을 하거나 `ocr config set provider <name>`으로
대화 없이 설정하라는 안내가 따라붙습니다.

## `ocr viewer` {#ocr-viewer}

```text
ocr viewer [flags]

Flags:
  --addr <address>   listen address (default: localhost:5483)

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
```

`~/.opencodereview/sessions/...`를 읽어 지난 리뷰 세션을 브라우저에서 보기 좋게
렌더링하는 내장 HTTP 서버를 띄웁니다. [세션 뷰어](../viewer/)를 참고하세요.

## `ocr version` {#ocr-version}

```text
ocr version
ocr --version
ocr -V
```

빌드 시점에 새겨진 버전, 짧은 Git 커밋(있을 때), 플랫폼(`<GOOS>/<GOARCH>`), 빌드
날짜(있을 때), GitHub URL(`https://github.com/alibaba/open-code-review`)을 출력합니다.


## ocr completion {#ocr-completion}

`ocr`의 셸 자동 완성 스크립트를 만들어, 셸에서 명령 이름과 플래그, 인자를 탭으로
완성할 수 있게 합니다.

### Bash {#bash}

현재 세션에만 적용:

```bash
source <(ocr completion bash)
```

계속 적용(Linux):

```bash
ocr completion bash > /etc/bash_completion.d/ocr
```

계속 적용(macOS):

```bash
ocr completion bash > $(brew --prefix)/etc/bash_completion.d/ocr
```

### Zsh {#zsh}

셸 자동 완성이 아직 켜져 있지 않다면 한 번만 켜 둡니다:

```bash
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

그런 다음 자동 완성을 계속 적용되도록 넣습니다:

```bash
ocr completion zsh > "${fpath[1]}/_ocr"
```

새 셸을 열어야 적용됩니다.

### Fish {#fish}

현재 세션에만 적용:

```bash
ocr completion fish | source
```

계속 적용:

```bash
ocr completion fish > ~/.config/fish/completions/ocr.fish
```

### PowerShell {#powershell}

현재 세션에만 적용:

```powershell
ocr completion powershell | Out-String | Invoke-Expression
```

계속 적용 — 스크립트를 한 번 만든 뒤 프로필에서 불러옵니다:

```powershell
ocr completion powershell > ocr.ps1
```

PowerShell 프로필에 `ocr.ps1`을 점으로 불러오는 줄을 추가하세요.


## 팁과 주의점 {#tips-gotchas}

- `--audience agent`는 `--format json`을 **뜻하지 않고**, `--format json`도 조용한
  터미널을 뜻하지 않습니다. 둘은 서로 다른 것을 다룹니다. 하나는 조용한 UI, 다른
  하나는 구조화된 페이로드입니다. `--format json`만 주면 진행 상황은 stderr에 계속
  보입니다. 이를 없애려면 `--audience agent`를 더하세요.
- `--background`는 리뷰 품질에 가장 크게 작용하는 플래그입니다. 다른 Agent에서
  호출할 때는 요구사항이나 PR 설명을 반드시 함께 넘기세요.
- diff만으로 `MAX_TOKENS`(기본값 `200000`)의 80%를 넘는 파일은 LLM을 호출하기 전에
  버립니다. 로그에는 남지만 실행이 실패하지는 않습니다.
- `MAX_TOKENS`는 **프롬프트**만 제한합니다. 모델의 출력은
  `MAX_COMPLETION_TOKENS`(`16384`)가 따로 제한하므로, 컨텍스트가 큰 모델을 쓰려고
  `--max-tokens`를 올려도 출력 비용은 늘지 않습니다.
- 변경된 라인 수가 `PLAN_MODE_LINE_THRESHOLD`(`50`, 그룹에서 가장 큰 파일 하나에
  적용)와 `PLAN_MODE_GROUP_LINE_THRESHOLD`(`100`, 파일이 2개 이상인 그룹의 변경량
  합계에 적용)를 모두 밑돌면 plan 단계를 **자동으로 건너뜁니다**.
- 리뷰 전에 메타데이터만 보는 저렴한 LLM 호출 한 번으로 파일을 의미 단위 그룹으로
  묶으므로, 서로 관련된 변경(핸들러 + 서비스 + 테스트)을 한 대화에서 함께 리뷰합니다.
  묶기에 실패하면 조용히 파일당 그룹 하나로 되돌아갑니다.
- 리뷰가 얕게 느껴질 때 가장 저렴한 품질 조절 수단은 `--effort high`입니다. 빠르게
  훑기만 하면 될 때는 `--effort low`가 기본값 대비 비용을 대략 절반으로 줄여 줍니다.

## 관련 문서 {#see-also}

- [빠른 시작](../quickstart/) — 설치하고 첫 리뷰를 실행합니다.
- [설정](../configuration/) — 플래그 뒤에 있는 환경 변수와 설정 키입니다.
- [리뷰 규칙](../review-rules/) — `--rule` 플래그와 규칙 해석 방식입니다.
- [연동](../integrations/agent-skill/) — Agent와 CI에서 `ocr review`를 호출하는
  방법입니다.
