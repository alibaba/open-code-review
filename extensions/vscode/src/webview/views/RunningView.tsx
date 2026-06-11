import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';

interface Props { logs: LogLine[]; onCancel: () => void; }

export function RunningView({ logs, onCancel }: Props) {
  return (
    <div class="action-running" style="display:block">
      <div class="files-label">审查日志</div>
      <LogViewer logs={logs} />
      <button class="cancel-pill" onClick={onCancel}>取消</button>
      <div style="clear:both"></div>
    </div>
  );
}
