package main

import (
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func escKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEscape}
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

func leftKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyLeft}
}

func rightKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyRight}
}

func downKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyDown}
}

func tabKeyMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyTab}
}

func charKey(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: c}
}

// --- Tab switching tests ---

func TestProviderTUI_TabSwitchRight(t *testing.T) {
	m := newProviderTUI(&Config{})
	if m.activeTab != tabOfficial {
		t.Fatalf("initial tab = %d, want %d", m.activeTab, tabOfficial)
	}

	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	if m2.activeTab != tabCustom {
		t.Errorf("after right, tab = %d, want %d", m2.activeTab, tabCustom)
	}

	result, _ = m2.Update(rightKey())
	m3 := result.(providerTUIModel)
	if m3.activeTab != tabManual {
		t.Errorf("after 2x right, tab = %d, want %d", m3.activeTab, tabManual)
	}

	// Should not go past last tab
	result, _ = m3.Update(rightKey())
	m4 := result.(providerTUIModel)
	if m4.activeTab != tabManual {
		t.Errorf("after 3x right, tab = %d, want %d (should clamp)", m4.activeTab, tabManual)
	}
}

func TestProviderTUI_TabSwitchLeft(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Go to manual tab first
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(rightKey())
	m3 := result.(providerTUIModel)
	if m3.activeTab != tabManual {
		t.Fatalf("setup: tab = %d, want %d", m3.activeTab, tabManual)
	}

	result, _ = m3.Update(leftKey())
	m4 := result.(providerTUIModel)
	if m4.activeTab != tabCustom {
		t.Errorf("after left, tab = %d, want %d", m4.activeTab, tabCustom)
	}

	result, _ = m4.Update(leftKey())
	m5 := result.(providerTUIModel)
	if m5.activeTab != tabOfficial {
		t.Errorf("after 2x left, tab = %d, want %d", m5.activeTab, tabOfficial)
	}

	// Should not go past first tab
	result, _ = m5.Update(leftKey())
	m6 := result.(providerTUIModel)
	if m6.activeTab != tabOfficial {
		t.Errorf("after 3x left, tab = %d, want %d (should clamp)", m6.activeTab, tabOfficial)
	}
}

func TestProviderTUI_TabKeyCycles(t *testing.T) {
	m := newProviderTUI(&Config{})

	result, _ := m.Update(tabKeyMsg())
	m2 := result.(providerTUIModel)
	if m2.activeTab != tabCustom {
		t.Errorf("after tab, tab = %d, want %d", m2.activeTab, tabCustom)
	}

	result, _ = m2.Update(tabKeyMsg())
	m3 := result.(providerTUIModel)
	if m3.activeTab != tabManual {
		t.Errorf("after 2x tab, tab = %d, want %d", m3.activeTab, tabManual)
	}

	result, _ = m3.Update(tabKeyMsg())
	m4 := result.(providerTUIModel)
	if m4.activeTab != tabOfficial {
		t.Errorf("after 3x tab, tab = %d, want %d (should wrap)", m4.activeTab, tabOfficial)
	}
}

func TestProviderTUI_TabSwitchOnlyOnStepProvider(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Advance to stepModel
	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepModel {
		t.Fatalf("step = %d, want %d", m2.step, stepModel)
	}

	// Tab keys should not change tab
	result, _ = m2.Update(rightKey())
	m3 := result.(providerTUIModel)
	if m3.activeTab != tabOfficial {
		t.Errorf("right on stepModel should not change tab: got %d", m3.activeTab)
	}
}

// --- Official tab tests (updated from original) ---

func TestProviderTUI_OfficialProvidersSortedByDisplayName(t *testing.T) {
	m := newProviderTUI(&Config{})

	displayNames := make([]string, len(m.providers))
	normalized := make([]string, len(m.providers))
	for i, p := range m.providers {
		displayNames[i] = p.DisplayName
		normalized[i] = strings.ToLower(p.DisplayName)
	}

	if !sort.StringsAreSorted(normalized) {
		t.Errorf("provider display names are not sorted: %v", displayNames)
	}
}

func TestProviderTUI_EscFromModelGoesBackToProvider(t *testing.T) {
	m := newProviderTUI(&Config{})

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepModel {
		t.Fatalf("after Enter, step = %d, want %d (stepModel)", m2.step, stepModel)
	}

	result, _ = m2.Update(escKey())
	m3 := result.(providerTUIModel)
	if m3.step != stepProvider {
		t.Errorf("after Esc on stepModel, step = %d, want %d (stepProvider)", m3.step, stepProvider)
	}
	if m3.cancelled {
		t.Error("should not be cancelled when going back from stepModel")
	}
}

func TestProviderTUI_EscFromAPIKeyGoesBackToModel(t *testing.T) {
	m := newProviderTUI(&Config{})

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)

	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if m3.step != stepAPIKey {
		t.Fatalf("after 2x Enter, step = %d, want %d (stepAPIKey)", m3.step, stepAPIKey)
	}

	result, _ = m3.Update(escKey())
	m4 := result.(providerTUIModel)
	if m4.step != stepModel {
		t.Errorf("after Esc on stepAPIKey, step = %d, want %d (stepModel)", m4.step, stepModel)
	}
}

