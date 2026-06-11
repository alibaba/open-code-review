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
});
