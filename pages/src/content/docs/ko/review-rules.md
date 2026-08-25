---
title: 리뷰 규칙
sidebar:
  order: 7
---

규칙은 OCR이 파일을 리뷰할 때 **무엇에 집중할지** 알려 줍니다. 세 계층의 JSON 파일에
들어 있고, 여기에 바이너리에 내장된 시스템 기본값이 더해집니다.

## 우선순위 사슬 {#priority-chain}

OCR은 **네 겹의 우선순위 사슬**로 규칙을 해석합니다. 파일 경로마다 계층을 순서대로
훑고, 처음 일치한 패턴이 이깁니다.

| 우선순위 | 출처 | 경로 | 설명 |
|---|---|---|---|
| 1(가장 높음) | `--rule` 플래그 | 사용자가 지정 | CLI 재정의. 지정하면 언제나 이깁니다. |
| 2 | 프로젝트 설정 | `<repoDir>/.opencodereview/rule.json` | 프로젝트별 규칙. 커밋해도 됩니다. |
| 3 | 전역 설정 | `~/.opencodereview/rule.json` | 사용자 전체에 적용할 취향. |
| 4(가장 낮음) | 시스템 기본값 | 내장 `system_rules.json` | 주요 언어를 다루는 내장 규칙. |

우선순위가 높은 계층의 파일이 없으면 오류 없이 조용히 건너뜁니다. 그래서
`.opencodereview/rule.json`을 두지 않은 프로젝트는 그대로 전역·시스템 계층으로
내려갑니다.

시스템 계층은 바이너리에 들어 있어 **항상** 존재하므로, 어떤 파일이든 규칙 하나는
해석됩니다.

## 규칙 파일 형식 (1~3계층) {#rule-file-format-layers-1-3}

```json
{
  "include": ["src/**/*.{ts,tsx}", "src/**/*.go"],
  "exclude": ["**/*.test.ts", "**/generated/**"],
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "All exported handlers must validate request bodies before use."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags."
    }
  ]
}
```

서로 독립적인 필드 세 개가 있습니다.

- `include` — 선택. 내장 기본 제외 패턴(아래에서 설명하는 테스트 파일 제외)을
  *건너뛰는* glob 패턴입니다. 화이트리스트가 아닙니다. 어떤 `include` 패턴에도
  걸리지 않은 파일도 `unsupported_ext`와 `default_path` 검사를 계속 거치며 리뷰될 수
  있습니다.
- `exclude` — 선택. OCR이 리뷰하면 *안 되는* 파일의 glob 패턴입니다. 필터 안에서
  가장 높은 우선순위를 가집니다.
- `rules` — `{path, rule}` 항목의 배열이며 **선언 순서대로** 평가합니다. 파일에
  처음 일치하는 `path`가 그 파일을 리뷰할 때 OCR이 모델에 보낼 프롬프트를 정합니다.

### glob 기능 {#glob-features}

OCR은 [`bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4)로
패턴을 맞춥니다.

- `*` — `/`를 제외한 모든 문자와 일치합니다.
- `**` — 디렉터리 경계를 넘어 일치합니다(`src/**/*.go`는 깊이에 상관없이 걸립니다).
- `{a,b,c}` — 중괄호 확장입니다. `*.{ts,tsx,js,jsx}`는 네 패턴으로 펼쳐져 차례로
  대조됩니다.
- `?` — 한 글자와 일치합니다.
- `[abc]` — 문자 클래스입니다.

> 패턴은 **대소문자를 가리지 않고** 대조합니다(파일 경로를 소문자로 바꾼 뒤
> 맞춥니다). 헷갈리면 `ocr rules check <path>`로 확인하세요.

## 파일을 걸러내는 방식 {#how-files-are-filtered}

필터는
[`internal/agent/preview.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/preview.go)에
있는 다섯 관문 알고리즘입니다. diff마다 OCR이 다음을 묻습니다.

1. **`binary`** — 바이너리 파일인가? 그렇다면 제외.
2. **`user_exclude`** — 경로가 사용자 `exclude` 패턴에 걸리는가? 그렇다면 제외.
3. **`user_include`** — 사용자가 `include`를 정의했다면 경로가 거기 걸리는가?
   걸리면 **바로 통과**합니다(아래 `unsupported_ext`와 `default_path` 관문을
   건너뜁니다).
4. **`unsupported_ext`** — 파일 확장자가
   [허용 목록](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)에
   있는가? 없으면 제외.
5. **`default_path`** — 경로가 내장 테스트 파일 제외 패턴(`**/*_test.go`,
   `**/*.test.{js,jsx,ts,tsx}`, `**/*_spec.rb` 등)에 걸리는가? 그렇다면 제외.

