<div align="center">
  <a href="https://open-codereview.ai">
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

Подробнее на [официальном сайте](https://open-codereview.ai).

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

```bash
npm install -g @alibaba-group/open-code-review
```

После установки команда `ocr` доступна глобально.

Другие способы установки (скрипт установки, бинарный файл из GitHub Release, сборка из исходников) описаны в [руководстве по установке](https://open-codereview.ai/docs/installation).

#### Быстрый старт

**1. Настройте LLM**

Перед запуском ревью необходимо настроить LLM, если только вы не используете [режим делегирования](https://open-codereview.ai/docs/integrations/delegate).

```bash
ocr config provider          # Выбрать встроенного провайдера или добавить пользовательский
ocr config model             # Выбрать модель для активного провайдера
```

![Provider setup](imgs/providers.jpg)

Интерактивный UI проведёт вас через выбор провайдера, ввод API-ключа и настройку модели, после чего автоматически проверит подключение.

Настройка через CLI, переменные окружения, пользовательские провайдеры и другие расширенные параметры описаны в [руководстве по конфигурации](https://open-codereview.ai/docs/configuration).

**2. Запустите ревью**

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

# Режим делегирования — AI-агент сам выполняет ревью
# OCR отвечает за выбор файлов и разрешение правил; настройка LLM не требуется
ocr delegate preview
ocr delegate rule src/main.go src/handler.go
```

## Документация

Полная документация доступна на **[open-codereview.ai/docs](https://open-codereview.ai/docs)**:

- [Быстрый старт](https://open-codereview.ai/docs/quickstart) — установка и запуск первого ревью
- [Установка](https://open-codereview.ai/docs/installation) — все платформы и менеджеры пакетов
- [Справочник CLI](https://open-codereview.ai/docs/cli-reference) — все команды и флаги
- [Правила ревью](https://open-codereview.ai/docs/review-rules) — кастомизация правил ревью, фильтрация и таргетинг по путям
- [Конфигурация](https://open-codereview.ai/docs/configuration) — ключи конфигурации и переменные окружения
- [MCP-сервер](https://open-codereview.ai/docs/mcp) — расширение агента ревью внешними инструментами
- Интеграция с кодинг-агентами — встраивание OCR в Claude Code, Codex, Cursor и др.
  - [Skill](https://open-codereview.ai/docs/integrations/agent-skill) — установка как переиспользуемый навык агента
  - [Plugin](https://open-codereview.ai/docs/integrations/claude-code) — установка как плагин Claude Code / Codex / Cursor
  - [Режим делегирования](https://open-codereview.ai/docs/integrations/delegate) — агент ревьюит своей собственной LLM
- [Интеграция с CI/CD](https://open-codereview.ai/docs/cicd) — GitHub Actions, GitLab CI, GitFlic CI и Gerrit
- [Просмотр сессий](https://open-codereview.ai/docs/viewer) — просмотр и воспроизведение сессий ревью в браузере
- [Телеметрия](https://open-codereview.ai/docs/telemetry) — интеграция с OpenTelemetry для наблюдаемости
- [FAQ](https://open-codereview.ai/docs/faq) — частые вопросы и устранение неполадок

## Участие в разработке

Этот проект существует благодаря всем, кто вносит свой вклад. В [CONTRIBUTING.ru-RU.md](CONTRIBUTING.ru-RU.md) описаны настройка окружения разработки, рекомендации по коду и порядок отправки pull request'ов.

<a href="https://github.com/alibaba/open-code-review/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=alibaba/open-code-review" />
</a>

## Лицензия

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
