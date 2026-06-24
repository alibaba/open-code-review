import { CliResult, CommentStatus, FileChange, GitState, LogLine, OcrConfig, ReviewMode, ReviewState } from '../shared/types';
import { HostToWebview } from '../shared/messages';

export type AppView = 'idle' | 'running' | 'done' | 'empty' | 'cancelled' | 'failed';

export type CliStatus = 'unknown' | 'checking' | 'installed' | 'missing';
export type ConnTest = { status: 'idle' | 'testing' | 'ok' | 'fail'; message?: string };

export interface AppState {
  view: AppView;
  configOpen: boolean;
  config: OcrConfig | null;
  gitState: GitState;
  modeFiles: FileChange[];
  filesLoading: boolean;
  logs: LogLine[];
  session: { state: ReviewState; result: CliResult | null; error?: string };
  commentStatus: Record<number, CommentStatus>;
  cliStatus: CliStatus;
  installing: boolean;
  installLogs: LogLine[];
  connTest: ConnTest;
  reviewMode: ReviewMode;
}

export const initialState: AppState = {
  view: 'idle',
  configOpen: false,
  config: null,
  gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
  modeFiles: [],
  filesLoading: true,
  logs: [],
  session: { state: 'idle', result: null },
  commentStatus: {},
  cliStatus: 'unknown',
  installing: false,
  installLogs: [],
  connTest: { status: 'idle' },
  reviewMode: 'workspace',
};

const STATE_TO_VIEW: Record<ReviewState, AppView> = {
  idle: 'idle', running: 'running', done: 'done',
  empty: 'empty', cancelled: 'cancelled', failed: 'failed',
};

export type LocalAction =
  | { type: 'openConfig' }
  | { type: 'closeConfig' }
  | { type: 'filesLoading' }
  | { type: 'checkingCli' }
  | { type: 'installingCli' }
  | { type: 'testingConn' }
  | { type: 'startReview'; mode: ReviewMode };

export function reducer(state: AppState, msg: HostToWebview | LocalAction): AppState {
  switch (msg.type) {
    case 'openConfig':
      return { ...state, configOpen: true, connTest: { status: 'idle' } };
    case 'closeConfig':
      return { ...state, configOpen: false, installLogs: [], connTest: { status: 'idle' } };
    case 'filesLoading':
      return { ...state, filesLoading: true };
    case 'checkingCli':
      return { ...state, cliStatus: 'checking' };
    case 'installingCli':
      return { ...state, installing: true, installLogs: [] };
    case 'testingConn':
      return { ...state, connTest: { status: 'testing' } };
    case 'startReview':
      return { ...state, reviewMode: msg.mode };
    case 'cliStatus':
      return { ...state, cliStatus: msg.installed ? 'installed' : 'missing' };
    case 'installLog':
      return { ...state, installLogs: [...state.installLogs, msg.line] };
    case 'installDone':
      return { ...state, installing: false };
    case 'init':
      return {
        ...state,
        config: msg.config,
        gitState: msg.gitState,
        view: 'idle',
        filesLoading: false,
      };
    case 'gitState':
      return { ...state, gitState: msg.gitState, filesLoading: false };
    case 'modeFiles':
      return { ...state, modeFiles: msg.files, filesLoading: false };
    case 'config':
      // 保存配置后：更新 config，若已配置则关闭浮层
      return { ...state, config: msg.config, configOpen: msg.config ? false : state.configOpen };
    case 'stateChange': {
      const starting = msg.state === 'running';
      return {
        ...state,
        logs: starting ? [] : state.logs,
        commentStatus: starting ? {} : state.commentStatus,
        session: { state: msg.state, result: starting ? null : state.session.result, error: msg.error },
        view: STATE_TO_VIEW[msg.state],
      };
    }
    case 'logLine':
      return { ...state, logs: [...state.logs, msg.line] };
    case 'reviewDone':
      return { ...state, session: { ...state.session, result: msg.result } };
    case 'commentSync': {
      const commentStatus = { ...state.commentStatus };
      for (const c of msg.comments) commentStatus[c.index] = c.status;
      return { ...state, commentStatus };
    }
    case 'connectionResult':
      return { ...state, connTest: { status: msg.ok ? 'ok' : 'fail', message: msg.message } };
    default:
      return state;
  }
}
