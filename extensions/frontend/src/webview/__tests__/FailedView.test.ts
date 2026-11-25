// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { h } from 'preact';
import { renderToString } from 'preact-render-to-string';
import { CliResult } from '../../shared/types';
import { initialState, reducer } from '../store';
import { FailedView } from '../views/FailedView';

// `ocr review` reports a partial run (some items failed) as `partial`. The
// extension host shows it as failed; the process log of that run must stay
// reachable from the failed view, as it is from the empty and done views.
describe('FailedView after a partial review', () => {
  const message = 'Review partially completed: 0 finding(s); 2 of 5 selected item(s) failed.';
  const partial: CliResult = { status: 'partial', comments: [], warnings: [], message };

  function runPartialReview() {
    let s = reducer(initialState, { type: 'stateChange', state: 'running' });
    s = reducer(s, { type: 'logLine', line: { text: 'reviewing a.go', level: 'info' } });
    s = reducer(s, { type: 'logLine', line: { text: 'b.go: LLM request failed', level: 'warn' } });
    s = reducer(s, { type: 'stateChange', state: 'failed', error: message });
    return reducer(s, { type: 'reviewDone', result: partial });
  }

  it('keeps the failed view and the run logs after completion', () => {
    const s = runPartialReview();
    expect(s.view).toBe('failed');
    expect(s.session.error).toBe(message);
    expect(s.logs.map((l) => l.text)).toEqual(['reviewing a.go', 'b.go: LLM request failed']);
  });

  it('offers the process log toggle alongside the CLI message', () => {
    const s = runPartialReview();
    const html = renderToString(h(FailedView, { error: s.session.error, logs: s.logs, onRetry: () => {} }));
    expect(html).toContain(message);
    expect(html).toContain('logs-toggle');
  });

  it('shows no log toggle when the run produced no logs', () => {
    const html = renderToString(h(FailedView, { error: message, logs: [], onRetry: () => {} }));
    expect(html).not.toContain('logs-toggle');
  });
});
