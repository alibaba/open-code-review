package llm

import (
	"sort"
	"strings"
)

// Provider holds the preset configuration for a known LLM provider.
//
// Protocol uses the canonical names defined in protocol.go:
//   - ProtocolAnthropic ("anthropic")
//   - ProtocolOpenAIChatCompletions ("openai")
//   - ProtocolOpenAIResponses ("openai-responses")
//
// To add a built-in provider that speaks a different protocol, set Protocol
// accordingly and ensure NewLLMClient has a matching case.
type Provider struct {
	Name        string
	DisplayName string
	Protocol    string
	BaseURL     string
	AuthHeader  string // Anthropic-only; empty for OpenAI-compatible
	EnvVar      string // environment variable name for API key fallback
	Models      []string
}

var registry = []Provider{
	{
		Name:        "anthropic",
		DisplayName: "Anthropic Claude API",
		Protocol:    ProtocolAnthropic,
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
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.openai.com/v1",
		EnvVar:      "OPENAI_API_KEY",
		Models: []string{
			"gpt-5.5",
			"gpt-5.4",
			"gpt-5.4-mini",
		},
	},
	{
		Name:        "edenai",
		DisplayName: "Eden AI",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.edenai.run/v3",
		EnvVar:      "EDENAI_API_KEY",
		Models: []string{
			"anthropic/claude-opus-4-5",
			"anthropic/claude-sonnet-4-5",
			"anthropic/claude-haiku-4-5",
			"openai/gpt-5.1",
			"openai/gpt-5.1-codex",
			"google/gemini-3.1-pro-preview",
			"mistral/devstral-medium-latest",
			"mistral/codestral-latest",
			"deepseek/deepseek-v4-pro",
			"xai/grok-4",
		},
	},
	{
		Name:        "dashscope",
		DisplayName: "Alibaba DashScope API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvVar:      "DASHSCOPE_API_KEY",
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
		Name:        "dashscope-tokenplan",
		DisplayName: "Alibaba DashScope Token Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		EnvVar:      "DASHSCOPE_TOKENPLAN_KEY",
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
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
		EnvVar:      "ARK_API_KEY",
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
		Name:        "deepseek",
		DisplayName: "DeepSeek API",
		Protocol:    ProtocolOpenAIChatCompletions,
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
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://tokenhub.tencentmaas.com/v1",
		EnvVar:      "TENCENT_TOKENHUB_API_KEY",
		Models: []string{
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
		Name:        "hy-tokenplan",
		DisplayName: "Tencent Hunyuan Token Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.lkeap.cloud.tencent.com/plan/v3",
		EnvVar:      "TENCENT_HUNYUAN_TOKENPLAN_KEY",
		Models: []string{
			"hy3-preview",
		},
	},
	{
		Name:        "kimi",
		DisplayName: "Kimi Moonshot API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.moonshot.cn/v1",
		EnvVar:      "MOONSHOT_API_KEY",
		Models: []string{
			"kimi-k2.7-code",
			"kimi-k2.7-code-highspeed",
			"kimi-k2.6",
			"kimi-k2.5",
		},
	},
	{
		Name:        "z-ai",
		DisplayName: "Z.AI API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://open.bigmodel.cn/api/paas/v4",
		EnvVar:      "Z_AI_API_KEY",
		Models: []string{
			"glm-5.2",
			"glm-5.1",
			"glm-5",
			"glm-5-turbo",
			"glm-4.7",
		},
	},
	{
		Name:        "z-ai-coding",
		DisplayName: "Z.AI Coding Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://open.bigmodel.cn/api/coding/paas/v4",
		EnvVar:      "Z_AI_CODING_API_KEY",
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
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.xiaomimimo.com/v1",
		EnvVar:      "MIMO_API_KEY",
		Models: []string{
			"mimo-v2.5-pro",
			"mimo-v2.5",
		},
	},
	{
		Name:        "minimax",
		DisplayName: "MiniMax API",
		Protocol:    ProtocolOpenAIChatCompletions,
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
	{
		Name:        "baidu-qianfan",
		DisplayName: "Baidu Qianfan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://qianfan.baidubce.com/v2",
		EnvVar:      "QIANFAN_API_KEY",
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
