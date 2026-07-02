import { describeActiveProvider, isConfigReady } from '../../shared/configUtils';
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
  it('init 设置 config 和 gitState', () => {
    const s = reducer(initialState, {
      type: 'init',
      config: baseConfig,
      gitState: { branches: [], currentBranch: 'main', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.config?.llm.model).toBe('m');
    expect(s.gitState.currentBranch).toBe('main');
    expect(s.view).toBe('idle');
  });

  it('init 时 config 为 null → 主界面仍是 idle', () => {
    const s = reducer(initialState, {
      type: 'init', config: null,
      gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.view).toBe('idle');
  });

  it('init / gitState / modeFiles 结束 loading；filesLoading action 开启 loading', () => {
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

  it('config 消息更新 config', () => {
    const s = reducer(initialState, { type: 'config', config: baseConfig });
    expect(s.config?.llm.model).toBe('m');
  });

  it('stateChange running 清空旧日志并切到 running 视图', () => {
    const s = reducer({ ...initialState, logs: [{ text: 'old', level: 'info' }] }, { type: 'stateChange', state: 'running' });
    expect(s.session.state).toBe('running');
    expect(s.logs).toEqual([]);
    expect(s.view).toBe('running');
  });

  it('logLine 追加日志', () => {
    const s = reducer(initialState, { type: 'logLine', line: { text: 'x', level: 'info' } });
    expect(s.logs).toHaveLength(1);
  });

  it('reviewDone 保存结果', () => {
    const s = reducer(initialState, {
      type: 'reviewDone',
      result: { status: 'success', comments: [], warnings: [], summary: undefined },
    });
    expect(s.session.result?.status).toBe('success');
  });

  it('stateChange done → view 切 done', () => {
    expect(reducer(initialState, { type: 'stateChange', state: 'done' }).view).toBe('done');
  });

  it('commentSync 更新评论状态映射', () => {
    const s = reducer(initialState, { type: 'commentSync', comments: [{ index: 0, status: 'applied' }] });
    expect(s.commentStatus[0]).toBe('applied');
  });
});

describe('isConfigReady', () => {
  it('legacy llm 配置需要 url/model/token', () => {
    expect(isConfigReady({
      ...baseConfig,
      llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: true },
    })).toBe(true);
    expect(isConfigReady({
      ...baseConfig,
      llm: { url: '', authToken: '', model: '', useAnthropic: true },
    })).toBe(false);
  });

  it('official provider 需要 model', () => {
    expect(isConfigReady({
      ...baseConfig,
      provider: 'anthropic',
      providers: { anthropic: { model: 'claude-opus-4-6' } },
    })).toBe(true);
  });

  it('custom provider 需要 url/protocol/apiKey/model', () => {
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
  it('官方 provider', () => {
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

  it('自定义 provider', () => {
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

  it('未配置时返回 null', () => {
    expect(describeActiveProvider({
      ...baseConfig,
      llm: { url: '', authToken: '', model: '', useAnthropic: true },
    })).toBeNull();
  });
});

describe('modeFiles 消息', () => {
  it('保存 mode 对应文件列表', () => {
    const next = reducer(initialState, {
      type: 'modeFiles',
      mode: 'branch',
      files: [{ path: 'src/a.ts', status: 'modified' }],
    });
    expect(next.modeFiles).toEqual([{ path: 'src/a.ts', status: 'modified' }]);
  });

  it('init 时 modeFiles 为空数组', () => {
    expect(initialState.modeFiles).toEqual([]);
  });
});
