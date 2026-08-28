---
title: Правила ревью
sidebar:
  order: 7
---

Правила сообщают OCR, **на чём сосредоточиться** при ревью каждого файла. Они
хранятся в JSON-файлах на трёх уровнях, плюс встроенный системный стандарт,
поставляемый в бинарнике.

## Цепочка приоритетов

OCR разрешает правила через **четырёхуровневую цепочку приоритетов**. Для
каждого пути файла уровни опробуются по порядку; побеждает первый совпавший
шаблон.

| Приоритет | Источник | Путь | Примечания |
|---|---|---|---|
| 1 (наивысший) | флаг `--rule` | пользовательский | Переопределение через CLI; всегда побеждает, если задано. |
| 2 | Конфиг проекта | `<repoDir>/.opencodereview/rule.json` | Правила уровня проекта — безопасно коммитить. |
| 3 | Глобальный конфиг | `~/.opencodereview/rule.json` | Пользовательские предпочтения. |
| 4 (низший) | Системный стандарт | встроенный `system_rules.json` | Встроенные правила для распространённых языков. |

Если файл более приоритетного уровня не существует, он тихо
пропускается — это не ошибка. Поэтому проект, в котором никогда не добавляли
`.opencodereview/rule.json`, просто проваливается на глобальный / системный
уровни.

Системный уровень **всегда** присутствует (он вшит в бинарник), поэтому всегда
разрешается *какое-то* правило.

## Формат файла правил (уровни 1–3) {#rule-file-format-layers-1-3}

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

Три независимых поля:

- `include` — необязательно. Glob-шаблоны, которые *обходят* встроенные
  стандартные шаблоны исключения (исключения тестовых файлов — см. ниже). Это
  не белый список: файлы, не совпавшие ни с одним шаблоном `include`, всё равно
  проходят проверки `unsupported_ext` и `default_path` и могут быть
  отревьюены.
- `exclude` — необязательно. Glob-шаблоны для файлов, которые OCR *не должен*
  ревьюить. Наивысший приоритет внутри фильтра.
- `rules` — массив записей `{path, rule}`, вычисляемых **в порядке объявления**.
  Первый `path`, чей glob совпадает с файлом, определяет промпт, который OCR
  отправляет модели для этого файла.

### Возможности glob

OCR использует [`bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4)
для сопоставления:

- `*` — любые символы, кроме `/`.
- `**` — через границы каталогов (`src/**/*.go` покрывает любую глубину).
- `{a,b,c}` — раскрытие скобок. `*.{ts,tsx,js,jsx}` раскрывается в четыре
  шаблона, сопоставляемых по очереди.
- `?` — один символ.
- `[abc]` — класс символов.

> Шаблоны сопоставляются **без учёта регистра** (путь файла приводится к нижнему
> регистру перед сопоставлением). Если сомневаетесь, используйте `ocr rules check
> <path>` для проверки.

## Как фильтруются файлы

Фильтр — пятношаговый алгоритм в
[`internal/agent/preview.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/preview.go).
Для каждого diff OCR спрашивает:

1. **`binary`** — Файл бинарный? Исключается.
2. **`user_exclude`** — Путь совпадает с каким-либо пользовательским шаблоном
   `exclude`? Исключается.
3. **`user_include`** — Если пользователь задал `include`, путь совпадает? Если
   да, **сразу остаётся** (обходит проверки `unsupported_ext` и `default_path`
   ниже).
4. **`unsupported_ext`** — Расширение файла есть в
   [списке разрешённых](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)?
   Исключается, если нет.
5. **`default_path`** — Путь совпадает со встроенным шаблоном исключения
   тестовых файлов (`**/*_test.go`, `**/*.test.{js,jsx,ts,tsx}`,
   `**/*_spec.rb`, …)? Исключается.

Файлы, прошедшие все пять проверок, отправляются в LLM. Причина `deleted`
(не этап — она вычисляется отдельно в `Preview()`) помечает файлы, чей новый
путь — `/dev/null`; нового содержимого для ревью нет. Используйте `ocr review
--preview`, чтобы вывести результат этого фильтра, не тратя ни одного токена.

### Стандартные исключения путей

