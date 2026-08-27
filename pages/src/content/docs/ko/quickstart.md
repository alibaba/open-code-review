---
title: 빠른 시작
sidebar:
  order: 3
---

몇 분이면 첫 코드 리뷰를 실행할 수 있습니다.

## 사전 요구 사항

- **Git ≥ 2.41**
- **Node.js ≥ 18**
- **LLM API 키** ([Delegation Mode](../integrations/delegate/)를 사용한다면 필요 없음)

## 1단계 — CLI 설치

```bash
npm install -g @alibaba-group/open-code-review
```

```bash
ocr version
```

> 다른 설치 방법은 [설치](../installation/)를 참고하세요.

## 2단계 — LLM 설정

> [Delegation Mode](../integrations/delegate/)를 사용 중이라면(예: Claude Code 안에서 실행) 호스트 Agent가 모델을 제공하므로 4단계로 건너뛰세요.

```bash
ocr config provider
```

내장 프로바이더나 커스텀 프로바이더를 고르고 API 키를 입력한 뒤 모델을 선택하면, 명령이 모든 값을 설정 파일에 저장하고 `ocr llm test`를 한 번 실행해 엔드포인트를 검증합니다. 나중에 모델을 바꾸려면:

```bash
ocr config model
```

### 대안: 비대화형 명령

CI나 TUI가 없는 환경에서는 `ocr config set`으로 같은 설정 파일에 직접 기록합니다:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

## 3단계 — 연결 테스트

```bash
ocr llm test
```

`no valid LLM endpoint configured` 같은 오류가 나오면 2단계 설정을 다시 확인하세요. 401 / 403은 토큰이 잘못됐거나 만료됐다는 뜻입니다.

## 4단계 — 첫 리뷰 실행

아무 Git 저장소로 이동해 실행합니다:

```bash
cd path/to/your-repo

# 워크스페이스 모드 — 스테이징된 + 스테이징되지 않은 + 추적되지 않은 변경을 리뷰(기본값)
ocr review

# 브랜치 범위 — feature-branch가 main에서 분기한 이후의 변경을 리뷰(merge-base 모드)
ocr review --from main --to feature-branch

# 단일 커밋 — 해당 커밋이 도입한 diff를 리뷰
ocr review --commit abc123
```

> `ocr review`의 전체 플래그 목록(동시성 조정, 출력 형식, 출력 대상 모드, 배경 맥락 등)과 다른 모든 하위 명령은 [CLI 레퍼런스](../cli-reference/)를 참고하세요.

### 무엇이 리뷰될지 먼저 보고 싶다면

```bash
ocr review --preview              # 워크스페이스
ocr review -c abc123 --preview    # 커밋
```

### 시스템 연동을 위한 JSON 출력

`--audience agent`는 사람용 진행 UI를 숨겨 stdout에 JSON / 최종 요약만 남깁니다. 상위 Agent나 CI 스크립트가 원하는 형태 그대로입니다.

```bash
ocr review --format json --audience agent > review.json
```

## 관련 문서

- [설치](../installation/) — 모든 설치 방법과 OCR의 상태 디렉터리.
- [설정](../configuration/) — OCR이 인식하는 모든 환경 변수, 설정 키, 내장 프로바이더.
- [CLI 레퍼런스](../cli-reference/) — 모든 하위 명령, 플래그, 출력 모드.
- [리뷰 규칙](../review-rules/) — 리뷰 대상 커스터마이즈.
- [연동](../integrations/agent-skill/) — Claude Code, Agent 스킬, CI에 OCR 넣기.
- [FAQ](../faq/) — 알려진 오류와 해결 방법.
