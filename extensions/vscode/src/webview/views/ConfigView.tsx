import { useEffect, useMemo, useState } from 'preact/hooks';
import type { ComponentChildren } from 'preact';
import { ConfigEntry, ConfigPanelFocus, ProviderTab, buildCustomCreateSaveEntries, buildCustomUpdateSaveEntries, buildOfficialSaveEntries, describeActiveProvider, detectInitialTab, isConfigReady, listCustomProviderNames } from '../../shared/configUtils';
import { mergeModelLists, PROVIDER_PRESETS } from '../../shared/providers';
import { EnvCheckResult, LogLine, OcrConfig } from '../../shared/types';
import { CliStatus, ConnTest } from '../configStore';
import { CustomProviderManager } from '../components/CustomProviderManager';
import { EnvSetupGuide } from '../components/EnvSetupGuide';
import { LogViewer } from '../components/LogViewer';
import { PasswordInput } from '../components/PasswordInput';
import { Select } from '../components/Select';

interface Props {
  layout?: 'modal' | 'panel';
  panelFocus?: ConfigPanelFocus | null;
  skipEnvCheck?: boolean;
  config: OcrConfig | null;
  cliStatus: CliStatus;
  installing: boolean;
  installLogs: LogLine[];
  connTest: ConnTest;
  onInstall: () => void;
  onCheckCli: () => void;
  onCheckEnv?: () => void;
  onCopy?: (text: string) => void;
  envCheck?: EnvCheckResult | null;
  onTest: (entries: ConfigEntry[]) => void;
  onSave: (entries: ConfigEntry[]) => void;
  onClearConnTest?: () => void;
  onDeleteCustomProvider?: (name: string) => void;
  onActivateCustomProvider?: (name: string) => void;
  onClose: () => void;
}

const CUSTOM_NEW = '__new__';
const MODEL_CUSTOM = '__custom__';

function resolvePanelState(config: OcrConfig | null, panelFocus?: ConfigPanelFocus | null) {
  const tab = panelFocus?.tab ?? detectInitialTab(config);
  const step = panelFocus?.step ?? (isConfigReady(config) ? 2 : 1);
  const customNames = listCustomProviderNames(config);
  const customView = panelFocus?.customView
    ?? (tab === 'custom' && customNames.length > 0 ? 'list' : 'form');
  const customSelection = panelFocus?.customSelection
    ?? (panelFocus?.tab === 'custom' && panelFocus.customView === 'form' ? CUSTOM_NEW : customNames[0] ?? CUSTOM_NEW);
  return { step, tab, customView, customSelection };
}

