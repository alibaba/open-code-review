package main

import (
	"os"
	"path/filepath"
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
	return tea.KeyPressMsg{Code: c, Text: string(c)}
}

// --- Tab switching tests ---

func TestProviderTUI_TabSwitchRight(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(cfg, "")

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
	if !m3.manualTokenMasked {
		t.Error("Token should be masked after Esc restore")
	}
	if m3.manualTokenOriginal != "token-123" {
		t.Errorf("Token original not restored: got %q, want %q", m3.manualTokenOriginal, "token-123")
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
	m := newProviderTUI(cfg, "")

	if m.activeTab != tabManual {
		t.Fatalf("should auto-select manual tab when Llm.URL is set, got %d", m.activeTab)
	}
	if m.manualURLInput.Value() != "https://example.com/v1" {
		t.Errorf("URL not prefilled: got %q", m.manualURLInput.Value())
	}
	if m.manualModelInput.Value() != "test-model" {
		t.Errorf("Model not prefilled: got %q", m.manualModelInput.Value())
	}
	if !m.manualTokenMasked {
		t.Error("Token should be masked when prefilled")
	}
	if m.manualTokenOriginal != "token-123" {
		t.Errorf("Token original not prefilled: got %q, want %q", m.manualTokenOriginal, "token-123")
	}
	if m.manualTokenInput.Value() != strings.Repeat("*", 20) {
		t.Errorf("Token input not masked display: got %q", m.manualTokenInput.Value())
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
	m := newProviderTUI(cfg, "")

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
	m := newProviderTUI(cfg, "")

	if m.activeTab != tabCustom {
		t.Fatalf("should auto-select custom tab, got %d", m.activeTab)
	}
	if m.manualURLInput.Value() != "https://manual.example.com/v1" {
		t.Errorf("URL not prefilled: got %q", m.manualURLInput.Value())
	}
	if m.manualModelInput.Value() != "manual-model" {
		t.Errorf("Model not prefilled: got %q", m.manualModelInput.Value())
	}
	if !m.manualTokenMasked {
		t.Error("Token should be masked when prefilled")
	}
	if m.manualTokenOriginal != "manual-token" {
		t.Errorf("Token original not prefilled: got %q, want %q", m.manualTokenOriginal, "manual-token")
	}
}

func TestProviderTUI_ManualFormPrefillsAuthHeader(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{
			URL:        "https://manual.example.com/v1",
			Model:      "manual-model",
			AuthToken:  "manual-token",
			AuthHeader: "X-Custom-Auth",
		},
	}
	m := newProviderTUI(cfg, "")

	if got := m.manualAuthHeaderInput.Value(); got != "X-Custom-Auth" {
		t.Errorf("manualAuthHeaderInput not prefilled: got %q, want %q", got, "X-Custom-Auth")
	}
}

func TestProviderTUI_ManualFormSkipsEmptyTokenWhenOriginalExists(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{
			URL:       "https://example.com/v1",
			Model:     "test-model",
			AuthToken: "token-123",
		},
	}
	m := newProviderTUI(cfg, "")
	m.inManualForm = true
	m.manualStep = manualStepAuthToken
	m.manualTokenOriginal = "token-123"
	m.manualTokenMasked = false
	m.manualTokenInput.SetValue("")
	m.manualTokenInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.manualStep != manualStepAuthHeader {
		t.Errorf("manualStep = %d, want %d", m2.manualStep, manualStepAuthHeader)
	}

	m2.confirmed = true
	r := m2.result()
	if r.apiKey != "token-123" {
		t.Errorf("result apiKey = %q, want %q", r.apiKey, "token-123")
	}
}

func TestProviderTUI_ManualFormRequiresTokenOnFirstSetup(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.inManualForm = true
	m.manualStep = manualStepAuthToken
	m.manualTokenInput.SetValue("")
	m.manualTokenInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.manualStep != manualStepAuthToken {
		t.Errorf("should stay on auth token step, got %d", m2.manualStep)
	}
}

// --- Custom tab tests ---

