import { useState, useEffect } from 'preact/hooks';
import { useT } from '../I18nProvider';
import { GitState, ReviewMode, CliRunOptions, FileChange } from '../../shared/types';
import { FileList } from '../components/FileList';
import { Select } from '../components/Select';


interface Props {
  gitState: GitState;
  modeFiles: FileChange[];
  filesLoading: boolean;
  configured: boolean;
  onModeChange: (mode: ReviewMode) => void;
  onRequestModeFiles: (mode: ReviewMode, from?: string, to?: string, commit?: string) => void;
  onOpenFile: (file: FileChange, mode: ReviewMode, from?: string, to?: string, commit?: string) => void;
  onStart: (options: CliRunOptions) => void;
  onOpenConfig: () => void;
  onOpenCustomProviders: () => void;
  running?: boolean;
}

export function IdleView({ gitState, modeFiles, filesLoading, configured, onModeChange, onRequestModeFiles, onOpenFile, onStart, onOpenConfig, onOpenCustomProviders, running }: Props) {
  const [mode, setMode] = useState<ReviewMode>('workspace');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [commit, setCommit] = useState('');
  const [prompt, setPrompt] = useState('');
  const t = useT();

  const getPrimaryLabel = () => {
    if (!configured) return t('view.idle.configFirst');
    if (running) return t('view.idle.reviewing');
    if (!selectionReady) {
      return mode === 'branch' ? t('view.idle.selectBranch') : t('view.idle.selectCommit');
    }
    if (files.length === 0) return t('view.idle.noFiles');
    return t('view.idle.reviewAll');
  };

  const switchMode = (m: ReviewMode) => { setMode(m); onModeChange(m); };

  // 分支两端都选好后,拉取 diff 文件列表
  useEffect(() => {
    if (mode === 'branch' && from && to) onRequestModeFiles('branch', from, to);
  }, [mode, from, to]);

  // 选中某 commit 后,拉取该 commit 文件列表
  useEffect(() => {
    if (mode === 'commit' && commit) onRequestModeFiles('commit', undefined, undefined, commit);
  }, [mode, commit]);

  const files = mode === 'workspace' ? gitState.workspaceFiles : modeFiles;
  // 仅在「确实发起了请求」时显示 loading:分支需选满两端,提交需选中 commit。
  const willRequest = mode === 'workspace' || (mode === 'branch' && !!from && !!to) || (mode === 'commit' && !!commit);
  const loading = filesLoading && willRequest;
  // 可发起审查的前置条件:按 tab 校验选择已就绪,且有待审查文件、不在加载/审查中。
  const selectionReady =
    mode === 'workspace' || (mode === 'branch' && !!from && !!to) || (mode === 'commit' && !!commit);
  const canReview = configured && !running && !loading && selectionReady && files.length > 0;
  const primaryDisabled = configured ? !canReview : running || loading;

  const handlePrimary = () => {
    if (!configured) {
      onOpenConfig();
      return;
    }
    onStart({ mode, from, to, commit, customPrompt: prompt });
  };

  return (
    <div class="setup">
      <div class="mode-tabs">
        {(['workspace', 'branch', 'commit'] as ReviewMode[]).map((m) => (
          <button key={m} class={`mode-tab${mode === m ? ' active' : ''}`} onClick={() => switchMode(m)}>
            {m === 'workspace' ? t('view.idle.workspace') : m === 'branch' ? t('view.idle.branch') : t('view.idle.commit')}
          </button>
        ))}
      </div>

      {mode === 'branch' && (
        <div class="mode-params active">
          <div class="mode-param-label">{t('view.idle.baseRef')}</div>
          <Select value={from} placeholder={t('view.idle.chooseBranch')} onChange={setFrom}
            options={gitState.branches.map((b) => ({ value: b, label: b }))} />
          <div class="mode-param-label">{t('view.idle.targetRef')}</div>
          <Select value={to} placeholder={t('view.idle.chooseBranch')} onChange={setTo}
            options={gitState.branches.map((b) => ({ value: b, label: b }))} />
        </div>
      )}

      {mode === 'commit' && (
        <div class="mode-params active">
          <div class="files-label">{t('view.idle.commitHistory')}</div>
          <div class="commit-list">
            {gitState.recentCommits.map((c) => (
              <label key={c.sha} class={`commit-row${commit === c.sha ? ' active' : ''}`} onClick={() => setCommit(c.sha)}>
                <input type="radio" name="commit" class="commit-radio" checked={commit === c.sha} />
                <div class="commit-info">
                  <div class="commit-msg">{c.message}</div>
                  <div class="commit-meta"><span class="commit-sha">{c.sha}</span> · {c.relativeTime}</div>
                </div>
              </label>
            ))}
          </div>
        </div>
      )}

      <FileList files={files} loading={loading}
        onOpenFile={(f) => onOpenFile(f, mode, from, to, commit)} />

      <textarea class="mode-param-input" rows={3} placeholder={t('view.idle.customPrompt')}
        value={prompt} onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)} />

      {configured && (
        <div class="setup-secondary">
          <button type="button" class="link-btn" onClick={onOpenConfig}>{t('view.idle.modelConfig')}</button>
        </div>
      )}

      {loading ? (
        <div class="primary-btn skeleton-btn"><div class="skeleton-bar" style={{ width: '40%' }} /></div>
      ) : (
        <button class={`primary-btn${!configured ? ' configure' : ''}`} disabled={primaryDisabled}
          onClick={handlePrimary}>
          {getPrimaryLabel()}
        </button>
      )}
    </div>
  );
}
