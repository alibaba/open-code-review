---
title: Телеметрия
sidebar:
  order: 11
---

OCR поддерживает **OpenTelemetry** на уровне первого класса. Каждый запуск ревью создаёт структурированные спаны, метрики и события. При подключении коллектора этих данных достаточно, чтобы ответить на вопросы: «на что агент потратил время?», «сколько стоят разные модели?» и «почему запуск завершился ошибкой?».

## Обзор

По умолчанию телеметрия **выключена**. После включения OCR экспортирует:

- **Спаны** — три спана уровня конвейера (`review.run`, `diff.parse`, `subtask.execute.<file>`) и один короткоживущий спан `event.*` на каждое событие точки принятия решения.
- **Метрики** — агрегированные счётчики и гистограммы длительности ревью, числа проверенных файлов и сгенерированных комментариев, запросов / токенов / задержки LLM и вызовов / задержки инструментов.
- **События** — отдельные внутриспановые события, такие как `plan.skipped`, `token.threshold.exceeded`, `review.started`.

Поддерживаются два экспортёра:

| Экспортёр | Когда использовать |
|---|---|
| `console` | Личное использование / отладка. Красиво выводит спаны в stdout. |
| `otlp` | Интеграция с системами. Отправляет данные в любой совместимый с OTLP коллектор (Jaeger, Tempo, OTel Collector, Datadog Agent, …). |

## Включение телеметрии

Как и endpoint LLM, телеметрия настраивается постоянной конфигурацией или переменными окружения; при конфликте переменные окружения имеют приоритет.

### Через файл конфигурации

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

### Через переменные окружения

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # подразумевает exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # по умолчанию. ВНИМАНИЕ: сейчас реализован только grpc;
                                                    # http/protobuf и http/json принимаются, но ещё не подключены.
export OTEL_SERVICE_NAME=open-code-review-prod      # необязательно; по умолчанию: open-code-review
export OCR_CONTENT_LOGGING=0                        # зарезервировано / сейчас ничего не делает (см. «Логирование содержимого»)
```

Установка `OTEL_EXPORTER_OTLP_ENDPOINT` также принудительно задаёт `exporter=otlp`; это удобно для разовых запусков `OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review`.

## Что экспортируется

### Спаны

Полное дерево спанов для ревью:

```
review.run
├── diff.parse
├── event.review.started                   (событие точки принятия решения)
├── subtask.execute.<file1>
│   ├── event.plan.skipped                 (когда изменения ниже порога)
│   ├── event.plan.failed                  (когда фаза планирования завершилась ошибкой)
│   ├── event.token.threshold.exceeded     (когда prompt > 80% от max_tokens)
│   └── event.subtask.error                (когда подзадача завершилась ошибкой)
├── subtask.execute.<file2>
└── …
```

Обращения к LLM и выполнение инструментов **не** отправляются отдельными спанами: они отражаются только в метриках (см. ниже). События точек принятия решений создают короткоживущие спаны `event.<name>`, привязанные к текущему контексту.

Каждый спан содержит полезные атрибуты:

| Спан | Ключевые атрибуты |
|---|---|
| `review.run` | `error` (задаётся при ошибке запуска) |
| `diff.parse` | `files.changed`, `lines.inserted`, `lines.deleted` |
| `subtask.execute.<file>` | `file.path`, `lines.changed`, `lines.inserted`, `lines.deleted` |
| `event.review.started` | `file.count`, `review.count`, `repo.dir` |
| `event.plan.skipped` | `file.path`, `lines.changed`, `threshold` |
| `event.plan.failed` | `file.path`, `message` |
| `event.token.threshold.exceeded` | `file.path`, `tokens`, `max_tokens` |
| `event.subtask.error` | `file.path`, `error` |

### Метрики

OCR записывает числовые метрики через OTel meter — счётчики и гистограммы, которые коллектор агрегирует далее:

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

События создаются как короткоживущие спаны `event.<name>` в точках принятия решений. Полный список:

| Событие | Значение |
|---|---|
| `review.started` | Диффы загружены; известно количество файлов для ревью. |
| `no.files.changed` | Дифф содержит ноль файлов. |
| `plan.skipped` | Файл оказался ниже `PLAN_MODE_LINE_THRESHOLD`. |
| `plan.failed` | Фаза планирования завершилась ошибкой; основной цикл запущен без плана. |
| `token.threshold.exceeded` | Токены первоначального prompt > 80 % от `MAX_TOKENS`; файл пропущен. |
| `subtask.error` | Подзадача для отдельного файла завершилась ошибкой; спан имеет статус `Error`. |

Используйте их для оповещений о деградации качества ревью задолго до того, как это заметит пользователь.

## Логирование содержимого

Телеметрия экспортирует **структуру** трафика LLM (число запросов, длительности, статусы), но **никогда** не сами prompt или ответы. OCR не пытается прикреплять содержимое сообщений LLM к спанам или событиям: процесс покидают только описанные выше схемы метрик и событий.

Ключ конфигурации `content_logging` (и переопределение `OCR_CONTENT_LOGGING=1`) проходит через слой конфигурации, но сейчас **не** управляет кодом, который отправлял бы содержимое prompt. Считайте флаг зарезервированным.

Чтобы изучить отправленное в LLM или полученное от него, используйте локальные JSONL-расшифровки, которые читает [просмотр сессий](../viewer/). Они целиком хранятся на диске в `~/.opencodereview/` и никогда не отправляются коллектору.

## Рецепты

### Экспортёр console для локальной отладки

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

Спаны выводятся в stdout в удобном для чтения виде. Для длинного запуска передайте вывод в `less`.

### OTel Collector с Tempo + Prometheus

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

Затем в оболочке:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

Откройте Tempo → найдите по `service.name=open-code-review` → выберите любой trace, чтобы увидеть полное дерево спанов.

### Datadog

Приёмник OTLP агента Datadog по умолчанию использует OTLP/gRPC:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

Спаны появятся в APM под именем сервиса, а метрики LLM — в Metrics с перечисленными выше метками.

### Запуск в CI с результатами на панели

Добавьте переменные окружения в шаг конвейера:

```yaml
- name: Код-ревью
  env:
    OCR_LLM_URL: ${{ secrets.OCR_LLM_URL }}
    OCR_LLM_TOKEN: ${{ secrets.OCR_LLM_TOKEN }}
    OCR_LLM_MODEL: claude-opus-4-6
    OCR_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_COLLECTOR_URL }}
    OTEL_SERVICE_NAME: open-code-review-ci
  run: ocr review --from origin/main --to HEAD --audience agent
