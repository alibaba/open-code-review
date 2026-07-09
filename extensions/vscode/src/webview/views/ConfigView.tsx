import { useEffect, useMemo, useState } from 'preact/hooks';
import type { ComponentChildren } from 'preact';
import { ConfigEntry, ConfigPanelFocus, ProviderTab, buildCustomCreateSaveEntries, buildCustomUpdateSaveEntries, buildOfficialSaveEntries, describeActiveProvider, detectInitialTab, isConfigReady, listCustomProviderNames } from '../../shared/configUtils';
import { mergeModelLists, PROVIDER_PRESETS } from '../../shared/providers';
import { EnvCheckResult, LogLine, OcrConfig } from '../../shared/types';
import { CliStatus, ConnTest } from '../configStore';
import { CustomProviderManager } from '../components/CustomProviderManager';
import { EnvSetupGuide } from '../components/EnvSetupGuide';
import { PasswordInput } from '../components/PasswordInput';
import { Select } from '../components/Select';
import { useT } from '../I18nProvider';

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
  }, [panelFocus, config]);

  const wide = layout === 'panel';
  const t = useT();
  const stepper = (
    <div class="config-stepper">
      <div class={`config-step-pill${step === 1 ? ' done' : ''}`}>
        <span class="config-step-num">1</span>
        <span>{t('view.config.step1')}</span>
      </div>
      <div class={`config-step-pill${step === 2 ? ' done' : ''}`}>
        <span class="config-step-num">2</span>
        <span>{t('view.config.step2')}</span>
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
              <h1 class="config-page-title">{t('view.config.title')}</h1>
              <p class="config-page-desc">{t('view.config.desc')}</p>
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
          <span class="config-form-title">{t('view.config.title')}</span>
          <span class="config-form-subtitle">{t('view.config.desc')}</span>
        </div>
        <button type="button" class="config-list-close" onClick={onClose} aria-label={t('view.config.close')}>×</button>
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
  const t = useT();
  let label = t('view.config.checking');
  if (envCheck?.ocr.ok && envCheck.ocr.version) {
    label = envCheck.ocr.version.startsWith('v') ? envCheck.ocr.version : `v${envCheck.ocr.version}`;
  } else if (envCheck && !envCheck.ocr.ok) {
    label = t('view.config.notInstalled');
  } else if (cliStatus !== 'checking' && cliStatus === 'missing') {
    label = t('view.config.notInstalled');
  }
  return (
    <div class="config-page-meta" title={t('view.config.ocrVersionTooltip')}>
      <span class="config-page-meta-label">OCR</span>
      <span class="config-page-meta-value">{label}</span>
    </div>
  );
}

