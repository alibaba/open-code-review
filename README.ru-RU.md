<div align="center">
  <a href="https://alibaba.github.io/open-code-review/">
    <img src="imgs/logo-core.svg" alt="OpenCodeReview logo" width="180" />
  </a>
  <h1>OpenCodeReview</h1>
</div>

<p align="center">
  <a href="https://trendshift.io/repositories/41087" target="_blank">
    <img src="https://trendshift.io/api/badge/trendshift/repositories/41087/weekly?language=Go" alt="alibaba%2Fopen-code-review | Trendshift" style="width: 320px; height: 70px;" width="320" height="70" />
  </a>
</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://deepwiki.com/alibaba/open-code-review"><img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg" /></a>
  <a href="https://www.bestpractices.dev/projects/13328"><img alt="OpenSSF Best Practices" src="https://img.shields.io/badge/OpenSSF-Silver-4C566A?style=flat-square" /></a>
</p>
<p align="center">
  <a href="#supported-platforms"><img alt="Windows" src="https://img.shields.io/badge/Windows-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="macOS" src="https://img.shields.io/badge/macOS-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="Linux" src="https://img.shields.io/badge/Linux-supported-blue.svg" /></a>
  <a href="#supported-agents"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Codex" src="https://img.shields.io/badge/Codex-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Cursor" src="https://img.shields.io/badge/Cursor-supported-blueviolet.svg" /></a>
</p>
<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja-JP.md">日本語</a> | <a href="README.ko-KR.md">한국어</a> | Русский
</p>

---

## Что такое Open Code Review?

Open Code Review — это CLI-инструмент для код-ревью на основе ИИ. Он появился как внутренний официальный ИИ-ассистент код-ревью Alibaba Group: за последние два года им воспользовались десятки тысяч разработчиков, и он выявил миллионы дефектов в коде. После тщательной проверки в огромных масштабах мы превратили его в open-source-проект для сообщества. Чтобы начать работу, достаточно настроить эндпоинт модели.

Инструмент читает git-диффы, отправляет изменённые файлы настраиваемой LLM через агента с поддержкой вызова инструментов (tool use) и генерирует структурированные ревью-комментарии с точностью до строки. Агент может читать полное содержимое файлов, искать по кодовой базе, заглядывать в другие изменённые файлы за контекстом и выполнять глубокое ревью — а не только давать поверхностные замечания по диффу. Помимо ревью диффов, `ocr scan` позволяет проверять файлы целиком — удобно для аудита незнакомой кодовой базы или каталогов без значимого диффа.

