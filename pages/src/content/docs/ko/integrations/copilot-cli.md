---
title: GitHub Copilot CLI
sidebar:
  order: 3
---

다른 코딩 Agent가 쓰는 것과 같은 이식 가능한 [Agent Skill](../agent-skill/)을 통해
[GitHub Copilot CLI](https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli)
안에서 OCR을 처음부터 끝까지 실행합니다. diff를 리뷰하고, 발견 사항을 분류하고,
반영할 만한 항목은 수정합니다. 이 연동은 복사해서 바로 쓰는 스킬입니다. 훅을
설치하지 않고, 필수 지침을 추가하지 않으며, 저장소 동작도 바꾸지 않습니다.

## 저장소에 포함된 것 {#what-ships-in-the-repo}

워크플로는 Agent에 종속되지 않는 스킬인
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)에
들어 있습니다. Copilot CLI 플러그인 매니페스트
[`plugins/open-code-review/plugin.json`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/plugin.json)
(Agent Plugins 1.0)가 번들 스킬을 `copilot plugin install`용으로 묶습니다.
Copilot CLI는 표준 위치에서도 스킬을 읽으므로 스킬 디렉터리를 복사하는 방법도
계속 사용할 수 있습니다.

## 설치 {#install}

플러그인을 설치하세요(권장).

```bash
copilot plugin install alibaba/open-code-review:plugins/open-code-review
```

`open-code-review`와 `open-code-review-delegate` 스킬이 설치됩니다. 제거하려면
`copilot plugin uninstall open-code-review`를 실행하세요.

Copilot CLI의 설치 프로그램은 현재 직접 플러그인 설치(repo/path/URL)가 더 이상
권장되지 않고 마켓플레이스 설치가 권장된다고 경고합니다. 나중에 마켓플레이스 항목이
추가되면 `plugin@open-code-review`가 권장 형식이 될 수 있습니다.

### 수동 대안: 스킬 복사 {#manual-alternative-copy-the-skill}

Copilot CLI가 인식하는 디렉터리에 스킬을 복사하세요. **프로젝트 수준**(저장소에 함께
커밋해 팀이 공유):

```bash
mkdir -p .github/skills/open-code-review
curl -o .github/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

**사용자 수준**(머신의 모든 프로젝트에서 사용):

```bash
mkdir -p ~/.copilot/skills/open-code-review
curl -o ~/.copilot/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

Copilot CLI는 프로젝트의 `.agents/skills/`와 `.claude/skills/`도 인식합니다. 이름이
같으면 프로젝트 스킬이 개인 스킬보다 우선합니다. 로드된 스킬은 세션 안에서
`/skills list`로, 세션 밖에서 `copilot skill list`로 확인하세요. 직접 복사한 스킬은
`/skills reload`를 실행하거나 새 세션을 시작한 뒤에 적용됩니다. 설치한 플러그인은
다시 설치하거나(`copilot plugin install` 재실행) `copilot plugin update`로
업데이트합니다.

## 사용 {#use}

`/open-code-review`로 스킬을 직접 언급하거나, 작업을 설명하기만 하세요. Copilot이
설명을 보고 스킬을 고릅니다:

```
/open-code-review
/open-code-review review this PR against main
/open-code-review focus on race conditions in commit abc123
```

| 이렇게 말하면 | 스킬이 실행하는 명령 |
| --- | --- |
| "내 변경 사항을 리뷰해 줘" | `ocr review --audience agent` (스테이징된 변경, 스테이징되지 않은 변경, 추적되지 않은 파일) |
| "이 PR을 리뷰해 줘" | `ocr review --from main --to <branch>` |
| "커밋 abc123을 리뷰해 줘" | `ocr review --commit abc123` |

스킬은 자체 설정된 모델로 `ocr`을 구동하고, 발견 사항을 심각도별로 묶어 보고합니다.
`ocr`용 LLM을 따로 설정하고 싶지 않다면 [위임 모드](../delegate/)를 사용해 보세요.
호스트 Agent(Copilot)가 모델을 제공하고 `ocr`은 파일 선택과 규칙 해석만 담당합니다.

### 선택 사항: 리뷰를 습관으로 만들기 {#optional-make-review-a-habit}

