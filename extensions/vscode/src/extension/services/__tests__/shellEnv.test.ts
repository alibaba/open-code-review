// src/extension/services/__tests__/shellEnv.test.ts
process.env.OCR_SKIP_SHELL_RESOLVE = '1';
import { parseEnvBlock, getShellEnv } from '../shellEnv';

const DELIM = '_OCR_ENV_DELIM_';

describe('parseEnvBlock', () => {
  it('解析分隔标记之间的 key=value', () => {
    const stdout = `noise\n${DELIM}\nPATH=/usr/local/bin:/usr/bin\nFOO=bar\n${DELIM}\ntrailing`;
    expect(parseEnvBlock(stdout)).toEqual({
      PATH: '/usr/local/bin:/usr/bin',
      FOO: 'bar',
    });
  });

  it('value 中含 = 时只按首个 = 切分', () => {
    const stdout = `${DELIM}\nKEY=a=b=c\n${DELIM}`;
    expect(parseEnvBlock(stdout)).toEqual({ KEY: 'a=b=c' });
  });

  it('无分隔标记 → 空对象', () => {
    expect(parseEnvBlock('PATH=/usr/bin')).toEqual({});
  });
});

describe('getShellEnv', () => {
  it('总是包含 PATH', () => {
    expect(getShellEnv().PATH).toBeDefined();
  });
});
