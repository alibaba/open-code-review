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
| `bedrock` | anthropic-bedrock | определяется `aws_region` | — (цепочка учётных данных AWS) |
| `claude-code` | claude-cli | — (локальный CLI `claude`) | — (вход через CLI) |
| `codex` | codex-cli | — (локальный CLI `codex`) | — (вход через CLI) |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `openai-responses` | openai-responses | `https://api.openai.com/v1` | `OPENAI_RESPONSES_API_KEY` |
| `gemini` | openai | `https://generativelanguage.googleapis.com/v1beta/openai` | `GEMINI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `kimi-global` | openai | `https://api.moonshot.ai/v1` | `MOONSHOT_GLOBAL_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow` | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |
| `xai` | openai | `https://api.x.ai/v1` | `XAI_API_KEY` |

### Переопределение Base URL встроенного провайдера

У каждого встроенного провайдера есть предустановленный Base URL
(см. таблицу выше). Чтобы направить встроенный провайдер на другую конечную
точку — например, на собственный шлюз LiteLLM, который редко находится по
предустановленному адресу `http://localhost:4000/v1` — задайте
`providers.<name>.url`:

```bash
ocr config set provider                   litellm
ocr config set model                      openai/gpt-5.4
ocr config set providers.litellm.api_key  "$LITELLM_API_KEY"
ocr config set providers.litellm.url      https://gateway.internal:8000/v1
```

Заданный `url` имеет приоритет над предустановленным Base URL. Если
`providers.<name>.url` не задан (или очищен), OCR возвращается к
предустановленному значению по умолчанию — поэтому его нужно задавать только
когда ваша конечная точка отличается.

### AWS Bedrock

`bedrock` использует тот же Messages API, что и `anthropic`, но запросы
подписываются по SigV4 из стандартной цепочки учётных данных AWS вместо
передачи API-ключа, а хост определяется регионом. Задавать `api_key` не нужно,
и он не принимается как замена подписи:

```bash
ocr config set provider                      bedrock
ocr config set model                         us.anthropic.claude-sonnet-4-6
ocr config set providers.bedrock.aws_region  us-west-2
ocr config set providers.bedrock.aws_profile example-profile
```

| Поле | Значение |
|---|---|
| `providers.bedrock.aws_region` | Регион, чей хост `bedrock-runtime` обслуживает запрос. По умолчанию — `AWS_REGION` или активный профиль. |
| `providers.bedrock.aws_profile` | Именованный профиль для получения учётных данных. По умолчанию — `AWS_PROFILE` или окружающая цепочка. |

Оба поля необязательны: если они не заданы, выбор делает стандартная цепочка,
как и для любого другого инструмента AWS. Явная фиксация делает запуск
воспроизводимым без предварительного экспорта `AWS_PROFILE` — это особенно
важно на CI-раннерах с другим значением по умолчанию.

Идентификаторы моделей привязаны к аккаунту **и** к региону, поэтому список,
который поставляется с OCR, — отправная точка, а не закрытый набор: подходящий
для вашего аккаунта ID inference-профиля или ARN прикладного inference-профиля
принимается, даже если его нет в списке. Выполните
`aws bedrock list-inference-profiles --region <region>`, чтобы увидеть, что
доступно в аккаунте; суффикс версии вроде `-v1:0` недопустим для новых семейств.

`ocr llm test` показывает регион и профиль вместо URL, потому что у bedrock нет
настроенного URL — хост определяется регионом:

```
Source: provider:bedrock
Region: us-east-1
Profile: example-profile
Model:  claude-sonnet-5
✓ Connection test successful
```

Через `llm.protocol` и `OCR_LLM_PROTOCOL` bedrock **недоступен**. Этот блок
описывает один URL и один токен, в нём негде указать регион или профиль, а сами
эти значения bedrock не использует, поэтому такая комбинация отклоняется, а не
принимается и молча игнорируется.

### Claude Code CLI и Codex CLI (без API-ключа) {#claude-code-cli-and-codex-cli-no-api-key}

OCR может работать через локально установленный CLI Claude Code или Codex вместо
API-ключа. Модель доступна через ваш уже выполненный вход в CLI, поэтому нет ни
`api_key`, ни `url`, и хранить нужно только выбор провайдера.

**Предварительные требования.** CLI должен быть установлен, и в нём должен быть
выполнен вход:

- `claude-code` требует, чтобы бинарник `claude` был в `PATH` и был выполнен `claude login`.
- `codex` требует, чтобы бинарник `codex` был в `PATH` и был выполнен `codex login`.

