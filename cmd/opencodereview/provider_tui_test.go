package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/open-code-review/open-code-review/internal/llm"
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

	result, cmd := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.manualStep != manualStepAuthToken {
		t.Errorf("should stay on auth token step, got %d", m2.manualStep)
	}
	if m2.formError != manualAuthTokenRequiredError {
		t.Errorf("formError = %q, want %q", m2.formError, manualAuthTokenRequiredError)
	}
	if cmd != nil {
		t.Error("Enter with empty token should not quit")
	}
}

func TestProviderTUI_ManualFormRejectsWhitespaceOnlyToken(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.inManualForm = true
	m.manualStep = manualStepAuthToken
	m.manualTokenInput.SetValue("   ")
	m.manualTokenInput.Focus()

	result, cmd := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.manualStep != manualStepAuthToken {
		t.Errorf("should stay on auth token step, got %d", m2.manualStep)
	}
	if m2.formError != manualAuthTokenRequiredError {
		t.Errorf("formError = %q, want %q", m2.formError, manualAuthTokenRequiredError)
	}
	if cmd != nil {
		t.Error("Enter with whitespace-only token should not quit")
	}
}

func TestProviderTUI_SessionModelPickSurvivesOfficialProviderSwitch(t *testing.T) {
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "baidu-qianfan" {
			m.officialIdx = i
			break
		}
	}

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	m2.modelIdx = modelIdxForName(t, m2, "glm-5")

	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if got := m3.sessionModelPick["baidu-qianfan"]; got != "glm-5" {
		t.Errorf("sessionModelPick = %q, want glm-5", got)
	}

	result, _ = m3.Update(escKey())
	m4 := result.(providerTUIModel)
	result, _ = m4.Update(escKey())
	m5 := result.(providerTUIModel)

	result, _ = m5.Update(enterKey())
	m6 := result.(providerTUIModel)
	if got := m6.models()[m6.modelIdx]; got != "glm-5" {
		t.Errorf("model cursor = %q, want glm-5", got)
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

func TestApplyEditCustomProviderSave_ClearsAPIKeyWhenEditedEmpty(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"aaa": {
				URL:      "https://example.com/v1",
				Protocol: "anthropic",
				APIKey:   "old-saved-key",
				Model:    "test",
				Models:   []string{"test"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.editingCustom = true
	m.editTargetName = "aaa"
	m.cpProtocolIdx = 0
	m.cpNameInput.SetValue("aaa")
	m.cpURLInput.SetValue("https://example.com/v1")
	m.beginAPIKeyReplace()

	if err := m.applyEditCustomProviderSave(); err != nil {
		t.Fatalf("applyEditCustomProviderSave: %v", err)
	}
	if got := cfg.CustomProviders["aaa"].APIKey; got != "" {
		t.Errorf("APIKey = %q, want empty", got)
	}
	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := diskCfg.CustomProviders["aaa"].APIKey; got != "" {
		t.Errorf("persisted APIKey = %q, want empty", got)
	}
}

func TestApplyEditCustomProviderSave_PreservesAPIKeyWhenMasked(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"aaa": {
				URL:      "https://example.com/v1",
				Protocol: "anthropic",
				APIKey:   "keep-me",
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.customIdx = 0
	m.enterEditCustomProvider()

	if err := m.applyEditCustomProviderSave(); err != nil {
		t.Fatalf("applyEditCustomProviderSave: %v", err)
	}
	if got := cfg.CustomProviders["aaa"].APIKey; got != "keep-me" {
		t.Errorf("APIKey = %q, want keep-me", got)
	}
}

func TestProviderTUI_EditCustomClearKey_NoMaskedOnStepAPIKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"aaa": {
				URL:      "https://example.com/v1",
				Protocol: "anthropic",
				APIKey:   "old-saved-key",
				Model:    "test",
				Models:   []string{"test", "aaa"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	for i, cp := range m.customProviders {
		if cp.name == "aaa" {
			m.customIdx = i
			break
		}
	}
	m.enterEditCustomProvider()
	m.cpStep = cpStepAPIKey
	m.beginAPIKeyReplace()
	m.cpStep = cpStepAuthHeader
	m.cpAuthInput.SetValue("")
	m.cpAuthInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepModel {
		t.Fatalf("step = %d, want stepModel", m2.step)
	}
	if got := m2.customProviders[m2.customIdx].entry.APIKey; got != "" {
		t.Errorf("saved APIKey = %q, want empty", got)
	}

	m2.modelIdx = modelIdxForName(t, m2, "test")
	result, _ = m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if m3.step != stepAPIKey {
		t.Fatalf("step = %d, want stepAPIKey", m3.step)
	}
	if m3.apiKeyMasked {
		t.Error("apiKeyMasked should be false after clearing key in edit")
	}
	got := stripANSI(m3.View().Content)
	if strings.Contains(got, "Type or paste to replace the saved key") {
		t.Errorf("view should not show replace hint; got:\n%s", got)
	}
}

func TestProviderTUI_ReenterEditAfterClearKey_ShowsEmpty(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"aaa": {
				URL:      "https://example.com/v1",
				Protocol: "anthropic",
				APIKey:   "old-saved-key",
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.customIdx = 0
	m.editingCustom = true
	m.editTargetName = "aaa"
	m.cpProtocolIdx = 0
	m.cpNameInput.SetValue("aaa")
	m.cpURLInput.SetValue("https://example.com/v1")
	m.beginAPIKeyReplace()
	if err := m.applyEditCustomProviderSave(); err != nil {
		t.Fatalf("applyEditCustomProviderSave: %v", err)
	}

	m.enterEditCustomProvider()
	if m.apiKeyMasked {
		t.Error("apiKeyMasked should be false when key was cleared")
	}
	if got := m.apiKeyInput.Value(); got != "" {
		t.Errorf("apiKeyInput = %q, want empty", got)
	}
}

func TestCustomAPIKeyForSave(t *testing.T) {
	m := providerTUIModel{
		apiKeyMasked:   true,
		apiKeyOriginal: "keep-me",
	}
	key, edited := m.customAPIKeyForSave()
	if edited {
		t.Fatal("masked key should not count as edited")
	}
	if key != "keep-me" {
		t.Errorf("key = %q, want keep-me", key)
	}

	m.apiKeyMasked = false
	m.apiKeyInput.SetValue("  ")
	key, edited = m.customAPIKeyForSave()
	if !edited {
		t.Fatal("cleared field should count as edited")
	}
	if key != "" {
		t.Errorf("key = %q, want empty", key)
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

func TestProviderTUI_OfficialTab_CustomModelInput_PersistsName(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {
				Model:  "qwen3.7-max",
				Models: []string{"qwen3.7-max"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabOfficial

	// Land the cursor on the official provider we configured.
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepModel
	m.modelIdx = len(m.models()) // "Enter custom model name..."
	m.customModel = true
	m.modelInput.SetValue("my-custom-model")
	m.modelInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)

	if m2.customModel {
		t.Error("customModel should be cleared after Enter")
	}
	if m2.formError != "" {
		t.Errorf("formError = %q, want empty", m2.formError)
	}
	if !m2.savedInSession {
		t.Error("savedInSession should be true after successful add")
	}
	got := m2.existingCfg.Providers["dashscope"].Models
	want := []string{"qwen3.7-max", "my-custom-model"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("official Models = %v, want %v", got, want)
	}

	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load disk config: %v", err)
	}
	diskModels := diskCfg.Providers["dashscope"].Models
	if len(diskModels) != 2 || diskModels[1] != "my-custom-model" {
		t.Errorf("disk Models = %v, want [qwen3.7-max my-custom-model]", diskModels)
	}
}

func TestProviderTUI_OfficialTab_CustomModelInput_RejectsDuplicate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {
				Model:  "qwen3.7-max",
				Models: []string{"qwen3.7-max"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepModel
	m.modelIdx = len(m.models())
	m.customModel = true
	m.modelInput.SetValue("qwen3.7-max")
	m.modelInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)

	if !m2.customModel {
		t.Error("customModel should stay true after duplicate reject")
	}
	if m2.formError != "Already in list: qwen3.7-max" {
		t.Errorf("formError = %q, want %q", m2.formError, "Already in list: qwen3.7-max")
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("disk file should not exist; duplicate did not persist")
	}
}

func officialDashscopeModelTUI(t *testing.T, configPath string, extraModels []string) providerTUIModel {
	t.Helper()
	models := []string{"qwen3.7-max"}
	models = append(models, extraModels...)
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {
				Model:  "qwen3.7-max",
				Models: models,
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepModel
	return m
}

func customStepfunModelTUI(t *testing.T, configPath string, models []string) providerTUIModel {
	t.Helper()
	if len(models) == 0 {
		models = []string{"step-3.5-flash"}
	}
	cfg := &Config{
		Provider: "stepfun",
		Model:    models[0],
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:      "https://api.stepfun.com/v1",
				Protocol: "openai",
				Model:    models[0],
				Models:   models,
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	for i, cp := range m.customProviders {
		if cp.name == "stepfun" {
			m.customIdx = i
			break
		}
	}
	m.step = stepModel
	return m
}

func modelIdxForName(t *testing.T, m providerTUIModel, name string) int {
	t.Helper()
	for i, model := range m.models() {
		if model == name {
			return i
		}
	}
	t.Fatalf("model %q not found in %v", name, m.models())
	return -1
}

func TestProviderTUI_OfficialTab_DeleteUserAddedModel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := officialDashscopeModelTUI(t, configPath, []string{"my-custom-model"})
	m.modelIdx = modelIdxForName(t, m, "my-custom-model")

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if !m2.confirmingDeleteModel {
		t.Fatal("pressing d on user-added model should set confirmingDeleteModel = true")
	}
	if m2.deleteModelName != "my-custom-model" {
		t.Errorf("deleteModelName = %q, want my-custom-model", m2.deleteModelName)
	}

	result, _ = m2.Update(yKey())
	m3 := result.(providerTUIModel)
	if m3.confirmingDeleteModel {
		t.Error("confirmingDeleteModel should be false after y")
	}
	got := m3.existingCfg.Providers["dashscope"].Models
	if len(got) != 1 || got[0] != "qwen3.7-max" {
		t.Errorf("Models = %v, want [qwen3.7-max]", got)
	}
	if !m3.savedInSession {
		t.Error("savedInSession should be true after delete")
	}

	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load disk config: %v", err)
	}
	if len(diskCfg.Providers["dashscope"].Models) != 1 {
		t.Errorf("disk Models = %v, want [qwen3.7-max]", diskCfg.Providers["dashscope"].Models)
	}
}

func TestProviderTUI_OfficialTab_DeleteBuiltInModelIgnored(t *testing.T) {
	m := officialDashscopeModelTUI(t, "", []string{"my-custom-model"})
	m.modelIdx = modelIdxForName(t, m, "qwen3.7-max")

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on built-in model should not trigger delete confirmation")
	}
}

func TestProviderTUI_OfficialTab_RegistryModelNotDeletable(t *testing.T) {
	m := officialDashscopeModelTUI(t, "", []string{"my-custom-model"})
	m.modelIdx = modelIdxForName(t, m, "qwen3.7-max")

	if m.isUserAddedOfficialModel("qwen3.7-max") {
		t.Error("qwen3.7-max should not be user-added when it is in the registry")
	}

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on registry model should not trigger delete confirmation")
	}
}

func TestProviderTUI_OfficialTab_DeleteOnCustomModelInputIgnored(t *testing.T) {
	m := officialDashscopeModelTUI(t, "", []string{"my-custom-model"})
	m.modelIdx = len(m.models())

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on Enter custom model name... should not trigger delete confirmation")
	}
}

func TestProviderTUI_CustomTab_DeleteOnCustomModelInputIgnored(t *testing.T) {
	m := customStepfunModelTUI(t, "", []string{"step-3.5-flash", "aaa"})
	m.modelIdx = len(m.models())

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on Enter custom model name... should not trigger delete confirmation")
	}
}

func TestProviderTUI_CustomTab_ModelShowsDeleteHint(t *testing.T) {
	m := customStepfunModelTUI(t, "", []string{"step-3.5-flash", "aaa"})

	m.modelIdx = len(m.models())
	got := stripANSI(m.View().Content)
	if strings.Contains(got, "d Delete") {
		t.Errorf("custom input row should not show d Delete hint; got:\n%s", got)
	}

	m.modelIdx = modelIdxForName(t, m, "aaa")
	got = stripANSI(m.View().Content)
	if !strings.Contains(got, "d Delete") {
		t.Errorf("custom model row should show d Delete hint; got:\n%s", got)
	}
}

func TestProviderTUI_OfficialTab_UserAddedModelShowsDeleteHint(t *testing.T) {
	m := officialDashscopeModelTUI(t, "", []string{"my-custom-model"})

	m.modelIdx = modelIdxForName(t, m, "qwen3.7-max")
	got := stripANSI(m.View().Content)
	if strings.Contains(got, "d Delete") {
		t.Errorf("built-in model should not show d Delete hint; got:\n%s", got)
	}

	m.modelIdx = modelIdxForName(t, m, "my-custom-model")
	got = stripANSI(m.View().Content)
	if !strings.Contains(got, "d Delete") {
		t.Errorf("user-added model should show d Delete hint; got:\n%s", got)
	}
}

func TestProviderTUI_OfficialTab_DeleteModelPreservesActiveModel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := officialDashscopeModelTUI(t, configPath, []string{"my-custom-model"})
	m.existingCfg.Model = "qwen3.7-max"
	m.existingCfg.Providers["dashscope"] = ProviderEntry{
		Model:  "qwen3.7-max",
		Models: []string{"qwen3.7-max", "my-custom-model"},
	}
	m.modelIdx = modelIdxForName(t, m, "my-custom-model")

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(yKey())
	m3 := result.(providerTUIModel)

	if m3.existingCfg.Providers["dashscope"].Model != "qwen3.7-max" {
		t.Errorf("entry.Model = %q, want qwen3.7-max", m3.existingCfg.Providers["dashscope"].Model)
	}
	if m3.existingCfg.Model != "qwen3.7-max" {
		t.Errorf("cfg.Model = %q, want qwen3.7-max", m3.existingCfg.Model)
	}
}

func TestProviderTUI_OfficialTab_DeleteUserAddedModelCancel(t *testing.T) {
	cancelKeys := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"n", nKey()},
		{"esc", escKey()},
	}
	for _, tc := range cancelKeys {
		t.Run(tc.name, func(t *testing.T) {
			m := officialDashscopeModelTUI(t, "", []string{"my-custom-model"})
			m.modelIdx = modelIdxForName(t, m, "my-custom-model")

			result, _ := m.Update(dKey())
			m2 := result.(providerTUIModel)
			if !m2.confirmingDeleteModel {
				t.Fatal("expected confirmingDeleteModel after d")
			}

			result, _ = m2.Update(tc.key)
			m3 := result.(providerTUIModel)
			if m3.confirmingDeleteModel {
				t.Error("confirmingDeleteModel should be false after cancel")
			}
			got := m3.existingCfg.Providers["dashscope"].Models
			if len(got) != 2 || got[1] != "my-custom-model" {
				t.Errorf("Models = %v, want model unchanged", got)
			}
		})
	}
}

func TestProviderTUI_OfficialTab_DeleteActiveUserModelClearsCfg(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := officialDashscopeModelTUI(t, configPath, []string{"my-custom-model"})
	m.existingCfg.Model = "my-custom-model"
	m.existingCfg.Providers["dashscope"] = ProviderEntry{
		Model:  "my-custom-model",
		Models: []string{"qwen3.7-max", "my-custom-model"},
	}
	m.modelIdx = modelIdxForName(t, m, "my-custom-model")

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	result, _ = m2.Update(yKey())
	m3 := result.(providerTUIModel)

	if m3.existingCfg.Providers["dashscope"].Model != "" {
		t.Errorf("entry.Model = %q, want empty", m3.existingCfg.Providers["dashscope"].Model)
	}
	if m3.existingCfg.Model != "" {
		t.Errorf("cfg.Model = %q, want empty", m3.existingCfg.Model)
	}
}

func TestOfficialSelectedModelUsesRegistryNotStaleMerge(t *testing.T) {
	registry := []string{"built-in"}
	deletedCustom := "my-custom"
	staleMerged := mergeModelLists(registry, []string{deletedCustom})

	if llm.ModelListContains(registry, deletedCustom) {
		t.Fatal("test setup: custom name should not be in registry")
	}
	if !llm.ModelListContains(staleMerged, deletedCustom) {
		t.Fatal("test setup: stale merge should still list deleted custom name")
	}
	// runConfigModel must use registryModels, not staleMerged, or re-selected custom
	// names would skip ensureModelInList when still present in the pre-TUI merge.
	if llm.ModelListContains(staleMerged, deletedCustom) && llm.ModelListContains(registry, deletedCustom) {
		t.Error("would skip persisting re-selected custom model")
	}
	if llm.ModelListContains(registry, deletedCustom) {
		t.Error("registry-only check must not treat custom model as built-in")
	}
}

func TestProviderTUI_CustomTab_DeleteModelSkipsSavedInSessionWhenNoModelDeleted(t *testing.T) {
	m := customStepfunModelTUI(t, "", []string{"step-3.5-flash", "aaa"})
	m.modelIdx = len(m.models()) // out of range — no model row selected
	m.deleteModelName = "aaa"
	m.confirmingDeleteModel = true

	result, _ := m.confirmDeleteCustomModel()
	m2 := result.(providerTUIModel)
	if m2.savedInSession {
		t.Error("savedInSession should be false when modelIdx is out of range")
	}
}

func TestProviderTUI_CustomTab_DeleteModelViaDKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:      "https://api.stepfun.com/v1",
				Protocol: "openai",
				Model:    "step-3.5-flash",
				Models:   []string{"step-3.5-flash", "aaa"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	for i, cp := range m.customProviders {
		if cp.name == "stepfun" {
			m.customIdx = i
			break
		}
	}
	m.step = stepModel
	m.modelIdx = modelIdxForName(t, m, "aaa")

	result, _ := m.Update(dKey())
	m2 := result.(providerTUIModel)
	if !m2.confirmingDeleteModel || m2.deleteModelName != "aaa" {
		t.Fatalf("after d: confirming=%v deleteModelName=%q", m2.confirmingDeleteModel, m2.deleteModelName)
	}

	result, _ = m2.Update(yKey())
	m3 := result.(providerTUIModel)
	got := m3.existingCfg.CustomProviders["stepfun"].Models
	if len(got) != 1 || got[0] != "step-3.5-flash" {
		t.Errorf("Models = %v, want [step-3.5-flash]", got)
	}

	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load disk config: %v", err)
	}
	if len(diskCfg.CustomProviders["stepfun"].Models) != 1 {
		t.Errorf("disk Models = %v, want [step-3.5-flash]", diskCfg.CustomProviders["stepfun"].Models)
	}
}

