package llm

import (
	"fmt"
	"strings"
)

const ReasoningEffortDefault = ""

var (
	openAIReasoningEfforts    = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
	anthropicReasoningEfforts = []string{"low", "medium", "high", "xhigh", "max"}
)

// NormalizeReasoningEffort returns the canonical wire value. An empty value or
// "default" means that the provider should choose its own default.
func NormalizeReasoningEffort(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "default" {
		return ReasoningEffortDefault
	}
	return value
}

// ReasoningEffortOptions returns the protocol-level effort values. Individual
// models may support only a subset; the provider remains authoritative.
func ReasoningEffortOptions(protocol string) []string {
	var values []string
	if NormalizeProtocol(protocol) == ProtocolAnthropic {
		values = anthropicReasoningEfforts
	} else {
		values = openAIReasoningEfforts
	}
	return append([]string(nil), values...)
}

// ValidateReasoningEffort rejects values that cannot exist on the selected
// protocol. Model-specific support is intentionally left to the provider.
func ValidateReasoningEffort(protocol, value string) error {
	value = NormalizeReasoningEffort(value)
	if value == ReasoningEffortDefault {
		return nil
	}
	for _, candidate := range ReasoningEffortOptions(protocol) {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("reasoning effort %q is not supported for protocol %q; valid values: %s",
		value, NormalizeProtocol(protocol), strings.Join(ReasoningEffortOptions(protocol), ", "))
}