Подробнее на [официальном сайте](https://alibaba.github.io/open-code-review/).

![Highlights](imgs/highlights-en.png)

## Бенчмарк

> По сравнению с агентами общего назначения (Claude Code), Open Code Review при той же базовой модели достигает значительно более высоких показателей **Precision** и **F1**, потребляя лишь **~1/9 токенов** и выполняя ревью быстрее. При этом показатель Recall ниже, чем у агентов общего назначения — это осознанный компромисс в пользу точности и минимального шума.

Бенчмарк на основе реальных код-ревью: **50** популярных open-source-репозиториев, **200** реальных Pull Request, **10** языков программирования — перекрёстная валидация 80+ старшими инженерами (**1 505** размеченных дефектов).

| Метрика | Что измеряет | Почему важна |
|---------|-------------|--------------|
| **F1** | Гармоническое среднее precision и recall | Лучший единый показатель качества ревью |
| **Precision** | Доля найденных проблем, являющихся реальными дефектами | Выше = меньше ложных срабатываний |
| **Recall** | Доля реальных дефектов, которые были найдены | Выше = меньше пропущенных проблем |
| **Avg Time** | Время выполнения одного ревью | Влияет на задержки в CI-пайплайне |
| **Avg Token** | Суммарное потребление токенов за ревью | Прямо влияет на стоимость API |

![Benchmark](imgs/benchmark-en.png)

## Почему Open Code Review?

### Проблема агентов общего назначения

Если вы использовали для код-ревью агентов общего назначения, например Claude Code со Skills, вы наверняка сталкивались с этими болевыми точками:

- **Неполное покрытие** — на крупных ченджсетах агенты склонны «срезать углы»: выборочно проверяют часть файлов и пропускают остальные.
- **Дрейф позиций** — найденные проблемы часто не совпадают с реальным местом в коде: номера строк и ссылки на файлы «уезжают» от цели.
- **Нестабильное качество** — Skills, управляемые естественным языком, трудно отлаживать, и качество ревью заметно колеблется при небольших изменениях промпта.

Первопричина: чисто языковая архитектура не накладывает жёстких ограничений на процесс ревью.

### Ключевая идея: детерминированная инженерия × агент

Ключевая философия Open Code Review — сочетать детерминированную инженерию и агента так, чтобы каждый занимался тем, что у него получается лучше всего.

**Детерминированная инженерия — жёсткие гарантии**

Для тех шагов ревью, где *нельзя ошибаться*, корректность гарантирует инженерная логика, а не языковая модель:

- **Точный отбор файлов** — точно определяет, какие файлы нуждаются в ревью, а какие следует отфильтровать, гарантируя, что ни одно важное изменение не будет упущено.
- **Умный бандлинг файлов** — группирует связанные файлы в одну единицу ревью (например, `message_en.properties` и `message_zh.properties` объединяются вместе). Каждый бандл выполняется как суб-агент с изолированным контекстом — стратегия «разделяй и властвуй», которая сохраняет стабильность на очень больших ченджсетах и естественным образом поддерживает конкурентное ревью.
- **Тонкий матчинг правил** — сопоставляет правила ревью с характеристиками каждого файла, удерживая внимание модели сфокусированным и устраняя информационный шум у самого источника. По сравнению с чисто языковым управлением правилами матчинг правил на основе шаблонизатора стабильнее и предсказуемее.
- **Внешние модули позиционирования и рефлексии** — независимые модули позиционирования комментариев и рефлексии над комментариями системно повышают точность как расположения, так и содержания замечаний ИИ.

**Агент — динамические решения**

Сильные стороны агента сосредоточены там, где они важнее всего, — в динамических решениях и динамическом доборе контекста:

- **Промпты, заточенные под сценарий** — шаблоны промптов, глубоко оптимизированные под код-ревью: выше качество при меньшем расходе токенов.
- **Набор инструментов, заточенный под сценарий** — выведен из глубокого анализа трейсов вызовов инструментов на больших продакшен-данных, включая распределение частоты вызовов, долю повторных вызовов каждого инструмента и влияние новых инструментов на всю цепочку вызовов. В результате получился специализированный набор инструментов, который для код-ревью стабильнее и предсказуемее, чем универсальный агентский тулкит.

## Как использовать

### Предварительные требования

- **Git >= 2.41** — Open Code Review использует Git для генерации diff, поиска по коду и операций с репозиторием.

### CLI

#### Установка

**Через NPM (рекомендуется)**

```bash
npm install -g @alibaba-group/open-code-review
```

После установки команда `ocr` доступна глобально.

**Обновление**

Если установка выполнена через NPM, обновите вручную до последней версии:

```bash
npm install -g @alibaba-group/open-code-review@latest
```

Установка через NPM также по умолчанию проверяет новые версии в фоне и обновляется автоматически. Чтобы отключить автообновления, задайте `OCR_NO_UPDATE=1`.

Если вы устанавливали через install script или вручную скачанный бинарный файл, повторно запустите ту же команду установки/скачивания, чтобы заменить локальный бинарный файл последним релизом. Используйте `OCR_VERSION`, если нужно зафиксировать конкретный тег релиза.

**Из GitHub Release**

Установите свежий бинарный файл для вашей ОС/архитектуры одной командой (macOS / Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

Скрипт сам выбирает подходящий бинарный файл релиза, проверяет его контрольную сумму SHA-256 и устанавливает его как `ocr` в `/usr/local/bin`. Каталог установки можно переопределить через `OCR_INSTALL_DIR`, а версию релиза зафиксировать через `OCR_VERSION`:

```bash
OCR_INSTALL_DIR="$HOME/.local/bin" OCR_VERSION=v1.3.13 \
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh)"
```

<details>
<summary>Ручная загрузка (все платформы, включая Windows)</summary>

Скачайте бинарный файл для вашей платформы со страницы [GitHub Releases](https://github.com/alibaba/open-code-review/releases):

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (x86_64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (ARM64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (x86_64) — переместите ocr.exe в каталог из вашего PATH
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64) — переместите ocr.exe в каталог из вашего PATH
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

</details>

**Из исходников**

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### Быстрый старт

**1. Настройте LLM**

**Перед запуском ревью необходимо настроить LLM.**

OCR управляет конфигурацией LLM через единую систему **провайдеров (Provider)**. Множество популярных провайдеров встроено, также поддерживается добавление пользовательских провайдеров для подключения к приватным развёртываниям или другим совместимым эндпоинтам. Конфигурация хранится в `~/.opencodereview/config.json`.

**Вариант A: интерактивная настройка (рекомендуется)**

```bash
ocr config provider          # Выбрать встроенного провайдера или добавить пользовательский
ocr config model             # Выбрать модель для активного провайдера
```

![Provider setup](imgs/providers.jpg)

Интерактивный UI проведёт вас через выбор провайдера, ввод API-ключа и настройку модели, после чего автоматически проверит подключение.

Выполните `ocr llm providers`, чтобы увидеть все встроенные провайдеры. У встроенных провайдеров предустановлены URL API и протокол — достаточно указать API-ключ. Если соответствующая переменная окружения уже задана (например, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`), API-ключ будет подхвачен автоматически.

**Пользовательские провайдеры** также добавляются через интерактивный UI — потребуется указать имя, URL API, тип протокола (`anthropic` или `openai`) и API-ключ.

**Вариант B: настройка через CLI (для CI/CD и неинтерактивных сред)**

Используйте `ocr config set` для записи конфигурации провайдера напрямую — подходит для скриптов и автоматизации.

Использование встроенного провайдера:

```bash
ocr config set provider anthropic
ocr config set providers.anthropic.api_key your-api-key-here
ocr config set providers.anthropic.model claude-sonnet-4-6
```

Использование пользовательского провайдера (приватный шлюз или другой совместимый эндпоинт):

```bash
ocr config set provider my-gateway
ocr config set custom_providers.my-gateway.url https://my-llm-gateway.internal/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.api_key your-api-key-here
ocr config set custom_providers.my-gateway.model gpt-4o
```

> Для пользовательских провайдеров `url` и `protocol` обязательны. Поддерживаемые протоколы: `anthropic`, `openai`.

Дополнительные настройки:

| Ключ | Описание |
|------|----------|
| `providers.<name>.auth_header` | Заголовок аутентификации: `x-api-key` или `authorization` (по умолчанию: `authorization`) |
| `providers.<name>.extra_body` | Пользовательские JSON-поля, добавляемые в тело запроса |
| `providers.<name>.extra_headers` | Пары `key=value`, разделённые запятыми — пользовательские HTTP-заголовки для каждого запроса |
| `providers.<name>.models` | Список моделей для интерактивного выбора |

**`extra_headers` (необязательно):** добавляет пользовательские HTTP-заголовки к каждому запросу к LLM API. Полезно для прокси, шлюзов или корпоративных эндпоинтов, требующих дополнительных заголовков (например, ID организации, ID трассировки). Формат — пары `key=value`, разделённые запятыми. Значения с запятыми заключается в двойные кавычки:

```bash
ocr config set llm.extra_headers "X-Org-ID=org-123,X-Forwarded-For=\"1.2.3.4,5.6.7.8\""
```

Дополнительные заголовки также можно задать для отдельного провайдера:

```bash
ocr config set providers.anthropic.extra_headers "X-Org-ID=org-123"
```

**Переменные окружения (наивысший приоритет)**

Переменные окружения переопределяют настройки из файла конфигурации — удобно в CI/CD, где запись в конфиг-файл затруднена:

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

Также совместим с переменными окружения Claude Code (`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`) и разбирает `~/.zshrc` / `~/.bashrc` в поисках соответствующих export'ов.

> **Примечание для пользователей CC-Switch**: если вы используете [CC-Switch](https://github.com/farion1231/cc-switch) с включённым [routing service](https://www.ccswitch.io/en/docs?section=proxy&item=service), можно указать в `url` провайдера адрес прокси CC-Switch без дополнительной настройки:
> - Для провайдера **Claude**: установите `providers.anthropic.url` в `http://127.0.0.1:15721`
> - Для провайдера **Codex**: установите `url` соответствующего провайдера в `http://127.0.0.1:15721/v1`
> - `api_key` может быть любым, настройки `extra_body` продолжают действовать

**2. Проверьте подключение**

```bash
ocr llm test
```

**3. Запустите ревью**

```bash
cd your-project

# Режим рабочей копии — ревью всех staged, unstaged и untracked изменений
ocr review

# Диапазон веток — сравнение двух ref'ов
ocr review --from main --to feature-branch

# Один коммит
ocr review --commit abc123

# Возобновить прерванное ревью диапазона или одного коммита
ocr session list
ocr review --from main --to feature-branch --resume <session-id>

# Полнофайловое сканирование — ревью целых файлов вместо диффа (история git не нужна)
ocr scan                          # сканировать весь репозиторий
ocr scan --path internal/agent    # сканировать каталог или конкретные файлы
```

### Интеграция с кодинг-агентами

OCR легко встраивается в ИИ-агентов для разработки в виде slash-команды, позволяя выполнять код-ревью прямо в рабочем процессе агента.

#### Вариант 1: установка как Skill

Установите скилл OCR в свой проект через `npx`:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

Это установит скилл `open-code-review` из [реестра скиллов](skills/open-code-review/SKILL.md), который объясняет вашему кодинг-агенту, как вызывать `ocr` для код-ревью, классифицировать найденные проблемы по приоритету и при необходимости применять исправления.

#### Вариант 2: установка как плагин Claude Code

Для [Claude Code](https://docs.anthropic.com/en/docs/claude-code) установите плагин с командой, выполнив в Claude Code:

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

Это зарегистрирует slash-команду `/open-code-review:review`, которая запускает OCR и автоматически фильтрует и исправляет найденные проблемы.

#### Вариант 3: установка как плагин Codex

Для локального Codex установите плагин Open Code Review из этого репозитория:

```bash
codex plugin marketplace add alibaba/open-code-review
codex
/plugins
```

Для локального чекаута или форка:

```bash
codex plugin marketplace add .
codex
/plugins
```

Установите и включите `Open Code Review`, затем начните новый тред Codex и вызывайте плагин явно:

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

Это зарегистрирует Codex-скилл, запускающий локальный CLI OCR:

```bash
ocr review --audience agent
```

Эта интеграция не меняет внутренний LLM-бэкенд OCR и не требует настройки эндпоинта OpenAI Responses API для Codex. Самому OCR по-прежнему нужен установленный и настроенный CLI `ocr`, как описано в разделе про настройку CLI.

Руководство на корейском: [`plugins/open-code-review/CODEX.ko-KR.md`](plugins/open-code-review/CODEX.ko-KR.md)

#### Вариант 4: установка как плагин Cursor

Для [Cursor](https://www.cursor.com/) установите плагин Open Code Review из этого репозитория:

```
cursor-plugin marketplace add alibaba/open-code-review
```

Или добавьте маркетплейс вручную. В Cursor откройте `/plugins`, найдите `Open Code Review` и установите.

Для локального чекаута или форка:

```
cursor-plugin marketplace add .
```

После установки вызывайте плагин в Cursor:

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

Это зарегистрирует Cursor-скилл, запускающий локальный CLI OCR:

```bash
ocr review --audience agent
```

Эта интеграция не меняет внутренний LLM-бэкенд OCR. Самому OCR по-прежнему нужен установленный и настроенный CLI `ocr`, как описано в разделе про настройку CLI.

#### Вариант 5: просто скопировать файл команды

Для быстрой настройки без пакетных менеджеров достаточно скопировать файл команды, чтобы использовать slash-команду `/open-code-review` в Claude Code.

**На уровне проекта** (общий для команды через git):

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

**На уровне пользователя** (личное глобальное использование во всех проектах):

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

> **Требование**: для всех способов интеграции необходим установленный CLI `ocr` и настроенная LLM. См. разделы [Установка](#установка) и [Настройте LLM](#быстрый-старт) выше.

### Интеграция с CI/CD

OCR можно встроить в CI/CD-пайплайны для автоматического код-ревью Merge Request'ов / Pull Request'ов.

Базовая команда для интеграции с CI:

```bash
ocr review \
  --from "origin/main" \
  --to "<commit_sha>" \
  --format json
```

Флаг `--from` принимает в качестве базы ref ветки (например, `origin/main`) или SHA коммита, а `--to` — SHA коммита или ref ветки в качестве head. В CI-окружениях для `--to` рекомендуется использовать SHA коммита: это корректно обрабатывает PR/MR из форков, у которых исходная ветка отсутствует в remote `origin`.

Флаг `--format json` выводит машиночитаемый результат, удобный для разбора в CI-скриптах.

Каждое замечание содержит два структурированных поля, чтобы CI-интеграции могли сортировать, группировать, фильтровать замечания или блокировать сборку без повторного разбора текста комментария:

| Поле | Допустимые значения | Примечание |
|------|---------------------|------------|
| `category` | `bug`, `security`, `performance`, `maintainability`, `test`, `style`, `documentation`, `other` | Категория, к которой относится замечание. |
| `severity` | `critical`, `high`, `medium`, `low` | Важность замечания. |

В JSON-выводе эти два поля располагаются рядом с `content`, `start_line` и др. В терминале они отображаются перед комментарием как встроенный бейдж `[category · severity]`, цвет которого определяется важностью.

Примеры интеграции — в каталоге [`examples/`](./examples/):

- [`github_actions/`](./examples/github_actions/) — пример интеграции с GitHub Actions
- [`gitlab_ci/`](./examples/gitlab_ci/) — пример интеграции с GitLab CI
- [`gitflic_ci/`](./examples/gitflic_ci/) — пример интеграции с GitFlic CI

#### GitHub Action

Для GitHub в корне репозитория также поставляется готовая к использованию composite Action ([`action.yml`](./action.yml)). Вместо того чтобы вручную скриптовать `ocr review`, просто подключите её — она берёт на себя весь конвейер: checkout, установку OCR, запуск ревью, публикацию инлайн- и сводных комментариев, загрузку артефактов, а также повтор и идемпотентность:

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

Для воспроизводимости зафиксируйте тег версии или SHA коммита. Полный демо-воркфлоу, а также полный список входов, выходов и режимов публикации комментариев (закреплённая сводка, инкрементальная неразрушающая публикация) см. в каталоге [`examples/github_actions/`](./examples/github_actions/).

## Команды

| Команда | Алиас | Описание |
|---------|-------|----------|
| `ocr review` | `ocr r` | Запустить код-ревью на основе диффа |
| `ocr scan` | `ocr s` | Ревью целых файлов (дифф не нужен) |
| `ocr rules check <file>` | — | Показать, какое правило ревью применяется к пути файла |
| `ocr config provider` | — | Интерактивная настройка провайдера (встроенный, пользовательский или ручной) |
| `ocr config model` | — | Интерактивный выбор модели для активного провайдера |
| `ocr config set <key> <value>` | — | Установить значения конфигурации |
| `ocr config unset custom_providers.<name>` | — | Удалить пользовательского провайдера |
| `ocr llm test` | — | Проверить подключение к LLM |
| `ocr llm providers` | — | Показать список встроенных LLM-провайдеров |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | Показать сохранённые сессии ревью |
| `ocr session show <id>` | `ocr sessions show <id>` | Показать одну сессию и её checkpoint'ы по файлам |
| `ocr viewer` | `ocr v` | Запустить WebUI-просмотрщик сессий на `localhost:5483` |
| `ocr version` | — | Показать информацию о версии |

### Флаги `ocr review`

| Флаг | Короткая форма | По умолчанию | Описание |
|------|----------------|--------------|----------|
| `--repo` | — | текущий каталог | Корень git-репозитория |
| `--from` | — | — | Исходный ref (например, `main`) |
| `--to` | — | — | Целевой ref (например, `feature-branch`) |
| `--commit` | `-c` | — | Один коммит для ревью |
| `--exclude` | — | — | Паттерны в стиле gitignore через запятую для пропуска файлов; объединяются с excludes из rule.json |
| `--preview` | `-p` | `false` | Показать, какие файлы попадут в ревью, без запуска LLM |
| `--resume` | — | — | Возобновить предыдущую совместимую сессию ревью диапазона или одного коммита |
| `--format` | `-f` | `text` | Формат вывода: `text` или `json` |
| `--concurrency` | — | `8` | Максимум одновременных ревью файлов |
| `--timeout` | — | `10` | Таймаут конкурентной задачи в минутах |
| `--audience` | — | `human` | `human` (показывать прогресс) или `agent` (только сводка) |
| `--background` | `-b` | — | Необязательный контекст требований/бизнес-логики для ревью; при `--commit` автоматически заполняется из сообщения коммита |
| `--background-file` | `-B` | — | Необязательный контекст требований/бизнес-логики из Markdown-файла; при совместном использовании с `--background` встроенное значение идёт первым |
| `--model` | — | — | Выбрать или переопределить LLM-модель для этого ревью |
| `--rule` | — | — | Путь к пользовательским JSON-правилам ревью |
| `--max-tools` | — | встроенное | Максимум раундов вызова инструментов на файл; действует, только если больше значения шаблона по умолчанию |
| `--max-git-procs` | — | встроенное | Максимум одновременных git-подпроцессов |
| `--tools` | — | — | Путь к пользовательскому JSON-конфигу инструментов |

#### Возобновляемые ревью и сессии

Каждый запуск `ocr review` сохраняет локальный журнал сессии в
`~/.opencodereview/sessions/`. Успешный текстовый вывод остаётся сфокусированным
на результате ревью и не печатает session ID. Сохранённые сессии можно найти через
`ocr session list/show`, а `--format json` добавляет `session_id` в машиночитаемый
вывод. Если ревью диапазона или одного коммита было прервано, выберите сохранённую
сессию с тем же целевым ревью и возобновите её:

```bash
ocr session list
ocr session show <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

Возобновление намеренно строгое: поддерживаются только ревью диапазона веток и одного
коммита, но не ревью рабочей копии. Текущие `--from/--to` или `--commit` должны
совпадать с сохранённой сессией. `--preview` нельзя использовать вместе с `--resume`.

При `--format json` возобновлённый запуск включает:

- `session_id` — session ID текущего запуска
- `resume.resumed_from` — исходный session ID
- `resume.reused_files` — файлы, повторно использованные из сохранённых checkpoint'ов
- `resume.rerun_files` — файлы, заново проверенные в текущем запуске

### Флаги `ocr session`

| Команда | Флаг | По умолчанию | Описание |
|---------|------|--------------|----------|
| `ocr session list` | `--repo` | текущий каталог | Репозиторий, для которого нужно показать сессии |
| `ocr session list` | `--json` | `false` | Вывести сводки сессий в JSON |
| `ocr session list` | `--limit` | `20` | Ограничить количество сессий; `0` означает без ограничения |
| `ocr session show <id>` | `--repo` | текущий каталог | Репозиторий, сессию которого нужно посмотреть |
| `ocr session show <id>` | `--json` | `false` | Вывести метаданные сессии и элементы по файлам в JSON |

### Флаги `ocr scan`

`ocr scan` проверяет целые файлы, а не дифф — удобно для аудита незнакомой кодовой базы, предмиграционного сканирования или любого каталога без значимого диффа. Работает и в каталогах без git (используется обход файловой системы с учётом `.gitignore`).

| Флаг | Короткая форма | По умолчанию | Описание |
|------|----------------|--------------|----------|
| `--path` | — | весь репозиторий | Каталоги/файлы для сканирования через запятую |
| `--exclude` | — | — | Паттерны в стиле gitignore через запятую для пропуска файлов; объединяются с excludes из rule.json |
| `--preview` | `-p` | `false` | Показать список файлов для сканирования без запуска LLM |
| `--max-tokens-budget` | — | `0` (без ограничений) | Ограничить суммарное потребление токенов; при превышении диспетчеризация прекращается |
| `--no-plan` | — | `false` | Пропустить предварительное планирование по файлам |
| `--no-dedup` | — | `false` | Пропустить дедупликацию похожих комментариев в рамках батча |
| `--no-summary` | — | `false` | Пропустить сводку на уровне проекта |
| `--batch` | — | `by-language` | Стратегия батчинга: `none`, `by-language` или `by-directory` |
| `--format` | `-f` | `text` | Формат вывода: `text` или `json` (JSON включает поле `project_summary`) |
| `--concurrency` | — | `8` | Максимум одновременных сканирований файлов |
| `--rule` | — | — | Путь к пользовательским JSON-правилам ревью |
| `--repo` | — | текущий каталог | Корень репозитория или каталога для сканирования |

Перед каждым запуском `ocr scan` выводит приблизительную оценку стоимости в токенах. Используйте `--preview`, чтобы сначала посмотреть список файлов, и `--max-tokens-budget`, чтобы ограничить расход на больших репозиториях.

## Примеры

```bash
# Интерактивная настройка провайдера и модели
ocr config provider
ocr config model
ocr llm providers

# Удалить пользовательского провайдера
ocr config unset custom_providers.my-gateway

# Показать, какие файлы попадут в ревью (без вызовов LLM)
ocr review --preview
ocr review -c abc123 -p

# Ревью изменений рабочей копии с настройками по умолчанию
ocr review

# Ревью диффа веток с заданной конкурентностью
ocr review --from main --to my-feature --concurrency 4

# Ревью конкретного коммита с подробным JSON-выводом
ocr review --commit abc123 --format json --audience agent

# Возобновить прерванное ревью диапазона или одного коммита
ocr session list
ocr session show <session-id>
ocr review --from main --to my-feature --resume <session-id>
ocr review --commit abc123 --resume <session-id>

# Выбрать или переопределить модель для этого ревью
ocr review --model claude-opus-4-6
ocr review --commit abc123 --model claude-sonnet-4-6

# Передать контекст требований для более прицельного ревью
ocr review --background "Добавляем rate limiting в API логина"

# Передать контекст требований из Markdown-файла
ocr review --background-file ./docs/my_business_context.md

# Совместить встроенный контекст с локальным файлом контекста (используются оба)
ocr review --background "Фокус на аутентификации" --background-file ./docs/my_business_context.md

# Использовать собственные правила ревью
ocr review --rule /path/to/my-rules.json

# Посмотреть, какое правило применяется к файлу
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# Полнофайловое сканирование: сначала просмотреть список файлов (без вызовов LLM)
ocr scan --preview

# Сканировать весь репозиторий, ограничив расход ~500k токенов
ocr scan --max-tokens-budget 500000

# Сканировать подкаталог, пропустив сгенерированные/тестовые файлы
ocr scan --path internal --exclude '**/*_test.go,**/generated/**'

# Сканировать каталог без git с JSON-выводом (включает project_summary)
ocr scan --repo /path/to/plain/dir --format json

# Самое быстрое сканирование: пропустить планирование, дедупликацию и сводку проекта
ocr scan --no-plan --no-dedup --no-summary

# Открыть историю сессий ревью в браузере
ocr viewer
ocr viewer --addr :3000
```

### Безопасность viewer'а

Viewer отдаёт содержимое сессионных JSONL-файлов (сообщения запросов к LLM и ответы) по HTTP. На каждый запрос применяется allowlist по заголовку Host: loopback-имена (`localhost`, `127.0.0.0/8`, `::1`) и конкретный хост привязки разрешены всегда. Wildcard-привязки (`--addr :3000`, `--addr 0.0.0.0:3000`) и прочие не-loopback имена хостов нужно добавлять через переменную окружения `OCR_VIEWER_ALLOWED_HOSTS` (через запятую):

```bash
OCR_VIEWER_ALLOWED_HOSTS=review.internal,ocr.lan ocr viewer --addr :3000
```

Это блокирует атаки DNS rebinding на локальный viewer.

## Правила ревью

OCR разрешает правила ревью по цепочке приоритетов из четырёх уровней. На каждом уровне действует принцип «первое совпадение побеждает»: если путь файла совпал с паттерном, используется это правило; иначе поиск продолжается на следующем уровне.

| Приоритет | Источник | Путь | Описание |
|-----------|----------|------|----------|
| 1 (высший) | Флаг `--rule` | Путь, указанный пользователем | Явное переопределение из CLI |
| 2 | Конфиг проекта | `<repoDir>/.opencodereview/rule.json` | Правила уровня проекта, можно коммитить в git |
| 3 | Глобальный конфиг | `~/.opencodereview/rule.json` | Личные настройки пользователя |
| 4 (низший) | Системные по умолчанию | Встроенный `system_rules.json` | Встроенные правила для распространённых языков и типов файлов |

### Формат файла правил

Уровни 1–3 используют один и тот же JSON-формат:

```json
{
  "rules": [
    {
      "path": "force-api/**/*.java",
      "rule": "Все новые методы должны проверять обязательные параметры на null",
      "merge_system_rule": true
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Проверять SQL на риски инъекций, ошибки в параметрах и незакрытые теги"
    }
  ]
}
```

- `path` поддерживает рекурсивное сопоставление `**` и расширение фигурных скобок `{java,kt}`.
- `merge_system_rule` необязателен. Если указано `true`, совпавшее встроенное системное правило объединяется с этим пользовательским правилом.
- Внутри каждого уровня правила проверяются в порядке объявления — побеждает первое совпадение.
- Если файл правил не существует, он молча пропускается.

**Поле `rule` поддерживает как встроенный текст, так и пути к файлам.** Система определяет тип автоматически:

1. Если значение содержит переносы строк → **встроенный текст** (многострочные правила никогда не считаются путями).
2. Если значение — одна строка, без пробелов, и заканчивается на `.md` / `.txt` / `.markdown` → **путь к файлу**.
   - Абсолютные пути (начинающиеся с `/`) используются напрямую.
   - Относительные пути проверяются в корне проекта. Выход за пределы директории (например, `../../etc/passwd.md`) блокируется. Если не найдены — выводится `[WARN]` и правило очищается (без fallback на inline).
   - Файл должен пройти проверку: допустимое расширение, ≤ 512 KB, цель симлинка также должна иметь допустимое расширение. При ошибке проверки правило очищается.
3. Иначе → **встроенный текст**.

```json
{
  "rules": [
    {
      "path": "**/*mapper*.xml",
      "rule": "docs/sql-rules.md"
    },
    {
      "path": "**/*.java",
      "rule": "Always check for null safety and resource leaks"
    },
    {
      "path": "**/*.go",
      "rule": "shared/go-concurrency.md"
    },
    {
      "path": "**/*.py",
      "rule": "/Users/me/team-rules/python.md"
    }
  ]
}
```

- `docs/sql-rules.md` — относительный путь, загружается из `<project>/docs/sql-rules.md`.
- `Always check for null safety…` — встроенная строка, используется напрямую.
- `shared/go-concurrency.md` — относительный путь, аналогично.
- `/Users/me/team-rules/python.md` — абсолютный путь, используется напрямую.

> Абсолютные пути могут указывать на файлы вне директории проекта — это сделано намеренно. `rule.json` пишут мейнтейнеры проекта, это доверенный ввод. Команды могут хранить общие правила по единому пути (например, `/opt/company-rules/`) и не копировать их в каждый проект.

### Фильтрация путей

Файлы правил также поддерживают поля `include` и `exclude`, управляющие тем, какие файлы попадают в область ревью:

```json
{
  "rules": [
    {"path": "**/*.java", "rule": "Проверять null-безопасность"}
  ],
  "include": ["src/main/**/*.java", "lib/**/*.kt"],
  "exclude": ["**/generated/**", "vendor/**"]
}
```

**Приоритет решений фильтра (от высшего к низшему):**

| Шаг | Условие | Результат |
|-----|---------|-----------|
| 1 | Файл бинарный | Исключён |
| 2 | Путь совпадает с пользовательским паттерном `exclude` | Исключён |
| 3 | Расширение файла не входит в список поддерживаемых | Исключён |
| 4 | `include` настроен и путь совпадает | **В ревью** (шаг 5 пропускается) |
| 5 | Путь совпадает со встроенным паттерном исключения по умолчанию (тестовые файлы и т. п.) | Исключён |
| 6 | Ничего из перечисленного | В ревью |

**Как это работает:**

- `include` и `exclude` следуют той же цепочке приоритетов, что и правила ревью (`--rule` > конфиг проекта > глобальный конфиг). Действует **целиком самый приоритетный уровень, на котором include/exclude настроены** — паттерны разных уровней не объединяются.
- `exclude` всегда сильнее `include` — файл, совпавший с обоими, исключается.
- `include` работает как **обход встроенных паттернов исключения по умолчанию** (например, тестовых файлов), а не как эксклюзивный allowlist: файлы, не совпавшие ни с одним паттерном `include`, всё равно обычным образом проходят проверки фильтра по умолчанию.
- Синтаксис паттернов: поддерживаются рекурсивное сопоставление `**`, односегментное `*` и расширение фигурных скобок `{a,b}`. Сопоставление регистронезависимое.

**Встроенные паттерны исключения по умолчанию** (отфильтровывают тестовые файлы и т. п. — можно переопределить через `include`):

```
**/*_test.go, **/*Test.java, **/*Tests.java, **/*_test.rs,
**/*.test.{js,jsx,ts,tsx}, **/*.spec.{js,jsx,ts,tsx}, **/__tests__/**,
**/src/test/java/**/*.java, **/src/test/**/*.kt,
**/test/**/*_test.py, **/tests/**/*_test.py, **/*_test.py,
**/*_spec.rb, **/spec/**/*_spec.rb, **/oh_modules/**
```

## Справочник по конфигурации

Файл конфигурации: `~/.opencodereview/config.json`

| Ключ | Тип | Пример |
|------|-----|--------|
| `provider` | string | `anthropic` \| `openai` \| `dashscope` \| `deepseek` \| `z-ai` |
| `providers.<name>.api_key` | string | API-ключ провайдера |
| `providers.<name>.url` | string | Переопределение base URL провайдера |
| `providers.<name>.protocol` | string | `anthropic` \| `openai` |
| `providers.<name>.model` | string | Имя модели провайдера |
| `providers.<name>.models` | array | Необязательный список моделей для интерактивного выбора |
| `providers.<name>.auth_header` | string | `x-api-key` \| `authorization` |
| `providers.<name>.extra_body` | object | JSON-объект, добавляемый в каждое тело запроса |
| `providers.<name>.timeout_sec` | integer | Таймаут HTTP-запроса в секундах, по умолчанию `300` |
| `providers.<name>.extra_headers` | string | HTTP-заголовки `key=value` через запятую |
| `custom_providers.<name>.*` | — | Те же поля, что и `providers.<name>.*`, включая необязательное `models` |
| `llm.url` | string | `https://api.openai.com/v1/chat/completions` |
| `llm.auth_token` | string | `sk-xxxxxxx` |
| `llm.auth_header` | string | Только для Anthropic: `x-api-key` \| `authorization` |
| `llm.extra_body` | object | JSON-объект, добавляемый в каждое тело запроса |
| `llm.timeout_sec` | integer | Таймаут HTTP-запроса в секундах, по умолчанию `300` |
| `llm.extra_headers` | string | HTTP-заголовки `key=value` через запятую |
| `llm.model` | string | `claude-opus-4-6` |
| `llm.use_anthropic` | boolean | `true` \| `false` |
| `mcp_servers.<name>.command` | string | Команда для запуска MCP-сервера |
| `mcp_servers.<name>.args` | array | Аргументы командной строки для MCP-сервера |
| `mcp_servers.<name>.env` | array | Переменные окружения в формате `KEY=VALUE` |
| `mcp_servers.<name>.tools` | array | Разрешённые имена инструментов (пусто = все инструменты) |
| `mcp_servers.<name>.setup` | string | Команда настройки перед запуском сервера |
| `language` | string | Любое название языка, например `English`, `Chinese` (по умолчанию: `English`) |
| `telemetry.enabled` | boolean | `true` \| `false` |
| `telemetry.exporter` | string | `console` \| `otlp` |
| `telemetry.otlp_endpoint` | string | Адрес OTLP-коллектора |
| `telemetry.content_logging` | boolean | Включать промпты в телеметрию |

Переменные окружения имеют приоритет над файлом конфигурации.

### MCP-сервер

Open Code Review поддерживает серверы [Model Context Protocol (MCP)](https://modelcontextprotocol.io/), позволяя агенту ревью использовать внешние инструменты во время проверки кода через stdio-транспорт.

Настройка MCP-серверов через CLI:

```bash
# Добавить MCP-сервер
ocr config set mcp_servers.<name>.command <command>
ocr config set mcp_servers.<name>.args '["arg1","arg2"]'
ocr config set mcp_servers.<name>.env '["KEY=VALUE"]'
ocr config set mcp_servers.<name>.tools '["tool_name"]'
ocr config set mcp_servers.<name>.setup '<setup command>'

# Удалить MCP-сервер
ocr config unset mcp_servers.<name>
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `command` | Да | Исполняемая команда для запуска MCP-сервера |
| `args` | Нет | Аргументы командной строки для сервера |
| `env` | Нет | Переменные окружения в формате `KEY=VALUE` |
| `tools` | Нет | Разрешённые имена инструментов; если пусто — доступны все инструменты сервера |
| `setup` | Нет | Shell-команда для выполнения перед запуском сервера (например, построение индекса) |

> **Примечание:** Если имя MCP-инструмента конфликтует со встроенным инструментом, он будет пропущен с предупреждением. Таймаут команды `setup` составляет 5 минут.

**Пример: добавление [CodeGraph](https://github.com/nicholasgasior/codegraph) для усиления анализа структуры кода**

```bash
ocr config set mcp_servers.codegraph.command codegraph
ocr config set mcp_servers.codegraph.args '["serve","--mcp"]'
ocr config set mcp_servers.codegraph.tools '["codegraph_explore"]'
ocr config set mcp_servers.codegraph.setup 'codegraph init && codegraph index'
```

### Переменные окружения

| Переменная | Назначение |
|------------|------------|
| `OCR_LLM_URL` | URL эндпоинта LLM API |
| `OCR_LLM_TOKEN` | API-ключ / токен авторизации |
| `OCR_LLM_AUTH_HEADER` | Заголовок авторизации Anthropic (`x-api-key` или `authorization`) |
| `OCR_LLM_EXTRA_HEADERS` | HTTP-заголовки `key=value` через запятую |
| `OCR_LLM_MODEL` | Имя модели |
| `OCR_LLM_TIMEOUT` | Таймаут HTTP-запроса в секундах (переопределяет `timeout_sec` из файла конфигурации) |
| `OCR_USE_ANTHROPIC` | `true` = Anthropic, `false` = OpenAI |


## Телеметрия

Интеграция с OpenTelemetry для наблюдаемости (спаны, метрики). По умолчанию выключена.

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

Установите `telemetry.content_logging`, чтобы включать промпты и ответы LLM в экспортируемые данные.

**Выбор протокола:** Переменная окружения `OTEL_EXPORTER_OTLP_PROTOCOL` определяет протокол экспорта:

| Значение | Транспорт | Описание |
|---|---|---|
| `grpc` (по умолчанию) | gRPC | Порт по умолчанию 4317 |
| `http/protobuf` | HTTP | Порт по умолчанию 4318 |

**Формат endpoint:** `telemetry.otlp_endpoint` принимает базовый URL в формате `host:port` или `http://host:port` без компонента пути. SDK автоматически добавляет путь сигнала (например, `/v1/traces`) в соответствии со [спецификацией OTLP](https://opentelemetry.io/docs/specs/otlp/#otlphttp-request).

## Участие в разработке

Этот проект существует благодаря всем, кто вносит свой вклад. В [CONTRIBUTING.ru-RU.md](CONTRIBUTING.ru-RU.md) описаны настройка окружения разработки, рекомендации по коду и порядок отправки pull request'ов.

<a href="https://github.com/alibaba/open-code-review/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=alibaba/open-code-review" />
</a>

## Лицензия

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
