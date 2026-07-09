package llm

import (
	"maps"
	"sort"
	"strings"
)

// ModelThinkingConfig describes per-model thinking behavior for a provider.
// A zero value means "use the provider's default ThinkingStyle, toggleable".
// All fields are value types (strings/enums), so shallow-copying this struct is safe.
type ModelThinkingConfig struct {
	// Style overrides the provider's default ThinkingStyle for this model.
	// Zero value means no override.
	Style ThinkingStyle
	// Policy overrides the default toggleable behavior for this model.
	// Zero value means "no override: fall back to the provider's default policy".
	// The only meaningful explicit values are AlwaysOn, ThinkingOnly, and NotSupported.
	Policy ThinkingPolicy
	// ObjectOn sets thinking.type when mode is "on" (ThinkingStyleObject only).
	// Zero value means "enabled". Use "adaptive" for models that reject "enabled".
	ObjectOn string
}

// Provider holds the preset configuration for a known LLM provider.
type Provider struct {
	Name        string
	DisplayName string
	Protocol    string // "anthropic" or "openai"
	BaseURL     string
	AuthHeader  string // Anthropic-only; empty for OpenAI-compatible
	EnvVar      string // environment variable name for API key fallback
	Models      []string
	// ThinkingStyle is the default encoding style for this provider's models.
	// Use ThinkingStyleNotSupported to hide the thinking toggle entirely.
	ThinkingStyle ThinkingStyle
	// ModelThinking overrides default thinking behavior per model (case-insensitive key).
	// Override keys are lowercase; Models entries may use provider-canonical casing.
	ModelThinking map[string]ModelThinkingConfig
}

// thinkingProviderMeta is the runtime index built from registry thinking fields.
type thinkingProviderMeta struct {
	defaultStyle   ThinkingStyle
	modelOverrides map[string]ModelThinkingConfig
}

var thinkingProviderIndex map[string]thinkingProviderMeta

