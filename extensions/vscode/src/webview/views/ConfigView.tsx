import { useState } from 'preact/hooks';
import { OcrConfig } from '../../shared/types';
import { CliStatus, ConnTest } from '../store';
import { LogLine } from '../../shared/types';
import { LogViewer } from '../components/LogViewer';
import { Select } from '../components/Select';

interface Props {
  config: OcrConfig | null;
  cliStatus: CliStatus;
  installing: boolean;
  installLogs: LogLine[];
  connTest: ConnTest;
  onInstall: () => void;
  onCheckCli: () => void;
  onTest: () => void;
  onSave: (entries: { key: string; value: string }[]) => void;
  onClose: () => void;
}

export function ConfigView({
  config, cliStatus, installing, installLogs, connTest,
  onInstall, onCheckCli, onTest, onSave, onClose,
}: Props) {
  const [step, setStep] = useState<1 | 2>(1);

  const [url, setUrl] = useState(config?.llm.url ?? '');
  const [token, setToken] = useState(config?.llm.authToken ?? '');
  const [model, setModel] = useState(config?.llm.model ?? '');
  const [useAnthropic, setUseAnthropic] = useState(config?.llm.useAnthropic ?? false);
  const [authHeader, setAuthHeader] = useState(config?.llm.authHeader ?? '');

  const canSave = url.trim() !== '' && model.trim() !== '';

  const save = () => {
    if (!canSave) return;
    const entries = [
      { key: 'llm.url', value: url.trim() },
      { key: 'llm.auth_token', value: token.trim() },
      { key: 'llm.model', value: model.trim() },
      { key: 'llm.use_anthropic', value: String(useAnthropic) },
    ];
    if (authHeader) entries.push({ key: 'llm.auth_header', value: authHeader });
    onSave(entries);
  };

  return (
    <div class="modal-backdrop" onClick={onClose}>
      <div class="modal-panel" onClick={(e) => e.stopPropagation()}>
        <div class="config-form-header">
          <span class="config-form-title">模型配置</span>
          <button class="config-list-close" onClick={onClose}>×</button>
        </div>

        <div class="wizard-steps">
          <span class={`wizard-step${step === 1 ? ' active' : ''}${cliStatus === 'installed' ? ' done' : ''}`}>1 环境检测</span>
          <span class="wizard-step-line"></span>
          <span class={`wizard-step${step === 2 ? ' active' : ''}`}>2 模型配置</span>
        </div>

        {step === 1 ? (
          <Step1
            cliStatus={cliStatus} installing={installing} installLogs={installLogs}
            onInstall={onInstall} onCheckCli={onCheckCli} onNext={() => setStep(2)}
          />
        ) : (
          <Step2
            url={url} token={token} model={model} useAnthropic={useAnthropic} authHeader={authHeader}
            setUrl={setUrl} setToken={setToken} setModel={setModel}
            setUseAnthropic={setUseAnthropic} setAuthHeader={setAuthHeader}
            connTest={connTest} canSave={canSave}
            onBack={() => setStep(1)} onTest={onTest} onSave={save}
          />
        )}
      </div>
    </div>
  );
}

function Step1({ cliStatus, installing, installLogs, onInstall, onCheckCli, onNext }: {
  cliStatus: CliStatus; installing: boolean; installLogs: LogLine[];
  onInstall: () => void; onCheckCli: () => void; onNext: () => void;
}) {
  if (installing) {
    return (
      <div class="wizard-body">
        <div class="cli-status checking">正在安装 ocr CLI…</div>
        <LogViewer logs={installLogs} />
      </div>
    );
  }

  if (cliStatus === 'checking' || cliStatus === 'unknown') {
    return <div class="wizard-body"><div class="cli-status checking">正在检测 ocr 命令…</div></div>;
  }

  if (cliStatus === 'missing') {
    return (
      <div class="wizard-body">
        <div class="cli-status missing">未检测到 ocr 命令。需要全局安装后才能进行代码审查。</div>
        <div class="cli-hint">将执行：<code>npm install -g @alibaba-group/open-code-review</code></div>
        {installLogs.length > 0 && <LogViewer logs={installLogs} />}
        <div class="form-actions">
          <button class="btn-cancel" onClick={onCheckCli}>重新检测</button>
          <button class="btn-save" onClick={onInstall}>一键安装</button>
        </div>
      </div>
    );
  }

  // installed
  return (
    <div class="wizard-body">
      <div class="cli-status ok">✓ ocr 命令已安装</div>
      <div class="form-actions">
        <button class="btn-save" onClick={onNext}>下一步</button>
      </div>
    </div>
  );
}

function Step2({
  url, token, model, useAnthropic, authHeader,
  setUrl, setToken, setModel, setUseAnthropic, setAuthHeader,
  connTest, canSave, onBack, onTest, onSave,
}: {
  url: string; token: string; model: string; useAnthropic: boolean; authHeader: string;
  setUrl: (v: string) => void; setToken: (v: string) => void; setModel: (v: string) => void;
  setUseAnthropic: (v: boolean) => void; setAuthHeader: (v: string) => void;
  connTest: ConnTest; canSave: boolean;
  onBack: () => void; onTest: () => void; onSave: () => void;
}) {
  return (
    <div class="wizard-body">
      <div class="form-group">
        <label class="form-label">接口地址</label>
        <input class="form-input" value={url} onInput={(e) => setUrl((e.target as HTMLInputElement).value)} placeholder="https://api.anthropic.com/v1/messages" />
      </div>
      <div class="form-group">
        <label class="form-label">API 密钥</label>
        <input class="form-input" type="password" value={token} onInput={(e) => setToken((e.target as HTMLInputElement).value)} placeholder="sk-..." />
      </div>
      <div class="form-group">
        <label class="form-label">模型</label>
        <input class="form-input" value={model} onInput={(e) => setModel((e.target as HTMLInputElement).value)} placeholder="claude-opus-4-6" />
      </div>
      <div class="toggle-row">
        <span class="toggle-label">使用 Anthropic 协议</span>
        <button class={`toggle-switch${useAnthropic ? ' on' : ''}`} onClick={() => setUseAnthropic(!useAnthropic)}>
          <span class="toggle-knob"></span>
        </button>
      </div>

      <details class="advanced-section">
        <summary>高级选项</summary>
        <div class="adv-content">
          <div class="form-group">
            <label class="form-label">Auth Header <span class="optional">可选</span></label>
            <Select value={authHeader} placeholder="默认 (authorization)" onChange={setAuthHeader}
              options={[
                { value: '', label: '默认 (authorization)' },
                { value: 'x-api-key', label: 'x-api-key' },
                { value: 'authorization', label: 'authorization' },
              ]} />
            <div class="cli-hint">标准 sk-ant-* 密钥需选 x-api-key</div>
          </div>
        </div>
      </details>

      {connTest.status !== 'idle' && (
        <div class={`conn-result ${connTest.status}`}>
          {connTest.status === 'testing' && '正在测试连接…'}
          {connTest.status === 'ok' && '✓ 连接成功'}
          {connTest.status === 'fail' && `✗ 连接失败${connTest.message ? '：' + connTest.message : ''}`}
        </div>
      )}

      <div class="form-actions">
        <button class="btn-cancel" onClick={onBack}>上一步</button>
        <button class="btn-cancel" disabled={connTest.status === 'testing'} onClick={onTest}>测试连接</button>
        <button class="btn-save" disabled={!canSave} onClick={onSave}>保存</button>
      </div>
    </div>
  );
}