func TestProviderTUI_EscFromProviderCancels(t *testing.T) {
	m := newProviderTUI(&Config{})

	result, cmd := m.Update(escKey())
	m2 := result.(providerTUIModel)
	if !m2.cancelled {
		t.Error("Esc on stepProvider should set cancelled = true")
	}
	if cmd == nil {
		t.Error("Esc on stepProvider should return tea.Quit")
	}
}

func TestProviderTUI_EscKeyString(t *testing.T) {
	esc := escKey()
	if s := esc.String(); s != "esc" {
		t.Errorf("escape key String() = %q, want %q", s, "esc")
	}
}

// --- Manual tab tests ---

func TestProviderTUI_ManualTabEnterStartsForm(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Switch to manual tab
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(rightKey())
	m3 := result.(providerTUIModel)
	if m3.activeTab != tabManual {
		t.Fatalf("tab = %d, want %d", m3.activeTab, tabManual)
	}

	// Press Enter to start form
	result, _ = m3.Update(enterKey())
	m4 := result.(providerTUIModel)
	if !m4.inManualForm {
		t.Error("Enter on manual tab should set inManualForm = true")
	}
	if m4.manualStep != manualStepURL {
		t.Errorf("manualStep = %d, want %d", m4.manualStep, manualStepURL)
	}
}

func TestProviderTUI_ManualFormEscFromURLExitsForm(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Switch to manual tab and enter form
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(rightKey())
	m3 := result.(providerTUIModel)
	result, _ = m3.Update(enterKey())
	m4 := result.(providerTUIModel)
	if !m4.inManualForm {
		t.Fatalf("should be in manual form")
	}

	// Esc should exit form, not cancel
	result, _ = m4.Update(escKey())
	m5 := result.(providerTUIModel)
	if m5.inManualForm {
		t.Error("Esc from URL step should exit form")
	}
	if m5.cancelled {
		t.Error("should not be cancelled when exiting form")
	}
}

func TestProviderTUI_ManualFormEscRestoresOriginalValues(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{
			URL:       "https://example.com/v1",
			Model:     "test-model",
			AuthToken: "token-123",
		},
	}
	m := newProviderTUI(cfg)

	// Enter the form
	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if !m2.inManualForm {
		t.Fatalf("should be in manual form")
	}

	// Simulate editing by directly modifying the input value
	m2.manualURLInput.SetValue("https://modified.example.com")

	// Esc should restore original values
	result, _ = m2.Update(escKey())
	m3 := result.(providerTUIModel)
	if m3.inManualForm {
		t.Error("should have exited form")
	}
	if m3.manualURLInput.Value() != "https://example.com/v1" {
		t.Errorf("URL not restored: got %q, want %q", m3.manualURLInput.Value(), "https://example.com/v1")
	}
	if m3.manualModelInput.Value() != "test-model" {
		t.Errorf("Model not restored: got %q, want %q", m3.manualModelInput.Value(), "test-model")
	}
	if m3.manualTokenInput.Value() != "token-123" {
		t.Errorf("Token not restored: got %q, want %q", m3.manualTokenInput.Value(), "token-123")
	}
}

func TestProviderTUI_ManualFormPrefilledValues(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{
			URL:       "https://example.com/v1",
			Model:     "test-model",
			AuthToken: "token-123",
		},
	}
	m := newProviderTUI(cfg)

	if m.activeTab != tabManual {
		t.Fatalf("should auto-select manual tab when Llm.URL is set, got %d", m.activeTab)
	}
	if m.manualURLInput.Value() != "https://example.com/v1" {
		t.Errorf("URL not prefilled: got %q", m.manualURLInput.Value())
	}
	if m.manualModelInput.Value() != "test-model" {
		t.Errorf("Model not prefilled: got %q", m.manualModelInput.Value())
	}
	if m.manualTokenInput.Value() != "token-123" {
		t.Errorf("Token not prefilled: got %q", m.manualTokenInput.Value())
	}
}

func TestProviderTUI_ManualResult(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{
			URL:       "https://example.com/v1",
			Model:     "test-model",
			AuthToken: "token-123",
		},
	}
	m := newProviderTUI(cfg)

	// Enter the form
	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	m2.confirmed = true

	r := m2.result()
	if !r.isManual {
		t.Error("result should have isManual = true")
	}
	if r.url != "https://example.com/v1" {
		t.Errorf("result url = %q, want %q", r.url, "https://example.com/v1")
	}
	if r.model != "test-model" {
		t.Errorf("result model = %q, want %q", r.model, "test-model")
	}
}

