import { CliResult, CliRunOptions, LogLine, ReviewState } from '../../shared/types';
import { CliService } from './CliService';

export function resultToState(result: CliResult): ReviewState {
  if (result.comments.length > 0) return 'done';
  if (result.status === 'completed_with_errors') return 'failed';
  return 'empty';
}

export interface SessionCallbacks {
  onState: (state: ReviewState, error?: string) => void;
  onLog: (line: LogLine) => void;
  onDone: (result: CliResult) => void;
}

export class ReviewSession {
  private cancelled = false;

  constructor(private cli: CliService, private cwd: string) {}

  async run(opts: CliRunOptions, cb: SessionCallbacks): Promise<void> {
    this.cancelled = false;
    cb.onState('running');
    try {
      const result = await this.cli.review(opts, this.cwd, cb.onLog);
      if (this.cancelled) {
        cb.onState('cancelled');
        return;
      }
      cb.onState(resultToState(result));
      cb.onDone(result);
    } catch (e) {
      if (this.cancelled) {
        cb.onState('cancelled');
      } else {
        const msg = e instanceof Error ? e.message : String(e);
        cb.onLog({ text: `[ocr] ${msg}`, level: 'error' });
        cb.onState('failed', msg);
      }
    }
  }

  cancel(cb: Pick<SessionCallbacks, 'onState'>): void {
    this.cancelled = true;
    this.cli.cancel();
    cb.onState('cancelled');
  }
}
