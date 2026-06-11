# OCR VSCode 插件 UI 原型 · 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 产出一个 `prototype.html` 单文件高保真交互原型，在 360px 侧栏宽度内展示 OCR VSCode 插件的完整产品形态：从首次配置 → 选范围与文件 → 启动审查 → 流式过程 → 结果浏览 → 异常路径。

**Architecture:** 单文件 HTML，所有 CSS/JS 内联。通过 `body[data-state]` 和 `body[data-config]` 属性驱动状态机切换可见性。localStorage 持久化配置。无外部依赖，`file://` 双击可开。

**Tech Stack:** 纯 HTML + CSS（silent-night-ui 变量系统）+ Vanilla JS（< 200 行）

**Spec:** `docs/superpowers/specs/2026-06-04-ocr-vscode-ui-design.md`

**Visual reference:** `silent-night-ui/style-reference.md`（CSS 变量 §1、全局基底 §2、禁区清单 §8）

---

## File Structure

单文件交付，内部按注释分区：

```
prototype.html          ← 唯一交付物
  <style>
    /* === 1. CSS Variables (from silent-night-ui §1) === */
    /* === 2. Global Base (from silent-night-ui §2) === */
    /* === 3. VSCode Chrome Layout === */
    /* === 4. Status Bar === */
    /* === 5. Setup Region === */
    /* === 6. Action Region (6 states) === */
    /* === 7. Config Overlay (3 views) === */
    /* === 8. Demo Switcher === */
    /* === 9. State Selectors === */
  </style>
  <body data-state="idle" data-config="">
    <!-- demo-switcher -->
    <!-- vscode-chrome: activity-bar + sidebar + editor-stub -->
    <!--   sidebar: status-bar + setup + action-region + config-overlay -->
  </body>
  <script>
    /* State machine + interactions */
  </script>
```

---

### Task 1: HTML 骨架 + CSS 变量 + 全局基底 + VSCode Chrome 布局

**Files:**
- Create: `prototype.html`

**验收标准（浏览器打开后）：**
- 页面全黑背景，有微弱的 mint 光晕和星点网格
- 左侧 48px 深色 Activity Bar 有 3 个圆角占位方块
- 中间 360px 侧栏区域（`--card` 色背景）
- 右侧灰色编辑器占位区，有 "(editor area · prototype only)" 小字
- 无滚动条溢出，无 console 报错

- [ ] **Step 1: 创建 prototype.html 基础结构**

创建文件 `prototype.html`，包含完整 HTML 骨架、silent-night-ui CSS 变量、全局基底、VSCode chrome 三栏布局：

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OCR · VSCode Plugin UI Prototype</title>
<style>
/* === 1. CSS Variables (silent-night-ui §1) === */
:root {
  --bg: #0a0d12;
  --bg-soft: #11151a;
  --bg-deeper: #06090d;
  --card: #14181e;
  --card-soft: #1a1f27;
  --card-quiet: #20262f;
  --card-deeper: #0d1015;
  --ink: #ececec;
  --ink-soft: #b8b8b2;
  --ink-quiet: #7e828a;
  --ink-faint: #4f535b;
  --moon: #f3f3ef;
  --moon-soft: #b8b8b3;
  --moon-quiet: #6b6f76;
  --moon-faint: #4a4f56;
  --mint: #45e6a4;
  --mint-soft: #8ff0c2;
  --mint-glow: rgba(69, 230, 164, 0.35);
  --mint-tint: rgba(69, 230, 164, 0.10);
  --rule: rgba(255, 255, 255, 0.07);
  --rule-soft: rgba(255, 255, 255, 0.04);
  --font: -apple-system, BlinkMacSystemFont, 'SF Pro Text', 'Inter', system-ui, 'Helvetica Neue', 'PingFang SC', 'Noto Sans SC', sans-serif;
  --font-display: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Inter', system-ui, 'PingFang SC', sans-serif;
  --font-mono: 'JetBrains Mono', 'SF Mono', 'Berkeley Mono', Consolas, Menlo, monospace;
}

/* === 2. Global Base (silent-night-ui §2) === */
* { box-sizing: border-box; margin: 0; padding: 0; }
html { scroll-behavior: smooth; }
body {
  background: var(--bg);
  color: var(--ink);
  font-family: var(--font);
  font-size: 13px;
  line-height: 1.55;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-rendering: optimizeLegibility;
  overflow: hidden;
  height: 100vh;
}
body::before {
  content: '';
  position: fixed; inset: 0; z-index: 0; pointer-events: none;
  background:
    radial-gradient(ellipse at 20% -10%, rgba(69,230,164,0.06) 0%, transparent 50%),
    radial-gradient(ellipse at 90% 10%, rgba(120,140,200,0.04) 0%, transparent 45%),
    radial-gradient(ellipse at 50% 100%, rgba(69,230,164,0.03) 0%, transparent 50%);
}
body::after {
  content: '';
  position: fixed; inset: 0; z-index: 0; pointer-events: none;
  background-image: radial-gradient(circle at 1px 1px, rgba(255,255,255,0.025) 1px, transparent 0);
  background-size: 28px 28px;
}
::-webkit-scrollbar { width: 4px; height: 4px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.08); border-radius: 2px; }
::-webkit-scrollbar-thumb:hover { background: rgba(255,255,255,0.18); }
::selection { background: var(--mint-tint); color: var(--mint); }

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.45; transform: scale(0.85); }
}

/* === 3. VSCode Chrome Layout === */
.vscode-chrome {
  position: relative; z-index: 1;
  display: flex;
  height: 100vh;
  background: var(--bg);
}
.activity-bar {
  width: 48px; flex-shrink: 0;
  background: var(--bg-deeper);
  border-right: 1px solid var(--rule);
  display: flex; flex-direction: column;
  align-items: center;
  padding: 12px 0;
  gap: 6px;
}
.activity-bar .ab-icon {
  width: 28px; height: 28px;
  border-radius: 6px;
  background: var(--card);
  opacity: 0.5;
}
.activity-bar .ab-icon.active {
  opacity: 1;
  background: var(--card-quiet);
  box-shadow: inset 2px 0 0 var(--ink);
}
.sidebar {
  width: 360px; flex-shrink: 0;
  background: var(--card);
  border-right: 1px solid var(--rule);
  display: flex; flex-direction: column;
  overflow: hidden;
  position: relative;
}
.editor-stub {
  flex: 1;
  background: var(--bg-soft);
  display: flex;
  align-items: center;
  justify-content: center;
}
.editor-stub-text {
  font-size: 12px;
  color: var(--ink-faint);
  letter-spacing: 0.05em;
}
</style>
</head>
<body data-state="idle" data-config="">

<div class="vscode-chrome">
  <aside class="activity-bar">
    <div class="ab-icon active"></div>
    <div class="ab-icon"></div>
    <div class="ab-icon"></div>
    <div class="ab-icon"></div>
  </aside>
  <aside class="sidebar">
    <!-- status-bar, setup, action-region, config-overlay go here -->
  </aside>
  <main class="editor-stub">
    <span class="editor-stub-text">(editor area · prototype only)</span>
  </main>
</div>

