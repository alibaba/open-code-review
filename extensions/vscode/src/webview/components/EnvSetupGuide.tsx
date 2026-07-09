import { EnvCheckResult, LogLine } from '../../shared/types';
import { CliStatus } from '../configStore';
import { LogViewer } from './LogViewer';
import { useT } from '../I18nProvider';

export const OCR_INSTALL_CMD = 'npm install -g @alibaba-group/open-code-review';

const CHECK_ITEMS = [
  { key: 'node', label: 'Node.js' },
  { key: 'npm', label: 'npm' },
  { key: 'ocr', label: 'ocr CLI' },
] as const;

interface Props {
  layout?: 'modal' | 'panel';
  cliStatus: CliStatus;
  envCheck: EnvCheckResult | null;
  skipEnvCheck?: boolean;
  installing: boolean;
  installLogs: LogLine[];
  onInstall: () => void;
  onCheckEnv: () => void;
  onCopy: (text: string) => void;
  onNext: () => void;
}

type StepState = 'pending' | 'checking' | 'ok' | 'fail';

function resolveStepState(
  active: boolean,
  checking: boolean,
  ok: boolean | undefined,
): StepState {
  if (checking && active) return 'checking';
  if (!active) return 'pending';
  if (ok === undefined) return 'pending';
  return ok ? 'ok' : 'fail';
}

export function EnvSetupGuide({
  layout, cliStatus, envCheck, skipEnvCheck = false, installing, installLogs,
  onInstall, onCheckEnv, onCopy, onNext,
}: Props) {
  const t = useT();
  const checking = cliStatus === 'checking' || cliStatus === 'unknown';

  if (installing) {
    return (
      <div class="wizard-body">
        <EnvCheckingBanner label={t('view.env.installing')} />
        <LogViewer logs={installLogs} />
      </div>
    );
  }

  if (checking) {
    return (
      <div class="wizard-body">
        <EnvCheckingBanner label={t('view.env.checking')} />
        <EnvChecklist checking />
      </div>
    );
  }

  if (cliStatus === 'installed' && envCheck) {
    return (
      <div class="wizard-body">
        <EnvChecklist env={envCheck} />
        <div class={`form-footer${layout === 'panel' ? ' page-footer' : ''}`}>
          <div class={`form-actions${layout === 'panel' ? ' panel-actions' : ''}`}>
            <button type="button" class="btn-primary" onClick={onNext}>{t('view.config.continueProvider')}</button>
          </div>
        </div>
      </div>
    );
  }

  if (cliStatus === 'installed' && skipEnvCheck) {
    return (
      <div class="wizard-body">
        <p class="env-guide-lead">{t('view.env.ready')}</p>
        <div class={`form-footer${layout === 'panel' ? ' page-footer' : ''}`}>
          <div class={`form-actions${layout === 'panel' ? ' panel-actions' : ''}`}>
            <button type="button" class="btn-primary" onClick={onNext}>{t('view.config.continueProvider')}</button>
          </div>
        </div>
      </div>
    );
  }

  const nodeActive = true;
  const npmActive = envCheck?.node.ok ?? false;
  const ocrActive = npmActive && (envCheck?.npm.ok ?? false);

  return (
    <div class="wizard-body">
      <p class="env-guide-lead">{t('view.env.stepLead')}</p>

      <div class="env-timeline">
        <EnvTimelineItem
          title="Node.js"
          state={resolveStepState(nodeActive, false, envCheck?.node.ok)}
          version={envCheck?.node.version}
          command="node --version"
          hint={t('view.env.nodeHint')}
        />
        <EnvTimelineItem
          title="npm"
          state={resolveStepState(npmActive, false, envCheck?.npm.ok)}
          version={envCheck?.npm.version}
          command="npm --version"
          hint={t('view.env.npmHint')}
        />
        <EnvTimelineItem
          title="ocr CLI"
          state={resolveStepState(ocrActive, false, envCheck?.ocr.ok)}
          version={envCheck?.ocr.version}
          command={OCR_INSTALL_CMD}
          hint={t('view.env.ocrHint')}
          onCopy={onCopy}
          last
        />
      </div>

      {installLogs.length > 0 && <LogViewer logs={installLogs} />}

      <div class={`form-footer${layout === 'panel' ? ' page-footer' : ''}`}>
        <div class={`form-actions${layout === 'panel' ? ' panel-actions' : ''}`}>
          <button type="button" class="btn-default" onClick={onCheckEnv}>{t('view.env.redetect')}</button>
          {ocrActive && !envCheck?.ocr.ok && (
            <button type="button" class="btn-primary" onClick={onInstall}>{t('view.env.oneClickInstall')}</button>
          )}
        </div>
      </div>
    </div>
  );
}

function EnvCheckingBanner({ label }: { label: string }) {
  return (
    <div class="env-checking-banner">
      <span class="env-spinner" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

function EnvChecklist({ checking, env }: { checking?: boolean; env?: EnvCheckResult }) {
  const t = useT();
  return (
    <ul class={`env-checklist${checking ? ' is-checking' : ''}${env ? ' is-done' : ''}`}>
      {CHECK_ITEMS.map(({ key, label }, i) => {
        const item = env?.[key];
        const ok = item?.ok;
        const last = i === CHECK_ITEMS.length - 1;
        return (
          <li key={key} class={`env-checklist-item${checking ? ' loading' : ''}${ok ? ' ok' : env ? ' fail' : ''}${last ? ' last' : ''}`}>
            <span class="env-checklist-marker" aria-hidden="true" />
            <span class="env-checklist-label">{label}</span>
            <span class="env-checklist-meta">
              {checking && t('view.env.checkingStatus')}
              {!checking && ok && (item?.version ?? t('view.env.readyStatus'))}
              {!checking && env && !ok && t('view.env.notReady')}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

function EnvTimelineItem({
  title, state, version, command, hint, onCopy, last,
}: {
  title: string;
  state: StepState;
  version?: string;
  command: string;
  hint: string;
  onCopy?: (text: string) => void;
  last?: boolean;
}) {
  const t = useT();
  const showDetail = state === 'fail' || state === 'ok';
  return (
    <div class={`env-timeline-item ${state}${last ? ' last' : ''}`}>
      <div class="env-timeline-track">
        <span class="env-timeline-dot" aria-hidden="true" />
        {!last && <span class="env-timeline-line" aria-hidden="true" />}
      </div>
      <div class="env-timeline-content">
        <div class="env-timeline-head">
          <span class="env-timeline-title">{title}</span>
          <span class={`env-timeline-status ${state}`}>
            {state === 'ok' && (version ?? t('view.env.pass'))}
            {state === 'fail' && t('view.env.fail')}
            {state === 'pending' && t('view.env.waitPrev')}
            {state === 'checking' && t('view.env.checkingStatus')}
          </span>
        </div>
        {showDetail && (
          <div class="env-timeline-detail">
            {state === 'fail' && <p class="env-timeline-hint">{hint}</p>}
            <div class="env-cmd-block">
              <code>{command}</code>
              {onCopy && (
                <button type="button" class="env-cmd-copy" onClick={() => onCopy(command)}>{t('view.env.copy')}</button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
