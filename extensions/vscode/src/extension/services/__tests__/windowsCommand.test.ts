// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import * as fs from 'fs';
import { parseWindowsCmdShim, resolveWindowsCommand } from '../windowsCommand';

jest.mock('fs', () => ({
  ...jest.requireActual('fs'),
  existsSync: jest.fn(),
  readFileSync: jest.fn(),
}));

const env: NodeJS.ProcessEnv = {
  PATH: 'C:\\Program Files\\nodejs;C:\\Tools',
  PATHEXT: '.COM;.EXE;.BAT;.CMD',
};

function mockFiles(files: Record<string, string>): void {
  jest.mocked(fs.existsSync).mockImplementation(
    (candidate: fs.PathLike) => Object.prototype.hasOwnProperty.call(files, String(candidate)),
  );
  jest.mocked(fs.readFileSync).mockImplementation(((
    candidate: fs.PathLike,
  ) => {
    const contents = files[String(candidate)];
    if (contents === undefined) {
      throw Object.assign(new Error(`ENOENT: ${String(candidate)}`), { code: 'ENOENT' });
    }
    return contents;
  }) as typeof fs.readFileSync);
}

describe('resolveWindowsCommand', () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  it('resolves a .cmd shim in a path with spaces without passing user args through a shell', () => {
    const shim = 'C:\\Program Files\\nodejs\\ocr.cmd';
    const node = 'C:\\Program Files\\nodejs\\node.exe';
    const jsEntry = 'C:\\Program Files\\nodejs\\node_modules\\@alibaba\\open-code-review\\bin\\ocr.js';
    const args = ['--message', 'a & b | c %PATH% ^ "quoted"'];

    mockFiles({
      [shim]: [
        '@ECHO off',
        'SET "NODE_EXE=%~dp0\\node.exe"',
        'SET "OCR_CLI_JS=%~dp0\\node_modules\\@alibaba\\open-code-review\\bin\\ocr.js"',
        '"%NODE_EXE%" "%OCR_CLI_JS%" %*',
      ].join('\r\n'),
      [node]: '',
      [jsEntry]: '',
    });

    const resolved = resolveWindowsCommand(shim, args, env);

    expect(resolved).toEqual({ executable: node, args: [jsEntry, ...args] });
    expect(resolved).not.toHaveProperty('shell');
  });

  it('resolves a bare command through PATH and PATHEXT', () => {
    const shim = 'C:\\Tools\\ocr.cmd';
    const node = 'C:\\Tools\\node.exe';
    const jsEntry = 'C:\\Tools\\lib\\ocr.js';

    mockFiles({
      [shim]: [
        'SET "CLI_JS=%~dp0\\lib\\ocr.js"',
        'SET "NODE_EXE=%~dp0\\node.exe"',
        '"%NODE_EXE%" "%CLI_JS%" %*',
      ].join('\n'),
      [node]: '',
      [jsEntry]: '',
    });

    expect(resolveWindowsCommand('ocr', ['review'], env)).toEqual({
      executable: node,
      args: [jsEntry, 'review'],
    });
  });

  it('leaves executable commands and their arguments unchanged', () => {
    const executable = 'C:\\Tools\\ocr.exe';

    mockFiles({ [executable]: '' });

    expect(resolveWindowsCommand(executable, ['a & b'], env)).toEqual({
      executable,
      args: ['a & b'],
    });
  });

  it('rejects a command that cannot be found', () => {
    mockFiles({});

    expect(() => resolveWindowsCommand('missing-command', [], env)).toThrow(
      'Unable to resolve Windows command "missing-command" on PATH.',
    );
  });

  it('rejects a shim that does not invoke a JavaScript entry point', () => {
    const shim = 'C:\\Tools\\tool.cmd';

    mockFiles({
      [shim]: '@ECHO off\r\necho not a node shim\r\n',
    });

    expect(() => resolveWindowsCommand(shim, [], env)).toThrow(
      'does not invoke a Node.js entry point',
    );
  });

  it('rejects a shim whose JavaScript entry point is missing', () => {
    const shim = 'C:\\Tools\\tool.cmd';
    const node = 'C:\\Tools\\node.exe';

    mockFiles({
      [shim]: '@ECHO off\r\nSET "NODE_EXE=%~dp0\\node.exe"\r\n"%NODE_EXE%" "%~dp0\\missing.js" %*\r\n',
      [node]: '',
    });

    expect(() => resolveWindowsCommand(shim, [], env)).toThrow(
      'points to a missing entry point',
    );
  });

  it('rejects a shim when node.exe cannot be located', () => {
    const shim = 'C:\\Tools\\tool.cmd';
    const jsEntry = 'C:\\Tools\\tool.js';

    mockFiles({
      [shim]: '@ECHO off\r\nnode "%~dp0\\tool.js" %*\r\n',
      [jsEntry]: '',
    });

    expect(() => resolveWindowsCommand(shim, [], { ...env, PATH: 'C:\\Empty' })).toThrow(
      'Unable to locate node.exe',
    );
  });
});

describe('parseWindowsCmdShim', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('supports legacy SET syntax, percent variables, and an empty PATH', () => {
    const shim = 'C:\\Tools\\tool.cmd';
    const jsEntry = 'C:\\Tools\\dist\\tool.js';

    mockFiles({ [jsEntry]: '' });

    expect(parseWindowsCmdShim(
      [
        'set CLI_JS=%~dp0\\dist\\tool.js',
        'set NODE_EXE=%~dp0\\node.exe',
        '"%NODE_EXE%" "%CLI_JS%" %*',
      ].join('\n'),
      shim,
      {},
    )).toEqual({ jsEntry, nodeHint: 'C:\\Tools\\node.exe' });
  });
});
