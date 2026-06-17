package llm

import (
	"sort"
	"strings"
)

// Provider holds the preset configuration for a known LLM provider.
type Provider struct {
	Name        string
	DisplayName string
	Protocol    string // "anthropic" or "openai"
	BaseURL     string
	AuthHeader  string // Anthropic-only; empty for OpenAI-compatible
	EnvVar      string // environment variable name for API key fallback
	Models      []string
}

var registry = []Provider{
	{
		Name:        "anthropic",
		DisplayName: "Anthropic Claude API",
		Protocol:    "anthropic",
		BaseURL:     "https://api.anthropic.com",
		AuthHeader:  "x-api-key",
		EnvVar:      "ANTHROPIC_API_KEY",
		Models: []string{
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
		},
	},
	{
		Name:        "openai",
		DisplayName: "OpenAI API",
		Protocol:    "openai",
		BaseURL:     "https://api.openai.com/v1",
		EnvVar:      "OPENAI_API_KEY",
		Models: []string{
			"gpt-5.5",
			"gpt-5.4",
			"gpt-5.4-mini",
		},
	},
	{
		Name:        "dashscope",
		DisplayName: "Alibaba DashScope API",
		Protocol:    "openai",
		BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvVar:      "DASHSCOPE_API_KEY",
		Models: []string{
			"qwen3.7-max",
			"qwen3.7-plus",
			"qwen3.6-plus",
			"qwen3.6-flash",
		},
	},
	{
		Name:        "volcengine",
		DisplayName: "Volcano Engine Ark API",
		Protocol:    "openai",
		BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
		EnvVar:      "ARK_API_KEY",
		Models: []string{
			"doubao-seed-2-0-lite-260428",
			"doubao-seed-2-0-mini-260428",
			"doubao-seed-2-0-pro-260215",
		},
	},
	{
		Name:        "deepseek",
		DisplayName: "DeepSeek API",
		Protocol:    "openai",
		BaseURL:     "https://api.deepseek.com",
		EnvVar:      "DEEPSEEK_API_KEY",
		Models: []string{
			"deepseek-v4-pro",
			"deepseek-v4-flash",
		},
	},
	{
		Name:        "tencent-tokenhub",
		DisplayName: "Tencent TokenHub API",
		Protocol:    "openai",
		BaseURL:     "https://tokenhub.tencentmaas.com/v1",
		EnvVar:      "TENCENT_TOKENHUB_API_KEY",
		Models: []string{
			"hy3-preview",
		},
	},
	{
		Name:        "hy-tokenplan",
		DisplayName: "Tencent Hunyuan Token Plan API",
		Protocol:    "openai",
		BaseURL:     "https://api.lkeap.cloud.tencent.com/plan/v3",
		EnvVar:      "TENCENT_HUNYUAN_TOKENPLAN_KEY",
		Models: []string{
			"hy3-preview",
		},
	},
	{
		Name:        "kimi",
		DisplayName: "Kimi Moonshot API",
		Protocol:    "openai",
		BaseURL:     "https://api.moonshot.cn/v1",
		EnvVar:      "MOONSHOT_API_KEY",
		Models: []string{
			"kimi-k2.7-code",
			"kimi-k2.6",
			"kimi-k2.5",
		},
	},
	{
		Name:        "z-ai",
		DisplayName: "Z.AI API",
		Protocol:    "openai",
		BaseURL:     "https://open.bigmodel.cn/api/paas/v4",
		EnvVar:      "Z_AI_API_KEY",
		Models: []string{
			"glm-5.2",
			"glm-5.1",
			"glm-5-turbo",
			"glm-4.7",
		},
	},
	{
		Name:        "mimo",
		DisplayName: "Xiaomi MiMo API",
		Protocol:    "openai",
		BaseURL:     "https://api.xiaomimimo.com/v1",
		EnvVar:      "MIMO_API_KEY",
		Models: []string{
			"mimo-v2.5-pro",
			"mimo-v2.5",
			"mimo-v2-pro",
			"mimo-v2-omni",
			"mimo-v2-flash",
		},
	},
	{
		Name:        "minimax",
		DisplayName: "MiniMax API",
		Protocol:    "openai",
		BaseURL:     "https://api.minimaxi.com/v1",
		EnvVar:      "MINIMAX_API_KEY",
		Models: []string{
			"MiniMax-M3",
			"MiniMax-M2.7",
			"MiniMax-M2.7-highspeed",
			"MiniMax-M2.5",
			"MiniMax-M2.5-highspeed",
		},
	},
}

var registryMap map[string]Provider

func init() {
	registryMap = make(map[string]Provider, len(registry))
	for _, p := range registry {
		registryMap[strings.ToLower(p.Name)] = p
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
	return p
}
