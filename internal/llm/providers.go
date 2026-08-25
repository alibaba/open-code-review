// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
	Models      []string

	// AmbientAuth marks a provider whose credentials come from the
	// environment's own chain rather than an api_key — AWS SigV4, for
	// instance. The resolver skips its api_key requirement for these, because
	// there is no key to configure and demanding one would make the provider
	// impossible to use.
	AmbientAuth bool
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
			"claude-opus-5",
			"claude-sonnet-5",
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
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
			"us.anthropic.claude-opus-5",
			"us.anthropic.claude-sonnet-5",
			"us.anthropic.claude-opus-4-8",
			"us.anthropic.claude-opus-4-7",
			"us.anthropic.claude-sonnet-4-6",
			"global.anthropic.claude-opus-5",
			"global.anthropic.claude-sonnet-5",
			"global.anthropic.claude-opus-4-8",
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
		Name:        "openai-responses",
		DisplayName: "OpenAI Responses API",
		Protocol:    ProtocolOpenAIResponses,
		BaseURL:     "https://api.openai.com/v1",
		EnvVar:      "OPENAI_RESPONSES_API_KEY",
		Models: []string{
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
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
		Name:        "gemini",
		DisplayName: "Google Gemini API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://generativelanguage.googleapis.com/v1beta/openai",
		EnvVar:      "GEMINI_API_KEY",
		Models: []string{
			"gemini-3-flash-preview",
			"gemini-3.1-flash-lite",
			"gemini-3.1-pro",
			"gemini-3.5-flash-lite",
			"gemini-3.5-flash",
			"gemini-3.6-flash",
		},
	},
	{
		Name:        "dashscope",
		DisplayName: "Alibaba DashScope API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvVar:      "DASHSCOPE_API_KEY",
		Models: []string{
			"qwen3.8-max",
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
			"qwen3.8-max",
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
			"deepseek-v4-pro",
			"deepseek-v4-flash",
			"glm-5.2",
			"glm-5.1",
			"glm-5",
			"glm-5-turbo",
			"kimi-k2.7-code",
			"kimi-k2.7-code-highspeed",
			"kimi-k2.6",
			"minimax-m3",
			"minimax-m2.7",
		},
	},
	{
		Name:        "hy-tokenplan",
		DisplayName: "Tencent Hunyuan Token Plan API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.lkeap.cloud.tencent.com/plan/v3",
		EnvVar:      "TENCENT_HUNYUAN_TOKENPLAN_KEY",
		Models: []string{
			"hy3",
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
			"generalv3.5",
			"max-32k",
			"generalv3",
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
			"kimi-k3",
			"kimi-k2.7-code",
			"kimi-k2.7-code-highspeed",
			"kimi-k2.6",
			"kimi-k2.5",
		},
	},
	{
		Name:        "kimi-global",
		DisplayName: "Kimi Moonshot API (Global)",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.moonshot.ai/v1",
		EnvVar:      "MOONSHOT_GLOBAL_API_KEY",
		Models: []string{
			"kimi-k3",
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
			"glm-5.3",
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
		BaseURL:     "https://api.minimax.io/v1",
		EnvVar:      "MINIMAX_GLOBAL_API_KEY",
		Models: []string{
			"MiniMax-M3",
			"MiniMax-M2.7",
			"MiniMax-M2.7-highspeed",
			"MiniMax-M2.5",
			"MiniMax-M2.5-highspeed",
		},
	},
	{
		Name:        "minimax-cn",
		DisplayName: "MiniMax CN API",
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
	{
		Name:        "ollama-cloud",
		DisplayName: "Ollama Cloud API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://ollama.com/v1",
		EnvVar:      "OLLAMA_API_KEY",
		Models: []string{
			"deepseek-v4-flash",
			"deepseek-v4-pro",
			"gemma4:31b",
			"glm-5.1",
			"glm-5.2",
			"gpt-oss:120b",
			"gpt-oss:20b",
			"kimi-k2.5",
			"kimi-k2.6",
			"kimi-k2.7-code",
			"minimax-m2.5",
			"minimax-m2.7",
			"minimax-m3",
			"mistral-large-3:675b",
			"nemotron-3-nano:30b",
			"nemotron-3-super",
			"nemotron-3-ultra",
			"qwen3.5:397b",
		},
	},
	{
		Name:        "novita",
		DisplayName: "Novita API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.novita.ai/openai",
		EnvVar:      "NOVITA_API_KEY",
		Models: []string{
			"moonshotai/kimi-k3",
			"zai-org/glm-5.2",
			"deepseek/deepseek-v4-flash-0731",
		},
	},
	{
		Name:        "xai",
		DisplayName: "xAI Grok API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.x.ai/v1",
		EnvVar:      "XAI_API_KEY",
		Models: []string{
			"grok-4.6",
			"grok-4.5",
			"grok-4.3",
		},
	},
	{
		Name:        "litellm",
		DisplayName: "LiteLLM AI Gateway",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "http://localhost:4000/v1",
		EnvVar:      "LITELLM_API_KEY",
		Models: []string{
			"anthropic/claude-sonnet-4-6",
			"anthropic/claude-opus-4-6",
			"anthropic/claude-haiku-4-5",
			"openai/gpt-4o",
			"openai/gpt-5.4",
			"openai/o3",
			"vertex_ai/gemini-2.5-flash",
			"vertex_ai/gemini-2.5-pro",
			"bedrock/anthropic.claude-sonnet-4-6-v1",
			"groq/llama-4-scout-17b-16e-instruct",
			"mistral/mistral-large-latest",
			"deepseek/deepseek-chat",
		},
	},
	{
		Name:        "siliconflow",
		DisplayName: "SiliconFlow API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.siliconflow.com/v1",
		EnvVar:      "SILICONFLOW_GLOBAL_API_KEY",
		Models: []string{
			"deepseek-ai/DeepSeek-V4-Pro",
			"deepseek-ai/DeepSeek-V4-Flash",
			"Qwen/Qwen3.6-27B",
			"moonshotai/Kimi-K2.7-Code",
			"zai-org/GLM-5.2",
		},
	},
	{
		Name:        "siliconflow-cn",
		DisplayName: "SiliconFlow CN API",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.siliconflow.cn/v1",
		EnvVar:      "SILICONFLOW_API_KEY",
		Models: []string{
			"deepseek-ai/DeepSeek-V4-Pro",
			"deepseek-ai/DeepSeek-V4-Flash",
			"Qwen/Qwen3.6-27B",
			"moonshotai/Kimi-K2.7-Code",
			"zai-org/GLM-5.2",
		},
	},
	{
		Name:        "mistral",
		DisplayName: "Mistral AI",
		Protocol:    ProtocolOpenAIChatCompletions,
		BaseURL:     "https://api.mistral.ai/v1",
		EnvVar:      "MISTRAL_API_KEY",
		// Deliberately minimal list to keep this preset low-maintenance for
		// alibaba/open-code-review maintainers. Users can point to any other
		// Mistral model via `ocr config set model <name>`; the preset only
		// seeds the picker UI. See https://docs.mistral.ai/getting-started/models/models_overview/
		Models: []string{
			"codestral-latest",
			"mistral-large-latest",
			"mistral-small-latest",
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
