---
title: Agent Skill
sidebar:
  order: 1
---

OCR을 호출 가능한 스킬로 등록하면, Agent 프레임워크가 알맞은 플래그와 발견 사항 검증
절차에 따라 OCR을 실행합니다. 호출하는 쪽에서 그 내용을 다시 만들 필요가
없습니다.

## 저장소에 포함된 것 {#what-ships-in-the-repo}

저장소에는 SKILL 매니페스트가
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)에
들어 있습니다. OCR 호출, 발견 사항 검증, 불확실성 보고, 요청에 따른 수정 절차를
정의합니다. GitHub PR 리뷰 도우미도 포함되므로 SKILL.md만이 아닌 스킬 디렉터리 전체를 설치하세요.

## 설치 {#install}

### 방법 1: `npx skills add` (권장) {#option-1-npx-skills-add-recommended}

스킬을 사용할 프로젝트 안에서 실행하세요:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

이 명령은
[스킬 레지스트리](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)에서
스킬 디렉터리를 가져와 프로젝트에 넣습니다. 스킬 규약을 따르는 코딩 Agent라면 다음
호출부터 이를 인식합니다. 같은 명령을 다시 실행하면 스킬이 최신 버전으로 갱신됩니다.

> **사전 요구 사항:** 네이티브 모드에는 로컬에 설정된 OCR 모델 공급자가 필요합니다.
> 자격 증명을 대화에 붙여 넣지 마세요. `ocr`이 없을 때의 문제 해결 절차에는
> `npm install -g @alibaba-group/open-code-review` 설치가 포함됩니다.
> GitHub PR 모드에만 Python 3와 인증된 `gh`도 필요합니다. [설정](../../configuration/)을 참고하세요.

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

1. **비즈니스 맥락 추출.** 리뷰 대상(커밋, 브랜치, 작업 복사본)을 살펴 짧은
   `--background` 문자열을 만듭니다.
2. **요청한 리뷰 실행.** 로컬 리뷰는 `ocr review --audience agent --background "…"`를
   바로 호출하고, 커밋된 변경에는 `--commit` 또는 `--from`/`--to`를 추가합니다.
   정상 경로에서는 설치나 연결을 먼저 점검하지 않습니다. 새 커밋 및 범위 리뷰는 기본적으로
   `--analysis`를 추가하고, 다른 형식을 요청하지 않으면 JSON을 사용합니다.
   작업 복사본, 미리보기, 재개, SARIF 또는 분석 생략을 명시한 요청에는 분석을 추가하지 않습니다.
   실패하면 원인을 확인한 뒤 재시도합니다.
   GitHub PR 번호나 URL에는 설치된 `python3 "$SKILL_DIR/scripts/review_pr.py" <id-or-url>`를
   사용합니다. 번호는 체크아웃의 `gh` 기본 저장소를 선택하며, 포크에서는 상위 저장소일 수 있습니다.
   PR URL, 리뷰한 리비전, 스냅샷의 최신 여부와 결과물 경로를 보고합니다.
3. **검증과 보고.** 낮은 심각도를 포함한 모든 발견 사항을 리뷰 대상 코드와 대조합니다.
   **Reproduced(재현됨)**와 **Uncertain(미확정)**으로 나누고, OCR의 심각도와 각 그룹 안의 순서를
   유지합니다. 구체적인 반증이 있을 때만 항목을 생략하며, 불확실하다는 이유만으로 버리지 않습니다.
   원본 결과를 보존하고 Review의 검토 범위와 누락도 보고합니다. 함께 실행한 Analysis의 Markdown은
   따로 표시하고, 실행 상태와 완전성에 대한 모델의 판단을 구별합니다.
4. **요청이 있으면 수정.** 편집을 요청한 경우에만 검증된 안전한 수정을 적용하며 critical, high,
   medium 항목을 우선합니다. 리뷰만 요청한 경우 코드 변경 권한은 없습니다. PR 모드에서는 사용자가
   정리에 명시적으로 동의할 때까지 스냅샷을 보관하며, 정리 후에도 보고서는 남습니다.

검증 요구 사항, 보고 방법, PR 모드 제한과 주의할 점을 포함한 전체 프롬프트는
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
호출 실패를 조사하고, 자격 증명이 필요하면 사용자에게 로컬에서 설정하도록 안내합니다.

## 다른 Agent 프레임워크 {#other-agent-frameworks}

"외부 스킬 등록" 기능이 있는 프레임워크라면 SKILL.md를 그대로 읽어 들일 수 있습니다.
frontmatter가 붙은 마크다운 파일일 뿐이기 때문입니다. 프레임워크가 다른 스키마를
요구하더라도 마크다운 본문은 프롬프트 템플릿으로 쓸 수 있습니다.

## 관련 문서 {#see-also}

- [Command (Claude Code 플러그인)](../claude-code/) — 같은 스킬을 슬래시 명령
  형태로 제공합니다.
