// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { h } from 'preact';
import { renderToString } from 'preact-render-to-string';
import { ConfigPathNotice } from '../components/ConfigPathNotice';
import { EnvSetupGuide, OCR_INSTALL_CMD } from '../components/EnvSetupGuide';
import { configPanelInitialState, configPanelReducer } from '../configStore';
import { initialState, reducer } from '../store';
import { EnvCheckResult } from '../../shared/types';

const tools = { node: { ok: true }, npm: { ok: true }, ocr: { ok: true, version: 'v1.0.0' } };

function guide(env: EnvCheckResult): string {
  return renderToString(h(EnvSetupGuide, {
    cliStatus: 'missing', envCheck: env, installing: false, installLogs: [],
    onInstall: () => {}, onCheckEnv: () => {}, onCopy: () => {}, onNext: () => {},
  }));
}

describe('configuration path compatibility', () => {
  it('offers an upgrade for a version-only CLI instead of continuing to model setup', () => {
    const env = { ...tools, configPath: { ok: false, upgradeRequired: true, message: 'Upgrade the OCR CLI' } };
    const state = configPanelReducer(configPanelInitialState, { type: 'environmentResult', env });
    expect(state.cliStatus).toBe('missing');
    const html = guide(env);
    expect(html).toContain('Upgrade CLI');
    expect(html).toContain(OCR_INSTALL_CMD);
    expect(html).not.toContain('Continue');
  });

  it('shows ordinary failures with retry, without offering an unsupported upgrade diagnosis', () => {
    const html = guide({ ...tools, configPath: { ok: false, message: 'OCR config path query timed out' } });
    expect(html).toContain('timed out');
    expect(html).not.toContain('Upgrade CLI');
    expect(html).not.toContain(OCR_INSTALL_CMD);
  });

  it('distinguishes sidebar lookup failures from a genuinely missing config and clears them after recovery', () => {
    const failure = { ok: false, upgradeRequired: true, message: 'Upgrade the OCR CLI' };
    const state = reducer(initialState, { type: 'init', config: null, gitState: initialState.gitState, locale: 'en', configPathStatus: failure });
    expect(renderToString(h(ConfigPathNotice, { status: state.configPathStatus, onOpen: () => {} }))).toContain('Upgrade CLI');
    const recovered = reducer(state, { type: 'config', config: null });
    expect(recovered.configPathStatus).toBeNull();
    expect(renderToString(h(ConfigPathNotice, { status: recovered.configPathStatus, onOpen: () => {} }))).toBe('');
  });
});
