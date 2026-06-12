import { useState } from 'preact/hooks';
import { CliResult, CommentStatus, LogLine } from '../../shared/types';
import { CommentCard } from '../components/CommentCard';
import { LogViewer } from '../components/LogViewer';

interface Props {
  result: CliResult;
  commentStatus: Record<number, CommentStatus>;
  logs: LogLine[];
  canJump: boolean;
  onOpen: (index: number) => void;
  onAction: (index: number, action: 'apply' | 'discard' | 'falsePositive') => void;
}

export function DoneView({ result, commentStatus, logs, canJump, onOpen, onAction }: Props) {
  const [showLogs, setShowLogs] = useState(false);
  const s = result.summary;
  return (
    <div class="action-done" style="display:block">
      <div class="done-summary">
        <span class="ds-dot"></span>
        <span>{result.comments.length} 条评论 · {s?.filesReviewed ?? 0} 个文件 · {s?.elapsed ?? ''}</span>
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

      {result.comments.map((c, i) => (
        <CommentCard key={i} comment={c} index={i} canJump={canJump}
          status={commentStatus[i] ?? 'pending'} onOpen={onOpen} onAction={onAction} />
      ))}
    </div>
  );
}
