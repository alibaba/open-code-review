# 리뷰 워크플로 플러그인 (Review Workflow Plugins)

[English](../review-workflow-plugins.md) | [简体中文](review-workflow-plugins.zh-CN.md) | [日本語](review-workflow-plugins.ja-JP.md) | [한국어](review-workflow-plugins.ko-KR.md) | [Русский](review-workflow-plugins.ru-RU.md)

패키지 설치, 실행 권한 및 런타임 어댑터 간의 관계는 [네이티브 플러그인 아키텍처 가이드](../plugin-architecture.md)를 참조하십시오.

코드 리뷰에서 특정 실행 단계 순서와 완료 기준이 필요한 경우(예: 먼저 변경 사항을 검사한 후 후보 지적 사항을 별도 단계에서 재검증하고 최종 완료 보고서를 생성하는 경우) **리뷰 워크플로 플러그인**을 사용합니다. 패키지는 해당 단계를 선언하고 모델의 제출물을 검증하며, OCR 호스트는 diff, 규칙, 컨텍스트 도구 및 토큰 예산을 제공합니다.

## single-pass 예제 실행

Python 3이 `python`으로 사용 가능한 환경에서 예제를 설치하고 작업 트리의 변경 사항에 대해 미리보기를 실행합니다:

```sh
ocr plugin install ./examples/ocr-plugins/single-pass
ocr review --review-plugin single-pass --preview
```

모델 프로바이더가 설정되어 있고 리뷰할 변경 사항이 있는 상태에서 다음을 실행합니다:

```sh
ocr review --review-plugin single-pass --format json --output review.json
```

이 예제는 1개 단계로 구성됩니다. 모델에 지적 사항을 요청하고 OCR이 코멘트를 게시하기 전에 플러그인이 제출 내용을 검증합니다. `review.json`의 `review_plugin`에서 단계 완료 상태 및 보고서를 확인할 수 있습니다. 설치만으로는 패키지 코드가 실행되지 않으며, 미리보기(`--preview`)에서도 Python 프로세스나 모델을 호출하지 않습니다.

위 명령은 전역 설치된 패키지를 사용합니다. 리포지토리에 프로젝트 플러그인이 선언된 경우, 아래 프로젝트 워크플로를 사용하여 선언 및 승인해야 합니다. 전역 플러그인을 명시적으로 지정해도 프로젝트의 잠긴 세트를 우회할 수 없습니다.

## 프로젝트 선택 및 실행 권한

