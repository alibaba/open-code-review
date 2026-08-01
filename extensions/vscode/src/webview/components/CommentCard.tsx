import { useT } from '../I18nProvider';
import { ReviewComment, CommentStatus } from '../../shared/types';

interface Props {
  comment: ReviewComment;
  index: number;
  status: CommentStatus;
  canJump: boolean;
  onOpen: (index: number) => void;
  onAction: (index: number, action: 'apply' | 'discard' | 'falsePositive') => void;
}

export function CommentCard({ comment, index, status, canJump, onOpen, onAction }: Props) {
  const t = useT();
  const open = () => onOpen(index);
  return (
    <div class={`comment-card${status !== 'pending' ? ' dismissed' : ''}`}>
      <div
        class={`comment-header${canJump ? ' jumpable' : ''}`}
        onClick={canJump ? open : undefined}
        onKeyDown={canJump ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); open(); } } : undefined}
        role={canJump ? 'button' : undefined}
        tabIndex={canJump ? 0 : undefined}
        title={canJump ? t('cmp.comment.view') : undefined}
      >
        <span class="comment-file">{comment.path}</span>
        <span class="comment-line">L{comment.startLine}</span>
      </div>
      <div class="comment-body">{comment.content}</div>
      <div class="comment-actions">
        {canJump && <button type="button" onClick={open}>{t('cmp.comment.view')}</button>}
        <button type="button" onClick={() => onAction(index, 'discard')}>{t('cmp.comment.discard')}</button>
      </div>
    </div>
  );
}
