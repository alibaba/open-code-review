// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { CliResult, CliRunOptions, LogLine, ReviewState } from '@shared/types';
import { CliService } from './CliService';

// `ocr review` reports the manifest terminal state (complete | partial | failed | skipped);
// completed_with_errors is the manifest-less status. The statuses below mean some or all
// items were not reviewed, so they are never a clean pass.
const INCOMPLETE_STATUSES: ReadonlySet<string> = new Set(['completed_with_errors', 'partial', 'failed']);

export function resultToState(result: CliResult): ReviewState {
  if (result.comments.length > 0) return 'done';
  if (INCOMPLETE_STATUSES.has(result.status)) return 'failed';
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
      const state = resultToState(result);
      const error = state === 'failed'
        ? result.message || `Review did not complete successfully (${result.status})`
        : undefined;
      cb.onState(state, error);
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
