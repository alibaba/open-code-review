---
title: Установка
sidebar:
  order: 4
---

Установить CLI `ocr` можно шестью способами.

## npm (рекомендуется)

#### Установка

```bash
npm install -g @alibaba-group/open-code-review
```

Закрепить конкретную версию:

```bash
npm install -g @alibaba-group/open-code-review@<version>
```

#### Обновление

При установке через npm `ocr` обновляется автоматически. Статический бинарник
в этом механизме не участвует. При каждом запуске обёртка проверяет реестр npm
в фоне и устанавливает найденное обновление, не прерывая текущее ревью.
Интервал между проверками составляет 18 минут. Изменить его можно через
`OCR_UPDATE_INTERVAL` (в минутах).

Чтобы отключить автообновления, задайте `OCR_NO_UPDATE` любым непустым значением:

```bash
export OCR_NO_UPDATE=1
```

#### Удаление

```bash
npm uninstall -g @alibaba-group/open-code-review
```

## Homebrew (macOS / Linux)

```bash
brew install open-code-review
```

Формула собирает `ocr` из исходников и устанавливает бинарник.

Для обновления:

```bash
brew upgrade open-code-review
```

## MacPorts (macOS)

```bash
sudo port install open-code-review
```

Порт собирает `ocr` из исходников и устанавливает бинарник.

Для обновления:

```bash
sudo port upgrade open-code-review
```

## Скрипт установки (curl | sh)

Установщик загружает бинарник из GitHub Release и проверяет его контрольную
сумму. Он удобен для базовых образов CI и систем без графического интерфейса:

```bash
curl -fsSL https://open-codereview.ai/install.sh | sh
```

Скрипт учитывает три переменные окружения:

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `OCR_INSTALL_DIR` | `/usr/local/bin` | Куда положить бинарник `ocr`. |
| `OCR_VERSION` | последний релиз | Закрепить конкретный тег релиза (например `v1.2.3`). |
| `OCR_GITHUB_MIRROR` | не задана | Скачивать бинарник релиза и его контрольную сумму через зеркало GitHub (например `gh-proxy.com`). |

Скрипт поддерживает `darwin` и `linux` на `amd64` / `arm64`.

#### Использование зеркала GitHub

В некоторых регионах доступ к GitHub может быть медленным. Задайте `OCR_GITHUB_MIRROR` как домен зеркала, чтобы загружать бинарник релиза и его контрольную сумму через него:

```bash
export OCR_GITHUB_MIRROR='YOUR_MIRROR_DOMAIN'
```

Значение должно быть «голым» доменом — без схемы `https://` и без завершающего слэша (`gh-proxy.com`, а не `https://gh-proxy.com/`). Оно используется как зеркало с *префиксом пути*: бинарник загружается с
`https://<зеркало>/github.com/alibaba/open-code-review/releases/download/<версия>/…`.
Зеркала с подменой домена (например, переписывающие `github.com` на `hub.example.org`) не подходят под эту форму — используйте зеркало с префиксом пути.

Зеркало покрывает и бинарник релиза, и его контрольную сумму `sha256sum.txt`. Разрешение версии (когда `OCR_VERSION` не задана) по-прежнему обращается к GitHub API напрямую, а не к зеркалу. Чтобы полностью пропустить разрешение версии, закрепите версию:

```bash
export OCR_VERSION='v1.2.3'
```

