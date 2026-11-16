// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useState } from 'preact/hooks';
import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';
import { useT } from '../I18nProvider';

interface Props { onRetry: () => void; error?: string; logs?: LogLine[]; }
export function FailedView({ onRetry, error, logs = [] }: Props) {
  const [showLogs, setShowLogs] = useState(false);
  const t = useT();
  return (
    <div class="action-failed" style="display:block">
      <div class="failed-card">
        <div class="fc-msg">{t('view.failed.title')}<br/>{error ? t('view.failed.checkConfig') : t('view.failed.checkApiKey')}</div>
        {error && <div class="fc-detail">{error}</div>}
        <button class="retry-pill" onClick={onRetry}>{t('view.failed.retry')}</button>
      </div>

      {logs.length > 0 && (
        <div class="logs-disclosure">
          <button class="logs-toggle" onClick={() => setShowLogs(!showLogs)}>
            <span class={`logs-toggle-arrow${showLogs ? ' open' : ''}`}></span>
            {t('view.failed.processLog')}
          </button>
          {showLogs && <LogViewer logs={logs} />}
        </div>
      )}
    </div>
  );
}
