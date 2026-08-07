---
title: Справочник CLI
sidebar:
  order: 6
---

Полный справочник по всем подкомандам и флагам `ocr`, а также поведению при
завершении.

## Общее использование

```text
OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  session, sessions  List and inspect saved review sessions
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr config provider                      Interactive provider setup
  ocr config model                         Interactive model selection
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr llm providers                        List built-in providers
  ocr session list                         List saved review sessions
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.
Use "ocr session -h" for more information about session inspection.

GitHub: https://github.com/alibaba/open-code-review
```

## Краткий обзор команд

| Команда | Псевдоним | Назначение |
|---|---|---|
| `ocr review` | `ocr r` | Запускает код-ревью и выводит комментарии. |
| `ocr rules check <file>` | — | Показывает, какое правило применяется к указанному пути файла и откуда оно получено. |
| `ocr config set <key> <value>` | — | Сохраняет значение конфигурации в `~/.opencodereview/config.json`. |
| `ocr config unset custom_providers.<name>` | — | Удаляет пользовательского провайдера (если он был активен, сбрасывает активные `provider`/`model`). |
| `ocr config provider` | — | Интерактивный TUI для настройки провайдера. |
| `ocr config model` | — | Интерактивный TUI для выбора модели. |
| `ocr llm test` | — | Отправляет небольшой запрос в чат для проверки настроенного эндпоинта. |
| `ocr llm providers` | — | Выводит список всех встроенных LLM-провайдеров. |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | Выводит список сохранённых сессий ревью. |
| `ocr session show <id>` | `ocr sessions show <id>` | Показывает одну сессию и контрольные точки по каждому файлу. |
| `ocr session comments <id>` | `ocr sessions comments <id>` | Выводит комментарии ревью, записанные в одной сессии. |
| `ocr viewer` | — | Запускает локальный веб-интерфейс для просмотра прошлых сессий ревью (`localhost:5483`). |
| `ocr version` | — | Выводит версию, коммит, платформу, дату сборки и URL GitHub. |

Команды `ocr` и `ocr -h` выводят справку верхнего уровня. Каждая подкоманда
также принимает `-h` / `--help`.

## `ocr review`

Основная команда. Определяет Git diff, распределяет файлы между субагентами,
собирает комментарии ревью и выводит их.

### Синтаксис

```text
ocr review [flags]
ocr r      [flags]   (alias)
```

Если флаги не переданы, OCR работает в **режиме рабочей области** — проверяет
все индексированные, неиндексированные и неотслеживаемые изменения в
репозитории текущего каталога.

### Флаги

| Флаг | Краткая форма | По умолчанию | Описание |
|---|---|---|---|
| `--repo <path>` | — | текущий каталог | Корень Git-репозитория. |
| `--from <ref>` | — | — | Исходная ссылка, от которой начинается diff (например, `main`). |
| `--to <ref>` | — | — | Целевая ссылка, на которой заканчивается diff (например, `feature-branch`). Если задана, OCR вычисляет `merge-base(from, to)..to`. |
| `--commit <sha>` | `-c` | — | Один коммит для ревью (сравнивается с родительским). |
| `--preview` | `-p` | `false` | Запускает конвейер фильтрации, но пропускает LLM. Выводит список файлов и причины исключения. |
| `--resume <session-id>` | — | — | Возобновляет предыдущую совместимую сессию ревью диапазона или коммита. |
| `--format <fmt>` | `-f` | `text` | `text` (для чтения человеком) или `json` (машиночитаемый массив комментариев). |
| `--audience <who>` | — | `human` | `human` выводит ход выполнения; `agent` отключает этот вывод в stdout и печатает только итоговую сводку / JSON. |
| `--background <text>` | `-b` | — | Необязательные требования / бизнес-контекст, добавляемые в промпты планирования и основной задачи. |
| `--concurrency <n>` | — | `8` | Максимальное число файлов, проверяемых параллельно. |
| `--timeout <minutes>` | — | `10` | Срок выполнения для каждого файла. `0` отключает тайм-аут. |
| `--rule <path>` | — | — | Путь к пользовательскому JSON-файлу правил ревью. Переопределяет проектный и глобальный `rule.json`. |
| `--max-tools <n>` | — | значение шаблона | Максимальное число раундов вызова инструментов для каждого файла. `0` использует значение шаблона (`30`); значения 1–9 повышаются до `10`; любое значение `≥ 10` переопределяет значение шаблона (даже если оно меньше `30`). |
| `--max-tokens <n>` | — | значение конфигурации или шаблона | Предел токенов запроса для каждого файла. Переопределяет сохранённое значение `max_tokens` для этого запуска. |
| `--model <name>` | — | — | Переопределяет выбранную LLM-модель для этого ревью (например, `claude-opus-4-6`). |
| `--max-git-procs <n>` | — | `16` | Максимальное число одновременно выполняемых подпроцессов Git. |
| `--tools <path>` | — | встроенные | Путь к пользовательскому JSON-файлу конфигурации инструментов. Переопределяет встроенные определения инструментов. |

