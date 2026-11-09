---
title: GitHub Copilot CLI
sidebar:
  order: 3
---

Запускайте OCR от начала до конца внутри
[GitHub Copilot CLI](https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli)
через тот же переносимый [навык агента](../agent-skill/), который используют
другие агенты программирования: проверяйте diff, классифицируйте замечания и
исправляйте те, что стоит принять. Интеграция представляет собой навык
«скопировал и готово»: он не устанавливает хуки, не добавляет обязательных
инструкций и не меняет поведение репозитория.

## Что поставляется в репозитории

Рабочий процесс находится в не зависящем от агента навыке по пути
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md).
Манифест плагина Copilot CLI
[`plugins/open-code-review/plugin.json`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/plugin.json)
(Agent Plugins 1.0) упаковывает встроенные навыки для `copilot plugin install`.
Copilot CLI по-прежнему читает навыки из стандартных расположений, поэтому
копирование каталога навыка тоже работает.

## Установка

Установите плагин (рекомендуется):

```bash
copilot plugin install alibaba/open-code-review:plugins/open-code-review
```

Устанавливаются навыки `open-code-review` и `open-code-review-delegate`.
Удалить плагин можно командой `copilot plugin uninstall open-code-review`.

Установщик Copilot CLI сейчас предупреждает, что прямая установка плагинов
(repo/path/URL) устарела в пользу установки через marketplace; запись в marketplace
может позже сделать `plugin@open-code-review` рекомендуемой формой.

### Ручной вариант: скопируйте навык

Скопируйте навык в каталог, который обнаруживает Copilot CLI. **На уровне
проекта** (закоммитьте его, чтобы навык был доступен всей команде):

```bash
mkdir -p .github/skills/open-code-review
curl -o .github/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

**На уровне пользователя** (доступен в каждом проекте на компьютере):

```bash
mkdir -p ~/.copilot/skills/open-code-review
curl -o ~/.copilot/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

Copilot CLI также обнаруживает `.agents/skills/` и `.claude/skills/` в проекте;
навыки проекта имеют приоритет над личными навыками с тем же именем. Проверьте,
что загружено, командой `/skills list` внутри сеанса (или `copilot skill list`
вне его). Скопированные вручную навыки вступают в силу после `/skills reload` или
в новом сеансе; установленные плагины обновляются переустановкой (повторный
`copilot plugin install`) или командой `copilot plugin update`.

## Использование

Укажите навык явно через `/open-code-review` или просто опишите задачу —
Copilot выберет навык по его описанию:

```
/open-code-review
/open-code-review review this PR against main
/open-code-review focus on race conditions in commit abc123
```

| Вы говорите | Что запускает навык |
| --- | --- |
| «проверь мои изменения» | `ocr review --audience agent` (индексированные, неиндексированные и неотслеживаемые изменения) |
| «проверь этот PR» | `ocr review --from main --to <branch>` |
| «проверь коммит abc123» | `ocr review --commit abc123` |

Навык управляет `ocr` с собственной настроенной моделью и сообщает замечания,
сгруппированные по серьёзности. Если вы не хотите настраивать отдельную LLM для
`ocr`, попробуйте [режим делегирования](../delegate/) — основной агент (Copilot)
предоставляет модель, а `ocr` только выбирает файлы и разрешает правила.

### Необязательно: сделайте ревью привычкой

Чтобы этап ревью выполнялся автоматически, а не по запросу, добавьте одно
правило в `.github/copilot-instructions.md` или `AGENTS.md` — выберите один
файл, потому что Copilot объединяет файлы инструкций без приоритета:

```markdown
Before reporting a coding task complete, run `ocr review --audience agent`.
Fix all `critical` and `high` findings first.
```

Устанавливать такое правило или нет — решает каждый проект; этот репозиторий его
не навязывает.

## Неинтерактивный запуск

```bash
copilot -p "Review the current branch with /open-code-review and report the findings." \
  --allow-tool='shell(ocr:*)'
```

- `--allow-tool='shell(ocr:*)'` заранее одобряет только shell-команды `ocr` — он не
  разрешает редактирование файлов. В неинтерактивных сеансах (`-p`) действия,
  требующие подтверждения, отклоняются, а не приостанавливаются; интерактивные
  сеансы вместо этого ждут подтверждения. Чтобы Copilot применял исправления,
  запускайте в интерактивном режиме (или намеренно расширьте список разрешённых
  инструментов на инструменты редактирования).
- В недоверенных папках неинтерактивные сеансы могут пропускать навыки уровня
  проекта (`.github/skills/`); Copilot CLI v1.0.107+ предлагает `--trust-folder`,
  чтобы доверять папке в рамках одного сеанса.
- При включённой песочнице Copilot `ocr` нужен сетевой доступ к вашему
  эндпоинту LLM, а также доступ на чтение и запись к `~/.opencodereview/`
  (там хранится состояние сеанса).
- `--output-format json` выводит события сеанса Copilot в формате JSONL; сам
  результат ревью — это JSON от `ocr`, получаемый явным запуском
  `ocr review --format json --audience agent`.

Для детерминированных проверок в CI пропустите агента и вызывайте `ocr`
напрямую — см. [CI/CD](../cicd/).

## Примечания

- **Никаких хуков — намеренно.** Copilot CLI может блокировать `git commit` или
  `git push` с помощью хуков `preToolUse`, но OCR такой хук не поставляет: ревью
  запускает LLM (это медленно и требует учётных данных, которых у участника
  может не быть), проверяемая рабочая область — не тот коммит, который
  создаётся, а хук, который при сбое не блокирует, обещает контроль, который он
  не может гарантировать. Добавляйте собственный хук, только если принимаете
  эти компромиссы.
- **Файлы промптов только для IDE.** Copilot CLI не читает
  `.github/prompts/*.prompt.md`; поддерживаемый интерфейс — навык.
- **Замечания не меняют коды выхода.** `ocr` завершается с кодом 0 после
  завершённого ревью, даже если нашёл проблемы — ориентируйтесь на серьёзность
  из `--format json`, а не на код завершения.

## Устранение неполадок

- **Навык отсутствует в списке** — проверьте путь (`~/.copilot/skills/` или
  `<repo>/.github/skills/`), затем выполните `/skills reload` или начните новый
  сеанс.
- **`ocr: command not found`** — установите командой
  `npm i -g @alibaba-group/open-code-review`.
- **Ревью завершается ошибкой LLM** — настройте провайдера и проверьте его
  командой `ocr llm test`; см. [Конфигурация](../configuration/).
- **Copilot продолжает запрашивать подтверждение `ocr`** — разрешите
  `shell(ocr:*)` через `--allow-tool` или одобрите его навсегда в
  соответствующем запросе.
