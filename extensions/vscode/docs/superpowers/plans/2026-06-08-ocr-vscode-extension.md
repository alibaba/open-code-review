# OCR VSCode 插件 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个基于 `ocr` CLI 的 VSCode 代码审查插件，以 Preact WebView 还原原型体验，可独立运行，并能通过一键脚本同步复用到 aone-copilot-vscode。

**Architecture:** Thin Extension Host（Node.js）负责 CLI 进程、Git、配置、Comment API；Preact WebView SPA 还原原型 UI；两者通过类型安全的 postMessage 协议通信。`shared/` 目录放双端共享的类型与协议，不依赖 vscode。

**Tech Stack:** TypeScript、VSCode Extension API、Preact、webpack（双入口）、Jest（ts-jest）、Go CLI（`ocr`，外部依赖）。

**关联 Spec:** `docs/superpowers/specs/2026-06-08-ocr-vscode-extension-design.md`

---

## 文件结构（实现完成后）

```
open-code-review-vscode/
├── src/
│   ├── extension/
│   │   ├── extension.ts            独立运行入口
│   │   ├── index.ts                公共导出（activateOcr / deactivateOcr / OcrAdapter）
│   │   ├── services/
│   │   │   ├── CliService.ts       spawn ocr，流式解析
│   │   │   ├── ConfigService.ts    读写 CLI config
│   │   │   ├── GitService.ts       git 状态
│   │   │   └── ReviewSession.ts    状态机 + 生命周期
│   │   ├── providers/
│   │   │   ├── SidebarProvider.ts  WebviewViewProvider
│   │   │   └── CommentProvider.ts  Comment API
│   │   └── commands.ts             命令注册
│   ├── webview/
│   │   ├── index.tsx               SPA 入口
│   │   ├── App.tsx                 根组件
│   │   ├── store.ts                状态容器（useReducer + Context）
│   │   ├── bridge.ts               postMessage 封装
│   │   ├── views/                  IdleView / RunningView / DoneView / EmptyView / FailedView / ConfigView
│   │   ├── components/             StatusBar / ModelDropdown / FileList / LogViewer / CommentCard
│   │   └── styles/global.css       silent-night 样式
│   └── shared/
│       ├── types.ts                ReviewComment / ReviewMode / OcrConfig / GitState ...
│       ├── messages.ts             postMessage 协议
│       └── constants.ts            枚举 + 命令 ID
├── scripts/sync-to-aone.js         一键同步脚本
├── package.json
├── tsconfig.json                   base
├── tsconfig.extension.json
├── tsconfig.webview.json
├── webpack.config.js               双入口
├── jest.config.js
└── .vscodeignore
```

测试策略：纯逻辑（CLI 参数构造、JSON 解析、日志解析、config 读取、消息归约 reducer）走 TDD + Jest。VSCode API 绑定层（Providers、命令注册）和 WebView 视图组件以手动验收为主，在对应任务中明确验收步骤。

---

## Task 1: 项目脚手架与构建配置

**Files:**
- Create: `package.json`
- Create: `tsconfig.json`
- Create: `tsconfig.extension.json`
- Create: `tsconfig.webview.json`
- Create: `jest.config.js`
- Create: `.vscodeignore`
- Create: `.gitignore`（追加 node_modules / out）

- [ ] **Step 1: 写 package.json**

注意：当前已有的 `package.json` 仅含 `@anthropic-ai/claude-code` 依赖（用于本地工作流），需整体替换为 extension manifest，并把该依赖移除（它不是插件运行所需）。

```json
{
  "name": "open-code-review-vscode",
  "displayName": "Open Code Review",
  "description": "AI 代码审查 —— 基于 open-code-review CLI",
  "version": "0.1.0",
  "publisher": "open-code-review",
  "license": "Apache-2.0",
  "engines": { "vscode": "^1.74.0" },
  "categories": ["Other"],
  "main": "./out/extension.js",
  "activationEvents": ["onStartupFinished"],
  "contributes": {
    "viewsContainers": {
      "activitybar": [
        {
          "id": "ocr-container",
          "title": "Open Code Review",
          "icon": "resources/icon.svg"
        }
      ]
    },
    "views": {
      "ocr-container": [
        { "id": "ocr.sidebar", "type": "webview", "name": "Code Review" }
      ]
    },
    "commands": [
      { "command": "ocr.review.start", "title": "OCR: 开始代码审查" },
      { "command": "ocr.review.cancel", "title": "OCR: 取消审查" },
      { "command": "ocr.config.open", "title": "OCR: 打开配置" },
      { "command": "ocr.comment.apply", "title": "应用建议" },
      { "command": "ocr.comment.applyAndNext", "title": "应用并下一个" },
      { "command": "ocr.comment.discard", "title": "忽略" },
      { "command": "ocr.comment.discardAndNext", "title": "忽略并下一个" },
      { "command": "ocr.comment.falsePositive", "title": "误报" },
      { "command": "ocr.comment.falsePositiveAndNext", "title": "误报并下一个" },
      { "command": "ocr.comment.prev", "title": "上一个评论" },
      { "command": "ocr.comment.next", "title": "下一个评论" }
    ],
    "menus": {
      "comments/commentThread/title": [
        { "command": "ocr.comment.prev", "group": "navigation@1", "when": "commentController == ocr-review" },
        { "command": "ocr.comment.next", "group": "navigation@2", "when": "commentController == ocr-review" }
      ]
    }
  },
  "scripts": {
    "compile": "webpack --mode development",
    "watch": "webpack --mode development --watch",
    "build": "webpack --mode production",
    "test": "jest",
    "lint": "eslint src --ext ts,tsx",
    "vscode:prepublish": "yarn build"
  },
  "devDependencies": {
    "@types/node": "^18.0.0",
    "@types/vscode": "^1.74.0",
    "@types/jest": "^29.5.0",
    "ts-jest": "^29.1.0",
    "jest": "^29.7.0",
    "ts-loader": "^9.5.0",
    "typescript": "^5.3.0",
    "webpack": "^5.89.0",
    "webpack-cli": "^5.1.0",
    "css-loader": "^6.8.0",
    "style-loader": "^3.3.0",
    "eslint": "^8.56.0",
    "@typescript-eslint/parser": "^6.18.0",
    "@typescript-eslint/eslint-plugin": "^6.18.0"
  },
  "dependencies": {
    "preact": "^10.19.0"
  }
}
```

- [ ] **Step 2: 写 tsconfig.json（base）**

```json
{
  "compilerOptions": {
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": false,
    "sourceMap": true
  },
  "exclude": ["node_modules", "out", "reference"]
}
```

- [ ] **Step 3: 写 tsconfig.extension.json**

```json
{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "module": "commonjs",
    "target": "ES2021",
    "lib": ["ES2021"],
    "outDir": "out",
    "types": ["node", "vscode", "jest"]
  },
  "include": ["src/extension/**/*", "src/shared/**/*"]
}
```

- [ ] **Step 4: 写 tsconfig.webview.json**

```json
{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "target": "ES2020",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "jsx": "react-jsx",
    "jsxImportSource": "preact",
    "types": []
  },
  "include": ["src/webview/**/*", "src/shared/**/*"]
}
```

- [ ] **Step 5: 写 jest.config.js**

```js
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  transform: {
    '^.+\\.tsx?$': ['ts-jest', { isolatedModules: true, tsconfig: 'tsconfig.extension.json' }],
  },
  testMatch: ['**/__tests__/**/*.test.ts'],
};
```

- [ ] **Step 6: 写 webpack.config.js（双入口）**

```js
const path = require('path');

/** @type {import('webpack').Configuration} */
const extensionConfig = {
  name: 'extension',
  target: 'node',
  entry: { extension: './src/extension/extension.ts' },
  output: {
    path: path.resolve(__dirname, 'out'),
    filename: '[name].js',
    libraryTarget: 'commonjs2',
  },
  externals: { vscode: 'commonjs vscode' },
  resolve: { extensions: ['.ts', '.js'] },
  module: {
    rules: [
      { test: /\.ts$/, exclude: /node_modules/, use: 'ts-loader' },
    ],
  },
  devtool: 'source-map',
};

/** @type {import('webpack').Configuration} */
const webviewConfig = {
  name: 'webview',
  target: 'web',
  entry: { webview: './src/webview/index.tsx' },
  output: {
    path: path.resolve(__dirname, 'out'),
    filename: '[name].js',
  },
  resolve: { extensions: ['.ts', '.tsx', '.js'] },
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        exclude: /node_modules/,
        use: { loader: 'ts-loader', options: { configFile: 'tsconfig.webview.json' } },
      },
      { test: /\.css$/, use: ['style-loader', 'css-loader'] },
    ],
  },
  devtool: 'source-map',
};

module.exports = [extensionConfig, webviewConfig];
```

ts-loader 默认读 `tsconfig.json`，extension 入口需要 commonjs。把 extension 的 ts-loader 也指定 configFile：

在 `extensionConfig` 的 rule 中改为：
```js
        use: { loader: 'ts-loader', options: { configFile: 'tsconfig.extension.json' } },
```

- [ ] **Step 7: 写 .vscodeignore**

```
src/**
reference/**
docs/**
scripts/**
node_modules/**
**/*.ts
**/*.map
tsconfig*.json
webpack.config.js
jest.config.js
.gitignore
```

- [ ] **Step 8: 确认 .gitignore 含 node_modules 和 out**

读取现有 `.gitignore`，若缺少则追加：
```
node_modules/
out/
```

- [ ] **Step 9: 创建占位图标**

Create: `resources/icon.svg`（简单占位，后续替换）
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24"><circle cx="12" cy="12" r="6" fill="#45e6a4"/></svg>
```

- [ ] **Step 10: 安装依赖并验证构建脚手架**

Run: `yarn install`
Expected: 安装成功，无 peer 冲突。

- [ ] **Step 11: Commit**

```bash
git add package.json tsconfig*.json jest.config.js webpack.config.js .vscodeignore .gitignore resources/
git commit -m "chore: scaffold VSCode extension build setup"
```

---

## Task 2: 共享类型与协议（shared/）

**Files:**
- Create: `src/shared/types.ts`
- Create: `src/shared/messages.ts`
- Create: `src/shared/constants.ts`

- [ ] **Step 1: 写 src/shared/types.ts**

字段命名与 CLI JSON 输出对齐（camelCase 转换在 CliService 解析时完成）。

```typescript
export type ReviewMode = 'workspace' | 'branch' | 'commit';

export type ReviewState =
  | 'idle' | 'running' | 'done' | 'empty' | 'cancelled' | 'failed';

export type CommentStatus = 'pending' | 'applied' | 'discarded' | 'falsePositive';

export interface ReviewComment {
  path: string;
  content: string;
  suggestionCode?: string;
  existingCode?: string;
  startLine: number;
  endLine: number;
  thinking?: string;
}

export interface ReviewSummary {
  filesReviewed: number;
  comments: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  elapsed: string;
}

export interface AgentWarning {
  type: string;
  file: string;
  message: string;
}

export interface CliResult {
  status: 'success' | 'completed_with_errors' | 'completed_with_warnings' | 'skipped';
  comments: ReviewComment[];
  warnings: AgentWarning[];
  summary?: ReviewSummary;
  message?: string;
}

export interface OcrConfig {
  llm: {
    url: string;
    authToken: string;
    model: string;
    useAnthropic: boolean;
  };
  language: string;
}

