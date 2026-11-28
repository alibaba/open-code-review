// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

//go:generate go run ./gen -output ../../extensions/frontend/src/shared/providers.generated.ts -kotlin-output ../../extensions/idea/src/main/kotlin/com/alibaba/opencodereview/idea/services/ProviderNames.generated.kt

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
//   - ProtocolAnthropicBedrock ("anthropic-bedrock")
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

	// AmbientAuth marks a provider whose credentials come from the
	// environment's own chain rather than an api_key — AWS SigV4, for
	// instance. The resolver skips its api_key requirement for these, because
	// there is no key to configure and demanding one would make the provider
	// impossible to use.
	AmbientAuth           bool
	ExternalAuth          bool
	RequiresStreaming     bool
	RejectsSamplingParams bool
	Models                []string // Suggested IDs for the model picker
}

// After modifying the built-in provider registry, run `go generate ./internal/llm`
// and commit both generated catalogs. See the root AGENTS.md for paths and verification.
var registry = []Provider{
	{
		Name:        "anthropic",
		DisplayName: "Anthropic Claude API",
		Protocol:    ProtocolAnthropic,
		BaseURL:     "https://api.anthropic.com",
		AuthHeader:  "x-api-key",
		EnvVar:      "ANTHROPIC_API_KEY",
		Models: []string{
			"claude-fable-5-2",
			"claude-opus-5-5",
			"claude-sonnet-5-5",
			"claude-haiku-5-5",
		},
	},
	{
		// Bedrock takes no api_key and no base URL: the SDK's bedrock
		// middleware derives the host from the resolved AWS region and signs
		// each request from the ambient credential chain (profile, SSO,
		// instance role, or AWS_* variables). Set AWS_REGION or AWS_PROFILE the
		// way any other AWS tool expects.
		//
		// Model accepts anything Bedrock will route: a foundation model ID, an
		// inference profile ID, or the ARN of an application inference profile
		// when usage needs to be attributed for cost allocation. Run
		// `aws bedrock list-inference-profiles` to see what an account offers —
		// IDs differ per account and per region, so the list below is only a
		// starting point.
		Name:        "bedrock",
		DisplayName: "AWS Bedrock (Anthropic models)",
		Protocol:    ProtocolAnthropicBedrock,
		AmbientAuth: true,
		Models: []string{
			"us.anthropic.claude-opus-5-5",
			"us.anthropic.claude-sonnet-5-5",
			"global.anthropic.claude-fable-5-2",
			"global.anthropic.claude-opus-5-5",
			"global.anthropic.claude-sonnet-5-5",
			"global.anthropic.claude-haiku-5-5",
		},
	},
	{
		Name:                  "chatgpt",
		DisplayName:           "ChatGPT plan (Sign in with ChatGPT preview)",
		Protocol:              ProtocolOpenAIResponses,
		BaseURL:               "https://api.openai.com/v1",
		ExternalAuth:          true,
		RequiresStreaming:     true,
		RejectsSamplingParams: true,
		Models:                []string{},
	},
	{
		Name:        "openai",
		DisplayName: "OpenAI API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.openai.com/v1",
		EnvVar:      "OPENAI_API_KEY",
		// GPT-6 needs Responses for tool calls at its default reasoning settings.
		Models: []string{
			"gpt-5.5",
		},
	},
	{
		Name:        "openai-responses",
		DisplayName: "OpenAI Responses API",
		Protocol:    ProtocolOpenAIResponses,
		BaseURL:     "https://api.openai.com/v1",
		EnvVar:      "OPENAI_RESPONSES_API_KEY",
		Models: []string{
			"gpt-6.2-astra",
			"gpt-6.2-sol",
			"gpt-6.1-sol",
			"gpt-6.1-terra",
			"gpt-6-luna",
		},
	},
	{
		Name:        "openrouter",
		DisplayName: "OpenRouter",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://openrouter.ai/api/v1",
		EnvVar:      "OPENROUTER_API_KEY",
		Models: []string{
			"anthropic/claude-fable-5.2",
			"openai/gpt-6.2-sol",
			"google/gemini-4-neon",
			"moonshotai/kimi-k3.1-pro",
			"deepseek/deepseek-v4.1-flash-1017",
		},
	},
	{
		Name:        "edenai",
		DisplayName: "Eden AI",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.edenai.run/v3",
		EnvVar:      "EDENAI_API_KEY",
		Models: []string{
			"anthropic/claude-opus-5-5",
			"openai/gpt-6.2-sol",
			"google/gemini-4-neon",
			"moonshot/kimi-k3.1-pro",
			"deepseek/deepseek-v4-pro",
		},
	},
	{
		Name:        "gemini",
		DisplayName: "Google Gemini API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://generativelanguage.googleapis.com/v1beta/openai",
		EnvVar:      "GEMINI_API_KEY",
		Models: []string{
			"gemini-4-carbon",
			"gemini-4-neon",
			"gemini-4-hydrogen",
			"gemini-4-argon",
		},
	},
	{
		Name:        "dashscope",
		DisplayName: "Alibaba DashScope API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvVar:      "DASHSCOPE_API_KEY",
		Models: []string{
			"qwen4-max",
			"qwen3.8-flash",
			"deepseek-v4.1-flash",
			"kimi-k3.1-pro",
			"glm-5.3",
		},
	},
	{
		Name:        "dashscope-tokenplan",
		DisplayName: "Alibaba DashScope Token Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		EnvVar:      "DASHSCOPE_TOKENPLAN_KEY",
		Models: []string{
			"qwen4-max",
			"qwen3.8-flash",
			"deepseek-v4.1-flash-1017",
			"deepseek-v4-pro-0813",
			"glm-5.3",
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
			"doubao-seed-2-1-pro-260915",
			"doubao-seed-2-1-lite-260915",
			"doubao-seed-2-1-turbo-260628",
			"kimi-k3-1-pro-261024",
			"deepseek-v4-1-flash-261017",
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
			"deepseek-flash",
		},
	},
	{
		Name:        "tencent-tokenhub",
		DisplayName: "Tencent TokenHub API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://tokenhub.tencentmaas.com/v1",
		EnvVar:      "TENCENT_TOKENHUB_API_KEY",
		Models: []string{
			"hy4",
			"hy4-flash",
			"deepseek/deepseek-flash",
			"kimi-k3.1-pro",
			"minimax-m3.1-flash",
			"glm-5.3",
		},
	},
	{
		Name:        "hy-tokenplan",
		DisplayName: "Tencent Hunyuan Token Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://tokenhub.tencentmaas.com/plan/v3",
		EnvVar:      "TENCENT_HUNYUAN_TOKENPLAN_KEY",
		Models: []string{
			"hy4",
			"hy4-flash",
		},
	},
	{
		Name:        "iflytek",
		DisplayName: "iFlytek Spark API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://spark-api-open.xf-yun.com/v1",
		EnvVar:      "SPARK_API_KEY",
		Models: []string{
			"4.0Ultra",
			"pro-128k",
			"lite",
		},
	},
	{
		Name:        "kimi",
		DisplayName: "Kimi Moonshot API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.moonshot.cn/v1",
		EnvVar:      "MOONSHOT_API_KEY",
		Models: []string{
			"kimi-k3.1-pro",
			"kimi-k3.1-pro-highspeed",
			"kimi-k3",
		},
	},
	{
		Name:        "kimi-global",
		DisplayName: "Kimi Moonshot API (Global)",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.moonshot.ai/v1",
		EnvVar:      "MOONSHOT_GLOBAL_API_KEY",
		Models: []string{
			"kimi-k3.1-pro",
			"kimi-k3.1-pro-highspeed",
			"kimi-k3",
		},
	},
	{
		Name:        "z-ai",
		DisplayName: "Z.AI API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://open.bigmodel.cn/api/paas/v4",
		EnvVar:      "Z_AI_API_KEY",
		Models: []string{
			"glm-5.3",
			"glm-5.3-flash",
			"glm-5.3-flashx",
		},
	},
	{
		Name:        "z-ai-coding",
		DisplayName: "Z.AI Coding Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://open.bigmodel.cn/api/coding/paas/v4",
		EnvVar:      "Z_AI_CODING_API_KEY",
		Models: []string{
			"glm-5.4",
			"glm-5.3",
			"glm-5.3-flash",
		},
	},
	{
		Name:        "mimo",
		DisplayName: "Xiaomi MiMo API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.xiaomimimo.com/v1",
		EnvVar:      "MIMO_API_KEY",
		Models: []string{
			"mimo-v2.6-pro",
			"mimo-v2.6-pro-ultraspeed",
			"mimo-v2.6-flash",
		},
	},
	{
		Name:        "minimax",
		DisplayName: "MiniMax API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.minimax.io/v1",
		EnvVar:      "MINIMAX_GLOBAL_API_KEY",
		Models: []string{
			"MiniMax-M3.1-Flash",
			"MiniMax-M3",
		},
	},
	{
		Name:        "minimax-cn",
		DisplayName: "MiniMax CN API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.minimaxi.com/v1",
		EnvVar:      "MINIMAX_API_KEY",
		Models: []string{
			"MiniMax-M3.1-Flash",
			"MiniMax-M3",
		},
	},
	{
		Name:        "baidu-qianfan",
		DisplayName: "Baidu Qianfan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://qianfan.baidubce.com/v2",
		EnvVar:      "QIANFAN_API_KEY",
		Models: []string{
			"ernie-5.5-flash-preview",
			"ernie-5.1",
			"ernie-light-dt-1.0",
			"deepseek-v4.1-flash-1017",
			"deepseek-v4-pro-0813",
			"glm-5.3",
		},
	},
	{
		Name:        "ollama-cloud",
		DisplayName: "Ollama Cloud API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://ollama.com/v1",
		EnvVar:      "OLLAMA_API_KEY",
		Models: []string{
			"deepseek-v4.1-flash",
			"deepseek-v4-pro:0813",
			"kimi-k3.1-pro",
			"glm-5.3-flash",
			"minimax-m3.1-flash",
			"mistral-large-4",
		},
	},
	{
		Name:        "novita",
		DisplayName: "Novita API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.novita.ai/openai",
		EnvVar:      "NOVITA_API_KEY",
		Models: []string{
			"deepseek/deepseek-v4.1-flash-1017",
			"moonshotai/kimi-k3.1-pro",
			"qwen/qwen4-max",
			"zai-org/glm-5.3",
			"tencent/hy4",
		},
	},
	{
		Name:        "xai",
		DisplayName: "xAI Grok API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.x.ai/v1",
		EnvVar:      "XAI_API_KEY",
		Models: []string{
			"grok-4.7",
		},
	},
	{
		Name:        "litellm",
		DisplayName: "LiteLLM AI Gateway",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "http://localhost:4000/v1",
		EnvVar:      "LITELLM_API_KEY",
		// The proxy configuration determines which aliases it serves.
		Models: []string{
			"anthropic/claude-sonnet-5-5",
			"openai/gpt-5.5",
			"vertex_ai/gemini-4-neon",
			"mistral/mistral-large-latest",
			"deepseek/deepseek-flash",
		},
	},
	{
		Name:        "siliconflow",
		DisplayName: "SiliconFlow API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.siliconflow.com/v1",
		EnvVar:      "SILICONFLOW_GLOBAL_API_KEY",
		Models: []string{
			"deepseek-ai/DeepSeek-V4.1-Flash",
			"zai-org/GLM-5.3",
			"moonshotai/Kimi-K3.1-Pro",
			"Qwen/Qwen3.8-27B",
			"MiniMaxAI/MiniMax-M3.1-Flash",
		},
	},
	{
		Name:        "siliconflow-cn",
		DisplayName: "SiliconFlow CN API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.siliconflow.cn/v1",
		EnvVar:      "SILICONFLOW_API_KEY",
		Models: []string{
			"deepseek-ai/DeepSeek-V4.1-Flash",
			"deepseek-ai/DeepSeek-V4-Pro",
			"zai-org/GLM-5.3-Flash",
			"Qwen/Qwen3.8-27B",
		},
	},
	{
		Name:        "mistral",
		DisplayName: "Mistral AI",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.mistral.ai/v1",
		EnvVar:      "MISTRAL_API_KEY",
		Models: []string{
			"mistral-large-latest",
			"mistral-small-latest",
			"ministral-14b-latest",
			"zai-glm-5-3",
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
