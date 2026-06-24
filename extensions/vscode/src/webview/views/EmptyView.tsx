import { useState } from 'preact/hooks';
import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';

interface Props { logs?: LogLine[]; }

export function EmptyView({ logs = [] }: Props) {
  const [showLogs, setShowLogs] = useState(false);
  return (
    <div class="action-empty" style="display:block">
      <div class="empty-note">
        <div class="en-dot"></div>
        <div class="en-text">未发现问题 · 已通过</div>
      </div>

      {logs.length > 0 && (
        <div class="logs-disclosure">
          <button class="logs-toggle" onClick={() => setShowLogs(!showLogs)}>
            <span class={`logs-toggle-arrow${showLogs ? ' open' : ''}`}></span>
            过程日志
          </button>
          {showLogs && <LogViewer logs={logs} />}
        </div>
      )}
    </div>
  );
}
