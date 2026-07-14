import { resolveLocale, toHtmlLang } from '../../shared/i18n';
import * as vscode from 'vscode';
import { ConfigPanelFocus } from '../../shared/configUtils';
import { HostToWebview, WebviewToHost } from '../../shared/messages';
import { FileChange, ReviewMode } from '../../shared/types';
import { CliService } from '../services/CliService';
import { ConfigService } from '../services/ConfigService';
import { GitService } from '../services/GitService';
import { ReviewSession } from '../services/ReviewSession';
import { CommentProvider } from './CommentProvider';

export class SidebarProvider implements vscode.WebviewViewProvider {
  private view?: vscode.WebviewView;
  private session?: ReviewSession;
  private openConfigPanel?: (focus?: ConfigPanelFocus) => void;
  private gitWatchDisposable?: vscode.Disposable;

  constructor(
    private extensionUri: vscode.Uri,
    private cli: CliService,
    private config: ConfigService,
    private git: GitService,
    private comments: CommentProvider,
  ) {
    this.comments.onSync((states) => this.post({ type: 'commentSync', comments: states }));
  }

  bindConfigPanel(open: (focus?: ConfigPanelFocus) => void): void {
    this.openConfigPanel = open;
  }

  pushConfig(config: ReturnType<ConfigService['read']>): void {
    this.post({ type: 'config', config });
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = { enableScripts: true, localResourceRoots: [this.extensionUri] };
    view.webview.html = this.html(view.webview);
    view.webview.onDidReceiveMessage((msg: WebviewToHost) => this.handle(msg));

    this.gitWatchDisposable?.dispose();
    this.gitWatchDisposable = this.git.watchWorkspaceChanges((gitState) => {
      this.post({ type: 'gitState', gitState });
    });
    view.onDidDispose(() => {
      this.gitWatchDisposable?.dispose();
      this.gitWatchDisposable = undefined;
      this.view = undefined;
    });
  }

  private post(msg: HostToWebview): void {
    this.view?.webview.postMessage(msg);
  }

  private async handle(msg: WebviewToHost): Promise<void> {
    const cwd = vscode.workspace.workspaceFolders?.[0].uri.fsPath ?? process.cwd();
    switch (msg.type) {
      case 'ready': {
        const config = this.config.read();
        const gitState = await this.git.getState(ReviewMode.Workspace);
        const locale = resolveLocale(vscode.env.language);
        this.post({ type: 'init', config, gitState, locale });
        break;
      }
      case 'getGitState': {
        this.post({ type: 'gitState', gitState: await this.git.getState(msg.mode) });
        break;
      }
      case 'getModeFiles': {
        let files: FileChange[] = [];
        if (msg.mode === ReviewMode.Branch && msg.from && msg.to) {
          files = await this.git.getBranchDiff(msg.from, msg.to);
        } else if (msg.mode === ReviewMode.Commit && msg.commit) {
          files = await this.git.getCommitFiles(msg.commit);
        }
        this.post({ type: 'modeFiles', mode: msg.mode, files });
        break;
      }
      case 'openFileDiff':
        await this.git.openDiff({
          path: msg.path, status: msg.status, mode: msg.mode,
          from: msg.from, to: msg.to, commit: msg.commit,
        });
        break;
      case 'startReview': {
        this.session = new ReviewSession(this.cli, cwd);
        await this.session.run(msg.options, {
          onState: (state, error) => this.post({ type: 'stateChange', state, error }),
          onLog: (line) => this.post({ type: 'logLine', line }),
          onDone: (result) => {
            void (async () => {
              if (result.comments.length) {
                await this.comments.show(result.comments, {
                  mode: msg.options.mode,
                  from: msg.options.from,
                  to: msg.options.to,
                  commit: msg.options.commit,
                });
              }
              this.post({ type: 'reviewDone', result });
            })();
          },
        });
        break;
      }
      case 'cancelReview':
        this.session?.cancel({ onState: (state) => this.post({ type: 'stateChange', state }) });
        break;
      case 'openConfigPanel':
        this.openConfigPanel?.(msg.focus);
        break;
      case 'getConfig':
        this.post({ type: 'config', config: this.config.read() });
        break;
      case 'jumpToComment':
        await this.comments.jumpTo(msg.index);
        break;
      case 'commentAction':
        if (msg.action === 'apply') await this.comments.apply(msg.index);
        else if (msg.action === 'discard') this.comments.discard(msg.index);
        else this.comments.falsePositive(msg.index);
        break;
    }
  }

  private html(webview: vscode.Webview): string {
    const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(this.extensionUri, 'out', 'webview.js'));
    const nonce = String(Date.now());
    const resolved = resolveLocale(vscode.env.language);
    const lang = toHtmlLang(resolved);
    return `<!DOCTYPE html>
<html lang="${lang}"><head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
</head><body><div id="root"></div>
<script nonce="${nonce}" src="${scriptUri}"></script>
</body></html>`;
  }
}