func TestProviderTUI_PersistCustomModelName_SaveFailureRollsBack(t *testing.T) {
	blockPath := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	m := officialDashscopeModelTUI(t, blockPath, nil)
	before := append([]string(nil), m.existingCfg.Providers["dashscope"].Models...)
	m.modelIdx = len(m.models())
	m.customModel = true
	m.modelInput.SetValue("failed-model")
	m.modelInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.formError == "" {
		t.Fatal("expected formError on save failure")
	}
	if !strings.Contains(m2.formError, "failed to save") {
		t.Errorf("formError = %q, want save failure message", m2.formError)
	}
	got := m2.existingCfg.Providers["dashscope"].Models
	if len(got) != len(before) {
		t.Errorf("Models = %v, want unchanged %v", got, before)
	}
	if m2.savedInSession {
		t.Error("savedInSession should be false after failed persist")
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

func TestProviderTUI_ViewAPIKey_MaskedShowsReplaceHintAndLastFour(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {APIKey: "sk-secret-1234567890abcd"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	if !m.apiKeyMasked {
		t.Fatal("apiKeyMasked should be true when an existing key is loaded")
	}

	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "Type or paste to replace the saved key") {
		t.Errorf("view missing replace hint; got:\n%s", got)
	}
	if !strings.Contains(got, "(saved: sk-sec...abcd)") {
		t.Errorf("view missing prefix+suffix fingerprint; got:\n%s", got)
	}
}

func TestProviderTUI_ViewAPIKey_ShortKeyOmitsFingerprint(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {APIKey: "12345678901234"}, // 14 runes — below min length 15
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "Type or paste to replace the saved key") {
		t.Errorf("view missing replace hint; got:\n%s", got)
	}
	if strings.Contains(got, "(saved:") {
		t.Errorf("view should omit fingerprint for keys shorter than 15 runes; got:\n%s", got)
	}
}

func TestProviderTUI_ViewAPIKey_MinLenKeyShowsFingerprint(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {APIKey: "123456789012345"}, // 15 runes — at min length
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "(saved: 123456...2345)") {
		t.Errorf("view should show fingerprint at min length 15; got:\n%s", got)
	}
}