export interface CommitInfo {
  sha: string;
  message: string;
  relativeTime: string;
}

export interface FileChange {
  path: string;
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'binary';
}

export interface GitState {
  branches: string[];
  currentBranch: string;
  recentCommits: CommitInfo[];
  workspaceFiles: FileChange[];
}

export interface LogLine {
  text: string;
  level: 'info' | 'warn' | 'error';
}

export interface CliRunOptions {
  mode: ReviewMode;
  from?: string;
  to?: string;
  commit?: string;
  customPrompt?: string;
  concurrency?: number;
}

export interface CommentSyncState {
  index: number;
  status: CommentStatus;
}
```

- [ ] **Step 2: 写 src/shared/constants.ts**

```typescript
export const SIDEBAR_VIEW_ID = 'ocr.sidebar';
export const COMMENT_CONTROLLER_ID = 'ocr-review';

export const COMMANDS = {
  reviewStart: 'ocr.review.start',
  reviewCancel: 'ocr.review.cancel',
  configOpen: 'ocr.config.open',
  commentApply: 'ocr.comment.apply',
  commentApplyAndNext: 'ocr.comment.applyAndNext',
  commentDiscard: 'ocr.comment.discard',
  commentDiscardAndNext: 'ocr.comment.discardAndNext',
  commentFalsePositive: 'ocr.comment.falsePositive',
  commentFalsePositiveAndNext: 'ocr.comment.falsePositiveAndNext',
  commentPrev: 'ocr.comment.prev',
  commentNext: 'ocr.comment.next',
} as const;
```

- [ ] **Step 3: 写 src/shared/messages.ts**

```typescript
import {
  CliResult, CliRunOptions, CommentSyncState, GitState, LogLine,
  OcrConfig, ReviewMode, ReviewState,
} from './types';

export type WebviewToHost =
  | { type: 'ready' }
  | { type: 'getGitState'; mode: ReviewMode }
  | { type: 'startReview'; options: CliRunOptions }
  | { type: 'cancelReview' }
  | { type: 'getConfig' }
  | { type: 'setConfig'; key: string; value: string }
  | { type: 'testConnection' }
  | { type: 'jumpToComment'; index: number }
  | { type: 'commentAction'; index: number; action: 'apply' | 'discard' | 'falsePositive' };

export type HostToWebview =
  | { type: 'init'; config: OcrConfig | null; gitState: GitState }
  | { type: 'gitState'; gitState: GitState }
  | { type: 'logLine'; line: LogLine }
  | { type: 'stateChange'; state: ReviewState }
  | { type: 'reviewDone'; result: CliResult }
  | { type: 'config'; config: OcrConfig | null }
  | { type: 'connectionResult'; ok: boolean; message?: string }
  | { type: 'commentSync'; comments: CommentSyncState[] };
```

- [ ] **Step 4: 类型编译校验**

Run: `npx tsc -p tsconfig.extension.json --noEmit`
Expected: 无错误。

- [ ] **Step 5: Commit**

```bash
git add src/shared/
git commit -m "feat: add shared types and postMessage protocol"
```

---

## Task 3: CliService — 纯逻辑（参数构造 + 输出解析）

把可测的纯逻辑（构造 CLI 参数、解析 stdout JSON、解析 stderr 日志行）抽成独立函数，TDD 覆盖；进程 spawn 部分在 Task 4 处理。

**Files:**
- Create: `src/extension/services/cliParse.ts`
- Test: `src/extension/services/__tests__/cliParse.test.ts`

- [ ] **Step 1: 写失败测试 — buildReviewArgs**

```typescript
// src/extension/services/__tests__/cliParse.test.ts
import { buildReviewArgs, parseCliResult, parseLogLine } from '../cliParse';

describe('buildReviewArgs', () => {
  it('workspace 模式只加 --format json', () => {
    expect(buildReviewArgs({ mode: 'workspace' }))
      .toEqual(['review', '--format', 'json']);
  });

  it('branch 模式加 --from/--to', () => {
    expect(buildReviewArgs({ mode: 'branch', from: 'main', to: 'dev' }))
      .toEqual(['review', '--from', 'main', '--to', 'dev', '--format', 'json']);
  });

  it('commit 模式加 --commit', () => {
    expect(buildReviewArgs({ mode: 'commit', commit: 'abc123' }))
      .toEqual(['review', '--commit', 'abc123', '--format', 'json']);
  });

  it('customPrompt 追加 --background', () => {
    expect(buildReviewArgs({ mode: 'workspace', customPrompt: '关注安全' }))
      .toEqual(['review', '--format', 'json', '--background', '关注安全']);
  });

  it('concurrency 追加 --concurrency', () => {
    expect(buildReviewArgs({ mode: 'workspace', concurrency: 4 }))
      .toEqual(['review', '--format', 'json', '--concurrency', '4']);
  });
});
```

- [ ] **Step 2: 写失败测试 — parseCliResult**

```typescript
describe('parseCliResult', () => {
  it('解析 success + comments + summary，字段转 camelCase', () => {
    const raw = JSON.stringify({
      status: 'success',
      comments: [{
        path: 'src/a.ts', content: 'bug', start_line: 10, end_line: 12,
        suggestion_code: 'fix', existing_code: 'old',
      }],
      summary: {
        files_reviewed: 2, comments: 1, total_tokens: 100,
        input_tokens: 80, output_tokens: 20, elapsed: '5s',
      },
    });
    const r = parseCliResult(raw);
    expect(r.status).toBe('success');
    expect(r.comments[0]).toEqual({
      path: 'src/a.ts', content: 'bug', startLine: 10, endLine: 12,
      suggestionCode: 'fix', existingCode: 'old', thinking: undefined,
    });
    expect(r.summary?.filesReviewed).toBe(2);
  });

  it('skipped 状态无 comments', () => {
    const raw = JSON.stringify({ status: 'skipped', message: 'No supported files changed.', comments: [] });
    const r = parseCliResult(raw);
    expect(r.status).toBe('skipped');
    expect(r.comments).toEqual([]);
  });

  it('忽略 JSON 前的非 JSON 噪声行', () => {
    const raw = '[ocr] some log\n{"status":"success","comments":[]}';
    const r = parseCliResult(raw);
    expect(r.status).toBe('success');
  });
});
```

- [ ] **Step 3: 写失败测试 — parseLogLine**

```typescript
describe('parseLogLine', () => {
  it('普通 [ocr] 行 → info', () => {
    expect(parseLogLine('[ocr] Reviewing src/a.ts')).toEqual({ text: '[ocr] Reviewing src/a.ts', level: 'info' });
  });
  it('含 Retrying 的行 → warn', () => {
    expect(parseLogLine('[llm] Retrying in 1.46s (attempt 1/3)').level).toBe('warn');
  });
  it('含 WARNING 的行 → warn', () => {
    expect(parseLogLine('[ocr] WARNING [x] f: m').level).toBe('warn');
  });
  it('空行 → null', () => {
    expect(parseLogLine('   ')).toBeNull();
  });
});
```

- [ ] **Step 4: 运行测试确认失败**

Run: `npx jest cliParse`
Expected: FAIL（模块不存在 / 函数未定义）。

- [ ] **Step 5: 实现 src/extension/services/cliParse.ts**

```typescript
import { CliResult, CliRunOptions, LogLine, ReviewComment } from '../../shared/types';

export function buildReviewArgs(opts: CliRunOptions): string[] {
  const args: string[] = ['review'];
  if (opts.mode === 'branch') {
    if (opts.from) args.push('--from', opts.from);
    if (opts.to) args.push('--to', opts.to);
  } else if (opts.mode === 'commit') {
    if (opts.commit) args.push('--commit', opts.commit);
  }
  args.push('--format', 'json');
  if (opts.customPrompt && opts.customPrompt.trim()) {
    args.push('--background', opts.customPrompt.trim());
  }
  if (typeof opts.concurrency === 'number') {
    args.push('--concurrency', String(opts.concurrency));
  }
  return args;
}

function toComment(raw: any): ReviewComment {
  return {
    path: raw.path,
    content: raw.content,
    suggestionCode: raw.suggestion_code || undefined,
    existingCode: raw.existing_code || undefined,
    startLine: raw.start_line,
    endLine: raw.end_line,
    thinking: raw.thinking || undefined,
  };
}

export function parseCliResult(stdout: string): CliResult {
  const start = stdout.indexOf('{');
  if (start < 0) throw new Error('no JSON in CLI output');
  const json = JSON.parse(stdout.slice(start));
  const s = json.summary;
  return {
    status: json.status,
    message: json.message,
    comments: Array.isArray(json.comments) ? json.comments.map(toComment) : [],
    warnings: Array.isArray(json.warnings) ? json.warnings : [],
    summary: s ? {
      filesReviewed: s.files_reviewed,
      comments: s.comments,
      totalTokens: s.total_tokens,
      inputTokens: s.input_tokens,
      outputTokens: s.output_tokens,
      elapsed: s.elapsed,
    } : undefined,
  };
}