다섯 관문을 모두 통과한 파일이 LLM으로 갑니다. `deleted` 사유는 관문이 아니라
`Preview()`에서 따로 계산하며, 새 경로가 `/dev/null`인 파일을 가리킵니다. 리뷰할 새
내용이 없다는 뜻입니다. 토큰을 쓰지 않고 이 필터의 결과만 보려면
`ocr review --preview`를 쓰세요.

### 기본 경로 제외 목록 {#default-path-exclusions}

내장 제외 목록은 테스트 파일 패턴에 걸립니다
([`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json)
참고).

- `**/*_test.go`
- `**/src/test/java/**/*.java`
- `**/src/test/**/*.kt`
- `**/*.test.{js,jsx,ts,tsx}`
- `**/*.spec.{js,jsx,ts,tsx}`
- `**/__tests__/**`
- `**/test/**/*_test.py`
- `**/tests/**/*_test.py`
- `**/*_test.py`
- `**/*_spec.rb`
- `**/spec/**/*_spec.rb`
- `**/*Test.java`
- `**/*Tests.java`
- `**/*_test.rs`
- `**/oh_modules/**`
- `**/*.test.ets`

잡음이 많은 디렉터리(`vendor/`, `node_modules/`, `target/` 등)를 걸러내는 일은 더
앞에서, 파일별 필터가 돌기 전
[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go)의
diff 단계에서 일어납니다.

이런 테스트 파일 패턴에 걸리는 파일을 **리뷰하고 싶다면** 사용자 `include` 목록에
넣으세요. `include`가 기본 경로 관문을 덮어씁니다.

## 파일별 규칙 해석 {#rule-resolution-per-file}

필터가 파일을 리뷰하기로 정하고 나면, OCR은 Agent가 따를 규칙 내용을 고릅니다.

1. `--rule`(커스텀) 계층을 선언 순서대로 확인합니다.
2. `<repo>/.opencodereview/rule.json`을 선언 순서대로 확인합니다.
3. `~/.opencodereview/rule.json`을 선언 순서대로 확인합니다.
4. 내장 시스템 규칙 계층으로 넘어갑니다.

내장 `system_rules.json`에서 고른 패턴을 상대적인 대조 순서대로 아래에 정리했습니다.

| 패턴 | 규칙 문서 |
|---|---|
| `**/*.properties` | `properties.md` — i18n / 설정 파일. |
| `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md` — MyBatis 형식의 매퍼 SQL. |
| `**/pom.xml` | `pom_xml.md` — Maven 의존성. |
| `**/build.gradle` | `build_gradle.md` — Gradle 의존성. |
| `**/package.json` | `package_json.md` — NPM 의존성 / 스크립트. |
| `**/Cargo.toml` | `cargo_toml.md` — Rust 매니페스트. |
| `**/composer.json` | `composer_json.md` — Composer 의존성, 오토로딩, 스크립트, 플러그인, 패키지 설정. |
| `**/*.{json,json5}` | `json.md` — 일반 JSON(`.json5`도 포함). |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md` — GitHub Actions 워크플로 YAML. |
| `.github/**/*.{yaml,yml}` | `github_config.md` — 그 밖의 `.github` 설정 YAML. |
| `**/*.{yaml,yml}` | `yaml.md` |
| `**/*.java` | `java.md` |
| `**/*.go` | `go.md` — Go 소스. |
| `**/*.{ftl,ftlh,ftlx}` | `freemarker.md` — FreeMarker 템플릿(SSTI / XSS / null 처리). |
| `**/*.{hbs,mustache}` | `handlebars_mustache.md` — Handlebars 및 Mustache 템플릿. |
| `**/*.{jinja2,j2}` | `jinja.md` — Jinja 템플릿. |
| `**/*.ets` | `arkts.md` — ArkTS / HarmonyOS. |
| `**/*.astro` | `astro.md` — Astro 컴포넌트와 아일랜드. |
| `**/*.{ts,js,tsx,jsx}` | `ts_js_tsx_jsx.md` |
| `**/*.{kt}` | `kotlin.md` |
| `**/*.rs` | `rust.md` |
| `**/*.R` | `r.md` |
| `**/*.{cpp,cc,hpp}` | `cpp.md` |
| `**/*.c` | `c.md` |
| `**/*.{py,ipynb}` | `python.md` — Python 소스. |
| `**/*.{php,phtml}` | `php.md` — PHP 소스와 PHP 템플릿. |
| `**/*.proto` | `protobuf.md` — Protocol Buffers 통신 호환성. |
| `**/*.po` | `po.md` — gettext 번역 원본 카탈로그. |
| `**/*.pot` | `pot.md` — gettext 템플릿 파일. |
| `**/*.{graphql,gql}` | `graphql.md` — GraphQL 스키마와 오퍼레이션. |
| `**/*.prisma` | `prisma.md` — Prisma 스키마. |
| `**/*.jl` | `julia.md` — Julia 소스. |
| `**/*.{tf,hcl,tfvars}` | `terraform.md` — Terraform / HCL. |
| `**/*.bicep` | `bicep.md` — Bicep(Azure) 템플릿. |
| `**/*.elm` | `elm.md` — Elm 소스. |
| `**/*.{jsonnet,libsonnet}` | `jsonnet.md` — Jsonnet 설정 템플릿과 라이브러리. |
| `**/*.thrift` | `thrift.md` — Apache Thrift IDL 통신 호환성. |
| `**/*.capnp` | `capnp.md` — Cap'n Proto 스키마 통신 호환성. |
| `**/*.m` | `matlab.md`(또는 [내용 탐지](#content-sniffing-for-m-files)로 `objc.md`) |
| `**/*.sol` | `solidity.md` — Solidity 스마트 컨트랙트. |
| `**/*.vy` | `vyper.md` — Vyper 스마트 컨트랙트. |
| *(대체값)* | `default.md` |

해석된 규칙 본문은 plan과 main 작업 프롬프트에서 `{{system_rule}}` 자리에 들어갑니다.

### `.m` 파일의 내용 탐지 {#content-sniffing-for-m-files}

`.m`은 MATLAB과 Objective-C가 함께 씁니다. OCR은 파일의 첫 비어 있지 않은 줄을 살펴
둘을 구분합니다. Objective-C처럼 보이면(예: `#import`, `@implementation`, C 형식
주석) `matlab.md` 대신 `objc.md`를 씁니다. 내용을 읽을 수 없으면 `matlab.md`로
넘어갑니다.

> **안정성 참고.** 이 탐지 방식은 OCR 버전에 따라 바뀔 수 있습니다. `.m` 경로를
> 확실하게 정해 두고 싶다면 프로젝트 수준 규칙을 명시하세요. 프로젝트 규칙은 언제나
> 시스템 계층보다 우선합니다.

## 어떤 규칙이 이겼는지 확인하기: `ocr rules check` {#inspecting-which-rule-wins-ocr-rules-check}

```bash
$ ocr rules check src/main/java/com/example/UserService.java
File: src/main/java/com/example/UserService.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
…contents of java.md…
────────────────────────────────────────
```

```bash
$ ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml
File: src/main/resources/mapper/UserMapper.xml
Source: Custom (--rule)
Pattern: **/*mapper*.xml
Rule:
────────────────────────────────────────
…contents of your custom rule…
────────────────────────────────────────
```

규칙이 예상과 다르게 동작할 때마다 쓰세요. 어느 **계층**의 어떤 **패턴**이 이겼는지
알려 줍니다.

## 레시피 {#recipes}

### 프로젝트 수준: 코딩 표준 강제하기 {#project-level-enforce-a-coding-standard}

`<repo>/.opencodereview/rule.json`으로 저장하고 커밋합니다:

```json
{
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "Every public handler must `defer tx.Rollback()` immediately after starting a transaction."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, missing parameter binding, and unclosed XML tags."
    }
  ]
}
```

### 프로젝트 수준: 생성된 코드는 건너뛰고 src에 집중하기 {#project-level-skip-generated-code-focus-on-src}

```json
{
  "include": ["src/**/*.{ts,tsx,js,jsx}"],
  "exclude": ["**/*.gen.ts", "**/generated/**"]
}
```

`include`를 지정하면 `src/` 안의 파일은 내장 기본 제외 패턴에 걸릴 파일(예: 테스트
파일)이라도 남습니다. `src/` 밖의 파일은 평소대로 확장자와 기본 검사를 거칩니다.
`include`는 건너뛰기이지 화이트리스트가 아닙니다.

### PR 단위 재정의 {#per-pr-override}

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

프로젝트 계층과 전역 계층을 모두 건너뜁니다. PR 하나에만 전혀 다른 리뷰
체크리스트(예: 보안만 보는 리뷰)가 필요할 때 쓸 만합니다.

### 개인 전역 설정 {#global-personal-preferences}

`~/.opencodereview/rule.json`에 두면 그 머신의 모든 저장소가 물려받습니다:

```json
{
  "rules": [
    {
      "path": "**/*.{ts,tsx,js,jsx}",
      "rule": "Always check for unhandled promise rejections; warn on `// eslint-disable` without a reason comment."
    }
  ]
}
```

## 관련 문서 {#see-also}

- [CLI 레퍼런스](../cli-reference/) — `ocr review --rule`, `--preview`,
  `ocr rules check`입니다.
- [설정](../configuration/) — 설정 파일 위치와 계층 해석 사슬입니다.
- [아키텍처](../architecture/) — 해석된 규칙이 Agent 프롬프트로 들어가는 과정입니다.
