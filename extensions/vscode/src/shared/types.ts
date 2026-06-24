export type ReviewMode = 'workspace' | 'branch' | 'commit';

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

export interface OcrConfig {
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

export interface CliRunOptions {
  mode: ReviewMode;
  from?: string;
  to?: string;
  commit?: string;
  customPrompt?: string;
  concurrency?: number;
}

export interface CommentSyncState {
  index: number;
  status: CommentStatus;
}
