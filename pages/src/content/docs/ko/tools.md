---
title: 도구
sidebar:
  order: 9
---

OCR에는 리뷰 중에 LLM이 호출할 수 있는 **내장 도구 여섯 개**가 들어 있습니다. 이
페이지에서는 도구마다 용도와 입력 스키마, 입출력 예시를 다룹니다. 기계가 읽는 전체
정의는
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)에
있습니다.

## 단계별 도구 제공 여부 {#tool-availability-per-phase}

도구마다 **plan 단계**와 **main 작업** 중 어디에 노출될지 선언되어 있습니다.

| 도구 | Plan | Main | 용도 |
|---|---|---|---|
| `task_done` | ✗ | ✓ | "끝났다"고 알려 루프를 끝냅니다. |
| `code_comment` | ✗ | ✓ | 라인 범위와 제안을 담은 리뷰 코멘트를 냅니다. |
| `file_read` | ✗ | ✓ | 변경 이후 스냅숏에서 파일 일부를 읽습니다. |
| `file_read_diff` | ✓ | ✓ | 다른 파일의 diff를 읽어 파일 간 문제를 확인합니다. |
| `file_find` | ✓ | ✓ | 파일 이름 키워드로 파일을 찾습니다. |
| `code_search` | ✓ | ✓ | 저장소 전체를 grep합니다(문자열 또는 정규식). |

`task_done`과 `code_comment`는 plan 단계에서 **일부러** 제공하지 않습니다. 계획
단계는 읽기 전용입니다.

> **맥락 도구는 읽기용이지 코멘트 대상이 아닙니다.** `main_task` 프롬프트는 *다른*
> 파일에서 발견한 문제에 코멘트를 다는 것을 명시적으로 금지합니다. `file_read`,
> `file_read_diff`, `file_find`, `code_search`는 모델이 현재 파일의 diff를 더 잘
> 이해하라고 있는 도구이며, 그 맥락을 모으다 눈에 띈 문제는 설계상 무시합니다. 파일
> 간 문제는 **현재 파일의 diff에서 드러날 때만** 코멘트가 됩니다.

도구 목록을 바꾸려면 내장 파일과 같은 형태의 JSON 파일을 `--tools <path>`로
넘기세요. 도구를 끄거나, 설명을 고치거나, 기존 제공자를 쓰는 새 도구를 추가할 수
있습니다.

## `task_done` {#taskdone}

main 루프를 끝냅니다.

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| 필드 | 필수 | 뜻 |
|---|---|---|
| `state` | 예 | `DONE`(기본값) 또는 `FAILED`. `FAILED`는 "주어진 도구로는 도저히 이 일을 할 수 없다"는 뜻이며, 올바른 선택인 경우가 거의 없습니다. |

Agent가 `task_done`을 보면 LLM 호출을 멈추고 그동안 쌓인 `code_comment` 호출을
처리하기 시작합니다. `task_done`은 결과가 세션 로그에 기록되기 전에 즉시
반환하므로, `state` 값은 받아들이되 **저장하지는 않습니다**. 종료 코드에도 영향을
주지 않습니다.

## `code_comment` {#codecomment}

리뷰 코멘트를 하나 이상 냅니다. 각 코멘트는 코드 조각(`existing_code`)에 고정되므로
OCR이 라인 번호를 알아서 계산합니다.

### 스키마 {#schema}

```json
{
  "name": "code_comment",
  "input": {
    "path": "string — optional, override the file path for this comment",
    "comments": [
      {
        "content": "string — the comment in the configured language",
        "existing_code": "string — snippet from the diff to anchor on",
        "suggestion_code": "string — optional fix snippet",
        "thinking": "string — optional, the model's reasoning for this comment"
      }
    ]
  }
}
```

`comments`가 배열이라 모델이 한 번의 도구 호출로 코멘트를 여러 개 낼 수 있습니다.
`content`와 `existing_code`는 필수이고 `suggestion_code`는 선택이지만 채우는 편이
좋습니다. `path`는 최상위의 선택적 재정의 값이며, 없으면 Agent가 현재 리뷰 중인
파일을 넣습니다. 모델이 빠뜨려도 Agent가 자동으로 채우므로 모델이 직접 지정할 일은
거의 없습니다. 코멘트별 `thinking`은 모델의 판단 근거를 담아 코멘트에 함께
보존합니다. OCR이 현재 턴에서 모델이 낸 추론 내용으로 채우며(모델이 내지 않으면 빈
채로 둡니다) JSON 출력에 포함합니다(터미널 출력에는 표시하지 않습니다).