export function ConfigView({
  layout = 'modal',
  panelFocus = null,
  skipEnvCheck = false,
  config, cliStatus, envCheck = null, installing, installLogs, connTest,
  onInstall, onCheckCli, onCheckEnv, onCopy, onTest, onSave, onClearConnTest,
  onDeleteCustomProvider, onActivateCustomProvider, onClose,
}: Props) {
  const initial = resolvePanelState(config, panelFocus);
  const [step, setStep] = useState<1 | 2>(initial.step);
  const [tab, setTab] = useState<ProviderTab>(initial.tab);
  const [customView, setCustomView] = useState<'list' | 'form'>(initial.customView);
  const [customSelection, setCustomSelection] = useState(initial.customSelection);

  useEffect(() => {
    if (!panelFocus) return;
    const next = resolvePanelState(config, panelFocus);
    onClearConnTest?.();
    setStep(next.step);
    setTab(next.tab);
    setCustomView(next.customView);
    setCustomSelection(next.customSelection);
  }, [panelFocus, config, onClearConnTest]);

  const wide = layout === 'panel';
  const stepper = (
    <div class="config-stepper">
      <div class={`config-step-pill${step === 1 ? ' active' : ''}${cliStatus === 'installed' ? ' done' : ''}`}>
        <span class="config-step-num">1</span>
        <span>环境检测</span>
      </div>
      <div class={`config-step-pill${step === 2 ? ' active' : ''}`}>
        <span class="config-step-num">2</span>
        <span>Provider 配置</span>
      </div>
    </div>
  );

  const stepContent = step === 1 ? (
    <EnvSetupGuide
      layout={layout}
      cliStatus={cliStatus}
      envCheck={envCheck}
      skipEnvCheck={skipEnvCheck}
      installing={installing}
      installLogs={installLogs}
      onInstall={onInstall}
      onCheckEnv={onCheckEnv ?? onCheckCli}
      onCopy={onCopy ?? (() => {})}
      onNext={() => setStep(2)}
    />
  ) : (
    <ProviderStep
      wide={wide}
      config={config}
      tab={tab}
      setTab={(next) => {
        onClearConnTest?.();
        setTab(next);
        if (next === 'custom' && listCustomProviderNames(config).length > 0) {
          setCustomView('list');
        }
      }}
      customView={customView}
      setCustomView={setCustomView}
      customSelection={customSelection}
      setCustomSelection={setCustomSelection}
      connTest={connTest}
      onBack={() => setStep(1)}
      onTest={onTest}
      onSave={onSave}
      onDeleteCustomProvider={onDeleteCustomProvider}
      onActivateCustomProvider={onActivateCustomProvider}
      onClearConnTest={onClearConnTest}
    />
  );

  if (layout === 'panel') {
    return (
      <div class="config-page">
        <header class="config-page-header">
          <div class="config-page-header-row">
            <div>
              <h1 class="config-page-title">模型配置</h1>
              <p class="config-page-desc">连接 LLM Provider 以开始代码审查</p>
            </div>
            <OcrVersionMeta envCheck={envCheck} cliStatus={cliStatus} />
          </div>
        </header>
        <div class="config-page-body">
          {stepper}
          {stepContent}
        </div>
      </div>
    );
  }

  const body = (
    <>
      <div class="config-form-header">
        <div class="config-form-heading">
          <span class="config-form-title">模型配置</span>
          <span class="config-form-subtitle">连接 LLM Provider 以开始代码审查</span>
        </div>
        <button type="button" class="config-list-close" onClick={onClose} aria-label="关闭">×</button>
      </div>
      {stepper}
      {stepContent}
    </>
  );

  return (
    <div class="modal-backdrop" onClick={onClose}>
      <div class="modal-panel" onClick={(e) => e.stopPropagation()}>
        {body}
      </div>
    </div>
  );
}

function OcrVersionMeta({ envCheck, cliStatus }: { envCheck: EnvCheckResult | null; cliStatus: CliStatus }) {
  let label = 'ocr 检测中…';
  if (envCheck?.ocr.ok && envCheck.ocr.version) {
    label = envCheck.ocr.version.startsWith('v') ? envCheck.ocr.version : `v${envCheck.ocr.version}`;
  } else if (envCheck && !envCheck.ocr.ok) {
    label = 'ocr 未安装';
  } else if (cliStatus !== 'checking' && cliStatus === 'missing') {
    label = 'ocr 未安装';
  }
  return (
    <div class="config-page-meta" title="Open Code Review CLI 版本">
      <span class="config-page-meta-label">OCR</span>
      <span class="config-page-meta-value">{label}</span>
    </div>
  );
}

function ActiveProviderBanner({ config }: { config: OcrConfig | null }) {
  const active = describeActiveProvider(config);
  if (!active) {
    return (
      <div class="active-provider-banner empty">
        <span class="active-provider-label">当前使用</span>
        <span class="active-provider-empty-text">尚未配置 Provider</span>
      </div>
    );
  }
  const kindLabel = active.kind === 'official' ? '官方' : active.kind === 'custom' ? '自定义' : 'Legacy';
  return (
    <div class="active-provider-banner">
      <span class="active-provider-label">当前使用</span>
      <span class="active-provider-badge">{kindLabel}</span>
      <span class="active-provider-name">{active.displayName}</span>
      <span class="active-provider-dot">·</span>
      <span class="active-provider-model">{active.model}</span>
      {active.detail && (
        <>
          <span class="active-provider-dot">·</span>
          <span class="active-provider-detail" title={active.detail}>{active.detail}</span>
        </>
      )}
    </div>
  );
}