<script>
// State machine + interactions will go here
</script>
</body>
</html>
```

- [ ] **Step 2: 浏览器验证**

Run: 双击 `prototype.html` 在浏览器中打开

验证：
1. 深色背景 + mint 光晕可见
2. 左侧 48px activity bar，4 个圆角方块，第一个有左侧白色条高亮
3. 中间 360px 空白侧栏（`--card` 色）
4. 右侧灰色编辑器区域有 "(editor area · prototype only)" 文字
5. 无 console 报错，无滚动条

- [ ] **Step 3: Commit**

```bash
git add prototype.html
git commit -m "feat: prototype skeleton with CSS variables and VSCode chrome layout"
```

---

### Task 2: Status Bar 顶部条 + Model Dropdown

**Files:**
- Modify: `prototype.html`（CSS 区 + HTML sidebar 内部）

**验收标准：**
- 侧栏顶部一行：mint 脉动圆点 + 模型名 + " · " + provider 名 + ▾ 下拉 + ⚙ 齿轮按钮
- 点击 ▾ 弹出 dropdown，列出模型（按 provider 分组），active 项左侧有 mint 点
- 点击 ⚙ 暂无反应（Task 8 接入）
- dropdown 点外面自动关闭

- [ ] **Step 1: 添加 Status Bar CSS**

在 `</style>` 前追加：

```css
/* === 4. Status Bar === */
.status-bar {
  display: flex; align-items: center;
  padding: 10px 14px;
  border-bottom: 1px solid var(--rule);
  gap: 8px;
  flex-shrink: 0;
  position: relative;
}
.status-dot {
  width: 7px; height: 7px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 8px var(--mint-glow);
  animation: pulse 1.6s ease-in-out infinite;
  flex-shrink: 0;
}
.status-dot.dim {
  background: var(--ink-faint);
  box-shadow: none;
  animation: none;
}
.status-model {
  font-size: 12.5px; font-weight: 600;
  color: var(--ink);
  white-space: nowrap;
}
.status-sep {
  color: var(--ink-faint);
  font-size: 12px;
}
.status-provider {
  font-size: 12px;
  color: var(--ink-quiet);
  white-space: nowrap;
}
.status-dropdown-trigger {
  background: none; border: none;
  color: var(--ink-quiet);
  font-size: 11px; cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
  transition: background-color 0.2s;
}
.status-dropdown-trigger:hover {
  background: var(--card-quiet);
  color: var(--ink-soft);
}
.status-spacer { flex: 1; }
.status-gear {
  background: none; border: none;
  color: var(--ink-quiet);
  font-size: 14px; cursor: pointer;
  padding: 4px 6px;
  border-radius: 6px;
  transition: background-color 0.2s, color 0.2s;
}
.status-gear:hover {
  background: var(--card-quiet);
  color: var(--ink);
}
.status-reset {
  background: none; border: none;
  color: var(--ink-faint);
  font-size: 9px; cursor: pointer;
  padding: 2px 4px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  opacity: 0;
  transition: opacity 0.2s;
}
.status-bar:hover .status-reset { opacity: 1; }

/* Model Dropdown */
.model-dropdown {
  position: absolute;
  top: 100%; left: 14px; right: 14px;
  background: var(--card-soft);
  border: 1px solid var(--rule);
  border-radius: 12px;
  padding: 6px;
  z-index: 20;
  box-shadow: 0 12px 32px -8px rgba(0,0,0,0.6);
  display: none;
}
.model-dropdown.open { display: block; }
.model-dropdown-group {
  font-size: 10px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--ink-faint);
  padding: 8px 10px 4px;
  font-weight: 600;
}
.model-dropdown-item {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 10px;
  border-radius: 8px;
  font-size: 12.5px;
  color: var(--ink-soft);
  cursor: pointer;
  transition: background-color 0.15s;
}
.model-dropdown-item:hover {
  background: var(--card-quiet);
  color: var(--ink);
}
.model-dropdown-item .md-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: transparent;
  flex-shrink: 0;
}
.model-dropdown-item.active { color: var(--ink); font-weight: 600; }
.model-dropdown-item.active .md-dot {
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
}
```

- [ ] **Step 2: 添加 Status Bar HTML**

在 `<aside class="sidebar">` 内部 `<!-- status-bar ... -->` 注释处替换为：

```html
<div class="status-bar">
  <span class="status-dot"></span>
  <span class="status-model">claude-opus-4-7</span>
  <span class="status-sep">·</span>
  <span class="status-provider">Anthropic</span>
  <button class="status-dropdown-trigger" onclick="toggleDropdown()">▾</button>
  <span class="status-spacer"></span>
  <button class="status-reset" onclick="resetAll()">Reset</button>
  <button class="status-gear" onclick="toggleConfig()">⚙</button>
  <div class="model-dropdown" id="modelDropdown">
    <div class="model-dropdown-group">Anthropic</div>
    <div class="model-dropdown-item active" onclick="selectModel(this, 'claude-opus-4-7', 'Anthropic')">
      <span class="md-dot"></span>claude-opus-4-7
    </div>
    <div class="model-dropdown-item" onclick="selectModel(this, 'claude-sonnet-4-6', 'Anthropic')">
      <span class="md-dot"></span>claude-sonnet-4-6
    </div>
    <div class="model-dropdown-group">OpenAI</div>
    <div class="model-dropdown-item" onclick="selectModel(this, 'gpt-5', 'OpenAI')">
      <span class="md-dot"></span>gpt-5
    </div>
    <div class="model-dropdown-item" onclick="selectModel(this, 'o3', 'OpenAI')">
      <span class="md-dot"></span>o3
    </div>
  </div>
</div>
```

- [ ] **Step 3: 添加 dropdown 交互 JS**

在 `<script>` 标签内添加：

```javascript
function toggleDropdown() {
  document.getElementById('modelDropdown').classList.toggle('open');
}

function selectModel(el, model, provider) {
  document.querySelectorAll('.model-dropdown-item').forEach(i => i.classList.remove('active'));
  el.classList.add('active');
  document.querySelector('.status-model').textContent = model;
  document.querySelector('.status-provider').textContent = provider;
  document.getElementById('modelDropdown').classList.remove('open');
}

function toggleConfig() {
  // Will be implemented in Task 8
}

function resetAll() {
  localStorage.removeItem('ocrPrototype');
  location.reload();
}

document.addEventListener('click', function(e) {
  var dd = document.getElementById('modelDropdown');
  if (dd && !e.target.closest('.status-bar')) {
    dd.classList.remove('open');
  }
});
```

- [ ] **Step 4: 浏览器验证**

验证：
1. 顶部状态条：mint 脉动圆点 + "claude-opus-4-7 · Anthropic ▾ ⚙"
2. hover status-bar 时右上角出现 "RESET" 小字
3. 点击 ▾ 弹出 dropdown，模型按 provider 分组
4. "claude-opus-4-7" 左侧有 mint 圆点（active）
5. 点击 "gpt-5" → 顶部文字变为 "gpt-5 · OpenAI"，dropdown 关闭
6. 点击 dropdown 外部区域 → dropdown 关闭

- [ ] **Step 5: Commit**

```bash
git add prototype.html
git commit -m "feat: add status bar with model dropdown"
```

---

### Task 3: Setup 区（范围 pill + 文件列表 + 主按钮）

**Files:**
- Modify: `prototype.html`

**验收标准：**
- Status bar 下方显示 "NEW REVIEW" 标签 + "workspace" pill
- 文件列表 3 行，每行 checkbox + 文件名（mono 字体）+ A/M 标记
- 全宽 mint 主按钮 "Review all changes"
- 文件列表与主按钮间有适当间距

- [ ] **Step 1: 添加 Setup 区 CSS**

在 Status Bar CSS 之后追加：

```css
/* === 5. Setup Region === */
.setup {
  padding: 14px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--rule);
}
.setup-label {
  font-size: 10.5px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-quiet);
  font-weight: 600;
  margin-bottom: 10px;
  display: flex; align-items: center; gap: 8px;
}
.setup-label::before {
  content: '';
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
}

/* Range Pill */
.range-pill {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 5px 12px;
  background: var(--card-soft);
  border: 1px solid var(--rule);
  border-radius: 999px;
  font-size: 12px;
  color: var(--ink-soft);
  cursor: pointer;
  margin-bottom: 14px;
  transition: background-color 0.2s, border-color 0.2s;
  position: relative;
}
.range-pill:hover {
  background: var(--card-quiet);
  border-color: rgba(255,255,255,0.12);
}
.range-pill-chevron {
  font-size: 9px;
  color: var(--ink-faint);
}

/* Range Popover */
.range-popover {
  position: absolute;
  top: calc(100% + 6px); left: 0;
  background: var(--card-soft);
  border: 1px solid var(--rule);
  border-radius: 10px;
  padding: 4px;
  z-index: 15;
  box-shadow: 0 8px 24px -6px rgba(0,0,0,0.5);
  display: none;
  min-width: 200px;
}
.range-popover.open { display: block; }
.range-option {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 10px;
  border-radius: 7px;
  font-size: 12px;
  color: var(--ink-soft);
  cursor: pointer;
  transition: background-color 0.15s;
}
.range-option:hover {
  background: var(--card-quiet);
  color: var(--ink);
}
.range-option.active {
  color: var(--mint);
  font-weight: 600;
}
.range-option .ro-dot {
  width: 5px; height: 5px; border-radius: 50%;
  background: transparent;
}
.range-option.active .ro-dot {
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
}

/* Files Label */
.files-label {
  font-size: 10.5px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-quiet);
  font-weight: 600;
  margin-bottom: 8px;
}

