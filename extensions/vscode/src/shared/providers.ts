export interface OcrProviderPreset {
  name: string;
  displayName: string;
  protocol: 'anthropic' | 'openai';
  baseUrl: string;
  authHeader?: string;
  envVar: string;
  models: string[];
}

/** 与 internal/llm/providers.go 内置 registry 对齐 */
export const PROVIDER_PRESETS: OcrProviderPreset[] = [
  {
    name: 'anthropic',
    displayName: 'Anthropic Claude API',
    protocol: 'anthropic',
    baseUrl: 'https://api.anthropic.com',
    authHeader: 'x-api-key',
    envVar: 'ANTHROPIC_API_KEY',
    models: ['claude-opus-4-8', 'claude-opus-4-7', 'claude-opus-4-6', 'claude-sonnet-4-6'],
  },
  {
    name: 'openai',
    displayName: 'OpenAI API',
    protocol: 'openai',
    baseUrl: 'https://api.openai.com/v1',
    envVar: 'OPENAI_API_KEY',
    models: ['gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini'],
  },
  {
    name: 'dashscope',
    displayName: 'Alibaba DashScope API',
    protocol: 'openai',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    envVar: 'DASHSCOPE_API_KEY',
    models: ['qwen3.7-max', 'qwen3.7-plus', 'qwen3.6-plus', 'qwen3.6-flash'],
  },
  {
    name: 'dashscope-tokenplan',
    displayName: 'Alibaba DashScope Token Plan API',
    protocol: 'openai',
    baseUrl: 'https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1',
    envVar: 'DASHSCOPE_TOKENPLAN_KEY',
    models: [
      'qwen3.7-max', 'qwen3.7-plus', 'qwen3.6-plus', 'qwen3.6-flash',
      'deepseek-v4-pro', 'deepseek-v4-flash', 'kimi-k2.6', 'kimi-k2.5',
      'glm-5.2', 'glm-5.1', 'glm-5', 'MiniMax-M2.5',
    ],
  },
  {
    name: 'volcengine',
    displayName: 'Volcano Engine Ark API',
    protocol: 'openai',
    baseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
    envVar: 'ARK_API_KEY',
    models: ['doubao-seed-2-0-lite-260428', 'doubao-seed-2-0-mini-260428', 'doubao-seed-2-0-pro-260215'],
  },
  {
    name: 'deepseek',
    displayName: 'DeepSeek API',
    protocol: 'openai',
    baseUrl: 'https://api.deepseek.com',
    envVar: 'DEEPSEEK_API_KEY',
    models: ['deepseek-v4-pro', 'deepseek-v4-flash'],
  },
  {
    name: 'tencent-tokenhub',
    displayName: 'Tencent TokenHub API',
    protocol: 'openai',
    baseUrl: 'https://tokenhub.tencentmaas.com/v1',
    envVar: 'TENCENT_TOKENHUB_API_KEY',
    models: ['hy3-preview'],
  },
  {
    name: 'hy-tokenplan',
    displayName: 'Tencent Hunyuan Token Plan API',
    protocol: 'openai',
    baseUrl: 'https://api.lkeap.cloud.tencent.com/plan/v3',
    envVar: 'TENCENT_HUNYUAN_TOKENPLAN_KEY',
    models: ['hy3-preview'],
  },
  {
    name: 'kimi',
    displayName: 'Kimi Moonshot API',
    protocol: 'openai',
    baseUrl: 'https://api.moonshot.cn/v1',
    envVar: 'MOONSHOT_API_KEY',
    models: ['kimi-k2.7-code', 'kimi-k2.6', 'kimi-k2.5'],
  },
  {
    name: 'z-ai',
    displayName: 'Z.AI API',
    protocol: 'openai',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    envVar: 'Z_AI_API_KEY',
    models: ['glm-5.2', 'glm-5.1', 'glm-5-turbo', 'glm-4.7'],
  },
  {
    name: 'mimo',
    displayName: 'Xiaomi MiMo API',
    protocol: 'openai',
    baseUrl: 'https://api.xiaomimimo.com/v1',
    envVar: 'MIMO_API_KEY',
    models: ['mimo-v2.5-pro', 'mimo-v2.5', 'mimo-v2-pro', 'mimo-v2-omni', 'mimo-v2-flash'],
  },
  {
    name: 'minimax',
    displayName: 'MiniMax API',
    protocol: 'openai',
    baseUrl: 'https://api.minimaxi.com/v1',
    envVar: 'MINIMAX_API_KEY',
    models: ['MiniMax-M3', 'MiniMax-M2.7', 'MiniMax-M2.7-highspeed', 'MiniMax-M2.5', 'MiniMax-M2.5-highspeed'],
  },
  {
    name: 'baidu-qianfan',
    displayName: 'Baidu Qianfan API',
    protocol: 'openai',
    baseUrl: 'https://qianfan.baidubce.com/v2',
    envVar: 'QIANFAN_API_KEY',
    models: ['ernie-5.1', 'ernie-5.0', 'ernie-x1.1', 'ernie-x1-turbo-32k-preview', 'deepseek-v4-pro', 'deepseek-v4-flash'],
  },
];

const presetMap = new Map(PROVIDER_PRESETS.map((p) => [p.name.toLowerCase(), p]));

export function lookupPreset(name: string): OcrProviderPreset | undefined {
  return presetMap.get(name.trim().toLowerCase());
}

export function isPresetProvider(name: string): boolean {
  return presetMap.has(name.trim().toLowerCase());
}

export function mergeModelLists(...lists: string[][]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const list of lists) {
    for (const raw of list) {
      const model = raw.trim();
      if (!model || seen.has(model)) continue;
      seen.add(model);
      out.push(model);
    }
  }
  return out;
}
