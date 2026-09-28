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

| Приоритет     | Источник           | Путь                                  | Примечания                                                |
| ------------- | ------------------ | ------------------------------------- | --------------------------------------------------------- |
| 1 (наивысший) | флаг `--rule`      | пользовательский                      | Переопределение через CLI; всегда побеждает, если задано. |
| 2             | Конфиг проекта     | `<repoDir>/.opencodereview/rule.json` | Правила уровня проекта — безопасно коммитить.             |
| 3             | Глобальный конфиг  | `~/.opencodereview/rule.json`         | Пользовательские предпочтения.                            |
| 4 (низший)    | Системный стандарт | встроенный `system_rules.json`        | Встроенные правила для распространённых языков.           |

Если файл более приоритетного уровня не существует, он тихо
пропускается — это не ошибка. Поэтому проект, в котором никогда не добавляли
`.opencodereview/rule.json`, просто проваливается на глобальный / системный
уровни.

Системный уровень **всегда** присутствует (он вшит в бинарник), поэтому всегда
разрешается _какое-то_ правило.

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

- `include` — необязательно. Glob-шаблоны, которые _обходят_ встроенные
  стандартные шаблоны исключения (исключения тестовых файлов — см. ниже). Это
  не белый список: файлы, не совпавшие ни с одним шаблоном `include`, всё равно
  проходят проверки `unsupported_ext` и `default_path` и могут быть
  отревьюены.
- `exclude` — необязательно. Glob-шаблоны для файлов, которые OCR _не должен_
  ревьюить. Наивысший приоритет среди пользовательских правил фильтрации.
- `rules` — массив записей `{path, rule}`, вычисляемых **в порядке объявления**.
  Первый `path`, чей glob совпадает с файлом, определяет промпт, который OCR
  отправляет модели для этого файла.

Каждая запись в `rules` принимает ещё одно необязательное поле:

- `merge_system_rule` — необязательно, по умолчанию `false`. При `false`
  (по умолчанию) совпавшая запись **заменяет** встроенное системное правило
  для этого файла. При `true` совпавшее системное правило сохраняется, а
  пользовательское правило **объединяется** с ним.

```json
{
  "rules": [
    {
      "path": "**/*",
      "rule": "Security review: flag hardcoded secrets, unvalidated redirects, and missing authz checks.",
      "merge_system_rule": true
    }
  ]
}
```

С этой записью `ocr rules check src/main/java/com/example/UserService.java`
сообщает обе половины:

```
$ ocr rules check src/main/java/com/example/UserService.java
Source: Project (.opencodereview/rule.json)
Pattern: **/*
Rule:
────────────────────────────────────────
## System-Specific Rules (Mandatory)

…contents of java.md…

---

## User-Specific Rules (Mandatory)

Security review: flag hardcoded secrets, unvalidated redirects, and missing authz checks.
────────────────────────────────────────
```

О том, как собирается это объединение, стоит знать две вещи:

- **Системная половина разрешается для каждого файла**, а не один раз за
  запуск. Запись выше — catch-all, однако файл `.java` получает `java.md`,
  файл `.py` получает `python.md`, а расширение, которое OCR не узнаёт,
  откатывается к `default.md`. Таким образом одна запись добавляет ваше
  правило поверх правильных языковых правил везде, без необходимости
  перечислять расширения.
- Любая из половин может быть пустой. Если для файла системный слой
  разрешается в пустоту, вы получаете только своё правило; если текст
  вашего правила пуст, вы получаете только системное правило. Ни в одном
  из случаев вторая половина не заменяется пустотой.

Поле читается из всех трёх пользовательских слоёв — `--rule`, проектного
`.opencodereview/rule.json` и `~/.opencodereview/rule.json`. Приоритет
слоёв не меняется: первая совпавшая запись по-прежнему выигрывает, а
совпавший слой по-прежнему затеняет слои ниже.

Ограничение, о котором стоит помнить: объединение достигает **только
системного** слоя. Если два ваших собственных правила совпадают с одним
файлом, первое по-прежнему выигрывает целиком — `merge_system_rule` не
объединяет их друг с другом.

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
<path>` для проверки.

## Как фильтруются файлы

Фильтр — шестишаговый алгоритм в
[`internal/agent/selection.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/selection.go).
Для каждого diff OCR спрашивает:

1. **`binary`** — Файл бинарный? Исключается.
2. **`secret_exclude`** — Старый или новый путь подпадает под встроенную защиту секретных путей? Если да, путь исключается. Эта защита применяется до пользовательских правил и не может быть переопределена шаблоном `include`; список шаблонов — в разделе [Встроенные секретные пути](#built-in-secret-paths).

3. **`user_exclude`** — Путь совпадает с каким-либо пользовательским шаблоном
   `exclude`? Исключается.
4. **`user_include`** — Если пользователь задал `include`, путь совпадает? Если
   да, **сразу остаётся** (обходит проверки `unsupported_ext` и `default_path`
   ниже).
5. **`unsupported_ext`** — Расширение файла есть в
   [списке разрешённых](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)?
   Исключается, если нет.
6. **`default_path`** — Путь совпадает со встроенным шаблоном исключения
   тестовых файлов (`**/*_test.go`, `**/*.test.{js,jsx,ts,tsx}`,
   `**/*_spec.rb`, …)? Исключается.

Файлы, прошедшие все шесть проверок, отправляются в LLM, если только сам diff
не превышает 80% от `max_tokens`: `selectFiles` применяет этот предел после
проверок и исключает файл как `too_large`. Он же помечает файл, чей новый
путь — `/dev/null`, причиной `deleted`; нового содержимого для ревью нет.
Используйте `ocr review --preview`, чтобы вывести результат этого фильтра, не
тратя ни одного токена.

### Встроенные секретные пути {#built-in-secret-paths}

Встроенные секретные пути не попадают в ревью (см.
[`internal/config/allowlist/default_secret_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_secret_patterns.json)):

- `**/.ssh/**`
- `**/id_rsa`
- `**/id_dsa`
- `**/id_ecdsa`
- `**/id_ed25519`
- `**/.netrc`
- `**/_netrc`
- `**/.npmrc`
- `**/.pypirc`
- `**/.dockercfg`

Кроме того, `.env` и любой вариант `.env.*` тоже считаются секретным путём; исключение — шаблоны `.env.example`, `.env.sample` и `.env.template`.

### Стандартные исключения путей

Встроенный список исключений (см.
[`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json))
охватывает две группы. Первая — тестовые файлы разных языков, а также
фикстуры, снапшоты и сгенерированный код:

- `**/*_test.go`
- `**/src/test/java/**/*.java`
- `**/src/test/**/*.{kt,kts}`
- `**/*Test.fs`
- `**/*.test.{js,jsx,ts,tsx}`
- `**/*.spec.{js,jsx,ts,tsx}`
- `**/__tests__/**`
- `**/test/**/*_test.py`
- `**/tests/**/*_test.py`
- `**/*_test.py`
- `**/test_*.py`
- `**/*_spec.rb`
- `**/spec/**/*_spec.rb`
- `**/*Test.java`
- `**/*Tests.java`
- `**/*_test.rs`
- `**/oh_modules/**`
- `**/*.test.ets`
- `**/test/**/*.jl`
- `**/test/**/*.hs`
- `**/*Spec.hs`
- `**/test/**/*.lhs`
- `**/*Spec.lhs`
- `**/tests/**/*.nim`
- `**/tests/**/*.R`
- `**/__snapshots__/**`
- `**/*.snap`
- `**/testdata/**`
- `**/fixtures/**`
- `**/.ipynb_checkpoints/**`
- `**/*.generated.*`
- `**/*.gen.go`
- `**/*.pb.go`
- `**/*.pb.cc`
- `**/*.pb.h`
- `**/*Test.swift`
- `**/*Tests.swift`
- `**/Tests/**/*.swift`
- `**/tests/**/*.elm`
- `**/vendor/**/*.{jsonnet,libsonnet}`
- `**/test/**/*.zig`
- `**/*_test.zig`
- `**/kitex_gen/**/*.go`
- `**/*.capnp.h`
- `**/*.capnp.go`
- `**/*.capnp.ts`
- `**/*_capnp.rs`
- `**/*_capnp.py`
- `**/test/**/*.ml`
- `**/tb_*.{v,sv,vhd,vhdl}`
- `**/*_tb.{v,sv,vhd,vhdl}`
- `lib/**/*.sol`
- `**/*.t.sol`
- `**/test/**/*.sol`
- `**/tests/**/*.sol`
- `**/test/**/*.vy`
- `**/tests/**/*.vy`

…и каталоги зависимостей и сборки:

- `**/node_modules/**`
- `**/bower_components/**`
- `**/vendor/**`
- `**/target/**`
- `**/dist/**`
- `**/__pycache__/**`, `**/.venv/**`, `**/site-packages/**`
- `**/Pods/**`, `**/Carthage/**`
- `**/.next/**`, `**/.nuxt/**`, `**/.gradle/**`, `**/.terraform/**`, …

`**/build/**` и `**/bin/**` намеренно отсутствуют: во многих проектах в них
лежат написанные вручную исходники.

Те же шумные каталоги фильтруются и раньше, на уровне diff в
[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go).
Этот список сопоставляется по префиксу пути, поэтому ловит каталог только в
**корне репозитория**: `vendor/pkg/x.go` не доходит до файлового фильтра и
отмечается как `provider_directory`, а `api/vendor/pkg/x.go` доходит и
исключается как `default_path`. Вернуть правилом `include` можно только
второй.

Чтобы **отревьюить** файл, совпадающий с одним из этих шаблонов,
добавьте его в пользовательский список `include` — это переопределяет
этап default_path.

## Разрешение правила для файла

Когда фильтр решил, что файл _будет_ отревьюен, OCR выбирает текст правила,
которому должен следовать агент:

1. Опробовать уровень `--rule` (пользовательский) в порядке объявления.
2. Опробовать `<repo>/.opencodereview/rule.json` в порядке объявления.
3. Опробовать `~/.opencodereview/rule.json` в порядке объявления.
4. Откатиться к встроенному системному уровню правил.

Выбранные шаблоны встроенного `system_rules.json` показаны ниже в относительном
порядке сопоставления:

| Шаблон                              | Документ правила                                                                                 |
| ----------------------------------- | ------------------------------------------------------------------------------------------------ |
| `**/*.properties`                   | `properties.md` — i18n / файлы конфигурации.                                                     |
| `**/*{mapper,dao}*.xml`             | `mapper_dao_xml.md` — MyBatis-стиль mapper SQL.                                                  |
| `**/pom.xml`                        | `pom_xml.md` — зависимости Maven.                                                                |
| `**/build.gradle`                   | `build_gradle.md` — зависимости Gradle.                                                          |
| `**/package.json`                   | `package_json.md` — зависимости / скрипты NPM.                                                   |
| `**/Cargo.toml`                     | `cargo_toml.md` — манифест Rust.                                                                 |
| `**/composer.json`                  | `composer_json.md` — зависимости Composer, автозагрузка, скрипты, плагины и конфигурация пакета. |
| `**/*.{json,json5}`                 | `json.md` — обычный JSON (также совпадает `.json5`).                                             |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md` — YAML workflow GitHub Actions.                                            |
| `.github/**/*.{yaml,yml}`           | `github_config.md` — прочий конфигурационный YAML `.github`.                                     |
| `**/*.{yaml,yml}`                   | `yaml.md`                                                                                        |
| `**/*.java`                         | `java.md`                                                                                        |
| `**/*.go`                           | `go.md` — исходный код Go.                                                                       |
| `**/*.{ftl,ftlh,ftlx}`              | `freemarker.md` — шаблоны FreeMarker (SSTI / XSS / обработка null).                              |
| `**/*.{hbs,mustache}`               | `handlebars_mustache.md` — шаблоны Handlebars и Mustache.                                        |
| `**/*.ets`                          | `arkts.md` — ArkTS / HarmonyOS.                                                                  |
| `**/*.astro`                        | `astro.md` — компоненты и islands Astro.                                                         |
| `**/*.{ts,js,tsx,jsx,mjs,cjs}`      | `ts_js_tsx_jsx.md`                                                                               |
| `**/*.{kt,kts}`                     | `kotlin.md`                                                                                      |
| `**/*.{fs,fsi,fsx}`                 | `fsharp.md` — файлы реализации, сигнатур и скриптов F#.                                          |
| `**/*.rs`                           | `rust.md`                                                                                        |
| `**/*.R`                            | `r.md`                                                                                           |
| `**/*.{cpp,cc,cxx,hpp,hxx}`         | `cpp.md`                                                                                         |
| `**/*.c`                            | `c.md`                                                                                           |
| `**/*.{py,pyi,ipynb}`               | `python.md` — исходный код Python.                                                               |
| `**/*.{php,phtml}`                  | `php.md` — исходный код PHP и шаблоны PHP.                                                       |
| `**/*.proto`                        | `protobuf.md` — совместимость Protocol Buffers на уровне wire.                                   |
| `**/*.po`                           | `po.md` — исходные каталоги переводов gettext.                                                   |
| `**/*.pot`                          | `pot.md` — файлы шаблонов gettext.                                                               |
| `**/*.{graphql,gql}`                | `graphql.md` — схема и операции GraphQL.                                                         |
| `**/*.prisma`                       | `prisma.md` — схема Prisma.                                                                      |
| `**/*.jl`                           | `julia.md` — исходный код Julia.                                                                 |
| `**/*.{tf,hcl,tfvars}`              | `terraform.md` — Terraform / HCL.                                                                |
| `**/*.bicep`                        | `bicep.md` — шаблоны Bicep (Azure).                                                              |
| `**/*.elm`                          | `elm.md` - исходный код Elm.                                                                     |
| `**/*.{jsonnet,libsonnet}`          | `jsonnet.md` — шаблоны конфигурации и библиотеки Jsonnet.                                        |
| `**/*.thrift`                       | `thrift.md` — совместимость Apache Thrift IDL на уровне wire.                                    |
| `**/*.capnp`                        | `capnp.md` — совместимость схем Cap'n Proto на уровне wire.                                      |
| `**/*.{v,sv,vh}`                    | `verilog.md` — RTL на Verilog и SystemVerilog.                                                   |
| `**/*.{vhd,vhdl}`                   | `vhdl.md` — RTL на VHDL.                                                                         |
| `**/*.m`                            | `matlab.md` (или `objc.md` через [определение содержимого](#content-sniffing-for-m-files))       |
| `**/*.mm`                           | `objc.md` — исходный код Objective-C++.                                                          |
| `**/*.sol`                          | `solidity.md` — смарт-контракты Solidity.                                                        |
| `**/*.vy`                           | `vyper.md` — смарт-контракты Vyper.                                                              |
| `**/*.rego`                         | `rego.md` — политики Rego (OPA).                                                                 |
| _(fallback)_                        | `default.md`                                                                                     |

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

### Глобальные правила безопасности поверх встроенных языковых правил

Широкое пользовательское правило по умолчанию **заменяет** встроенные
правила для языка, поэтому одна запись для `**/*` незаметно уберёт
`java.md`, `python.md` и остальные повсюду. Установите
`merge_system_rule`, чтобы сохранить и то, и другое:

```json
{
  "rules": [
    {
      "path": "**/*",
      "rule": "Security review: flag hardcoded secrets, unvalidated redirects, and missing authz checks.",
      "merge_system_rule": true
    }
  ]
}
```

Сохраните в `~/.opencodereview/rule.json`, чтобы применить ко всем
репозиториям, или в `<repo>/.opencodereview/rule.json` — к одному.
Поскольку системная половина разрешается для каждого файла, каждый язык
по-прежнему получает свои встроенные правила вдобавок к вашим —
перечислять расширения не нужно.

Глобальный файл — **самый нижний** из трёх пользовательских слоёв. Если в
`--rule` или в проектном `.opencodereview/rule.json` есть запись, которая
совпадает с тем же файлом, она побеждает, а глобальная запись не читается
вовсе — поэтому размещайте обобщённую запись только в одном месте.

## Смотрите также

- [Справочник CLI](../cli-reference/) — `ocr review --rule`, `--preview` и `ocr rules check`.
- [Конфигурация](../configuration/) — расположение файлов конфигурации и многоуровневая цепочка разрешения.
- [Архитектура](../architecture/) — как разрешённое правило попадает в промпт агента.
