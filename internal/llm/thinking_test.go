package llm

import (
	"strings"
	"testing"
)

func TestTranslateThinking_Object_Off(t *testing.T) {
	body := translateThinking("off", "deepseek", "deepseek-v4-pro")
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("body = %#v, want thinking.type disabled", body)
	}
}

func TestTranslateThinking_Object_On(t *testing.T) {
	body := translateThinking("on", "deepseek", "deepseek-v4-pro")
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("body = %#v, want thinking.type enabled", body)
	}
}

func TestTranslateThinking_Bool_Off(t *testing.T) {
	body := translateThinking("off", "dashscope", "qwen3.7-max")
	if body["enable_thinking"] != false {
		t.Fatalf("body = %#v, want enable_thinking false", body)
	}
}

func TestTranslateThinking_Bool_On(t *testing.T) {
	body := translateThinking("on", "dashscope", "qwen3.7-max")
	if body["enable_thinking"] != true {
		t.Fatalf("body = %#v, want enable_thinking true", body)
	}
}

func TestTranslateThinking_NotSupported(t *testing.T) {
	if body := translateThinking("off", "openai", "gpt-5.5"); body != nil {
		t.Fatalf("body = %#v, want nil", body)
	}
}

func TestTranslateThinking_AlwaysOn(t *testing.T) {
	if body := translateThinking("off", "kimi", "kimi-k2.7-code"); body != nil {
		t.Fatalf("body = %#v, want nil for always-on model", body)
	}
}

func TestCanDisableThinking(t *testing.T) {
	if CanDisableThinking("anthropic", "claude-opus-4-6") {
		t.Error("anthropic claude should not allow disable (not_supported)")
	}
	if CanDisableThinking("kimi", "kimi-k2.7-code") {
		t.Error("kimi-k2.7-code should not allow disable")
	}
	if CanDisableThinking("dashscope", "kimi-k2.7-code") {
		t.Error("dashscope kimi-k2.7-code passthrough should not allow disable")
	}
	if CanDisableThinking("dashscope", "MiniMax-M2.5") {
		t.Error("dashscope MiniMax-M2.5 passthrough should not allow disable")
	}
	if CanDisableThinking("openai", "gpt-5.5") {
		t.Error("openai should not allow disable")
	}
	if !CanDisableThinking("mimo", "mimo-v2.5-pro") {
		t.Error("mimo should allow disable")
	}
	if !CanDisableThinking("baidu-qianfan", "kimi-k2.6") {
		t.Error("qianfan kimi-k2.6 should allow disable (thinking_object)")
	}
	if !CanDisableThinking("baidu-qianfan", "glm-5.2") {
		t.Error("qianfan glm-5.2 should allow disable")
	}
}

func TestIsThinkingSupported_EmptyModelName(t *testing.T) {
	if IsThinkingSupported("deepseek", "") {
		t.Fatal("empty model name should not be thinking-supported")
	}
	if IsThinkingSupported("dashscope", "  ") {
		t.Fatal("whitespace model name should not be thinking-supported")
	}
}

func TestLookupThinkingStyle_EmptyModelName(t *testing.T) {
	if got := lookupThinkingStyle("deepseek", ""); got != ThinkingStyleNotSupported {
		t.Fatalf("empty model = %q, want NotSupported", got)
	}
	if got := lookupThinkingStyle("deepseek", "  "); got != ThinkingStyleNotSupported {
		t.Fatalf("whitespace model = %q, want NotSupported", got)
	}
}

func TestMergeExtraBody_EmptyMaps(t *testing.T) {
	if got := mergeExtraBody(map[string]any{}, nil); got != nil {
		t.Fatalf("empty map + nil = %#v, want nil", got)
	}
	if got := mergeExtraBody(nil, map[string]any{}); got != nil {
		t.Fatalf("nil + empty map = %#v, want nil", got)
	}
	if got := mergeExtraBody(nil, nil); got != nil {
		t.Fatalf("nil + nil = %#v, want nil", got)
	}
}

func TestCloneExtraBodyMap_EmptyMap(t *testing.T) {
	if got := cloneExtraBodyMap(map[string]any{}); got != nil {
		t.Fatalf("empty map should clone to nil, got %#v", got)
	}
}

