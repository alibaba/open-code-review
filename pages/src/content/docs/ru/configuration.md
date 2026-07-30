---
title: Конфигурация
sidebar:
  order: 5
---

Файл конфигурации находится по пути `~/.opencodereview/config.json`. Его можно изменить тремя способами:

- **Интерактивный TUI** — `ocr config provider` / `ocr config model` с пошаговыми меню.
- **Командная строка** — `ocr config set <key> <value>`; удобно для скриптов и CI.
- **Ручное редактирование (не рекомендуется)** — непосредственно JSON-файл (он будет переформатирован при следующей записи через `ocr config set`).

## Настройка модели

### Рекомендуемый способ: интерактивная настройка

```bash
ocr config provider
```

Команда позволяет выбрать встроенный или пользовательский провайдер, ввести API-ключ, выбрать модель, сохраняет всё в файл конфигурации, а затем один раз запускает `ocr llm test` для проверки эндпоинта. Чтобы позже сменить модель:

```bash
ocr config model
```

### Неинтерактивная настройка (CI / среды без TUI)

Запишите те же значения конфигурации с помощью `ocr config set`:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### Встроенные провайдеры

Следующие провайдеры поставляются вместе с OCR: их Base URL и протокол уже заданы. После выбора остаётся только указать API-ключ. Если `providers.<name>.api_key` не задан, OCR использует соответствующую переменную окружения.

| Имя | Протокол | Базовый URL | Переменная окружения API-ключа |
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
| `minimax` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |

### Пользовательские провайдеры

Любое имя провайдера, которого нет в таблице выше, считается пользовательским и должно содержать как минимум `url` и `protocol` (`protocol` — `anthropic`, `openai` или `openai-responses`):

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

Используйте `openai-responses`, если провайдеру или модели требуется OpenAI Responses API (`/v1/responses`):

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

Локальная модель, обслуживаемая Ollama, — это пользовательский провайдер, указывающий на локальный OpenAI-совместимый эндпоинт:

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama игнорирует API-ключ, но пользовательским провайдерам необходим непустой `api_key` (для них нет запасного варианта с переменной окружения), поэтому укажите любое значение-заполнитель. Сама модель должна поддерживать нативные вызовы инструментов — перед выбором ознакомьтесь с разделом FAQ [«Вызовы инструментов не распознаны» (локальные модели / Ollama)](../faq/#no-tool-calls-parsed-local-models-ollama).

### Тайм-ауты

Для каждого запроса к LLM действует HTTP-тайм-аут, по умолчанию **300 секунд**. Медленным локальным моделям (или большим файлам) может потребоваться больше. Есть три настройки, от более узкой к более широкой области действия:

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec`
  — для отдельного провайдера, в секундах.
- `llm.timeout_sec` — для устаревшего раздела `llm`, в секундах.
- Переменная окружения `OCR_LLM_TIMEOUT` — целое число секунд; переопределяет значение из файла конфигурации для всех путей разрешения.

Ключи `timeout_sec` не поддерживаются `ocr config set` — отредактируйте `~/.opencodereview/config.json` напрямую:

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

### Проверка подключения

```bash
ocr llm test
```

### Повторное использование существующих переменных окружения

Если у вас уже настроены переменные окружения `ANTHROPIC_*` из Claude Code или собственные `OCR_LLM_*` из OCR, OCR автоматически их подхватит — файл конфигурации не нужен.

### Использование CC-Switch

Если вы используете [CC-Switch](https://github.com/farion1231/cc-switch) с включённым [сервисом маршрутизации](https://www.ccswitch.io/en/docs?section=proxy&item=service), укажите в `url` провайдера локальный прокси — больше ничего настраивать не нужно:

```bash
# Claude (совместимый с Anthropic)
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / совместимый с OpenAI — укажите ключ url этого провайдера
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

`api_key` может иметь любое значение. `extra_body` (и другие поля для отдельного провайдера) продолжают работать как обычно.

### Отправка полей, специфичных для провайдера

Некоторым провайдерам требуются нестандартные поля запроса (например, `thinking` в стиле Bedrock). Используйте `extra_body` (он объединяется с каждым запросом), чтобы передавать их без изменения исходного кода:

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

## Настройка языка ревью

`language` определяет язык комментариев ревью; если он не задан, используется английский:

```bash
ocr config set language 中文
ocr config set language English
```

## См. также

- [Быстрый старт](../quickstart/) — минимальная настройка и первое ревью.
- [Справочник CLI](../cli-reference/) — все флаги, которые принимает команда ревью.