func TestSavedSecretFingerprint_TrimsLeadingWhitespace(t *testing.T) {
	const key = "sk-secret-1234567890abcd"
	got := savedSecretFingerprint("  " + key)
	want := "sk-sec...abcd"
	if got != want {
		t.Errorf("savedSecretFingerprint(%q) = %q, want %q", "  "+key, got, want)
	}
}

func TestProviderTUI_ViewAPIKey_FreshHidesReplaceHint(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()

	if m.apiKeyMasked {
		t.Fatal("apiKeyMasked should be false when no key is loaded")
	}

	got := stripANSI(m.View().Content)
	if strings.Contains(got, "Type or paste to replace the saved key") {
		t.Errorf("view should not show replace hint when fresh; got:\n%s", got)
	}
}

func TestOfficialAPIKeyEnvSetHint(t *testing.T) {
	const envVar = "DEEPSEEK_API_KEY"
	if got := officialAPIKeyEnvSetHint(envVar, false); got != "$DEEPSEEK_API_KEY is set. Leave empty to use it; enter a key here to override." {
		t.Errorf("no saved key hint = %q", got)
	}
	if got := officialAPIKeyEnvSetHint(envVar, true); got != "$DEEPSEEK_API_KEY is set; used only when no key is saved here." {
		t.Errorf("saved key hint = %q", got)
	}
}

