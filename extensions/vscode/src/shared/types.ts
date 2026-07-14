export enum ReviewMode {
  Workspace = 'workspace',
  Branch = 'branch',
  Commit = 'commit',
}

export type ReviewState =
  | 'idle' | 'running' | 'done' | 'empty' | 'cancelled' | 'failed';

export type CommentStatus = 'pending' | 'applied' | 'discarded' | 'falsePositive';

export interface ReviewComment {
  path: string;
  content: string;
  suggestionCode?: string;
  existingCode?: string;
  startLine: number;
  endLine: number;
  thinking?: string;
}

export interface ReviewSummary {
  filesReviewed: number;
  comments: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  elapsed: string;
}

export interface AgentWarning {
  type: string;
  file: string;
  message: string;
}

export interface CliResult {
  status: 'success' | 'completed_with_errors' | 'completed_with_warnings' | 'skipped';
  comments: ReviewComment[];
  warnings: AgentWarning[];
  summary?: ReviewSummary;
  message?: string;
}

export interface ProviderEntry {
  apiKey?: string;
  url?: string;
  protocol?: string;
  model?: string;
  models?: string[];
  authHeader?: string;
}

export interface OcrConfig {
  provider?: string;
  model?: string;
  providers: Record<string, ProviderEntry>;
  customProviders: Record<string, ProviderEntry>;
  llm: {
    url: string;
    authToken: string;
    model: string;
    useAnthropic: boolean;
    authHeader?: string;
  };
  language: string;
}

export interface CommitInfo {
  sha: string;
  message: string;
  relativeTime: string;
}

export interface FileChange {
  path: string;
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'binary';
}

export interface GitState {
  branches: string[];
  currentBranch: string;
  recentCommits: CommitInfo[];
  workspaceFiles: FileChange[];
}

export interface LogLine {
  text: string;
  level: 'info' | 'warn' | 'error';
}

export interface EnvToolStatus {
  ok: boolean;
  version?: string;
}

export interface EnvCheckResult {
  node: EnvToolStatus;
  npm: EnvToolStatus;
  ocr: EnvToolStatus;
}

export interface CliRunOptions {
  mode: ReviewMode;
  from?: string;
  to?: string;
  commit?: string;
  customPrompt?: string;
  concurrency?: number;
}

/** 审查完成后的评论挂载上下文（与 CliRunOptions 字段一致）。 */
export type ReviewContext = Pick<CliRunOptions, 'mode' | 'from' | 'to' | 'commit'>;

export interface CommentSyncState {
  index: number;
  status: CommentStatus;
  jumpable?: boolean;
}
