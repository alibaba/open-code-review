import { useState } from 'preact/hooks';
import { CliResult, CommentStatus, LogLine } from '../../shared/types';
import { CommentCard } from '../components/CommentCard';
import { LogViewer } from '../components/LogViewer';
import { useT } from '../I18nProvider';

interface Props {
  result: CliResult;
  commentStatus: Record<number, CommentStatus>;
  commentJumpable: Record<number, boolean>;
  logs: LogLine[];
  onOpen: (index: number) => void;
  onAction: (index: number, action: 'apply' | 'discard' | 'falsePositive') => void;
}

export function DoneView({ result, commentStatus, commentJumpable, logs, onOpen, onAction }: Props) {
  const [showLogs, setShowLogs] = useState(false);
  const t = useT();
  const s = result.summary;
  return (
    <div class="action-done" style="display:block">
      <div class="done-summary">
        <span class="ds-dot"></span>
        <span>{result.comments.length} {t('view.done.comments')} · {s?.filesReviewed ?? 0} {t('view.done.files')} · {s?.elapsed ?? ''}</span>
      </div>

      {logs.length > 0 && (
        <div class="logs-disclosure">
          <button class="logs-toggle" onClick={() => setShowLogs(!showLogs)}>
            <span class={`logs-toggle-arrow${showLogs ? ' open' : ''}`}></span>
            {t('view.done.processLog')}
          </button>
          {showLogs && <LogViewer logs={logs} />}
        </div>
      )}

      {result.comments.map((c, i) => (
        <CommentCard key={i} comment={c} index={i}
          canJump={commentJumpable[i] !== false}
          status={commentStatus[i] ?? 'pending'} onOpen={onOpen} onAction={onAction} />
      ))}
    </div>
  );
}