리포지토리에서 관리되는 워크플로의 경우 `ocr plugin add <source>`로 선언하고 `.opencodereview/plugins.json`과 `plugins.lock.json`을 커밋한 후 `ocr plugin sync --frozen`으로 동기화 설치합니다. `--review-plugin <id>`로 선택하기 전에 잠긴 패키지를 검사하고 `ocr plugin trust`를 실행하여 로컬 실행 권한을 부여하십시오. `ocr plugin preflight --json`을 실행하면 워크플로를 시작하거나 모델을 호출하지 않고도 설치 내용, 호환성 및 로컬 권한 상태를 검증할 수 있습니다. 자세한 내용은 [네이티브 OCR 플러그인 가이드](native-plugins.ko-KR.md#project-plugins)를 참조하십시오.

리뷰 워크플로 플러그인은 단계 계획, 프롬프트 지침, 완료 스키마, 검증 및 단계 간 상태를 자체적으로 유지합니다. OCR은 diff, 규칙, 모델/도구 실행, 세션, 토큰 계산, 코멘트 처리 및 출력을 담당합니다.

단일 실행에서 워크플로 선택:

```sh
ocr review --review-plugin PACKAGE_ID --preview
ocr review --review-plugin PACKAGE_ID --format json --output review.json
```

플러그인 설치나 활성화만으로는 워크플로가 실행되지 않습니다. `--preview`는 매니페스트만 검사하며 프로세스를 시작하지 않습니다. 실제 리뷰 실행 시에는 로컬 MCP 서버와 마찬가지로 현재 사용자의 로컬 권한으로 실행 파일이 실행됩니다. `--review-plugin`으로 명시적으로 선택한 경우, 전역에서 비활성화되어 있더라도 해당 플러그인의 규칙과 MCP 서버가 이번 실행에 함께 로드됩니다.

## 매니페스트 구성 (Manifest)

```json
{
  "schema_version": 1,
  "id": "example-review",
  "name": "Example Review",
  "version": "1.0.0",
  "review": {
    "protocol": "stdio-v1",
    "command": "${plugin_dir}/bin/${os}-${arch}/reviewer${exe}",
    "args": ["--policy", "${plugin_dir}/policy.md"],
    "stages": ["inspect", "verify"]
  }
}
```

`command`는 실행 파일 경로이며 셸 구문 분석을 거치지 않습니다. 명령어와 인자에서는 `${plugin_dir}`, `${os}`, `${arch}`, `${exe}`(Windows는 `.exe`, 기타 플랫폼은 빈 문자열)가 치환됩니다. PATH에 있는 `python` 등의 런타임도 지정할 수 있습니다. 번들된 Python 예제는 `python` 명령어를 기본으로 합니다. 시스템에 `python3`만 있는 경우 패키징 또는 설치 전에 매니페스트의 명령어를 `python3`으로 변경하십시오.

플러그인은 리뷰 워크플로, 규칙 및 MCP 도구를 자유롭게 결합할 수 있습니다. 단계 ID는 고유한 식별자이며 1개에서 64개의 순차적 실행 단계를 선언할 수 있습니다.

설치된 스냅샷은 이번 실행의 규칙과 워크플로를 함께 제공합니다. 패키지를 업데이트해도 실행 중인 스냅샷은 바뀌지 않습니다. 이전 `engine`/`prompt` 실험 형식만 지원하는 호스트는 업그레이드해야 합니다. 호스트는 이전 설치 메타데이터를 읽어 목록 조회, 삭제, 업데이트를 허용합니다. 관리 출력은 업데이트 필요 상태(JSON의 `review.update_required`)를 표시하고 실행 안내는 생략합니다. 이전 형식으로 실행하려 하면 업데이트를 요청합니다.

## 호환성

워크플로는 선택적으로 `requires.ocr_min_version`과 `requires.features`를 선언할 수 있습니다. 예: `["rules", "mcp", "stdio-v1"]`. 자세한 조건은 [호스트 요구 사항](native-plugins.ko-KR.md#compatibility-and-host-requirements)을 참고하세요. 충족하지 못하는 요구 사항은 프로세스 시작 전에 거부됩니다. 워크플로 API는 실험 단계입니다. 기존 `stdio-v1`은 같은 `begin`/`submit` 메시지를 사용하며 필수 핸드셰이크나 JSON-RPC 마이그레이션이 없습니다. 알 수 없는 응답 봉투 필드는 오류입니다. `report`와 완료 인자의 구조는 플러그인이 정의합니다. 호환되지 않는 봉투 변경은 새 프로토콜 또는 메시지 버전을 사용해야 합니다.

## stdio-v1 프로토콜 사양

OCR은 파일 그룹마다 하나의 프로세스를 시작하고 검토 대상 저장소를 작업 디렉터리로 사용합니다. 프로세스는 그룹의 단계 사이에 상태를 유지합니다. 요청과 응답은 한 줄의 UTF-8 JSON 객체이며 최대 4 MiB입니다. 요청마다 정확히 하나의 응답을 반환하고 진단은 stderr에 출력하세요. 각 응답에는 `"version": 1`이 필요합니다. 비어 있지 않은 `error` 문자열은 요청을 수행할 수 없음을 나타내며 그룹을 중단합니다.

### 단계 시작 (`begin`)

각 단계 시작 전에 OCR이 전송합니다:

```json
{"version":1,"method":"begin","stage":"inspect","paths":["main.go"],"severities":["critical","high","medium","low"],"categories":["bug","security","performance","maintainability","test","style","documentation","other"]}
```

플러그인은 작업 지침과 초기 보고서를 반환합니다:

```json
{"version":1,"task":{"instructions":"선택된 코드를 검사하십시오.","input":"공개 API 반환 계약을 확인하십시오.","completion":{"description":"결과를 제출하십시오.","parameters":{"type":"object","properties":{"checked":{"type":"boolean"}},"required":["checked"]}}},"report":{"pending":["inspection"]}}
```

OCR은 instructions와 input을 일반 diff/규칙 컨텍스트에 추가하고 컨텍스트 압축 후에도 유지합니다. `instructions`와 `input`은 별도 모델 메시지가 되므로 모두 공백 이외의 텍스트가 필요합니다. 모델은 OCR의 컨텍스트 도구를 계속 사용할 수 있습니다. `task_done` 정의는 `completion`에서 가져옵니다. 직접 `code_comment`를 호출하는 도구는 제공하지 않아 게시할 코멘트가 플러그인 검증을 거치게 합니다.

프로토콜 요청에는 모델 자격 증명이 포함되지 않습니다. 그러나 실행 파일은 일반 프로세스 환경을 상속하며 여기에는 프로바이더 토큰이나 프록시 설정이 있을 수 있습니다. 사용자 권한으로 읽을 수 있는 설정 파일에도 접근할 수 있습니다. 이 릴리스에는 런타임 샌드박스가 없으므로 신뢰할 수 있는 작성자의 워크플로만 설치하고 선택하세요.

### 모델 제출 (`submit`)

모델이 `task_done`을 호출하면 OCR은 인자를 그대로 전달합니다:

```json
{"version":1,"method":"submit","stage":"inspect","arguments":{"checked":true}}
```

제출을 수락하는 응답에서는 명시적인 `accepted` 및 `complete` 불리언 값을 반환합니다:

```json
{"version":1,"accepted":true,"complete":true,"report":{"checked":true},"comments":[]}
```

보고서 구조와 완료 조건은 플러그인이 정의합니다. `accepted:true`와 `complete:false`를 반환하면 그룹을 미완료 상태로 중단합니다. `feedback`은 미완료 사유로 저장되며 모델 수정을 재개하지 않습니다. 수락된 코멘트는 작업이 미완료여도 일반 게시 및 필터링 절차를 따릅니다. 코멘트는 선택된 경로, 비어 있지 않은 내용, `begin`에서 제공한 severity/category 값을 사용해야 합니다. 나머지 필드는 `existing_code`, `suggestion_code`, `start_line`, `end_line`, `thinking`입니다.

모델이 수정할 수 있는 제출 오류에는 `accepted:false`, 비어 있지 않은 `feedback`, 최신 `report`를 반환하고 코멘트를 포함하지 마세요. OCR은 기존 요청 한도 안에서 피드백을 모델에 전달합니다. 플러그인은 유효한 부분 결과를 직접 보존해야 합니다. 프로세스 충돌, 잘못된 프로토콜 응답, 필수 불리언 누락은 실행을 중단하며 모델 오류로 재시도하지 않습니다.

## 완료 상태 및 출력

앞 단계에서 문제가 발견되지 않았더라도 선언된 모든 단계가 완료되어야 합니다. `--effort`는 플러그인 계획을 줄이지 않습니다. 기존 컨텍스트, 도구 라운드, 타임아웃 설정이 적용되며 그룹 타임아웃은 선언된 단계 수를 포함합니다. 사용자가 토큰 예산을 지정하면 남은 단계가 미사용 예산을 공유하며 별도의 총 토큰 제한은 추가하지 않습니다.

JSON의 `review_plugin`은 `plugin`, `group`, `stage`, `complete`, 선택적인 `token_limit`, 플러그인이 정의한 `report`, 실행 `error`를 포함합니다. 이후 모델이나 프로세스가 실패해도 마지막 보고서를 유지합니다. 완료되거나 중단된 각 단계는 텍스트 출력에서도 세션에 `plugin_review`를 저장합니다. `ocr session show SESSION_ID --json`의 `review_plugin` 배열에서 확인할 수 있습니다. 응답에서 `report`를 생략하면 이전 보고서를 유지합니다. 필수 작업이 미완료이면 커버리지에 기록하고 CLI는 0이 아닌 코드로 종료합니다. 일반 리뷰의 종료 동작은 바뀌지 않습니다.

워크플로 리뷰는 새로운 호출이 필요합니다. `--resume`은 아직 임의의 프로세스 상태를 복원하지 않습니다. JSON과 세션 기록은 계속 조회할 수 있습니다.

Python 구현은 [single-pass 예제](../../examples/ocr-plugins/single-pass)를 참고하세요. [Behavior Review](https://github.com/Qiyuanqiii/ocr-behavior-review)는 세 단계와 증거 보존 기능을 갖춘 독립 실행 플러그인입니다.