/* File Row */
.file-list { margin-bottom: 14px; }
.file-row {
  display: flex; align-items: center; gap: 8px;
  padding: 5px 4px;
  border-radius: 6px;
  transition: background-color 0.15s;
  cursor: pointer;
}
.file-row:hover { background: var(--card-soft); }
.file-row input[type="checkbox"] {
  appearance: none; -webkit-appearance: none;
  width: 14px; height: 14px;
  border: 1.5px solid var(--ink-faint);
  border-radius: 3px;
  background: transparent;
  cursor: pointer;
  flex-shrink: 0;
  position: relative;
  transition: border-color 0.15s, background-color 0.15s;
}
.file-row input[type="checkbox"]:checked {
  background: var(--mint);
  border-color: var(--mint);
}
.file-row input[type="checkbox"]:checked::after {
  content: '';
  position: absolute;
  left: 3.5px; top: 1px;
  width: 4px; height: 7px;
  border: solid #0a1010;
  border-width: 0 1.5px 1.5px 0;
  transform: rotate(45deg);
}
.file-name {
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--ink);
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.file-badge {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
  padding: 1px 6px;
  border-radius: 4px;
  flex-shrink: 0;
}
.file-badge.added { color: var(--mint); background: var(--mint-tint); }
.file-badge.modified { color: var(--ink-quiet); background: rgba(255,255,255,0.05); }
.file-badge.deleted { color: var(--ink-faint); background: rgba(255,255,255,0.03); }

/* Primary Button */
.primary-btn {
  width: 100%;
  padding: 10px 16px;
  background: var(--mint);
  color: #0a1010;
  border: none;
  border-radius: 10px;
  font-family: var(--font);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
  display: flex; align-items: center; justify-content: center; gap: 8px;
}
.primary-btn:hover { background: var(--mint-soft); }
.primary-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
```

- [ ] **Step 2: 添加 Setup 区 HTML**

在 `</div><!-- status-bar -->` 后（sidebar 内）添加：

```html
<div class="setup">
  <div class="setup-label">New Review</div>
  <div class="range-pill" onclick="toggleRange(event)">
    <span class="range-pill-text">workspace</span>
    <span class="range-pill-chevron">▾</span>
    <div class="range-popover" id="rangePopover">
      <div class="range-option active" onclick="selectRange(event, this, 'workspace')">
        <span class="ro-dot"></span>workspace
      </div>
      <div class="range-option" onclick="selectRange(event, this, 'branch:master..HEAD')">
        <span class="ro-dot"></span>branch:master..HEAD
      </div>
      <div class="range-option" onclick="selectRange(event, this, 'branch:custom')">
        <span class="ro-dot"></span>branch:custom
      </div>
    </div>
  </div>
  <div class="files-label">Files to Review (<span id="fileCount">3</span>)</div>
  <div class="file-list" id="fileList">
    <label class="file-row">
      <input type="checkbox" checked onchange="updateFileCount()">
      <span class="file-name">CHANGELOG.md</span>
      <span class="file-badge modified">M</span>
    </label>
    <label class="file-row">
      <input type="checkbox" checked onchange="updateFileCount()">
      <span class="file-name">README.md</span>
      <span class="file-badge modified">M</span>
    </label>
    <label class="file-row">
      <input type="checkbox" checked onchange="updateFileCount()">
      <span class="file-name">src/api.ts</span>
      <span class="file-badge added">A</span>
    </label>
  </div>
  <button class="primary-btn" id="reviewBtn" onclick="startReview()">
    Review all changes
  </button>
</div>
```

- [ ] **Step 3: 添加 Setup 交互 JS**

在 `<script>` 中追加：

```javascript
function toggleRange(e) {
  e.stopPropagation();
  document.getElementById('rangePopover').classList.toggle('open');
}

function selectRange(e, el, value) {
  e.stopPropagation();
  document.querySelectorAll('.range-option').forEach(o => o.classList.remove('active'));
  el.classList.add('active');
  document.querySelector('.range-pill-text').textContent = value;
  document.getElementById('rangePopover').classList.remove('open');
}

function updateFileCount() {
  var checked = document.querySelectorAll('.file-list input[type="checkbox"]:checked').length;
  var total = document.querySelectorAll('.file-list input[type="checkbox"]').length;
  document.getElementById('fileCount').textContent = checked;
  var btn = document.getElementById('reviewBtn');
  btn.textContent = checked === total ? 'Review all changes' : 'Review ' + checked + ' changes';
  btn.disabled = checked === 0;
}

function startReview() {
  document.body.dataset.state = 'running';
}

document.addEventListener('click', function(e) {
  if (!e.target.closest('.range-pill')) {
    document.getElementById('rangePopover').classList.remove('open');
  }
});
```

- [ ] **Step 4: 浏览器验证**

验证：
1. "NEW REVIEW" 标签左侧有 mint 圆点
2. "workspace ▾" pill 可点击，弹出 3 个选项，active 项有 mint 点
3. 选择 "branch:master..HEAD" → pill 文字更新，popover 关闭
4. 3 个文件行，checkbox 选中时为 mint 色带白色勾
5. 取消勾选一个文件 → 文件数变为 2，按钮文字变为 "Review 2 changes"
6. 全部取消 → 按钮灰色 disabled
7. 全宽 mint 主按钮 hover 时颜色变浅

- [ ] **Step 5: Commit**

```bash
git add prototype.html
git commit -m "feat: add setup region with range pill, file list, and review button"
```

---

### Task 4: Action 区六态（静态 HTML + CSS + data-state 选择器）

**Files:**
- Modify: `prototype.html`

**验收标准：**
- 6 个 Action 子视图的完整 HTML 和 CSS 就位
- 通过手动修改 `body[data-state]` 可以切换显示不同状态
- Idle: 灰色占位文字
- Running: 4 阶段进度条 + 10 行 timeline + Cancel pill
- Done: mint summary + 3 张评论卡（critical/warn/info）
- Empty: mint 圆点 + "No issues found" 收束语
- Cancelled: 灰色摘要 + 1 张部分评论卡
- Failed: 错误卡 + Retry pill

- [ ] **Step 1: 添加 Action 区 CSS**

在 Setup CSS 之后追加：

```css
/* === 6. Action Region === */
.action-region {
  flex: 1;
  overflow-y: auto;
  padding: 14px;
}

/* State visibility */
.action-idle,
.action-running,
.action-done,
.action-empty,
.action-cancelled,
.action-failed { display: none; }

body[data-state="idle"] .action-idle { display: block; }
body[data-state="running"] .action-running { display: block; }
body[data-state="done"] .action-done { display: block; }
body[data-state="empty"] .action-empty { display: block; }
body[data-state="cancelled"] .action-cancelled { display: block; }
body[data-state="failed"] .action-failed { display: block; }

/* Idle */
.idle-note {
  text-align: center;
  padding: 40px 20px;
  font-size: 12.5px;
  color: var(--ink-faint);
  line-height: 1.6;
}

/* Running: Stage Bar */
.stage-bar {
  display: flex; align-items: center; gap: 0;
  margin-bottom: 16px;
  padding: 10px 12px;
  background: var(--card-soft);
  border-radius: 10px;
}
.stage-item {
  display: flex; align-items: center; gap: 6px;
  font-size: 10.5px;
  letter-spacing: 0.06em;
  color: var(--ink-faint);
  white-space: nowrap;
}
.stage-item.done { color: var(--mint); }
.stage-item.active { color: var(--ink); font-weight: 600; }
.stage-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--ink-faint);
  flex-shrink: 0;
}
.stage-item.done .stage-dot {
  background: var(--mint);
}
.stage-item.active .stage-dot {
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
  animation: pulse 1.6s ease-in-out infinite;
}
.stage-connector {
  flex: 1;
  height: 1px;
  background: var(--rule);
  margin: 0 6px;
  min-width: 8px;
}
.stage-connector.done { background: var(--mint-tint); }

