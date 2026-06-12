interface Props { onRetry: () => void; error?: string; }
export function FailedView({ onRetry, error }: Props) {
  return (
    <div class="action-failed" style="display:block">
      <div class="failed-card">
        <div class="fc-msg">审查失败。<br/>{error ? '请检查模型配置后重试。' : '请检查 API Key 和网络连接。'}</div>
        {error && <div class="fc-detail">{error}</div>}
        <button class="retry-pill" onClick={onRetry}>重试</button>
      </div>
    </div>
  );
}