> **`thinking`은 런타임 전용 필드입니다.** OCR은 이 값을 파싱해 저장하지만,
> `tools.json`에서 모델에 알리는 `code_comment` 스키마에는 일부러 **넣지
> 않았습니다**(`content`, `existing_code`, `suggestion_code`만 있습니다). 그래도
> `thinking` 블록을 내는 성능 좋은 모델이라면 그 값이 저장됩니다. 대부분은 보내지
> 않으며 그래도 문제없습니다.

### 위치 고정 알고리즘 {#anchoring-algorithm}

OCR은 **동적 슬라이딩 윈도**로 diff를 훑어 `existing_code`의 텍스트를 찾습니다.
대조는 다음 순서로 시도합니다.

1. **hunk의 새 쪽** — 연이어 붙은 **문맥 줄과 추가된 줄**의 묶음(삭제된 줄만 있거나
   변경 없는 줄만 있는 묶음은 제외)에서 찾아 새 파일 기준 라인 번호를 얻습니다.
   실패하면 **hunk의 옛 쪽**(문맥 줄과 삭제된 줄)을 다시 시도해 옛 파일 기준 라인
   번호를 얻습니다.
2. **새 파일 전체 훑기** — 어떤 hunk도 걸리지 않으면 변경 이후 파일 내용을 한 줄씩
   훑어 연속으로 일치하는 곳을 찾습니다(`resolveFromFileContent`).
3. **재배치 작업** — 그래도 텍스트 대조가 실패하고 diff가 사소하지 않으면 OCR이
   `RE_LOCATION_TASK` 프롬프트를 돌려 모델에 위치를 다시 잡아 달라고 요청합니다.

대조는 **공백을 가리지 않습니다**. 비교 전에 줄 양끝을 다듬고 diff의 `+`/`-`
표시를 떼어 내므로 들여쓰기가 정확히 같을 필요는 없습니다. 끝내 찾지 못하면 코멘트를
`start_line=0`으로 전달해 "문제는 실재하지만 위치는 직접 찾아야 한다"고 알려 줍니다.

### 예시 {#example}

```json
{
  "comments": [
    {
      "content": "`tx.Rollback()` is never deferred — early returns leak the transaction.",
      "existing_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}",
      "suggestion_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}\ndefer tx.Rollback()"
    }
  ]
}
```

## `file_read` {#fileread}

파일의 **변경 이후** 내용에서 라인 범위를 읽습니다.

### 스키마 {#schema}

```json
{
  "name": "file_read",
  "input": {
    "file_path": "src/foo.go",
    "start_line": 10,
    "end_line": 80
  }
}
```

| 필드 | 필수 | 기본값 | 설명 |
|---|---|---|---|
| `file_path` | 예 | — | 저장소 루트 기준 상대 경로. |
| `start_line` | 아니요 | `1` | 1부터 셉니다. |
| `end_line` | 아니요 | 파일 끝 | 해당 줄을 포함합니다. |

### 출력 {#output}

```
File: src/foo.go (Total lines: 220)
IS_TRUNCATED: false
LINE_RANGE: 10-80
10|package foo
11|
12|import (
13|    "fmt"
…
```

내용의 각 줄 앞에는 1부터 센 라인 번호와 `|` 구분자가 붙습니다. 모델이 이어지는
`code_comment` 호출에서 라인 번호를 정확히 인용할 수 있게 하기 위해서입니다.

### 제한 {#limits}

- **호출당 최대 500줄입니다.** 더 큰 범위는 잘리고 `IS_TRUNCATED: true`가 설정되며,
  끝에 `Note: Results truncated to 500 lines. Please narrow your line range.`가
  붙습니다.
- 파일의 **변경된 쪽만** 읽습니다. 이전 버전을 보려면 `file_read_diff`를 쓰세요.

