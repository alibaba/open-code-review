---
title: Быстрый старт
sidebar:
  order: 3
---

Запустите первое код-ревью за несколько минут.

## Предварительные требования

- **Git ≥ 2.41**
- **Node.js ≥ 18**
- **API-ключ LLM** (не нужен при использовании [режима делегирования](../integrations/delegate/))

## Шаг 1. Установите CLI

```bash
npm install -g @alibaba-group/open-code-review
```

```bash
ocr version
```

> Другие способы установки описаны в разделе [Установка](../installation/).

## Шаг 2. Настройте LLM

> Если вы используете [режим делегирования](../integrations/delegate/) (например, внутри Claude Code), модель предоставляет основной агент. Переходите сразу к шагу 4.

```bash
ocr config provider
```

Выберите встроенного или пользовательского провайдера, введите API-ключ и укажите модель. Команда сохранит настройки в файл конфигурации и запустит `ocr llm test`, чтобы проверить подключение. Чтобы позже сменить модель:

```bash
ocr config model
```

### Альтернатива: неинтерактивная команда

В CI или среде без TUI задайте те же параметры напрямую через `ocr config set`:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

## Шаг 3. Проверьте подключение

```bash
ocr llm test
```

Если видите ошибку вроде `no valid LLM endpoint configured`, проверьте настройки из шага 2. Коды 401 / 403 означают, что токен неверный или истёк.

## Шаг 4. Запустите первое ревью

Перейдите в любой Git-репозиторий и выполните:

```bash
cd path/to/your-repo

# Все локальные изменения, включая новые файлы
ocr review

# Изменения в feature-branch относительно merge-base(main, feature-branch)
ocr review --from main --to feature-branch

# Изменения, внесённые коммитом abc123
ocr review --commit abc123
```

> Полный список флагов `ocr review`, включая параметры параллельного выполнения, формат вывода, режим `--audience` и контекст требований, приведён в [Справочнике CLI](../cli-reference/). Там же перечислены остальные подкоманды.

### Какие файлы попадут в ревью

```bash
ocr review --preview              # локальные изменения
ocr review -c abc123 --preview    # коммит abc123
```

### JSON для автоматизации

`--audience agent` скрывает интерфейс прогресса: в стандартный вывод (`stdout`) попадают только JSON и итоговая сводка. Такой формат подходит вызывающему агенту или CI-скрипту.

```bash
ocr review --format json --audience agent > review.json
```

## См. также

- [Установка](../installation/): все способы установки и каталог с данными OCR.
- [Конфигурация](../configuration/): все переменные окружения, ключи конфигурации и встроенные провайдеры.
- [Справочник CLI](../cli-reference/): все подкоманды, флаги и режимы вывода.
- [Правила ревью](../review-rules/): настройка того, что попадает в ревью.
- [Интеграции](../integrations/agent-skill/): встраивание OCR в Claude Code, Agent skill или CI.
- [FAQ](../faq/): известные ошибки и способы их устранения.