> Флаги режимов взаимоисключающие: передайте либо `--from`/`--to`, либо
> `--commit`, либо не передавайте ни одного из них (режим рабочей области).
> Их сочетание приводит к ошибке. `--resume` поддерживает только ревью
> диапазона или коммита и несовместим с `--preview`.

### Режимы

#### Режим рабочей области (по умолчанию)

```bash
ocr review
```

OCR формирует изменения рабочего дерева с помощью двух команд Git:

- отслеживаемые изменения — через `git diff HEAD` (индексированные и
  неиндексированные изменения вместе относительно `HEAD`; если результат
  пуст, OCR использует `git diff --staged`);
- неотслеживаемые файлы — через `git ls-files --others --exclude-standard`:
  они читаются с диска и считаются целиком добавленными файлами.

Обычно именно это требуется перед коммитом. Если нужна более узкая область,
добавляйте файлы в индекс выборочно.

#### Режим диапазона

```bash
ocr review --from main --to feature-branch
```

OCR вычисляет `merge-base(main, feature-branch)..feature-branch`, поэтому вы
видите только diff, *внесённый* веткой функций, без несвязанных изменений,
которые попали в `main` после её создания.

#### Режим коммита

```bash
ocr review --commit abc123
ocr review -c abc123
```

Проверяет diff, полученный командой `git show abc123` (то есть изменения,
внесённые этим отдельным коммитом).

### Возобновление прерванных ревью

Каждый запуск `ocr review` сохраняет локальный журнал сессии в каталоге
`~/.opencodereview/sessions/`. Успешный текстовый вывод сосредоточен на
результатах ревью и не показывает идентификатор сессии; чтобы найти
сохранённые сессии, используйте `ocr session list/show`, а чтобы получить
`session_id` в машиночитаемом выводе — `--format json`. Если ревью диапазона
или коммита было прервано, выведите список сохранённых сессий и возобновите
ту, которая соответствует той же цели ревью:

```bash
ocr session list
ocr session show <session-id>
ocr session comments <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

Возобновление намеренно выполняется строго:

- ревью рабочей области нельзя возобновить;
- для ревью диапазона нужно использовать те же `--from` и `--to`;
- для ревью коммита нужно использовать тот же `--commit`;
- `--preview` и `--resume` нельзя использовать вместе.

### Вывод

#### Текст (по умолчанию, `--audience human`)

Во время ревью выводятся строки хода выполнения, после чего для каждого
комментария печатается отдельный блок (приглушённый заголовок из символов
Unicode с `path:start-end`, текст комментария с переносом на 100 столбцах и,
если есть, цветной встроенный diff предлагаемой замены). В конце в stdout
выводится сводка запуска:

```
[ocr] 17 file(s) changed, reviewing 9 in /path/to/repo
[ocr] Skipping image.png — filtered by path/extension rules
[ocr]   ▶ file_read "src/foo.go"
[ocr]   ✔ file_read (12ms)
[ocr] Plan completed for src/foo.go
…

─── src/foo.go:42-47 ───
Concurrent map access without a lock — wrap with sync.RWMutex.

- m[k] = v
+ mu.Lock(); defer mu.Unlock(); m[k] = v