func TestProviderTUI_CustomTabShowsAddOption(t *testing.T) {
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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
	m := newProviderTUI(&Config{}, "")

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

func TestProviderTUI_CustomFormRejectsDuplicateName(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {Model: "xxx"},
		},
	}
	m := newProviderTUI(cfg, "")

	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)

	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if !m3.creatingCustom {
		t.Fatal("should be creating custom")
	}

	m3.cpNameInput.SetValue("stepfun")
	result, _ = m3.Update(enterKey())
	m4 := result.(providerTUIModel)
	if m4.cpStep != cpStepName {
		t.Errorf("cpStep = %d, want %d", m4.cpStep, cpStepName)
	}
	if m4.formError == "" {
		t.Error("expected formError for duplicate name")
	}
	if !strings.Contains(m4.formError, "stepfun") {
		t.Errorf("formError = %q, want to mention stepfun", m4.formError)
	}

	result, _ = m4.Update(charKey('x'))
	m4b := result.(providerTUIModel)
	if m4b.formError != "" {
		t.Errorf("formError should clear on keystroke, got %q", m4b.formError)
	}

	m4b.cpNameInput.SetValue("stepfun2")
	result, _ = m4b.Update(enterKey())
	m5 := result.(providerTUIModel)
	if m5.cpStep != cpStepProtocol {
		t.Errorf("cpStep = %d, want %d", m5.cpStep, cpStepProtocol)
	}
	if m5.formError != "" {
		t.Errorf("formError = %q, want empty after valid name", m5.formError)
	}
}

func TestProviderTUI_CustomFormRejectsInvalidAuthHeader(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{}
	m := newProviderTUI(cfg, configPath)

	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)

	m3.cpNameInput.SetValue("my-new")
	result, _ = m3.Update(enterKey())
	m4 := result.(providerTUIModel)
	result, _ = m4.Update(enterKey())
	m5 := result.(providerTUIModel)
	m5.cpURLInput.SetValue("https://api.example.com")
	result, _ = m5.Update(enterKey())
	m6 := result.(providerTUIModel)
	result, _ = m6.Update(enterKey())
	m7 := result.(providerTUIModel)
	if m7.cpStep != cpStepAuthHeader {
		t.Fatalf("cpStep = %d, want %d", m7.cpStep, cpStepAuthHeader)
	}

	for _, c := range "bad-header" {
		result, _ = m7.Update(charKey(c))
		m7 = result.(providerTUIModel)
	}
	result, _ = m7.Update(enterKey())
	m8 := result.(providerTUIModel)

	if m8.cpStep != cpStepAuthHeader {
		t.Errorf("cpStep = %d, want %d", m8.cpStep, cpStepAuthHeader)
	}
	if m8.formError == "" {
		t.Error("expected formError for invalid auth header")
	}
	if !strings.Contains(m8.formError, "Unsupported Auth Header") {
		t.Errorf("formError = %q, want unsupported auth header message", m8.formError)
	}
	if !m8.creatingCustom {
		t.Error("creatingCustom should remain true when validation fails")
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Error("config should not be saved for invalid auth header")
	}
}

func TestProviderTUI_CustomFormEditRejectsInvalidAuthHeader(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:        "https://api.example.com",
				Protocol:   "anthropic",
				AuthHeader: "authorization",
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.customIdx = 0
	m.enterEditCustomProvider()
	m.cpStep = cpStepAuthHeader
	m.cpAuthInput.SetValue("bad-header")
	m.cpAuthInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)

	if m2.cpStep != cpStepAuthHeader {
		t.Errorf("cpStep = %d, want %d", m2.cpStep, cpStepAuthHeader)
	}
	if m2.formError == "" {
		t.Error("expected formError for invalid auth header")
	}
	if !m2.editingCustom {
		t.Error("editingCustom should remain true when validation fails")
	}
	if got := cfg.CustomProviders["stepfun"].AuthHeader; got != "authorization" {
		t.Errorf("AuthHeader = %q, want unchanged %q", got, "authorization")
	}
}