function ProviderStep({
  wide, config, tab, setTab, customView, setCustomView, customSelection, setCustomSelection,
  connTest, onBack, onTest, onSave, onDeleteCustomProvider, onActivateCustomProvider, onClearConnTest,
}: {
  wide?: boolean;
  config: OcrConfig | null;
  tab: ProviderTab;
  setTab: (t: ProviderTab) => void;
  customView: 'list' | 'form';
  setCustomView: (v: 'list' | 'form') => void;
  customSelection: string;
  setCustomSelection: (v: string) => void;
  connTest: ConnTest;
  onBack: () => void;
  onTest: (entries: ConfigEntry[]) => void;
  onSave: (entries: ConfigEntry[]) => void;
  onDeleteCustomProvider?: (name: string) => void;
  onActivateCustomProvider?: (name: string) => void;
  onClearConnTest?: () => void;
}) {
  return (
    <div class="wizard-body provider-step">
      <div class="segmented-control">
        {([
          ['official', '官方 Provider'],
          ['custom', '自定义 Provider'],
        ] as const).map(([id, label]) => (
          <button
            key={id}
            type="button"
            class={`segmented-item${tab === id ? ' active' : ''}`}
            onClick={() => setTab(id)}
          >
            {label}
          </button>
        ))}
      </div>

      <ActiveProviderBanner config={config} />

      {tab === 'official' && (
        <OfficialForm wide={wide} config={config} connTest={connTest} onBack={onBack} onTest={onTest} onSave={onSave} />
      )}
      {tab === 'custom' && customView === 'list' && (
        <CustomProviderManager
          config={config}
          onAdd={() => { onClearConnTest?.(); setCustomSelection(CUSTOM_NEW); setCustomView('form'); }}
          onEdit={(name) => { onClearConnTest?.(); setCustomSelection(name); setCustomView('form'); }}
          onActivate={(name) => onActivateCustomProvider?.(name)}
          onDelete={(name) => onDeleteCustomProvider?.(name)}
        />
      )}
      {tab === 'custom' && customView === 'form' && (
        <CustomForm
          key={customSelection}
          wide={wide}
          config={config}
          connTest={connTest}
          selection={customSelection}
          onBack={onBack}
          onBackToList={() => { onClearConnTest?.(); setCustomView('list'); }}
          onTest={onTest}
          onSave={(entries) => {
            onSave(entries);
            setCustomView('list');
          }}
        />
      )}
    </div>
  );
}

function FormSection({ wide, children }: { wide?: boolean; children: ComponentChildren }) {
  if (wide) return <div class="provider-form">{children}</div>;
  return <>{children}</>;
}

function FormItem({
  label, span = 1, optional, hint, children,
}: {
  label: string;
  span?: 1 | 2;
  optional?: boolean;
  hint?: string;
  children: ComponentChildren;
}) {
  return (
    <div class={`form-item${span === 2 ? ' span-2' : ''}`}>
      <label class="form-label">
        {label}
        {optional && <span class="optional">（可选）</span>}
      </label>
      {children}
      {hint && <div class="form-hint">{hint}</div>}
    </div>
  );
}

