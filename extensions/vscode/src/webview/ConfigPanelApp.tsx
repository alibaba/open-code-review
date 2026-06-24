import { useEffect, useReducer } from 'preact/hooks';
import { bridge } from './bridge';
import { ConfigView } from './views/ConfigView';
import { configPanelInitialState, configPanelReducer } from './configStore';
import './styles/global.css';

function runEnvCheck(dispatch: (action: { type: 'checkingEnv' }) => void): void {
  dispatch({ type: 'checkingEnv' });
  bridge.post({ type: 'checkEnvironment' });
}

export function ConfigPanelApp() {
  const [state, dispatch] = useReducer(configPanelReducer, configPanelInitialState);

  useEffect(() => {
    bridge.onMessage((msg) => dispatch(msg));
    bridge.post({ type: 'readyConfigPanel' });
  }, []);

  useEffect(() => {
    if (state.envCheck !== null) return;
    if (state.cliStatus === 'checking') return;
    runEnvCheck(dispatch);
  }, [state.envCheck, state.cliStatus]);

  useEffect(() => {
    if (!state.copyHint) return;
    const t = setTimeout(() => dispatch({ type: 'clearCopyHint' }), 2000);
    return () => clearTimeout(t);
  }, [state.copyHint]);

  return (
    <div class="config-panel-root">
      {state.copyHint && <div class="config-toast">{state.copyHint}</div>}
      <ConfigView
        layout="panel"
        panelFocus={state.panelFocus}
        config={state.config}
        cliStatus={state.cliStatus}
        envCheck={state.envCheck}
        installing={state.installing}
        installLogs={state.installLogs}
        connTest={state.connTest}
        onInstall={() => { dispatch({ type: 'installingCli' }); bridge.post({ type: 'installCli' }); }}
        onCheckCli={() => runEnvCheck(dispatch)}
        onCheckEnv={() => runEnvCheck(dispatch)}
        onCopy={(text) => bridge.post({ type: 'copyToClipboard', text })}
        onTest={(entries) => { dispatch({ type: 'testingConn' }); bridge.post({ type: 'testConnection', entries }); }}
        onSave={(entries) => bridge.post({ type: 'setConfigBatch', entries })}
        onClearConnTest={() => dispatch({ type: 'clearConnTest' })}
        onDeleteCustomProvider={(name) => bridge.post({ type: 'deleteCustomProvider', name })}
        onActivateCustomProvider={(name) => bridge.post({ type: 'activateCustomProvider', name })}
        onClose={() => bridge.post({ type: 'closeConfigPanel' })}
      />
    </div>
  );
}
