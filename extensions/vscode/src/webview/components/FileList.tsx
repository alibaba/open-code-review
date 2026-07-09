import { useT } from '../I18nProvider';
import { FileChange } from '../../shared/types';

const BADGE: Record<FileChange['status'], string> = {
  added: 'A', modified: 'M', deleted: 'D', renamed: 'R', binary: 'B',
};

interface Props { files: FileChange[]; loading?: boolean; onOpenFile?: (file: FileChange) => void; }

export function FileList({ files, loading, onOpenFile }: Props) {
  const t = useT();
  return (
    <div class="file-list">
      <div class="files-label">{t('cmp.fileList.pending')} {loading ? '' : `(${files.length})`}</div>
      {loading ? (
        <div class="file-loading">
          {[68, 52, 60].map((w, i) => (
            <div class="skeleton-row" key={i}>
              <div class="skeleton-bar" style={{ width: `${w}%` }} />
            </div>
          ))}
        </div>
      ) : files.length === 0 ? (
        <div class="file-empty">{t('cmp.fileList.noChanges')}</div>
      ) : (
        <div class="file-scroll">
          {files.map((f) => (
            <div class="file-row" key={f.path} title={onOpenFile ? t('cmp.fileList.viewDiff') : undefined}
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