/* Running: Timeline */
.timeline {
  font-family: var(--font-mono);
  font-size: 11.5px;
  line-height: 1.7;
  color: var(--ink-soft);
}
.timeline-row {
  display: flex; align-items: flex-start; gap: 10px;
  padding: 2px 0;
}
.timeline-dot {
  width: 6px; height: 6px; border-radius: 50%;
  margin-top: 6px;
  flex-shrink: 0;
}
.timeline-dot.tool {
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
  animation: pulse 1.6s ease-in-out infinite;
}
.timeline-dot.file {
  background: var(--mint);
}
.timeline-dot.comment {
  background: var(--ink-faint);
  width: 5px; height: 5px;
}
.timeline-text { flex: 1; word-break: break-all; }
.timeline-text .t-file { color: var(--mint); }
.timeline-text .t-dim { color: var(--ink-faint); }

/* Cancel Pill */
.cancel-pill {
  display: inline-flex; align-items: center;
  padding: 5px 14px;
  background: rgba(255,255,255,0.04);
  border: 1px solid var(--rule);
  border-radius: 999px;
  font-family: var(--font);
  font-size: 11.5px;
  color: var(--ink-quiet);
  cursor: pointer;
  margin-top: 12px;
  float: right;
  transition: background-color 0.2s, color 0.2s;
}
.cancel-pill:hover {
  background: rgba(255,255,255,0.08);
  color: var(--ink-soft);
}

/* Done: Summary */
.done-summary {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 14px;
  background: var(--mint-tint);
  border-radius: 10px;
  margin-bottom: 14px;
  font-size: 12.5px;
  color: var(--mint);
  font-weight: 500;
}
.done-summary .ds-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--mint);
  flex-shrink: 0;
}

/* Comment Card */
.comment-card {
  background: var(--card-soft);
  border-radius: 12px;
  padding: 14px;
  margin-bottom: 10px;
  transition: opacity 0.3s, max-height 0.3s, padding 0.3s, margin 0.3s;
  max-height: 400px;
  overflow: hidden;
}
.comment-card.dismissed {
  opacity: 0;
  max-height: 0;
  padding: 0 14px;
  margin-bottom: 0;
}
.comment-header {
  display: flex; align-items: center; gap: 8px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.comment-file {
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--ink-soft);
}
.comment-line {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-quiet);
  background: var(--card-quiet);
  padding: 1px 6px;
  border-radius: 4px;
}
.severity-pill {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  padding: 2px 8px;
  border-radius: 999px;
}
.severity-pill.critical {
  background: var(--mint-tint);
  color: var(--mint);
}
.severity-pill.warn {
  background: var(--card-quiet);
  color: var(--ink-soft);
}
.severity-pill.info {
  background: rgba(255,255,255,0.04);
  color: var(--ink-quiet);
}
.comment-body {
  font-size: 13px;
  line-height: 1.6;
  color: var(--ink);
  margin-bottom: 10px;
}
.comment-actions {
  display: flex; gap: 6px;
}
.comment-actions button {
  background: none;
  border: 1px solid var(--rule);
  border-radius: 6px;
  padding: 4px 10px;
  font-family: var(--font);
  font-size: 11px;
  color: var(--ink-quiet);
  cursor: pointer;
  transition: background-color 0.15s, color 0.15s, border-color 0.15s;
}
.comment-actions button:hover {
  background: var(--card-quiet);
  color: var(--ink-soft);
  border-color: rgba(255,255,255,0.12);
}

/* Empty note */
.empty-note {
  text-align: center;
  padding: 40px 20px;
}
.empty-note .en-dot {
  display: inline-block;
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 8px var(--mint-glow);
  margin-bottom: 14px;
}
.empty-note .en-text {
  font-size: 13px;
  color: var(--mint);
  font-weight: 500;
}

/* Cancelled */
.cancelled-note {
  padding: 12px 14px;
  border-left: 2.5px solid var(--ink-faint);
  border-radius: 0 10px 10px 0;
  background: var(--card-soft);
  margin-bottom: 14px;
  font-size: 12.5px;
  color: var(--ink-quiet);
  line-height: 1.5;
}

/* Failed */
.failed-card {
  background: var(--card-soft);
  border-radius: 12px;
  padding: 18px;
  text-align: center;
}
.failed-card .fc-msg {
  font-size: 13px;
  color: var(--ink-soft);
  margin-bottom: 14px;
  line-height: 1.6;
}
.retry-pill {
  display: inline-flex; align-items: center;
  padding: 6px 18px;
  background: var(--mint);
  color: #0a1010;
  border: none;
  border-radius: 999px;
  font-family: var(--font);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
}
.retry-pill:hover { background: var(--mint-soft); }
```

- [ ] **Step 2: 添加 Action 区 HTML**

在 `</div><!-- setup -->` 后（sidebar 内）添加：

```html
<div class="action-region">
  <!-- Idle -->
  <div class="action-idle">
    <div class="idle-note">Ready to review · pick files above</div>
  </div>

  <!-- Running -->
  <div class="action-running">
    <div class="stage-bar">
      <div class="stage-item done"><span class="stage-dot"></span>Parse</div>
      <div class="stage-connector done"></div>
      <div class="stage-item done"><span class="stage-dot"></span>Pack</div>
      <div class="stage-connector"></div>
      <div class="stage-item active"><span class="stage-dot"></span>Review 2/3</div>
      <div class="stage-connector"></div>
      <div class="stage-item"><span class="stage-dot"></span>Reflect</div>
    </div>
    <div class="timeline">
      <div class="timeline-row">
        <span class="timeline-dot tool"></span>
        <span class="timeline-text">tool:read_file <span class="t-file">src/api.ts</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot tool"></span>
        <span class="timeline-text">tool:search <span class="t-dim">"auth middleware"</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot file"></span>
        <span class="timeline-text">reviewing <span class="t-file">src/api.ts</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot comment"></span>
        <span class="timeline-text"><span class="t-dim">comment +1 @ L42</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot tool"></span>
        <span class="timeline-text">tool:read_file <span class="t-file">CHANGELOG.md</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot file"></span>
        <span class="timeline-text">reviewing <span class="t-file">CHANGELOG.md</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot comment"></span>
        <span class="timeline-text"><span class="t-dim">comment +1 @ L15</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot tool"></span>
        <span class="timeline-text">tool:read_file <span class="t-file">README.md</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot file"></span>
        <span class="timeline-text">reviewing <span class="t-file">README.md</span></span>
      </div>
      <div class="timeline-row">
        <span class="timeline-dot comment"></span>
        <span class="timeline-text"><span class="t-dim">comment +1 @ L8</span></span>
      </div>
    </div>
    <button class="cancel-pill" onclick="cancelReview()">Cancel</button>
    <div style="clear:both"></div>
  </div>

  <!-- Done (with comments) -->
  <div class="action-done">
    <div class="done-summary">
      <span class="ds-dot"></span>
      <span id="doneSummaryText">5 comments · 3 files · 24s</span>
    </div>
    <div id="commentList">
      <div class="comment-card">
        <div class="comment-header">
          <span class="comment-file">src/api.ts</span>
          <span class="comment-line">L42</span>
          <span class="severity-pill critical">critical</span>
        </div>
        <div class="comment-body">SQL query is constructed via string concatenation. Use parameterized queries to prevent injection attacks.</div>
        <div class="comment-actions">
          <button onclick="openFile('src/api.ts', 42)">Open</button>
          <button onclick="copyComment(this)">Copy</button>
          <button onclick="dismissComment(this)">Dismiss</button>
        </div>
      </div>
      <div class="comment-card">
        <div class="comment-header">
          <span class="comment-file">src/api.ts</span>
          <span class="comment-line">L88</span>
          <span class="severity-pill warn">warn</span>
        </div>
        <div class="comment-body">Missing error handling for the async fetch call. If the API is unreachable, this will throw an unhandled promise rejection.</div>
        <div class="comment-actions">
          <button onclick="openFile('src/api.ts', 88)">Open</button>
          <button onclick="copyComment(this)">Copy</button>
          <button onclick="dismissComment(this)">Dismiss</button>
        </div>
      </div>
      <div class="comment-card">
        <div class="comment-header">
          <span class="comment-file">src/api.ts</span>
          <span class="comment-line">L120</span>
          <span class="severity-pill warn">warn</span>
        </div>
        <div class="comment-body">The retry logic uses a fixed delay. Consider exponential backoff to avoid hammering the server during outages.</div>
        <div class="comment-actions">
          <button onclick="openFile('src/api.ts', 120)">Open</button>
          <button onclick="copyComment(this)">Copy</button>
          <button onclick="dismissComment(this)">Dismiss</button>
        </div>
      </div>
      <div class="comment-card">
        <div class="comment-header">
          <span class="comment-file">CHANGELOG.md</span>
          <span class="comment-line">L15</span>
          <span class="severity-pill info">info</span>
        </div>
        <div class="comment-body">Version date format is inconsistent with previous entries. Consider using ISO 8601 (YYYY-MM-DD) throughout.</div>
        <div class="comment-actions">
          <button onclick="openFile('CHANGELOG.md', 15)">Open</button>
          <button onclick="copyComment(this)">Copy</button>
          <button onclick="dismissComment(this)">Dismiss</button>
        </div>
      </div>
      <div class="comment-card">
        <div class="comment-header">
          <span class="comment-file">README.md</span>
          <span class="comment-line">L8</span>
          <span class="severity-pill info">info</span>
        </div>
        <div class="comment-body">Installation command references a deprecated package name. Update to the current npm package.</div>
        <div class="comment-actions">
          <button onclick="openFile('README.md', 8)">Open</button>
          <button onclick="copyComment(this)">Copy</button>
          <button onclick="dismissComment(this)">Dismiss</button>
        </div>
      </div>
    </div>
  </div>

  <!-- Empty (no comments) -->
  <div class="action-empty">
    <div class="empty-note">
      <div class="en-dot"></div>
      <div class="en-text">No issues found · 3 files cleared</div>
    </div>
  </div>

  <!-- Cancelled -->
  <div class="action-cancelled">
    <div class="cancelled-note">
      Review cancelled after 12s · 2 of 3 files reviewed · 2 comments found
    </div>
    <div class="comment-card">
      <div class="comment-header">
        <span class="comment-file">src/api.ts</span>
        <span class="comment-line">L42</span>
        <span class="severity-pill critical">critical</span>
      </div>
      <div class="comment-body">SQL query is constructed via string concatenation. Use parameterized queries to prevent injection attacks.</div>
      <div class="comment-actions">
        <button onclick="openFile('src/api.ts', 42)">Open</button>
        <button onclick="copyComment(this)">Copy</button>
        <button onclick="dismissComment(this)">Dismiss</button>
      </div>
    </div>
    <div class="comment-card">
      <div class="comment-header">
        <span class="comment-file">src/api.ts</span>
        <span class="comment-line">L88</span>
        <span class="severity-pill warn">warn</span>
      </div>
      <div class="comment-body">Missing error handling for the async fetch call.</div>
      <div class="comment-actions">
        <button onclick="openFile('src/api.ts', 88)">Open</button>
        <button onclick="copyComment(this)">Copy</button>
        <button onclick="dismissComment(this)">Dismiss</button>
      </div>
    </div>
  </div>

  <!-- Failed -->
  <div class="action-failed">
    <div class="failed-card">
      <div class="fc-msg">
        Unable to reach LLM endpoint.<br>
        Check your API key and network connection.
      </div>
      <button class="retry-pill" onclick="retryReview()">Retry</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: 添加 Action 交互 JS**

