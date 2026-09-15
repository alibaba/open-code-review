#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Contract tests for the security-sensitive MCP documentation surface.
//
// Run via: node scripts/github-actions/check-mcp-docs.test.js
// (also wired as `npm run test:github-actions`).

const assert = require("assert");
const fs = require("fs");
const path = require("path");

const ROOT = path.resolve(__dirname, "../..");
const DOCS = path.join(ROOT, "pages/src/content/docs");
const COMPLETE_LOCALES = ["en", "zh", "ja", "ru"];
const SUPPORT_PAGES = ["configuration.md", "cli-reference.md", "integrations/ci.md"];

// Both push and PR filters must run this contract for documentation-only edits.
const actionWorkflow = fs.readFileSync(path.join(ROOT, ".github/workflows/action-contract.yml"), "utf8");
for (const triggerPath of ["pages/src/content/docs/**", "pages/src/i18n/**"]) {
  assert.strictEqual(actionWorkflow.split(`- '${triggerPath}'`).length - 1, 2,
    `Both workflow event filters must include ${triggerPath}`);
}

const COMMANDS = [
  "ocr mcp",
  "ocr mcp add [name]",
  "ocr mcp import [file] [--yes]",
  "ocr mcp list [--json]",
  "ocr mcp show <name> [--json]",
  "ocr mcp edit <name>",
  "ocr mcp discover <name> [--json] [--yes]",
  "ocr mcp tools <name> [--enable TOOL ...] [--disable TOOL ...] [--yes]",
  "ocr mcp permissions [name]",
  "ocr mcp enable <name>",
  "ocr mcp disable <name>",
  "ocr mcp remove <name> [--yes]",
];

const CONFIG_TOKENS = [
  "mcp.version",
  "mcp.enabled",
  "mcp.default_permission",
  "mcp.approval_timeout_seconds",
  "mcp_servers",
  "default_permission",
  "tools",
  "tool_permissions",
  "tool_definition_sha256",
  "allow_insecure_http",
];

// These expressions intentionally describe outcomes, not only vocabulary. The
// localized alternatives make a mistranslation that reverses a security rule a
// blocking documentation failure.
const INVARIANTS = {
  en: [
    [/`ocr scan` is not loaded by MCP|MCP is not loaded by `ocr scan`/i, "MCP is review-only"],
    [/empty or missing `tools` list means \*\*no tools\*\*/i, "empty allowlist exposes nothing"],
    [/two independent gates[\s\S]*execution authorization/i, "visibility and execution are independent"],
    [/rejection, cancellation, timeout[\s\S]*no `tools\/call`/i, "failed approval never calls the tool"],
    [/Neither an[\s\S]*`allow` permission[\s\S]*can add a tool[\s\S]*allowlist/i, "allow cannot widen capabilities"],
    [/annotations are untrusted[\s\S]*never grants permission/i, "annotations never authorize"],
    [/Discovery performs[\s\S]*`tools\/list`[\s\S]*does\s+not call `tools\/call`[\s\S]*register tools with the model/i, "discovery is non-executing"],
    [/mcp__<server-slug>__<tool-slug>__<16-hex>[\s\S]*collision[\s\S]*zero\s+partial exposure/i, "names and collisions fail closed"],
    [/does\s+not inherit the full\s+OCR process environment[\s\S]*redacted[\s\S]*Cross-origin redirects/i, "credentials and redirects are constrained"],
  ],
  zh: [
    [/`ocr scan` 不会加载 MCP/i, "MCP is review-only"],
    [/`tools` 缺失或为空都表示\*\*零工具\*\*/i, "empty allowlist exposes nothing"],
    [/两道安全关卡[\s\S]*执行授权/i, "visibility and execution are independent"],
    [/拒绝、取消、超时[\s\S]*不会发送[\s\S]*`tools\/call`/i, "failed approval never calls the tool"],
    [/`allow` 权限[\s\S]*不能扩大白名单/i, "allow cannot widen capabilities"],
    [/annotation[\s\S]*绝不会自动授权/i, "annotations never authorize"],
    [/发现阶段[\s\S]*`tools\/list`[\s\S]*不会调用 `tools\/call`[\s\S]*不会把工具交给模型/i, "discovery is non-executing"],
    [/mcp__<server-slug>__<tool-slug>__<16-hex>[\s\S]*冲突[\s\S]*零部分暴露/i, "names and collisions fail closed"],
    [/不会继承 OCR 进程的完整环境[\s\S]*脱敏[\s\S]*跨源重定向/i, "credentials and redirects are constrained"],
  ],
  ja: [
    [/`ocr scan` は MCP を読み込みません/i, "MCP is review-only"],
    [/`tools` が空または未指定なら[\s\S]*ツールなし/i, "empty allowlist exposes nothing"],
    [/2 つのセキュリティゲート[\s\S]*実行時認可/i, "visibility and execution are independent"],
    [/拒否、キャンセル、[\s\S]*タイムアウト[\s\S]*`tools\/call` は送信されません/i, "failed approval never calls the tool"],
    [/`allow`[\s\S]*allowlist を広げません/i, "allow cannot widen capabilities"],
    [/annotation[\s\S]*認可には使われません/i, "annotations never authorize"],
    [/発見[\s\S]*`tools\/list`[\s\S]*`tools\/call`[\s\S]*モデルへのツール登録[\s\S]*行いません/i, "discovery is non-executing"],
    [/mcp__<server-slug>__<tool-slug>__<16-hex>[\s\S]*衝突[\s\S]*部分的にも公開されません/i, "names and collisions fail closed"],
    [/環境全体は継承しません[\s\S]*マスク[\s\S]*cross-origin redirect/i, "credentials and redirects are constrained"],
  ],
  ru: [
    [/`ocr scan` MCP не загружает/i, "MCP is review-only"],
    [/Пустой или отсутствующий `tools` означает[\s\S]*ноль инструментов/i, "empty allowlist exposes nothing"],
    [/Два независимых рубежа безопасности[\s\S]*Авторизация выполнения/i, "visibility and execution are independent"],
    [/Отказ,[\s\S]*отмена,[\s\S]*тайм-аут[\s\S]*`tools\/call` не отправляется/i, "failed approval never calls the tool"],
    [/`allow`[\s\S]*не расширяют allowlist/i, "allow cannot widen capabilities"],
    [/annotations[\s\S]*никогда не даёт разрешение/i, "annotations never authorize"],
    [/Discovery[\s\S]*`tools\/list`[\s\S]*не\s+вызывает `tools\/call`[\s\S]*не регистрирует инструменты для модели/i, "discovery is non-executing"],
    [/mcp__<server-slug>__<tool-slug>__<16-hex>[\s\S]*конфликт[\s\S]*частичной видимости/i, "names and collisions fail closed"],
    [/полное окружение OCR\s+не наследуется[\s\S]*маскир[\s\S]*Cross-origin redirects/i, "credentials and redirects are constrained"],
  ],
};

