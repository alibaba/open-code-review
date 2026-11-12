---
title: 도구
sidebar:
  order: 9
---

OCR에는 리뷰 중에 LLM이 호출할 수 있는 **내장 도구 여섯 개**가 들어 있습니다. 이
페이지에서는 도구마다 용도와 입력 스키마, 입출력 예시를 다룹니다. 기계가 읽는 기본
정의는
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)에
있습니다.

## 단계별 도구 제공 여부 {#tool-availability-per-phase}

작업 템플릿은 공유 정의에서 이름으로 도구를 선택합니다. `task_template.json`의
`MAIN_TASK.tools`는 모델에 전달하는 호출 가능한 도구의 순서 있는 목록이며,
`PLAN_TASK.tool_references`는 계획 프롬프트에 참고 텍스트로만 렌더링됩니다.
계획은 호출 가능한 도구가 없는 단일 요청으로 유지되며 두 필드는 도구 정의를 변경하지 않습니다.

`scan_template.json`은 main 작업 도구를 독립적으로 선택하며 전체 파일 스캔에는 diff가 없으므로
`file_read_diff`를 제외합니다. 생략되거나 빈 목록은 도구를 선택하지 않으며 전체 카탈로그를 활성화하지 않습니다.
중복 이름은 처음 나타난 순서로 한 번만 포함됩니다. 알 수 없는 참조, 중복 카탈로그 항목,
잘못된 정의는 작업 초기화 시 오류로 처리됩니다.

기본 diff 리뷰의 선택은 다음과 같습니다.

| 도구             | Plan | Main | 용도                                               |
| ---------------- | ---- | ---- | -------------------------------------------------- |
| `task_done`      | ✗    | ✓    | "끝났다"고 알려 루프를 끝냅니다.                   |
| `code_comment`   | ✗    | ✓    | 라인 범위와 제안을 담은 리뷰 코멘트를 냅니다.      |
| `file_read`      | ✓    | ✓    | 선택한 비교 쪽에서 파일 일부를 읽습니다.           |
| `file_read_diff` | ✓    | ✓    | 다른 파일의 diff를 읽어 파일 간 문제를 확인합니다. |
| `file_find`      | ✓    | ✓    | 파일 이름 키워드로 파일을 찾습니다.                |
| `code_search`    | ✓    | ✓    | 저장소 전체를 grep합니다(문자열 또는 정규식).      |

`task_done`과 `code_comment`는 plan 단계에서 **일부러** 제공하지 않습니다. 계획
단계는 읽기 전용입니다.

> **맥락 도구는 읽기용이지 코멘트 대상이 아닙니다.** `main_task` 프롬프트는 _다른_
> 파일에서 발견한 문제에 코멘트를 다는 것을 명시적으로 금지합니다. `file_read`,
> `file_read_diff`, `file_find`, `code_search`는 모델이 현재 파일의 diff를 더 잘
> 이해하라고 있는 도구이며, 그 맥락을 모으다 눈에 띈 문제는 설계상 무시합니다. 파일
> 간 문제는 **현재 파일의 diff에서 드러날 때만** 코멘트가 됩니다.

