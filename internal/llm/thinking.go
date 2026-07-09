package llm

import (
	"fmt"
	"sort"
	"strings"
)

// ThinkingStyle describes how a provider encodes thinking on/off in extra_body.
type ThinkingStyle string

const (
	ThinkingStyleObject       ThinkingStyle = "thinking_object"
	ThinkingStyleEnableBool   ThinkingStyle = "enable_thinking_bool"
	ThinkingStyleNotSupported ThinkingStyle = "not_supported"
)

// ThinkingPolicy describes per-model thinking toggle behavior.
type ThinkingPolicy string

const (
	// ThinkingPolicyToggle is the zero value: the model is toggleable (on/off).
	ThinkingPolicyToggle ThinkingPolicy = ""
	// ThinkingPolicyAlwaysOn marks models that always run with thinking enabled.
	ThinkingPolicyAlwaysOn ThinkingPolicy = "always_on"
	// ThinkingPolicyThinkingOnly marks thinking-only models that reject enable_thinking: false.
	ThinkingPolicyThinkingOnly ThinkingPolicy = "thinking_only"
	// ThinkingPolicyNotSupported hides the thinking toggle for the model.
	ThinkingPolicyNotSupported ThinkingPolicy = "not_supported"
)

const (
	// ModelsThinkingOn enables thinking for a model (persisted in config).
	ModelsThinkingOn = "on"
	// ModelsThinkingOff disables thinking for a model (persisted in config).
	ModelsThinkingOff = "off"

	thinkingObjectOnEnabled  = "enabled"
	thinkingObjectOnAdaptive = "adaptive"
	thinkingObjectOff        = "disabled"
)

// ValidateModelsThinkingMode reports whether mode is a supported models_thinking value.
func ValidateModelsThinkingMode(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != ModelsThinkingOn && mode != ModelsThinkingOff {
		return fmt.Errorf("must be %q or %q, got %q", ModelsThinkingOn, ModelsThinkingOff, mode)
	}
	return nil
}

// ValidateModelsThinking validates every entry in a models_thinking map.
func ValidateModelsThinking(m map[string]string) error {
	seen := make(map[string]string, len(m))
	for model, mode := range m {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("models_thinking keys must be non-empty model names")
		}
		key := strings.ToLower(strings.TrimSpace(model))
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("models_thinking has ambiguous keys %q and %q (case-insensitive duplicate)", prev, model)
		}
		seen[key] = model
		if err := ValidateModelsThinkingMode(mode); err != nil {
			return fmt.Errorf("models_thinking[%q]: %w", model, err)
		}
	}
	return nil
}

func translateThinking(mode, providerName, modelName string) map[string]any {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return nil
	}
	policy := lookupModelThinkingPolicy(providerName, modelName)
	if policy == ThinkingPolicyAlwaysOn || policy == ThinkingPolicyThinkingOnly {
		return nil
	}
	if policy == ThinkingPolicyNotSupported {
		return nil
	}
	style := lookupThinkingStyle(providerName, modelName)
	switch style {
	case ThinkingStyleObject:
		switch mode {
		case ModelsThinkingOn:
			onType := lookupObjectOnType(providerName, modelName)
			return map[string]any{"thinking": map[string]any{"type": onType}}
		case ModelsThinkingOff:
			return map[string]any{"thinking": map[string]any{"type": thinkingObjectOff}}
		default:
			return nil
		}
	case ThinkingStyleEnableBool:
		switch mode {
		case ModelsThinkingOn:
			return map[string]any{"enable_thinking": true}
		case ModelsThinkingOff:
			return map[string]any{"enable_thinking": false}
		default:
			return nil
		}
	default:
		return nil
	}
}

func hasToggleStyle(providerName, modelName string) bool {
	style := lookupThinkingStyle(providerName, modelName)
	return style == ThinkingStyleObject || style == ThinkingStyleEnableBool
}

// IsThinkingSupported reports whether the provider exposes a thinking toggle.
func IsThinkingSupported(providerName, modelName string) bool {
	policy := lookupModelThinkingPolicy(providerName, modelName)
	switch policy {
	case ThinkingPolicyAlwaysOn, ThinkingPolicyThinkingOnly:
		return true
	case ThinkingPolicyNotSupported:
		return false
	default:
		return hasToggleStyle(providerName, modelName)
	}
}

// CanDisableThinking reports whether the user can turn thinking off for a model.
func CanDisableThinking(providerName, modelName string) bool {
	policy := lookupModelThinkingPolicy(providerName, modelName)
	if policy == ThinkingPolicyAlwaysOn || policy == ThinkingPolicyThinkingOnly || policy == ThinkingPolicyNotSupported {
		return false
	}
	return hasToggleStyle(providerName, modelName)
}