모델이 주변 맥락이 필요할 때(diff에서만 보이는 함수에 코멘트를 달아야 할 때)는 diff의
hunk 머리글 `@@ -x,y +m,n @@`에서 범위를 계산해야 합니다. 보통 `m-50`부터 `m+n+50`
사이입니다.

## `file_read_diff` {#filereaddiff}

같은 변경 집합에 있는 *다른* 파일의 diff를 읽습니다. 관련된 파일이 함께 수정됐는지에
따라 코멘트가 달라질 때 유용합니다.

### 스키마 {#schema}

```json
{
  "name": "file_read_diff",
  "input": {
    "path_array": ["src/api/handler.go", "src/db/queries.go"]
  }
}
```

### 출력 {#output}

```
==== FILE: src/api/handler.go ====
--- a/src/api/handler.go
+++ b/src/api/handler.go
@@ -10,1 +10,2 @@
- old line
+ new line 1
+ new line 2

==== FILE: src/db/queries.go ====
@@ -5,1 +5,1 @@
- query := "SELECT *"
+ query := "SELECT id"
```

변경 집합에 없는 경로는 조용히 빠집니다. 요청한 경로가 **하나도** 변경 집합에 없으면
`Error: diff not found for the requested paths`를 반환하고, `path_array`가 비어
있으면 `Error: no files found`를 반환합니다.

## `file_find` {#filefind}

파일 이름 키워드로 저장소에서 파일을 찾습니다(부분 문자열 대조).

### 스키마 {#schema}

```json
{
  "name": "file_find",
  "input": {
    "query_name": "UserService",
    "case_sensitive": false
  }
}
```

| 필드 | 필수 | 기본값 | 설명 |
|---|---|---|---|
| `query_name` | 예 | — | 전체 경로가 아니라 각 파일의 **기본 이름**(마지막 `/` 뒤 부분)과 부분 문자열로 대조합니다. |
| `case_sensitive` | 아니요 | `false` | 대소문자를 정확히 맞추려면 `true`로 설정합니다. |

후보 목록은 워크스페이스 모드에서는 `git ls-files --cached --others
--exclude-standard`, range·commit 모드에서는 `git ls-tree -r --name-only <ref>`에서
옵니다. 확장자가 없는 파일은 건너뛰되 `Makefile`, `Dockerfile`, `LICENSE`,
`Vagrantfile`, `Containerfile`은 예외입니다.

### 출력 {#output}

줄바꿈으로 구분한 경로 목록입니다:

```
src/main/java/com/example/UserService.java
src/test/java/com/example/UserServiceTest.java
src/main/java/com/example/internal/UserServiceImpl.java
```

일치하는 파일이 없거나 `query_name`이 비어 있으면 `// The file was not found`라는
문자열을 그대로 반환합니다.

### 제한 {#limits}

최대 **100개**까지 반환하고 나머지는 조용히 잘립니다. 더 넓게 찾아야 한다면 모델이
`code_search`로 넘어가야 합니다.

## `code_search` {#codesearch}

저장소 전체를 대상으로 하는 전문 검색입니다. `git grep`을 쓰므로 `pathspec` 문법을
이해하고 `.gitignore`를 따릅니다.

### 스키마 {#schema}

```json
{
  "name": "code_search",
  "input": {
    "search_text": "TODO|FIXME",
    "file_patterns": ["*.go", ":(exclude)vendor/"],
    "case_sensitive": false,
    "use_perl_regexp": true
  }
}
```

| 필드 | 필수 | 기본값 | 설명 |
|---|---|---|---|
| `search_text` | 예 | — | 문자열 그대로이거나 PCRE 패턴입니다(`use_perl_regexp` 참고). |
| `file_patterns` | 아니요 | 저장소 전체 | pathspec 항목의 배열입니다. 빼려면 `:(exclude)pat`을 쓰세요. |
| `case_sensitive` | 아니요 | `false` | — |
| `use_perl_regexp` | 아니요 | `false` | `true`면 `search_text`를 정규식으로 다룹니다. |

### 출력 {#output}

결과는 파일별로 묶입니다. 각 묶음은 `File: <path>`와 `Match lines: <n>`으로
시작하고, 걸린 곳마다 `line|content` 줄이 이어집니다:

