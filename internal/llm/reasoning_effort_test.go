package llm

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeReasoningEffort(t *testing.T) {
	tests := map[string]string{
		"":          ReasoningEffortDefault,
		" default ": ReasoningEffortDefault,
		" XHIGH ":   "xhigh",
	}
	for input, want := range tests {
		if got := NormalizeReasoningEffort(input); got != want {
			t.Errorf("NormalizeReasoningEffort(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReasoningEffortOptions(t *testing.T) {
	if got, want := ReasoningEffortOptions(ProtocolAnthropic), []string{"low", "medium", "high", "xhigh", "max"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Anthropic options = %v, want %v", got, want)
	}
	if got, want := ReasoningEffortOptions(ProtocolOpenAIResponses), []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}; !reflect.DeepEqual(got, want) {
		t.Errorf("OpenAI options = %v, want %v", got, want)
	}
}

func TestValidateReasoningEffortRejectsOpenAIOnlyValuesForAnthropic(t *testing.T) {
	for _, effort := range []string{"none", "minimal"} {
		err := ValidateReasoningEffort(ProtocolAnthropic, effort)
		if err == nil || !strings.Contains(err.Error(), effort) {
			t.Errorf("ValidateReasoningEffort(anthropic, %q) = %v, want descriptive error", effort, err)
		}
	}
}
