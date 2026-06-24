import { useState, useEffect } from 'preact/hooks';
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
  running?: boolean;
}

export function IdleView({ gitState, modeFiles, filesLoading, configured, onModeChange, onRequestModeFiles, onOpenFile, onStart, running }: Props) {
  const [mode, setMode] = useState<ReviewMode>('workspace');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [commit, setCommit] = useState('');
  const [prompt, setPrompt] = useState('');

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

  return (
    <div class="setup">
      <div class="mode-tabs">
        {(['workspace', 'branch', 'commit'] as ReviewMode[]).map((m) => (
          <button key={m} class={`mode-tab${mode === m ? ' active' : ''}`} onClick={() => switchMode(m)}>
            {m === 'workspace' ? '工作区' : m === 'branch' ? '分支对比' : '单次提交'}
          </button>
        ))}
      </div>

      {mode === 'branch' && (
        <div class="mode-params active">
          <div class="mode-param-label">基础引用</div>
          <Select value={from} placeholder="选择分支" onChange={setFrom}
            options={gitState.branches.map((b) => ({ value: b, label: b }))} />
          <div class="mode-param-label">目标引用</div>
          <Select value={to} placeholder="选择分支" onChange={setTo}
            options={gitState.branches.map((b) => ({ value: b, label: b }))} />
        </div>
      )}

      {mode === 'commit' && (
        <div class="mode-params active">
          <div class="files-label">提交历史</div>
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

      <textarea class="mode-param-input" rows={3} placeholder="自定义审查提示词（可选）"
        value={prompt} onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)} />

      {loading ? (
        <div class="primary-btn skeleton-btn"><div class="skeleton-bar" style={{ width: '40%' }} /></div>
      ) : (
        <button class="primary-btn" disabled={!canReview}
          onClick={() => onStart({ mode, from, to, commit, customPrompt: prompt })}>
          {!configured ? '请先配置模型'
            : running ? '审查中…'
            : !selectionReady ? (mode === 'branch' ? '请选择对比分支' : '请选择提交')
            : files.length === 0 ? '无可审查文件'
            : '审查所有变更'}
        </button>
      )}
    </div>
  );
}
