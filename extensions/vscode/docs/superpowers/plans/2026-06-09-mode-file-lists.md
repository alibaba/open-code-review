# 三 Tab 文件列表按模式加载 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让三个 Tab(工作区/分支对比/单次提交)各自展示与其审查范围一致的 diff 文件列表,移除无效的文件选择 checkbox。

**Architecture:** 在 GitService 新增 per-mode 文件列表方法(分支用三点 diff,commit 用 `git show`),复用 name-status 解析;新增 `getModeFiles`/`modeFiles` 消息让 webview 在选定分支或 commit 后按需拉取;IdleView 根据当前 mode 切换 FileList 数据源;FileList 去掉 checkbox 改纯展示。

**Tech Stack:** TypeScript, Preact (webview), Jest + ts-jest, vscode.git API + 原生 git 子进程。

---

## File Structure

- `src/extension/services/gitMap.ts` — 新增 `parseNameStatus`(复用现有 `mapStatusCode`)。
- `src/extension/services/GitService.ts` — 新增 `getBranchDiff`、`getCommitFiles`。
- `src/shared/messages.ts` — 新增 `getModeFiles`(W→H)、`modeFiles`(H→W)。
- `src/extension/providers/SidebarProvider.ts` — 处理 `getModeFiles`。
- `src/webview/store.ts` — 新增 `modeFiles` 状态 + 处理 `modeFiles` 消息。
- `src/webview/views/IdleView.tsx` — 选定分支/commit 后请求文件;FileList 数据源按 mode 切换。
- `src/webview/components/FileList.tsx` — 删除 checkbox。
- 测试:`gitMap.test.ts`(扩充)、`store.test.ts`(扩充)。

---

## Task 1: parseNameStatus 解析器

**Files:**
- Modify: `src/extension/services/gitMap.ts`
- Test: `src/extension/services/__tests__/gitMap.test.ts`

- [ ] **Step 1: 写失败测试**

在 `src/extension/services/__tests__/gitMap.test.ts` 末尾追加:

```typescript
import { mapStatusCode, parsePorcelain, parseNameStatus } from '../gitMap';

describe('parseNameStatus', () => {
  it('解析 git diff/show --name-status 输出', () => {
    const out = [
      'M\tsrc/a.ts',
      'A\tsrc/b.ts',
      'D\tsrc/c.ts',
    ].join('\n');
    expect(parseNameStatus(out)).toEqual([
      { path: 'src/a.ts', status: 'modified' },
      { path: 'src/b.ts', status: 'added' },
      { path: 'src/c.ts', status: 'deleted' },
    ]);
  });

  it('重命名行 R<score> old new 取新路径', () => {
    expect(parseNameStatus('R100\told/x.ts\tnew/x.ts')).toEqual([
      { path: 'new/x.ts', status: 'renamed' },
    ]);
  });

  it('去重同一路径', () => {
    expect(parseNameStatus('M\tsrc/a.ts\nM\tsrc/a.ts')).toEqual([
      { path: 'src/a.ts', status: 'modified' },
    ]);
  });

  it('空输出返回空数组', () => {
    expect(parseNameStatus('')).toEqual([]);
    expect(parseNameStatus('\n \n')).toEqual([]);
  });
});
```

注意:把测试文件顶部已有的 `import { mapStatusCode, parsePorcelain } from '../gitMap';` 改成包含 `parseNameStatus` 的版本(上面那行),避免重复 import。

- [ ] **Step 2: 跑测试确认失败**

Run: `yarn jest gitMap -t parseNameStatus`
Expected: FAIL —— `parseNameStatus is not a function` / 类型错误。

- [ ] **Step 3: 实现 parseNameStatus**

在 `src/extension/services/gitMap.ts` 末尾追加:

```typescript
/**
 * 解析 `git diff --name-status` / `git show --name-status` 输出。
 * 每行制表符分隔：status<TAB>path,重命名为 R<score><TAB>old<TAB>new(取 new)。
 */
export function parseNameStatus(output: string): FileChange[] {
  const files: FileChange[] = [];
  const seen = new Set<string>();
  for (const rawLine of output.split('\n')) {
    if (!rawLine.trim()) continue;
    const parts = rawLine.split('\t');
    if (parts.length < 2) continue;
    const codeChar = parts[0][0];
    const path = parts.length >= 3 ? parts[parts.length - 1] : parts[1];
    if (seen.has(path)) continue;
    seen.add(path);
    files.push({ path, status: mapStatusCode(codeChar) });
  }
  return files;
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `yarn jest gitMap`
Expected: PASS(含原有 parsePorcelain/mapStatusCode 用例)。

- [ ] **Step 5: 提交**

```bash
git add src/extension/services/gitMap.ts src/extension/services/__tests__/gitMap.test.ts
git commit -m "feat: 新增 parseNameStatus 解析 git diff/show 输出"
```

---

## Task 2: GitService 新增 per-mode 文件列表方法

**Files:**
- Modify: `src/extension/services/GitService.ts`

无新增单测(依赖 vscode.git API 与子进程,难以纯单元测;解析逻辑已在 Task 1 覆盖)。通过编译验证。

- [ ] **Step 1: 引入 parseNameStatus**

修改 `src/extension/services/GitService.ts:4`,把:

```typescript
import { parsePorcelain } from './gitMap';
```

改为:

```typescript
import { parsePorcelain, parseNameStatus } from './gitMap';
```

- [ ] **Step 2: 新增两个方法**

在 `GitService` 类内、`getState` 方法之后(`src/extension/services/GitService.ts:92` 的 `}` 之后,类闭合 `}` 之前)插入:

```typescript
  /** 分支对比:merge-base 三点 diff。 */
  async getBranchDiff(from: string, to: string): Promise<FileChange[]> {
    const root = await this.repoRoot();
    if (!root || !from || !to) return [];
    try {
      const out = await runGit(root, ['diff', '--name-status', `${from}...${to}`]);
      const files = parseNameStatus(out);
      this.trace(`getBranchDiff(${from}...${to}): files=${files.length}`);
      return files;
    } catch (e) {
      this.trace(`getBranchDiff failed: ${e instanceof Error ? e.message : String(e)}`);
      return [];
    }
  }

  /** 单次提交:该 commit 相对父提交的改动文件。 */
  async getCommitFiles(sha: string): Promise<FileChange[]> {
    const root = await this.repoRoot();
    if (!root || !sha) return [];
    try {
      const out = await runGit(root, ['show', '--name-status', '--format=', sha]);
      const files = parseNameStatus(out);
      this.trace(`getCommitFiles(${sha}): files=${files.length}`);
      return files;
    } catch (e) {
      this.trace(`getCommitFiles failed: ${e instanceof Error ? e.message : String(e)}`);
      return [];
    }
  }

  private async repoRoot(): Promise<string | null> {
    const repo = await this.waitForRepo();
    if (!repo) return null;
    return repo.rootUri?.fsPath
      ?? vscode.workspace.workspaceFolders?.[0].uri.fsPath
      ?? process.cwd();
  }
```

- [ ] **Step 3: 编译验证**

Run: `yarn compile`
Expected: webpack 构建成功,无 TS 错误。

- [ ] **Step 4: 提交**

```bash
git add src/extension/services/GitService.ts
git commit -m "feat: GitService 新增分支 diff 与 commit 文件列表方法"
```

---

## Task 3: 消息协议新增 getModeFiles / modeFiles

**Files:**
- Modify: `src/shared/messages.ts`

- [ ] **Step 1: 扩充消息类型**

修改 `src/shared/messages.ts`。在 `WebviewToHost` 联合类型里(`src/shared/messages.ts:8` 的 `getGitState` 行下方)加一行:

```typescript
  | { type: 'getModeFiles'; mode: ReviewMode; from?: string; to?: string; commit?: string }
```

在 `HostToWebview` 联合类型里(`gitState` 行下方)加一行:

```typescript
  | { type: 'modeFiles'; mode: ReviewMode; files: FileChange[] }
```

同时确保顶部 import 包含 `FileChange`。把现有:

```typescript
import {
  CliResult, CliRunOptions, CommentSyncState, GitState, LogLine,
  OcrConfig, ReviewMode, ReviewState,
} from './types';
```

改为(加入 `FileChange`):

```typescript
import {
  CliResult, CliRunOptions, CommentSyncState, FileChange, GitState, LogLine,
  OcrConfig, ReviewMode, ReviewState,
} from './types';
```

- [ ] **Step 2: 编译验证**

Run: `yarn compile`
Expected: 构建成功。

- [ ] **Step 3: 提交**

```bash
git add src/shared/messages.ts
git commit -m "feat: 新增 getModeFiles/modeFiles 消息"
```

---

## Task 4: SidebarProvider 处理 getModeFiles

**Files:**
- Modify: `src/extension/providers/SidebarProvider.ts`

- [ ] **Step 1: 新增 case**

在 `src/extension/providers/SidebarProvider.ts` 的 `handle` switch 里,`getGitState` case 之后(`src/extension/providers/SidebarProvider.ts:47` 的 `}` 之后)插入:

```typescript
      case 'getModeFiles': {
        let files: import('../../shared/types').FileChange[] = [];
        if (msg.mode === 'branch' && msg.from && msg.to) {
          files = await this.git.getBranchDiff(msg.from, msg.to);
        } else if (msg.mode === 'commit' && msg.commit) {
          files = await this.git.getCommitFiles(msg.commit);
        }
        this.post({ type: 'modeFiles', mode: msg.mode, files });
        break;
      }