…
[ocr] Summary: 9 file(s) reviewed, 14 comment(s), ~21344 token(s) used (input: ~18012, output: ~3332), 1m12s elapsed
```

#### Текст (агент, `--audience agent`)

Вывод комментариев остаётся тем же, но строки хода выполнения подавляются
внутренним переключаемым в тихий режим средством записи stdout
([`internal/stdout`](https://github.com/alibaba/open-code-review/blob/main/internal/stdout/stdout.go)).
Используйте этот режим в CI или при передаче вывода другому агенту.

#### JSON

```bash
ocr review --format json --audience agent
```

```json
{
  "status": "success",
  "summary": {
    "files_reviewed": 9,
    "comments": 1,
    "total_tokens": 21344,
    "input_tokens": 18012,
    "output_tokens": 3332,
    "elapsed": "1m12s"
  },
  "comments": [
    {
      "path": "src/foo.go",
      "content": "Concurrent map access without a lock — wrap with sync.RWMutex.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "m[k] = v",
      "suggestion_code": "mu.Lock(); defer mu.Unlock(); m[k] = v",
      "thinking": "Looking at line 42, the map …"
    }
  ]
}
```

Поля верхнего уровня:

| Поле | Примечания |
|---|---|
| `status` | `success`, `completed_with_warnings`, `completed_with_errors` или `skipped`. |
| `message` | Необязательно. Сводка для чтения человеком, например, `"No comments generated. Looks good to me."`. |
| `summary` | Необязательно. Сводные показатели запуска: `files_reviewed`, `comments`, `total_tokens`, `input_tokens`, `output_tokens`, `cache_read_tokens` (omitempty), `cache_write_tokens` (omitempty), `elapsed`. Отсутствует у запусков со статусом `skipped`. |
| `comments` | Присутствует всегда, но может быть пустым. Поля комментария показаны в примере выше. |
| `warnings` | Необязательно. Присутствует, если один или несколько субагентов завершились с ошибкой; каждая запись описывает затронутый файл и ошибку. |
| `session_id` | Необязательно. Присутствует у сохранённых запусков ревью; передайте его в `ocr review --resume <session-id>`, чтобы повторить совместимое ревью диапазона или коммита. |
| `resume` | Необязательно. Присутствует у возобновлённых запусков и содержит `resumed_from`, `reused_files`, `rerun_files`, `previous_model` и `current_model`. |

Если для ревью не подошёл ни один файл, режим JSON вместо этого выводит
объект со статусом `skipped`, чтобы вызывающая сторона могла отличить
«изменений нет» от «замечаний нет»:

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "comments": []
}
```

### Коды завершения

| Код | Значение |
|---|---|
| `0` | Ревью завершено (возможно, без комментариев или с некритическими предупреждениями). |
| `1` | Критическая ошибка — неверные флаги, не удалось определить эндпоинт LLM, ошибкой завершились все субагенты по файлам и т. п. Текст ошибки выводится в stderr. |

Некритические предупреждения (ошибка отдельного субагента, превышение
порогового числа токенов файлом и т. п.) выводятся в процессе работы; в режиме
JSON они добавляются в массив `warnings`.

## `ocr session`

Выводит список и содержимое локальных журналов сессий ревью, сохранённых в
`~/.opencodereview/sessions/`. Используйте эту команду, чтобы найти
идентификатор сессии, проверить состояние контрольных точек отдельных файлов
и возобновить прерванное ревью диапазона или коммита.

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls        List recent review sessions for the current repo
  show <id>       Show one session's metadata and per-file items
  comments <id>   Show the review comments recorded in one session
```

### `ocr session list`

```bash
ocr session list
ocr session list --limit 50
ocr session list --json
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--repo <path>` | текущий каталог | Репозиторий, сессии которого нужно вывести. |
| `--json` | `false` | Вывести сводки сессий в формате JSON. |
| `--limit <n>` | `20` | Ограничить число выведенных сессий. Используйте `0`, чтобы снять ограничение. |

### `ocr session show`

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--repo <path>` | текущий каталог | Репозиторий, сессию которого нужно проверить. |
| `--json` | `false` | Вывести метаданные сессии и элементы по файлам в формате JSON. |

### `ocr session comments`

Выводит все сохранённые в сессии комментарии ревью в том же стиле, что и
терминальный вывод `ocr review` (путь, диапазон строк, значок серьёзности,
diff предложения).

```bash
ocr session comments <session-id>
ocr session comments --json <session-id>
ocr session comments --severity high <session-id>
ocr session comments --severity critical,high --category bug,security <session-id>
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--repo <path>` | текущий каталог | Репозиторий, сессию которого нужно проверить. |
| `--json` | `false` | Вывести комментарии как массив JSON. |
| `--severity <list>` | все | Включить перечисленные через запятую уровни серьёзности (`critical`, `high`, `medium`, `low`). |
| `--category <list>` | все | Включить перечисленные через запятую категории (например, `bug`, `security`). |

## `ocr rules`

Проверка правил. Доступна ровно одна подкоманда:

```text
ocr rules check [flags] <file-path>

Flags:
  --repo <path>    Git repository root (default: current dir)
  --rule <path>    Path to a custom rule JSON file
```

Для указанного пути файла OCR:

