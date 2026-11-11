---
title: Qwen Code
sidebar:
  order: 3
---

Запускайте OCR в Qwen Code через существующий плагин Claude Code Marketplace
из этого репозитория. Установщик преобразует плагин и загружает команды ревью.

Инструкции проверены на Qwen Code 0.27.2 в Windows. Совместимость с отдельным
выпуском [Qwen Code 1.0 Preview](https://github.com/QwenLM/qwen-code/releases/tag/v1.0.0-preview)
пока не проверена.

## Требования

Установите Qwen Code и Git 2.41 или новее, затем установите OCR CLI:

```bash
npm install -g @alibaba-group/open-code-review
```

## Установка

Выполните в терминале:

```bash
qwen extensions install alibaba/open-code-review:open-code-review
qwen extensions list
```

После установки через CLI начните новый сеанс Qwen Code. Плагин использует
[готовые промпты команд](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code/commands)
из репозитория.

## Запуск ревью

Настройте и проверьте LLM, которую будет вызывать OCR:

```bash
ocr config provider
ocr config model
ocr llm test
```

Если встроенная команда Qwen Code `/review` включена, вызывайте OCR так:

```text
/open-code-review.review
/open-code-review.review --from main --to feature
/open-code-review.review --commit abc123
```

Qwen Code сохраняет встроенную `/review` и добавляет к конфликтующей команде OCR
префикс `open-code-review.`. Если встроенный навык ревью отключён, OCR может
использовать `/review`. Список загруженных имён команд доступен через `/help`
в текущем сеансе.

## Режим делегирования

Чтобы использовать модель и аутентификацию самого Qwen Code, выполните:

```text
/delegate-review
/delegate-review --from main --to feature
/delegate-review --commit abc123
```

OCR выбирает файлы и определяет правила через `ocr delegate preview` и
`ocr delegate rule`, а Qwen Code выполняет ревью. Требуется OCR CLI, но
отдельная настройка LLM для OCR не нужна.

Если имя `/delegate-review` уже занято другой командой, Qwen Code регистрирует
команду OCR как `/open-code-review.delegate-review`.

Обе команды могут автоматически применять исправления для принятых замечаний.

## Поиск в маркетплейсе

Добавьте репозиторий как источник, чтобы найти плагин на вкладке Discover:

```bash
qwen extensions sources add alibaba/open-code-review
```

Выполните `/extensions` в Qwen Code и откройте Discover.

## См. также

- [Режим делегирования](../delegate/)
- [Конфигурация](../configuration/)
- [Документация расширений Qwen Code](https://qwenlm.github.io/qwen-code-docs/en/users/extension/introduction/)
