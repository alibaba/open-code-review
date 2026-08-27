---
title: Телеметрия
sidebar:
  order: 11
---

OCR включает полноценную встроенную поддержку **OpenTelemetry**. Каждый запуск
ревью создаёт структурированные спаны, метрики и события. После подключения к
коллектору этих данных достаточно, чтобы ответить на вопросы «на что агент
потратил время?», «сколько стоят разные модели?» и «почему этот запуск
завершился с ошибкой?».

## Обзор

По умолчанию телеметрия **отключена**. После включения OCR экспортирует:

- **Спаны** — три спана уровня конвейера (`review.run`, `diff.parse`,
  `subtask.execute.group.<group-key>`) и по одному кратковременному спану
  `event.*` для каждого события в точке принятия решения.
- **Метрики** — агрегированные счётчики и гистограммы длительности ревью,
  проверенных файлов, созданных комментариев, запросов / токенов / задержки LLM,
  а также вызовов / задержки инструментов.
- **События** — отдельные события внутри спанов, например `plan.skipped`,
  `token.threshold.exceeded`, `review.started`.

Поддерживаются два экспортёра:

| Экспортёр | Когда использовать |
|---|---|
| `console` | Личное использование / отладка. Выводит спаны в удобочитаемом виде в stdout. |
| `otlp` | Системная интеграция. Отправляет данные любому OTLP-совместимому коллектору (Jaeger, Tempo, OTel Collector, Datadog Agent и т. д.). |

## Включение телеметрии

Как и эндпоинт LLM, телеметрия настраивается с помощью постоянной конфигурации
или переменных окружения; при конфликте переменные окружения имеют приоритет.

### Настройка через файл конфигурации

```bash
ocr config set telemetry.enabled        true
ocr config set telemetry.exporter       otlp
ocr config set telemetry.otlp_endpoint  localhost:4317
ocr config set telemetry.content_logging false
```

Результат в `~/.opencodereview/config.json`:

```json
{
  "telemetry": {
    "enabled": true,
    "exporter": "otlp",
    "otlp_endpoint": "localhost:4317",
    "content_logging": false
  }
}
```

### Настройка через переменные окружения

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # implies exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # default; http/protobuf and http/json
                                                    # are also supported
export OTEL_SERVICE_NAME=open-code-review-prod      # optional; default: open-code-review
export OCR_CONTENT_LOGGING=0                        # reserved / currently a no-op (see Content logging)
```

Установка `OTEL_EXPORTER_OTLP_ENDPOINT` также принудительно задаёт
`exporter=otlp`. Это удобно для разовых запусков вида
`OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review`.

### Формат эндпоинта

Эндпоинт можно указать как простой `host:port` или вместе со схемой. При
наличии схемы она определяет безопасность транспорта: `http://` использует
незашифрованное соединение, а `https://` — TLS. Для простого `host:port`
используется TLS.

Обратите внимание, что порт по умолчанию зависит от протокола: `4317` для gRPC
и `4318` для HTTP:

```bash
# gRPC (default)
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# HTTP
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Для протоколов HTTP эндпоинт является **базовым URL**, к которому добавляется
путь соответствующего сигнала. Поэтому `http://localhost:4318` отправляет
трассировки в `http://localhost:4318/v1/traces`. Базовый путь сохраняется, что
необходимо для бэкендов, доступных по определённому префиксу:

```bash
# traces go to http://collector:3000/api/public/otel/v1/traces
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:3000/api/public/otel
```

В gRPC пути URL отсутствуют, поэтому это относится только к протоколам HTTP.

## Что экспортируется

### Спаны

Полное дерево спанов одного ревью:

```
review.run
├── diff.parse
├── event.review.started                   (decision-point event)
├── subtask.execute.group.<group-key1>
│   ├── event.plan.skipped                 (when changes are below both thresholds)
│   ├── event.plan.failed                  (when plan phase errored)
│   ├── event.token.threshold.exceeded     (when prompt > 80% of max_tokens)
│   ├── main.loop                          (one span per review round)
│   └── event.subtask.error                (when the subtask errored)
├── subtask.execute.group.<group-key2>
└── …
```

Один спан `subtask.execute.group.*` создаётся на каждую проверенную **группу**,
а не на каждый файл: файлы семантически объединяются в группы до ревью. Ключ
группы — это пути файлов группы, отсортированные и соединённые запятыми (для
группы из одного файла это просто этот путь).