Проверено с Claude Code 2.1.261 и Codex 0.153.2.

**Как это работает.** OCR сам выполняет цикл ревью и вызывает CLI только как
модель: он запускает по одному процессу CLI на каждый раунд основного цикла и
обменивается с ним запросом и ответом. Собственные инструменты CLI остаются
отключёнными — Claude Code запускается с отключёнными инструментами, Codex — в
песочнице только для чтения — потому что ревью ведёт OCR, а не CLI. При этом
используется ваш собственный вход в CLI, а автоматизированное использование
регулируется условиями обслуживания соответствующего поставщика.

**Настройка.** Выберите провайдера и модель, которую принимает CLI:

```bash
ocr config set provider claude-code
ocr config set model    claude-sonnet-5
```

```bash
ocr config set provider codex
ocr config set model    gpt-5.5
```

**Необязательные поля.** Оба необязательны и используются только
CLI-провайдерами:

| Поле | Значение |
|---|---|
| `providers.<name>.cli_path` | Путь к бинарнику CLI, переопределяющий имя, которое ищется в `PATH`. |
| `providers.<name>.cli_args` | Дополнительные аргументы, передаваемые при каждом запуске, в виде JSON-массива строк, например `'["--add-dir","/extra"]'`. |

```bash
ocr config set providers.claude-code.cli_path /opt/claude/bin/claude
ocr config set providers.claude-code.cli_args '["--add-dir","/extra"]'
```

**Ограничения частоты запросов.** Параллельные группы ревью означают параллельные
процессы CLI. `--concurrency N` (по умолчанию 8) ограничивает, сколько их
выполняется одновременно, и является рычагом, позволяющим не превышать лимиты
частоты запросов подписки — уменьшите его, если CLI начинает ограничивать
запросы.

`ocr llm test` печатает строку `Backend:` вместо `URL:`, потому что у
CLI-бэкенда нет URL:

```
Source:  provider:claude-code
Backend: claude (local CLI, uses its own login)
Model:   claude-sonnet-5
✓ Connection test successful
```

### Пользовательские провайдеры

Любое имя провайдера, которого нет в таблице выше, считается
пользовательским. Для него необходимо задать как минимум `url` и `protocol`
(`protocol` может принимать значения `anthropic`, `openai`,
`openai-responses`, `anthropic-bedrock`, `claude-cli` или `codex-cli`):

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

Пользовательскому провайдеру на протоколе `anthropic-bedrock` не нужен `url` —
хост определяется регионом, — и он принимает те же поля AWS, что и встроенный.
Так второй регион или профиль получает собственную запись:

```bash
ocr config set provider                                bedrock-eu
ocr config set custom_providers.bedrock-eu.protocol    anthropic-bedrock
ocr config set custom_providers.bedrock-eu.aws_region  eu-west-1
ocr config set custom_providers.bedrock-eu.aws_profile eu-profile
ocr config set custom_providers.bedrock-eu.model       eu.anthropic.claude-sonnet-4-6
```

