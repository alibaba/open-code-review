---
title: Участие в разработке
sidebar:
  order: 13
---

OCR распространяется с открытым исходным кодом по лицензии Apache-2.0. Мы
приветствуем сообщения об ошибках, исправления документации и изменения кода.
Эта страница служит кратким справочником; каноническая версия находится в
файле
[`CONTRIBUTING.md`](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md).

## Как можно помочь проекту

Чтобы быть полезным, необязательно писать на Go:

- **Сообщения об ошибках** — создайте [задачу на GitHub](https://github.com/alibaba/open-code-review/issues/new/choose)
  с шагами воспроизведения.
- **Предложения функций** — начните обсуждение в разделе
  [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)
  или создайте задачу с предложением функции.
- **Документация** — исправления опечаток, недостающие примеры и неработающие
  ссылки; такие PR часто принимаются быстрее всего.
- **Ревью других PR** — комментарии участников, не являющихся мейнтейнерами,
  помогают снизить нагрузку на ревьюеров.
- **Код** — исправления ошибок, оптимизация производительности и новые функции.

## Настройка локальной среды разработки

### Предварительные требования

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Получение исходного кода

```bash
# Fork on GitHub, then:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # writes dist/opencodereview
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> Удалённый репозиторий `upstream` доступен только для чтения. Отправляйте
> изменения в `origin` (ваш форк) и создавайте PR оттуда.

### Запуск локальной сборки

```bash
./dist/opencodereview review --preview
```

Для удобства создайте в `~/bin/ocr-dev` символическую ссылку на
`dist/opencodereview`, чтобы вызывать `ocr-dev` из любого репозитория.

### Цели Make

| Цель | Назначение |
|---|---|
| `make build` | Собирает бинарный файл для текущей платформы → `dist/opencodereview`. |
| `make build-darwin-amd64` | Кросс-компиляция для macOS Intel. |
| `make build-darwin-arm64` | Кросс-компиляция для macOS Apple Silicon. |
| `make build-linux-amd64` | Кросс-компиляция для Linux x86_64. |
| `make build-linux-arm64` | Кросс-компиляция для Linux ARM64. |
| `make build-windows-amd64` | Кросс-компиляция для Windows x86_64. |
| `make build-windows-arm64` | Кросс-компиляция для Windows ARM64. |
| `make build-all` | Все шесть кросс-компилированных бинарных файлов (linux/darwin/windows × amd64/arm64). |
| `make sha256sum` | Создаёт `sha256sum.txt` для артефактов сборки. |
| `make dist` | `clean → build-all → sha256sum`. Эту цель запускает CI. |
| `make test` | Запускает тесты с детектором гонок. |
| `make clean` | Удаляет `dist/`. |

## Соглашения о ветках и коммитах

### Префиксы веток

| Префикс | Назначение |
|---|---|
| `feat/` | Новая функция |
| `fix/` | Исправление ошибки |
| `docs/` | Только документация |
| `refactor/` | Рефакторинг без изменения поведения |
| `test/` | Только изменения тестов |
| `chore/` | Сборка / CI / инструменты |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### Сообщения коммитов

Используйте формат [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short summary>

[optional body explaining the why]
```

Примеры:

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(readme): clarify endpoint resolution priority
refactor(viewer): extract task-card rendering into helper
```

Тот же формат используется для **заголовков PR**, чтобы они аккуратно
отображались в автоматически созданном журнале изменений.

## Структура проекта

```
open-code-review/
├── cmd/opencodereview/        # CLI entry point — flag parsing, dispatch
├── internal/
│   ├── agent/                 # Review agent logic, sub-agent dispatch
│   ├── config/                # Template, rules, allowlist, embedded JSON
│   ├── diff/                  # Git diff parsing, three modes
│   ├── gitcmd/                # Git subprocess runner
│   ├── llm/                   # LLM client (Anthropic & OpenAI), endpoint resolver
│   ├── model/                 # Data structs (LlmComment, Diff, …)
│   ├── pathutil/              # Path utilities
│   ├── release/               # Release-notes generation
│   ├── session/               # JSONL session writer
│   ├── stdout/                # Quiet-able stdout writer
│   ├── suggestdiff/           # Suggestion diff rendering
│   ├── telemetry/             # OpenTelemetry config + helpers
│   ├── tool/                  # Tool registry + provider impls
│   └── viewer/                # Embedded HTTP UI
├── pages/                     # WebUI marketing page (separate React app)
├── plugins/                   # Claude Code slash command
├── extensions/                # Editor extensions (VS Code)
├── examples/                  # CI recipes (GitHub Actions, GitLab CI)
├── skills/                    # Agent SDK skill manifest
├── scripts/                   # NPM postinstall + cross-build scripts
├── npm/                       # Per-platform optional dependency packages
└── bin/                       # NPM wrapper (Node)
```

Большинство изменений затрагивает `internal/agent/`, `internal/tool/` или
`internal/llm/`. Интерфейс CLI в `cmd/opencodereview/` намеренно сделан
тонким: он разбирает флаги, а затем передаёт управление пакету агента.

## Проверки качества кода

Перед созданием PR выполните:

```bash
go fmt ./...
go vet ./...
make test       # race-enabled, runs in CI on every push
make build      # smoke test the binary builds
```

CI запускает тот же набор при каждой отправке изменений — никаких неожиданных
проверок нет.

## Добавление новых инструментов

Инструмент состоит из двух частей:

1. **Определение JSON** в
   [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json):
   имя, описание и параметры JSON Schema, которые видит LLM.
2. **Провайдер Go**, зарегистрированный в `internal/tool/definitions.go` и
   содержащий фактическую реализацию.

Чтобы новое имя инструмента работало, необходимы обе части. Существующие шесть
инструментов описаны в разделе [Инструменты](../tools/); используйте их как
шаблоны.

## Добавление новых шаблонов правил

Измените `internal/config/rules/system_rules.json`, сопоставив новый glob-шаблон
с документом правил, и добавьте соответствующий файл Markdown в
`internal/config/rules/rule_docs/`. Для каждого шаблона используется отдельный
документ правил на английском языке. Параметр `language` лишь добавляет в
системный промпт указание отвечать на выбранном языке; он не переключает файлы
документов правил.

## Процесс работы с PR

1. **Для крупных изменений сначала создайте задачу.** Лучше заранее согласовать
   подход, чем обнаружить расхождения во время ревью кода.
2. **Один PR — одно логическое изменение.** Если у вас есть два несвязанных
   исправления, отправьте два PR.
3. **Обновляйте тесты.** Изменения поведения требуют тестового покрытия;
   `make test` должен проходить.
4. **Обновляйте документацию.** Если изменение затрагивает флаги, ключи
   конфигурации или конвейер ревью, обновите и этот сайт документации (в
   [`docs/`](https://github.com/alibaba/open-code-review)), и соответствующую
   встроенную справку.
5. **Заполните шаблон PR.** Мейнтейнер проведёт ревью, обычно в течение
   нескольких рабочих дней.

## Лицензионное соглашение участника (CLA)

Проект требует подписания Alibaba Open Source CLA. При создании первого PR бот
опубликует ссылку — подпишите соглашение в электронном виде (это займёт около
минуты). Для последующих PR повторная подпись не требуется.

## Первый вклад в проект?

Ищите задачи с метками
[`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue)
или [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted).
Большинство из них невелики, самодостаточны и содержат достаточно контекста в
описании для начала работы.

## См. также

- [Архитектура](../architecture/) — модель устройства проекта, которую стоит
  понять перед изменением `internal/agent/`.
- [Инструменты](../tools/) — как выглядят существующие инструменты.
- Полное руководство для участников:
  [CONTRIBUTING.md](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)
