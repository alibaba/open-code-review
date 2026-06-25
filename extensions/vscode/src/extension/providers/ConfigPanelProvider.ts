import * as vscode from 'vscode';
import { ConfigPanelFocus, isConfigReady } from '../../shared/configUtils';
import { ConfigPanelHostToWebview, WebviewToHost } from '../../shared/messages';
import { OcrConfig } from '../../shared/types';
import { CliService } from '../services/CliService';
import { ConfigService } from '../services/ConfigService';

const PANEL_VIEW_TYPE = 'ocr.configPanel';

export class ConfigPanelProvider implements vscode.Disposable {
  private panel?: vscode.WebviewPanel;
  private pendingFocus?: ConfigPanelFocus;
  private messageDisposable?: vscode.Disposable;

  constructor(
    private extensionUri: vscode.Uri,
    private cli: CliService,
    private config: ConfigService,
    private onConfigChanged: (config: OcrConfig | null) => void,
  ) {}

  open(focus?: ConfigPanelFocus): void {
    this.pendingFocus = focus;
    if (this.panel) {
      this.post({ type: 'configPanelFocus', focus: focus ?? null });
      this.panel.reveal(vscode.ViewColumn.One);
      this.pendingFocus = undefined;
      return;
    }

    this.panel = vscode.window.createWebviewPanel(
      PANEL_VIEW_TYPE,
      '模型配置',
      vscode.ViewColumn.One,
      { enableScripts: true, retainContextWhenHidden: true, localResourceRoots: [this.extensionUri] },
    );
    this.panel.iconPath = new vscode.ThemeIcon('sparkle');

    this.panel.webview.html = this.html(this.panel.webview);
    this.messageDisposable = this.panel.webview.onDidReceiveMessage((msg: WebviewToHost) => {
      void this.handle(msg);
    });
    this.panel.onDidDispose(() => {
      this.messageDisposable?.dispose();
      this.messageDisposable = undefined;
      this.panel = undefined;
    });
  }

  dispose(): void {
    this.messageDisposable?.dispose();
    this.messageDisposable = undefined;
    this.panel?.dispose();
    this.panel = undefined;
  }

  private post(msg: ConfigPanelHostToWebview): void {
    this.panel?.webview.postMessage(msg);
  }

  private notifyConfig(config: OcrConfig | null): void {
    this.post({ type: 'config', config });
    this.onConfigChanged(config);
  }

  private async handle(msg: WebviewToHost): Promise<void> {
    try {
      await this.handleMessage(msg);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.post({ type: 'panelError', message });
    }
  }

  private async handleMessage(msg: WebviewToHost): Promise<void> {
    switch (msg.type) {
      case 'readyConfigPanel': {
        const focus = this.pendingFocus;
        this.pendingFocus = undefined;
        const config = this.config.read();
        const cached = this.cli.getCachedEnvironment();
        const skipEnvCheck = focus?.step === 2 || isConfigReady(config);
        this.post({
          type: 'configPanelInit',
          config,
          focus: focus ?? null,
          env: cached,
          skipEnvCheck,
        });
        break;
      }
      case 'closeConfigPanel':
        this.panel?.dispose();
        break;
      case 'setConfig':
        await this.config.set(msg.key, msg.value);
        this.notifyConfig(this.config.read());
        break;
      case 'setConfigBatch':
        await this.config.setMany(msg.entries);
        this.notifyConfig(this.config.read());
        break;
      case 'testConnection': {
        const r = await this.config.testWithEntries(msg.entries);
        this.post({ type: 'connectionResult', ok: r.ok, message: r.message });
        break;
      }
      case 'deleteCustomProvider': {
        const confirmed = await vscode.window.showWarningMessage(
          `确定删除自定义 Provider「${msg.name}」？`,
          { modal: true },
          '删除',
        );
        if (confirmed !== '删除') break;
        this.notifyConfig(this.config.deleteCustomProvider(msg.name));
        break;
      }
      case 'activateCustomProvider':
        await this.config.set('provider', msg.name);
        this.notifyConfig(this.config.read());
        break;
      case 'checkCli':
      case 'checkEnvironment': {
        const env = await this.cli.checkEnvironment(true);
        this.post({ type: 'environmentResult', env });
        break;
      }
      case 'copyToClipboard':
        await vscode.env.clipboard.writeText(msg.text);
        this.post({ type: 'copyDone' });
        break;
      case 'installCli': {
        const ok = await this.cli.install((line) => this.post({ type: 'installLog', line }));
        this.post({ type: 'installDone', ok });
        const env = await this.cli.checkEnvironment();
        this.post({ type: 'environmentResult', env });
        break;
      }
      default:
        break;
    }
  }

  private html(webview: vscode.Webview): string {
    const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(this.extensionUri, 'out', 'configPanel.js'));
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
