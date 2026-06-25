import { ConfigPanelFocus, isConfigReady } from '../shared/configUtils';
import { ConfigPanelHostToWebview, HostToWebview } from '../shared/messages';
import { EnvCheckResult, OcrConfig, LogLine } from '../shared/types';

export type CliStatus = 'unknown' | 'checking' | 'installed' | 'missing';
export type ConnTest = { status: 'idle' | 'testing' | 'ok' | 'fail'; message?: string };

export interface ConfigPanelState {
  config: OcrConfig | null;
  panelFocus: ConfigPanelFocus | null;
  skipEnvCheck: boolean;
  cliStatus: CliStatus;
  envCheck: EnvCheckResult | null;
  installing: boolean;
  installLogs: LogLine[];
  connTest: ConnTest;
  copyHint: string;
  errorHint: string;
}

export const configPanelInitialState: ConfigPanelState = {
  config: null,
  panelFocus: null,
  skipEnvCheck: false,
  cliStatus: 'unknown',
  envCheck: null,
  installing: false,
  installLogs: [],
  connTest: { status: 'idle' },
  copyHint: '',
  errorHint: '',
};

export type ConfigPanelLocalAction =
  | { type: 'checkingEnv' }
  | { type: 'installingCli' }
  | { type: 'testingConn' }
  | { type: 'clearConnTest' }
  | { type: 'clearCopyHint' }
  | { type: 'clearErrorHint' };

function envToCliStatus(env: EnvCheckResult): CliStatus {
  return env.node.ok && env.npm.ok && env.ocr.ok ? 'installed' : 'missing';
}

export function configPanelReducer(
  state: ConfigPanelState,
  msg: ConfigPanelHostToWebview | ConfigPanelLocalAction | HostToWebview,
): ConfigPanelState {
  switch (msg.type) {
    case 'checkingEnv':
      return { ...state, cliStatus: 'checking', envCheck: null };
    case 'installingCli':
      return { ...state, installing: true, installLogs: [] };
    case 'testingConn':
      return { ...state, connTest: { status: 'testing' } };
    case 'clearConnTest':
      return { ...state, connTest: { status: 'idle' } };
    case 'configPanelInit': {
      const env = msg.env ?? null;
      const skipEnvCheck = msg.skipEnvCheck ?? false;
      return {
        ...state,
        config: msg.config,
        panelFocus: msg.focus ?? null,
        skipEnvCheck,
        envCheck: env,
        cliStatus: env ? envToCliStatus(env) : (skipEnvCheck ? 'installed' : 'unknown'),
      };
    }
    case 'configPanelFocus':
      return {
        ...state,
        panelFocus: msg.focus ?? null,
        skipEnvCheck: msg.focus?.step === 2 || isConfigReady(state.config),
        connTest: { status: 'idle' },
      };
    case 'config':
      return { ...state, config: msg.config, connTest: { status: 'idle' as const } };
    case 'environmentResult':
      return {
        ...state,
        envCheck: msg.env,
        cliStatus: envToCliStatus(msg.env),
      };
    case 'cliStatus':
      return { ...state, cliStatus: msg.installed ? 'installed' : 'missing' };
    case 'installLog':
      return { ...state, installLogs: [...state.installLogs, msg.line] };
    case 'installDone':
      return { ...state, installing: false };
    case 'copyDone':
      return { ...state, copyHint: '已复制到剪贴板' };
    case 'clearCopyHint':
      return { ...state, copyHint: '' };
    case 'clearErrorHint':
      return { ...state, errorHint: '' };
    case 'panelError':
      return { ...state, errorHint: msg.message };
    case 'connectionResult':
      return { ...state, connTest: { status: msg.ok ? 'ok' : 'fail', message: msg.message } };
    default:
      return state;
  }
}

export { isConfigReady };