func TestProviderTUI_EditCustomProviderSaveRejectsDuplicateRename(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:      "https://stepfun.example.com",
				Protocol: "anthropic",
			},
			"other": {
				URL:      "https://other.example.com",
				Protocol: "openai",
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.editingCustom = true
	m.editTargetName = "other"
	m.cpProtocolIdx = 1 // openai
	m.cpNameInput.SetValue("stepfun")
	m.cpURLInput.SetValue("https://other.example.com")

	err := m.applyEditCustomProviderSave()
	if err == nil {
		t.Fatal("expected error when renaming to existing provider name")
	}
	if !strings.Contains(m.formError, "stepfun") {
		t.Errorf("formError = %q, want to mention stepfun", m.formError)
	}
	if _, ok := cfg.CustomProviders["other"]; !ok {
		t.Error("original provider 'other' should still exist")
	}
	if cfg.CustomProviders["other"].URL != "https://other.example.com" {
		t.Errorf("provider 'other' URL = %q, want unchanged", cfg.CustomProviders["other"].URL)
	}
}

func TestProviderTUI_CustomFormCreateReturnsToModelList(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{}
	m := newProviderTUI(cfg, configPath)

	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)

	m3.cpNameInput.SetValue("my-new")
	result, _ = m3.Update(enterKey()) // name -> protocol
	m4 := result.(providerTUIModel)
	result, _ = m4.Update(enterKey()) // protocol -> URL
	m5 := result.(providerTUIModel)
	m5.cpURLInput.SetValue("https://api.example.com")
	result, _ = m5.Update(enterKey()) // URL -> API key
	m6 := result.(providerTUIModel)
	m6.apiKeyInput.SetValue("key-123")
	result, _ = m6.Update(enterKey()) // API key -> auth header
	m7 := result.(providerTUIModel)
	result, cmd := m7.Update(enterKey()) // auth header -> save
	m8 := result.(providerTUIModel)

	if cmd != nil {
		t.Error("create should not quit TUI")
	}
	if m8.creatingCustom {
		t.Error("creatingCustom should be false after create")
	}
	// Create should drop the user into the model selection step for the new
	// provider so they can pick/add a model right away.
	if m8.step != stepModel {
		t.Errorf("step = %d, want stepModel", m8.step)
	}
	if len(m8.customProviders) != 1 {
		t.Fatalf("expected 1 custom provider, got %d", len(m8.customProviders))
	}
	if m8.customProviders[0].name != "my-new" {
		t.Errorf("provider name = %q, want %q", m8.customProviders[0].name, "my-new")
	}
	if cfg.Provider != "" {
		t.Error("active provider should not be set when only creating")
	}
	if !m8.savedInSession {
		t.Error("savedInSession should be true after create")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config should be saved: %v", err)
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
	m := newProviderTUI(cfg, "")

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
	m := newProviderTUI(cfg, "")

	// Enter on existing custom provider should go to model selection first.
	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepModel {
		t.Errorf("step = %d, want %d (stepModel)", m2.step, stepModel)
	}
	gotModels := m2.models()
	if len(gotModels) != 2 || gotModels[0] != "custom-model" || gotModels[1] != "custom-fast" {
		t.Errorf("models = %v, want [custom-model custom-fast] (config order)", gotModels)
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

// --- Delete custom provider tests ---

func dKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'd'}
}

func yKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'y'}
}

func nKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'n'}
}

func TestProviderTUI_DeleteCustomProvider(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "https://custom.api/v1", Protocol: "openai", Model: "custom-model"},
		},
	}
	m := newProviderTUI(cfg, "")

	// Switch to custom tab
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	if m2.activeTab != tabCustom {
		t.Fatalf("tab = %d, want %d", m2.activeTab, tabCustom)
	}

	// Select the existing provider (index 0), press d
	m2.customIdx = 0
	result, _ = m2.Update(dKey())
	m3 := result.(providerTUIModel)
	if !m3.confirmingDelete {
		t.Fatal("pressing d should set confirmingDelete = true")
	}
	if m3.deleteTargetName != "my-llm" {
		t.Errorf("deleteTargetName = %q, want %q", m3.deleteTargetName, "my-llm")
	}

	// Confirm with y
	result, _ = m3.Update(yKey())
	m4 := result.(providerTUIModel)
	if m4.confirmingDelete {
		t.Error("confirmingDelete should be false after y")
	}
	if len(m4.deletedProviders) != 1 || m4.deletedProviders[0] != "my-llm" {
		t.Errorf("deletedProviders = %v, want [my-llm]", m4.deletedProviders)
	}
	if len(m4.customProviders) != 0 {
		t.Errorf("customProviders length = %d, want 0", len(m4.customProviders))
	}
}