export function parseLogLine(raw: string): LogLine | null {
  const text = raw.replace(/\s+$/, '');
  if (!text.trim()) return null;
  const level: LogLine['level'] = /retrying|warning|warn/i.test(text) ? 'warn' : 'info';
  return { text, level };
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `npx jest cliParse`
Expected: PASS（全部用例）。

- [ ] **Step 7: Commit**

```bash
git add src/extension/services/cliParse.ts src/extension/services/__tests__/cliParse.test.ts
git commit -m "feat: add CLI argument building and output parsing"
```

---

## Task 4: CliService — 进程管理

包裹 `cliParse`，spawn `ocr`，流式推日志，支持取消、检测 CLI 是否存在。VSCode 无关，可在 Node 环境测基本行为（用 `echo` 模拟）。

**Files:**
- Create: `src/extension/services/CliService.ts`
- Test: `src/extension/services/__tests__/CliService.test.ts`

- [ ] **Step 1: 写失败测试（用真实子进程模拟）**

```typescript
// src/extension/services/__tests__/CliService.test.ts
import { CliService } from '../CliService';

describe('CliService.isAvailable', () => {
  it('node 一定存在 → true', async () => {
    const svc = new CliService('node');
    expect(await svc.isAvailable()).toBe(true);
  });
  it('不存在的命令 → false', async () => {
    const svc = new CliService('definitely-not-a-real-binary-xyz');
    expect(await svc.isAvailable()).toBe(false);
  });
});

describe('CliService.runRaw', () => {
  it('收集 stdout 并在结束时 resolve', async () => {
    // 用 node 打印一段 JSON 模拟 ocr
    const svc = new CliService('node');
    const logs: string[] = [];
    const out = await svc.runRaw(
      ['-e', 'process.stdout.write(JSON.stringify({status:"success",comments:[]}))'],
      '.', (line) => logs.push(line.text),
    );
    expect(out).toContain('"status":"success"');
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest CliService`
Expected: FAIL（模块不存在）。

- [ ] **Step 3: 实现 src/extension/services/CliService.ts**

```typescript
import { spawn } from 'child_process';
import { CliResult, CliRunOptions, LogLine } from '../../shared/types';
import { buildReviewArgs, parseCliResult, parseLogLine } from './cliParse';

export class CliService {
  private current: ReturnType<typeof spawn> | null = null;

  constructor(private cliPath: string = 'ocr') {}

  async isAvailable(): Promise<boolean> {
    return new Promise((resolve) => {
      const p = spawn(this.cliPath, ['--version']);
      p.on('error', () => resolve(false));
      p.on('close', (code) => resolve(code === 0 || code === null ? code === 0 : false));
      // 某些命令 --version 退出码非 0 但存在；以 error 事件为准
      p.on('exit', (code) => resolve(code !== null));
    });
  }

  /** 运行任意参数，流式回调日志，结束返回 stdout 全文。 */
  runRaw(args: string[], cwd: string, onLog: (l: LogLine) => void): Promise<string> {
    return new Promise((resolve, reject) => {
      const proc = spawn(this.cliPath, args, { cwd });
      this.current = proc;
      let stdout = '';
      proc.stdout.on('data', (d) => { stdout += d.toString(); });
      proc.stderr.on('data', (d) => {
        for (const line of d.toString().split('\n')) {
          const parsed = parseLogLine(line);
          if (parsed) onLog(parsed);
        }
      });
      proc.on('error', (err) => { this.current = null; reject(err); });
      proc.on('close', () => { this.current = null; resolve(stdout); });
    });
  }

  async review(opts: CliRunOptions, cwd: string, onLog: (l: LogLine) => void): Promise<CliResult> {
    const stdout = await this.runRaw(buildReviewArgs(opts), cwd, onLog);
    return parseCliResult(stdout);
  }

  async testConnection(): Promise<{ ok: boolean; message?: string }> {
    try {
      await this.runRaw(['llm', 'test'], process.cwd(), () => {});
      return { ok: true };
    } catch (e) {
      return { ok: false, message: e instanceof Error ? e.message : String(e) };
    }
  }

  cancel(): void {
    if (this.current && this.current.pid) {
      this.current.kill('SIGTERM');
      const proc = this.current;
      setTimeout(() => { if (!proc.killed) proc.kill('SIGKILL'); }, 3000);
    }
  }
}
```

注意 `isAvailable` 简化为：以 `exit` 事件触发即代表二进制存在。修正实现，移除 close 中的矛盾逻辑：

```typescript
  async isAvailable(): Promise<boolean> {
    return new Promise((resolve) => {
      const p = spawn(this.cliPath, ['--version']);
      let errored = false;
      p.on('error', () => { errored = true; resolve(false); });
      p.on('close', () => { if (!errored) resolve(true); });
    });
  }
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest CliService`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add src/extension/services/CliService.ts src/extension/services/__tests__/CliService.test.ts
git commit -m "feat: add CliService process management"
```

---

## Task 5: ConfigService — 读写 CLI config

读取 `~/.opencodereview/config.json`（纯逻辑可测），写入通过 `ocr config set`。

**Files:**
- Create: `src/extension/services/configParse.ts`
- Create: `src/extension/services/ConfigService.ts`
- Test: `src/extension/services/__tests__/configParse.test.ts`

- [ ] **Step 1: 写失败测试 — parseConfig / configKeyValue**

```typescript
// src/extension/services/__tests__/configParse.test.ts
import { parseConfig, toConfigSetArgs } from '../configParse';

describe('parseConfig', () => {
  it('完整 config 转 camelCase', () => {
    const raw = JSON.stringify({
      llm: { url: 'u', auth_token: 't', model: 'm', use_anthropic: true },
      language: 'Chinese',
    });
    expect(parseConfig(raw)).toEqual({
      llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: true },
      language: 'Chinese',
    });
  });

  it('缺字段时给默认值', () => {
    const cfg = parseConfig('{}');
    expect(cfg.llm.url).toBe('');
    expect(cfg.llm.useAnthropic).toBe(false);
    expect(cfg.language).toBe('Chinese');
  });

  it('空字符串 → null', () => {
    expect(parseConfig('')).toBeNull();
  });
});

