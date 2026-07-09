import { useState } from 'preact/hooks';
import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';
import { useT } from '../I18nProvider';

interface Props { logs?: LogLine[]; }

export function EmptyView({ logs = [] }: Props) {
  const [showLogs, setShowLogs] = useState(false);
  const t = useT();
  return (
    <div class="action-empty" style="display:block">
      <div class="empty-note">
        <div class="en-dot"></div>
        <div class="en-text">{t('view.empty.noIssues')}</div>
      </div>

      {logs.length > 0 && (
        <div class="logs-disclosure">
          <button class="logs-toggle" onClick={() => setShowLogs(!showLogs)}>
            <span class={`logs-toggle-arrow${showLogs ? ' open' : ''}`}></span>
            {t('view.empty.processLog')}
          </button>
          {showLogs && <LogViewer logs={logs} />}
        </div>
      )}
    </div>
  );
}
