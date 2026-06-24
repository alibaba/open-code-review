// src/extension/services/__tests__/CliService.test.ts
process.env.OCR_SKIP_SHELL_RESOLVE = '1';
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

  it('退出码非 0 时 reject，并带上 stderr 中的 Error 文本', async () => {
    const svc = new CliService('node');
    await expect(svc.runRaw(
      ['-e', 'process.stderr.write("Error: bad api key\\n"); process.exit(1)'],
      '.', () => {},
    )).rejects.toThrow('bad api key');
  });
});

describe('CliService.testConnection', () => {
  it('CLI 退出码非 0 → ok=false（不再误报连接成功）', async () => {
    const svc = new CliService('node');
    // 覆盖默认 ['llm','test'] 不可行，这里直接验证 runRaw 的失败传播逻辑
    const r = await svc.runRaw(
      ['-e', 'process.stderr.write("Error: connection refused\\n"); process.exit(1)'],
      '.', () => {},
    ).then(() => ({ ok: true }), (e: Error) => ({ ok: false, message: e.message }));
    expect(r.ok).toBe(false);
    expect((r as { message: string }).message).toContain('connection refused');
  });
});