Циклы запросов к LLM и выполнения инструментов **не** создают отдельных
спанов: они отображаются только в метриках (см. ниже). События в точках
принятия решений создаются как кратковременные спаны `event.<name>`,
присоединённые к текущему контексту.

Каждый спан содержит полезные атрибуты:

| Спан | Основные атрибуты |
|---|---|
| `review.run` | `error` (устанавливается при сбое запуска) |
| `diff.parse` | `files.changed`, `lines.inserted`, `lines.deleted` |
| `subtask.execute.group.<group-key>` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file` |
| `main.loop` | `group.label`, `round` |
| `event.review.started` | `file.count`, `review.count`, `repo.dir` |
| `event.plan.skipped` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file`, `threshold`, `threshold.group` |
| `event.plan.failed` | `group.label`, `message` |
| `event.token.threshold.exceeded` | `group.label`, `tokens`, `max_tokens`, `round` |
| `event.subtask.error` | `group.label`, `error` |

### Метрики

OCR записывает числовые метрики с помощью измерителя OTel: счётчики и гистограммы,
которые затем агрегирует коллектор:

| Метрика | Тип | Единица | Метки |
|---|---|---|---|
| `ocr.review.duration_seconds` | гистограмма | `s` | — |
| `ocr.files_reviewed_total` | счётчик | — | — |
| `ocr.comments_generated_total` | счётчик | — | — |
| `ocr.llm.requests_total` | счётчик | — | `model`, `status` (`ok` / `error`) |
| `ocr.llm.request_duration_seconds` | гистограмма | `s` | `model` |
| `ocr.llm.tokens_used` | счётчик | — | `model`, `type` (сейчас всегда `total`) |
| `ocr.tool.calls_total` | счётчик | — | `tool.name`, `status` (`ok` / `error`) |
| `ocr.tool.execution_duration_seconds` | гистограмма | `s` | `tool.name` |

### События

В точках принятия решений события создаются как кратковременные спаны
`event.<name>`. Полный список:

| Событие | Значение |
|---|---|
| `review.started` | Различия загружены; известно количество файлов для ревью. |
| `no.files.changed` | После разрешения diff не осталось файлов. |
| `plan.skipped` | Группа оказалась ниже обоих порогов plan: у самого большого файла группы изменений меньше, чем `PLAN_MODE_LINE_THRESHOLD`, и (для групп из 2+ файлов) суммарно меньше, чем `PLAN_MODE_GROUP_LINE_THRESHOLD`. |
| `plan.failed` | Этап планирования завершился с ошибкой; основной цикл запущен без плана. |
| `token.threshold.exceeded` | Число токенов промпта превысило 80 % от `MAX_TOKENS` (предел ввода); группа пропущена. |
| `subtask.error` | Подзадача отдельной группы завершилась с ошибкой; создаётся со статусом спана `Error`. |

Используйте эти события, чтобы обнаруживать ухудшение качества ревью задолго до
того, как его заметит пользователь.

## Журналирование содержимого

Телеметрия экспортирует **форму** трафика LLM (количество, длительность,
статусы), но **никогда** не экспортирует сами промпты или ответы. OCR не
прикрепляет содержимое сообщений LLM к спанам или событиям: за пределы процесса
выходят только описанные выше схемы метрик и событий.

Ключ конфигурации `content_logging` (и переопределение переменной окружения
`OCR_CONTENT_LOGGING=1`) проходит через слой конфигурации, но сейчас **не**
управляет ни одной ветвью кода, которая могла бы отправлять содержимое промпта.
Считайте этот флаг зарезервированным.

Чтобы проверить, что было отправлено в LLM или получено от неё, используйте
локальные расшифровки JSONL, которые читает [Просмотр сессий](../viewer/).
Они хранятся только на диске в `~/.opencodereview/` и никогда не отправляются
коллектору.

Для более глубокой отладки установите `OCR_RAW_LOGGING=1`: исходные тела
запросов и ответов каждого вызова LLM будут записываться в
`~/.opencodereview/raw/`, что даёт более подробную отладочную информацию,
чем расшифровки сессий. По умолчанию выключено.

## Рецепты

### Консольный экспортёр для локальной отладки

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

Спаны выводятся в stdout в удобочитаемом виде. Для просмотра длинного запуска
направьте вывод в `less`.

### OTel Collector с Tempo и Prometheus

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols: { grpc: { endpoint: 0.0.0.0:4317 } }

exporters:
  otlp/tempo:
    endpoint: tempo:4317
    tls: { insecure: true }
  prometheus:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    traces:  { receivers: [otlp], exporters: [otlp/tempo] }
    metrics: { receivers: [otlp], exporters: [prometheus] }
