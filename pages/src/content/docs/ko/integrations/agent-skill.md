---
title: Agent Skill
sidebar:
  order: 1
---

OCR을 호출 가능한 스킬로 등록하면, Agent 프레임워크가 알맞은 플래그와 사전 점검,
분류 기준을 갖춘 채로 OCR을 실행합니다. 호출하는 쪽에서 그 내용을 다시 만들 필요가
없습니다.

## 저장소에 포함된 것 {#what-ships-in-the-repo}

저장소에는 SKILL 매니페스트가
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)에
들어 있습니다. OCR을 호출 가능한 스킬로 선언하며, 사전 점검과 호출 워크플로,
코멘트 분류 기준(High/Medium/Low)을 함께 담고 있습니다.

## 설치 {#install}

### 방법 1: `npx skills add` (권장) {#option-1-npx-skills-add-recommended}

스킬을 사용할 프로젝트 안에서 실행하세요:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

이 명령은
[스킬 레지스트리](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)에서
매니페스트를 가져와 프로젝트에 넣습니다. 스킬 규약을 따르는 코딩 Agent라면 다음
호출부터 이를 인식합니다. 같은 명령을 다시 실행하면 스킬이 최신 버전으로 갱신됩니다.

> **사전 요구 사항:** `ocr` 바이너리가 `PATH`에 없으면 스킬이 처음 실행될 때 CLI를
> 직접 설치합니다(`npm install -g @alibaba-group/open-code-review`). 아래
> [스킬이 하는 일](#what-the-skill-does)을 참고하세요. 다만 LLM은 **미리 설정해
> 두어야 합니다**. 스킬이 대신 설정할 수 없으므로, 설정이 없으면 실행을 멈추고
> 사용자에게 묻습니다. [설정](../../configuration/)을 참고하세요.

### 방법 2: 수동 복사 (시스템 전역) {#option-2-manual-copy-system-wide}

프로젝트마다 설치하는 대신 전역에 두고 싶다면 스킬 폴더를 스킬 디렉터리로
복사하세요:

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

이렇게 하면 그 머신의 모든 프로젝트에서 스킬을 쓸 수 있습니다.

## 스킬이 하는 일 {#what-the-skill-does}

SKILL.md는 프롬프트입니다. 호출한 Agent가 이 파일을 읽고 각 단계를 직접 실행합니다.
`/open-code-review`(또는 이에 준하는) 요청 하나가 처음부터 끝까지 다음 순서로
진행됩니다.

1. **사전 점검.** `which ocr`로 CLI가 `PATH`에 있는지 확인한 뒤, `ocr llm test`로
   LLM에 연결되는지 확인합니다.
2. **CLI가 없으면 자동 설치.** `which ocr`이 "NOT INSTALLED"를 반환하면 Agent가
   `npm install -g @alibaba-group/open-code-review`를 실행하고 작업을 이어 갑니다.
   일상적인 설치 단계로 여겨 사용자에게 묻지 않습니다.
3. **LLM 설정이 없으면 멈추고 질문.** `ocr llm test`가 실패하면 Agent는 자격 증명을
   임의로 지어내지 *않습니다*. 지원하는 두 가지 방법(환경 변수 또는
   `ocr config set …`)을 안내하고 사용자가 API 키를 줄 때까지 기다립니다.
4. **비즈니스 맥락 추출.** 리뷰 대상(커밋, 브랜치, 작업 복사본)을 살펴 짧은
   `--background` 문자열을 만듭니다.
5. **리뷰 실행.**
   `ocr review --audience agent --background "…" [--commit | --from/--to]`를
   호출합니다. 사용자가 작업 복사본, 특정 커밋, 브랜치 범위 중 무엇을 리뷰해
   달라고 했는지에 따라 플래그를 고릅니다.
6. **분류와 보고.** SKILL.md의 기준에 따라 JSON 코멘트를 **High** / **Medium** /
   **Low**로 묶습니다(버그와 보안 이슈는 High, 사소한 지적과 오탐으로 보이는 항목은
   조용히 버립니다). 그런 다음 Markdown 요약을 렌더링합니다.
7. **요청이 있으면 수정.** 사용자가 "리뷰하고 **고쳐 줘**"처럼 요청했다면
   High/Medium 항목을 안전한 범위에서 바로 수정합니다. 그렇지 않으면 코드를 건드리기
   전에 먼저 묻습니다.

정확한 분류 기준과 출력 템플릿, 주의할 점을 포함한 전체 프롬프트는
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)에
있습니다. 위 동작을 더 엄격하게 바꾸고 싶다면(예: 수정 전에 항상 묻도록 기본값을
바꾸는 등) 로컬 사본을 편집하세요.

## Anthropic Agent SDK {#anthropic-agent-sdk}

SDK를 초기화할 때 설치된 스킬 경로를 지정하세요:

```python
from anthropic_agent_sdk import Agent

agent = Agent(
    skill_paths=["/path/to/open-code-review/skills/open-code-review"],
)

agent.run("Review my staged changes — focus on race conditions.")
```

SDK가 SKILL.md 프롬프트를 읽으면 Agent가
[스킬이 하는 일](#what-the-skill-does)에 적힌 워크플로를 실행합니다.
`npm install` 대체 설치도, LLM이 설정되어 있지 않을 때 자격 증명을 묻는 단계도
여기에 포함됩니다.

## 다른 Agent 프레임워크 {#other-agent-frameworks}

"외부 스킬 등록" 기능이 있는 프레임워크라면 SKILL.md를 그대로 읽어 들일 수 있습니다.
frontmatter가 붙은 마크다운 파일일 뿐이기 때문입니다. 프레임워크가 다른 스키마를
요구하더라도 마크다운 본문은 프롬프트 템플릿으로 쓸 수 있습니다.

## 관련 문서 {#see-also}

- [Command (Claude Code 플러그인)](../claude-code/) — 같은 스킬을 슬래시 명령
  형태로 제공합니다.
