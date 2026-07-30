---
title: Участие в разработке
sidebar:
  order: 13
---

OCR — проект с открытым исходным кодом под лицензией Apache-2.0. Мы рады сообщениям об ошибках, исправлениям документации и вкладам в код. Эта страница — краткая справка; каноническая версия находится в [`CONTRIBUTING.md`](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md).

## Как можно помочь

Чтобы быть полезным, необязательно писать на Go:

- **Сообщения об ошибках** — создайте [GitHub issue](https://github.com/alibaba/open-code-review/issues/new/choose) с шагами воспроизведения.
- **Запросы функций** — начните обсуждение в [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas) или создайте issue с запросом функции.
- **Документация** — опечатки, недостающие примеры, неработающие ссылки; такие PR часто вливаются быстрее всего.
- **Ревью других PR** — комментарии от пользователей, не являющихся мейнтейнерами, снижают нагрузку на ревьюеров.
- **Код** — исправления ошибок, работа над производительностью, новые возможности.

## Настройка локальной разработки

### Предварительные требования

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Получение исходного кода

```bash
# Сделайте fork на GitHub, затем:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # создаёт dist/opencodereview
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> Remote `upstream` доступен только для чтения. Отправляйте изменения в `origin` (свой fork) и создавайте PR оттуда.

### Запуск локальной сборки

```bash
./dist/opencodereview review --preview
```

Для удобства создайте символьную ссылку `~/bin/ocr-dev` на `dist/opencodereview`, чтобы вызывать `ocr-dev` из любого репозитория.

### Цели Make

| Цель | Назначение |
|---|---|
| `make build` | Собрать для текущей платформы → `dist/opencodereview`. |
| `make build-darwin-amd64` | Кросс-компиляция для macOS Intel. |
| `make build-darwin-arm64` | Кросс-компиляция для macOS Apple Silicon. |
| `make build-linux-amd64` | Кросс-компиляция для Linux x86_64. |
| `make build-linux-arm64` | Кросс-компиляция для Linux ARM64. |
| `make build-windows-amd64` | Кросс-компиляция для Windows x86_64. |
| `make build-windows-arm64` | Кросс-компиляция для Windows ARM64. |
| `make build-all` | Все шесть бинарников кросс-компиляции (linux/darwin/windows × amd64/arm64). |
| `make sha256sum` | Создать `sha256sum.txt` для артефактов сборки. |
| `make dist` | `clean → build-all → sha256sum`. Выполняется в CI. |
| `make test` | Запустить тесты с race detector. |
| `make clean` | Удалить `dist/`. |

## Ветки и соглашения о коммитах

### Префиксы веток

| Префикс | Назначение |
|---|---|
| `feat/` | Новая возможность |
| `fix/` | Исправление ошибки |
| `docs/` | Только документация |
| `refactor/` | Рефакторинг без изменения поведения |
| `test/` | Изменения только в тестах |
| `chore/` | Сборка / CI / инструменты |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### Сообщения коммитов

Используйте формат [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <краткое описание>

[необязательное пояснение причины]
```

Примеры:

```
feat(agent): добавить поддержку пользовательских определений инструментов
fix(llm): обработать ошибки тайм-аута в вызовах Anthropic API
docs(readme): уточнить приоритет разрешения endpoint
refactor(viewer): вынести отрисовку карточек задач в вспомогательную функцию
```

Этот же формат используется для **названий PR**, чтобы они аккуратно попадали в сгенерированный changelog.

## Структура проекта

```
open-code-review/
├── cmd/opencodereview/        # точка входа CLI — разбор флагов, диспетчеризация
├── internal/
│   ├── agent/                 # логика агента ревью, запуск подагентов
│   ├── config/                # шаблоны, правила, allowlist, встроенный JSON
│   ├── diff/                  # разбор Git-диффа, три режима
│   ├── gitcmd/                # запуск подпроцессов Git
│   ├── llm/                   # клиент LLM (Anthropic и OpenAI), разрешение endpoint
│   ├── model/                 # структуры данных (LlmComment, Diff, …)
│   ├── pathutil/              # утилиты для путей
│   ├── release/               # генерация заметок к релизу
│   ├── session/               # запись JSONL-сессии
│   ├── stdout/                # отключаемый writer stdout
│   ├── suggestdiff/           # отрисовка диффа предложений
│   ├── telemetry/             # конфигурация и помощники OpenTelemetry
│   ├── tool/                  # реестр инструментов и реализации провайдеров
│   └── viewer/                # встроенный HTTP-интерфейс
├── pages/                     # маркетинговая WebUI-страница (отдельное React-приложение)
├── plugins/                   # slash-команда Claude Code
├── extensions/                # расширения редакторов (VS Code)
├── examples/                  # рецепты CI (GitHub Actions, GitLab CI)
├── skills/                    # манифест навыка Agent SDK
├── scripts/                   # NPM postinstall и скрипты кросс-сборки
├── npm/                       # пакеты необязательных зависимостей для платформ
└── bin/                       # NPM-обёртка (Node)
```

Большинство вкладов затрагивает `internal/agent/`, `internal/tool/` или `internal/llm/`. Поверхность CLI в `cmd/opencodereview/` намеренно тонкая: разбор флагов, затем диспетчеризация в пакет агента.

## Проверки качества кода

Перед созданием PR:

```bash
go fmt ./...
go vet ./...
make test       # с race detector; выполняется в CI при каждом push
make build      # smoke-тест сборки бинарника
```

CI запускает тот же набор при каждом push, без неожиданностей.

## Добавление новых инструментов

Инструмент состоит из двух частей:

1. **Определение JSON** в [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json): имя, описание и параметры JSON Schema, которые видит LLM.
2. **Провайдер Go**, зарегистрированный в `internal/tool/definitions.go`, с фактической реализацией.

Для работы нового имени инструмента необходимы обе части. См. [Инструменты](../tools/) для шести существующих примеров и используйте их как шаблоны.

## Добавление шаблонов правил

Измените `internal/config/rules/system_rules.json`, чтобы сопоставить новый glob с документом правила, и добавьте соответствующий Markdown в `internal/config/rules/rule_docs/`. Документы правил создаются по одному файлу на шаблон (на английском). Настройка `language` лишь добавляет директиву в системный prompt с указанием отвечать на этом языке; файлы документов правил она не переключает.

## Процесс PR

1. **Сначала создайте issue для больших изменений.** Согласовать подход лучше, чем обнаружить расхождение во время код-ревью.
2. **Одно логическое изменение на PR.** Если у вас два несвязанных исправления, отправьте два PR.
3. **Обновляйте тесты.** Изменения поведения требуют покрытия тестами; `make test` должен проходить.
4. **Обновляйте документацию.** Если изменение затрагивает флаги, ключи конфигурации или конвейер ревью, обновите и этот сайт документации (в [`docs/`](https://github.com/alibaba/open-code-review)), и подходящую встроенную справку.
5. **Заполните шаблон PR.** Мейнтейнер проведёт ревью, обычно в течение нескольких рабочих дней.

## Contributor License Agreement (CLA)

Проект требует Alibaba Open Source CLA. При первом PR бот опубликует ссылку: подпишите соглашение электронно (это займёт минуту). Для последующих PR повторная подпись не нужна.

## Первый вклад?

Ищите задачи с метками [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) или [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted). Большинство из них невелики, самодостаточны и содержат достаточно контекста в описании задачи, чтобы начать работу.

## См. также

- [Архитектура](../architecture/) — ментальная модель, которая понадобится перед изменением `internal/agent/`.
- [Инструменты](../tools/) — устройство существующих инструментов.
- Полное руководство для участников: [CONTRIBUTING.md](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)