// IsThinkingAlwaysOn reports models that always run with thinking enabled.
func IsThinkingAlwaysOn(providerName, modelName string) bool {
	return lookupModelThinkingPolicy(providerName, modelName) == ThinkingPolicyAlwaysOn
}

// IsThinkingOnly reports models that only support thinking mode.
func IsThinkingOnly(providerName, modelName string) bool {
	return lookupModelThinkingPolicy(providerName, modelName) == ThinkingPolicyThinkingOnly
}

func lookupModelThinkingPolicy(providerName, modelName string) ThinkingPolicy {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	if modelLower == "" {
		return ThinkingPolicyNotSupported
	}
	meta, ok := thinkingProviderIndex[providerName]
	if !ok {
		return ThinkingPolicyNotSupported
	}
	if cfg, ok := meta.modelOverrides[modelLower]; ok {
		// NOTE: The zero value of Policy ("") doubles as "unset", so an explicit
		// ThinkingPolicyToggle override is indistinguishable from no override.
		// Do not set Policy to ThinkingPolicyToggle in ModelThinkingConfig;
		// omit the field instead.
		if cfg.Policy != "" {
			return cfg.Policy
		}
	}
	if lookupThinkingStyleFromMeta(meta, modelLower) == ThinkingStyleNotSupported {
		return ThinkingPolicyNotSupported
	}
	return ThinkingPolicyToggle
}

func lookupThinkingStyle(providerName, modelName string) ThinkingStyle {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	if modelLower == "" {
		return ThinkingStyleNotSupported
	}

	meta, ok := thinkingProviderIndex[providerName]
	if !ok {
		return ThinkingStyleNotSupported
	}
	return lookupThinkingStyleFromMeta(meta, modelLower)
}

func lookupObjectOnType(providerName, modelName string) string {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	if modelLower == "" {
		return thinkingObjectOnEnabled
	}
	meta, ok := thinkingProviderIndex[providerName]
	if !ok {
		return thinkingObjectOnEnabled
	}
	if cfg, ok := meta.modelOverrides[modelLower]; ok && cfg.ObjectOn != "" {
		return cfg.ObjectOn
	}
	return thinkingObjectOnEnabled
}

func lookupThinkingStyleFromMeta(meta thinkingProviderMeta, modelLower string) ThinkingStyle {
	if modelLower != "" {
		if cfg, ok := meta.modelOverrides[modelLower]; ok && cfg.Style != "" {
			return cfg.Style
		}
	}
	if meta.defaultStyle != "" {
		return meta.defaultStyle
	}
	return ThinkingStyleNotSupported
}

func thinkingProviderKnown(providerName string) bool {
	_, ok := thinkingProviderIndex[strings.ToLower(strings.TrimSpace(providerName))]
	return ok
}

// ApplyModelsThinking merges models_thinking for the current model into extraBody.
// When the model has a config entry, "on"/"off" inject fields via mergeExtraBody and
// take precedence over conflicting extraBody keys. When there is no entry for the
// model (or models_thinking is empty), extraBody is passed through unchanged.
// The returned map is always a new value; the input extraBody is never mutated.
// extraBody must be JSON-shaped (from json.Unmarshal): only map[string]any and []any
// are deep-cloned; other types are shared by reference.
// For mode "on", translateThinking returns explicit thinking fields (enabled or
// adaptive per model ObjectOn); those are merged into extraBody the same way as "off".
// Intentional asymmetry: "on" silently no-ops when thinking cannot be injected;
// "off" returns an error on preset providers when disable is unsupported, so users
// can detect stale models_thinking entries. Custom (non-registry) providers ignore
// models_thinking on both paths via thinkingProviderKnown.
func ApplyModelsThinking(extraBody map[string]any, modelsThinking map[string]string, providerName, model string) (map[string]any, error) {
	if len(modelsThinking) == 0 {
		return cloneExtraBodyMap(extraBody), nil
	}
	mode, ok := LookupModelsThinkingMode(modelsThinking, model)
	if !ok {
		return cloneExtraBodyMap(extraBody), nil
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case ModelsThinkingOn:
		if !IsThinkingSupported(providerName, model) {
			return cloneExtraBodyMap(extraBody), nil
		}
		translated := translateThinking(ModelsThinkingOn, providerName, model)
		if translated == nil {
			// always_on / thinking_only models need no injection; "on" is a no-op.
			return cloneExtraBodyMap(extraBody), nil
		}
		return mergeExtraBody(extraBody, translated), nil
	case ModelsThinkingOff:
		// Custom providers are not in thinkingProviderIndex; ignore models_thinking.
		if !thinkingProviderKnown(providerName) {
			return cloneExtraBodyMap(extraBody), nil
		}
		if !CanDisableThinking(providerName, model) {
			return nil, fmt.Errorf("provider %q does not support disabling thinking for model %q; remove models_thinking entry or change provider", providerName, model)
		}
		translated := translateThinking(ModelsThinkingOff, providerName, model)
		if translated == nil {
			return nil, fmt.Errorf("internal error: provider %q model %q passed CanDisableThinking but translateThinking returned nil", providerName, model)
		}
		return mergeExtraBody(extraBody, translated), nil
	default:
		return nil, fmt.Errorf("provider %q model %q: invalid models_thinking mode %q", providerName, model, mode)
	}
}

