---
title: 네이티브 플러그인
sidebar:
  order: 8
---

네이티브 플러그인은 추가 리뷰 규칙을 로컬 디렉터리에 묶습니다. OCR은 디렉터리를
사용자 단위 스냅샷으로 복사합니다. 활성화한 플러그인은 모든 저장소의 `ocr review`와
`ocr scan`에 규칙을 추가하며, 패키지 파일은 데이터로 읽습니다.

## 설치 및 확인 {#install-and-inspect}

OCR 소스 체크아웃에서 함께 제공되는 예제를 설치합니다.

```bash
ocr plugin install ./examples/ocr-plugins/review-checklist
ocr plugin list
ocr plugin show review-checklist
ocr plugin enable review-checklist
ocr rules check src/main.go
```

설치 직후에는 비활성 상태입니다. `--enable`을 추가하면 설치하면서 활성화합니다.
`ocr rules check`는 일치하는 각 플러그인의 ID, 버전, 규칙 팩 경로, 패턴과 결합된
규칙 본문을 표시합니다. 모든 플러그인 하위 명령은 `--json`을 지원합니다.
예: `ocr plugin list --json`.

활성화하기 전에 규칙 본문을 확인하세요. 모델의 리뷰 프롬프트에 포함됩니다.
설치 원본은 `ocr-plugin.json`이 있는 로컬 디렉터리여야 합니다.
URL, GitHub 저장소 참조, ZIP 또는 tar.gz 압축 파일은 지원하지 않습니다.

## 패키지 형식 {#package-format}

예제에는 세 파일이 있습니다.

```text
review-checklist/
  ocr-plugin.json
  rules.json
  checklist.md
```

`ocr-plugin.json`:

```json
{
  "schema_version": 1,
  "id": "review-checklist",
  "name": "Review Checklist",
  "version": "1.0.0",
  "description": "Additional review checks for error handling and resource ownership.",
  "rules": ["rules.json"],
  "requires": {"features": ["rules"]}
}
```

| 필드 | 요구 사항 |
|---|---|
| `schema_version` | 필수. 값은 `1`. |
| `id` | 필수. 소문자 ASCII 영문자, 숫자, `.`, `_`, `-`로 된 1~64자. 처음과 끝은 영문자나 숫자. Windows 예약 이름은 거부. |
| `name`, `version` | 필수. 공백만 있는 문자열은 허용하지 않음. |
| `description` | 선택 사항. `ocr plugin show`에 표시할 설명. |
| `rules` | 필수. 패키지 기준 상대 `.json` 경로를 순서대로 나열한 비어 있지 않은 목록. 중복 경로는 허용하지 않음. |
| `requires.ocr_min_version` | 선택 사항. `1.0.0`과 같은 완전한 시맨틱 버전. OCR 빌드 버전과 비교. |
| `requires.features` | 선택 사항. 필요한 기능 목록. 지원하는 값은 `rules`. |
| `metadata` | 선택 사항. 해석하거나 실행하지 않고 보관하는 JSON 객체. |

매니페스트의 알 수 없는 필드와 중복 JSON 키는 거부합니다. 설치, 활성화, 활성 플러그인
불러오기 시 호환성을 확인합니다. `dev` 빌드는 `ocr_min_version`을 충족할 수 없으므로
개발 빌드에서 사용할 패키지는 이 요구 사항을 생략하세요.

`rules.json`:

```json
{
  "rules": [
    {
      "path": "**/*.{go,js,mjs,ts,tsx,py}",
      "rule": "checklist.md"
    }
  ]
}
```

`checklist.md`에는 리뷰 지침을 작성합니다.

```text
Check error propagation, cleanup, and resource ownership.
Report only concrete failure paths in the changed code.
Verify the relevant caller and explain the trigger and user-visible consequence.
```

`rule`에 본문을 직접 쓸 수도 있습니다. Markdown 참조는 규칙 팩 파일을 기준으로
해석하며 패키지 안에 있어야 합니다. 경로에는 슬래시(`/`)를 사용하세요.
규칙 팩은 리뷰 규칙을 제공합니다. `include`와 `exclude`는 프로젝트 또는 사용자
규칙에서 설정하세요. 플러그인의 비어 있지 않은 두 목록은 거부합니다.

## 규칙 결합 방식 {#how-rules-combine}

OCR은 먼저 `--rule`, 프로젝트, 전역, 시스템 순서로 기존 기본 규칙을 결정합니다.
그다음 활성 플러그인을 ID 오름차순으로, 각 매니페스트의 `rules` 순서대로 추가합니다.
각 규칙 팩에서는 첫 번째로 일치하는 항목을 적용하므로 여러 팩이 같은 파일에
동시에 지침을 추가할 수 있습니다.

플러그인 규칙은 항상 추가되며, 팩의 `merge_system_rule`은 효과가 없습니다.
리뷰 대상 파일은 기존 파일 필터가 결정합니다. 패턴, 필터링, 기본 우선순위는
[리뷰 규칙](../review-rules/)을 참고하세요.

## 비활성화, 교체 및 제거 {#disable-replace-and-remove}

```bash
ocr plugin disable review-checklist
ocr plugin remove review-checklist
ocr plugin install ./examples/ocr-plugins/review-checklist --enable
```

교체하기 전에 설치된 ID를 제거하세요. 원본 디렉터리를 수정한 내용은 다시 설치한 뒤에
반영됩니다. 상태는 `~/.opencodereview/plugins/state.json`에, 스냅샷은
`plugins/packages/`에 저장됩니다. 제거하면 등록이 해제되며 실행 중인 리뷰에 필요한
스냅샷은 보관합니다.

재개할 때 쓰는 구성 식별 정보에는 활성 매니페스트와 패키지 내용이 포함됩니다.
같은 내용을 다시 설치하면 식별 정보가 유지되고, 내용이 바뀌면 이전 플러그인 구성의
결과는 재사용되지 않습니다.

패키지는 최대 64 MiB이며 디렉터리를 포함해 최대 4,096개 항목을 허용합니다.
매니페스트 한도는 1 MiB, 불러올 때 각 규칙 팩 JSON의 한도는 512 KiB입니다.
심볼릭 링크, 특수 파일, 대소문자만 다른 충돌 이름, Windows 예약 파일 이름,
패키지 밖으로 나가는 경로는 거부합니다. 스냅샷은 그대로 두고 원본 디렉터리를
수정한 다음 다시 설치하세요.

명령 요약은 [CLI 참조](../cli-reference/#ocr-plugin)를 참고하세요.
