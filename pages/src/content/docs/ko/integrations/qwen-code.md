---
title: Qwen Code
sidebar:
  order: 3
---

저장소의 기존 Claude Code Marketplace 플러그인으로 Qwen Code에서 OCR을
실행합니다. 설치 프로그램이 플러그인을 변환하고 리뷰 명령을 불러옵니다.

다음 절차는 Windows의 Qwen Code 0.27.2에서 검증했습니다. 별도로 출시된
[Qwen Code 1.0 Preview](https://github.com/QwenLM/qwen-code/releases/tag/v1.0.0-preview)
호환성은 아직 검증하지 않았습니다.

## 사전 요구 사항

Qwen Code와 Git 2.41 이상을 설치한 다음 OCR CLI를 설치합니다.

```bash
npm install -g @alibaba-group/open-code-review
```

## 설치

터미널에서 다음 명령을 실행합니다.

```bash
qwen extensions install alibaba/open-code-review:open-code-review
qwen extensions list
```

CLI로 설치한 뒤 새 Qwen Code 세션을 시작하세요. 플러그인은
[함께 제공되는 명령 프롬프트](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code/commands)를 재사용합니다.

## 리뷰 실행

OCR이 호출할 LLM을 설정하고 연결을 확인합니다.

```bash
ocr config provider
ocr config model
ocr llm test
```

Qwen Code의 기본 `/review`가 활성화되어 있으면 다음 명령으로 OCR을 호출합니다.

```text
/open-code-review.review
/open-code-review.review --from main --to feature
/open-code-review.review --commit abc123
```

Qwen Code는 기본 `/review`를 유지하고 충돌하는 OCR 명령에
`open-code-review.` 접두사를 붙입니다. 기본 리뷰 스킬을 비활성화하면 OCR이
`/review`를 사용할 수 있습니다. 세션에서 `/help`를 실행하여 실제 명령 이름을
확인하세요.

## 위임 모드

Qwen Code 자체 모델과 인증으로 리뷰하려면 다음 명령을 사용합니다.

```text
/delegate-review
/delegate-review --from main --to feature
/delegate-review --commit abc123
```

OCR은 `ocr delegate preview`와 `ocr delegate rule`로 파일을 선택하고 규칙을
해석하며, Qwen Code가 리뷰를 수행합니다. OCR CLI는 필요하지만 OCR의 LLM을
별도로 설정할 필요는 없습니다.

다른 명령이 이미 `/delegate-review`를 사용하면 OCR 명령은
`/open-code-review.delegate-review`로 등록됩니다.

두 명령 모두 채택한 지적 사항에 대한 수정을 자동으로 적용할 수 있습니다.

## 마켓플레이스 검색

저장소를 소스로 추가하면 Discover 탭에서 플러그인을 찾을 수 있습니다.

```bash
qwen extensions sources add alibaba/open-code-review
```

Qwen Code에서 `/extensions`를 실행하고 Discover를 엽니다.

## 관련 문서

- [위임 모드](../delegate/)
- [설정](../configuration/)
- [Qwen Code 확장 문서](https://qwenlm.github.io/qwen-code-docs/en/users/extension/introduction/)
