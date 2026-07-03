---
title: Installation
sidebar:
  order: 4
---

There are four supported ways to install the `ocr` CLI. They all produce
the same binary — pick whichever fits your environment.

## NPM (recommended)

```bash
npm install -g @alibaba-group/open-code-review
```

The NPM package ships a small wrapper script (`bin/ocr.js`) plus a
[postinstall hook](https://github.com/alibaba/open-code-review/blob/main/scripts/install.js)
that:

1. Detects your platform (`darwin-amd64`, `darwin-arm64`, `linux-amd64`,
   `linux-arm64`, `windows-amd64`, `windows-arm64`).
2. Downloads the matching binary from GitHub Releases.
3. Verifies it (when checksum data is present) and places it next to the
   wrapper.

If a platform-specific npm package (e.g. `@alibaba-group/ocr-darwin-arm64`)
is installed as an optional dependency, the binary is used directly and the
download is skipped.

When you run `ocr`, the wrapper just `exec`s the downloaded binary, so the
overhead is effectively zero after first run.

### Updating

```bash
npm update -g @alibaba-group/open-code-review
# or pin a specific version:
npm install -g @alibaba-group/open-code-review@<version>
```

### Uninstalling

```bash
npm uninstall -g @alibaba-group/open-code-review
```

## GitHub Release binary

If you don't want Node.js, grab the static binary directly from the
[releases page](https://github.com/alibaba/open-code-review/releases):

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

Each release also publishes `sha256sum.txt` next to the binaries so you can
verify integrity:

```bash
curl -LO https://github.com/alibaba/open-code-review/releases/latest/download/sha256sum.txt
shasum -a 256 -c sha256sum.txt --ignore-missing
```

## Install script (curl | sh)

A convenience installer that wraps the GitHub Release binary download
(with checksum verification) — handy for CI base images and headless
machines:

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

It honours two environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `OCR_INSTALL_DIR` | `/usr/local/bin` | Where to place the `ocr` binary. |
| `OCR_VERSION` | latest release | Pin a specific release tag (e.g. `v1.2.3`). |

The script supports `darwin` and `linux` on `amd64` / `arm64`; for
Windows, use the [GitHub Release binary](#github-release-binary) or
[NPM](#npm-recommended) path instead.

## Build from source

You only need this path if you're hacking on OCR or running on a platform
without a pre-built binary.

### Prerequisites

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Build

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build              # writes dist/opencodereview
sudo cp dist/opencodereview /usr/local/bin/ocr
```

### Build for another platform

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64   # Windows (x86_64)
make build-windows-arm64   # Windows (ARM64)
make build-all          # all six at once
make sha256sum          # also produce sha256sum.txt
```

`make dist` runs `clean → build-all → sha256sum` and writes a `VERSION`
file alongside the binaries — that's exactly what the release pipeline
runs.

### Run tests

```bash
make test               # LC_ALL=C go test -v -race -count=1 ./...
```

## Verifying the install

Wherever you got the binary from:

```bash
ocr version             # prints version + git commit + build date
ocr --help              # top-level usage
ocr review --help       # full review-command flag list
```

If you see a "command not found" error, double-check that the install
location is on your `$PATH`:

```bash
which ocr
echo $PATH
```

## Where OCR stores state

| Path | What it holds |
|---|---|
| `~/.opencodereview/config.json` | LLM endpoint, language, telemetry config (managed by `ocr config set`). |
| `~/.opencodereview/rule.json` | Optional global review rules. |
| `~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl` | Streaming JSONL transcript of every review session, used by `ocr viewer`. |
| `~/.opencodereview/{last-update-check,update.lock,update-available}` | NPM wrapper's background update-check state. The wrapper polls for a newer release (every ~18 min by default) and prints an upgrade hint. Disable with `OCR_NO_UPDATE=1`, or tune the interval with `OCR_UPDATE_INTERVAL` (seconds). Not written by the static binary. |
| `<repo>/.opencodereview/rule.json` | Optional per-project review rules — safe to commit. |

OCR never writes outside `~/.opencodereview/` (besides the transient binary
download via NPM). Removing the directory is a clean uninstall.

## See Also

- [QuickStart](../quickstart/) — configure an LLM and run your first review.
- [Configuration](../configuration/) — every env var and config key OCR honors.
- [Contributing](../contributing/) — build from source, run tests, and hack on OCR.
