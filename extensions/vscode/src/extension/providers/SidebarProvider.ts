import * as vscode from 'vscode';
import { HostToWebview, WebviewToHost } from '../../shared/messages';
import { FileChange } from '../../shared/types';
import { CliService } from '../services/CliService';
import { ConfigService } from '../services/ConfigService';
import { GitService } from '../services/GitService';
import { ReviewSession } from '../services/ReviewSession';
import { CommentProvider } from './CommentProvider';

export class SidebarProvider implements vscode.WebviewViewProvider {
  private view?: vscode.WebviewView;
  private session?: ReviewSession;

  constructor(
    private extensionUri: vscode.Uri,
    private cli: CliService,
    private config: ConfigService,
    private git: GitService,
    private comments: CommentProvider,
  ) {
    this.comments.onSync((states) => this.post({ type: 'commentSync', comments: states }));
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = { enableScripts: true, localResourceRoots: [this.extensionUri] };
    view.webview.html = this.html(view.webview);
    view.webview.onDidReceiveMessage((msg: WebviewToHost) => this.handle(msg));
  }

  private post(msg: HostToWebview): void {
    this.view?.webview.postMessage(msg);
  }

  private async handle(msg: WebviewToHost): Promise<void> {
    const cwd = vscode.workspace.workspaceFolders?.[0].uri.fsPath ?? process.cwd();
    switch (msg.type) {
      case 'ready': {
        const config = this.config.read();
        const gitState = await this.git.getState('workspace');
        this.post({ type: 'init', config, gitState });
        break;
      }
      case 'getGitState': {
        this.post({ type: 'gitState', gitState: await this.git.getState(msg.mode) });
        break;
      }
      case 'getModeFiles': {
        let files: FileChange[] = [];
        if (msg.mode === 'branch' && msg.from && msg.to) {
          files = await this.git.getBranchDiff(msg.from, msg.to);
        } else if (msg.mode === 'commit' && msg.commit) {
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
        // 仅工作区模式在编辑器内放置评论 thread；分支/提交模式代码不在工作区，会错位。
        const inEditor = msg.options.mode === 'workspace';
        await this.session.run(msg.options, {
          onState: (state, error) => this.post({ type: 'stateChange', state, error }),
          onLog: (line) => this.post({ type: 'logLine', line }),
          onDone: (result) => {
            this.post({ type: 'reviewDone', result });
            if (result.comments.length) this.comments.show(result.comments, inEditor);
          },
        });
        break;
      }
      case 'cancelReview':
        this.session?.cancel({ onState: (state) => this.post({ type: 'stateChange', state }) });
        break;
      case 'getConfig':
        this.post({ type: 'config', config: this.config.read() });
        break;
      case 'setConfig':
        await this.config.set(msg.key, msg.value);
        this.post({ type: 'config', config: this.config.read() });
        break;
      case 'testConnection': {
        const r = await this.cli.testConnection();
        this.post({ type: 'connectionResult', ok: r.ok, message: r.message });
        break;
      }
      case 'checkCli': {
        this.post({ type: 'cliStatus', installed: await this.cli.isAvailable() });
        break;
      }
      case 'installCli': {
        const ok = await this.cli.install((line) => this.post({ type: 'installLog', line }));
        this.post({ type: 'installDone', ok });
        this.post({ type: 'cliStatus', installed: await this.cli.isAvailable() });
        break;
      }
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
    return `<!DOCTYPE html>
<html lang="zh-CN"><head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
</head><body><div id="root"></div>
<script nonce="${nonce}" src="${scriptUri}"></script>
</body></html>`;
  }
}