function read(relativePath) {
  return fs.readFileSync(path.join(DOCS, relativePath), "utf8");
}

for (const locale of COMPLETE_LOCALES) {
  const page = read(`${locale}/mcp.md`);

  for (const token of ["Space", "Ctrl-B", "Esc", "OCR_CONFIG_PATH", "Save disabled without connecting", "Import connection (JSON / TOML)", "Ctrl-S", "Tools and individual permissions", "1 MiB", "OAuth", "CONFIGURED SERVERS", "MANAGEMENT ACTIONS", "Tab", "PgUp/PgDn", "Ctrl-C"]) {
    assert.ok(page.includes(token), `${locale}/mcp.md must describe the actual terminal flow: ${token}`);
  }

  for (const command of COMMANDS) {
    assert.ok(page.includes(command), `${locale}/mcp.md must document: ${command}`);
  }
  for (const token of CONFIG_TOKENS) {
    assert.ok(page.includes(token), `${locale}/mcp.md must document config token: ${token}`);
  }
  assert.match(page, /approval_timeout_seconds[\s\S]*(1[–-]600|1 through 600|1–600)/i,
    `${locale}/mcp.md must document the 1-600 second approval timeout`);

  assert.strictEqual(INVARIANTS[locale].length, 9, `${locale} must define nine MCP invariants`);
  for (const [pattern, description] of INVARIANTS[locale]) {
    assert.match(page, pattern, `${locale}/mcp.md must preserve invariant: ${description}`);
  }

  for (const supportPage of SUPPORT_PAGES) {
    const support = read(`${locale}/${supportPage}`);
    assert.ok(support.includes("OCR_CONFIG_PATH"), `${locale}/${supportPage} must document configuration isolation`);
    assert.ok(support.includes("../mcp/"), `${locale}/${supportPage} must link to the MCP guide`);
  }

  const configuration = read(`${locale}/configuration.md`);
  assert.ok(configuration.includes("mcp.approval_timeout_seconds"),
    `${locale}/configuration.md must describe the configurable approval timeout`);
  assert.ok(configuration.includes("tool_definition_sha256"),
    `${locale}/configuration.md must describe definition fingerprints`);

  const cli = read(`${locale}/cli-reference.md`);
  assert.ok(cli.includes("ocr mcp permissions [name]"),
    `${locale}/cli-reference.md must include the permissions command`);
  assert.ok(cli.includes("mcp.approval_timeout_seconds"),
    `${locale}/cli-reference.md must describe the configurable approval timeout`);

  const ci = read(`${locale}/integrations/ci.md`);
  assert.match(ci, /fails? closed/i, `${locale}/integrations/ci.md must document fail-closed CI behavior`);
  for (const token of ["tool_definition_sha256", "tools/call"]) {
    assert.ok(ci.includes(token), `${locale}/integrations/ci.md must document CI contract token: ${token}`);
  }
}

for (const supportPage of SUPPORT_PAGES) {
  const korean = read(`ko/${supportPage}`);
  assert.ok(korean.includes("../mcp/"), `ko/${supportPage} must link to the English MCP fallback`);
  assert.match(korean, /영문 MCP 가이드/, `ko/${supportPage} must label the fallback as English`);
}

assert.ok(read("ko/configuration.md").includes("mcp.approval_timeout_seconds"),
  "ko/configuration.md must document the configurable approval timeout");
assert.ok(read("ko/cli-reference.md").includes("ocr mcp permissions [name]"),
  "ko/cli-reference.md must include the permissions command");
assert.match(read("ko/integrations/ci.md"), /fails? closed/i,
  "ko/integrations/ci.md must document fail-closed CI behavior");
for (const token of ["tool_definition_sha256", "tools/call"]) {
  assert.ok(read("ko/integrations/ci.md").includes(token),
    `ko/integrations/ci.md must document CI contract token: ${token}`);
}

for (const locale of ["en", "zh", "ja", "ru", "ko"]) {
  const i18n = fs.readFileSync(path.join(ROOT, `pages/src/i18n/${locale}.ts`), "utf8");
  assert.doesNotMatch(i18n, /['"]docs\.mcp(?:Title|Desc|Config|Delete|Fields|Field|Yes|No|Note|Example)/,
    `${locale}.ts must not retain the unused legacy MCP copy block`);
  assert.match(i18n, /['"]docs\.sidebar\.mcp['"]\s*:/,
    `${locale}.ts must retain the MCP sidebar label`);
}

console.log("MCP documentation contracts passed for en, zh, ja, ru, and ko fallback pages.");