```
File: path/to/example.java
Match lines: 2
433|      String name = toolRequest.get().getName();
438|      logToolRequest(newPath, tool, toolRequest.get());

File: path/to/other.java
Match lines: 1
22|      var req = new ToolRequest();
```

일치하는 곳이 없으면 `No matches found`라는 문자열을 그대로 반환합니다.

### pathspec 사용 예 {#pathspec-cookbook}

| 목적 | `file_patterns` |
|---|---|
| 파일 하나 | `["src/main.go"]` |
| Go 파일 전체 | `["*.go"]` |
| 테스트를 뺀 Go 파일 | `["*.go", ":(exclude)*_test.go"]` |
| 디렉터리 하나만 | `["src/api/"]` |
| 여러 확장자에서 vendor 제외 | `["*.go", "*.ts", ":(exclude)vendor/", ":(exclude)node_modules/"]` |

### 제한 {#limits}

- `git grep --max-count 100`으로 **파일당 100건**까지만 받으므로, 파일이 많으면 전체
  출력은 100건을 넘을 수 있습니다. 파일당 상한에 걸리면 출력 앞에 `Note: The results
  have been truncated. Only showing first 100 results.`가 붙습니다.
- `search_text`가 비어 있거나 공백뿐이면 모든 줄로 번지지 않고 `Error: search_text is
  blank`를 반환합니다.
- 워크스페이스 모드에서는 **현재 작업 트리**를, range·commit 모드에서는 해석된 대상
  ref를 검색합니다(`FileReader.Ref`가 `git grep`에 위치 인자로 넘어갑니다).

## 도구 실행과 오류 {#tool-execution-errors}

도구는 Agent 루프 안에서 동기적으로 실행되며, 예외가 둘 있습니다.

- `code_comment`는 **CommentWorkerPool**로 넘겨, 라인 위치 해석과 재검토 때문에
  루프가 막히지 않게 합니다.
- `task_done`은 곧바로 빠져나갑니다. 어떤 제공자도 호출하지 않고 즉시 반환합니다.

도구에서 오류가 나면(네트워크 실패, 잘못된 인자, 파일 없음) 결과가 평범한 도구 결과
형태로 모델에 전달됩니다. 예를 들면 `"Error: file not found: src/missing.go"` 같은
텍스트입니다. 그다음 다시 시도할지, 다른 파일을 요청할지, `task_done`을 부를지는
모델이 정합니다.

등록되지 않은 도구 이름이 오면 OCR은 죽지 않고 `tool.NotAvailableMsg` 상수를
반환합니다. 덕분에 `--tools`로 실행 시점에 도구를 꺼도 안전합니다.

## 도구 커스터마이즈 {#customizing-tools}

확장하는 방법은 두 가지입니다.

### 1. 도구 끄기 {#1-disable-a-tool}

`tools.json`을 복사해 원하지 않는 항목을 지운 뒤 실행합니다:

```bash
ocr review --tools ./my-tools.json
```

예를 들어 추가 맥락을 전혀 읽지 않는 "코멘트 전용" 리뷰어를 원한다면
`code_comment`와 `task_done`만 남기세요.

### 2. 도구 설명 바꾸기 {#2-re-describe-a-tool}

`name`은 그대로 두고(내부적으로 이름으로 제공자를 찾습니다) `description`만 고쳐
모델을 유도합니다. 프로젝트 고유의 지침을 넣는 가장 쉬운 방법입니다. 예를 들면
"`file_read`를 쓸 때는 변경 지점 주위를 최소 30줄은 읽어라"처럼 적을 수 있습니다.

> **새로운** 도구 *이름*을 추가하려면 Go 쪽 작업이 필요합니다.
> `internal/tool/definitions.go`와 `internal/tool/` 아래의 제공자들을 참고하세요.
> JSON 파일만으로는 새 동작을 추가할 수 없습니다.

## 관련 문서 {#see-also}

- [아키텍처](../architecture/) — Agent 루프가 도구를 호출하는 방식입니다.
- [리뷰 규칙](../review-rules/) — LLM에 무엇에 집중하라고 지시하는지 다룹니다.
- [세션 뷰어](../viewer/) — 지난 리뷰에서 어떤 도구가 실행됐는지 정확히 확인할 수
  있습니다.