function OfficialForm({ wide, config, connTest, onBack, onTest, onSave }: FormProps) {
  const initialProvider = config?.provider && PROVIDER_PRESETS.some((p) => p.name === config.provider)
    ? config.provider
    : PROVIDER_PRESETS[0].name;
  const [providerName, setProviderName] = useState(initialProvider);
  const preset = PROVIDER_PRESETS.find((p) => p.name === providerName) ?? PROVIDER_PRESETS[0];
  const savedEntry = config?.providers[providerName];

  const modelOptions = useMemo(
    () => mergeModelLists(preset.models, savedEntry?.models ?? []),
    [preset.models, savedEntry?.models, providerName],
  );

  const initialModel = savedEntry?.model || modelOptions[0] || '';
  const [modelChoice, setModelChoice] = useState(
    modelOptions.includes(initialModel) ? initialModel : MODEL_CUSTOM,
  );
  const [customModel, setCustomModel] = useState(
    modelOptions.includes(initialModel) ? '' : initialModel,
  );
  const [apiKey, setApiKey] = useState('');
  const [apiKeyTouched, setApiKeyTouched] = useState(false);
  const hasStoredKey = Boolean(savedEntry?.apiKey);

  const resolvedModel = modelChoice === MODEL_CUSTOM ? customModel.trim() : modelChoice;
  const canSave = resolvedModel !== '';

  const buildEntries = () => buildOfficialSaveEntries(
    providerName,
    resolvedModel,
    apiKey,
    apiKeyTouched || !hasStoredKey,
  );

  const save = () => {
    if (!canSave) return;
    onSave(buildEntries());
  };

  const test = () => {
    if (!canSave) return;
    onTest(buildEntries());
  };

  return (
    <FormSection wide={wide}>
      <FormItem label="Provider" span={wide ? 2 : 1}>
        <Select
          value={providerName}
          onChange={(v) => {
            setProviderName(v);
            const nextPreset = PROVIDER_PRESETS.find((p) => p.name === v);
            const entry = config?.providers[v];
            const models = mergeModelLists(nextPreset?.models ?? [], entry?.models ?? []);
            const m = entry?.model || models[0] || '';
            setModelChoice(models.includes(m) ? m : MODEL_CUSTOM);
            setCustomModel(models.includes(m) ? '' : m);
            setApiKey('');
            setApiKeyTouched(false);
          }}
          options={PROVIDER_PRESETS.map((p) => ({ value: p.name, label: p.displayName }))}
        />
      </FormItem>

      <FormItem label="模型">
        <Select
          value={modelChoice}
          onChange={setModelChoice}
          options={[
            ...modelOptions.map((m) => ({ value: m, label: m })),
            { value: MODEL_CUSTOM, label: '输入自定义模型…' },
          ]}
        />
        {modelChoice === MODEL_CUSTOM && (
          <input
            class="form-input form-input-mt"
            value={customModel}
            onInput={(e) => setCustomModel((e.target as HTMLInputElement).value)}
            placeholder="model name"
          />
        )}
      </FormItem>

      <FormItem
        label="API 密钥"
        hint={`也可通过环境变量 ${preset.envVar} 提供密钥`}
      >
        <PasswordInput
          value={apiKey}
          onInput={(v) => { setApiKey(v); setApiKeyTouched(true); }}
          placeholder={hasStoredKey && !apiKeyTouched ? '已保存（留空保持不变）' : 'sk-...'}
        />
      </FormItem>

      <ConnActions wide={wide} connTest={connTest} canSave={canSave} onBack={onBack} onTest={test} onSave={save} />
    </FormSection>
  );
}