在 `<script>` 中追加：

```javascript
function cancelReview() {
  document.body.dataset.state = 'cancelled';
}

function retryReview() {
  document.body.dataset.state = 'running';
}

function openFile(file, line) {
  // Prototype only — no real file opening
  alert('Open ' + file + ':' + line + ' (prototype placeholder)');
}

function copyComment(btn) {
  var card = btn.closest('.comment-card');
  var text = card.querySelector('.comment-body').textContent;
  navigator.clipboard.writeText(text).then(function() {
    btn.textContent = 'Copied';
    setTimeout(function() { btn.textContent = 'Copy'; }, 1500);
  });
}

function dismissComment(btn) {
  var card = btn.closest('.comment-card');
  card.classList.add('dismissed');
  // Update summary count
  setTimeout(function() {
    var visible = document.querySelectorAll('.action-done .comment-card:not(.dismissed)').length;
    var summaryEl = document.getElementById('doneSummaryText');
    if (summaryEl) {
      summaryEl.textContent = visible + ' comments · 3 files · 24s';
    }
  }, 350);
}
```

- [ ] **Step 4: 浏览器验证**

手动在浏览器 DevTools 中修改 `document.body.dataset.state` 来切换各状态：

验证：
1. `idle`: 居中灰色文字 "Ready to review · pick files above"
2. `running`: 4 阶段进度条（Parse/Pack 已完成 mint 色，Review 2/3 活跃脉动，Reflect 灰色）+ 10 行 timeline（tool=mint 脉动，file=mint 实心，comment=灰色小点）+ 右下 Cancel pill
3. `done`: mint 背景 summary 条 "5 comments · 3 files · 24s" + 5 张评论卡（1 critical + 2 warn + 2 info），severity pill 颜色分明
4. `empty`: mint 圆点 + "No issues found · 3 files cleared"
5. `cancelled`: 灰色左边框摘要 + 2 张部分评论卡
6. `failed`: 错误说明 + mint Retry 按钮
7. 评论卡 Dismiss 点击后淡出折叠，summary 数字减一
8. Copy 按钮点击后文字变为 "Copied"

- [ ] **Step 5: Commit**

```bash
git add prototype.html
git commit -m "feat: add action region with all 6 states (idle/running/done/empty/cancelled/failed)"
```

---

### Task 5: Config Overlay 三视图

**Files:**
- Modify: `prototype.html`

**验收标准：**
- Config overlay 覆盖整个 sidebar（z-index 高于 setup/action）
- `config-empty`: 引导页 "Connect a model to begin" + 大主按钮
- `config-list`: Provider 列表，每行 provider 名 + model 数 + active 标记
- `config-form`: 完整表单（Provider name / Base URL / API Key / Models 多行 / Advanced toggle）
- Cancel/Save 按钮在表单底部，Save 为 mint 色

- [ ] **Step 1: 添加 Config CSS**

在 Action CSS 之后追加：