function ActiveProviderBanner({ config }: { config: OcrConfig | null }) {
  const t = useT();
  const active = describeActiveProvider(config);
  if (!active) {
    return (
      <div class="active-provider-banner empty">
        <span class="active-provider-label">{t('view.config.currentUse')}</span>
        <span class="active-provider-empty-text">{t('view.config.notConfigured')}</span>
      </div>
    );
  }
  const kindLabelMap: Record<string, string> = {
    official: t('view.config.officialLabel'),
    custom: t('view.config.customLabel'),
    legacy: t('view.config.legacyLabel'),
  };
  const kindLabel = kindLabelMap[active.kind] ?? t('view.config.legacyLabel');
  const displayName = active.kind === 'legacy' ? t('ext.config.legacyDisplayName') : active.displayName;
  return (
    <div class="active-provider-banner">
      <span class="active-provider-label">{t('view.config.currentUse')}</span>
      <span class="active-provider-badge">{kindLabel}</span>
      <span class="active-provider-name">{displayName}</span>
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
  const t = useT();
  return (
    <div class="wizard-body provider-step">
      <div class="segmented-control">
        {([
          ['official', t('view.config.official')],
          ['custom', t('view.config.custom')],
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
  const t = useT();
  return (
    <div class={`form-item${span === 2 ? ' span-2' : ''}`}>
      <label class="form-label">
        {label}
        {optional && <span class="optional">{t('view.config.optional')}</span>}
      </label>
      {children}
      {hint && <div class="form-hint">{hint}</div>}
    </div>
  );
}

function OfficialForm({ wide, config, connTest, onBack, onTest, onSave }: FormProps) {
  const t = useT();
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

      <FormItem label={t('view.config.model')}>
        <Select
          value={modelChoice}
          onChange={setModelChoice}
          options={[
            ...modelOptions.map((m) => ({ value: m, label: m })),
            { value: MODEL_CUSTOM, label: t('view.config.customModel') },
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
        label={t('view.config.apiKey')}
        hint={`${t('view.config.apiKeyEnvHint')} ${preset.envVar}`}
      >
        <PasswordInput
          value={apiKey}
          onInput={(v) => { setApiKey(v); setApiKeyTouched(true); }}
          placeholder={hasStoredKey && !apiKeyTouched ? t('view.config.apiKeySaved') : 'sk-...'}
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
  const t = useT();
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
          <button type="button" class="btn-text back-link" onClick={onBackToList}>{t('view.config.backToList')}</button>
        </div>
      )}

      <FormItem label={t('view.config.providerName')}>
        <input
          class="form-input"
          value={name}
          disabled={!isCreate}
          onInput={(e) => setName((e.target as HTMLInputElement).value)}
          placeholder="my-llm"
        />
      </FormItem>
      <FormItem label={t('view.config.protocol')}>
        <Select
          value={protocol}
          onChange={(v) => setProtocol(v as 'anthropic' | 'openai')}
          options={[
            { value: 'anthropic', label: 'anthropic' },
            { value: 'openai', label: 'openai' },
          ]}
        />
      </FormItem>
      <FormItem label={t('view.config.baseUrl')} span={2}>
        <input class="form-input" value={url} onInput={(e) => setUrl((e.target as HTMLInputElement).value)} placeholder="https://api.example.com/v1" />
      </FormItem>
      <FormItem label={t('view.config.model')}>
        <input class="form-input" value={model} onInput={(e) => setModel((e.target as HTMLInputElement).value)} placeholder="model name" />
      </FormItem>
      <FormItem label={t('view.config.modelList')} optional>
        <input class="form-input" value={models} onInput={(e) => setModels((e.target as HTMLInputElement).value)} placeholder={t('view.config.modelListPlaceholder')} />
      </FormItem>
      <FormItem label={t('view.config.apiKey')} span={2}>
        <PasswordInput
          value={apiKey}
          onInput={(v) => { setApiKey(v); setApiKeyTouched(true); }}
          placeholder={!isCreate && entry?.apiKey && !apiKeyTouched ? t('view.config.apiKeySaved') : 'sk-...'}
        />
      </FormItem>
      <FormItem label={t('view.config.authHeader')} optional hint={t('view.config.authHeaderHint')}>
        <Select value={authHeader} placeholder={t('view.config.authHeaderDefault')} onChange={setAuthHeader}
          options={[
            { value: '', label: t('view.config.authHeaderDefault') },
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
  const t = useT();
  return (
    <div class={`form-footer${wide ? ' page-footer' : ''}`}>
      {connTest.status !== 'idle' && (
        <div class={`conn-result ${connTest.status}`}>
          {connTest.status === 'testing' && t('view.config.testing')}
          {connTest.status === 'ok' && t('view.config.testOk')}
          {connTest.status === 'fail' && t(connTest.message ? 'view.config.testFailDetail' : 'view.config.testFail').replace('{message}', connTest.message ?? '')}
        </div>
      )}
      <div class="form-actions">
        <button type="button" class="btn-default" onClick={onBack}>{t('view.config.previous')}</button>
        <button type="button" class="btn-default" disabled={connTest.status === 'testing' || !canSave} onClick={onTest}>{t('view.config.test')}</button>
        <button type="button" class="btn-primary" disabled={!canSave} onClick={onSave}>{t('view.config.save')}</button>
      </div>
    </div>
  );
}