func TestProviderTUI_ManualFormPrefilledWhenProviderSet(t *testing.T) {
	cfg := &Config{
		Provider: "my-gateway",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway": {URL: "https://gw.example.com/v1", Protocol: "openai", Model: "llama-3"},
		},
		Llm: LlmConfig{
			URL:       "https://manual.example.com/v1",
			Model:     "manual-model",
			AuthToken: "manual-token",
		},
	}
	m := newProviderTUI(cfg)

	if m.activeTab != tabCustom {
		t.Fatalf("should auto-select custom tab, got %d", m.activeTab)
	}
	if m.manualURLInput.Value() != "https://manual.example.com/v1" {
		t.Errorf("URL not prefilled: got %q", m.manualURLInput.Value())
	}
	if m.manualModelInput.Value() != "manual-model" {
		t.Errorf("Model not prefilled: got %q", m.manualModelInput.Value())
	}
	if m.manualTokenInput.Value() != "manual-token" {
		t.Errorf("Token not prefilled: got %q", m.manualTokenInput.Value())
	}
}

// --- Custom tab tests ---

func TestProviderTUI_CustomTabShowsAddOption(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Switch to custom tab
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	if m2.activeTab != tabCustom {
		t.Fatalf("tab = %d, want %d", m2.activeTab, tabCustom)
	}

	// With no custom providers, only "Add" option exists at index 0
	if m2.customListCount() != 1 {
		t.Errorf("customListCount() = %d, want 1 (only add option)", m2.customListCount())
	}
}

func TestProviderTUI_CustomTabSelectAddStartsForm(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Switch to custom tab
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)

	// Enter on "Add" option
	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if !m3.creatingCustom {
		t.Error("Enter on add option should set creatingCustom = true")
	}
	if m3.cpStep != cpStepName {
		t.Errorf("cpStep = %d, want %d", m3.cpStep, cpStepName)
	}
}

func TestProviderTUI_CustomFormEscFromNameExitsForm(t *testing.T) {
	m := newProviderTUI(&Config{})

	// Switch to custom tab and start form
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if !m3.creatingCustom {
		t.Fatalf("should be creating custom")
	}

	// Esc from name step should exit form
	result, _ = m3.Update(escKey())
	m4 := result.(providerTUIModel)
	if m4.creatingCustom {
		t.Error("Esc from name step should exit custom form")
	}
	if m4.cancelled {
		t.Error("should not be cancelled")
	}
}

func TestProviderTUI_CustomProviderExistsInList(t *testing.T) {
	cfg := &Config{
		Provider: "my-llm",
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {
				URL:      "https://custom.api/v1",
				Protocol: "openai",
				Model:    "custom-model",
				APIKey:   "key-123",
			},
		},
	}
	m := newProviderTUI(cfg)

	if m.activeTab != tabCustom {
		t.Fatalf("should auto-select custom tab, got %d", m.activeTab)
	}
	if len(m.customProviders) != 1 {
		t.Fatalf("expected 1 custom provider, got %d", len(m.customProviders))
	}
	if m.customProviders[0].name != "my-llm" {
		t.Errorf("custom provider name = %q, want %q", m.customProviders[0].name, "my-llm")
	}
}

func TestProviderTUI_SelectExistingCustomGoesToModel(t *testing.T) {
	cfg := &Config{
		Provider: "my-llm",
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {
				URL:      "https://custom.api/v1",
				Protocol: "openai",
				Model:    "custom-model",
				Models:   []string{"custom-model", "custom-fast"},
				APIKey:   "key-123",
			},
		},
	}
	m := newProviderTUI(cfg)

	// Enter on existing custom provider should go to model selection first.
	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepModel {
		t.Errorf("step = %d, want %d (stepModel)", m2.step, stepModel)
	}
	if m2.models()[0] != "custom-model" {
		t.Errorf("first model = %q, want %q", m2.models()[0], "custom-model")
	}
}

// --- collectCustomProviders tests ---

func TestCollectCustomProviders_NilConfig(t *testing.T) {
	result := collectCustomProviders(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestCollectCustomProviders_ReadsCustomProviders(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderEntry{
			"anthropic": {APIKey: "key1"},
			"openai":    {APIKey: "key2"},
		},
		CustomProviders: map[string]ProviderEntry{
			"my-custom": {URL: "https://example.com", Protocol: "openai"},
		},
	}
	result := collectCustomProviders(cfg)
	if len(result) != 1 {
		t.Fatalf("expected 1 custom provider, got %d", len(result))
	}
	if result[0].name != "my-custom" {
		t.Errorf("name = %q, want %q", result[0].name, "my-custom")
	}
}

func TestCollectCustomProviders_SortedByName(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"zzz-provider": {URL: "https://z.example.com"},
			"aaa-provider": {URL: "https://a.example.com"},
		},
	}
	result := collectCustomProviders(cfg)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].name != "aaa-provider" {
		t.Errorf("first = %q, want %q", result[0].name, "aaa-provider")
	}
	if result[1].name != "zzz-provider" {
		t.Errorf("second = %q, want %q", result[1].name, "zzz-provider")
	}
}