```css
/* === 7. Config Overlay === */
.config-overlay {
  position: absolute;
  top: 0; left: 0; right: 0; bottom: 0;
  background: var(--card);
  z-index: 10;
  display: none;
  flex-direction: column;
  overflow-y: auto;
}
body[data-config="empty"] .config-overlay,
body[data-config="list"] .config-overlay,
body[data-config="form"] .config-overlay { display: flex; }

.config-empty,
.config-list,
.config-form { display: none; }
body[data-config="empty"] .config-empty { display: block; }
body[data-config="list"] .config-list { display: block; }
body[data-config="form"] .config-form { display: block; }

/* Config Empty (Onboarding) */
.config-empty {
  padding: 60px 24px;
  text-align: center;
}
.config-empty .ce-dot {
  display: inline-block;
  width: 10px; height: 10px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 12px var(--mint-glow);
  margin-bottom: 20px;
}
.config-empty .ce-label {
  font-size: 10.5px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-quiet);
  font-weight: 600;
  margin-bottom: 16px;
}
.config-empty .ce-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--ink);
  margin-bottom: 10px;
}
.config-empty .ce-desc {
  font-size: 12.5px;
  color: var(--ink-quiet);
  line-height: 1.6;
  margin-bottom: 28px;
  max-width: 260px;
  margin-left: auto;
  margin-right: auto;
}
.config-empty .ce-btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 10px 24px;
  background: var(--mint);
  color: #0a1010;
  border: none;
  border-radius: 10px;
  font-family: var(--font);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
}
.config-empty .ce-btn:hover { background: var(--mint-soft); }

/* Config List */
.config-list {
  padding: 14px;
}
.config-list-header {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 14px;
}
.config-list-title {
  font-size: 10.5px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-quiet);
  font-weight: 600;
  display: flex; align-items: center; gap: 8px;
}
.config-list-title::before {
  content: '';
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
}
.config-list-close {
  background: none; border: none;
  color: var(--ink-quiet);
  font-size: 16px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
  transition: background-color 0.2s, color 0.2s;
}
.config-list-close:hover {
  background: var(--card-quiet);
  color: var(--ink);
}
.provider-card {
  background: var(--card-soft);
  border-radius: 12px;
  padding: 14px;
  margin-bottom: 8px;
  cursor: pointer;
  transition: background-color 0.15s;
  display: flex; align-items: center; gap: 10px;
}
.provider-card:hover { background: var(--card-quiet); }
.provider-card .pc-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
  flex: 1;
}
.provider-card .pc-models {
  font-size: 11px;
  color: var(--ink-quiet);
}
.provider-card .pc-active {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
}
.config-add-btn {
  width: 100%;
  padding: 10px;
  background: transparent;
  border: 1px dashed var(--rule);
  border-radius: 10px;
  font-family: var(--font);
  font-size: 12.5px;
  color: var(--ink-quiet);
  cursor: pointer;
  margin-top: 8px;
  transition: background-color 0.2s, color 0.2s, border-color 0.2s;
}
.config-add-btn:hover {
  background: var(--card-soft);
  color: var(--ink-soft);
  border-color: rgba(255,255,255,0.12);
}

/* Config Form */
.config-form {
  padding: 14px;
}
.config-form-header {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 18px;
}
.config-form-title {
  font-size: 10.5px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-quiet);
  font-weight: 600;
  display: flex; align-items: center; gap: 8px;
}
.config-form-title::before {
  content: '';
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--mint);
  box-shadow: 0 0 6px var(--mint-glow);
}
.form-group {
  margin-bottom: 14px;
}
.form-label {
  font-size: 11px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ink-quiet);
  font-weight: 600;
  margin-bottom: 6px;
  display: block;
}
.form-label .optional {
  font-weight: 400;
  text-transform: none;
  letter-spacing: 0;
  color: var(--ink-faint);
  font-size: 10.5px;
}
.form-input {
  width: 100%;
  padding: 8px 12px;
  background: var(--card-soft);
  border: 1px solid var(--rule);
  border-radius: 8px;
  font-family: var(--font);
  font-size: 13px;
  color: var(--ink);
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.form-input:focus {
  border-color: var(--mint);
  box-shadow: 0 0 0 2px var(--mint-tint);
}
.form-input::placeholder { color: var(--ink-faint); }

/* Model Rows in Form */
.model-rows { margin-bottom: 8px; }
.model-row-entry {
  display: grid;
  grid-template-columns: 1fr 1fr 28px;
  gap: 6px;
  margin-bottom: 6px;
  align-items: center;
}
.model-row-entry .form-input {
  padding: 7px 10px;
  font-size: 12px;
}
.model-row-delete {
  background: none; border: none;
  color: var(--ink-faint);
  font-size: 14px;
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  transition: color 0.15s;
  text-align: center;
}
.model-row-delete:hover { color: var(--mint); }
.model-add-btn {
  background: none; border: none;
  color: var(--ink-quiet);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 0;
  transition: color 0.15s;
}
.model-add-btn:hover { color: var(--mint); }

/* Advanced Disclosure */
.advanced-section {
  margin-bottom: 18px;
}
.advanced-section summary {
  font-size: 12px;
  color: var(--ink-quiet);
  cursor: pointer;
  list-style: none;
  padding: 6px 0;
  transition: color 0.15s;
}
.advanced-section summary::-webkit-details-marker { display: none; }
.advanced-section summary:hover { color: var(--ink-soft); }
.advanced-section .adv-content {
  padding: 10px 0;
}
.toggle-row {
  display: flex; align-items: center; justify-content: space-between;
}
.toggle-label {
  font-size: 12.5px;
  color: var(--ink-soft);
}
.toggle-switch {
  position: relative;
  width: 36px; height: 20px;
  background: var(--card-quiet);
  border-radius: 999px;
  cursor: pointer;
  transition: background-color 0.2s;
  border: none; padding: 0;
}
.toggle-switch.on { background: var(--mint); }
.toggle-switch .toggle-knob {
  position: absolute;
  top: 2px; left: 2px;
  width: 16px; height: 16px;
  background: white;
  border-radius: 50%;
  transition: transform 0.2s;
}
.toggle-switch.on .toggle-knob {
  transform: translateX(16px);
}

/* Form Actions */
.form-actions {
  display: flex; gap: 8px;
  margin-top: 20px;
}
.form-actions .btn-cancel {
  flex: 1;
  padding: 9px;
  background: transparent;
  border: 1px solid var(--rule);
  border-radius: 8px;
  font-family: var(--font);
  font-size: 12.5px;
  color: var(--ink-quiet);
  cursor: pointer;
  transition: background-color 0.15s, color 0.15s;
}
.form-actions .btn-cancel:hover {
  background: var(--card-soft);
  color: var(--ink-soft);
}
.form-actions .btn-save {
  flex: 1;
  padding: 9px;
  background: var(--mint);
  color: #0a1010;
  border: none;
  border-radius: 8px;
  font-family: var(--font);
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
}
.form-actions .btn-save:hover { background: var(--mint-soft); }
```

- [ ] **Step 2: 添加 Config HTML**

在 `</div><!-- action-region -->` 后（sidebar 内）添加：

```html
<div class="config-overlay">
  <!-- Config Empty (Onboarding) -->
  <div class="config-empty">
    <div class="ce-dot"></div>
    <div class="ce-label">Configure</div>
    <div class="ce-title">Connect a model to begin</div>
    <div class="ce-desc">Add an LLM provider with an API key to start reviewing code.</div>
    <button class="ce-btn" onclick="showConfigForm()">+ Add Provider</button>
  </div>

  <!-- Config List -->
  <div class="config-list">
    <div class="config-list-header">
      <span class="config-list-title">Providers</span>
      <button class="config-list-close" onclick="closeConfig()">×</button>
    </div>
    <div id="providerList">
      <div class="provider-card" onclick="editProvider('Anthropic')">
        <span class="pc-name">Anthropic</span>
        <span class="pc-models">2 models</span>
        <span class="pc-active"></span>
      </div>
      <div class="provider-card" onclick="editProvider('OpenAI')">
        <span class="pc-name">OpenAI</span>
        <span class="pc-models">2 models</span>
      </div>
    </div>
    <button class="config-add-btn" onclick="showConfigForm()">+ Add Provider</button>
  </div>

  <!-- Config Form -->
  <div class="config-form">
    <div class="config-form-header">
      <span class="config-form-title">Add Provider</span>
    </div>
    <div class="form-group">
      <label class="form-label">Provider Name</label>
      <input class="form-input" type="text" placeholder="e.g. OpenAI, DeepSeek" id="formProviderName">
    </div>
    <div class="form-group">
      <label class="form-label">Base URL</label>
      <input class="form-input" type="text" placeholder="https://api.openai.com/v1" id="formBaseUrl">
    </div>
    <div class="form-group">
      <label class="form-label">API Key <span class="optional">(optional)</span></label>
      <input class="form-input" type="password" placeholder="sk-..." id="formApiKey">
    </div>
    <div class="form-group">
      <label class="form-label">Models</label>
      <div class="model-rows" id="modelRows">
        <div class="model-row-entry">
          <input class="form-input" type="text" placeholder="Model ID">
          <input class="form-input" type="text" placeholder="Display name">
          <button class="model-row-delete" onclick="deleteModelRow(this)">×</button>
        </div>
      </div>
      <button class="model-add-btn" onclick="addModelRow()">+ Add model</button>
    </div>
    <details class="advanced-section">
      <summary>▸ Advanced</summary>
      <div class="adv-content">
        <div class="toggle-row">
          <span class="toggle-label">Use Anthropic protocol</span>
          <button class="toggle-switch" onclick="this.classList.toggle('on')">
            <span class="toggle-knob"></span>
          </button>
        </div>
      </div>
    </details>
    <div class="form-actions">
      <button class="btn-cancel" onclick="cancelConfigForm()">Cancel</button>
      <button class="btn-save" onclick="saveConfigForm()">Save</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: 添加 Config 交互 JS**

在 `<script>` 中更新 `toggleConfig()` 并添加相关函数：

```javascript
function toggleConfig() {
  var current = document.body.dataset.config;
  if (current) {
    closeConfig();
  } else {
    // Show list if providers exist, empty if none
    document.body.dataset.config = 'list';
  }
}

function closeConfig() {
  document.body.dataset.config = '';
}

function showConfigForm() {
  document.body.dataset.config = 'form';
}

function cancelConfigForm() {
  document.body.dataset.config = 'list';
}

function saveConfigForm() {
  document.body.dataset.config = 'list';
  document.body.dataset.state = 'idle';
}

function editProvider(name) {
  document.getElementById('formProviderName').value = name;
  document.body.dataset.config = 'form';
  document.querySelector('.config-form-title').innerHTML = '<span style="width:6px;height:6px;border-radius:50%;background:var(--mint);box-shadow:0 0 6px var(--mint-glow);display:inline-block;margin-right:8px"></span>Edit Provider';
}

