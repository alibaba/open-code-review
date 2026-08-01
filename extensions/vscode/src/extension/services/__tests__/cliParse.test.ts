import { buildReviewArgs, extractCliError, parseCliResult, parseLogLine } from '../cliParse';
import { ReviewMode } from '../../../shared/types';

describe('buildReviewArgs', () => {
  it('workspace 模式加 --format json', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Workspace }))
      .toEqual(['review', '--format', 'json']);
  });

  it('branch 模式加 --from/--to', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Branch, from: 'main', to: 'dev' }))
      .toEqual(['review', '--from', 'main', '--to', 'dev', '--format', 'json']);
  });

  it('commit 模式加 --commit', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Commit, commit: 'abc123' }))
      .toEqual(['review', '--commit', 'abc123', '--format', 'json']);
  });

  it('customPrompt 追加 --background', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Workspace, customPrompt: '关注安全' }))
      .toEqual(['review', '--format', 'json', '--background', '关注安全']);
  });

  it('concurrency 追加 --concurrency', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Workspace, concurrency: 4 }))
      .toEqual(['review', '--format', 'json', '--concurrency', '4']);
  });
});

describe('parseCliResult', () => {
  it('解析 success + comments + summary，字段转 camelCase', () => {
    const raw = JSON.stringify({
      status: 'success',
      comments: [{
        path: 'src/a.ts', content: 'bug', start_line: 10, end_line: 12,
        suggestion_code: 'fix', existing_code: 'old',
      }],
      summary: {
        files_reviewed: 2, comments: 1, total_tokens: 100,
        input_tokens: 80, output_tokens: 20, elapsed: '5s',
      },
    });
    const r = parseCliResult(raw);
    expect(r.status).toBe('success');
    expect(r.comments[0]).toEqual({
      path: 'src/a.ts', content: 'bug', startLine: 10, endLine: 12,
      suggestionCode: 'fix', existingCode: 'old', thinking: undefined,
    });
    expect(r.summary?.filesReviewed).toBe(2);
  });

  it('skipped 状态无 comments', () => {
    const raw = JSON.stringify({ status: 'skipped', message: 'No supported files changed.', comments: [] });
    const r = parseCliResult(raw);
    expect(r.status).toBe('skipped');
    expect(r.comments).toEqual([]);
  });

  it('忽略 JSON 前的非 JSON 噪声行', () => {
    const raw = '[ocr] some log\n{"status":"success","comments":[]}';
    const r = parseCliResult(raw);
    expect(r.status).toBe('success');
  });
});

describe('extractCliError', () => {
  it('优先提取 Error: 行并去掉前缀', () => {
    const stderr = '[ocr] starting\nError: llm request failed: 401 unauthorized\n';
    expect(extractCliError(stderr)).toBe('llm request failed: 401 unauthorized');
  });
  it('多个 Error 行取最后一个', () => {
    const stderr = 'Error: first\nError: last';
    expect(extractCliError(stderr)).toBe('last');
  });
  it('无 Error 行时取最后一行非空内容', () => {
    expect(extractCliError('foo\nbar\n\n')).toBe('bar');
  });
  it('空 stderr → 空字符串', () => {
    expect(extractCliError('')).toBe('');
  });
});

describe('parseLogLine', () => {
  it('普通 [ocr] 行 → info', () => {
    expect(parseLogLine('[ocr] Reviewing src/a.ts')).toEqual({ text: '[ocr] Reviewing src/a.ts', level: 'info' });
  });
  it('含 Retrying 的行 → warn', () => {
    expect(parseLogLine('[llm] Retrying in 1.46s (attempt 1/3)').level).toBe('warn');
  });
  it('含 WARNING 的行 → warn', () => {
    expect(parseLogLine('[ocr] WARNING [x] f: m').level).toBe('warn');
  });
  it('空行 → null', () => {
    expect(parseLogLine('   ')).toBeNull();
  });
});