리뷰 단계를 그때그때 실행하는 대신 자동으로 만들려면
`.github/copilot-instructions.md` 또는 `AGENTS.md`에 규칙을 하나 추가하세요. Copilot은
지침 파일을 우선순위 없이 합치므로 둘 중 하나만 고르세요:

```markdown
Before reporting a coding task complete, run `ocr review --audience agent`.
Fix all `critical` and `high` findings first.
```

이런 규칙을 둘지는 각 프로젝트가 정할 일입니다. 이 저장소는 그런 규칙을 강제하지
않습니다.

## 헤드리스 실행 {#headless-runs}

```bash
copilot -p "Review the current branch with /open-code-review and report the findings." \
  --allow-tool='shell(ocr:*)'
```

- `--allow-tool='shell(ocr:*)'`는 `ocr` 셸 명령만 미리 승인하며 파일 편집을
  허용하지는 않습니다. 비대화형(`-p`) 세션에서는 승인이 필요한 작업이 일시 중지되지
  않고 거부됩니다. 대화형 세션에서는 대신 승인을 기다리며 일시 중지합니다. Copilot이
  수정 사항을 적용하게 하려면 대화형으로 실행하세요(또는 허용 목록을 의도적으로 편집
  도구까지 확장하세요).
- 신뢰할 수 없는 폴더에서는 비대화형 세션이 프로젝트 수준 스킬(`.github/skills/`)을
  건너뛸 수 있습니다. Copilot CLI v1.0.107 이상은 단일 세션 동안 해당 폴더를
  신뢰하는 `--trust-folder`를 제공합니다.
- Copilot 샌드박스가 켜져 있으면 `ocr`에는 LLM 엔드포인트에 접근할 네트워크 권한과
  `~/.opencodereview/`에 대한 읽기 및 쓰기 권한이 필요합니다(세션 상태를 저장합니다).
- `--output-format json`은 Copilot의 JSONL 세션 이벤트를 출력합니다. 리뷰 결과 자체는
  `ocr`의 JSON이며, `ocr review --format json --audience agent`를 직접 실행해
  얻습니다.

결정적 CI 게이트가 필요하면 Agent를 건너뛰고 `ocr`을 직접 호출하세요.
[CI/CD](../cicd/)를 참고하세요.

## 참고 사항 {#notes}

- **훅은 의도적으로 제공하지 않습니다.** Copilot CLI는 `preToolUse` 훅으로
  `git commit`이나 `git push`를 게이트로 제어할 수 있지만, OCR은 그런 훅을 제공하지
  않습니다. 리뷰는 LLM을 실행하므로 느리고, 기여자가 갖지 못할 수도 있는 자격 증명이
  필요하며, 리뷰 대상 워크스페이스는 실제로 만들어지는 커밋과 다릅니다. 또한 실패해도
  통과시키는(fail-open) 훅은 보장할 수 없는 강제력을 내세우는 셈입니다. 이런
  트레이드오프를 받아들일 때만 자체 훅을 추가하세요.
- **프롬프트 파일은 IDE 전용입니다.** Copilot CLI는
  `.github/prompts/*.prompt.md`를 읽지 않습니다. 지원되는 방식은 스킬입니다.
- **발견 사항은 종료 코드를 바꾸지 않습니다.** `ocr`은 문제를 찾았더라도 리뷰가
  완료되면 0으로 종료합니다. 종료 상태가 아니라 `--format json`의 심각도를 기준으로
  게이트를 만드세요.

## 문제 해결 {#troubleshooting}

- **스킬이 목록에 없음** — 경로(`~/.copilot/skills/` 또는
  `<repo>/.github/skills/`)를 확인한 뒤 `/skills reload`를 실행하거나 새 세션을
  시작하세요.
- **`ocr: command not found`** — `npm i -g @alibaba-group/open-code-review`로
  설치하세요.
- **LLM 오류로 리뷰 실패** — 프로바이더를 설정하고 `ocr llm test`로 확인하세요.
  [설정](../configuration/)을 참고하세요.
- **Copilot이 계속 `ocr` 승인을 요청함** — `--allow-tool`로 `shell(ocr:*)`를
  허용하거나, 프롬프트에서 영구 승인하세요.