function addModelRow() {
  var rows = document.getElementById('modelRows');
  var entry = document.createElement('div');
  entry.className = 'model-row-entry';
  entry.innerHTML = '<input class="form-input" type="text" placeholder="Model ID"><input class="form-input" type="text" placeholder="Display name"><button class="model-row-delete" onclick="deleteModelRow(this)">×</button>';
  rows.appendChild(entry);
}

function deleteModelRow(btn) {
  var row = btn.closest('.model-row-entry');
  var rows = document.getElementById('modelRows');
  if (rows.children.length > 1) {
    row.remove();
  }
}
```

- [ ] **Step 4: 浏览器验证**

通过 DevTools 设置 `document.body.dataset.config` 切换：

验证：
1. `config="empty"`: 居中引导页，mint 圆点 + "Connect a model to begin" + 大主按钮
2. `config="list"`: Provider 列表，Anthropic（2 models + mint active 点）、OpenAI（2 models），底部 "+ Add Provider" 虚线按钮
3. `config="form"`: 表单完整——Provider Name / Base URL / API Key(password) / Models 多行（2 列 grid + × 删除）/ Advanced disclosure（toggle switch）/ Cancel + Save 按钮
4. 点击 "+ Add model" → 新增一行 model 输入
5. 点击 model 行 × → 删除该行（最后一行不可删）
6. toggle switch 点击切换 mint/灰
7. input focus 时有 mint 边框 + 发光
8. Config overlay 覆盖整个 sidebar（Setup/Action 不可见）

- [ ] **Step 5: Commit**

```bash
git add prototype.html
git commit -m "feat: add config overlay with empty/list/form views"
```

---

### Task 6: Demo Switcher + 状态机整合

**Files:**
- Modify: `prototype.html`

**验收标准：**
- 页面顶部 floating chips 行：Idle / Running / Done / Empty / Cancelled / Failed + Config Empty / Config List / Config Form
- 点击 chip 切换对应状态
- 活跃 chip 高亮为 mint
- 顶部状态条的 mint 脉动在 Failed 状态变灰（dim）

- [ ] **Step 1: 添加 Demo Switcher CSS**

在 Config CSS 之后追加：

```css
/* === 8. Demo Switcher === */
.demo-switcher {
  position: fixed;
  top: 10px; left: 50%;
  transform: translateX(-50%);
  z-index: 100;
  display: flex; gap: 4px;
  padding: 4px;
  background: rgba(10,13,18,0.9);
  border: 1px solid var(--rule);
  border-radius: 12px;
  backdrop-filter: none;
}
.demo-chip {
  padding: 5px 10px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: var(--ink-quiet);
  font-family: var(--font);
  font-size: 10.5px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: background-color 0.15s, color 0.15s;
}
.demo-chip:hover {
  background: var(--card-quiet);
  color: var(--ink-soft);
}
.demo-chip.active {
  background: var(--mint-tint);
  color: var(--mint);
  font-weight: 600;
}
.demo-sep {
  width: 1px;
  background: var(--rule);
  margin: 4px 2px;
}

/* === 9. State-dependent status bar adjustments === */
body[data-state="failed"] .status-dot { 
  background: var(--ink-faint); box-shadow: none; animation: none; 
}
body[data-state="running"] .setup { opacity: 0.6; pointer-events: none; }
```

- [ ] **Step 2: 添加 Demo Switcher HTML**

在 `<body>` 内最顶部（`<div class="vscode-chrome">` 之前）添加：

```html
<div class="demo-switcher">
  <button class="demo-chip active" onclick="switchState('idle')">Idle</button>
  <button class="demo-chip" onclick="switchState('running')">Running</button>
  <button class="demo-chip" onclick="switchState('done')">Done</button>
  <button class="demo-chip" onclick="switchState('empty')">Empty</button>
  <button class="demo-chip" onclick="switchState('cancelled')">Cancelled</button>
  <button class="demo-chip" onclick="switchState('failed')">Failed</button>
  <div class="demo-sep"></div>
  <button class="demo-chip" onclick="switchConfig('empty')">Cfg:Empty</button>
  <button class="demo-chip" onclick="switchConfig('list')">Cfg:List</button>
  <button class="demo-chip" onclick="switchConfig('form')">Cfg:Form</button>
</div>
```

- [ ] **Step 3: 添加 Demo Switcher JS**

在 `<script>` 中添加：

```javascript
function switchState(state) {
  document.body.dataset.state = state;
  document.body.dataset.config = '';
  updateDemoChips();
}

function switchConfig(view) {
  document.body.dataset.config = view;
  updateDemoChips();
}

function updateDemoChips() {
  var state = document.body.dataset.state;
  var config = document.body.dataset.config;
  document.querySelectorAll('.demo-chip').forEach(function(chip) {
    chip.classList.remove('active');
  });
  // Highlight active state chip
  document.querySelectorAll('.demo-chip').forEach(function(chip) {
    var text = chip.textContent.toLowerCase();
    if (text === state && !config) chip.classList.add('active');
    if (text === 'cfg:' + config && config) chip.classList.add('active');
  });
}
```

- [ ] **Step 4: 浏览器验证**

验证：
1. 顶部居中出现 demo chips 条，默认 "Idle" 高亮 mint
2. 点击 "Running" → Action 区显示 running 状态，chip 高亮切换，Setup 区变半透明不可交互
3. 点击 "Done" → 显示评论列表
4. 点击 "Failed" → 顶部 status dot 变灰（不脉动），显示错误卡
5. 点击 "Cfg:Empty" → config overlay 显示引导页，chip 高亮
6. 点击 "Cfg:Form" → config overlay 显示表单
7. 点击 "Idle" → config 关闭，回到正常视图

- [ ] **Step 5: Commit**

```bash
git add prototype.html
git commit -m "feat: add demo switcher and state machine integration"
```

---

### Task 7: Replay 动画

**Files:**
- Modify: `prototype.html`

**验收标准：**
- Running 状态下有一个 "▶ Replay" 浮动按钮
- 点击后，timeline 行逐个出现（200ms 间隔），stage-bar 依次推进
- Replay 完成后按钮恢复可点击

- [ ] **Step 1: 添加 Replay CSS**

追加到 CSS：

```css
/* Replay */
.replay-btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 5px 12px;
  background: rgba(255,255,255,0.04);
  border: 1px solid var(--rule);
  border-radius: 999px;
  font-family: var(--font);
  font-size: 11px;
  color: var(--ink-quiet);
  cursor: pointer;
  margin-top: 8px;
  transition: background-color 0.2s, color 0.2s;
}
.replay-btn:hover {
  background: rgba(255,255,255,0.08);
  color: var(--ink-soft);
}
.replay-btn:disabled { opacity: 0.4; cursor: not-allowed; }

.timeline-row.hidden { display: none; }
```

- [ ] **Step 2: 添加 Replay 按钮 HTML**

在 Running 区的 `<div style="clear:both"></div>` 前添加：

```html
<button class="replay-btn" onclick="startReplay()">▶ Replay</button>
```

- [ ] **Step 3: 添加 Replay JS**

```javascript
function startReplay() {
  var rows = document.querySelectorAll('.action-running .timeline-row');
  var replayBtn = document.querySelector('.replay-btn');
  replayBtn.disabled = true;
  
  // Hide all rows
  rows.forEach(function(row) { row.classList.add('hidden'); });
  
  // Show one by one
  var i = 0;
  var interval = setInterval(function() {
    if (i < rows.length) {
      rows[i].classList.remove('hidden');
      i++;
    } else {
      clearInterval(interval);
      replayBtn.disabled = false;
    }
  }, 200);
}
```

- [ ] **Step 4: 浏览器验证**

验证：
1. 切换到 Running 状态
2. 点击 "▶ Replay" → 所有 timeline 行消失
3. 行逐个出现，间隔 ~200ms
4. 完成后 Replay 按钮恢复可点击

- [ ] **Step 5: Commit**

```bash
git add prototype.html
git commit -m "feat: add replay animation for running timeline"
```

---

### Task 8: localStorage 持久化 + 首次安装检测

**Files:**
- Modify: `prototype.html`

**验收标准：**
- 刷新页面后状态保留（state / config / 选中的模型）
- 首次打开（localStorage 空）→ 自动进入 `config-empty` 视图
- 点击 Reset → 清空 localStorage 并刷新
- ⚙ 按钮在无 provider 时打开 `config-empty`，有 provider 时打开 `config-list`

- [ ] **Step 1: 添加 localStorage 持久化 JS**

替换现有 `<script>` 为完整版本，在文件底部（所有函数定义之后）添加初始化逻辑：

```javascript
// === Persistence ===
var STORAGE_KEY = 'ocrPrototype';

