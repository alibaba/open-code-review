---
title: Конфигурация
sidebar:
  order: 5
---

Файл конфигурации находится по пути `~/.opencodereview/config.json`. Изменить
его можно тремя способами:

- **Интерактивный TUI** — `ocr config provider` / `ocr config model` с пошаговыми меню.
- **Командная строка** — `ocr config set <key> <value>`, удобно для скриптов и CI.
- **Редактирование вручную (не рекомендуется)** — напрямую изменить JSON-файл (при следующей записи через `ocr config set` его форматирование изменится).

## Настройка модели

### Рекомендуемый способ: интерактивная настройка

```bash
ocr config provider
```

Команда позволяет выбрать встроенного или пользовательского провайдера,
ввести API-ключ и выбрать модель, сохраняет всё в файл конфигурации, а затем
один раз запускает `ocr llm test` для проверки эндпоинта. Чтобы позже сменить
модель:

```bash
ocr config model
```

### Неинтерактивная настройка (CI / среды без TUI)

Запишите параметры в тот же файл конфигурации с помощью `ocr config set`:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### Встроенные провайдеры

Перечисленные ниже провайдеры поставляются вместе с OCR; для них заранее
заданы базовый URL и протокол, поэтому после выбора достаточно указать
API-ключ. Если `providers.<name>.api_key` не задан, OCR использует
соответствующую переменную окружения.

| Имя | Протокол | Базовый URL | Переменная окружения для API-ключа |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow` | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |

### Пользовательские провайдеры

Любое имя провайдера, которого нет в таблице выше, считается
пользовательским. Для него необходимо задать как минимум `url` и `protocol`
(`protocol` может принимать значения `anthropic`, `openai` или
`openai-responses`):

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

Используйте `openai-responses`, если провайдеру или модели требуется OpenAI
Responses API (`/v1/responses`):

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

В качестве `url` можно указать как базовый URL API, так и полный эндпоинт
`/responses` — OCR нормализует оба варианта.

Локальная модель, запущенная через Ollama, — это обычный пользовательский
провайдер, указывающий на локальный OpenAI-совместимый эндпоинт:

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama игнорирует API-ключ, однако для пользовательских провайдеров требуется
непустое значение `api_key` (для них нет резервного варианта с переменной
окружения), поэтому укажите любое значение-заполнитель. Сама модель должна
поддерживать встроенные вызовы инструментов — перед выбором модели прочитайте
раздел [«Не удалось разобрать вызовы инструментов» (локальные модели / Ollama)](../faq/#no-tool-calls-parsed-local-models-ollama)
в FAQ.

### Тайм-ауты

Для каждого HTTP-запроса к LLM действует тайм-аут, по умолчанию **300 секунд**.
Медленным локальным моделям (или большим файлам) может потребоваться больше
времени. Доступны три настройки с возрастающей областью действия:

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec`
  — для отдельного провайдера, в секундах.
- `llm.timeout_sec` — для устаревшего раздела `llm`, в секундах.
- Переменная окружения `OCR_LLM_TIMEOUT` — целое число секунд; переопределяет
  значение из файла конфигурации для всех вариантов разрешения настроек.

Ключи `timeout_sec` не поддерживаются командой `ocr config set` — измените
`~/.opencodereview/config.json` напрямую:

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

### Дополнительные HTTP-коды для повторных попыток

Некоторые LLM-провайдеры используют нестандартные HTTP-коды 4xx для временных
ошибок — например, возвращают `403` или `400` при ограничении частоты запросов.
Параметр `retry_codes` позволяет OCR повторять такие запросы с помощью
существующего механизма повторных попыток SDK.

`retry_codes` — массив целых чисел. Параметр можно задать как
`llm.retry_codes` или `custom_providers.<name>.retry_codes`. При использовании
`ocr config set` передавайте коды через запятую:

```bash
ocr config set llm.retry_codes 403,400
ocr config set custom_providers.my-gateway.retry_codes 403,400
```

Допускаются только HTTP-коды 4xx. SDK уже автоматически повторяет запросы при
ответах с кодами `408`, `409` и `429`, поэтому при чтении файла конфигурации
OCR игнорирует эти избыточные коды. Если задать их через `ocr config set`, OCR также выводит
предупреждение и не сохраняет их в конфигурации. При ответах 5xx SDK уже
выполняет повторные попытки по умолчанию, поэтому такие коды нельзя добавлять
в `retry_codes`.

### Лимит запроса на файл

По умолчанию OCR ограничивает промпт для каждого ревью файла 58 888 токенами.
Чтобы увеличить лимит для модели с большим контекстным окном, сохраните
`max_tokens`:

```bash
ocr config set max_tokens 200000
```

Эта настройка применяется как к `ocr review`, так и к `ocr scan`. Используйте
`--max-tokens` для разового переопределения без изменения сохранённой
конфигурации:

```bash
ocr review --max-tokens 200000
ocr scan --max-tokens 200000
```

Флаг для конкретного запуска имеет приоритет над `max_tokens`; если не задано
ни то, ни другое, OCR использует встроенное значение по умолчанию из шаблона
задачи. Этот лимит действует на файл и не зависит ни от предела выходных
токенов модели, ни от `--max-tokens-budget`, который ограничивает общее
использование токенов за запуск. Чтобы восстановить встроенное значение по
умолчанию, используйте `ocr config unset max_tokens`.

### Проверка подключения

```bash
ocr llm test
```

### Использование существующих переменных окружения

Если у вас уже настроены переменные Claude Code `ANTHROPIC_*` или собственные
переменные OCR `OCR_LLM_*`, OCR обнаружит их автоматически — файл конфигурации
не нужен.

### Использование CC-Switch

Если вы используете [CC-Switch](https://github.com/farion1231/cc-switch) с
включённой [службой маршрутизации](https://www.ccswitch.io/en/docs?section=proxy&item=service),
укажите локальный прокси в качестве `url` провайдера — дополнительная настройка
не требуется:

```bash
# Claude (Anthropic-compatible)
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / OpenAI-compatible — set that provider's url key instead
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

В `api_key` можно указать любое значение. `extra_body` (как и другие поля
конкретного провайдера) продолжает применяться обычным образом.

### Отправка полей, специфичных для поставщика

Некоторым провайдерам требуются нестандартные поля запроса (например,
`thinking` в стиле Bedrock). Используйте `extra_body` (его содержимое
добавляется к каждому запросу), чтобы отправлять их без изменения исходного
кода:

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

## Настройка языка ревью

Параметр `language` определяет, на каком языке будут написаны комментарии
ревью; если он не задан, используется английский:

```bash
ocr config set language 中文
ocr config set language English
```

## См. также

- [Быстрый старт](../quickstart/) — минимальная настройка и первое ревью.
- [Справочник CLI](../cli-reference/) — все флаги, принимаемые командой ревью.