func TestProviderTUI_DeleteCustomProviderCancel(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "https://custom.api/v1", Protocol: "openai", Model: "custom-model"},
		},
	}
	m := newProviderTUI(cfg, "")

	// Force custom tab so this test is independent of init-time tab routing.
	// Switch to custom tab, select provider, press d
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	m2.customIdx = 0
	result, _ = m2.Update(dKey())
	m3 := result.(providerTUIModel)
	if !m3.confirmingDelete {
		t.Fatal("should be confirming delete")
	}

	// Cancel with n
	result, _ = m3.Update(nKey())
	m4 := result.(providerTUIModel)
	if m4.confirmingDelete {
		t.Error("confirmingDelete should be false after n")
	}
	if len(m4.deletedProviders) != 0 {
		t.Error("deletedProviders should be empty after cancel")
	}
	if len(m4.customProviders) != 1 {
		t.Error("customProviders should still have 1 entry after cancel")
	}
}

func TestProviderTUI_DeleteOnAddOptionIgnored(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "https://custom.api/v1", Protocol: "openai"},
		},
	}
	m := newProviderTUI(cfg, "")

	// Switch to custom tab
	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)

	// Move to "Add" option (index 1, since there's 1 provider)
	m2.customIdx = len(m2.customProviders)
	result, _ = m2.Update(dKey())
	m3 := result.(providerTUIModel)
	if m3.confirmingDelete {
		t.Error("pressing d on Add option should not trigger delete confirmation")
	}
}

func TestProviderTUI_DeleteActiveCustomProvider(t *testing.T) {
	cfg := &Config{
		Provider: "my-llm",
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "https://custom.api/v1", Protocol: "openai", Model: "custom-model"},
		},
	}
	m := newProviderTUI(cfg, "")

	// Should auto-select custom tab with active provider
	if m.activeTab != tabCustom {
		t.Fatalf("should auto-select custom tab, got %d", m.activeTab)
	}

	// Press d on the active provider
	m.customIdx = 0
	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if !m2.confirmingDelete {
		t.Fatal("should be confirming delete")
	}

	// Confirm
	result, _ = m2.Update(yKey())
	m3 := result.(providerTUIModel)
	if len(m3.deletedProviders) != 1 || m3.deletedProviders[0] != "my-llm" {
		t.Errorf("deletedProviders = %v, want [my-llm]", m3.deletedProviders)
	}
}

func TestProviderTUI_DeleteEscCancels(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "https://custom.api/v1", Protocol: "openai"},
		},
	}
	m := newProviderTUI(cfg, "")

	result, _ := m.Update(rightKey())
	m2 := result.(providerTUIModel)
	m2.customIdx = 0
	result, _ = m2.Update(dKey())
	m3 := result.(providerTUIModel)

	// Esc should cancel confirmation
	result, _ = m3.Update(escKey())
	m4 := result.(providerTUIModel)
	if m4.confirmingDelete {
		t.Error("Esc should cancel delete confirmation")
	}
	if len(m4.deletedProviders) != 0 {
		t.Error("no providers should be deleted after Esc")
	}
}

func TestActiveModelForProvider_PrefersEntryModel(t *testing.T) {
	cfg := &Config{Provider: "stepfun", Model: "step-3.7-flash"}
	entry := ProviderEntry{Model: "step-3.5-flash"}
	got := activeModelForProvider(cfg, "stepfun", entry)
	if got != "step-3.5-flash" {
		t.Errorf("got %q, want step-3.5-flash", got)
	}
}

