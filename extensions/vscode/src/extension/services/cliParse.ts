import { CliResult, CliRunOptions, LogLine, ReviewComment } from '../../shared/types';

export function buildReviewArgs(opts: CliRunOptions): string[] {
  const args: string[] = ['review'];
  if (opts.mode === 'branch') {
    if (opts.from) args.push('--from', opts.from);
    if (opts.to) args.push('--to', opts.to);
  } else if (opts.mode === 'commit') {
    if (opts.commit) args.push('--commit', opts.commit);
  }
  args.push('--format', 'json');
  // JSON 结果走 stdout，进度日志走 stderr，供扩展实时回显
  // TODO: 待 CLI 发布支持 --progress-stderr 后再启用（当前已安装版本不识别该 flag）
  // args.push('--progress-stderr');
  if (opts.customPrompt && opts.customPrompt.trim()) {
    args.push('--background', opts.customPrompt.trim());
  }
  if (typeof opts.concurrency === 'number') {
    args.push('--concurrency', String(opts.concurrency));
  }
  return args;
}

function toComment(raw: any): ReviewComment {
  return {
    path: raw.path,
    content: raw.content,
    suggestionCode: raw.suggestion_code || undefined,
    existingCode: raw.existing_code || undefined,
    startLine: raw.start_line,
    endLine: raw.end_line,
    thinking: raw.thinking || undefined,
  };
}

export function parseCliResult(stdout: string): CliResult {
  const start = stdout.indexOf('{');
  if (start < 0) throw new Error('no JSON in CLI output');
  const json = JSON.parse(stdout.slice(start));
  const s = json.summary;
  return {
    status: json.status,
    message: json.message,
    comments: Array.isArray(json.comments) ? json.comments.map(toComment) : [],
    warnings: Array.isArray(json.warnings) ? json.warnings : [],
    summary: s ? {
      filesReviewed: s.files_reviewed,
      comments: s.comments,
      totalTokens: s.total_tokens,
      inputTokens: s.input_tokens,
      outputTokens: s.output_tokens,
      elapsed: s.elapsed,
    } : undefined,
  };
}

/** 从 CLI stderr 中提取最有用的报错文本：优先 `Error:` 行，否则取最后一行非空内容。 */
export function extractCliError(stderr: string): string {
  const lines = stderr.split('\n').map((l) => l.trim()).filter(Boolean);
  const errLine = [...lines].reverse().find((l) => /^error:/i.test(l));
  if (errLine) return errLine.replace(/^error:\s*/i, '');
  return lines.length ? lines[lines.length - 1] : '';
}

export function parseLogLine(raw: string): LogLine | null {
  const text = raw.replace(/\s+$/, '');
  if (!text.trim()) return null;
  const level: LogLine['level'] = /retrying|warning|warn/i.test(text) ? 'warn' : 'info';
  return { text, level };
}