function CustomForm({
  wide, config, connTest, selection, onBack, onBackToList, onTest, onSave,
}: FormProps & {
  selection: string;
  onBackToList?: () => void;
}) {
  const isCreate = selection === CUSTOM_NEW;
  const entry = !isCreate ? config?.customProviders[selection] : undefined;

  const [name, setName] = useState(isCreate ? '' : selection);
  const [protocol, setProtocol] = useState<'anthropic' | 'openai'>(
    (entry?.protocol as 'anthropic' | 'openai') || 'openai',
  );
  const [url, setUrl] = useState(entry?.url ?? '');
  const [model, setModel] = useState(entry?.model ?? '');
  const [models, setModels] = useState((entry?.models ?? []).join(', '));
  const [apiKey, setApiKey] = useState('');
  const [apiKeyTouched, setApiKeyTouched] = useState(false);
  const [authHeader, setAuthHeader] = useState(entry?.authHeader ?? '');

  const canSaveCreate = name.trim() !== '' && url.trim() !== '' && model.trim() !== '' && apiKey.trim() !== '';
  const canSaveEdit = !isCreate && name.trim() !== '' && url.trim() !== '' && model.trim() !== ''
    && Boolean(entry?.apiKey || apiKey.trim());
  const canSave = isCreate ? canSaveCreate : canSaveEdit;

  const buildCreateEntries = () => buildCustomCreateSaveEntries({
    name: name.trim(),
    protocol,
    url,
    model,
    models,
    apiKey,
    authHeader,
  });

  const buildEditEntries = () => buildCustomUpdateSaveEntries({
    name: name.trim(),
    protocol,
    url,
    model,
    models,
    apiKey,
    apiKeyChanged: apiKeyTouched || !entry?.apiKey,
    authHeader,
  });

  const buildEntries = () => (isCreate ? buildCreateEntries() : buildEditEntries());

  const save = () => {
    if (!canSave) return;
    onSave(buildEntries());
  };

  const test = () => {
    if (!canSave) return;
    onTest(buildEntries());
  };

  return (
    <FormSection wide={wide}>
      {onBackToList && (
        <div class="form-item span-2">
          <button type="button" class="btn-text back-link" onClick={onBackToList}>← 返回列表</button>
        </div>
      )}

      <FormItem label="Provider 名称">
        <input
          class="form-input"
          value={name}
          disabled={!isCreate}
          onInput={(e) => setName((e.target as HTMLInputElement).value)}
          placeholder="my-llm"
        />
      </FormItem>
      <FormItem label="协议">
        <Select
          value={protocol}
          onChange={(v) => setProtocol(v as 'anthropic' | 'openai')}
          options={[
            { value: 'anthropic', label: 'anthropic' },
            { value: 'openai', label: 'openai' },
          ]}
        />
      </FormItem>
      <FormItem label="Base URL" span={2}>
        <input class="form-input" value={url} onInput={(e) => setUrl((e.target as HTMLInputElement).value)} placeholder="https://api.example.com/v1" />
      </FormItem>
      <FormItem label="模型">
        <input class="form-input" value={model} onInput={(e) => setModel((e.target as HTMLInputElement).value)} placeholder="model name" />
      </FormItem>
      <FormItem label="模型列表" optional>
        <input class="form-input" value={models} onInput={(e) => setModels((e.target as HTMLInputElement).value)} placeholder="逗号分隔，如 model-a, model-b" />
      </FormItem>
      <FormItem label="API 密钥" span={2}>
        <PasswordInput
          value={apiKey}
          onInput={(v) => { setApiKey(v); setApiKeyTouched(true); }}
          placeholder={!isCreate && entry?.apiKey && !apiKeyTouched ? '已保存（留空保持不变）' : 'sk-...'}
        />
      </FormItem>
      <FormItem label="Auth Header" optional hint="Anthropic 协议下可选 x-api-key 或 authorization">
        <Select value={authHeader} placeholder="默认 (Authorization)" onChange={setAuthHeader}
          options={[
            { value: '', label: '默认 (Authorization)' },
            { value: 'x-api-key', label: 'x-api-key' },
            { value: 'authorization', label: 'authorization' },
          ]} />
      </FormItem>
      <ConnActions wide={wide} connTest={connTest} canSave={canSave} onBack={onBack} onTest={test} onSave={save} />
    </FormSection>
  );
}

interface FormProps {
  config: OcrConfig | null;
  connTest: ConnTest;
  wide?: boolean;
  onBack: () => void;
  onTest: (entries: ConfigEntry[]) => void;
  onSave: (entries: ConfigEntry[]) => void;
}

function ConnActions({ wide, connTest, canSave, onBack, onTest, onSave }: {
  wide?: boolean;
  connTest: ConnTest; canSave: boolean;
  onBack: () => void; onTest: () => void; onSave: () => void;
}) {
  return (
    <div class={`form-footer${wide ? ' page-footer' : ''}`}>
      {connTest.status !== 'idle' && (
        <div class={`conn-result ${connTest.status}`}>
          {connTest.status === 'testing' && '正在测试连接…'}
          {connTest.status === 'ok' && '✓ 连接成功'}
          {connTest.status === 'fail' && `✗ 连接失败${connTest.message ? '：' + connTest.message : ''}`}
        </div>
      )}
      <div class="form-actions">
        <button type="button" class="btn-default" onClick={onBack}>上一步</button>
        <button type="button" class="btn-default" disabled={connTest.status === 'testing' || !canSave} onClick={onTest}>测试连接</button>
        <button type="button" class="btn-primary" disabled={!canSave} onClick={onSave}>保存</button>
      </div>
    </div>
  );
}