var registry = []Provider{
	{
		Name:        "anthropic",
		DisplayName: "Anthropic Claude API",
		Protocol:    "anthropic",
		BaseURL:     "https://api.anthropic.com",
		AuthHeader:  "x-api-key",
		EnvVar:      "ANTHROPIC_API_KEY",
		// Thinking auto-inject is off; users may pass thinking via extra_body.
		ThinkingStyle: ThinkingStyleNotSupported,
		Models: []string{
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
		},
	},
	{
		Name:          "openai",
		DisplayName:   "OpenAI API",
		Protocol:      "openai",
		BaseURL:       "https://api.openai.com/v1",
		EnvVar:        "OPENAI_API_KEY",
		ThinkingStyle: ThinkingStyleNotSupported,
		Models: []string{
			"gpt-5.5",
			"gpt-5.4",
			"gpt-5.4-mini",
		},
	},
	{
		Name:          "dashscope",
		DisplayName:   "Alibaba DashScope API",
		Protocol:      "openai",
		BaseURL:       "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvVar:        "DASHSCOPE_API_KEY",
		ThinkingStyle: ThinkingStyleEnableBool,
		ModelThinking: map[string]ModelThinkingConfig{
			"deepseek-v4-pro":   {Style: ThinkingStyleObject},
			"deepseek-v4-flash": {Style: ThinkingStyleObject},
			// MiniMax on DashScope uses thinking.type, not enable_thinking (see supplier report).
			"minimax-m2.5":   {Style: ThinkingStyleObject, Policy: ThinkingPolicyAlwaysOn},
			"kimi-k2.7-code": {Policy: ThinkingPolicyAlwaysOn},
		},
		Models: []string{
			"qwen3.7-max",
			"qwen3.7-plus",
			"qwen3.6-plus",
			"qwen3.6-flash",
			"deepseek-v4-pro",
			"deepseek-v4-flash",
			"kimi-k2.7-code",
			"glm-5.2",
			"MiniMax-M2.5",
		},
	},
	{
		Name:          "dashscope-tokenplan",
		DisplayName:   "Alibaba DashScope Token Plan API",
		Protocol:      "openai",
		BaseURL:       "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		EnvVar:        "DASHSCOPE_TOKENPLAN_KEY",
		ThinkingStyle: ThinkingStyleEnableBool,
		ModelThinking: map[string]ModelThinkingConfig{
			"deepseek-v4-pro":   {Style: ThinkingStyleObject},
			"deepseek-v4-flash": {Style: ThinkingStyleObject},
			// MiniMax on DashScope uses thinking.type, not enable_thinking (see supplier report).
			"minimax-m2.5":   {Style: ThinkingStyleObject, Policy: ThinkingPolicyAlwaysOn},
			"kimi-k2.7-code": {Policy: ThinkingPolicyAlwaysOn},
		},
		Models: []string{
			"qwen3.7-max",
			"qwen3.7-plus",
			"qwen3.6-plus",
			"qwen3.6-flash",
			"deepseek-v4-pro",
			"deepseek-v4-flash",
			"kimi-k2.7-code",
			"kimi-k2.6",
			"kimi-k2.5",
			"glm-5.2",
			"glm-5.1",
			"glm-5",
			"MiniMax-M2.5",
		},
	},
	{
		Name:        "volcengine",
		DisplayName: "Volcano Engine Ark API",
		Protocol:    "openai",
		BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
		EnvVar:      "ARK_API_KEY",
		// All doubao-seed models use thinking.type encoding (ThinkingStyleObject).
		ThinkingStyle: ThinkingStyleObject,
		Models: []string{
			"doubao-seed-evolving",
			"doubao-seed-2-1-pro-260628",
			"doubao-seed-2-1-turbo-260628",
			"doubao-seed-2-0-lite-260428",
			"doubao-seed-2-0-mini-260428",
			"doubao-seed-2-0-pro-260215",
		},
	},
	{
		Name:          "deepseek",
		DisplayName:   "DeepSeek API",
		Protocol:      "openai",
		BaseURL:       "https://api.deepseek.com",
		EnvVar:        "DEEPSEEK_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		Models: []string{
			"deepseek-v4-pro",
			"deepseek-v4-flash",
		},
	},
	{
		Name:          "tencent-tokenhub",
		DisplayName:   "Tencent TokenHub API",
		Protocol:      "openai",
		BaseURL:       "https://tokenhub.tencentmaas.com/v1",
		EnvVar:        "TENCENT_TOKENHUB_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		ModelThinking: map[string]ModelThinkingConfig{
			"kimi-k2.7-code": {Policy: ThinkingPolicyAlwaysOn},
			"minimax-m2.7":   {Policy: ThinkingPolicyAlwaysOn},
			"minimax-m2.5":   {Policy: ThinkingPolicyAlwaysOn},
			"minimax-m3":     {ObjectOn: "adaptive"},
		},
		Models: []string{
			"hy3",
			"hy3-preview",
			"deepseek-v4-pro",
			"deepseek-v4-flash",
			"glm-5.2",
			"glm-5.1",
			"glm-5",
			"glm-5-turbo",
			"kimi-k2.7-code",
			"kimi-k2.6",
			"kimi-k2.5",
			"minimax-m3",
			"minimax-m2.7",
			"minimax-m2.5",
		},
	},
	{
		Name:          "hy-tokenplan",
		DisplayName:   "Tencent Hunyuan Token Plan API",
		Protocol:      "openai",
		BaseURL:       "https://api.lkeap.cloud.tencent.com/plan/v3",
		EnvVar:        "TENCENT_HUNYUAN_TOKENPLAN_KEY",
		ThinkingStyle: ThinkingStyleObject,
		Models: []string{
			"hy3",
			"hy3-preview",
		},
	},
	{
		Name:          "kimi",
		DisplayName:   "Kimi Moonshot API",
		Protocol:      "openai",
		BaseURL:       "https://api.moonshot.cn/v1",
		EnvVar:        "MOONSHOT_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		ModelThinking: map[string]ModelThinkingConfig{
			"kimi-k2.7-code":           {Policy: ThinkingPolicyAlwaysOn},
			"kimi-k2.7-code-highspeed": {Policy: ThinkingPolicyAlwaysOn},
		},
		Models: []string{
			"kimi-k2.7-code",
			"kimi-k2.7-code-highspeed",
			"kimi-k2.6",
			"kimi-k2.5",
		},
	},
	{
		Name:          "z-ai",
		DisplayName:   "Z.AI API",
		Protocol:      "openai",
		BaseURL:       "https://open.bigmodel.cn/api/paas/v4",
		EnvVar:        "Z_AI_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		Models: []string{
			"glm-5.2",
			"glm-5.1",
			"glm-5",
			"glm-5-turbo",
			"glm-4.7",
		},
	},
	{
		Name:          "z-ai-coding",
		DisplayName:   "Z.AI Coding Plan API",
		Protocol:      "openai",
		BaseURL:       "https://open.bigmodel.cn/api/coding/paas/v4",
		EnvVar:        "Z_AI_CODING_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		Models: []string{
			"glm-5.2",
			"glm-5.1",
			"glm-5-turbo",
			"glm-4.7",
		},
	},
	{
		Name:          "mimo",
		DisplayName:   "Xiaomi MiMo API",
		Protocol:      "openai",
		BaseURL:       "https://api.xiaomimimo.com/v1",
		EnvVar:        "MIMO_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		Models: []string{
			"mimo-v2.5-pro",
			"mimo-v2.5",
		},
	},
	{
		Name:          "minimax",
		DisplayName:   "MiniMax API",
		Protocol:      "openai",
		BaseURL:       "https://api.minimaxi.com/v1",
		EnvVar:        "MINIMAX_API_KEY",
		ThinkingStyle: ThinkingStyleObject,
		ModelThinking: map[string]ModelThinkingConfig{
			"minimax-m3":             {ObjectOn: "adaptive"},
			"minimax-m2.7":           {Policy: ThinkingPolicyAlwaysOn},
			"minimax-m2.7-highspeed": {Policy: ThinkingPolicyAlwaysOn},
			"minimax-m2.5":           {Policy: ThinkingPolicyAlwaysOn},
			"minimax-m2.5-highspeed": {Policy: ThinkingPolicyAlwaysOn},
		},
		Models: []string{
			"MiniMax-M3",
			"MiniMax-M2.7",
			"MiniMax-M2.7-highspeed",
			"MiniMax-M2.5",
			"MiniMax-M2.5-highspeed",
		},
	},
	{
		Name:          "baidu-qianfan",
		DisplayName:   "Baidu Qianfan API",
		Protocol:      "openai",
		BaseURL:       "https://qianfan.baidubce.com/v2",
		EnvVar:        "QIANFAN_API_KEY",
		ThinkingStyle: ThinkingStyleEnableBool,
		ModelThinking: map[string]ModelThinkingConfig{
			"deepseek-v4-pro":   {Style: ThinkingStyleObject},
			"deepseek-v4-flash": {Style: ThinkingStyleObject},
			"glm-5.2":           {Style: ThinkingStyleObject},
			"glm-5.1":           {Style: ThinkingStyleObject},
			"glm-5":             {Style: ThinkingStyleObject},
			"kimi-k2.6":         {Style: ThinkingStyleObject},
		},
		Models: []string{
			"ernie-5.1",
			"ernie-5.0",
			"ernie-x1.1",
			"ernie-x1-turbo-32k-preview",
			"deepseek-v4-pro",
			"deepseek-v4-flash",
			"glm-5.2",
			"glm-5.1",
			"glm-5",
			"kimi-k2.6",
		},
	},
}

