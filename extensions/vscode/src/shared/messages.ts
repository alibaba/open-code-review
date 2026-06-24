import {
  CliResult, CliRunOptions, CommentSyncState, FileChange, GitState, LogLine,
  OcrConfig, ReviewMode, ReviewState,
} from './types';

export type WebviewToHost =
  | { type: 'ready' }
  | { type: 'getGitState'; mode: ReviewMode }
  | { type: 'getModeFiles'; mode: ReviewMode; from?: string; to?: string; commit?: string }
  | { type: 'openFileDiff'; path: string; status: FileChange['status']; mode: ReviewMode; from?: string; to?: string; commit?: string }
  | { type: 'startReview'; options: CliRunOptions }
  | { type: 'cancelReview' }
  | { type: 'getConfig' }
  | { type: 'setConfig'; key: string; value: string }
  | { type: 'testConnection' }
  | { type: 'checkCli' }
  | { type: 'installCli' }
  | { type: 'jumpToComment'; index: number }
  | { type: 'commentAction'; index: number; action: 'apply' | 'discard' | 'falsePositive' };

export type HostToWebview =
  | { type: 'init'; config: OcrConfig | null; gitState: GitState }
  | { type: 'gitState'; gitState: GitState }
  | { type: 'modeFiles'; mode: ReviewMode; files: FileChange[] }
  | { type: 'logLine'; line: LogLine }
  | { type: 'stateChange'; state: ReviewState; error?: string }
  | { type: 'reviewDone'; result: CliResult }
  | { type: 'config'; config: OcrConfig | null }
  | { type: 'connectionResult'; ok: boolean; message?: string }
  | { type: 'cliStatus'; installed: boolean }
  | { type: 'installLog'; line: LogLine }
  | { type: 'installDone'; ok: boolean }
  | { type: 'commentSync'; comments: CommentSyncState[] };
