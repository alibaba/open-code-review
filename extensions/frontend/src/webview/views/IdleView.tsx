// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
  const [mode, setMode] = useState<ReviewMode>(ReviewMode.Workspace);
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [commit, setCommit] = useState('');
  const [prompt, setPrompt] = useState('');
  const t = useT();

  const getPrimaryLabel = () => {
    if (!configured) return t('view.idle.configFirst');
    if (running) return t('view.idle.reviewing');
    if (!selectionReady) {
      return mode === ReviewMode.Branch ? t('view.idle.selectBranch') : t('view.idle.selectCommit');
    }
    if (files.length === 0) return t('view.idle.noFiles');
    return t('view.idle.reviewAll');
  };

  const switchMode = (m: ReviewMode) => { setMode(m); onModeChange(m); };

  // Fetch the diff file list once both branch endpoints are selected
  useEffect(() => {
    if (mode === ReviewMode.Branch && from && to) onRequestModeFiles(ReviewMode.Branch, from, to);
  }, [mode, from, to]);

  // Fetch the file list of a commit once it is selected
  useEffect(() => {
    if (mode === ReviewMode.Commit && commit) onRequestModeFiles(ReviewMode.Commit, undefined, undefined, commit);
  }, [mode, commit]);

  const files = mode === ReviewMode.Workspace ? gitState.workspaceFiles : modeFiles;
  // Show loading only when a request was actually fired: branch needs both endpoints, commit needs a selected commit.
  const willRequest = mode === ReviewMode.Workspace || (mode === ReviewMode.Branch && !!from && !!to) || (mode === ReviewMode.Commit && !!commit);
  const loading = filesLoading && willRequest;
  // Preconditions for starting a review: the per-tab selection is complete, files are pending review, and nothing is loading/reviewing.
  const selectionReady =
    mode === ReviewMode.Workspace || (mode === ReviewMode.Branch && !!from && !!to) || (mode === ReviewMode.Commit && !!commit);
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
        {([ReviewMode.Workspace, ReviewMode.Branch, ReviewMode.Commit]).map((m) => (
          <button key={m} class={`mode-tab${mode === m ? ' active' : ''}`} onClick={() => switchMode(m)}>
            {m === ReviewMode.Workspace ? t('view.idle.workspace') : m === ReviewMode.Branch ? t('view.idle.branch') : t('view.idle.commit')}
          </button>
        ))}
      </div>

      {mode === ReviewMode.Branch && (
        <div class="mode-params active">
          <div class="mode-param-label">{t('view.idle.baseRef')}</div>
          <Select value={from} placeholder={t('view.idle.chooseBranch')} onChange={setFrom}
            options={gitState.branches.map((b) => ({ value: b, label: b }))} />
          <div class="mode-param-label">{t('view.idle.targetRef')}</div>
          <Select value={to} placeholder={t('view.idle.chooseBranch')} onChange={setTo}
            options={gitState.branches.map((b) => ({ value: b, label: b }))} />
        </div>
      )}

      {mode === ReviewMode.Commit && (
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
