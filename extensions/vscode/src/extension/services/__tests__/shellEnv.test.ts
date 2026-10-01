// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/shellEnv.test.ts
process.env.OCR_SKIP_SHELL_RESOLVE = '1';
import { parseEnvBlock, getShellEnv } from '../shellEnv';

const DELIM = '_OCR_ENV_DELIM_';

describe('parseEnvBlock', () => {
  it('parses key=value pairs between delimiter markers', () => {
    const stdout = `noise\n${DELIM}\nPATH=/usr/local/bin:/usr/bin\nFOO=bar\n${DELIM}\ntrailing`;
    expect(parseEnvBlock(stdout)).toEqual({
      PATH: '/usr/local/bin:/usr/bin',
      FOO: 'bar',
    });
  });

  it('splits at the first = when the value contains =', () => {
    const stdout = `${DELIM}\nKEY=a=b=c\n${DELIM}`;
    expect(parseEnvBlock(stdout)).toEqual({ KEY: 'a=b=c' });
  });

  it('returns an empty object when delimiter markers are missing', () => {
    expect(parseEnvBlock('PATH=/usr/bin')).toEqual({});
  });
});

describe('getShellEnv', () => {
  it('always includes PATH', () => {
    expect(getShellEnv().PATH).toBeDefined();
  });
});
