import { useT } from '../I18nProvider';
import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';

interface Props { logs: LogLine[]; onCancel: () => void; }

export function RunningView({ logs, onCancel }: Props) {
  const t = useT();
  return (
    <div class="action-running" style="display:block">
      <div class="files-label">{t('view.running.reviewLog')}</div>
      <LogViewer logs={logs} />
      <button class="cancel-pill" onClick={onCancel}>{t('view.running.cancel')}</button>
      <div style="clear:both"></div>
    </div>
  );
}