func TestMergeExtraBody_OverrideWins(t *testing.T) {
	base := map[string]any{"thinking": map[string]any{"type": "disabled"}}
	override := map[string]any{"temperature": 0.2}
	merged := mergeExtraBody(base, override)
	if merged["temperature"] != 0.2 {
		t.Fatalf("merged = %#v", merged)
	}
	thinking, ok := merged["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking not preserved: %#v", merged)
	}

	override2 := map[string]any{"thinking": map[string]any{"type": "enabled"}}
	merged2 := mergeExtraBody(base, override2)
	thinking2 := merged2["thinking"].(map[string]any)
	if thinking2["type"] != "enabled" {
		t.Fatalf("override should win: %#v", merged2)
	}
}

func TestMergeExtraBody_DeepMergePreservesSiblingKeys(t *testing.T) {
	base := map[string]any{
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 5000,
		},
	}
	override := map[string]any{
		"thinking": map[string]any{"type": "disabled"},
	}
	merged := mergeExtraBody(base, override)
	thinking := merged["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("type = %#v", thinking["type"])
	}
	if thinking["budget_tokens"] != 5000 {
		t.Fatalf("budget_tokens should be preserved: %#v", thinking)
	}
}

func TestApplyModelsThinking_DeepMergePreservesThinkingBudget(t *testing.T) {
	base := map[string]any{
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 5000,
		},
	}
	body, err := ApplyModelsThinking(base, map[string]string{"deepseek-v4-pro": "off"}, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "disabled" || thinking["budget_tokens"] != 5000 {
		t.Fatalf("body = %#v", body)
	}
}

func TestTranslateThinking_UnknownMode(t *testing.T) {
	if body := translateThinking("enabled", "deepseek", "deepseek-v4-pro"); body != nil {
		t.Fatalf("unknown mode on object style = %#v, want nil", body)
	}
	if body := translateThinking("enabled", "dashscope", "qwen3.7-max"); body != nil {
		t.Fatalf("unknown mode on bool style = %#v, want nil", body)
	}
}

func TestLookupModelsThinkingMode_DuplicateCaseInsensitiveKeys(t *testing.T) {
	m := map[string]string{
		"Claude-3": "off",
		"CLAUDE-3": "on",
	}
	mode, ok := LookupModelsThinkingMode(m, "claude-3")
	if !ok {
		t.Fatal("expected match")
	}
	// Deterministic: sort picks "CLAUDE-3" before "Claude-3".
	if mode != "on" {
		t.Fatalf("mode = %q, want on (alphabetically first key)", mode)
	}
}

