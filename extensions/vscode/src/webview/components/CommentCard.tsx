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
  return (
    <div class={`comment-card${status !== 'pending' ? ' dismissed' : ''}`}>
      <div class="comment-header">
        <span class="comment-file">{comment.path}</span>
        <span class="comment-line">L{comment.startLine}</span>
      </div>
      <div class="comment-body">{comment.content}</div>
      <div class="comment-actions">
        {canJump && <button onClick={() => onOpen(index)}>查看</button>}
        <button onClick={() => onAction(index, 'discard')}>忽略</button>
      </div>
    </div>
  );
}