var registryMap map[string]Provider

func init() {
	registryMap = make(map[string]Provider, len(registry))
	thinkingProviderIndex = make(map[string]thinkingProviderMeta, len(registry))
	for _, p := range registry {
		key := strings.ToLower(p.Name)
		registryMap[key] = p
		meta := thinkingProviderMeta{
			defaultStyle: p.ThinkingStyle,
		}
		if len(p.ModelThinking) > 0 {
			meta.modelOverrides = make(map[string]ModelThinkingConfig, len(p.ModelThinking))
			for model, cfg := range p.ModelThinking {
				meta.modelOverrides[strings.ToLower(model)] = cfg
			}
		}
		thinkingProviderIndex[key] = meta
	}
}

// LookupProvider returns the preset provider by name.
// The returned Provider has its own copy of the Models slice.
func LookupProvider(name string) (Provider, bool) {
	p, ok := registryMap[strings.ToLower(strings.TrimSpace(name))]
	if ok {
		p = copyProvider(p)
	}
	return p, ok
}

// ListProviders returns all built-in providers sorted by provider name.
// Each returned Provider has its own copy of the Models slice in registry order.
func ListProviders() []Provider {
	out := make([]Provider, len(registry))
	for i, p := range registry {
		out[i] = copyProvider(p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func copyProvider(p Provider) Provider {
	if p.Models != nil {
		models := make([]string, len(p.Models))
		copy(models, p.Models)
		p.Models = models
	}
	if p.ModelThinking != nil {
		// Isolated copy for registry introspection; runtime resolution uses thinkingProviderIndex.
		p.ModelThinking = maps.Clone(p.ModelThinking)
	}
	return p
}
