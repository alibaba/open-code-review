---
title: 安装
sidebar:
  order: 4
---

安装 `ocr` CLI 有四种受支持的方式。

## NPM（推荐）

#### 安装

```bash
npm install -g @alibaba-group/open-code-review
```

固定到某个版本：

```bash
npm install -g @alibaba-group/open-code-review@<version>
```

#### 更新

通过 NPM 安装时，`ocr` 默认会自动保持最新（静态二进制不参与此机制）。每次运行
`ocr` 时，wrapper 会在后台静默检查 registry 上的最新版本，发现更新即自动升级，
不影响本次评审。两次检查之间有 18 分钟冷却，可用 `OCR_UPDATE_INTERVAL`（分钟）调整。

关闭自动更新，将 `OCR_NO_UPDATE` 设为任意非空值即可：

```bash
export OCR_NO_UPDATE=1
```

#### 卸载

```bash
npm uninstall -g @alibaba-group/open-code-review
```

## 安装脚本（curl | sh）

一个便捷安装器，封装了 GitHub Release 二进制下载（带校验）——适合 CI 基础
镜像和无界面环境：

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

它识别两个环境变量：

| 变量 | 默认值 | 用途 |
|---|---|---|
| `OCR_INSTALL_DIR` | `/usr/local/bin` | 放置 `ocr` 二进制的位置。 |
| `OCR_VERSION` | 最新 release | 固定到某个 release tag（如 `v1.2.3`）。 |

该脚本支持 `darwin` 与 `linux` 的 `amd64` / `arm64`；Windows 请改用
[GitHub Release 二进制](#github-release-binary)或 [NPM](#npm-recommended)
方式。

## GitHub Release 二进制

如果你不想装 Node.js，可直接从
[releases 页面](https://github.com/alibaba/open-code-review/releases)获取
静态二进制：

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

每个 release 还会在二进制旁发布 `sha256sum.txt`，供你校验完整性：

```bash
curl -LO https://github.com/alibaba/open-code-review/releases/latest/download/sha256sum.txt
shasum -a 256 -c sha256sum.txt --ignore-missing
```

## 从源码构建

仅当你要修改 OCR 本身，或在某个没有预编译二进制的平台上运行时才需要此方式。

#### 前置条件

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

#### 构建

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build              # 产出 dist/opencodereview
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### 为其他平台构建

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64   # Windows (x86_64)
make build-windows-arm64   # Windows (ARM64)
make build-all          # 一次性构建全部六个
make sha256sum          # 同时产出 sha256sum.txt
```

`make dist` 会运行 `clean → build-all → sha256sum`，并在二进制旁写入一个
`VERSION` 文件——这正是 release 流水线执行的步骤。

#### 运行测试

```bash
make test               # LC_ALL=C go test -v -race -count=1 ./...
```

## 验证安装

无论二进制来自哪里：

```bash
ocr version             # 打印版本 + git commit + 构建日期
ocr --help              # 顶层用法
ocr review --help       # 完整的 review 命令参数列表
```

如果出现 "command not found" 错误，请确认安装位置在你的 `$PATH` 上：

```bash
which ocr
echo $PATH
```

## OCR 在哪里存放状态

| 路径 | 存放内容 |
|---|---|
| `~/.opencodereview/config.json` | LLM 端点、语言、遥测配置（由 `ocr config set` 管理）。 |
| `~/.opencodereview/rule.json` | 可选的全局评审规则。 |
| `~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl` | 每次评审会话的流式 JSONL 转录，供 `ocr viewer` 使用。 |
| `~/.opencodereview/{last-update-check,update.lock,update-available}` | NPM wrapper 的后台更新检查状态。wrapper 会轮询是否有更新的 release（默认约每 18 分钟一次）并打印升级提示。用 `OCR_NO_UPDATE=1` 禁用，或用 `OCR_UPDATE_INTERVAL`（分钟）调整间隔。静态二进制不写入这些文件。 |
| `<repo>/.opencodereview/rule.json` | 可选的项目级评审规则——可安全提交。 |

OCR 永远不会写入 `~/.opencodereview/` 之外（除 NPM 临时下载二进制外）。
删除该目录即可完成干净的卸载。

## 另见

- [快速开始](../quickstart/)——配置 LLM 并完成首次评审。
- [配置](../configuration/)——OCR 接受的每个环境变量与 config key。
- [贡献](../contributing/)——从源码构建、跑测试并参与开发。