```

Затем выполните в оболочке:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

Откройте Tempo → выполните поиск по `service.name=open-code-review` → выберите
любую трассировку, чтобы увидеть полное дерево спанов.

### Datadog

Приёмник OTLP агента Datadog по умолчанию использует OTLP/gRPC:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

Спаны отображаются в APM под именем сервиса, а метрики LLM — в разделе Metrics
с указанными выше метками.

### Запуск в CI с результатами на панели мониторинга

Передайте переменные окружения на этапе конвейера:

```yaml
- name: Code review
  env:
    OCR_LLM_URL: ${{ secrets.OCR_LLM_URL }}
    OCR_LLM_TOKEN: ${{ secrets.OCR_LLM_TOKEN }}
    OCR_LLM_MODEL: claude-opus-4-6
    OCR_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_COLLECTOR_URL }}
    OTEL_SERVICE_NAME: open-code-review-ci
  run: ocr review --from origin/main --to HEAD --audience agent
```

`OTEL_SERVICE_NAME` отделяет трассировки CI от запусков разработчиков.

## Приоритет разрешения

При формировании итоговой конфигурации телеметрии OCR использует следующий
порядок:

1. Значения по умолчанию (`enabled=false`, `exporter=console`, эндпоинт
   отсутствует).
2. Ключи `telemetry.*` из `~/.opencodereview/config.json`.
3. Переменные окружения (наивысший приоритет, **переопределяют** файл).

Таким образом, в конфигурации можно оставить `telemetry.enabled=false` и
включать телеметрию для отдельного запуска с помощью
`OCR_ENABLE_TELEMETRY=1`.

## Сэмплирование и накладные расходы

OCR экспортирует **всё**. Настройка сэмплирования отсутствует: за неё отвечает
ваш коллектор OTel. Обычный запуск ревью создаёт:

- 1 спан `review.run` + 1 спан `diff.parse` + 1 спан
  `subtask.execute.group.<group-key>` на каждую проверенную группу (плюс его
  дочерние `plan.execute` / `main.loop` / `review_filter.execute`) +
  1 кратковременный спан `event.*` на каждое событие в точке принятия решения.
- PR из 10 файлов создаёт в общей сложности примерно 15–25 спанов — меньше,
  когда группировка объединяет файлы, и больше, когда предустановка effort
  выполняет дополнительные раунды ревью. Циклы запросов LLM и вызовы
  инструментов увеличивают счётчики метрик, но не создают дополнительные
  спаны.

Экспорт выполняется **пакетно и асинхронно**, поэтому телеметрия не блокирует
цикл ревью. Если коллектор недоступен, OCR записывает предупреждение и
продолжает работу; ревью по-прежнему создаёт обычный вывод.

## Устранение неполадок

| Симптом | Вероятная причина |
|---|---|
| Ничего не экспортируется | Не задана переменная `OCR_ENABLE_TELEMETRY` или параметр `telemetry.enabled`. По умолчанию телеметрия **отключена**. |
| OTLP работает локально, но не работает в рабочей среде | Убедитесь, что протокол соответствует коллектору. По умолчанию используется gRPC, а многие управляемые бэкенды принимают OTLP только через HTTP: задайте `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf` и используйте порт HTTP (`4318`, а не `4317`). |
| Данные не поступают, ошибки нет | Экспортёр создаётся лениво, поэтому неверный эндпоинт приводит к незаметной ошибке во время экспорта, а не при запуске. Проверьте журнал доступа коллектора и запрашиваемый путь: для протоколов HTTP эндпоинт является базовым URL, к которому добавляется путь сигнала, поэтому `http://host:4318` отправляет данные в `http://host:4318/v1/traces`. |
| Спаны отображаются, а метрики — нет | Некоторые коллекторы по умолчанию включают только конвейер трассировок; добавьте в конфигурацию конвейер `metrics`. |
| В спанах нет промптов | OCR никогда не добавляет содержимое промптов в телеметрию; см. [Журналирование содержимого](#content-logging). Просматривайте расшифровки через [Просмотр сессий](../viewer/). |

## См. также

- [Конфигурация](../configuration/) — полный справочник ключей пространства имён
  `telemetry.*`.
- [Архитектура](../architecture/) — что именно измеряет каждый спан.
- [Документация OpenTelemetry](https://opentelemetry.io/docs/) — настройка
  коллектора и экспортёров.