```

`OTEL_SERVICE_NAME` отделяет trace CI от запусков разработчиков.

## Приоритет разрешения

Когда OCR собирает итоговую конфигурацию телеметрии:

1. Значения по умолчанию (`enabled=false`, `exporter=console`, без endpoint).
2. Ключи `telemetry.*` из `~/.opencodereview/config.json`.
3. Переменные окружения (высший приоритет, **переопределяют** файл).

Поэтому можно оставить `telemetry.enabled=false` в конфигурации и включать телеметрию для отдельного запуска через `OCR_ENABLE_TELEMETRY=1`.

## Семплирование и накладные расходы

OCR экспортирует **всё**. Настройки семплирования нет: за неё отвечает ваш коллектор OTel. Для типичного запуска ревью это:

- 1 спан `review.run` + 1 спан `diff.parse` + 1 спан `subtask.execute.<file>` на каждый проверенный файл + 1 короткоживущий спан `event.*` на каждое событие точки принятия решения.
- PR из 10 файлов создаёт всего около 15–25 спанов. Обращения к LLM и вызовы инструментов увеличивают счётчики метрик, но не создают дополнительных спанов.

Экспорт выполняется **пакетно и асинхронно**, поэтому телеметрия не блокирует цикл ревью. Если коллектор недоступен, OCR записывает предупреждение и продолжает работу; ревью всё равно выдаёт обычный результат.

## Устранение неполадок

| Симптом | Вероятная причина |
|---|---|
| Ничего не экспортируется | Не задан `OCR_ENABLE_TELEMETRY` / `telemetry.enabled`. По умолчанию телеметрия **выключена**. |
| OTLP работает локально, но не в prod | OCR сейчас реализует только OTLP/gRPC: `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf` (или `http/json`) принимается, но ещё не подключён, поэтому переключение не поможет. Проверьте endpoint и прослушивание gRPC коллектором. |
| Есть спаны, но нет метрик | Некоторые коллекторы по умолчанию включают только конвейер traces; добавьте конвейер `metrics` в конфигурацию. |
| В спанах нет prompt | OCR никогда не добавляет содержимое prompt в телеметрию — см. [Логирование содержимого](#логирование-содержимого). Вместо этого изучите расшифровки через [просмотр сессий](../viewer/). |

## См. также

- [Конфигурация](../configuration/) — полный справочник ключей пространства имён `telemetry.*`.
- [Архитектура](../architecture/) — что именно измеряет каждый спан.
- [Документация OpenTelemetry](https://opentelemetry.io/docs/) — настройка коллектора и экспортёров.