```

- [ ] **Step 2: 编译验证**

Run: `yarn compile`
Expected: 构建成功。

- [ ] **Step 3: 提交**

```bash
git add src/extension/providers/SidebarProvider.ts
git commit -m "feat: SidebarProvider 处理 getModeFiles 请求"
```

---

## Task 5: store 新增 modeFiles 状态

**Files:**
- Modify: `src/webview/store.ts`
- Test: `src/webview/__tests__/store.test.ts`

- [ ] **Step 1: 写失败测试**

先查看 `src/webview/__tests__/store.test.ts` 现有写法(reducer 调用形式),仿照追加。在文件末尾的最后一个 `});` 之前(或新建独立 describe)追加:

```typescript
describe('modeFiles 消息', () => {
  it('保存 mode 对应文件列表', () => {
    const next = reducer(initialState, {
      type: 'modeFiles',
      mode: 'branch',
      files: [{ path: 'src/a.ts', status: 'modified' }],
    });
    expect(next.modeFiles).toEqual([{ path: 'src/a.ts', status: 'modified' }]);
  });

  it('init 时 modeFiles 为空数组', () => {
    expect(initialState.modeFiles).toEqual([]);
  });
});
```

确保该测试文件顶部已 import `reducer, initialState`(若未导入则补上 `import { reducer, initialState } from '../store';`)。

- [ ] **Step 2: 跑测试确认失败**

Run: `yarn jest store -t modeFiles`
Expected: FAIL —— `modeFiles` 不存在 / 类型错误。

- [ ] **Step 3: 实现**

修改 `src/webview/store.ts`:

1. `AppState` 接口加字段(在 `gitState` 行下方):

```typescript
  modeFiles: FileChange[];
```

2. `initialState` 加字段(在 `gitState` 行下方):

```typescript
  modeFiles: [],
```

3. 顶部 import 加入 `FileChange`,把:

```typescript
import { CliResult, CommentStatus, GitState, LogLine, OcrConfig, ReviewState } from '../shared/types';
```

改为:

```typescript
import { CliResult, CommentStatus, FileChange, GitState, LogLine, OcrConfig, ReviewState } from '../shared/types';
```

4. reducer 加 case(在 `case 'gitState':` 之后):

```typescript
    case 'modeFiles':
      return { ...state, modeFiles: msg.files };
```

- [ ] **Step 4: 跑测试确认通过**

Run: `yarn jest store`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add src/webview/store.ts src/webview/__tests__/store.test.ts
git commit -m "feat: store 新增 modeFiles 状态"
```

---

## Task 6: IdleView 按 mode 加载并展示文件

**Files:**
- Modify: `src/webview/views/IdleView.tsx`
- Modify: `src/webview/App.tsx`

无单测(纯 UI 交互,通过编译 + 手动验证)。

- [ ] **Step 1: App.tsx 传入 modeFiles 与请求回调**

修改 `src/webview/App.tsx` 渲染 IdleView 的那行(`src/webview/App.tsx` 中 `state.view === 'idle'` 块),把:

```tsx
          <IdleView gitState={state.gitState} configured={configured} onModeChange={onModeChange} onStart={start} />
```

改为:

```tsx
          <IdleView gitState={state.gitState} modeFiles={state.modeFiles} configured={configured}
            onModeChange={onModeChange} onRequestModeFiles={requestModeFiles} onStart={start} />
```

并在 `App` 组件内、`onModeChange` 定义附近新增:

```tsx
  const requestModeFiles = (mode: ReviewMode, from?: string, to?: string, commit?: string) =>
    bridge.post({ type: 'getModeFiles', mode, from, to, commit });
```

- [ ] **Step 2: 改 IdleView**

完整替换 `src/webview/views/IdleView.tsx` 内容为:

```tsx
import { useState, useEffect } from 'preact/hooks';
import { GitState, ReviewMode, CliRunOptions, FileChange } from '../../shared/types';
import { FileList } from '../components/FileList';

interface Props {
  gitState: GitState;
  modeFiles: FileChange[];
  configured: boolean;
  onModeChange: (mode: ReviewMode) => void;
  onRequestModeFiles: (mode: ReviewMode, from?: string, to?: string, commit?: string) => void;
  onStart: (options: CliRunOptions) => void;
}

export function IdleView({ gitState, modeFiles, configured, onModeChange, onRequestModeFiles, onStart }: Props) {
  const [mode, setMode] = useState<ReviewMode>('workspace');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [commit, setCommit] = useState('');
  const [prompt, setPrompt] = useState('');

  const switchMode = (m: ReviewMode) => { setMode(m); onModeChange(m); };

  // 分支两端都选好后,拉取 diff 文件列表
  useEffect(() => {
    if (mode === 'branch' && from && to) onRequestModeFiles('branch', from, to);
  }, [mode, from, to]);

  // 选中某 commit 后,拉取该 commit 文件列表
  useEffect(() => {
    if (mode === 'commit' && commit) onRequestModeFiles('commit', undefined, undefined, commit);
  }, [mode, commit]);

  const files = mode === 'workspace' ? gitState.workspaceFiles : modeFiles;

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
            <option value="">选择分支</option>
            {gitState.branches.map((b) => <option key={b} value={b}>{b}</option>)}
          </select>
          <div class="mode-param-label">目标引用</div>
          <select class="mode-param-select" value={to} onChange={(e) => setTo((e.target as HTMLSelectElement).value)}>
            <option value="">选择分支</option>
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

      <FileList files={files} />

      <textarea class="mode-param-input" rows={3} placeholder="自定义审查提示词（可选）"
        value={prompt} onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)} />

      <button class="primary-btn" disabled={!configured}
        onClick={() => onStart({ mode, from, to, commit, customPrompt: prompt })}>
        {configured ? '审查所有变更' : '请先配置模型'}
      </button>
    </div>
  );
}
```

- [ ] **Step 3: 编译验证**

Run: `yarn compile`
Expected: 构建成功,无 TS 错误。

- [ ] **Step 4: 提交**

```bash
git add src/webview/views/IdleView.tsx src/webview/App.tsx
git commit -m "feat: IdleView 按模式加载分支/commit 文件列表"
```

---

## Task 7: FileList 移除 checkbox

**Files:**
- Modify: `src/webview/components/FileList.tsx`

- [ ] **Step 1: 删除 checkbox**

修改 `src/webview/components/FileList.tsx`,把渲染行的 `<label>` 改为 `<div>` 并删掉 checkbox。把:

```tsx
      {files.map((f) => (
        <label class="file-row" key={f.path}>
          <input type="checkbox" checked />
          <span class="file-name">{f.path}</span>
          <span class={`file-badge ${f.status}`}>{BADGE[f.status]}</span>
        </label>
      ))}
```

改为:

```tsx
      {files.map((f) => (
        <div class="file-row" key={f.path}>
          <span class="file-name">{f.path}</span>
          <span class={`file-badge ${f.status}`}>{BADGE[f.status]}</span>
        </div>
      ))}
```

- [ ] **Step 2: 编译验证**

Run: `yarn compile`
Expected: 构建成功。

- [ ] **Step 3: 提交**

```bash
git add src/webview/components/FileList.tsx
git commit -m "feat: FileList 移除无效 checkbox 改为纯展示"
```

---

## Task 8: 全量验证

- [ ] **Step 1: 跑全部测试**

Run: `yarn jest`
Expected: 全绿。

- [ ] **Step 2: 生产构建**

Run: `yarn build`
Expected: 构建成功。

- [ ] **Step 3: 手动验证(F5 启动扩展开发宿主)**

在 VSCode 里按 F5 启动 Extension Development Host,打开侧边栏:
- 工作区 Tab:有未提交改动时列出文件,与 `git status` 一致;无改动时列表为空。
- 分支对比 Tab:选好两个分支后,文件列表 = `git diff --name-status from...to` 的结果。
- 单次提交 Tab:点选某个 commit 后,列表 = 该 commit 改动文件。
- 三个 Tab 均无 checkbox,文件行不可勾选。
- 切换 Tab / 改选分支或 commit 时,列表正确刷新。

注:若无法运行 Extension Host,明确说明「UI 未手动验证」,不要谎称通过。