Встроенный список исключений (см.
[`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json))
совпадает с шаблонами тестовых файлов:

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

Фильтрация шумных каталогов (`vendor/`, `node_modules/`, `target/`, …)
происходит раньше, на уровне diff в
[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go),
до запуска попереходного файлового фильтра.

Чтобы **отревьюить** файл, совпадающий с одним из этих шаблонов тестовых
файлов, добавьте его в пользовательский список `include` — это переопределяет
этап default_path.

## Разрешение правила для файла

Когда фильтр решил, что файл *будет* отревьюен, OCR выбирает текст правила,
которому должен следовать агент:

1. Опробовать уровень `--rule` (пользовательский) в порядке объявления.
2. Опробовать `<repo>/.opencodereview/rule.json` в порядке объявления.
3. Опробовать `~/.opencodereview/rule.json` в порядке объявления.
4. Откатиться к встроенному системному уровню правил.

Выбранные шаблоны встроенного `system_rules.json` показаны ниже в относительном
порядке сопоставления:

| Шаблон | Документ правила |
|---|---|
| `**/*.properties` | `properties.md` — i18n / файлы конфигурации. |
| `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md` — MyBatis-стиль mapper SQL. |
| `**/pom.xml` | `pom_xml.md` — зависимости Maven. |
| `**/build.gradle` | `build_gradle.md` — зависимости Gradle. |
| `**/package.json` | `package_json.md` — зависимости / скрипты NPM. |
| `**/Cargo.toml` | `cargo_toml.md` — манифест Rust. |
| `**/composer.json` | `composer_json.md` — зависимости Composer, автозагрузка, скрипты, плагины и конфигурация пакета. |
| `**/*.{json,json5}` | `json.md` — обычный JSON (также совпадает `.json5`). |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md` — YAML workflow GitHub Actions. |
| `.github/**/*.{yaml,yml}` | `github_config.md` — прочий конфигурационный YAML `.github`. |
| `**/*.{yaml,yml}` | `yaml.md` |
| `**/*.java` | `java.md` |
| `**/*.go` | `go.md` — исходный код Go. |
| `**/*.{ftl,ftlh,ftlx}` | `freemarker.md` — шаблоны FreeMarker (SSTI / XSS / обработка null). |
| `**/*.{hbs,mustache}` | `handlebars_mustache.md` — шаблоны Handlebars и Mustache. |
| `**/*.ets` | `arkts.md` — ArkTS / HarmonyOS. |
| `**/*.astro` | `astro.md` — компоненты и islands Astro. |
| `**/*.{ts,js,tsx,jsx}` | `ts_js_tsx_jsx.md` |
| `**/*.{kt}` | `kotlin.md` |
| `**/*.rs` | `rust.md` |
| `**/*.R` | `r.md` |
| `**/*.{cpp,cc,hpp}` | `cpp.md` |
| `**/*.c` | `c.md` |
| `**/*.{py,ipynb}` | `python.md` — исходный код Python. |
| `**/*.{php,phtml}` | `php.md` — исходный код PHP и шаблоны PHP. |
| `**/*.proto` | `protobuf.md` — совместимость Protocol Buffers на уровне wire. |
| `**/*.po` | `po.md` — исходные каталоги переводов gettext. |
| `**/*.pot` | `pot.md` — файлы шаблонов gettext. |
| `**/*.{graphql,gql}` | `graphql.md` — схема и операции GraphQL. |
| `**/*.prisma` | `prisma.md` — схема Prisma. |
| `**/*.jl` | `julia.md` — исходный код Julia. |
| `**/*.{tf,hcl,tfvars}` | `terraform.md` — Terraform / HCL. |
| `**/*.bicep` | `bicep.md` — шаблоны Bicep (Azure). |
| `**/*.elm` | `elm.md` - исходный код Elm. |
| `**/*.{jsonnet,libsonnet}` | `jsonnet.md` — шаблоны конфигурации и библиотеки Jsonnet. |
| `**/*.thrift` | `thrift.md` — совместимость Apache Thrift IDL на уровне wire. |
| `**/*.capnp` | `capnp.md` — совместимость схем Cap'n Proto на уровне wire. |
| `**/*.{mojo,🔥}` | `mojo.md` - исходный код Mojo. |
| `**/*.m` | `matlab.md` (или `objc.md` через [определение содержимого](#content-sniffing-for-m-files)) |
| `**/*.sol` | `solidity.md` — смарт-контракты Solidity. |
| `**/*.vy` | `vyper.md` — смарт-контракты Vyper. |
| *(fallback)* | `default.md` |

Разрешённое тело правила становится значением плейсхолдера `{{system_rule}}`
в промптах plan и main task.

### Определение содержимого для файлов `.m` {#content-sniffing-for-m-files}

Расширение `.m` используется и MATLAB, и Objective-C. OCR заглядывает в первую
непустую строку файла для различения: если она выглядит как Objective-C
(например, `#import`, `@implementation`, комментарий в стиле C), вместо
`matlab.md` используется `objc.md`. Если содержимое прочитать не удаётся,
разрешение откатывается к `matlab.md`.

> **Примечание о стабильности.** Эвристика определения может изменяться между
> версиями OCR. Если вам нужна детерминированная маршрутизация `.m`, задайте
> для `.m`-путей явное правило на уровне проекта — правила проекта всегда
> имеют приоритет над системным уровнем.

## Проверка, какое правило выиграло: `ocr rules check`

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

Используйте это всякий раз, когда правило ведёт себя не так, как ожидалось —
команда показывает **уровень** и **шаблон**, который победил.

## Рецепты

### Уровень проекта: внедрить стандарт кодирования

Сохраните как `<repo>/.opencodereview/rule.json` и закоммитьте:

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

### Уровень проекта: пропустить сгенерированный код, сфокусироваться на src

```json
{
  "include": ["src/**/*.{ts,tsx,js,jsx}"],
  "exclude": ["**/*.gen.ts", "**/generated/**"]
}
```

Когда задан `include`, файлы внутри `src/` остаются, даже если иначе они были
бы отброшены встроенным стандартным шаблоном исключения (например, тестовым
файлом). Файлы вне `src/` по-прежнему проходят обычные проверки ext /
default_path — `include` это обход, а не белый список.

### Переопределение для отдельного PR

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

Обходит и проектный, и глобальный уровни — удобно, когда один PR требует
совсем другого чек-листа ревью (например, только ревью безопасности).

### Глобальные личные предпочтения

Поместите их в `~/.opencodereview/rule.json`, чтобы каждый репозиторий на вашей
машине наследовал их:

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

## Смотрите также

- [Справочник CLI](../cli-reference/) — `ocr review --rule`, `--preview` и `ocr rules check`.
- [Конфигурация](../configuration/) — расположение файлов конфигурации и многоуровневая цепочка разрешения.
- [Архитектура](../architecture/) — как разрешённое правило попадает в промпт агента.