func TestProviderTUI_ViewAPIKey_EnvSetNoSavedKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-from-env")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "deepseek" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	got := stripANSI(m.View().Content)
	want := "$DEEPSEEK_API_KEY is set. Leave empty to use it; enter a key here to override."
	if !strings.Contains(got, want) {
		t.Errorf("view missing env hint; want %q; got:\n%s", want, got)
	}
}

func TestProviderTUI_ViewAPIKey_EnvSetWithSavedKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-from-env")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {APIKey: "sk-secret-1234567890abcd", Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "deepseek" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "Type or paste to replace the saved key") {
		t.Errorf("view missing replace hint; got:\n%s", got)
	}
	want := "$DEEPSEEK_API_KEY is set; used only when no key is saved here."
	if !strings.Contains(got, want) {
		t.Errorf("view missing env hint; want %q; got:\n%s", want, got)
	}
	if strings.Contains(got, "Leave empty to use it") {
		t.Errorf("saved-key view should not show empty-env hint; got:\n%s", got)
	}
}

func TestProviderTUI_ApiKeyPasteReplacesMaskedKey(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:    "https://api.stepfun.com/v1",
				APIKey: "old-key-ssss",
				Model:  "step-3.5-flash",
				Models: []string{"step-3.5-flash"},
			},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	if !m.apiKeyMasked {
		t.Fatal("expected masked key on load")
	}
	if m.apiKeyInput.Value() != maskedSecretPlaceholder() {
		t.Fatalf("placeholder = %q, want fixed %d asterisks", m.apiKeyInput.Value(), maskedSecretDisplayLen)
	}

	result, _ := m.Update(tea.PasteMsg{Content: "sk-new-pasted-key"})
	m2 := result.(providerTUIModel)

	if m2.apiKeyMasked {
		t.Fatal("paste should unmask the field")
	}
	if got := m2.apiKeyInput.Value(); got != "sk-new-pasted-key" {
		t.Errorf("input value = %q, want pasted key", got)
	}
	if r := m2.result(); r.apiKey != "sk-new-pasted-key" {
		t.Errorf("result().apiKey = %q, want pasted key", r.apiKey)
	}
}

