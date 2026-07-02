import { useT } from '../I18nProvider';

interface Props { onRetry: () => void; error?: string; }
export function FailedView({ onRetry, error }: Props) {
  const t = useT();
  return (
    <div class="action-failed" style="display:block">
      <div class="failed-card">
        <div class="fc-msg">{t('view.failed.title')}<br/>{error ? t('view.failed.checkConfig') : t('view.failed.checkApiKey')}</div>
        {error && <div class="fc-detail">{error}</div>}
        <button class="retry-pill" onClick={onRetry}>{t('view.failed.retry')}</button>
      </div>
    </div>
  );
}
