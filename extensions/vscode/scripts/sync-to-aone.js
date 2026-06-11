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