function loadState() {
  try {
    var saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
    if (!saved.providers || saved.providers.length === 0) {
      // First launch — show onboarding
      document.body.dataset.config = 'empty';
      document.body.dataset.state = 'idle';
      return;
    }
    document.body.dataset.state = saved.state || 'idle';
    document.body.dataset.config = saved.config || '';
    if (saved.activeModel) {
      document.querySelector('.status-model').textContent = saved.activeModel;
    }
    if (saved.activeProvider) {
      document.querySelector('.status-provider').textContent = saved.activeProvider;
    }
  } catch(e) {
    document.body.dataset.config = 'empty';
  }
  updateDemoChips();
}

function saveState() {
  var data = {
    state: document.body.dataset.state,
    config: document.body.dataset.config,
    activeModel: document.querySelector('.status-model').textContent,
    activeProvider: document.querySelector('.status-provider').textContent,
    providers: [{name: 'Anthropic'}, {name: 'OpenAI'}] // demo data
  };
  localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
}

// Observe state changes
var observer = new MutationObserver(function(mutations) {
  mutations.forEach(function(m) {
    if (m.attributeName === 'data-state' || m.attributeName === 'data-config') {
      saveState();
      updateDemoChips();
    }
  });
});
observer.observe(document.body, { attributes: true });

// Update toggleConfig to check providers
function toggleConfig() {
  var current = document.body.dataset.config;
  if (current) {
    closeConfig();
  } else {
    try {
      var saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      document.body.dataset.config = (saved.providers && saved.providers.length > 0) ? 'list' : 'empty';
    } catch(e) {
      document.body.dataset.config = 'empty';
    }
  }
}

function resetAll() {
  localStorage.removeItem(STORAGE_KEY);
  location.reload();
}

// Init on load
loadState();
```

- [ ] **Step 2: 浏览器验证**

验证：
1. 清空 localStorage（`localStorage.removeItem('ocrPrototype')`）并刷新 → 自动显示 config-empty 引导页
2. 点击 demo chip "Idle" → 刷新后仍为 Idle 状态
3. 切换到 "Done" → 刷新后仍为 Done 状态
4. 点击 Reset → 页面刷新，回到 config-empty 引导页
5. ⚙ 按钮点击 → 打开 config-list（因有 demo provider 数据）

- [ ] **Step 3: Commit**

```bash
git add prototype.html
git commit -m "feat: add localStorage persistence and first-launch detection"
```

---

### Task 9: 最终整合 + 禁区自检 + README

**Files:**
- Modify: `prototype.html`（顶部添加 README 注释）

**验收标准：**
- 禁区清单全部通过
- prototype.html 顶部有使用说明注释
- 所有交互流畅，无 console 错误
- 文件 `file://` 双击可正常打开和交互

- [ ] **Step 1: 在 prototype.html 顶部添加 README 注释**

在 `<!DOCTYPE html>` 前添加：

```html
<!--
  OCR VSCode Plugin UI Prototype
  ==============================
  双击此文件在浏览器中打开即可查看。

  使用方式：
  - 点击顶部 chip 切换 6 种 Action 状态（Idle / Running / Done / Empty / Cancelled / Failed）
  - 点击 Cfg:* chip 切换 3 种配置视图（Empty / List / Form）
  - 点击 ⚙ 进入配置管理
  - 点击 ▾ 切换模型
  - 点击 ▶ Replay 重放流式日志动画
  - hover status-bar 右上角出现 Reset，点击清空所有本地数据回到首次安装状态

  设计规范：silent-night-ui/style-reference.md
  产品设计：docs/superpowers/specs/2026-06-04-ocr-vscode-ui-design.md
-->
```

- [ ] **Step 2: 执行禁区自检**

逐项检查 prototype.html：

| 禁区项 | 预期 | 验证方式 |
|---|---|---|
| 单 mint accent | 全文无第二种 accent 颜色 | 搜索除 `--mint` 外的 accent hex |
| 无白色/米色卡片 | 所有 background 取 `--card` 系列或 `--bg` 系列 | 搜索 `#fff` / `#ffffff` / `white`（仅 checkbox knob 白色 OK）|
| 无渐变文字 | 无 `background-clip: text` | 全文搜索 |
| 无 emoji/SVG 装饰 | 无 emoji 字符，无 `<svg>` 标签 | 全文搜索 |
| 无 backdrop-filter | 无 blur | 搜索 `backdrop-filter` |
| 无 border-radius > 28px | 仅 `999px`（pill）例外 | 搜索 `border-radius` 值 |
| 无网络资源 | 无 CDN / Google Fonts / fetch / 远程图片 | 搜索 `http` / `fetch` / `url(` |
| dark-only | 无 `prefers-color-scheme` 或 light mode 切换 | 搜索 |
| severity 不分三色 | critical=mint-tint, warn=card-quiet, info=更深灰 | 目视 |

修复任何违规项。

- [ ] **Step 3: 全流程浏览器测试**

逐个走完以下流程：
1. 清空 localStorage → 刷新 → 应看到 config-empty 引导页
2. 点击 "+ Add Provider" → 看到表单
3. Cancel → 回到 empty
4. 点击 demo chip "Idle" → 看到 Setup + Idle
5. 勾选/取消文件 → 按钮文字和文件数更新
6. 切换 range pill → pill 文字变化
7. 点击 "Review all changes" → 进入 running 状态
8. 点击 Replay → timeline 逐行出现
9. 点击 Cancel → 进入 cancelled 状态
10. demo chip 切换 Done → 看到 5 张评论卡
11. Dismiss 一张 → 卡片淡出，summary 数字减一
12. Copy → 按钮变为 "Copied"
13. demo chip 切换 Empty / Failed / 各 Config 视图
14. ⚙ 按钮 toggle config 开关
15. dropdown ▾ 切换模型
16. Reset → 回到首次安装状态
17. 无 console 错误

- [ ] **Step 4: Commit**

```bash
git add prototype.html
git commit -m "feat: complete OCR VSCode UI prototype with all states and interactions"
```

---

## Self-Review Checklist

### Spec Coverage

| Spec 章节 | 覆盖 Task |
|---|---|
| §3 已确认产品形态 | Task 1-8 全覆盖 |
| §4 布局骨架（chrome + sidebar + editor） | Task 1 |
| §5.1 六种 Action 状态 | Task 4 |
| §5.2 三种 Config 视图 | Task 5 |
| §6.1 组件清单（24 组件） | Task 1-6 |
| §6.2 窄空间密度策略 | Task 3-5（padding/字号/圆角符合） |
| §7.1 状态机 | Task 6 |
| §7.2 Config 覆盖 | Task 5 |
| §7.3 Range pill | Task 3 |
| §7.4 文件勾选 | Task 3 |
| §7.5 伪流式 Replay | Task 7 |
| §7.6 评论 Dismiss | Task 4 |
| §7.7 Model-row 增删 | Task 5 |
| §7.8 localStorage 持久 | Task 8 |
| §8 文件结构 | 全局（单文件 HTML） |
| §9 视觉禁区 | Task 9 |

### Placeholder Scan

- 无 TBD / TODO / "implement later"
- 所有 step 含完整代码
- 所有函数名、类名在各 task 间保持一致

### Type Consistency

- `data-state`: idle / running / done / empty / cancelled / failed — 全局一致
- `data-config`: empty / list / form / ""（空）— 全局一致
- 函数名全部一致：`switchState()` / `switchConfig()` / `updateDemoChips()` / `toggleConfig()` / `toggleDropdown()` / `selectModel()` / `toggleRange()` / `selectRange()` / `updateFileCount()` / `startReview()` / `cancelReview()` / `retryReview()` / `openFile()` / `copyComment()` / `dismissComment()` / `showConfigForm()` / `cancelConfigForm()` / `saveConfigForm()` / `editProvider()` / `addModelRow()` / `deleteModelRow()` / `startReplay()` / `loadState()` / `saveState()` / `resetAll()`