func TestActiveModelForProvider_FallsBackToCfgModel(t *testing.T) {
	cfg := &Config{Provider: "stepfun", Model: "step-3.5-flash"}
	entry := ProviderEntry{}
	got := activeModelForProvider(cfg, "stepfun", entry)
	if got != "step-3.5-flash" {
		t.Errorf("got %q, want step-3.5-flash", got)
	}
}

func TestProviderTUI_CustomModelInput_AddsSingleName(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:    "https://api.stepfun.com/v1",
				Model:  "step-3.5-flash",
				Models: []string{"step-3.5-flash"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepModel
	m.modelIdx = len(m.models()) // land on "Enter custom model name..."
	m.customModel = true
	m.modelInput.SetValue("newmodel")
	m.modelInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)

	if m2.customModel {
		t.Error("customModel should be cleared after Enter")
	}
	if m2.formError != "" {
		t.Errorf("formError = %q, want empty", m2.formError)
	}
	got := m2.existingCfg.CustomProviders["stepfun"].Models
	want := []string{"step-3.5-flash", "newmodel"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Models = %v, want %v", got, want)
	}
	if !m2.savedInSession {
		t.Error("savedInSession should be true after add")
	}

	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load disk config: %v", err)
	}
	diskModels := diskCfg.CustomProviders["stepfun"].Models
	if len(diskModels) != 2 || diskModels[1] != "newmodel" {
		t.Errorf("disk Models = %v, want last=step-3.5-flash,newmodel", diskModels)
	}
}

func TestProviderTUI_CustomModelInput_RejectsDuplicate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:    "https://api.stepfun.com/v1",
				Model:  "step-3.5-flash",
				Models: []string{"step-3.5-flash"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepModel
	m.modelIdx = len(m.models())
	m.customModel = true
	m.modelInput.SetValue("step-3.5-flash")
	m.modelInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)

	if !m2.customModel {
		t.Error("customModel should stay true after duplicate reject")
	}
	if m2.formError != "Already in list: step-3.5-flash" {
		t.Errorf("formError = %q, want %q", m2.formError, "Already in list: step-3.5-flash")
	}
	if m2.modelInput.Value() != "step-3.5-flash" {
		t.Errorf("input should be preserved on dup; got %q", m2.modelInput.Value())
	}
	if len(m2.existingCfg.CustomProviders["stepfun"].Models) != 1 {
		t.Errorf("Models mutated: %v", m2.existingCfg.CustomProviders["stepfun"].Models)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("disk file should not exist; duplicate did not persist")
	}
	if m2.savedInSession {
		t.Error("savedInSession should be false after rejected duplicate")
	}
}

func TestProviderTUI_ManualFormPassesKToAuthHeaderInput(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{Llm: LlmConfig{URL: "https://example.com/v1", Model: "m", AuthToken: "k"}}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabManual
	m.inManualForm = true
	m.manualStep = manualStepAuthHeader
	m.manualAuthHeaderInput.Focus()

	result, _ := m.Update(charKey('x'))
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(charKey('-'))
	m3 := result.(providerTUIModel)
	result, _ = m3.Update(charKey('a'))
	m4 := result.(providerTUIModel)
	result, _ = m4.Update(charKey('p'))
	m5 := result.(providerTUIModel)
	result, _ = m5.Update(charKey('i'))
	m6 := result.(providerTUIModel)
	result, _ = m6.Update(charKey('-'))
	m7 := result.(providerTUIModel)
	result, _ = m7.Update(charKey('k'))
	m8 := result.(providerTUIModel)
	result, _ = m8.Update(charKey('e'))
	m9 := result.(providerTUIModel)
	result, _ = m9.Update(charKey('y'))
	m10 := result.(providerTUIModel)

	if got := m10.manualAuthHeaderInput.Value(); got != "x-api-key" {
		t.Errorf("manualAuthHeaderInput.Value() = %q, want %q", got, "x-api-key")
	}
}