func TestValidateModelsThinking(t *testing.T) {
	if err := ValidateModelsThinking(map[string]string{"m": "off"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateModelsThinking(map[string]string{"m": "bogus"}); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestValidateModelsThinking_RejectsEmptyModelKey(t *testing.T) {
	if err := ValidateModelsThinking(map[string]string{"": "off"}); err == nil {
		t.Fatal("expected error for empty model key")
	}
	if err := ValidateModelsThinking(map[string]string{"   ": "off"}); err == nil {
		t.Fatal("expected error for whitespace model key")
	}
}

func TestValidateModelsThinking_RejectsCaseInsensitiveDuplicateKeys(t *testing.T) {
	err := ValidateModelsThinking(map[string]string{"Model-A": "on", "model-a": "off"})
	if err == nil {
		t.Fatal("expected error for case-insensitive duplicate keys")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %q, want ambiguous duplicate message", err)
	}
}

func TestMergeModelsThinking(t *testing.T) {
	got := MergeModelsThinking(map[string]string{"a": "off"}, map[string]string{"a": "on", "b": "off"})
	if got["a"] != ModelsThinkingOn {
		t.Fatalf("on should override off for key a: %#v", got)
	}
	if got["b"] != "off" {
		t.Fatalf("off should be stored: %#v", got)
	}

	got = MergeModelsThinking(map[string]string{"a": "off"}, map[string]string{"a": " ON ", "b": " OFF "})
	if got["a"] != ModelsThinkingOn {
		t.Fatalf("normalized on should be stored: %#v", got)
	}
	if got["b"] != "off" {
		t.Fatalf("normalized OFF should be stored as off: %#v", got)
	}

	baseOnly := MergeModelsThinking(map[string]string{"x": "off"}, nil)
	if baseOnly["x"] != "off" {
		t.Fatalf("base copy = %#v", baseOnly)
	}
	baseOnly["x"] = "on"
	orig := MergeModelsThinking(map[string]string{"x": "off"}, nil)
	if orig["x"] != "off" {
		t.Fatal("MergeModelsThinking should return a copy")
	}
	if got := MergeModelsThinking(nil, nil); got != nil {
		t.Fatalf("nil/nil should return nil, got %#v", got)
	}
	if got := MergeModelsThinking(nil, map[string]string{"a": "off"}); got["a"] != "off" {
		t.Fatalf("nil base with override should store off, got %#v", got)
	}

	got = MergeModelsThinking(map[string]string{"Model-A": "off"}, map[string]string{"Model-B": " OFF "})
	if got["model-a"] != "off" || got["model-b"] != "off" {
		t.Fatalf("keys should be normalized: %#v", got)
	}

	got = MergeModelsThinking(nil, map[string]string{"bad-model": "disabled"})
	if got["bad-model"] != "disabled" {
		t.Fatalf("invalid override should be preserved: %#v", got)
	}

	got = MergeModelsThinking(map[string]string{"Model-A": "off", "model-a": "on"}, nil)
	if got["model-a"] != "off" {
		t.Fatalf("case-variant duplicates should keep first sorted key, got %#v", got)
	}
}

func TestApplyModelsThinking_UnknownProviderIgnoresModelsThinking(t *testing.T) {
	base := map[string]any{"enable_thinking": true}
	body, err := ApplyModelsThinking(base, map[string]string{"m": "off"}, "my-custom-provider", "some-model")
	if err != nil {
		t.Fatalf("custom provider should ignore models_thinking: %v", err)
	}
	if body["enable_thinking"] != true {
		t.Fatalf("extra_body should pass through, got %#v", body)
	}
}

func TestApplyModelsThinking_Off(t *testing.T) {
	body, err := ApplyModelsThinking(nil, map[string]string{"deepseek-v4-pro": "off"}, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("body = %#v", body)
	}
}

func TestApplyModelsThinking_InvalidMode(t *testing.T) {
	_, err := ApplyModelsThinking(nil, map[string]string{"m": "enabled"}, "anthropic", "m")
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestApplyModelsThinking_AlwaysOnError(t *testing.T) {
	_, err := ApplyModelsThinking(nil, map[string]string{"kimi-k2.7-code": "off"}, "kimi", "kimi-k2.7-code")
	if err == nil {
		t.Fatal("expected error for always-on model")
	}
}

func TestApplyModelsThinking_NotSupportedError(t *testing.T) {
	// anthropic is now not_supported; trying to disable thinking on any model should error.
	_, err := ApplyModelsThinking(nil, map[string]string{"claude-opus-4-6": "off"}, "anthropic", "claude-opus-4-6")
	if err == nil {
		t.Fatal("expected error for not-supported provider")
	}
}

func TestApplyModelsThinking_NoEntryExtraBodyWins(t *testing.T) {
	base := map[string]any{
		"temperature":     0.5,
		"enable_thinking": false,
	}
	body, err := ApplyModelsThinking(base, nil, "dashscope", "qwen3.7-max")
	if err != nil {
		t.Fatal(err)
	}
	if body["enable_thinking"] != false {
		t.Fatalf("without models_thinking entry extra_body should win: %#v", body)
	}
	if body["temperature"] != 0.5 {
		t.Fatalf("other extra_body fields should be preserved: %#v", body)
	}

	// Other models in map should not affect current model.
	body, err = ApplyModelsThinking(base, map[string]string{"glm-5.1": "off"}, "baidu-qianfan", "glm-5")
	if err != nil {
		t.Fatal(err)
	}
	if body["enable_thinking"] != false {
		t.Fatalf("missing model entry should leave extra_body unchanged: %#v", body)
	}
}

func TestApplyModelsThinking_ConfigOnOverridesExtraBodyDisable(t *testing.T) {
	base := map[string]any{
		"enable_thinking": false,
	}
	body, err := ApplyModelsThinking(base, map[string]string{"qwen3.7-max": "on"}, "dashscope", "qwen3.7-max")
	if err != nil {
		t.Fatal(err)
	}
	if body["enable_thinking"] != true {
		t.Fatalf("models_thinking on should override extra_body disable: %#v", body)
	}

	base = map[string]any{
		"thinking": map[string]any{"type": "disabled", "budget_tokens": 5000},
	}
	body, err = ApplyModelsThinking(base, map[string]string{"deepseek-v4-pro": "on"}, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("models_thinking on should override extra_body thinking disable: %#v", body)
	}
	if thinking["budget_tokens"] != 5000 {
		t.Fatalf("deep merge should preserve sibling keys: %#v", body)
	}
}

func TestApplyModelsThinking_NoEntryPreservesExtraBodyForUnsupportedProvider(t *testing.T) {
	base := map[string]any{"enable_thinking": false}
	body, err := ApplyModelsThinking(base, nil, "openai", "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	if body["enable_thinking"] != false {
		t.Fatalf("unsupported provider should pass extra_body unchanged: %#v", body)
	}
}

func TestApplyModelsThinking_OverridesExtraBody(t *testing.T) {
	base := map[string]any{"enable_thinking": true}
	body, err := ApplyModelsThinking(base, map[string]string{"qwen3.7-max": "off"}, "dashscope", "qwen3.7-max")
	if err != nil {
		t.Fatal(err)
	}
	if body["enable_thinking"] != false {
		t.Fatalf("models_thinking off should override extra_body: %#v", body)
	}

	base = map[string]any{"thinking": map[string]any{"type": "enabled"}}
	body, err = ApplyModelsThinking(base, map[string]string{"deepseek-v4-pro": "off"}, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("models_thinking off should override extra_body thinking: %#v", body)
	}
}

func TestApplyModelsThinking_CaseInsensitiveModelKey(t *testing.T) {
	body, err := ApplyModelsThinking(nil, map[string]string{"DeepSeek-V4-Pro": "off"}, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("case-insensitive models_thinking lookup failed: %#v", body)
	}
}

func TestNormalizeModelsThinking(t *testing.T) {
	got, err := NormalizeModelsThinking(map[string]string{"Model-A": " OFF "})
	if err != nil {
		t.Fatal(err)
	}
	if got["model-a"] != ModelsThinkingOff {
		t.Fatalf("got = %#v", got)
	}
	got, err = NormalizeModelsThinking(map[string]string{"m": "on"})
	if err != nil {
		t.Fatal("all-on input should be stored")
	}
	if got["m"] != ModelsThinkingOn {
		t.Fatalf("got = %#v", got)
	}
	if _, err := NormalizeModelsThinking(map[string]string{"m": "enabled"}); err == nil {
		t.Fatal("expected error for invalid models_thinking value")
	}
}

func TestPruneModelsThinking(t *testing.T) {
	got := PruneModelsThinking(map[string]string{
		"model-a": ModelsThinkingOff,
		"model-b": ModelsThinkingOff,
	}, []string{"Model-A", "other"})
	if len(got) != 1 || got["model-a"] != ModelsThinkingOff {
		t.Fatalf("PruneModelsThinking = %#v", got)
	}
}

func TestRemoveModelsThinkingKey(t *testing.T) {
	got := RemoveModelsThinkingKey(map[string]string{"Claude-Sonnet-4-6": ModelsThinkingOff}, "claude-sonnet-4-6")
	if got != nil {
		t.Fatalf("RemoveModelsThinkingKey should clear last entry, got %#v", got)
	}
}

func TestApplyModelsThinking_ReturnsCopy(t *testing.T) {
	orig := map[string]any{"temperature": 0.5}
	got, err := ApplyModelsThinking(orig, nil, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	got["temperature"] = 1.0
	if orig["temperature"] != 0.5 {
		t.Fatal("mutating result should not affect input extraBody")
	}

	nested := map[string]any{
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 5000,
		},
	}
	got, err = ApplyModelsThinking(nested, map[string]string{"deepseek-v4-pro": "off"}, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	thinking := got["thinking"].(map[string]any)
	thinking["budget_tokens"] = 1
	origThinking := nested["thinking"].(map[string]any)
	if origThinking["budget_tokens"] != 5000 {
		t.Fatal("mutating nested result should not affect input extraBody")
	}
}

func TestRemoveModelsThinkingKey_NormalizesKeys(t *testing.T) {
	got := RemoveModelsThinkingKey(map[string]string{"Model-A": ModelsThinkingOff}, "other")
	if got["model-a"] != ModelsThinkingOff {
		t.Fatalf("keys should be normalized even without removal: %#v", got)
	}
	got = RemoveModelsThinkingKey(map[string]string{"Model-A": ModelsThinkingOff}, "")
	if got["model-a"] != ModelsThinkingOff {
		t.Fatalf("empty model should still normalize keys: %#v", got)
	}
}

func TestApplyModelsThinking_ConfigOnUnsupportedProviderPassesExtraBody(t *testing.T) {
	base := map[string]any{"temperature": 0.5}
	body, err := ApplyModelsThinking(base, map[string]string{"m": "on"}, "anthropic", "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if body["temperature"] != 0.5 || len(body) != 1 {
		t.Fatalf("unsupported provider should pass extra_body through even with models_thinking on: %#v", body)
	}

	body, err = ApplyModelsThinking(base, map[string]string{"m": " ON "}, "anthropic", "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 {
		t.Fatalf("normalized on on unsupported provider should not inject: %#v", body)
	}
}

func TestTranslateThinking_QianfanDeepSeekUsesObject(t *testing.T) {
	body := translateThinking("off", "baidu-qianfan", "deepseek-v4-pro")
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("deepseek on qianfan should use thinking object: %#v", body)
	}

	boolBody := translateThinking("off", "baidu-qianfan", "ernie-5.1")
	if boolBody["enable_thinking"] != false {
		t.Fatalf("ernie on qianfan should use enable_thinking: %#v", boolBody)
	}

	glmBody := translateThinking("off", "baidu-qianfan", "glm-5.2")
	glmThinking, ok := glmBody["thinking"].(map[string]any)
	if !ok || glmThinking["type"] != "disabled" {
		t.Fatalf("glm on qianfan should use thinking object: %#v", glmBody)
	}
}

func TestTranslateThinking_QianfanKimiK26NotSupported(t *testing.T) {
	// kimi-k2.6 is now thinking_object on qianfan (verified by testing).
	body := translateThinking("off", "baidu-qianfan", "kimi-k2.6")
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("qianfan kimi-k2.6 should use thinking object: %#v", body)
	}
	if !IsThinkingSupported("baidu-qianfan", "kimi-k2.6") {
		t.Fatal("qianfan kimi-k2.6 should expose thinking toggle")
	}
}

func TestThinkingPolicy_ThinkingOnly(t *testing.T) {
	original, had := thinkingProviderIndex["thinking-only-fixture"]
	defer func() {
		if had {
			thinkingProviderIndex["thinking-only-fixture"] = original
		} else {
			delete(thinkingProviderIndex, "thinking-only-fixture")
		}
	}()
	thinkingProviderIndex["thinking-only-fixture"] = thinkingProviderMeta{
		defaultStyle: ThinkingStyleEnableBool,
		modelOverrides: map[string]ModelThinkingConfig{
			"fixture-model": {Policy: ThinkingPolicyThinkingOnly},
		},
	}

	if !IsThinkingOnly("thinking-only-fixture", "fixture-model") {
		t.Fatal("fixture model should be thinking only")
	}
	if !IsThinkingSupported("thinking-only-fixture", "fixture-model") {
		t.Fatal("thinking only models should report thinking supported")
	}
	if CanDisableThinking("thinking-only-fixture", "fixture-model") {
		t.Fatal("thinking only models cannot be disabled via toggle")
	}
	body, err := ApplyModelsThinking(nil, map[string]string{"fixture-model": "off"}, "thinking-only-fixture", "fixture-model")
	if err == nil {
		t.Fatal("expected error when disabling thinking on thinking_only model")
	}
	if body != nil {
		t.Fatalf("body = %#v, want nil on error", body)
	}
	onBody, err := ApplyModelsThinking(nil, map[string]string{"fixture-model": "on"}, "thinking-only-fixture", "fixture-model")
	if err != nil {
		t.Fatalf("on path should pass through: %v", err)
	}
	if onBody != nil {
		t.Fatalf("on path should not inject for thinking_only: %#v", onBody)
	}
}

func TestThinkingPolicy_DashScopeAlwaysOn(t *testing.T) {
	// kimi-k2.7-code and MiniMax-M2.5 are now always_on on dashscope (not thinking_only).
	if !IsThinkingAlwaysOn("dashscope", "kimi-k2.7-code") {
		t.Fatal("dashscope kimi-k2.7-code should be always on")
	}
	if IsThinkingOnly("dashscope", "kimi-k2.7-code") {
		t.Fatal("dashscope kimi-k2.7-code should not be thinking only")
	}
	if !IsThinkingAlwaysOn("dashscope", "MiniMax-M2.5") {
		t.Fatal("dashscope MiniMax-M2.5 should be always on")
	}
	if IsThinkingOnly("dashscope", "MiniMax-M2.5") {
		t.Fatal("dashscope MiniMax-M2.5 should not be thinking only")
	}
	if !CanDisableThinking("dashscope", "qwen3.7-max") {
		t.Fatal("mixed qwen models should remain toggleable on dashscope")
	}
}

func TestIsThinkingAlwaysOn_ProviderSpecificPrefixes(t *testing.T) {
	if !IsThinkingAlwaysOn("minimax", "MiniMax-M2.7-highspeed") {
		t.Fatal("minimax M2.7 should be always on")
	}
	if IsThinkingAlwaysOn("deepseek", "deepseek-v4-pro") {
		t.Fatal("deepseek should not inherit minimax always-on prefixes")
	}
	if !IsThinkingAlwaysOn("tencent-tokenhub", "kimi-k2.7-code") {
		t.Fatal("tokenhub kimi-k2.7 passthrough should be always on")
	}
	if IsThinkingAlwaysOn("kimi", "kimi-k2.6") {
		t.Fatal("kimi-k2.6 should not be always on")
	}
	if IsThinkingAlwaysOn("kimi", "kimi-k2.5") {
		t.Fatal("kimi-k2.5 should not be always on")
	}
	if IsThinkingAlwaysOn("minimax", "MiniMax-M3") {
		t.Fatal("MiniMax-M3 should not be always on")
	}
}

func TestTranslateThinking_MiniMaxM3UsesAdaptive(t *testing.T) {
	body := translateThinking(ModelsThinkingOn, "minimax", "MiniMax-M3")
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != thinkingObjectOnAdaptive {
		t.Fatalf("on = %#v, want thinking.type adaptive", body)
	}

	body = translateThinking(ModelsThinkingOff, "minimax", "MiniMax-M3")
	thinking, ok = body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("off = %#v, want thinking.type disabled", body)
	}
}

func TestTranslateThinking_TencentTokenhubMiniMaxM3UsesAdaptive(t *testing.T) {
	body := translateThinking(ModelsThinkingOn, "tencent-tokenhub", "minimax-m3")
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != thinkingObjectOnAdaptive {
		t.Fatalf("on = %#v, want thinking.type adaptive", body)
	}
}

func TestApplyModelsThinking_MiniMaxM3OnInjectsAdaptive(t *testing.T) {
	got, err := ApplyModelsThinking(nil, map[string]string{"minimax-m3": ModelsThinkingOn}, "minimax", "MiniMax-M3")
	if err != nil {
		t.Fatal(err)
	}
	thinking, ok := got["thinking"].(map[string]any)
	if !ok || thinking["type"] != thinkingObjectOnAdaptive {
		t.Fatalf("got = %#v, want thinking.type adaptive", got)
	}
}
