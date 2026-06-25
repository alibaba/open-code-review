import {
  CliResult, CliRunOptions, CommentSyncState, EnvCheckResult, FileChange, GitState, LogLine,
  OcrConfig, ReviewMode, ReviewState,
} from './types';
import { ConfigPanelFocus } from './configUtils';

export type WebviewToHost =
  | { type: 'ready' }
  | { type: 'readyConfigPanel' }
  | { type: 'openConfigPanel'; focus?: ConfigPanelFocus }
  | { type: 'deleteCustomProvider'; name: string }
  | { type: 'activateCustomProvider'; name: string }
  | { type: 'closeConfigPanel' }
  | { type: 'getGitState'; mode: ReviewMode }
  | { type: 'getModeFiles'; mode: ReviewMode; from?: string; to?: string; commit?: string }
  | { type: 'openFileDiff'; path: string; status: FileChange['status']; mode: ReviewMode; from?: string; to?: string; commit?: string }
  | { type: 'startReview'; options: CliRunOptions }
  | { type: 'cancelReview' }
  | { type: 'getConfig' }
  | { type: 'setConfig'; key: string; value: string }
  | { type: 'setConfigBatch'; entries: { key: string; value: string }[] }
  | { type: 'testConnection'; entries: { key: string; value: string }[] }
  | { type: 'checkCli' }
  | { type: 'checkEnvironment' }
  | { type: 'installCli' }
  | { type: 'copyToClipboard'; text: string }
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
  | { type: 'commentSync'; comments: CommentSyncState[] };

export type ConfigPanelHostToWebview =
  | { type: 'configPanelInit'; config: OcrConfig | null; focus?: ConfigPanelFocus | null; env?: EnvCheckResult | null; skipEnvCheck?: boolean }
  | { type: 'configPanelFocus'; focus?: ConfigPanelFocus | null }
  | { type: 'config'; config: OcrConfig | null }
  | { type: 'connectionResult'; ok: boolean; message?: string }
  | { type: 'cliStatus'; installed: boolean }
  | { type: 'environmentResult'; env: EnvCheckResult }
  | { type: 'copyDone' }
  | { type: 'panelError'; message: string }
  | { type: 'installLog'; line: LogLine }
  | { type: 'installDone'; ok: boolean };
