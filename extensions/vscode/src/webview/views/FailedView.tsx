interface Props { onRetry: () => void; }
export function FailedView({ onRetry }: Props) {
  return (
    <div class="action-failed" style="display:block">
      <div class="failed-card">
        <div class="fc-msg">审查失败。<br/>请检查 API Key 和网络连接。</div>
        <button class="retry-pill" onClick={onRetry}>重试</button>
      </div>
    </div>
  );
}
