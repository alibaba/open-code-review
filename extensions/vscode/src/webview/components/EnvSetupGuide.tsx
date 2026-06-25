import { EnvCheckResult, LogLine } from '../../shared/types';
import { CliStatus } from '../configStore';
import { LogViewer } from './LogViewer';

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
  const checking = cliStatus === 'checking' || cliStatus === 'unknown';

  if (installing) {
    return (
      <div class="wizard-body">
        <EnvCheckingBanner label="正在安装 ocr CLI…" />
        <LogViewer logs={installLogs} />
      </div>
    );
  }

  if (checking) {
    return (
      <div class="wizard-body">
        <EnvCheckingBanner label="正在检测，请稍候…" />
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
            <button type="button" class="btn-primary" onClick={onNext}>继续配置 Provider</button>
          </div>
        </div>
      </div>
    );
  }

  if (cliStatus === 'installed' && skipEnvCheck) {
    return (
      <div class="wizard-body">
        <p class="env-guide-lead">环境已就绪，可继续配置 Provider。</p>
        <div class={`form-footer${layout === 'panel' ? ' page-footer' : ''}`}>
          <div class={`form-actions${layout === 'panel' ? ' panel-actions' : ''}`}>
            <button type="button" class="btn-primary" onClick={onNext}>继续配置 Provider</button>
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
      <p class="env-guide-lead">按顺序完成环境准备，通过一项后再进行下一项。</p>

      <div class="env-timeline">
        <EnvTimelineItem
          title="Node.js"
          state={resolveStepState(nodeActive, false, envCheck?.node.ok)}
          version={envCheck?.node.version}
          command="node --version"
          hint="未检测到 Node.js。请前往 nodejs.org 安装 LTS 版本，完成后重启 VS Code。"
        />
        <EnvTimelineItem
          title="npm"
          state={resolveStepState(npmActive, false, envCheck?.npm.ok)}
          version={envCheck?.npm.version}
          command="npm --version"
          hint="未检测到 npm。npm 通常随 Node 一起安装，请确认 Node 安装完整。"
        />
        <EnvTimelineItem
          title="ocr CLI"
          state={resolveStepState(ocrActive, false, envCheck?.ocr.ok)}
          version={envCheck?.ocr.version}
          command={OCR_INSTALL_CMD}
          hint="在终端全局安装 open-code-review，或点击下方「一键安装」。"
          onCopy={onCopy}
          last
        />
      </div>

      {installLogs.length > 0 && <LogViewer logs={installLogs} />}

      <div class={`form-footer${layout === 'panel' ? ' page-footer' : ''}`}>
        <div class={`form-actions${layout === 'panel' ? ' panel-actions' : ''}`}>
          <button type="button" class="btn-default" onClick={onCheckEnv}>重新检测</button>
          {ocrActive && !envCheck?.ocr.ok && (
            <button type="button" class="btn-primary" onClick={onInstall}>一键安装</button>
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
              {checking && '检测中'}
              {!checking && ok && (item?.version ?? '就绪')}
              {!checking && env && !ok && '未就绪'}
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
            {state === 'ok' && (version ?? '通过')}
            {state === 'fail' && '未通过'}
            {state === 'pending' && '等待上一步'}
            {state === 'checking' && '检测中'}
          </span>
        </div>
        {showDetail && (
          <div class="env-timeline-detail">
            {state === 'fail' && <p class="env-timeline-hint">{hint}</p>}
            <div class="env-cmd-block">
              <code>{command}</code>
              {onCopy && (
                <button type="button" class="env-cmd-copy" onClick={() => onCopy(command)}>复制</button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
