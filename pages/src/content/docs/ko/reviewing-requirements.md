---
title: 요구사항에 따른 리뷰
sidebar:
  order: 8
---

관련 인수 기준을 OCR에 전달하면 코드 결함과 함께 비즈니스 동작도 검토할 수 있습니다.
권장 워크플로는 요구사항을 간결한 로컬 Markdown 파일로 작성해 `--background-file`로
전달하는 방식입니다.

## 변경별 요구사항 파일

특정 변경을 리뷰하려면 `docs/requirements/checkout.md`처럼 간결한 로컬 Markdown
파일을 작성하세요.

```markdown
# Checkout requirements

Scope: src/checkout/**
Review changes against these criteria and cite the requirement ID in findings.

- PAY-1: Reject checkout when the cart is empty.
- PAY-2: Calculate the total on the server; never trust a client-supplied total.
- PAY-3: A repeated idempotency key must not create a second charge.

Out of scope: refunds and subscription billing.
```

OCR 설정을 마치고 로컬에 `main`이 있는 상태에서 저장소 루트에서 실행합니다.

```bash
ocr review --from main --to HEAD --background-file docs/requirements/checkout.md
```

`--background-file`은 리뷰 전체에 맥락을 제공합니다. 상대 경로는 **저장소 루트**를
기준으로 해석하며, 하위 디렉터리에서 OCR을 실행해도 같습니다. 절대 경로도 사용할 수
있습니다. `--background`도 지정하면 파일 내용이 우선하고 인라인 텍스트는 무시됩니다.
내용은 간결하게 유지하세요. 정리한 뒤의 내용이 2,000자를 넘으면 경고하고, 8,000자를
넘으면 거부합니다. 입력 파일은 1 MiB 이하여야 하며 정리 후에도 비어 있으면 안 됩니다.

## 내부 지식 베이스의 요구사항

내부 지식 베이스를 활용하려면 관련 요구사항, 제약, 인수 기준을 간결한 로컬 Markdown
파일로 내보내고 `--background-file`로 전달하세요. background 본문에 파일 이름이나
원격 링크만 적는다고 문서나 그 안의 링크를 자동으로 불러오지는 않습니다. 실제 기준을
포함하고, 리뷰에서 발견한 문제를 명세와 테스트에 대조해 확인하세요.

## 현재 제한

OCR은 요구사항을 단일 리뷰의 맥락으로만 다룹니다. 현재 리뷰 규칙과 별개로 참조 문서를
모듈이나 디렉터리에 연결하는 전용 방법은 없습니다. 경로별 규칙은 일치하는 파일에 적용할
규칙 본문을 선택하는 것이므로 참고 자료를 붙이는 수단이 아닙니다. 지식 베이스 활용
사례가 성숙하면 전용 지원을 별도로 설계할 수 있습니다.

## 관련 문서 {#see-also}

- [CLI 레퍼런스](../cli-reference/) — `ocr review --background-file`과 나머지 리뷰 플래그입니다.
- [리뷰 규칙](../review-rules/) — 규칙 계층, 경로별 규칙, 규칙 해석입니다.