func TestProviderTUI_ManualTokenPasteReplacesMaskedToken(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{
			URL:       "https://example.com/v1",
			Model:     "m",
			AuthToken: "old-token-secret",
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabManual
	m.inManualForm = true
	m.manualStep = manualStepAuthToken
	m.manualTokenMasked = true
	m.manualTokenOriginal = "old-token-secret"
	m.manualTokenInput.SetValue(maskedSecretPlaceholder())
	m.manualTokenInput.Focus()

	if !m.manualTokenMasked {
		t.Fatal("expected masked token on load")
	}

	result, _ := m.Update(tea.PasteMsg{Content: "new-pasted-token"})
	m2 := result.(providerTUIModel)

	if m2.manualTokenMasked {
		t.Fatal("paste should unmask the token field")
	}
	if got := m2.manualTokenInput.Value(); got != "new-pasted-token" {
		t.Errorf("input value = %q, want pasted token", got)
	}
	if r := m2.result(); r.apiKey != "new-pasted-token" {
		t.Errorf("result().apiKey = %q, want pasted token", r.apiKey)
	}
}

func TestProviderTUI_ApiKeyTypingShowsOneStarPerChar(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {APIKey: "old-key-ssss", Model: "step-3.5-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	for _, ch := range "abc" {
		result, _ := m.Update(charKey(ch))
		m = result.(providerTUIModel)
	}
	if m.apiKeyInput.Value() != "abc" {
		t.Errorf("value = %q, want abc", m.apiKeyInput.Value())
	}
	// EchoPassword renders one '*' per character in the input view.
	masked := stripANSI(m.apiKeyInput.View())
	starCount := strings.Count(masked, "*")
	if starCount != 3 {
		t.Errorf("masked view has %d asterisks, want 3 (one * per char)", starCount)
	}
}

func TestProviderTUI_ApiKeyEnterWithoutEditKeepsOriginal(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {APIKey: "keep-me"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	if r := m.result(); r.apiKey != "keep-me" {
		t.Fatalf("before edit result().apiKey = %q", r.apiKey)
	}

	result, cmd := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if !m2.confirmed {
		t.Error("Enter without edit should confirm")
	}
	if cmd == nil {
		t.Error("Enter without edit should quit")
	}
	if r := m2.result(); r.apiKey != "keep-me" {
		t.Errorf("after Enter result().apiKey = %q, want keep-me", r.apiKey)
	}
}

func TestProviderTUI_ApiKeyClearSavedKeyReturnsEmpty(t *testing.T) {
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {APIKey: "old-saved-key", Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "deepseek" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.beginAPIKeyReplace()

	if r := m.result(); r.apiKey != "" {
		t.Errorf("result().apiKey = %q, want empty after clearing saved key", r.apiKey)
	}
}

func TestProviderTUI_ApiKeyResultTrimSpace(t *testing.T) {
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "deepseek" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.apiKeyInput.SetValue("   ")

	if r := m.result(); r.apiKey != "" {
		t.Errorf("result().apiKey = %q, want empty for whitespace-only input", r.apiKey)
	}
}

func TestProviderTUI_OfficialApiKeyEmptyWithoutEnvBlocksEnter(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	t.Setenv("DASHSCOPE_API_KEY", "")

	result, cmd := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepAPIKey {
		t.Errorf("step = %d, want stepAPIKey", m2.step)
	}
	if m2.formError != "API key is required (or set $DASHSCOPE_API_KEY)" {
		t.Errorf("formError = %q", m2.formError)
	}
	if cmd != nil {
		t.Error("Enter without key or env should not quit")
	}
}

func TestProviderTUI_OfficialApiKeyEmptyWithEnvAllowsEnter(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()

	t.Setenv("DASHSCOPE_API_KEY", "sk-from-env")

	result, cmd := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if !m2.confirmed {
		t.Error("Enter with env set should confirm")
	}
	if cmd == nil {
		t.Error("Enter with env set should quit")
	}
	if m2.formError != "" {
		t.Errorf("formError = %q, want empty", m2.formError)
	}
}

func TestProviderTUI_CustomExistingApiKeyEmptyBlocksEnter(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {APIKey: "old-key"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.apiKeyInput.Focus()
	m.beginAPIKeyReplace()

	result, cmd := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.step != stepAPIKey {
		t.Errorf("step = %d, want stepAPIKey", m2.step)
	}
	if m2.formError != "API key is required" {
		t.Errorf("formError = %q, want %q", m2.formError, "API key is required")
	}
	if cmd != nil {
		t.Error("Enter with cleared key should not quit")
	}
}

func TestProviderTUI_CustomCreateApiKeyOptional(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.creatingCustom = true
	m.cpStep = cpStepAPIKey
	m.apiKeyInput.SetValue("")
	m.apiKeyInput.Focus()

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.cpStep != cpStepAuthHeader {
		t.Errorf("cpStep = %d, want cpStepAuthHeader", m2.cpStep)
	}
	if m2.formError != "" {
		t.Errorf("formError = %q, want empty for optional API key", m2.formError)
	}
}

func TestProviderTUI_ViewAPIKey_ShowsFormError(t *testing.T) {
	cfg := &Config{
		Provider: "dashscope",
		Model:    "qwen3.7-max",
		Providers: map[string]ProviderEntry{
			"dashscope": {},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "dashscope" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepAPIKey
	m.loadExistingAPIKey()
	m.formError = "API key is required (or set $DASHSCOPE_API_KEY)"
	m.apiKeyInput.Focus()

	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "API key is required (or set $DASHSCOPE_API_KEY)") {
		t.Errorf("view missing formError; got:\n%s", got)
	}
}

func TestProviderTUI_CancelIncompleteOfficialProviderSwitch_NoPersistedChanges(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "baidu-qianfan" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepModel
	m.modelIdx = modelIdxForName(t, m, "glm-5")

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.savedInSession {
		t.Error("savedInSession should be false for cross-provider navigation")
	}
	if m2.step != stepAPIKey {
		t.Fatalf("step = %d, want stepAPIKey", m2.step)
	}

	result, _ = m2.Update(escKey())
	m3 := result.(providerTUIModel)
	result, _ = m3.Update(escKey())
	m4 := result.(providerTUIModel)
	result, cmd := m4.Update(escKey())
	m5 := result.(providerTUIModel)
	if !m5.cancelled {
		t.Error("expected cancelled = true")
	}
	if m5.savedInSession {
		t.Error("savedInSession should remain false after cancel")
	}
	if cmd == nil {
		t.Error("expected tea.Quit on final Esc")
	}
	if _, err := os.Stat(configPath); err == nil {
		diskCfg, err := loadOrCreateConfig(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if diskCfg.Provider != "deepseek" {
			t.Errorf("Provider = %q, want deepseek", diskCfg.Provider)
		}
		if entry, ok := diskCfg.Providers["baidu-qianfan"]; ok && entry.Model != "" {
			t.Errorf("baidu-qianfan model = %q, want no cross-provider draft persisted", entry.Model)
		}
	}
}

func TestProviderTUI_SameOfficialProviderModelChange_DefersPersistUntilConfirm(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabOfficial
	m.step = stepModel
	m.modelIdx = modelIdxForName(t, m, "deepseek-v4-pro")

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	if m2.savedInSession {
		t.Error("savedInSession should be false before API key confirm")
	}
	if m2.step != stepAPIKey {
		t.Fatalf("step = %d, want stepAPIKey", m2.step)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("config should not be written before wizard confirm")
	}
	if cfg.Model != "deepseek-v4-flash" {
		t.Errorf("cfg.Model = %q, want deepseek-v4-flash", cfg.Model)
	}
}

func TestProviderTUI_OfficialModelChangeBlockedAtAPIKey_KeepsGlobalModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "anthropic",
		Model:    "claude-opus-4-8",
		Providers: map[string]ProviderEntry{
			"anthropic": {
				Model:  "claude-opus-4-8",
				APIKey: "sk-test-key",
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabOfficial
	for i, p := range m.providers {
		if p.Name == "anthropic" {
			m.officialIdx = i
			break
		}
	}
	m.step = stepModel
	m.modelIdx = modelIdxForName(t, m, "claude-opus-4-7")

	result, _ := m.Update(enterKey())
	m2 := result.(providerTUIModel)
	m2.beginAPIKeyReplace()

	result, cmd := m2.Update(enterKey())
	m3 := result.(providerTUIModel)
	if cmd != nil {
		t.Error("Enter without key or env should not quit")
	}
	if m3.step != stepAPIKey {
		t.Fatalf("step = %d, want stepAPIKey", m3.step)
	}
	if cfg.Model != "claude-opus-4-8" {
		t.Errorf("cfg.Model = %q, want claude-opus-4-8", cfg.Model)
	}
	if got := cfg.Providers["anthropic"].Model; got != "claude-opus-4-8" {
		t.Errorf("providers.anthropic.Model = %q, want claude-opus-4-8", got)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("config should not be written when API key validation fails")
	}
}

// stripANSI removes ANSI escape sequences from a string so tests can assert
// against plain text content.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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

// --- protocol normalization / openai-responses support ---

func TestCpProtocols_ContainsAllCanonicalNames(t *testing.T) {
	// The slice drives both Custom and Manual forms — every canonical protocol
	// must appear, in canonical order, so the TUI result() picks up the right
	// string for each index.
	want := []string{
		llm.ProtocolAnthropic,
		llm.ProtocolOpenAIChatCompletions,
		llm.ProtocolOpenAIResponses,
	}
	if len(cpProtocols) != len(want) {
		t.Fatalf("cpProtocols has %d entries, want %d", len(cpProtocols), len(want))
	}
	for i, p := range want {
		if cpProtocols[i] != p {
			t.Errorf("cpProtocols[%d] = %q, want %q", i, cpProtocols[i], p)
		}
	}
}

func TestCpProtocolIndex(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		want     int
	}{
		{"canonical anthropic", llm.ProtocolAnthropic, 0},
		{"canonical chat-completions", llm.ProtocolOpenAIChatCompletions, 1},
		{"canonical responses", llm.ProtocolOpenAIResponses, 2},
		{"alias openai normalizes to chat-completions", "openai", 1},
		{"alias OPENAI case-insensitive", "OPENAI", 1},
		{"empty defaults to chat-completions", "", 1},
		{"unknown defaults to chat-completions", "grpc", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cpProtocolIndex(tt.protocol); got != tt.want {
				t.Errorf("cpProtocolIndex(%q) = %d, want %d", tt.protocol, got, tt.want)
			}
		})
	}
}

func TestNewProviderTUI_ManualProtocolFromConfig(t *testing.T) {
	t.Run("cfg.Llm.Protocol drives manualProtocolIdx (responses)", func(t *testing.T) {
		cfg := &Config{
			Llm: LlmConfig{
				URL:      "https://api.openai.com/v1",
				Protocol: llm.ProtocolOpenAIResponses,
				Model:    "gpt-5.4",
			},
		}
		m := newProviderTUI(cfg, "")
		if m.manualProtocolIdx != 2 {
			t.Errorf("manualProtocolIdx = %d, want 2 (openai-responses)", m.manualProtocolIdx)
		}
	})

	t.Run("cfg.Llm.Protocol alias openai normalizes", func(t *testing.T) {
		cfg := &Config{
			Llm: LlmConfig{
				URL:      "https://api.openai.com/v1",
				Protocol: "openai",
				Model:    "gpt-4o",
			},
		}
		m := newProviderTUI(cfg, "")
		if m.manualProtocolIdx != 1 {
			t.Errorf("manualProtocolIdx = %d, want 1 (openai)", m.manualProtocolIdx)
		}
	})

	t.Run("falls back to use_anthropic when Protocol empty", func(t *testing.T) {
		f := false
		cfg := &Config{
			Llm: LlmConfig{
				URL:          "https://api.openai.com/v1",
				UseAnthropic: &f,
				Model:        "gpt-4o",
			},
		}
		m := newProviderTUI(cfg, "")
		if m.manualProtocolIdx != 1 {
			t.Errorf("manualProtocolIdx = %d, want 1 (openai from use_anthropic=false)", m.manualProtocolIdx)
		}
	})
}

func TestEnterEditCustomProvider_ProtocolIndex(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		wantIdx  int
	}{
		{"anthropic", llm.ProtocolAnthropic, 0},
		{"openai", llm.ProtocolOpenAIChatCompletions, 1},
		{"openai-responses", llm.ProtocolOpenAIResponses, 2},
		{"legacy alias openai-chat-completions", "openai-chat-completions", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				CustomProviders: map[string]ProviderEntry{
					"cp": {URL: "https://x.example.com", Protocol: tt.protocol},
				},
			}
			m := newProviderTUI(cfg, "")
			m.activeTab = tabCustom
			m.customIdx = 0
			m.enterEditCustomProvider()
			if m.cpProtocolIdx != tt.wantIdx {
				t.Errorf("cpProtocolIdx = %d, want %d", m.cpProtocolIdx, tt.wantIdx)
			}
		})
	}
}

// TestApplyManualConfig_DoubleWritesProtocolAndUseAnthropic verifies the dual
// write: cfg.Llm.Protocol always gets the canonical name, and UseAnthropic is
// mirrored for the two protocols that have a boolean equivalent so older
// binaries can still read the config.
func TestApplyManualConfig_DoubleWritesProtocolAndUseAnthropic(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	t.Run("responses writes Protocol AND use_anthropic=false (openai-family fallback)", func(t *testing.T) {
		cfg := &Config{}
		result := providerTUIResult{
			isManual: true,
			url:      "https://example.com/v1",
			model:    "gpt-5.4",
			apiKey:   "tok",
			protocol: llm.ProtocolOpenAIResponses,
		}
		if err := applyManualConfig(configPath, cfg, result); err != nil {
			t.Fatalf("applyManualConfig: %v", err)
		}
		if cfg.Llm.Protocol != llm.ProtocolOpenAIResponses {
			t.Errorf("Protocol = %q, want %q", cfg.Llm.Protocol, llm.ProtocolOpenAIResponses)
		}
		// UseAnthropic falls back to false so older binaries use the OpenAI
		// family instead of wrongly defaulting to anthropic.
		if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic {
			t.Error("UseAnthropic should be false for responses protocol")
		}
	})

	t.Run("responses overwrites stale use_anthropic from prior protocol", func(t *testing.T) {
		stale := true
		cfg := &Config{}
		cfg.Llm.UseAnthropic = &stale // simulate switching from anthropic
		result := providerTUIResult{
			isManual: true,
			url:      "https://example.com/v1",
			model:    "gpt-5.4",
			apiKey:   "tok",
			protocol: llm.ProtocolOpenAIResponses,
		}
		if err := applyManualConfig(configPath, cfg, result); err != nil {
			t.Fatalf("applyManualConfig: %v", err)
		}
		if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic {
			t.Error("UseAnthropic should be false (overwriting stale true)")
		}
	})

	t.Run("anthropic writes Protocol AND use_anthropic=true (back-compat)", func(t *testing.T) {
		cfg := &Config{}
		result := providerTUIResult{
			isManual: true,
			url:      "https://example.com/v1",
			model:    "claude",
			apiKey:   "tok",
			protocol: llm.ProtocolAnthropic,
		}
		if err := applyManualConfig(configPath, cfg, result); err != nil {
			t.Fatalf("applyManualConfig: %v", err)
		}
		if cfg.Llm.Protocol != llm.ProtocolAnthropic {
			t.Errorf("Protocol = %q, want %q", cfg.Llm.Protocol, llm.ProtocolAnthropic)
		}
		if cfg.Llm.UseAnthropic == nil || !*cfg.Llm.UseAnthropic {
			t.Error("UseAnthropic should be mirrored to true for anthropic")
		}
	})

	t.Run("chat-completions writes Protocol AND use_anthropic=false (back-compat)", func(t *testing.T) {
		cfg := &Config{}
		result := providerTUIResult{
			isManual: true,
			url:      "https://example.com/v1",
			model:    "gpt-4o",
			apiKey:   "tok",
			protocol: llm.ProtocolOpenAIChatCompletions,
		}
		if err := applyManualConfig(configPath, cfg, result); err != nil {
			t.Fatalf("applyManualConfig: %v", err)
		}
		if cfg.Llm.Protocol != llm.ProtocolOpenAIChatCompletions {
			t.Errorf("Protocol = %q, want %q", cfg.Llm.Protocol, llm.ProtocolOpenAIChatCompletions)
		}
		if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic {
			t.Error("UseAnthropic should be mirrored to false for chat-completions")
		}
	})
}

// TestProviderTUIResult_ManualProtocolIsCanonical makes sure result() for the
// Manual tab returns the canonical protocol name picked from cpProtocols.
func TestProviderTUIResult_ManualProtocolIsCanonical(t *testing.T) {
	cfg := &Config{}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabManual
	m.inManualForm = true
	m.manualURLInput.SetValue("https://example.com/v1")
	m.manualModelInput.SetValue("m")
	m.manualTokenInput.SetValue("t")

	for i, want := range cpProtocols {
		m.manualProtocolIdx = i
		r := m.result()
		if r.protocol != want {
			t.Errorf("manualProtocolIdx=%d: result.protocol = %q, want %q", i, r.protocol, want)
		}
	}
}
