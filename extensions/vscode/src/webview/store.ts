import { CliResult, CommentStatus, FileChange, GitState, LogLine, OcrConfig, ReviewMode, ReviewState } from '../shared/types';
import { SupportedLocale } from '../shared/i18n';
import { HostToWebview } from '../shared/messages';

export type AppView = 'idle' | 'running' | 'done' | 'empty' | 'cancelled' | 'failed';

export interface AppState {
  view: AppView;
  config: OcrConfig | null;
  gitState: GitState;
  modeFiles: FileChange[];
  filesLoading: boolean;
  logs: LogLine[];
  session: { state: ReviewState; result: CliResult | null; error?: string };
  commentStatus: Record<number, CommentStatus>;
  commentJumpable: Record<number, boolean>;
  reviewMode: ReviewMode;
  locale: SupportedLocale;
}

export const initialState: AppState = {
  view: 'idle',
  config: null,
  gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
  modeFiles: [],
  filesLoading: true,
  logs: [],
  session: { state: 'idle', result: null },
  commentStatus: {},
  commentJumpable: {},
  reviewMode: ReviewMode.Workspace,
  locale: 'en',
};

const STATE_TO_VIEW: Record<ReviewState, AppView> = {
  idle: 'idle', running: 'running', done: 'done',
  empty: 'empty', cancelled: 'cancelled', failed: 'failed',
};

export type LocalAction =
  | { type: 'filesLoading' }
  | { type: 'startReview'; mode: ReviewMode };

export function reducer(state: AppState, msg: HostToWebview | LocalAction): AppState {
  switch (msg.type) {
    case 'filesLoading':
      return { ...state, filesLoading: true };
    case 'startReview':
      return { ...state, reviewMode: msg.mode };
    case 'init':
      return {
        ...state,
        config: msg.config,
        gitState: msg.gitState,
        view: 'idle',
        filesLoading: false,
        locale: msg.locale,
      };
    case 'gitState':
      return { ...state, gitState: msg.gitState, filesLoading: false };
    case 'modeFiles':
      return { ...state, modeFiles: msg.files, filesLoading: false };
    case 'config':
      return { ...state, config: msg.config };
    case 'stateChange': {
      const starting = msg.state === 'running';
      return {
        ...state,
        logs: starting ? [] : state.logs,
        commentStatus: starting ? {} : state.commentStatus,
        commentJumpable: starting ? {} : state.commentJumpable,
        session: { state: msg.state, result: starting ? null : state.session.result, error: msg.error },
        view: STATE_TO_VIEW[msg.state],
      };
    }
    case 'logLine':
      return { ...state, logs: [...state.logs, msg.line] };
    case 'reviewDone': {
      const commentJumpable: Record<number, boolean> = {};
      msg.result.comments.forEach((_, i) => { commentJumpable[i] = true; });
      return {
        ...state,
        session: { ...state.session, result: msg.result },
        commentJumpable,
      };
    }
    case 'commentSync': {
      const commentStatus = { ...state.commentStatus };
      const commentJumpable = { ...state.commentJumpable };
      for (const c of msg.comments) {
        commentStatus[c.index] = c.status;
        if (c.jumpable !== undefined) commentJumpable[c.index] = c.jumpable;
      }
      return { ...state, commentStatus, commentJumpable };
    }
    default:
      return state;
  }
}