describe('toConfigSetArgs', () => {
  it('生成 config set 参数', () => {
    expect(toConfigSetArgs('llm.model', 'opus')).toEqual(['config', 'set', 'llm.model', 'opus']);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest configParse`
Expected: FAIL。

- [ ] **Step 3: 实现 src/extension/services/configParse.ts**

```typescript
import { OcrConfig } from '../../shared/types';

export function parseConfig(raw: string): OcrConfig | null {
  if (!raw || !raw.trim()) return null;
  const j = JSON.parse(raw);
  const llm = j.llm || {};
  return {
    llm: {
      url: llm.url || '',
      authToken: llm.auth_token || '',
      model: llm.model || '',
      useAnthropic: Boolean(llm.use_anthropic),
    },
    language: j.language || 'Chinese',
  };
}

export function toConfigSetArgs(key: string, value: string): string[] {
  return ['config', 'set', key, value];
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest configParse`
Expected: PASS。

- [ ] **Step 5: 实现 src/extension/services/ConfigService.ts**

```typescript
import { readFileSync, existsSync } from 'fs';
import { homedir } from 'os';
import { join } from 'path';
import { OcrConfig } from '../../shared/types';
import { CliService } from './CliService';
import { parseConfig, toConfigSetArgs } from './configParse';

export class ConfigService {
  constructor(private cli: CliService) {}

  private configPath(): string {
    return join(homedir(), '.opencodereview', 'config.json');
  }

  read(): OcrConfig | null {
    const p = this.configPath();
    if (!existsSync(p)) return null;
    try {
      return parseConfig(readFileSync(p, 'utf8'));
    } catch {
      return null;
    }
  }

  async set(key: string, value: string): Promise<OcrConfig | null> {
    await this.cli.runRaw(toConfigSetArgs(key, value), process.cwd(), () => {});
    return this.read();
  }
}
```

- [ ] **Step 6: Commit**

```bash
git add src/extension/services/configParse.ts src/extension/services/ConfigService.ts src/extension/services/__tests__/configParse.test.ts
git commit -m "feat: add ConfigService backed by CLI config"
```

---

## Task 6: GitService — Git 状态

通过 VSCode 内置 git 扩展 API 取数据。纯映射逻辑可测，扩展 API 调用在集成时手动验收。

**Files:**
- Create: `src/extension/services/gitMap.ts`
- Create: `src/extension/services/GitService.ts`
- Test: `src/extension/services/__tests__/gitMap.test.ts`

- [ ] **Step 1: 写失败测试 — git status 码映射**

```typescript
// src/extension/services/__tests__/gitMap.test.ts
import { mapStatusCode } from '../gitMap';

describe('mapStatusCode', () => {
  it('VSCode git Status 枚举映射到 FileChange.status', () => {
    // VSCode Status: INDEX_ADDED=1, MODIFIED=5, DELETED=6, UNTRACKED=7 (示例值)
    expect(mapStatusCode('A')).toBe('added');
    expect(mapStatusCode('M')).toBe('modified');
    expect(mapStatusCode('D')).toBe('deleted');
    expect(mapStatusCode('R')).toBe('renamed');
    expect(mapStatusCode('?')).toBe('added'); // untracked 视为 added
    expect(mapStatusCode('X')).toBe('modified'); // 未知兜底
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest gitMap`
Expected: FAIL。

- [ ] **Step 3: 实现 src/extension/services/gitMap.ts**

```typescript
import { FileChange } from '../../shared/types';

export function mapStatusCode(code: string): FileChange['status'] {
  switch (code) {
    case 'A': return 'added';
    case '?': return 'added';
    case 'D': return 'deleted';
    case 'R': return 'renamed';
    case 'M': return 'modified';
    default: return 'modified';
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest gitMap`
Expected: PASS。

- [ ] **Step 5: 实现 src/extension/services/GitService.ts**

用 VSCode git 扩展 API（`vscode.git`）。从 `repo.state` 读分支、工作区变更；用 `repo.log()` 读最近提交。

```typescript
import * as vscode from 'vscode';
import { GitState, CommitInfo, FileChange, ReviewMode } from '../../shared/types';
import { mapStatusCode } from './gitMap';

interface GitExtensionApi {
  repositories: any[];
  getAPI(version: number): GitExtensionApi;
}

export class GitService {
  private api: any | null = null;

  private async ensureApi(): Promise<any | null> {
    if (this.api) return this.api;
    const ext = vscode.extensions.getExtension('vscode.git');
    if (!ext) return null;
    const exports = ext.isActive ? ext.exports : await ext.activate();
    this.api = exports.getAPI(1);
    return this.api;
  }

  private repo(): any | null {
    if (!this.api || this.api.repositories.length === 0) return null;
    return this.api.repositories[0];
  }

  async getState(_mode: ReviewMode): Promise<GitState> {
    await this.ensureApi();
    const repo = this.repo();
    if (!repo) {
      return { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] };
    }

    const head = repo.state.HEAD;
    const currentBranch = head?.name || '';

    const refs = await repo.getBranches({ remote: true });
    const branches: string[] = refs.map((r: any) => r.name).filter(Boolean);

    const commits = await repo.log({ maxEntries: 20 });
    const recentCommits: CommitInfo[] = commits.map((c: any) => ({
      sha: c.hash.slice(0, 7),
      message: c.message.split('\n')[0],
      relativeTime: formatRelative(c.authorDate),
    }));

    const changes = [
      ...repo.state.indexChanges,
      ...repo.state.workingTreeChanges,
      ...repo.state.untrackedChanges ?? [],
    ];
    const seen = new Set<string>();
    const workspaceFiles: FileChange[] = [];
    for (const ch of changes) {
      const path = vscode.workspace.asRelativePath(ch.uri);
      if (seen.has(path)) continue;
      seen.add(path);
      workspaceFiles.push({ path, status: mapStatusCode(letterFromStatus(ch.status)) });
    }

    return { branches, currentBranch, recentCommits, workspaceFiles };
  }
}

function letterFromStatus(status: number): string {
  // VSCode git Status 枚举：0 INDEX_MODIFIED,1 INDEX_ADDED,2 INDEX_DELETED,
  // 3 INDEX_RENAMED,5 MODIFIED,6 DELETED,7 UNTRACKED ...
  switch (status) {
    case 1: return 'A';
    case 2: case 6: return 'D';
    case 3: return 'R';
    case 7: return '?';
    default: return 'M';
  }
}

function formatRelative(date?: Date): string {
  if (!date) return '';
  const diff = Date.now() - date.getTime();
  const h = Math.floor(diff / 3.6e6);
  if (h < 1) return '刚刚';
  if (h < 24) return `${h} 小时前`;
  const d = Math.floor(h / 24);
  if (d === 1) return '昨天';
  return `${d} 天前`;
}
```

- [ ] **Step 6: 编译校验**

Run: `npx tsc -p tsconfig.extension.json --noEmit`
Expected: 无错误。

- [ ] **Step 7: Commit**

```bash
git add src/extension/services/gitMap.ts src/extension/services/GitService.ts src/extension/services/__tests__/gitMap.test.ts
git commit -m "feat: add GitService via vscode.git API"
```

---

## Task 7: ReviewSession — 状态机

管理一次审查的生命周期：状态转换 + 持有 CLI 调用 + 通过回调推送状态/日志/结果。状态转换逻辑纯可测。

**Files:**
- Create: `src/extension/services/ReviewSession.ts`
- Test: `src/extension/services/__tests__/ReviewSession.test.ts`

- [ ] **Step 1: 写失败测试 — 结果到状态的映射**

```typescript
// src/extension/services/__tests__/ReviewSession.test.ts
import { resultToState } from '../ReviewSession';

describe('resultToState', () => {
  it('有 comments → done', () => {
    expect(resultToState({ status: 'success', comments: [{} as any], warnings: [] })).toBe('done');
  });
  it('success 但无 comments → empty', () => {
    expect(resultToState({ status: 'success', comments: [], warnings: [] })).toBe('empty');
  });
  it('skipped 无 comments → empty', () => {
    expect(resultToState({ status: 'skipped', comments: [], warnings: [] })).toBe('empty');
  });
  it('completed_with_errors 无 comments → failed', () => {
    expect(resultToState({ status: 'completed_with_errors', comments: [], warnings: [] })).toBe('failed');
  });
  it('completed_with_errors 有 comments → done', () => {
    expect(resultToState({ status: 'completed_with_errors', comments: [{} as any], warnings: [] })).toBe('done');
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest ReviewSession`
Expected: FAIL。

- [ ] **Step 3: 实现 src/extension/services/ReviewSession.ts**

```typescript
import { CliResult, CliRunOptions, LogLine, ReviewState } from '../../shared/types';
import { CliService } from './CliService';

export function resultToState(result: CliResult): ReviewState {
  if (result.comments.length > 0) return 'done';
  if (result.status === 'completed_with_errors') return 'failed';
  return 'empty';
}

export interface SessionCallbacks {
  onState: (state: ReviewState) => void;
  onLog: (line: LogLine) => void;
  onDone: (result: CliResult) => void;
}

export class ReviewSession {
  private cancelled = false;

  constructor(private cli: CliService, private cwd: string) {}

  async run(opts: CliRunOptions, cb: SessionCallbacks): Promise<void> {
    this.cancelled = false;
    cb.onState('running');
    try {
      const result = await this.cli.review(opts, this.cwd, cb.onLog);
      if (this.cancelled) {
        cb.onState('cancelled');
        return;
      }
      cb.onState(resultToState(result));
      cb.onDone(result);
    } catch (e) {
      if (this.cancelled) {
        cb.onState('cancelled');
      } else {
        cb.onLog({ text: `[ocr] ${e instanceof Error ? e.message : String(e)}`, level: 'error' });
        cb.onState('failed');
      }
    }
  }

  cancel(cb: Pick<SessionCallbacks, 'onState'>): void {
    this.cancelled = true;
    this.cli.cancel();
    cb.onState('cancelled');
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest ReviewSession`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add src/extension/services/ReviewSession.ts src/extension/services/__tests__/ReviewSession.test.ts
git commit -m "feat: add ReviewSession state machine"
```

---

## Task 8: CommentProvider — Comment API

将 `ReviewComment[]` 渲染为 VSCode CommentThread，支持应用/忽略/误报、上一个/下一个、行号偏移追踪。参考 `aone-copilot-vscode/src/codeReview/codeReviewProvider.ts` 的成熟实现。VSCode API 重，以手动验收为主，仅对行号偏移纯函数做单测。

**Files:**
- Create: `src/extension/providers/lineOffset.ts`
- Create: `src/extension/providers/CommentProvider.ts`
- Test: `src/extension/providers/__tests__/lineOffset.test.ts`

- [ ] **Step 1: 写失败测试 — 行号偏移**

```typescript
// src/extension/providers/__tests__/lineOffset.test.ts
import { LineOffsetTracker } from '../lineOffset';

describe('LineOffsetTracker', () => {
  it('无变更时返回原行号', () => {
    const t = new LineOffsetTracker();
    expect(t.adjusted('a.ts', 10)).toBe(10);
  });
  it('在某行之前插入若干行，后续行号顺移', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 5, +2); // 第5行起增加2行
    expect(t.adjusted('a.ts', 10)).toBe(12);
    expect(t.adjusted('a.ts', 3)).toBe(3); // 之前的行不受影响
  });
  it('删除行使后续行号回退', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 5, -1);
    expect(t.adjusted('a.ts', 10)).toBe(9);
  });
  it('不同文件互不影响', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 1, +5);
    expect(t.adjusted('b.ts', 10)).toBe(10);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest lineOffset`
Expected: FAIL。

- [ ] **Step 3: 实现 src/extension/providers/lineOffset.ts**

```typescript
export class LineOffsetTracker {
  private records = new Map<string, Array<{ line: number; delta: number }>>();

  record(file: string, line: number, delta: number): void {
    const arr = this.records.get(file) ?? [];
    arr.push({ line, delta });
    this.records.set(file, arr);
  }

  adjusted(file: string, line: number): number {
    const arr = this.records.get(file) ?? [];
    let offset = 0;
    for (const r of arr) if (r.line < line) offset += r.delta;
    return Math.max(0, line + offset);
  }

  clear(): void {
    this.records.clear();
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest lineOffset`
Expected: PASS。

- [ ] **Step 5: 实现 src/extension/providers/CommentProvider.ts**

```typescript
import * as vscode from 'vscode';
import { ReviewComment, CommentStatus, CommentSyncState } from '../../shared/types';
import { COMMENT_CONTROLLER_ID } from '../../shared/constants';
import { LineOffsetTracker } from './lineOffset';

export class CommentProvider {
  private controller: vscode.CommentController;
  private threads: vscode.CommentThread[] = [];
  private comments: ReviewComment[] = [];
  private status = new Map<number, CommentStatus>();
  private offsets = new LineOffsetTracker();
  private syncListeners: Array<(s: CommentSyncState[]) => void> = [];

  constructor(private extensionUri: vscode.Uri) {
    this.controller = vscode.comments.createCommentController(COMMENT_CONTROLLER_ID, 'Open Code Review');
  }

  onSync(fn: (s: CommentSyncState[]) => void): void {
    this.syncListeners.push(fn);
  }

  private emitSync(): void {
    const states: CommentSyncState[] = this.comments.map((_, i) => ({
      index: i, status: this.status.get(i) ?? 'pending',
    }));
    this.syncListeners.forEach((fn) => fn(states));
  }

  async show(comments: ReviewComment[]): Promise<void> {
    this.clear();
    this.comments = [...comments].sort((a, b) =>
      a.path.localeCompare(b.path) || a.startLine - b.startLine || a.endLine - b.endLine);
    const root = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    if (!root) return;

    for (let i = 0; i < this.comments.length; i++) {
      const c = this.comments[i];
      try {
        const uri = vscode.Uri.file(`${root}/${c.path}`);
        const doc = await vscode.workspace.openTextDocument(uri);
        const range = new vscode.Range(Math.max(0, c.startLine - 1), 0, Math.max(0, c.endLine - 1), 0);
        const body = this.renderBody(c, i, 'pending');
        const thread = this.controller.createCommentThread(doc.uri, range, [{
          body, mode: vscode.CommentMode.Preview,
          author: { name: '⏳ [未处理]' },
        }]);
        thread.canReply = false;
        thread.label = `Code Review (${i + 1} / ${this.comments.length})`;
        thread.contextValue = COMMENT_CONTROLLER_ID;
        thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
        this.threads.push(thread);
        this.status.set(i, 'pending');
      } catch { /* 文件打不开则跳过 */ }
    }
    if (this.threads.length) await this.jumpTo(0);
    this.emitSync();
  }

  private renderBody(c: ReviewComment, index: number, status: CommentStatus): vscode.MarkdownString {
    let md = c.content;
    if (c.suggestionCode && c.suggestionCode.trim()) {
      md += `\n***\n\`\`\`diff\n${c.suggestionCode}\n\`\`\``;
    }
    if (status === 'pending') {
      const isLast = index === this.comments.length - 1;
      const arg = (cmd: string) => `command:${cmd}?${encodeURIComponent(JSON.stringify([index]))}`;
      const apply = isLast ? 'ocr.comment.apply' : 'ocr.comment.applyAndNext';
      const discard = isLast ? 'ocr.comment.discard' : 'ocr.comment.discardAndNext';
      const fp = isLast ? 'ocr.comment.falsePositive' : 'ocr.comment.falsePositiveAndNext';
      md += `\n\n---\n\n[应用](${arg(apply)}) | [忽略](${arg(discard)}) | [误报](${arg(fp)})`;
    }
    const s = new vscode.MarkdownString(md);
    s.isTrusted = true;
    return s;
  }

  async apply(index: number): Promise<void> {
    const c = this.comments[index];
    const root = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    if (!root) return;
    const uri = vscode.Uri.file(`${root}/${c.path}`);
    const doc = await vscode.workspace.openTextDocument(uri);
    const editor = await vscode.window.showTextDocument(doc);
    const before = doc.lineCount;
    const start = Math.max(0, this.offsets.adjusted(c.path, c.startLine) - 1);
    const end = Math.min(doc.lineCount - 1, this.offsets.adjusted(c.path, c.endLine) - 1);
    const range = new vscode.Range(start, 0, end, doc.lineAt(end).text.length);
    await editor.edit((e) => {
      if (c.suggestionCode && c.suggestionCode.trim()) e.replace(range, c.suggestionCode);
      else e.delete(range);
    });
    await doc.save();
    this.offsets.record(c.path, c.startLine, doc.lineCount - before);
    this.setStatus(index, 'applied');
  }

  discard(index: number): void { this.setStatus(index, 'discarded'); }
  falsePositive(index: number): void { this.setStatus(index, 'falsePositive'); }

  private setStatus(index: number, status: CommentStatus): void {
    this.status.set(index, status);
    const thread = this.threads[index];
    if (thread) {
      const label = { applied: '✅ [已应用]', discarded: '✅ [已忽略]', falsePositive: '✅ [已误报]', pending: '⏳ [未处理]' }[status];
      thread.comments = [{ ...thread.comments[0], author: { name: label }, body: this.renderBody(this.comments[index], index, status) }] as any;
      thread.collapsibleState = vscode.CommentThreadCollapsibleState.Collapsed;
    }
    this.emitSync();
  }

  async jumpTo(index: number): Promise<void> {
    const thread = this.threads[index];
    if (!thread) return;
    await vscode.window.showTextDocument(thread.uri, { selection: thread.range, preview: false });
    thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
  }

  next(index: number): void { if (index < this.threads.length - 1) this.jumpTo(index + 1); }
  prev(index: number): void { if (index > 0) this.jumpTo(index - 1); }

  clear(): void {
    this.threads.forEach((t) => t.dispose());
    this.threads = [];
    this.comments = [];
    this.status.clear();
    this.offsets.clear();
  }

  dispose(): void {
    this.clear();
    this.controller.dispose();
  }
}
```

- [ ] **Step 6: 编译校验**

Run: `npx tsc -p tsconfig.extension.json --noEmit`
Expected: 无错误。

- [ ] **Step 7: Commit**

```bash
git add src/extension/providers/lineOffset.ts src/extension/providers/CommentProvider.ts src/extension/providers/__tests__/lineOffset.test.ts
git commit -m "feat: add CommentProvider with line offset tracking"
```

---

## Task 9: WebView store（reducer，TDD）

WebView 状态容器的归约逻辑是纯函数，TDD 覆盖。

**Files:**
- Create: `src/webview/store.ts`
- Test: `src/webview/__tests__/store.test.ts`

注意：jest 默认用 `tsconfig.extension.json`（types: node/jest），store.ts 不引入 preact 之外的 DOM 依赖即可被 ts-jest 编译。reducer 与 React/Preact 无关，是纯 TS。

- [ ] **Step 1: 写失败测试 — reducer**

```typescript
// src/webview/__tests__/store.test.ts
import { initialState, reducer } from '../store';

describe('reducer', () => {
  it('init 设置 config 和 gitState', () => {
    const s = reducer(initialState, {
      type: 'init',
      config: { llm: { url: 'u', authToken: '', model: 'm', useAnthropic: false }, language: 'Chinese' },
      gitState: { branches: [], currentBranch: 'main', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.config?.llm.model).toBe('m');
    expect(s.gitState.currentBranch).toBe('main');
    expect(s.view).toBe('idle'); // 有 config → idle
  });

  it('init 时 config 为 null → 进入配置引导', () => {
    const s = reducer(initialState, {
      type: 'init', config: null,
      gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.view).toBe('config');
    expect(s.configView).toBe('empty');
  });

  it('stateChange running 清空旧日志并切到 running 视图', () => {
    const s = reducer({ ...initialState, logs: [{ text: 'old', level: 'info' }] }, { type: 'stateChange', state: 'running' });
    expect(s.session.state).toBe('running');
    expect(s.logs).toEqual([]);
    expect(s.view).toBe('running');
  });

  it('logLine 追加日志', () => {
    const s = reducer(initialState, { type: 'logLine', line: { text: 'x', level: 'info' } });
    expect(s.logs).toHaveLength(1);
  });

  it('reviewDone 保存结果', () => {
    const s = reducer(initialState, {
      type: 'reviewDone',
      result: { status: 'success', comments: [], warnings: [], summary: undefined },
    });
    expect(s.session.result?.status).toBe('success');
  });

  it('stateChange done → view 切 done', () => {
    expect(reducer(initialState, { type: 'stateChange', state: 'done' }).view).toBe('done');
  });

  it('commentSync 更新评论状态映射', () => {
    const s = reducer(initialState, { type: 'commentSync', comments: [{ index: 0, status: 'applied' }] });
    expect(s.commentStatus[0]).toBe('applied');
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest store`
Expected: FAIL。

- [ ] **Step 3: 实现 src/webview/store.ts**

```typescript
import { CliResult, CommentStatus, GitState, LogLine, OcrConfig, ReviewState } from '../shared/types';
import { HostToWebview } from '../shared/messages';

export type AppView = 'idle' | 'running' | 'done' | 'empty' | 'cancelled' | 'failed' | 'config';
export type ConfigView = 'empty' | 'list' | 'form';

export interface AppState {
  view: AppView;
  configView: ConfigView;
  config: OcrConfig | null;
  gitState: GitState;
  logs: LogLine[];
  session: { state: ReviewState; result: CliResult | null };
  commentStatus: Record<number, CommentStatus>;
}

export const initialState: AppState = {
  view: 'idle',
  configView: 'list',
  config: null,
  gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
  logs: [],
  session: { state: 'idle', result: null },
  commentStatus: {},
};

const STATE_TO_VIEW: Record<ReviewState, AppView> = {
  idle: 'idle', running: 'running', done: 'done',
  empty: 'empty', cancelled: 'cancelled', failed: 'failed',
};

export function reducer(state: AppState, msg: HostToWebview): AppState {
  switch (msg.type) {
    case 'init':
      return {
        ...state,
        config: msg.config,
        gitState: msg.gitState,
        view: msg.config ? 'idle' : 'config',
        configView: msg.config ? 'list' : 'empty',
      };
    case 'gitState':
      return { ...state, gitState: msg.gitState };
    case 'config':
      return { ...state, config: msg.config };
    case 'stateChange': {
      const logs = msg.state === 'running' ? [] : state.logs;
      return {
        ...state,
        logs,
        session: { ...state.session, state: msg.state },
        view: STATE_TO_VIEW[msg.state],
      };
    }
    case 'logLine':
      return { ...state, logs: [...state.logs, msg.line] };
    case 'reviewDone':
      return { ...state, session: { ...state.session, result: msg.result } };
    case 'commentSync': {
      const commentStatus = { ...state.commentStatus };
      for (const c of msg.comments) commentStatus[c.index] = c.status;
      return { ...state, commentStatus };
    }
    case 'connectionResult':
      return state;
    default:
      return state;
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest store`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add src/webview/store.ts src/webview/__tests__/store.test.ts
git commit -m "feat: add WebView state reducer"
```

---

## Task 10: WebView bridge + 入口骨架

**Files:**
- Create: `src/webview/bridge.ts`
- Create: `src/webview/index.tsx`
- Create: `src/webview/App.tsx`
- Create: `src/webview/styles/global.css`

- [ ] **Step 1: 写 bridge.ts**

```typescript
import { HostToWebview, WebviewToHost } from '../shared/messages';

interface VsCodeApi { postMessage(msg: unknown): void; }
declare function acquireVsCodeApi(): VsCodeApi;

const vscode = acquireVsCodeApi();

export const bridge = {
  post(msg: WebviewToHost): void {
    vscode.postMessage(msg);
  },
  onMessage(handler: (msg: HostToWebview) => void): void {
    window.addEventListener('message', (e) => handler(e.data as HostToWebview));
  },
};
```

- [ ] **Step 2: 写 styles/global.css**

从 `prototype.html` 的 `<style>` 复制 `:root` CSS 变量、全局 base、以及所有组件类（`.status-bar`, `.setup`, `.mode-tabs`, `.file-row`, `.log-viewer`, `.comment-card`, `.config-*` 等）。删除 demo-switcher 相关样式（`.demo-switcher`, `.demo-chip`, `.demo-sep`）和 editor-stub 相关（侧边栏不含编辑器区）。保留所有动画 keyframes（pulse / blink）。

验收：CSS 文件能被 css-loader 正常导入，无语法错误。

- [ ] **Step 3: 写 App.tsx（视图路由骨架）**

```tsx
import { useEffect, useReducer } from 'preact/hooks';
import { reducer, initialState, AppState } from './store';
import { bridge } from './bridge';
import './styles/global.css';

export function App() {
  const [state, dispatch] = useReducer(reducer, initialState);

  useEffect(() => {
    bridge.onMessage((msg) => dispatch(msg));
    bridge.post({ type: 'ready' });
  }, []);

  return (
    <div class="ocr-root">
      {/* StatusBar 与各视图在后续任务接入 */}
      <div data-view={state.view}>view: {state.view}</div>
    </div>
  );
}
```

- [ ] **Step 4: 写 index.tsx**

```tsx
import { render } from 'preact';
import { App } from './App';

const root = document.getElementById('root');
if (root) render(<App />, root);
```

- [ ] **Step 5: 构建 WebView 验证**

Run: `npx webpack --mode development --config-name webview`
Expected: 生成 `out/webview.js`，无编译错误。

- [ ] **Step 6: Commit**

```bash
git add src/webview/bridge.ts src/webview/index.tsx src/webview/App.tsx src/webview/styles/global.css
git commit -m "feat: add WebView bridge and app skeleton"
```

---

## Task 11: WebView 组件 — StatusBar / ModelDropdown / FileList / LogViewer / CommentCard

还原原型对应区块。组件为展示型（props 驱动 + 回调），以构建通过 + 手动验收为主。

**Files:**
- Create: `src/webview/components/StatusBar.tsx`
- Create: `src/webview/components/ModelDropdown.tsx`
- Create: `src/webview/components/FileList.tsx`
- Create: `src/webview/components/LogViewer.tsx`
- Create: `src/webview/components/CommentCard.tsx`

- [ ] **Step 1: StatusBar.tsx**

显示当前模型 + provider + 下拉触发 + 齿轮（打开配置）。对应原型 `.status-bar`。

```tsx
import { OcrConfig } from '../../shared/types';

interface Props {
  config: OcrConfig | null;
  onToggleDropdown: () => void;
  onOpenConfig: () => void;
}

export function StatusBar({ config, onToggleDropdown, onOpenConfig }: Props) {
  const model = config?.llm.model || '未配置';
  return (
    <div class="status-bar">
      <span class={`status-dot${config ? '' : ' dim'}`}></span>
      <span class="status-model">{model}</span>
      <button class="status-dropdown-trigger" onClick={onToggleDropdown}>▾</button>
      <span class="status-spacer"></span>
      <button class="status-gear" onClick={onOpenConfig}>⚙</button>
    </div>
  );
}
```

- [ ] **Step 2: ModelDropdown.tsx**

列出 config 中的模型（本设计仅单模型，展示当前模型；多模型留待 CLI 支持）。对应原型 `.model-dropdown`。

```tsx
import { OcrConfig } from '../../shared/types';

interface Props {
  config: OcrConfig | null;
  open: boolean;
  onSelect: (model: string) => void;
}

export function ModelDropdown({ config, open, onSelect }: Props) {
  if (!open || !config) return null;
  return (
    <div class="model-dropdown open">
      <div class="model-dropdown-item active" onClick={() => onSelect(config.llm.model)}>
        <span class="md-dot"></span>{config.llm.model}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: FileList.tsx**

展示待审查文件（来自 gitState.workspaceFiles），勾选仅为视觉确认。对应原型 `.file-list`。

```tsx
import { FileChange } from '../../shared/types';

const BADGE: Record<FileChange['status'], string> = {
  added: 'A', modified: 'M', deleted: 'D', renamed: 'R', binary: 'B',
};

interface Props { files: FileChange[]; }

export function FileList({ files }: Props) {
  return (
    <div class="file-list">
      <div class="files-label">待审查文件 ({files.length})</div>
      {files.map((f) => (
        <label class="file-row" key={f.path}>
          <input type="checkbox" checked />
          <span class="file-name">{f.path}</span>
          <span class={`file-badge ${f.status}`}>{BADGE[f.status]}</span>
        </label>
      ))}
    </div>
  );
}
```

- [ ] **Step 4: LogViewer.tsx**

流式日志展示。对应原型 `.log-viewer`。

```tsx
import { LogLine } from '../../shared/types';

interface Props { logs: LogLine[]; }

export function LogViewer({ logs }: Props) {
  return (
    <div class="log-viewer">
      {logs.map((l, i) => (
        <div class={`log-line ${l.level === 'warn' ? 'log-warn' : ''}`} key={i}>{l.text}</div>
      ))}
    </div>
  );
}
```

- [ ] **Step 5: CommentCard.tsx**

单条评论卡片，带打开/复制/忽略操作。对应原型 `.comment-card`。

```tsx
import { ReviewComment, CommentStatus } from '../../shared/types';

interface Props {
  comment: ReviewComment;
  index: number;
  status: CommentStatus;
  onOpen: (index: number) => void;
  onAction: (index: number, action: 'apply' | 'discard' | 'falsePositive') => void;
}

export function CommentCard({ comment, index, status, onOpen, onAction }: Props) {
  return (
    <div class={`comment-card${status !== 'pending' ? ' dismissed' : ''}`}>
      <div class="comment-header">
        <span class="comment-file">{comment.path}</span>
        <span class="comment-line">L{comment.startLine}</span>
      </div>
      <div class="comment-body">{comment.content}</div>
      <div class="comment-actions">
        <button onClick={() => onOpen(index)}>打开</button>
        <button onClick={() => onAction(index, 'apply')}>应用</button>
        <button onClick={() => onAction(index, 'discard')}>忽略</button>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: 构建验证**

Run: `npx webpack --mode development --config-name webview`
Expected: 无编译错误。

- [ ] **Step 7: Commit**

```bash
git add src/webview/components/
git commit -m "feat: add WebView presentational components"
```

---

## Task 12: WebView 视图 — Idle / Running / Done / Empty / Cancelled / Failed

组装组件成完整视图。对应原型各 `.action-*` 区块。

**Files:**
- Create: `src/webview/views/IdleView.tsx`
- Create: `src/webview/views/RunningView.tsx`
- Create: `src/webview/views/DoneView.tsx`
- Create: `src/webview/views/EmptyView.tsx`
- Create: `src/webview/views/CancelledView.tsx`
- Create: `src/webview/views/FailedView.tsx`

- [ ] **Step 1: IdleView.tsx**

模式切换 tab + 模式参数 + 文件列表 + 自定义 prompt + 开始按钮。对应原型 `.setup`。

```tsx
import { useState } from 'preact/hooks';
import { GitState, ReviewMode, CliRunOptions } from '../../shared/types';
import { FileList } from '../components/FileList';

interface Props {
  gitState: GitState;
  onModeChange: (mode: ReviewMode) => void;
  onStart: (options: CliRunOptions) => void;
}

export function IdleView({ gitState, onModeChange, onStart }: Props) {
  const [mode, setMode] = useState<ReviewMode>('workspace');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [commit, setCommit] = useState('');
  const [prompt, setPrompt] = useState('');

  const switchMode = (m: ReviewMode) => { setMode(m); onModeChange(m); };

  return (
    <div class="setup">
      <div class="setup-label">新建审查</div>
      <div class="mode-tabs">
        {(['workspace', 'branch', 'commit'] as ReviewMode[]).map((m) => (
          <button key={m} class={`mode-tab${mode === m ? ' active' : ''}`} onClick={() => switchMode(m)}>
            {m === 'workspace' ? '工作区' : m === 'branch' ? '分支对比' : '单次提交'}
          </button>
        ))}
      </div>

      {mode === 'branch' && (
        <div class="mode-params active">
          <div class="mode-param-label">基础引用</div>
          <select class="mode-param-select" value={from} onChange={(e) => setFrom((e.target as HTMLSelectElement).value)}>
            {gitState.branches.map((b) => <option key={b} value={b}>{b}</option>)}
          </select>
          <div class="mode-param-label">目标引用</div>
          <select class="mode-param-select" value={to} onChange={(e) => setTo((e.target as HTMLSelectElement).value)}>
            {gitState.branches.map((b) => <option key={b} value={b}>{b}</option>)}
          </select>
        </div>
      )}

      {mode === 'commit' && (
        <div class="mode-params active">
          <div class="commit-list">
            {gitState.recentCommits.map((c) => (
              <label key={c.sha} class={`commit-row${commit === c.sha ? ' active' : ''}`} onClick={() => setCommit(c.sha)}>
                <input type="radio" name="commit" class="commit-radio" checked={commit === c.sha} />
                <div class="commit-info">
                  <div class="commit-msg">{c.message}</div>
                  <div class="commit-meta"><span class="commit-sha">{c.sha}</span> · {c.relativeTime}</div>
                </div>
              </label>
            ))}
          </div>
        </div>
      )}

      <FileList files={gitState.workspaceFiles} />

      <textarea class="mode-param-input" rows={3} placeholder="自定义审查提示词（可选）"
        value={prompt} onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)} />

      <button class="primary-btn" onClick={() => onStart({ mode, from, to, commit, customPrompt: prompt })}>
        审查所有变更
      </button>
    </div>
  );
}
```

- [ ] **Step 2: RunningView.tsx**

```tsx
import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';

interface Props { logs: LogLine[]; onCancel: () => void; }

export function RunningView({ logs, onCancel }: Props) {
  return (
    <div class="action-running" style="display:block">
      <LogViewer logs={logs} />
      <button class="cancel-pill" onClick={onCancel}>取消</button>
      <div style="clear:both"></div>
    </div>
  );
}
```

- [ ] **Step 3: DoneView.tsx**

```tsx
import { CliResult, CommentStatus } from '../../shared/types';
import { CommentCard } from '../components/CommentCard';

interface Props {
  result: CliResult;
  commentStatus: Record<number, CommentStatus>;
  onOpen: (index: number) => void;
  onAction: (index: number, action: 'apply' | 'discard' | 'falsePositive') => void;
}

export function DoneView({ result, commentStatus, onOpen, onAction }: Props) {
  const s = result.summary;
  return (
    <div class="action-done" style="display:block">
      <div class="done-summary">
        <span class="ds-dot"></span>
        <span>{result.comments.length} 条评论 · {s?.filesReviewed ?? 0} 个文件 · {s?.elapsed ?? ''}</span>
      </div>
      {result.comments.map((c, i) => (
        <CommentCard key={i} comment={c} index={i}
          status={commentStatus[i] ?? 'pending'} onOpen={onOpen} onAction={onAction} />
      ))}
    </div>
  );
}
```

- [ ] **Step 4: EmptyView / CancelledView / FailedView**

```tsx
// EmptyView.tsx
export function EmptyView() {
  return (
    <div class="action-empty" style="display:block">
      <div class="empty-note">
        <div class="en-dot"></div>
        <div class="en-text">未发现问题 · 已通过</div>
      </div>
    </div>
  );
}
```

```tsx
// CancelledView.tsx
export function CancelledView() {
  return (
    <div class="action-cancelled" style="display:block">
      <div class="cancelled-note">审查已取消</div>
    </div>
  );
}
```

```tsx
// FailedView.tsx
interface Props { onRetry: () => void; }
export function FailedView({ onRetry }: Props) {
  return (
    <div class="action-failed" style="display:block">
      <div class="failed-card">
        <div class="fc-msg">审查失败。<br/>请检查 API Key 和网络连接。</div>
        <button class="retry-pill" onClick={onRetry}>重试</button>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: 构建验证**

Run: `npx webpack --mode development --config-name webview`
Expected: 无编译错误。

- [ ] **Step 6: Commit**

```bash
git add src/webview/views/
git commit -m "feat: add WebView review-flow views"
```

---

## Task 13: WebView ConfigView + 接线 App

配置视图（引导/列表/表单）+ 把所有视图接进 App，串起 bridge 消息流。

**Files:**
- Create: `src/webview/views/ConfigView.tsx`
- Modify: `src/webview/App.tsx`

- [ ] **Step 1: ConfigView.tsx**

三态：引导（empty）/ 列表（list）/ 表单（form）。对接 setConfig。对应原型 `.config-*`。

```tsx
import { useState } from 'preact/hooks';
import { OcrConfig } from '../../shared/types';
import { ConfigView as ConfigViewMode } from '../store';

interface Props {
  mode: ConfigViewMode;
  config: OcrConfig | null;
  onSetMode: (m: ConfigViewMode) => void;
  onSave: (entries: { key: string; value: string }[]) => void;
  onClose: () => void;
}

export function ConfigView({ mode, config, onSetMode, onSave, onClose }: Props) {
  const [url, setUrl] = useState(config?.llm.url ?? '');
  const [token, setToken] = useState(config?.llm.authToken ?? '');
  const [model, setModel] = useState(config?.llm.model ?? '');
  const [useAnthropic, setUseAnthropic] = useState(config?.llm.useAnthropic ?? false);

  if (mode === 'empty') {
    return (
      <div class="config-empty" style="display:block">
        <div class="ce-dot"></div>
        <div class="ce-label">配置</div>
        <div class="ce-title">连接模型以开始使用</div>
        <div class="ce-desc">添加一个 LLM 提供商和 API Key 来开始代码审查。</div>
        <button class="ce-btn" onClick={() => onSetMode('form')}>+ 添加提供商</button>
      </div>
    );
  }

  if (mode === 'list') {
    return (
      <div class="config-list" style="display:block">
        <div class="config-list-header">
          <span class="config-list-title">提供商</span>
          <button class="config-list-close" onClick={onClose}>×</button>
        </div>
        {config && (
          <div class="provider-card" onClick={() => onSetMode('form')}>
            <span class="pc-name">{config.llm.url || '当前配置'}</span>
            <span class="pc-models">{config.llm.model}</span>
            <span class="pc-active"></span>
          </div>
        )}
        <button class="config-add-btn" onClick={() => onSetMode('form')}>+ 编辑配置</button>
      </div>
    );
  }

  // form
  return (
    <div class="config-form" style="display:block">
      <div class="config-form-header"><span class="config-form-title">配置提供商</span></div>
      <div class="form-group">
        <label class="form-label">接口地址</label>
        <input class="form-input" value={url} onInput={(e) => setUrl((e.target as HTMLInputElement).value)} placeholder="https://api.anthropic.com/v1/messages" />
      </div>
      <div class="form-group">
        <label class="form-label">API 密钥</label>
        <input class="form-input" type="password" value={token} onInput={(e) => setToken((e.target as HTMLInputElement).value)} placeholder="sk-..." />
      </div>
      <div class="form-group">
        <label class="form-label">模型</label>
        <input class="form-input" value={model} onInput={(e) => setModel((e.target as HTMLInputElement).value)} placeholder="claude-opus-4-6" />
      </div>
      <div class="toggle-row">
        <span class="toggle-label">使用 Anthropic 协议</span>
        <button class={`toggle-switch${useAnthropic ? ' on' : ''}`} onClick={() => setUseAnthropic(!useAnthropic)}>
          <span class="toggle-knob"></span>
        </button>
      </div>
      <div class="form-actions">
        <button class="btn-cancel" onClick={() => onSetMode('list')}>取消</button>
        <button class="btn-save" onClick={() => onSave([
          { key: 'llm.url', value: url },
          { key: 'llm.auth_token', value: token },
          { key: 'llm.model', value: model },
          { key: 'llm.use_anthropic', value: String(useAnthropic) },
        ])}>保存</button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: 重写 App.tsx 接线所有视图**

```tsx
import { useEffect, useReducer, useState } from 'preact/hooks';
import { reducer, initialState, ConfigView as ConfigViewMode } from './store';
import { bridge } from './bridge';
import { ReviewMode, CliRunOptions } from '../shared/types';
import { StatusBar } from './components/StatusBar';
import { ModelDropdown } from './components/ModelDropdown';
import { IdleView } from './views/IdleView';
import { RunningView } from './views/RunningView';
import { DoneView } from './views/DoneView';
import { EmptyView } from './views/EmptyView';
import { CancelledView } from './views/CancelledView';
import { FailedView } from './views/FailedView';
import { ConfigView } from './views/ConfigView';
import './styles/global.css';

export function App() {
  const [state, dispatch] = useReducer(reducer, initialState);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [configMode, setConfigMode] = useState<ConfigViewMode>('list');

  useEffect(() => {
    bridge.onMessage((msg) => dispatch(msg));
    bridge.post({ type: 'ready' });
  }, []);

  const start = (options: CliRunOptions) => bridge.post({ type: 'startReview', options });
  const onModeChange = (mode: ReviewMode) => bridge.post({ type: 'getGitState', mode });

  const showConfig = state.view === 'config';
  const effectiveConfigMode = showConfig ? state.configView : configMode;

  return (
    <div class="ocr-root">
      <StatusBar
        config={state.config}
        onToggleDropdown={() => setDropdownOpen((v) => !v)}
        onOpenConfig={() => { setConfigMode('list'); dispatch({ type: 'init', config: state.config, gitState: state.gitState }); }}
      />
      <ModelDropdown config={state.config} open={dropdownOpen}
        onSelect={(m) => { bridge.post({ type: 'setConfig', key: 'llm.model', value: m }); setDropdownOpen(false); }} />

      {showConfig ? (
        <ConfigView
          mode={effectiveConfigMode}
          config={state.config}
          onSetMode={(m) => dispatch({ type: 'init', config: state.config, gitState: state.gitState }) || setConfigMode(m)}
          onSave={(entries) => entries.forEach((e) => bridge.post({ type: 'setConfig', key: e.key, value: e.value }))}
          onClose={() => dispatch({ type: 'stateChange', state: 'idle' })}
        />
      ) : (
        <div class="action-region">
          {state.view === 'idle' && <IdleView gitState={state.gitState} onModeChange={onModeChange} onStart={start} />}
          {state.view === 'running' && <RunningView logs={state.logs} onCancel={() => bridge.post({ type: 'cancelReview' })} />}
          {state.view === 'done' && state.session.result && (
            <DoneView result={state.session.result} commentStatus={state.commentStatus}
              onOpen={(i) => bridge.post({ type: 'jumpToComment', index: i })}
              onAction={(i, action) => bridge.post({ type: 'commentAction', index: i, action })} />
          )}
          {state.view === 'empty' && <EmptyView />}
          {state.view === 'cancelled' && <CancelledView />}
          {state.view === 'failed' && <FailedView onRetry={() => start({ mode: 'workspace' })} />}
        </div>
      )}
    </div>
  );
}
```

注意：上面 App 中配置视图的开合用了简化处理。实现时若发现 configMode 与 store 的 configView 协调复杂，可把 configView 完全收敛到 store（在 reducer 增加 `setConfigView` / `openConfig` / `closeConfig` action），App 只读不本地维护。这是允许的重构，目标是「打开齿轮→列表，空配置→引导，保存后→列表，关闭→idle」。

- [ ] **Step 3: 构建验证**

Run: `npx webpack --mode development --config-name webview`
Expected: 无编译错误，生成 `out/webview.js`。

- [ ] **Step 4: 运行全部单测回归**

Run: `npx jest`
Expected: 所有既有测试 PASS。

- [ ] **Step 5: Commit**

```bash
git add src/webview/views/ConfigView.tsx src/webview/App.tsx
git commit -m "feat: wire WebView views and config flow"
```

---

## Task 14: SidebarProvider — WebView 宿主

加载 webview.js + global.css，建立与 WebView 的消息桥，分发到各 service。

**Files:**
- Create: `src/extension/providers/SidebarProvider.ts`

- [ ] **Step 1: 实现 SidebarProvider.ts**

```typescript
import * as vscode from 'vscode';
import { SIDEBAR_VIEW_ID } from '../../shared/constants';
import { HostToWebview, WebviewToHost } from '../../shared/messages';
import { CliService } from '../services/CliService';
import { ConfigService } from '../services/ConfigService';
import { GitService } from '../services/GitService';
import { ReviewSession } from '../services/ReviewSession';
import { CommentProvider } from './CommentProvider';

export class SidebarProvider implements vscode.WebviewViewProvider {
  private view?: vscode.WebviewView;
  private session?: ReviewSession;

  constructor(
    private extensionUri: vscode.Uri,
    private cli: CliService,
    private config: ConfigService,
    private git: GitService,
    private comments: CommentProvider,
  ) {
    this.comments.onSync((states) => this.post({ type: 'commentSync', comments: states }));
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = { enableScripts: true, localResourceRoots: [this.extensionUri] };
    view.webview.html = this.html(view.webview);
    view.webview.onDidReceiveMessage((msg: WebviewToHost) => this.handle(msg));
  }

  private post(msg: HostToWebview): void {
    this.view?.webview.postMessage(msg);
  }

  private async handle(msg: WebviewToHost): Promise<void> {
    const cwd = vscode.workspace.workspaceFolders?.[0].uri.fsPath ?? process.cwd();
    switch (msg.type) {
      case 'ready': {
        const config = this.config.read();
        const gitState = await this.git.getState('workspace');
        this.post({ type: 'init', config, gitState });
        break;
      }
      case 'getGitState': {
        this.post({ type: 'gitState', gitState: await this.git.getState(msg.mode) });
        break;
      }
      case 'startReview': {
        this.session = new ReviewSession(this.cli, cwd);
        await this.session.run(msg.options, {
          onState: (state) => this.post({ type: 'stateChange', state }),
          onLog: (line) => this.post({ type: 'logLine', line }),
          onDone: (result) => {
            this.post({ type: 'reviewDone', result });
            if (result.comments.length) this.comments.show(result.comments);
          },
        });
        break;
      }
      case 'cancelReview':
        this.session?.cancel({ onState: (state) => this.post({ type: 'stateChange', state }) });
        break;
      case 'getConfig':
        this.post({ type: 'config', config: this.config.read() });
        break;
      case 'setConfig':
        await this.config.set(msg.key, msg.value);
        this.post({ type: 'config', config: this.config.read() });
        break;
      case 'testConnection': {
        const r = await this.cli.testConnection();
        this.post({ type: 'connectionResult', ok: r.ok, message: r.message });
        break;
      }
      case 'jumpToComment':
        await this.comments.jumpTo(msg.index);
        break;
      case 'commentAction':
        if (msg.action === 'apply') await this.comments.apply(msg.index);
        else if (msg.action === 'discard') this.comments.discard(msg.index);
        else this.comments.falsePositive(msg.index);
        break;
    }
  }

  private html(webview: vscode.Webview): string {
    const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(this.extensionUri, 'out', 'webview.js'));
    const nonce = String(Date.now());
    return `<!DOCTYPE html>
<html lang="zh-CN"><head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
</head><body><div id="root"></div>
<script nonce="${nonce}" src="${scriptUri}"></script>
</body></html>`;
  }
}
```

注意：CSS 通过 style-loader 注入（webview.js 内联），故 head 无需单独 link，但 CSP 的 style-src 需含 'unsafe-inline'（style-loader 用 inline `<style>`）。已包含。

- [ ] **Step 2: 编译校验**

Run: `npx tsc -p tsconfig.extension.json --noEmit`
Expected: 无错误。

- [ ] **Step 3: Commit**

```bash
git add src/extension/providers/SidebarProvider.ts
git commit -m "feat: add SidebarProvider hosting the WebView"
```

---

## Task 15: commands + activate 入口 + 公共导出

注册命令，把 CommentThread 内的导航/操作命令接到 CommentProvider；提供 `activateOcr` 公共入口与独立 `extension.ts`。

**Files:**
- Create: `src/extension/commands.ts`
- Create: `src/extension/index.ts`
- Create: `src/extension/extension.ts`

- [ ] **Step 1: 实现 src/extension/index.ts（公共导出 + activateOcr）**

```typescript
import * as vscode from 'vscode';
import { SIDEBAR_VIEW_ID } from '../shared/constants';
import { CliService } from './services/CliService';
import { ConfigService } from './services/ConfigService';
import { GitService } from './services/GitService';
import { CommentProvider } from './providers/CommentProvider';
import { SidebarProvider } from './providers/SidebarProvider';
import { registerCommands } from './commands';

export interface OcrAdapter {
  extensionUri?: vscode.Uri;
  telemetry?: (event: string, data: Record<string, unknown>) => void;
  cliPath?: string;
}

let disposables: vscode.Disposable[] = [];

export function activateOcr(context: vscode.ExtensionContext, adapter: OcrAdapter = {}): void {
  const extensionUri = adapter.extensionUri ?? context.extensionUri;
  const cli = new CliService(adapter.cliPath ?? 'ocr');
  const config = new ConfigService(cli);
  const git = new GitService();
  const comments = new CommentProvider(extensionUri);

  const sidebar = new SidebarProvider(extensionUri, cli, config, git, comments);
  const viewReg = vscode.window.registerWebviewViewProvider(SIDEBAR_VIEW_ID, sidebar);

  const cmdReg = registerCommands(comments);

  disposables.push(viewReg, cmdReg, comments);
  context.subscriptions.push(...disposables);
}

export function deactivateOcr(): void {
  disposables.forEach((d) => d.dispose());
  disposables = [];
}
```

- [ ] **Step 2: 实现 src/extension/commands.ts**

```typescript
import * as vscode from 'vscode';
import { COMMANDS } from '../shared/constants';
import { CommentProvider } from './providers/CommentProvider';

export function registerCommands(comments: CommentProvider): vscode.Disposable {
  const subs: vscode.Disposable[] = [];
  const reg = (id: string, fn: (...args: any[]) => any) =>
    subs.push(vscode.commands.registerCommand(id, fn));

  reg(COMMANDS.commentApply, (i: number) => comments.apply(i));
  reg(COMMANDS.commentApplyAndNext, async (i: number) => { await comments.apply(i); comments.next(i); });
  reg(COMMANDS.commentDiscard, (i: number) => comments.discard(i));
  reg(COMMANDS.commentDiscardAndNext, (i: number) => { comments.discard(i); comments.next(i); });
  reg(COMMANDS.commentFalsePositive, (i: number) => comments.falsePositive(i));
  reg(COMMANDS.commentFalsePositiveAndNext, (i: number) => { comments.falsePositive(i); comments.next(i); });
  reg(COMMANDS.commentPrev, (thread: vscode.CommentThread) => {
    const idx = findIndex(comments, thread);
    if (idx >= 0) comments.prev(idx);
  });
  reg(COMMANDS.commentNext, (thread: vscode.CommentThread) => {
    const idx = findIndex(comments, thread);
    if (idx >= 0) comments.next(idx);
  });

  return vscode.Disposable.from(...subs);
}

function findIndex(comments: CommentProvider, thread: vscode.CommentThread): number {
  // CommentProvider 暴露一个按 thread 查 index 的方法（见下）
  return comments.indexOfThread(thread);
}
```

注意：commands.ts 用到 `comments.indexOfThread(thread)`。在 `CommentProvider` 中补一个公共方法：

```typescript
  indexOfThread(thread: vscode.CommentThread): number {
    return this.threads.indexOf(thread);
  }
```

把这个方法加到 Task 8 创建的 `CommentProvider` 类里（在 `clear()` 之前）。

- [ ] **Step 3: 实现 src/extension/extension.ts（独立运行入口）**

```typescript
import * as vscode from 'vscode';
import { activateOcr, deactivateOcr } from './index';

export function activate(context: vscode.ExtensionContext): void {
  activateOcr(context);
}

export function deactivate(): void {
  deactivateOcr();
}
```

- [ ] **Step 4: 把 indexOfThread 补进 CommentProvider**

Modify: `src/extension/providers/CommentProvider.ts` —— 在类中新增 `indexOfThread` 公共方法（如 Step 2 所示）。

- [ ] **Step 5: 全量编译 + 构建**

Run: `npx tsc -p tsconfig.extension.json --noEmit && npx webpack --mode development`
Expected: extension 与 webview 两个产物都生成，无错误。

- [ ] **Step 6: 运行全部单测**

Run: `npx jest`
Expected: 全部 PASS。

- [ ] **Step 7: Commit**

```bash
git add src/extension/index.ts src/extension/commands.ts src/extension/extension.ts src/extension/providers/CommentProvider.ts
git commit -m "feat: add commands, activate entry, public exports"
```

---

## Task 16: 手动验收 — 独立运行端到端

无自动化，按步骤在 VSCode 中手动验证核心流程。前置：本机已安装 `ocr` CLI 并配置好可用的 LLM。

**Files:**
- Create: `.vscode/launch.json`

- [ ] **Step 1: 写 .vscode/launch.json**

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Run Extension",
      "type": "extensionHost",
      "request": "launch",
      "args": ["--extensionDevelopmentPath=${workspaceFolder}"],
      "outFiles": ["${workspaceFolder}/out/**/*.js"],
      "preLaunchTask": "npm: compile"
    }
  ]
}
```

注：若无 tasks.json，先手动 `yarn compile` 再按 F5；或移除 `preLaunchTask` 字段。

- [ ] **Step 2: 验收清单（手动）**

按 F5 启动 Extension Development Host，打开一个有 git 变更的项目，依次验证：

1. 活动栏出现 OCR 图标，点击打开侧边栏，WebView 正常渲染（silent-night 风格）。
2. 状态栏显示当前模型；点齿轮进入配置；若无 config 则显示引导页。
3. 配置表单填写并保存 → 调用 `ocr config set` → 状态栏模型更新。
4. 工作区模式：文件列表显示 git 变更；点「审查所有变更」→ 进入 running，日志流式滚动。
5. 审查完成有评论 → done 视图列出评论卡片；编辑器中出现 CommentThread。
6. 点评论卡片「打开」→ 跳转到对应文件行；CommentThread 展开。
7. 编辑器内点「应用」→ 代码被替换，卡片/线程状态变为已应用，侧边栏同步。
8. 取消：审查中点「取消」→ 进入 cancelled 视图，CLI 进程被杀。
9. 分支对比 / 单次提交模式：参数区正确显示分支/提交列表。
10. 无变更时审查 → empty 视图；CLI 失败（如断网）→ failed 视图 + 重试。

- [ ] **Step 3: 修复验收中发现的问题**

针对清单中未通过项逐一修复，每个修复独立 commit。

- [ ] **Step 4: Commit**

```bash
git add .vscode/launch.json
git commit -m "chore: add launch config for manual testing"
```

---

## Task 17: sync-to-aone.js 同步脚本

把核心源码复制进 `aone-copilot-vscode/src/ocr/`，并幂等 merge contributes 进 aone 的 package.json。

**Files:**
- Create: `scripts/sync-to-aone.js`
- Test: `scripts/__tests__/mergeContributes.test.ts`
- Create: `scripts/mergeContributes.js`

- [ ] **Step 1: 写失败测试 — contributes 幂等 merge**

把可测的 merge 逻辑抽到 `scripts/mergeContributes.js`（CommonJS，可被 ts-jest 通过 require 测）。

```typescript
// scripts/__tests__/mergeContributes.test.ts
const { mergeContributes } = require('../mergeContributes');

describe('mergeContributes', () => {
  const ocr = {
    commands: [{ command: 'ocr.review.start', title: 'x' }],
    views: { 'ocr-container': [{ id: 'ocr.sidebar', type: 'webview', name: 'CR' }] },
  };

  it('把 ocr 命令并入空 aone contributes', () => {
    const result = mergeContributes({}, ocr);
    expect(result.commands).toHaveLength(1);
    expect(result.commands[0].command).toBe('ocr.review.start');
  });

  it('幂等：重复 merge 不产生重复条目', () => {
    const once = mergeContributes({}, ocr);
    const twice = mergeContributes(once, ocr);
    expect(twice.commands).toHaveLength(1);
  });

  it('保留 aone 自己的非 ocr 命令', () => {
    const aone = { commands: [{ command: 'aone.foo', title: 'foo' }] };
    const result = mergeContributes(aone, ocr);
    expect(result.commands.map((c) => c.command).sort()).toEqual(['aone.foo', 'ocr.review.start']);
  });

  it('再次 merge 时移除旧 ocr.* 命令后重插（更新场景）', () => {
    const aone = { commands: [{ command: 'ocr.old.removed', title: 'old' }, { command: 'aone.foo', title: 'foo' }] };
    const result = mergeContributes(aone, ocr);
    const cmds = result.commands.map((c) => c.command).sort();
    expect(cmds).toEqual(['aone.foo', 'ocr.review.start']); // 旧的 ocr.old.removed 被清掉
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `npx jest mergeContributes`
Expected: FAIL。

- [ ] **Step 3: 实现 scripts/mergeContributes.js**

```js
const OCR_PREFIX = 'ocr.';
const OCR_VIEW_CONTAINER = 'ocr-container';

function isOcrCommand(c) { return c.command && c.command.startsWith(OCR_PREFIX); }

function mergeContributes(aone, ocr) {
  const result = JSON.parse(JSON.stringify(aone || {}));

  // commands：移除所有旧 ocr.* 再并入新的
  const aoneCommands = (result.commands || []).filter((c) => !isOcrCommand(c));
  result.commands = [...aoneCommands, ...(ocr.commands || [])];

  // views：替换 ocr-container 这一组
  result.views = result.views || {};
  if (ocr.views && ocr.views[OCR_VIEW_CONTAINER]) {
    result.views[OCR_VIEW_CONTAINER] = ocr.views[OCR_VIEW_CONTAINER];
  }

  // viewsContainers.activitybar：移除旧 ocr-container 再并入
  if (ocr.viewsContainers) {
    result.viewsContainers = result.viewsContainers || {};
    const aoneBar = (result.viewsContainers.activitybar || []).filter((v) => v.id !== OCR_VIEW_CONTAINER);
    const ocrBar = (ocr.viewsContainers.activitybar || []);
    result.viewsContainers.activitybar = [...aoneBar, ...ocrBar];
  }

  // menus.comments/commentThread/title：移除旧 ocr.* 命令项再并入
  if (ocr.menus && ocr.menus['comments/commentThread/title']) {
    result.menus = result.menus || {};
    const key = 'comments/commentThread/title';
    const aoneMenu = (result.menus[key] || []).filter((m) => !isOcrCommand(m));
    result.menus[key] = [...aoneMenu, ...ocr.menus[key]];
  }

  return result;
}

module.exports = { mergeContributes };
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npx jest mergeContributes`
Expected: PASS。

- [ ] **Step 5: 实现 scripts/sync-to-aone.js**

```js
#!/usr/bin/env node
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');
const { mergeContributes } = require('./mergeContributes');

function parseArgs() {
  const args = process.argv.slice(2);
  const out = { target: '../aone-copilot-vscode/src/ocr' };
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--target') out.target = args[++i];
  }
  return out;
}

function copyDir(src, dest, skip) {
  fs.mkdirSync(dest, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    if (skip(entry.name)) continue;
    const s = path.join(src, entry.name);
    const d = path.join(dest, entry.name);
    if (entry.isDirectory()) copyDir(s, d, skip);
    else fs.copyFileSync(s, d);
  }
}

function main() {
  const { target } = parseArgs();
  const repoRoot = path.resolve(__dirname, '..');
  const srcDir = path.join(repoRoot, 'src');
  const targetDir = path.resolve(repoRoot, target);

  // 1. 清空目标
  fs.rmSync(targetDir, { recursive: true, force: true });

  // 2. copy src/（排除独立入口、测试文件）
  const skip = (name) =>
    name === 'extension.ts' ||         // 独立运行入口不复制（aone 自己调 activateOcr）
    name === '__tests__' ||
    name.endsWith('.test.ts');
  copyDir(srcDir, targetDir, skip);

  // 3. merge contributes 到 aone package.json
  const ocrPkg = JSON.parse(fs.readFileSync(path.join(repoRoot, 'package.json'), 'utf8'));
  const aonePkgPath = path.resolve(targetDir, '..', '..', 'package.json'); // aone 根 package.json
  const aonePkg = JSON.parse(fs.readFileSync(aonePkgPath, 'utf8'));
  aonePkg.contributes = mergeContributes(aonePkg.contributes || {}, ocrPkg.contributes || {});
  fs.writeFileSync(aonePkgPath, JSON.stringify(aonePkg, null, 2) + '\n');

  // 4. 写来源版本标记
  let commit = 'unknown';
  try { commit = execSync('git rev-parse HEAD', { cwd: repoRoot }).toString().trim(); } catch {}
  fs.writeFileSync(path.join(targetDir, '.synced-version'), `${commit}\n${new Date().toISOString()}\n`);

  console.log(`Synced to ${targetDir} (from ${commit.slice(0, 7)})`);
  console.log('提醒：aone 需在 src/extension.ts 中调用 activateOcr(ctx, {...})，并在 webpack 加 webview 入口。');
}

main();
```

注意：步骤 3 中独立入口 `extension.ts` 被排除复制，aone 改为自己调用 `activateOcr`（来自复制进去的 `extension/index.ts`）。aone 的 webview 构建：在 aone webpack 增加一个入口指向 `src/ocr/webview/index.tsx`，产物命名为 `webview.js` 放到 aone 的 `out/`，使 SidebarProvider 的 `asWebviewUri('out/webview.js')` 能命中。

- [ ] **Step 6: 加 npm script**

Modify: `package.json` 的 scripts 增加：
```json
    "sync:aone": "node scripts/sync-to-aone.js",
```

- [ ] **Step 7: 干跑验证（不实际写 aone）**

手动验证脚本逻辑：临时把 `--target` 指到一个临时目录并造一个假的上级 package.json，运行 `node scripts/sync-to-aone.js --target /tmp/ocr-sync-test/src/ocr`，检查：
- 目标目录有 src 内容、无 extension.ts、无 test 文件
- 临时 package.json 的 contributes 被正确 merge
- 有 `.synced-version` 文件

验收后清理临时目录。

- [ ] **Step 8: Commit**

```bash
git add scripts/ package.json
git commit -m "feat: add sync-to-aone script with idempotent contributes merge"
```

---

## Task 18: 文档与收尾

**Files:**
- Create: `README.md`

- [ ] **Step 1: 写 README.md**

包含：
- 项目简介（基于 ocr CLI 的 VSCode 审查插件）
- 前置依赖：全局安装 `ocr` CLI（`npm i -g @alibaba-group/open-code-review`）并配置 LLM
- 开发：`yarn install` → `yarn compile` → F5
- 构建：`yarn build`
- 复用到 aone：`yarn sync:aone --target ../aone-copilot-vscode/src/ocr`，并说明 aone 侧需做的三步适配（调 activateOcr、webpack 加 webview 入口、merge 已自动）
- 架构图（引用 spec）

- [ ] **Step 2: 最终全量回归**

Run: `npx jest && npx tsc -p tsconfig.extension.json --noEmit && npx webpack --mode production`
Expected: 测试全过、类型无误、生产构建成功。

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add project README"
```

---

## 完成标准

- 所有 Jest 单测通过（cliParse / CliService / configParse / gitMap / ReviewSession / lineOffset / store / mergeContributes）。
- 独立插件 F5 可运行，Task 16 验收清单全部通过。
- `yarn build` 生产构建成功，产物含 `out/extension.js` + `out/webview.js`。
- `yarn sync:aone` 能把代码复制进 aone 并幂等 merge contributes。
- 原型中的全部功能（三模式、文件预览、自定义 prompt、流式日志、结果展示+双向同步、空/取消/失败、配置管理、模型切换、连通性测试）均已实现。