1. Проходит четырёхуровневую цепочку правил (пользовательское → проектное → глобальное → системное).
2. Выбирает первое совпадение.
3. Выводит **уровень источника**, совпавший **glob-шаблон** и итоговый **текст правила**.

```bash
$ ocr rules check src/main/java/com/example/Foo.java
File: src/main/java/com/example/Foo.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
<contents of internal/config/rules/rule_docs/java.md>
────────────────────────────────────────
```

Команда помогает разобраться, «почему моё пользовательское правило не
срабатывает». Полная схема приоритетов описана в разделе
[Правила ревью](../review-rules/).

## `ocr config`

Сохраняет ключи в `~/.opencodereview/config.json` и предоставляет
интерактивные TUI для настройки. Доступны четыре подкоманды:

```text
ocr config set <key> <value>
ocr config unset custom_providers.<name>   Delete a custom provider
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`** — записывает одно значение конфигурации неинтерактивно.
- **`unset`** — удаляет пользовательского провайдера. Поддерживается только
  `custom_providers.<name>`. Если удалённый провайдер был активным, значения
  `provider` и `model` сбрасываются (выполните `ocr config provider`, чтобы
  выбрать новый).
- **`provider`** — запускает интерактивный TUI настройки провайдера (без
  дополнительных аргументов; для неинтерактивной настройки используйте
  `ocr config set provider <name>`).
- **`model`** — запускает интерактивный TUI выбора модели (без дополнительных
  аргументов; для неинтерактивной настройки используйте
  `ocr config set model <name>`).

Полный справочник по ключам, схемы и примеры приведены в разделе
[Конфигурация](../configuration/).

## `ocr llm`

Служебные команды LLM. Доступны две подкоманды:

```text
ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers
```

### `ocr llm test`

```text
ocr llm test
```

Определяет эндпоинт LLM точно так же, как `ocr review`, отправляет один
подготовленный запрос чата из файла
[`internal/config/testconnection/task.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/testconnection/task.json)
и выводит:

```
Source: <which strategy was used>
URL:    <endpoint URL>
Model:  <effective model>
<the model's reply>
✓ Connection test successful
```

Ненулевой код завершения означает, что эндпоинт настроен не полностью либо
запрос завершился с ошибкой (сеть / аутентификация / модель). В сообщении об
ошибке указана причина.

### `ocr llm providers`

```text
ocr llm providers
```

Выводит всех встроенных LLM-провайдеров в таблице из трёх столбцов:

```
Built-in providers:
  NAME        PROTOCOL    BASE URL
  ----        --------    --------
  anthropic   anthropic   https://api.anthropic.com
  …
```

Затем выводится подсказка: настроить провайдера интерактивно можно командой
`ocr config provider`, а неинтерактивно — командой
`ocr config set provider <name>`.

## `ocr viewer`

```text
ocr viewer [flags]

Flags:
  --addr <address>   listen address (default: localhost:5483)

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
```

Запускает встроенный HTTP-сервер, который читает данные из
`~/.opencodereview/sessions/...` и отображает прошлые сессии ревью в удобном
для браузера интерфейсе. См. раздел [Просмотр сессий](../viewer/).

## `ocr version`

```text
ocr version
ocr --version
ocr -V
```

Выводит версию, записанную при сборке, короткий Git-коммит (если есть),
платформу (`<GOOS>/<GOARCH>`), дату сборки (если есть) и URL GitHub
(`https://github.com/alibaba/open-code-review`).

## Советы и подводные камни

- `--audience agent` **не** подразумевает `--format json`. Они управляют
  разными аспектами: тихим интерфейсом и структурированными данными. Если
  нужны оба, используйте их вместе.
- `--background` — один из наиболее значимых для качества ревью флагов.
  Всегда передавайте требования / описание PR при вызове из другого агента.
- Файл, один diff которого превышает 80 % от `MAX_TOKENS` (по умолчанию
  `58888`), отбрасывается до вызова LLM. Это записывается в журнал, но не
  приводит к сбою запуска.
- Этап планирования **автоматически пропускается**, если число изменённых строк
  в файле меньше `PLAN_MODE_LINE_THRESHOLD` (`50`).

## См. также

- [Быстрый старт](../quickstart/) — установка и запуск первого ревью.
- [Конфигурация](../configuration/) — переменные окружения и ключи конфигурации, связанные с флагами.
- [Правила ревью](../review-rules/) — флаг `--rule` и разрешение правил.
- [Интеграции](../integrations/agent-skill/) — вызов `ocr review` из агентов и CI.