> **Примечание по безопасности:** Зеркало — это сторонний сервис, поэтому при заданной `OCR_GITHUB_MIRROR` и бинарник, и его `sha256sum.txt` загружаются с зеркала. Это значит, что вредоносное зеркало может отдать подменённый бинарник вместе с подходящей контрольной суммой; в режиме зеркала гарантия целостности не действует. Если зеркало не заслуживает доверия, сверьте загруженный файл с оригинальным `sha256sum.txt` на [странице релизов](https://github.com/alibaba/open-code-review/releases).

В Windows с PowerShell 5.1 или новее запустите PowerShell-установщик:

```powershell
irm https://open-codereview.ai/install.ps1 | iex
```

Установщик учитывает те же переменные `OCR_INSTALL_DIR`, `OCR_VERSION` и
`OCR_GITHUB_MIRROR` (через `$env:OCR_INSTALL_DIR` /
`$env:OCR_VERSION` / `$env:OCR_GITHUB_MIRROR`). По умолчанию файлы
устанавливаются в `%LOCALAPPDATA%\Programs\ocr`.

## Бинарник из GitHub Release

Если Node.js не нужен, скачайте статический бинарник напрямую со
[страницы релизов](https://github.com/alibaba/open-code-review/releases):

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux x86_64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux ARM64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (AMD64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

К каждому релизу прилагается файл `sha256sum.txt` с контрольными суммами.
С его помощью можно проверить целостность загруженных файлов:

```bash
curl -LO https://github.com/alibaba/open-code-review/releases/latest/download/sha256sum.txt
shasum -a 256 -c sha256sum.txt --ignore-missing
```

## Сборка из исходников

Сборка из исходников понадобится, если вы разрабатываете OCR или для вашей
платформы нет готового бинарного файла.

#### Предварительные требования

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

#### Сборка

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build              # пишет dist/opencodereview
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### Сборка под другую платформу

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64   # Windows (x86_64)
make build-windows-arm64   # Windows (ARM64)
make build-all          # все шесть сразу
make sha256sum          # также создать sha256sum.txt
```

`make dist` выполняет `clean → build-all → sha256sum` и сохраняет файл `VERSION`
в каталоге с бинарными файлами. Таким же способом собираются официальные релизы.

#### Запуск тестов

```bash
make test               # LC_ALL=C go test -v -race -count=1 ./...
```

## Проверка установки

Независимо от способа получения бинарника:

```bash
ocr version             # версия + git commit + дата сборки
ocr --help              # справка верхнего уровня
ocr review --help       # полный список флагов команды review
```

Если видите ошибку «command not found», убедитесь, что каталог установки
есть в `$PATH`:

```bash
which ocr
echo $PATH
```

## Включение автодополнения оболочки (опционально)

`ocr` поддерживает автодополнение по Tab для bash, zsh, fish и PowerShell.

```bash
# bash
source <(ocr completion bash)

# zsh
ocr completion zsh > "${fpath[1]}/_ocr"
```

Подробности по fish, PowerShell и постоянной настройке см. в
[CLI Reference](./cli-reference.md#ocr-completion).


## Где OCR хранит состояние

| Путь | Содержимое |
|---|---|
| `~/.opencodereview/config.json` | LLM-эндпоинт, язык и настройки телеметрии (управляются через `ocr config set`). |
| `~/.opencodereview/rule.json` | Необязательные глобальные правила ревью. |
| `~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl` | Журнал каждой сессии ревью в формате JSONL, используется `ocr viewer`. |
| `~/.opencodereview/{last-update-check,update.lock,update-available}` | Состояние фоновой проверки обновлений npm-обёртки. Обёртка опрашивает наличие нового релиза (по умолчанию примерно раз в 18 мин) и печатает подсказку об обновлении. Отключить: `OCR_NO_UPDATE=1`; интервал: `OCR_UPDATE_INTERVAL` (минуты). Статический бинарник эти файлы не пишет. |
| `<repo>/.opencodereview/rule.json` | Необязательный файл с правилами ревью для конкретного проекта. Его можно хранить в репозитории. |

OCR не хранит постоянные данные за пределами `~/.opencodereview/`: исключение
составляет временная загрузка бинарника при установке через npm. Удаление
каталога полностью очищает локальные данные OCR.

## См. также

- [Быстрый старт](../quickstart/): настройка LLM и первое ревью.
- [Конфигурация](../configuration/): все переменные окружения и ключи конфигурации, которые учитывает OCR.
- [Участие в разработке](../contributing/): сборка из исходников, тесты и доработка OCR.
