// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describeActiveProvider, isConfigReady } from '../../shared/configUtils';
import { ReviewMode } from '../../shared/types';
import { initialState, reducer } from '../store';

const baseConfig = {
  provider: '',
  model: '',
  providers: {},
  customProviders: {},
  llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: false },
  language: 'Chinese',
};

describe('reducer', () => {
  it('init sets config and gitState', () => {
    const s = reducer(initialState, {
      type: 'init',
      config: baseConfig,
      gitState: { branches: [], currentBranch: 'main', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.config?.llm.model).toBe('m');
    expect(s.gitState.currentBranch).toBe('main');
    expect(s.view).toBe('idle');
  });

  it('init with null config keeps the main view idle', () => {
    const s = reducer(initialState, {
      type: 'init', config: null,
      gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.view).toBe('idle');
  });

  it('init / gitState / modeFiles stop loading; the filesLoading action starts it', () => {
    const init = reducer({ ...initialState, filesLoading: true }, {
      type: 'init', config: null,
      gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
    });
    expect(init.filesLoading).toBe(false);

    const started = reducer(init, { type: 'filesLoading' });
    expect(started.filesLoading).toBe(true);

    const loaded = reducer(started, { type: 'gitState', gitState: init.gitState });
    expect(loaded.filesLoading).toBe(false);
  });

  it('config message updates config', () => {
    const s = reducer(initialState, { type: 'config', config: baseConfig });
    expect(s.config?.llm.model).toBe('m');
  });

  it('stateChange running clears old logs and switches to the running view', () => {
    const s = reducer({ ...initialState, logs: [{ text: 'old', level: 'info' }] }, { type: 'stateChange', state: 'running' });
    expect(s.session.state).toBe('running');
    expect(s.logs).toEqual([]);
    expect(s.view).toBe('running');
  });

  it('logLine appends a log entry', () => {
    const s = reducer(initialState, { type: 'logLine', line: { text: 'x', level: 'info' } });
    expect(s.logs).toHaveLength(1);
  });

  it('reviewDone stores the result', () => {
    const s = reducer(initialState, {
      type: 'reviewDone',
      result: { status: 'success', comments: [], warnings: [], summary: undefined },
    });
    expect(s.session.result?.status).toBe('success');
  });

  it('stateChange done switches view to done', () => {
    expect(reducer(initialState, { type: 'stateChange', state: 'done' }).view).toBe('done');
  });

  it('commentSync updates the comment status map', () => {
    const s = reducer(initialState, { type: 'commentSync', comments: [{ index: 0, status: 'applied' }] });
    expect(s.commentStatus[0]).toBe('applied');
  });
});

describe('isConfigReady', () => {
  it('legacy llm config requires url/model/token', () => {
    expect(isConfigReady({
      ...baseConfig,
      llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: true },
    })).toBe(true);
    expect(isConfigReady({
      ...baseConfig,
      llm: { url: '', authToken: '', model: '', useAnthropic: true },
    })).toBe(false);
  });

  it('official provider requires model', () => {
    expect(isConfigReady({
      ...baseConfig,
      provider: 'anthropic',
      providers: { anthropic: { model: 'claude-opus-4-6' } },
    })).toBe(true);
  });

  it('custom provider requires url/protocol/apiKey/model', () => {
    expect(isConfigReady({
      ...baseConfig,
      provider: 'my-llm',
      customProviders: {
        'my-llm': { url: 'https://x', protocol: 'openai', model: 'm', apiKey: 'k' },
      },
    })).toBe(true);
  });
});

describe('describeActiveProvider', () => {
  it('official provider', () => {
    expect(describeActiveProvider({
      ...baseConfig,
      provider: 'anthropic',
      providers: { anthropic: { model: 'claude-opus-4-8' } },
    })).toMatchObject({
      kind: 'official',
      displayName: 'Anthropic Claude API',
      model: 'claude-opus-4-8',
    });
  });

  it('custom provider', () => {
    expect(describeActiveProvider({
      ...baseConfig,
      provider: 'my-llm',
      customProviders: {
        'my-llm': { url: 'https://x', protocol: 'openai', model: 'gpt-4', apiKey: 'k' },
      },
    })).toMatchObject({
      kind: 'custom',
      displayName: 'my-llm',
      model: 'gpt-4',
      detail: 'https://x',
    });
  });

  it('returns null when unconfigured', () => {
    expect(describeActiveProvider({
      ...baseConfig,
      llm: { url: '', authToken: '', model: '', useAnthropic: true },
    })).toBeNull();
  });
});

describe('modeFiles message', () => {
  it('stores the file list for the mode', () => {
    const next = reducer(initialState, {
      type: 'modeFiles',
      mode: ReviewMode.Branch,
      files: [{ path: 'src/a.ts', status: 'modified' }],
    });
    expect(next.modeFiles).toEqual([{ path: 'src/a.ts', status: 'modified' }]);
  });

  it('modeFiles starts as an empty array on init', () => {
    expect(initialState.modeFiles).toEqual([]);
  });
});
