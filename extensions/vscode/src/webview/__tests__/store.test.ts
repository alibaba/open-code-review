import { initialState, reducer } from '../store';

describe('reducer', () => {
  it('init 设置 config 和 gitState', () => {
    const s = reducer(initialState, {
      type: 'init',
      config: { llm: { url: 'u', authToken: '', model: 'm', useAnthropic: false }, language: 'Chinese' },
      gitState: { branches: [], currentBranch: 'main', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.config?.llm.model).toBe('m');
    expect(s.gitState.currentBranch).toBe('main');
    expect(s.view).toBe('idle'); // 主界面始终是 idle（review 界面）
    expect(s.configOpen).toBe(false); // 已配置 → 不弹配置浮层
  });

  it('init 时 config 为 null → 主界面仍是 idle，且不自动弹出配置浮层', () => {
    const s = reducer(initialState, {
      type: 'init', config: null,
      gitState: { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] },
    });
    expect(s.view).toBe('idle');
    expect(s.configOpen).toBe(false);
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

  it('openConfig / closeConfig 切换配置浮层', () => {
    const opened = reducer(initialState, { type: 'openConfig' });
    expect(opened.configOpen).toBe(true);
    const closed = reducer(opened, { type: 'closeConfig' });
    expect(closed.configOpen).toBe(false);
  });

  it('config 保存后更新 config 并关闭浮层', () => {
    const s = reducer({ ...initialState, configOpen: true }, {
      type: 'config',
      config: { llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: false }, language: 'Chinese' },
    });
    expect(s.config?.llm.model).toBe('m');
    expect(s.configOpen).toBe(false);
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