func cloneExtraBodyMap(m map[string]any) map[string]any {
	if m == nil || len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCloneValue(v)
	}
	return out
}

// deepCloneValue clones JSON-shaped extra_body values. Callers must populate extraBody
// from json.Unmarshal, which only produces map[string]any and []any. Non-JSON types
// are shared by reference, so the "input never mutated" guarantee requires JSON-shaped input.
func deepCloneValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v := range val {
			out[k] = deepCloneValue(v)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v := range val {
			out[i] = deepCloneValue(v)
		}
		return out
	default:
		// JSON scalars (string/number/bool/null) are immutable; shared by reference is safe.
		return v
	}
}

// LookupModelsThinkingMode returns the thinking mode for model using case-insensitive key matching.
func LookupModelsThinkingMode(modelsThinking map[string]string, model string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(model))
	if key == "" {
		return "", false
	}
	if mode, ok := modelsThinking[key]; ok {
		return mode, true
	}
	// Fallback for hand-edited config that may not have normalized keys.
	var candidates []string
	for k := range modelsThinking {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			candidates = append(candidates, k)
		}
	}
	switch len(candidates) {
	case 0:
		return "", false
	case 1:
		return modelsThinking[candidates[0]], true
	default:
		// Multiple case-insensitive matches: ambiguous config.
		// Deterministic first-wins fallback; callers should normalize keys on save.
		sort.Strings(candidates)
		return modelsThinking[candidates[0]], true
	}
}

// NormalizeModelsThinking lowercases keys and normalizes mode values to "on" or "off".
func NormalizeModelsThinking(m map[string]string) (map[string]string, error) {
	if err := ValidateModelsThinking(m); err != nil {
		return nil, err
	}
	return MergeModelsThinking(nil, m), nil
}

// MergeModelsThinking merges override into base. Both "on" and "off" are stored.
// Model keys are normalized to lowercase trimmed form.
// Callers must run ValidateModelsThinking on the result before persisting.
func MergeModelsThinking(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(override))
	applyThinkingMapLayer(merged, base, false)
	applyThinkingMapLayer(merged, override, true)
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func applyThinkingMapLayer(dest map[string]string, src map[string]string, overwrite bool) {
	if len(src) == 0 {
		return
	}
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := make(map[string]struct{}, len(src))
	for _, k := range keys {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if !overwrite {
			if _, exists := dest[key]; exists {
				continue
			}
		}
		switch strings.ToLower(strings.TrimSpace(src[k])) {
		case ModelsThinkingOff:
			dest[key] = ModelsThinkingOff
		case ModelsThinkingOn:
			dest[key] = ModelsThinkingOn
		default:
			// Callers must validate before merging; any other value is unsupported.
			dest[key] = src[k]
		}
	}
}

// PruneModelsThinking removes entries whose model keys are not in models (case-insensitive).
func PruneModelsThinking(thinking map[string]string, models []string) map[string]string {
	if len(thinking) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(models))
	for _, model := range models {
		key := strings.ToLower(strings.TrimSpace(model))
		if key != "" {
			allowed[key] = struct{}{}
		}
	}
	out := make(map[string]string, len(thinking))
	for k, v := range thinking {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; ok {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RemoveModelsThinkingKey drops one model's entry from the map (case-insensitive).
// Remaining keys are always normalized to lowercase trimmed form.
func RemoveModelsThinkingKey(thinking map[string]string, model string) map[string]string {
	model = strings.ToLower(strings.TrimSpace(model))
	if len(thinking) == 0 {
		return nil
	}
	out := make(map[string]string, len(thinking))
	for k, v := range thinking {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if model != "" && key == model {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeExtraBody(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(override))
	// Copy all base entries; override loop below replaces or deep-merges as needed.
	for k, v := range base {
		out[k] = deepCloneValue(v)
	}
	for k, v := range override {
		if baseMap, ok := base[k].(map[string]any); ok {
			if overrideMap, ok := v.(map[string]any); ok {
				out[k] = mergeExtraBody(baseMap, overrideMap)
				continue
			}
		}
		out[k] = deepCloneValue(v)
	}
	return out
}
