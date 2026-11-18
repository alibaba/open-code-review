---
title: 설치
sidebar:
  order: 4
---

`ocr` CLI를 설치하는 방법은 여섯 가지입니다.

## NPM (권장) {#npm-recommended}

#### 설치 {#install}

```bash
npm install -g @alibaba-group/open-code-review
```

특정 버전으로 고정:

```bash
npm install -g @alibaba-group/open-code-review@<version>
```

#### 업데이트 {#updating}

NPM으로 설치하면 `ocr`은 기본적으로 자동 업데이트를 유지합니다(정적 바이너리는 이 메커니즘을 사용하지 않습니다). `ocr`을 실행할 때마다 래퍼가 백그라운드에서 조용히 레지스트리의 최신 버전을 확인하고, 업데이트가 있으면 진행 중인 리뷰에 영향을 주지 않고 자동으로 업그레이드합니다. 확인 사이에는 18분의 쿨다운이 있으며 `OCR_UPDATE_INTERVAL`(분 단위)로 조정할 수 있습니다.

자동 업데이트를 끄려면 `OCR_NO_UPDATE`에 비어 있지 않은 아무 값이나 설정합니다:

```bash
export OCR_NO_UPDATE=1
```

#### 제거 {#uninstalling}

```bash
npm uninstall -g @alibaba-group/open-code-review
```

## Homebrew (macOS / Linux) {#homebrew-macos-linux}

```bash
brew install open-code-review
```

포뮬러는 소스에서 빌드한 `ocr` 바이너리를 설치합니다.

이후 업그레이드하려면:

```bash
brew upgrade open-code-review
```

## MacPorts (macOS) {#macports-macos}

```bash
sudo port install open-code-review
```

포트는 소스에서 빌드한 `ocr` 바이너리를 설치합니다.

이후 업그레이드하려면:

```bash
sudo port upgrade open-code-review
```

## 설치 스크립트 (curl | sh) {#install-script-curl-sh}

GitHub Release 바이너리 다운로드(체크섬 검증 포함)를 감싼 편의 인스톨러입니다. CI 베이스 이미지나 헤드리스 머신에 유용합니다:

```bash
curl -fsSL https://open-codereview.ai/install.sh | sh
```

세 가지 환경 변수를 인식합니다:

| 변수 | 기본값 | 용도 |
|---|---|---|
| `OCR_INSTALL_DIR` | `/usr/local/bin` | `ocr` 바이너리를 둘 위치. |
| `OCR_VERSION` | 최신 릴리스 | 특정 릴리스 태그로 고정(예: `v1.2.3`). |
| `OCR_GITHUB_MIRROR` | *(설정 안 함)* | 릴리스 바이너리와 체크섬을 GitHub 미러 도메인(예: `gh-proxy.com`)을 통해 내려받습니다. |

스크립트는 `amd64` / `arm64`의 `darwin`과 `linux`를 지원합니다.

#### GitHub 미러 사용 {#using-a-github-mirror}

GitHub 접속이 느린 지역에서는 `OCR_GITHUB_MIRROR`에 미러 도메인을 지정해 릴리스 바이너리와 체크섬을 미러를 통해 내려받을 수 있습니다:

```bash
export OCR_GITHUB_MIRROR='YOUR_MIRROR_DOMAIN'
```

값은 `https://` 스킴과 끝 슬래시가 없는 순수 도메인 이름이어야 합니다(`https://gh-proxy.com/`이 아니라 `gh-proxy.com`). 이 값은 *경로 접두사* 미러로 사용하므로, 바이너리는 `https://<mirror>/github.com/alibaba/open-code-review/releases/download/<version>/…`에서 받습니다. 도메인 치환 방식 미러(예: `github.com`을 `hub.example.org`로 바꾸는 형태)는 이 형태와 맞지 않으므로 경로 접두사 미러를 사용하세요.

미러는 릴리스 바이너리와 `sha256sum.txt` 체크섬 모두에 적용됩니다. `OCR_VERSION`이 설정되지 않았을 때의 버전 확인은 미러가 아니라 GitHub API를 직접 호출합니다. 버전 확인 자체를 건너뛰려면 버전을 고정하세요:

```bash
export OCR_VERSION='v1.2.3'
```

