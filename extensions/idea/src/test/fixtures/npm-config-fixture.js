// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

const [root, cli] = process.argv.slice(2);
const windows = process.platform === 'win32';
const repository = path.resolve(__dirname, '../../../../..');
const prefix = path.join(root, 'npm prefix with spaces');
const fixture = path.join(root, 'package');
for (const directory of ['bin', 'scripts']) fs.mkdirSync(path.join(fixture, directory), { recursive: true });
for (const file of ['bin/ocr.js', 'scripts/platform.js', 'scripts/version.js']) {
  fs.copyFileSync(path.join(repository, file), path.join(fixture, file));
}
const native = path.join(fixture, 'bin', windows ? 'opencodereview.exe' : 'opencodereview');
fs.copyFileSync(cli, native);
fs.writeFileSync(path.join(fixture, 'package.json'), JSON.stringify({
  name: 'ocr-idea-config-fixture', version: '1.0.0', bin: { ocr: 'bin/ocr.js' },
}));
const args = ['install', '--global', '--prefix', prefix, '--ignore-scripts', '--no-audit', '--no-fund', '--offline', fixture];
const installed = spawnSync('npm', windows ? args.map((arg) => `"${arg}"`) : args, {
  env: { ...process.env, npm_config_cache: path.join(root, 'npm cache') },
  encoding: 'utf8', shell: windows, timeout: 30000,
});
if (installed.error || installed.status !== 0) throw new Error(`npm install failed: ${installed.error || installed.stderr}`);
const configPath = path.join(root, 'custom storage', 'config.json');
fs.mkdirSync(path.dirname(configPath), { recursive: true });
fs.writeFileSync(configPath, JSON.stringify({ provider: 'openai', model: 'saved-model' }));
console.log(JSON.stringify({
  prefix, native, configPath, launcher: path.join(prefix, windows ? 'ocr.cmd' : 'bin/ocr'),
}));
