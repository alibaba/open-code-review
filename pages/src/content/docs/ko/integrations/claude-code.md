---
title: Command (Claude Code 플러그인)
sidebar:
  order: 2
---

함께 제공되는 명령을 설치하면
[Claude Code](https://docs.anthropic.com/en/docs/claude-code) 안에서 OCR이 처음부터
끝까지 실행됩니다. diff를 리뷰하고, 발견한 문제를 분류하고, 반영할 만한 항목은
자동으로 수정까지 합니다.

## 저장소에 포함된 것 {#what-ships-in-the-repo}

저장소에는 Claude Code 플러그인이
[`plugins/open-code-review/claude-code/`](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code)
아래에 들어 있습니다. 명령 프롬프트 자체는
[`plugins/open-code-review/claude-code/commands/review.md`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/claude-code/commands/review.md)에
있으며, 아래에서 설명하는 워크플로의 기준이 되는 파일입니다.

## 설치 {#install}

### 방법 1: 플러그인 마켓플레이스 (권장) {#option-1-plugin-marketplace-recommended}

**Claude Code 안에서** 다음 두 명령을 실행하세요:

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

`/open-code-review:review` 슬래시 명령이 등록되며, 이후 `/plugin`으로 계속 업데이트할
수 있습니다.

### 방법 2: 명령 파일 직접 복사 {#option-2-copy-the-command-file-directly}

플러그인 마켓플레이스를 거치지 않으려면 명령 파일을 `.claude/commands/`에 바로
넣으면 됩니다. 이때는 `:review` 접미사 없이 `/open-code-review`로 등록됩니다.

**프로젝트 수준**(저장소에 함께 커밋해 팀이 공유):

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

**사용자 수준**(머신의 모든 프로젝트에서 사용):

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

### 명령을 지원하는 다른 Agent {#other-agents-with-command-support}

명령 파일은 frontmatter 필드가 하나뿐인 평범한 마크다운이며, Claude Code에만 해당하는
내용은 없습니다. 사용하는 Agent가 비슷한 **명령** 규약(디렉터리에서 마크다운 프롬프트를
읽어 호출 가능한 명령으로 등록하는 방식)을 지원한다면, 위의 파일 복사 방법을 그대로
쓰면 됩니다. `open-code-review.md`를 그 Agent가 명령을 읽는 디렉터리에 넣고, 평소
명령을 부르던 방식으로 호출하세요. 프롬프트 본문은 특정 Agent에 매이지 않습니다.
어떤 `ocr` 플래그를 고르고 결과를 어떻게 분류할지만 모델에 알려 줍니다.

> **팁:** LLM을 직접 설정하고 싶지 않다면 [위임 모드](../delegate/)를 써 보세요.
> 호스트 Agent(Claude Code)가 모델을 제공하므로 별도의 LLM 설정이 필요 없습니다.

## 사용 {#use}

Claude Code에서 명령을 이름으로 호출합니다. 플러그인 마켓플레이스로 설치했다면
`/open-code-review:review`를, 파일을 직접 복사했다면 `/open-code-review`를 쓰세요:

```
/open-code-review:review
/open-code-review:review review this PR against main
/open-code-review:review focus on race conditions in commit abc123
```

프롬프트가 요청을 해석해 알맞은 `ocr review` 플래그를 고릅니다. 인자가 없으면
워크스페이스 모드(스테이징된 변경 + 스테이징되지 않은 변경 + 추적되지 않은 파일),
커밋을 언급하면 `--commit`, 브랜치 범위를 언급하면 `--from` / `--to`를 씁니다. OCR
플래그를 직접 넘겨도 됩니다(예: `/open-code-review:review --commit abc123`,
`--from main --to feature`).

## 명령이 하는 일 {#what-the-command-does}

명령 프롬프트는 짧습니다. 세 단계로 끝납니다.

1. **리뷰 실행.** 요청에서 유추한 플래그로 `ocr review --audience agent`를
   호출합니다(요구사항 맥락을 설명했다면 `--background`도 함께 붙습니다). 출력은
   5분 타임아웃으로 수집합니다.
2. **필터링과 평가.** 각 코멘트를 **High** / **Medium** / **Low**로 분류합니다.
   확신이 낮은 코멘트(오탐으로 보이거나, 사소한 지적이거나, 맥락이 부족한 항목)는
   조용히 버리고 나머지만 보여 줍니다.
3. **수정.** 반영할 만한 High/Medium 항목을 자동으로 고칩니다.
   [Agent Skill](../agent-skill/)과 달리 이 명령은 **기본적으로 자동 수정**합니다.
   "diff만 보여 줘"가 아니라 "리뷰하고 정리까지 해 줘" 흐름에 맞춘 선택입니다.

코드를 건드리기 전에 묻게 하거나 분류 기준을 더 엄격하게 바꾸고 싶다면 프롬프트의
로컬 사본을 편집하세요. Claude Code는 호출할 때마다 명령을 다시 읽으므로 재시작할
필요가 없습니다.

## 관련 문서 {#see-also}

- [Agent Skill](../agent-skill/) — SDK 수준의 대응 기능입니다. 같은 CLI를 쓰지만
  기본값이 다릅니다(수정 전에 먼저 묻습니다).