> **보안 참고:** 미러는 서드파티 서비스이므로 `OCR_GITHUB_MIRROR`를 설정하면 바이너리와 `sha256sum.txt`를 모두 미러에서 내려받습니다. 따라서 악의적인 미러는 변조된 바이너리와 그에 맞는 체크섬을 함께 내려줄 수 있으며, 미러 모드에서는 무결성 보장이 성립하지 않습니다. 미러를 신뢰할 수 없다면 [릴리스 페이지](https://github.com/alibaba/open-code-review/releases)의 원본 `sha256sum.txt`로 내려받은 파일을 검증하세요.

Windows(PowerShell 5.1 이상)에서는 PowerShell 인스톨러를 사용하세요:

```powershell
irm https://open-codereview.ai/install.ps1 | iex
```

같은 `OCR_INSTALL_DIR`, `OCR_VERSION`, `OCR_GITHUB_MIRROR` 변수를 인식합니다(`$env:OCR_INSTALL_DIR` / `$env:OCR_VERSION` / `$env:OCR_GITHUB_MIRROR`로 설정). 기본 설치 위치는 `%LOCALAPPDATA%\Programs\ocr`입니다.

## GitHub Release 바이너리 {#github-release-binary}

Node.js를 쓰고 싶지 않다면 [릴리스 페이지](https://github.com/alibaba/open-code-review/releases)에서 정적 바이너리를 직접 받으세요:

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

각 릴리스에는 바이너리와 나란히 `sha256sum.txt`도 게시되므로 무결성을 검증할 수 있습니다:

```bash
curl -LO https://github.com/alibaba/open-code-review/releases/latest/download/sha256sum.txt
shasum -a 256 -c sha256sum.txt --ignore-missing
```

## 소스에서 빌드 {#build-from-source}

OCR 자체를 수정하거나 사전 빌드 바이너리가 없는 플랫폼에서 실행할 때만 필요한 경로입니다.

#### 사전 요구 사항 {#prerequisites}

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

#### 빌드 {#build}

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build              # dist/opencodereview 생성
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### 다른 플랫폼용 빌드 {#build-for-another-platform}

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64   # Windows (x86_64)
make build-windows-arm64   # Windows (ARM64)
make build-all          # 여섯 개 한 번에
make sha256sum          # sha256sum.txt도 생성
```

`make dist`는 `clean → build-all → sha256sum`을 실행하고 바이너리 옆에 `VERSION` 파일을 기록합니다. 릴리스 파이프라인이 실행하는 것과 정확히 같습니다.

#### 테스트 실행 {#run-tests}

```bash
make test               # LC_ALL=C go test -v -race -count=1 ./...
```

## 설치 확인 {#verifying-the-install}

어떤 방법으로 설치했든:

```bash
ocr version             # 버전 + git 커밋 + 빌드 날짜 출력
ocr --help              # 최상위 사용법
ocr review --help       # review 명령의 전체 플래그 목록
```

"command not found" 오류가 나오면 설치 위치가 `$PATH`에 있는지 다시 확인하세요:

```bash
which ocr
echo $PATH
```

## 셸 자동 완성 활성화 (선택) {#enable-shell-completion-optional}

`ocr`은 bash, zsh, fish, PowerShell의 탭 자동 완성을 지원합니다.

```bash
# bash
source <(ocr completion bash)

# zsh
ocr completion zsh > "${fpath[1]}/_ocr"
```

fish, PowerShell 및 영구 설정 방법은 [CLI 레퍼런스](./cli-reference.md#ocr-completion)를 참고하세요.


## OCR이 상태를 저장하는 위치 {#where-ocr-stores-state}

| 경로 | 내용 |
|---|---|
| `~/.opencodereview/config.json` | LLM 엔드포인트, 언어, 텔레메트리 설정(`ocr config set`으로 관리). |
| `~/.opencodereview/rule.json` | 선택적인 전역 리뷰 규칙. |
| `~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl` | 모든 리뷰 세션의 스트리밍 JSONL 기록. `ocr viewer`가 사용. |
| `~/.opencodereview/{last-update-check,update.lock,update-available}` | NPM 래퍼의 백그라운드 업데이트 확인 상태. 래퍼는 새 릴리스를 주기적으로(기본 약 18분마다) 확인하고 업그레이드 안내를 출력합니다. `OCR_NO_UPDATE=1`로 끄거나 `OCR_UPDATE_INTERVAL`(분 단위)로 주기를 조정합니다. 정적 바이너리는 기록하지 않음. |
| `<repo>/.opencodereview/rule.json` | 선택적인 프로젝트별 리뷰 규칙 — 커밋해도 안전. |

OCR은 `~/.opencodereview/` 밖에는 기록하지 않습니다(NPM을 통한 일시적인 바이너리 다운로드 제외). 이 디렉터리를 지우면 깔끔하게 제거됩니다.

## 관련 문서 {#see-also}

- [빠른 시작](../quickstart/) — LLM을 설정하고 첫 리뷰 실행.
- [설정](../configuration/) — OCR이 인식하는 모든 환경 변수와 설정 키.
- [기여하기](../contributing/) — 소스 빌드, 테스트 실행, OCR 개발.