Пользовательскому провайдеру на протоколе `claude-cli` или `codex-cli` не нужны
ни `url`, ни `api_key` — он запускает локальный CLI под вашим собственным входом
(см. [Claude Code CLI и Codex CLI](#claude-code-cli-and-codex-cli-no-api-key)
выше). Используйте пользовательскую запись только для запуска второй
конфигурации того же CLI; в остальных случаях выбирайте встроенный провайдер
`claude-code` или `codex`.

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

### Получение API-ключа из команды

Вместо того чтобы хранить ключ в файле конфигурации, параметр `api_key_cmd`
получает его во время выполнения из менеджера секретов (1Password, `pass`,
`gopass`, …). Ключом становится однострочный вывод команды в stdout с
отброшенными пробелами по краям. Тот же параметр доступен и для устаревшего
раздела `llm` — под именем `auth_token_cmd`.

```bash
ocr config set providers.anthropic.api_key_cmd "op read op://dev/anthropic/api-key"
```

Точно так же работает связка ключей вашей ОС — через команду, которая уже
входит в её состав, поэтому ключ хранится в Keychain или Secret Service, а не
в `config.json`:

```bash
# macOS Keychain
ocr config set providers.anthropic.api_key_cmd \
  "security find-generic-password -s ocr-anthropic -w"

# Linux (Secret Service: GNOME Keyring, KWallet, …)
ocr config set providers.anthropic.api_key_cmd \
  "secret-tool lookup service ocr-anthropic"
```

Приоритет: заданный `api_key` всегда имеет приоритет над командой (если заданы
оба, команда игнорируется и выводится предупреждение); иначе выполняется
`api_key_cmd`; и только если не задано ни то, ни другое, OCR возвращается к
переменной окружения провайдера.

Команда выполняется один раз за запуск `ocr` и должна завершиться успешно:
ненулевой код возврата, пустой вывод, многострочный вывод или вывод объёмом
больше 64 КиБ считаются ошибкой и прерывают работу (OCR никогда не переключается
на резервный вариант молча). Команда должна уложиться в 60 секунд, включая
время, которое вы тратите на ответ на запрос. Команда наследует stdin и stderr
вашего терминала, поэтому интерактивные запросы (pinentry, Touch ID) и
отображаются, и допускают ответ. Если команда оставляет после себя фоновую
службу, удерживающую её канал stdout (`gpg-agent`, запускаемая при первом
использовании служба `op`), учётные данные всё равно будут получены, но каждый
запуск `ocr` дополнительно ждёт 5 секунд, пока этот канал не закроется, —
перенаправьте вывод службы (`>/dev/null 2>&1`), чтобы избавиться от ожидания.

В Windows команда выполняется через `cmd.exe`, а не через `sh`, поэтому
команда, написанная для одной из этих оболочек, как правило, не переносится в
другую: `%VAR%` и `^` — метасимволы `cmd.exe`, а раскрытие `$VAR` и
экранирование через `\` там не действуют. Аргументы в кавычках передаются без
изменений, поэтому `op read "op://Private/My Vault/api-key"` работает как
написано.

Поскольку это значение выполняется как команда оболочки, `config.json`
считается доверенным вводом — он должен принадлежать вам и быть недоступен для
записи другим пользователям (OCR записывает его с правами `0600`).

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

По умолчанию OCR ограничивает промпт для каждого ревью файла 200 000 токенами
(`ocr scan` использует меньшее значение — 58 888). Если контекстное окно вашей
модели отличается, сохраните `max_tokens`:

```bash
ocr config set max_tokens 400000
```

Эта настройка применяется как к `ocr review`, так и к `ocr scan`. Используйте
`--max-tokens` для разового переопределения без изменения сохранённой
конфигурации:

```bash
ocr review --max-tokens 400000
ocr scan --max-tokens 120000
```

Флаг для конкретного запуска имеет приоритет над `max_tokens`; если не задано
ни то, ни другое, OCR использует встроенное значение по умолчанию из шаблона
задачи. Этот лимит действует только на **промпт**: предел вывода модели задаётся
отдельным параметром `MAX_COMPLETION_TOKENS` (по умолчанию `16384`), поэтому
увеличение `max_tokens` не расширяет бюджет вывода. Он также не зависит от
`--max-tokens-budget`, который ограничивает общее использование токенов за
запуск. Чтобы восстановить встроенное значение по умолчанию, используйте
`ocr config unset max_tokens`.

### Предустановка усилий ревью (effort)

Параметр `effort` определяет, сколько раундов основного цикла выполняется для
каждой группы файлов: `low` — 1 раунд, `medium` — 2 раунда (по умолчанию),
`high` — 3 раунда. Больше раундов — выше полнота находок, но больше времени и
токенов.

```bash
ocr config set effort high     # сохранить в конфигурации
ocr review --effort low        # только для этого запуска
ocr config unset effort        # вернуться к значению medium
```

Приоритет: флаг `--effort` > сохранённое значение `effort` > значение по
умолчанию `medium`.

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

### Аффинити сессии для кеширования промптов

OCR выводит ключ аффинити кеша промптов для каждого диалога с LLM,
ограниченный областью сессии ревью и задачи внутри неё
(`<ID сессии>-<тип задачи>-<хеш области>`). Кеши промптов сопоставляются
по префиксам, поэтому ключи на уровне диалога удерживают каждый растущий
диалог (например, цикл инструментов при ревью одного файла) на одном и том
же узле кеша, вместо того чтобы стягивать весь запуск к одному «горячему»
ключу; префикс с ID сессии позволяет сопоставлять журналы кеша на стороне
поставщика с записями `ocr session`.

Чтобы включить это, вставьте шаблонную переменную `{ocr_session_key}`
в значения `extra_headers` или `extra_body` там, где её ожидает ваш
поставщик — OCR подставляет ключ диалога в каждый запрос, а без такой
настройки не отправляет ничего:

```bash
# Через поле в теле запроса в стиле OpenAI (например, prompt_cache_key)
ocr config set providers.openai.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'

# Через HTTP-заголовок (например, x-session-affinity)
ocr config set custom_providers.my-gateway.extra_headers "x-session-affinity={ocr_session_key}"
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