내장 도구 정의는 OCR과 함께 배포되며 실행 시점에 덮어쓸 수 없습니다.
프로젝트 지침에는 [리뷰 규칙](../review-rules/), 외부 업무 맥락에는 [MCP](../mcp/)를
사용하세요. 커스텀 도구 파일을 사용했다면 [이전 안내](#customizing-tools)를 확인하세요.

## 비교할 쪽 {#comparison-sides}

재개한 세션을 포함한 commit 및 range 리뷰에서는 `file_read`, `code_search`, `file_find`에 선택적
`side` 매개변수를 제공합니다. 생략하거나 `"new"`를 전달하면 기본 동작과 출력 형식을
유지합니다. `"old"`를 전달하면 같은 비교의 이전 쪽에서 파일을 읽거나 검색하거나
목록을 조회할 수 있습니다.

```json
{"file_path": "src/foo.go", "start_line": 10, "end_line": 80, "side": "old"}
{"search_text": "OldAPI", "file_patterns": ["src/"], "side": "old"}
{"query_name": "foo.go", "side": "old"}
```

| 리뷰 입력 | 새 쪽 | 이전 쪽 |
| --- | --- | --- |
| `--from BASE --to HEAD` | 고정된 head 커밋 | 고정된 양 끝점의 merge-base이며 대상 브랜치의 최신 커밋이 아님 |
| `--commit COMMIT` | 고정된 커밋 | 첫 번째 부모 커밋이며 병합 커밋도 동일 |
| 루트 커밋 | 고정된 커밋 | 빈 트리 |

diff와 세 도구는 동일한 불변 끝점을 공유합니다. 경로와 라인 번호는 선택한 쪽을
기준으로 합니다. 이름이 바뀐 파일은 이전 쪽에서 이전 경로를 사용해야 합니다.
도구는 이름을 자동 변환하거나 다른 버전으로 대체하지 않습니다. 삭제된 파일도
이전 쪽에서는 읽을 수 있습니다. 루트 커밋의 이전 쪽에는 파일이 없으므로 읽기는
파일이 없다는 오류를 반환하고 검색과 목록 조회는 빈 결과를 반환합니다. Git 객체를
사용할 수 없거나 Git 작업이 실패하면 오류이며, 빈 트리로 간주하지 않습니다.
얕은 클론의 경계에 있는 커밋은 부모 객체가 로컬에 있어도 리뷰 전에 필요한 이력을
추가로 가져와야 합니다. 그래야 Git diff와 이전 쪽 도구가 같은 비교 기준을 사용합니다.

이전 쪽 결과는 `SIDE: old`와 `REVISION: <commit-sha>`로 시작하며, 빈 트리에는
`SOURCE: empty-tree`를 사용합니다. 이전 쪽 오류에도 출처가 표시됩니다. 기존 라인 수와
일치 결과 수 제한은 양쪽에 적용됩니다. 문자열 `"new"`와 `"old"`만 허용하며 null,
빈 문자열, 임의의 Git ref는 유효하지 않습니다.

작업 공간 리뷰와 스캔은 기존 정의, 프롬프트, 기본 도구 출력 형식을 유지합니다.
`side`를 노출하지 않으며 전달하면 거부합니다. 공유 `tools.json` 카탈로그는
current-only 정의의 원본이며 `side`를 선언하지 않습니다. 비교 리뷰는 초기화 시
`WithComparisonSides`로 카탈로그에서 호출 가능한 정의와 계획용 참조를 파생합니다.
이 과정에서 기본 정의는 수정하지 않고 `side`를 추가하고 설명을 갱신합니다.

commit 또는 range 리뷰를 재개하면 현재 내장 도구를 사용하며, 선택한 patch가 그대로여도
해석된 새 끝점과 이전 끝점이 원래와 같아야 합니다. 어느 트리에서든 다른 부분의 문맥이
이전 리뷰에 영향을 주었을 수 있기 때문입니다. ref 이름이 달라도 같은 끝점을 가리키면
재개할 수 있습니다. 끝점 중 하나라도 바뀌거나 이전 세션의 끝점 정보가 부족해 비교를
검증할 수 없다면 새 리뷰를 시작하세요. 완료된 파일의 결과는 재사용하고 미완료 파일은
현재 도구로 리뷰합니다. 모든 파일을 현재 기능으로 다시 확인하려면 새 리뷰를 시작하세요.

## `task_done` {#taskdone}

main 루프를 끝냅니다.

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| 필드    | 필수 | 뜻                                                                                                                                 |
| ------- | ---- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `state` | 예   | `DONE`(기본값) 또는 `FAILED`. `FAILED`는 "주어진 도구로는 도저히 이 일을 할 수 없다"는 뜻이며, 올바른 선택인 경우가 거의 없습니다. |

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

기본적으로 파일의 **변경 이후** 내용에서 라인 범위를 읽습니다.
commit 및 range 리뷰에서는 선택적으로 [비교할 쪽](#comparison-sides)도 지정할 수 있습니다.

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

| 필드         | 필수   | 기본값  | 설명                        |
| ------------ | ------ | ------- | --------------------------- |
| `file_path`  | 예     | —       | 저장소 루트 기준 상대 경로. |
| `start_line` | 아니요 | `1`     | 1부터 셉니다.               |
| `end_line`   | 아니요 | 파일 끝 | 해당 줄을 포함합니다.       |

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
- 기본적으로 파일의 **변경된 쪽**을 읽습니다. commit 및 range 리뷰에서 `side: "old"`를
  사용하면 이전 쪽의 전체 내용을 읽을 수 있습니다. 변경 부분을 보려면 `file_read_diff`를 쓰세요.

모델이 주변 맥락이 필요할 때(diff에서만 보이는 함수에 코멘트를 달아야 할 때)는 diff의
hunk 머리글 `@@ -x,y +m,n @@`에서 범위를 계산해야 합니다. 보통 `m-50`부터 `m+n+50`
사이입니다.

## `file_read_diff` {#filereaddiff}

같은 변경 집합에 있는 _다른_ 파일의 diff를 읽습니다. 관련된 파일이 함께 수정됐는지에
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

변경 집합에 없는 경로는 조용히 버려지지 않습니다. `Error: diff not found for: <path>`에
해당 경로가 명시되고, 찾은 경로의 diff는 오류보다 먼저 반환됩니다. 다른 파일의 diff가
있는 실행에서는 메시지가 그 목록을 보여주어(`Diffs exist for: ...`) 잘못 쓴 경로를
고칠 수 있게 하고, diff가 전혀 없는 실행에서는 `file_read`를 대신 쓰도록 안내합니다.
비어 있는 `path_array`는 `Error: no files found`를 반환합니다.

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

| 필드             | 필수   | 기본값  | 설명                                                                                       |
| ---------------- | ------ | ------- | ------------------------------------------------------------------------------------------ |
| `query_name`     | 예     | —       | 전체 경로가 아니라 각 파일의 **기본 이름**(마지막 `/` 뒤 부분)과 부분 문자열로 대조합니다. |
| `case_sensitive` | 아니요 | `false` | 대소문자를 정확히 맞추려면 `true`로 설정합니다.                                            |

후보 목록은 워크스페이스 모드에서는 `git ls-files --cached --others
--exclude-standard`, range·commit 모드에서는 `git ls-tree -r --name-only <ref>`에서
옵니다. 확장자가 없는 파일은 건너뛰되 `Makefile`, `Dockerfile`, `LICENSE`,
`Vagrantfile`, `Containerfile`, `BUILD`, `WORKSPACE`는 예외입니다.

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

| 필드              | 필수   | 기본값      | 설명                                                         |
| ----------------- | ------ | ----------- | ------------------------------------------------------------ |
| `search_text`     | 예     | —           | 문자열 그대로이거나 PCRE 패턴입니다(`use_perl_regexp` 참고). |
| `file_patterns`   | 아니요 | 저장소 전체 | pathspec 항목의 배열입니다. 빼려면 `:(exclude)pat`을 쓰세요. |
| `case_sensitive`  | 아니요 | `false`     | —                                                            |
| `use_perl_regexp` | 아니요 | `false`     | `true`면 `search_text`를 정규식으로 다룹니다.                |

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

| 목적                        | `file_patterns`                                                    |
| --------------------------- | ------------------------------------------------------------------ |
| 파일 하나                   | `["src/main.go"]`                                                  |
| Go 파일 전체                | `["*.go"]`                                                         |
| 테스트를 뺀 Go 파일         | `["*.go", ":(exclude)*_test.go"]`                                  |
| 디렉터리 하나만             | `["src/api/"]`                                                     |
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
반환해 모델에 해당 도구를 사용할 수 없다고 알립니다.

## 도구 커스터마이즈 제한과 이전 {#customizing-tools}

OCR은 전용 리뷰 Agent를 사용합니다. 핵심 워크플로와 내장 도구는 사용자 정의
레지스트리가 아니라 테스트와 벤치마크를 통해 함께 검증합니다.

**이전 안내:** `ocr review`와 `ocr scan` 모두에서 `--tools`를 제거했습니다.
기존 호출은 `unknown flag: --tools`로 실패합니다. 스크립트, CI 작업, shell 별칭에서
이 플래그와 파일 인자를 삭제하세요.

- 프로젝트별 리뷰 지침에는 [리뷰 규칙](../review-rules/)이나 `--background`를
  사용하세요(`review`는 `--background-file`도 지원합니다).
- 요구사항, 지식 베이스 등 외부 업무 맥락에는 [MCP 서버](../mcp/)를 설정하세요.
  현재 MCP는 `review`에만 연결되며 `scan`에는 연결되지 않습니다.
  MCP는 같은 이름의 내장 도구를 대체할 수 없습니다.
- 내장 도구 비활성화나 설명, schema, 단계별 공개 설정 변경을 위한 실행 시점의
  대체 수단은 없습니다. 핵심 변경에는 소스 수정, 재빌드, 적절한 테스트와 벤치마크 검증이 필요합니다.

`--max-tools`는 계속 도구 호출 라운드 예산을 제어합니다.
MCP 설정 `mcp_servers.<name>.tools`도 외부 도구 필터로 유지되며 제거한 플래그와는 무관합니다.
기존 세션 파일은 읽을 수 있지만, 커스텀 도구 정의를 사용했던 세션은 재개하지 말고
새 리뷰를 시작하세요.

## 관련 문서 {#see-also}

- [아키텍처](../architecture/) — Agent 루프가 도구를 호출하는 방식입니다.
- [리뷰 규칙](../review-rules/) — LLM에 무엇에 집중하라고 지시하는지 다룹니다.
- [세션 뷰어](../viewer/) — 지난 리뷰에서 어떤 도구가 실행됐는지 정확히 확인할 수
  있습니다.
