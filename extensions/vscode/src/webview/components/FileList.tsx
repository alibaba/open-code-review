import { FileChange } from '../../shared/types';

const BADGE: Record<FileChange['status'], string> = {
  added: 'A', modified: 'M', deleted: 'D', renamed: 'R', binary: 'B',
};

interface Props { files: FileChange[]; loading?: boolean; onOpenFile?: (file: FileChange) => void; }

export function FileList({ files, loading, onOpenFile }: Props) {
  return (
    <div class="file-list">
      <div class="files-label">待审查文件 {loading ? '' : `(${files.length})`}</div>
      {loading ? (
        <div class="file-loading">
          {[68, 52, 60].map((w, i) => (
            <div class="skeleton-row" key={i}>
              <div class="skeleton-bar" style={{ width: `${w}%` }} />
            </div>
          ))}
        </div>
      ) : files.length === 0 ? (
        <div class="file-empty">无变更文件</div>
      ) : (
        <div class="file-scroll">
          {files.map((f) => (
            <div class="file-row" key={f.path} title={onOpenFile ? '点击查看 diff' : undefined}
              onClick={onOpenFile ? () => onOpenFile(f) : undefined}>
              <span class="file-name">{f.path}</span>
              <span class={`file-badge ${f.status}`}>{BADGE[f.status]}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
