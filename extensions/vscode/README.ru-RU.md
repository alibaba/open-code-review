<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja-JP.md">日本語</a> | <a href="README.ko-KR.md">한국어</a> | Русский
</p>

# Open Code Review (расширение для VS Code)

Расширение для проверки кода в VS Code, построенное на CLI [`open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review) (`ocr`). Оно воспроизводит поведение прототипа в WebView на Preact и переносит AI-проверку кода в редактор: запускайте проверку из боковой панели, следите за журналом в реальном времени и применяйте, отклоняйте или помечайте как ложное срабатывание каждый комментарий прямо в редакторе — обе стороны синхронизируются в обоих направлениях.

---

## Возможности

- **Три режима проверки**: изменения рабочей копии, сравнение веток (`--from` / `--to`) и отдельный коммит (`--commit`).
- **Предпросмотр файлов для проверки**: список изменённых файлов по текущему состоянию Git; нажмите на файл, чтобы посмотреть изменения в штатном diff-представлении.
- **Свой промпт для проверки**: при необходимости добавьте подсказку `--background` для текущей проверки.
- **Журнал в реальном времени**: вывод CLI транслируется по ходу проверки, отменить её можно в любой момент.
- **Результат и двусторонняя синхронизация**: по завершении в боковой панели появляются карточки комментариев, а в редакторе — CommentThread; действия «применить» / «отклонить» / «ложное срабатывание» синхронизируются с обеих сторон.
- **Пустое, отменённое и неуспешное состояния**: отдельные представления для случаев «проблем нет», отмены пользователем и сбоя CLI (сбой можно повторить, а текст ошибки берётся из ответа CLI).
- **Управление настройками**: просмотр и изменение настроек провайдера LLM прямо в расширении (запись через `ocr config set`).
- **Переключение моделей и проверка связи**: смена модели и проверка соединения с LLM из строки состояния.

---

## Требования

1. Установите CLI `ocr` глобально:

   ```bash
   npm i -g @alibaba-group/open-code-review
   ```

2. Настройте рабочего провайдера LLM (эндпоинт, API-ключ, модель). Это можно сделать напрямую через CLI или в представлении настроек расширения:

   ```bash
   ocr config set llm.url https://api.anthropic.com/v1/messages
   ocr config set llm.auth_token sk-...
   ocr config set llm.model claude-opus-4-6
   ocr config set llm.use_anthropic true
   ```

   Настройки записываются в `~/.opencodereview/config.json`.

---

## Разработка

### Окружение

- Node.js 18 и новее, менеджер пакетов — **Yarn** (в репозитории есть `yarn.lock`).
- VS Code 1.74 и новее.
- Доступный глобально CLI `ocr` (см. «Требования» выше): по сути расширение — это графический интерфейс к `ocr`.

### Запуск окружения разработки

```bash
cd extensions/vscode
yarn install      # установить зависимости
yarn watch        # сборка в режиме наблюдения (рекомендуется: пересобирает out/ при изменениях)
```

Затем откройте папку `extensions/vscode` в VS Code и нажмите **F5**, чтобы запустить Extension Development Host (конфигурация отладки лежит в `.vscode/launch.json`). В новом окне откройте проект с изменениями в Git — в панели действий появится значок Open Code Review, и можно запускать проверку.

> После правок кода: изменения WebView требуют **повторно открыть боковую панель** в окне хоста разработки (либо выполнить `Developer: Reload Webviews`); изменения Extension Host требуют **перезапуска сеанса отладки** (кнопка ⟳ на панели отладки или `Cmd+R` в окне хоста).

### Скрипты

```bash
yarn compile      # разовая сборка для разработки (webpack development)
yarn watch        # сборка в режиме наблюдения
yarn build        # продакшен-сборка (webpack production; выполняется автоматически перед упаковкой)
yarn test         # модульные тесты Jest
yarn lint         # ESLint
yarn package      # собрать .vsix для распространения (см. «Сборка релизного пакета»)
```

### Заметки по отладке

- **Двусторонний обмен сообщениями**: WebView и Extension Host общаются через `postMessage`, типы сообщений лежат в `extensions/frontend/src/shared/messages.ts`. Обе стороны проходят через `dispatch` / `handle` — с этого и стоит начинать отладку.
- **Вызов CLI**: все подкоманды `ocr` запускаются через `child_process.spawn` в `src/extension/services/CliService.ts`. `runRaw` отклоняет промис при ненулевом коде возврата CLI и включает текст `Error:` из stderr, что помогает разобраться со «сбоем проверки / сбоем подключения».
- **Чтение и запись настроек**: `ConfigService` читает `~/.opencodereview/config.json`, а запись делегирует `ocr config set`. Поля WebView в camelCase (например, `useAnthropic`), а на диске и в CLI — snake_case (например, `use_anthropic`); преобразование живёт в `src/extension/services/configParse.ts`.

---

## Сборка

### Только артефакты компиляции

```bash
yarn build        # продакшен-сборка (webpack production)
```

Артефакты: `out/extension.js` (Extension Host) и `out/webview.js` (SPA WebView).

### Сборка релизного пакета (.vsix)

```bash
yarn package      # = vsce package --no-yarn
```

Эта команда:

1. запускает `vscode:prepublish` → продакшен-сборку `yarn build`;
2. исключает исходники, тесты и файлы разработки согласно `.vscodeignore`;
3. создаёт `open-code-review-vscode-<version>.vsix` в текущем каталоге.

> Инструмент упаковки — `@vscode/vsce`, установленный как devDependency: глобальная установка и загрузка из сети не нужны. `--no-yarn` пропускает стандартную проверку дерева зависимостей npm в vsce (проект использует Yarn).

В релизный пакет попадает только необходимое для работы: `package.json`, `README.md`, `resources/icon.svg`, `out/extension.js`, `out/webview.js`.

### Локальная установка / проверка

```bash
code --install-extension open-code-review-vscode-<version>.vsix
```

Или в VS Code: панель расширений → `⋯` в правом верхнем углу → **Install from VSIX…** → выберите созданный файл `.vsix`.

> Для публикации в Marketplace используйте `vsce publish` (нужны аккаунт издателя и PAT); для повседневного распространения достаточно `.vsix` выше.

---

## Архитектура

Используется схема **Monolithic WebView + Thin Extension Host**:

- **WebView** — отдельно собираемое SPA на Preact из общего фронтенда `extensions/frontend/` (его же использует плагин для IntelliJ IDEA).
- Слой **Extension Host** тонкий: только вызовы CLI, файловая система, операции Git и комментарии в редакторе.
- Оба слоя общаются через `postMessage`, а общие типы TypeScript в `extensions/frontend/src/shared/` обеспечивают типобезопасность.

```
src/
└── extension/          Extension Host (Node.js): services / providers / commands
```

WebView и общие типы собираются из общего фронтенда `extensions/frontend/` (его же использует плагин для IntelliJ IDEA).

---

## Лицензия

Apache-2.0