func TestProviderTUI_CustomFormPassesKToAuthHeaderInput(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{}
	m := newProviderTUI(cfg, configPath)
	m.creatingCustom = true
	m.cpStep = cpStepAuthHeader
	m.cpAuthInput.Focus()

	result, _ := m.Update(charKey('k'))
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(charKey('e'))
	m3 := result.(providerTUIModel)
	result, _ = m3.Update(charKey('y'))
	m4 := result.(providerTUIModel)

	if got := m4.cpAuthInput.Value(); got != "key" {
		t.Errorf("cpAuthInput.Value() = %q, want %q", got, "key")
	}
}

func TestProviderTUI_DeleteModelPreservesActiveModel(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				Model:  "step-3.5-flash",
				Models: []string{"step-3.5-flash", "aaa"},
			},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepModel
	m.modelIdx = 1 // aaa

	m.confirmingDeleteModel = true
	m.deleteModelName = "aaa"
	result, _ := m.Update(yKey())
	m2 := result.(providerTUIModel)

	if m2.existingCfg.CustomProviders["stepfun"].Model != "step-3.5-flash" {
		t.Errorf("entry.Model = %q, want step-3.5-flash", m2.existingCfg.CustomProviders["stepfun"].Model)
	}
	if m2.existingCfg.Model != "step-3.5-flash" {
		t.Errorf("cfg.Model = %q, want step-3.5-flash", m2.existingCfg.Model)
	}
	if !m2.savedInSession {
		t.Error("savedInSession should be true after deleting a model")
	}
}

func TestApplyCustomProviderConfigPreservesModelOrder(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	models := []string{"test-model", "test-model-2", "bbb", "aaa", "test-model-3"}
	cfg := &Config{
		Provider: "test-provider",
		Model:    "test-model-2",
		CustomProviders: map[string]ProviderEntry{
			"test-provider": {
				Model:  "test-model-2",
				Models: append([]string(nil), models...),
			},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	result := providerTUIResult{
		provider: "test-provider",
		model:    "test-model-3",
		models:   append([]string(nil), models...),
		isCustom: true,
		isEdit:   true,
	}
	if err := applyCustomProviderConfig(configPath, cfg, result); err != nil {
		t.Fatalf("applyCustomProviderConfig: %v", err)
	}

	got := cfg.CustomProviders["test-provider"].Models
	if len(got) != len(models) {
		t.Fatalf("Models length = %d, want %d: %v", len(got), len(models), got)
	}
	for i := range models {
		if got[i] != models[i] {
			t.Errorf("Models[%d] = %q, want %q", i, got[i], models[i])
		}
	}
	if cfg.CustomProviders["test-provider"].Model != "test-model-3" {
		t.Errorf("entry.Model = %q, want test-model-3", cfg.CustomProviders["test-provider"].Model)
	}
}

func TestApplyManualConfigNormalizesAuthHeader(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{}

	result := providerTUIResult{
		isManual:   true,
		url:        "https://example.com/v1",
		model:      "test-model",
		apiKey:     "token",
		protocol:   "anthropic",
		authHeader: "X-Api-Key",
	}
	if err := applyManualConfig(configPath, cfg, result); err != nil {
		t.Fatalf("applyManualConfig: %v", err)
	}
	if got := cfg.Llm.AuthHeader; got != "x-api-key" {
		t.Errorf("Llm.AuthHeader = %q, want %q", got, "x-api-key")
	}
	useAnthropic := true
	if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic != useAnthropic {
		t.Errorf("UseAnthropic = %v, want %v", cfg.Llm.UseAnthropic, useAnthropic)
	}
}

func TestApplyCustomProviderConfigNormalizesAuthHeader(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"test-provider": {URL: "https://example.com", Model: "m"},
		},
	}

	result := providerTUIResult{
		provider:   "test-provider",
		model:      "m",
		url:        "https://example.com",
		protocol:   "anthropic",
		authHeader: "Authorization",
		isCustom:   true,
		isEdit:     true,
	}
	if err := applyCustomProviderConfig(configPath, cfg, result); err != nil {
		t.Fatalf("applyCustomProviderConfig: %v", err)
	}
	if got := cfg.CustomProviders["test-provider"].AuthHeader; got != "authorization" {
		t.Errorf("AuthHeader = %q, want %q", got, "authorization")
	}
}
