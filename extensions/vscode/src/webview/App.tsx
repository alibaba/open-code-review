import { useEffect, useReducer } from 'preact/hooks';
import { reducer, initialState } from './store';
import { bridge } from './bridge';
import { isConfigReady } from '../shared/configUtils';
import { CliRunOptions, FileChange, ReviewMode } from '../shared/types';
import { IdleView } from './views/IdleView';
import { RunningView } from './views/RunningView';
import { DoneView } from './views/DoneView';
import { EmptyView } from './views/EmptyView';
import { CancelledView } from './views/CancelledView';
import { FailedView } from './views/FailedView';
import './styles/global.css';

export function App() {
  const [state, dispatch] = useReducer(reducer, initialState);

  useEffect(() => {
    const unsub = bridge.onMessage((msg) => dispatch(msg));
    bridge.post({ type: 'ready' });
    return unsub;
  }, []);

  const configured = isConfigReady(state.config);
  const start = (options: CliRunOptions) => {
    dispatch({ type: 'startReview', mode: options.mode });
    bridge.post({ type: 'startReview', options });
  };
  const onModeChange = (mode: ReviewMode) => {
    dispatch({ type: 'filesLoading' });
    bridge.post({ type: 'getGitState', mode });
  };
  const requestModeFiles = (mode: ReviewMode, from?: string, to?: string, commit?: string) => {
    dispatch({ type: 'filesLoading' });
    bridge.post({ type: 'getModeFiles', mode, from, to, commit });
  };
  const openFile = (file: FileChange, mode: ReviewMode, from?: string, to?: string, commit?: string) => {
    bridge.post({ type: 'openFileDiff', path: file.path, status: file.status, mode, from, to, commit });
  };

  return (
    <div class="ocr-root">
      <div class="action-region">
        <IdleView gitState={state.gitState} modeFiles={state.modeFiles} filesLoading={state.filesLoading}
          configured={configured} onModeChange={onModeChange} onRequestModeFiles={requestModeFiles}
          onOpenFile={openFile} onStart={start} onOpenConfig={() => bridge.post({ type: 'openConfigPanel' })}
          onOpenCustomProviders={() => bridge.post({
            type: 'openConfigPanel',
            focus: { step: 2, tab: 'custom', customView: 'list' },
          })}
          running={state.view === 'running'} />

        {state.view !== 'idle' && (
          <div class="result-region">
            {state.view === 'running' && <RunningView logs={state.logs} onCancel={() => bridge.post({ type: 'cancelReview' })} />}
            {state.view === 'done' && state.session.result && (
              <DoneView result={state.session.result} commentStatus={state.commentStatus} logs={state.logs}
                canJump={state.reviewMode === 'workspace'}
                onOpen={(i) => bridge.post({ type: 'jumpToComment', index: i })}
                onAction={(i, action) => bridge.post({ type: 'commentAction', index: i, action })} />
            )}
            {state.view === 'empty' && <EmptyView logs={state.logs} />}
            {state.view === 'cancelled' && <CancelledView />}
            {state.view === 'failed' && <FailedView error={state.session.error} onRetry={() => start({ mode: 'workspace' })} />}
          </div>
        )}
      </div>
    </div>
  );
}
