package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/open-code-review/open-code-review/internal/llm"
)

func TestCustomProviderActiveModel_NilCfg(t *testing.T) {
	m := providerTUIModel{existingCfg: nil}
	cp := customProviderListItem{name: "test", entry: ProviderEntry{Model: "m1"}}
	got := m.customProviderActiveModel(cp)
	if got != "" {
		t.Errorf("expected empty string for nil cfg, got %q", got)
	}
}

func TestCustomProviderActiveModel_DifferentProvider(t *testing.T) {
	cfg := &Config{Provider: "other-provider"}
	m := newProviderTUI(cfg, "")
	cp := customProviderListItem{name: "test", entry: ProviderEntry{Model: "m1"}}
	got := m.customProviderActiveModel(cp)
	if got != "" {
		t.Errorf("expected empty string for different provider, got %q", got)
	}
}

func TestCustomProviderActiveModel_MatchingProvider(t *testing.T) {
	cfg := &Config{
		Provider: "my-custom",
		Model:    "gpt-4",
		CustomProviders: map[string]ProviderEntry{
			"my-custom": {URL: "http://localhost", Model: "gpt-4"},
		},
	}
	m := newProviderTUI(cfg, "")
	cp := customProviderListItem{name: "my-custom", entry: ProviderEntry{URL: "http://localhost"}}
	got := m.customProviderActiveModel(cp)
	if got != "gpt-4" {
		t.Errorf("expected gpt-4, got %q", got)
	}
}

func TestOfficialProviderActiveModel_NilCfg(t *testing.T) {
	m := providerTUIModel{existingCfg: nil}
	got := m.officialProviderActiveModel(llm.Provider{Name: "anthropic"})
	if got != "" {
		t.Errorf("expected empty string for nil cfg, got %q", got)
	}
}

func TestOfficialProviderActiveModel_DifferentProvider(t *testing.T) {
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	got := m.officialProviderActiveModel(llm.Provider{Name: "anthropic", DisplayName: "Anthropic Claude API"})
	if got != "" {
		t.Errorf("expected empty string for non-active provider, got %q", got)
	}
}

func TestOfficialProviderActiveModel_MatchingProvider(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		Model:    "claude-opus-4-8",
		Providers: map[string]ProviderEntry{
			"anthropic": {Model: "claude-opus-4-8"},
		},
	}
	m := newProviderTUI(cfg, "")
	got := m.officialProviderActiveModel(llm.Provider{Name: "anthropic", DisplayName: "Anthropic Claude API"})
	if got != "claude-opus-4-8" {
		t.Errorf("expected claude-opus-4-8, got %q", got)
	}
}

func TestOfficialProviderActiveModel_EmptyModel(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		Providers: map[string]ProviderEntry{
			"anthropic": {},
		},
	}
	m := newProviderTUI(cfg, "")
	got := m.officialProviderActiveModel(llm.Provider{Name: "anthropic"})
	if got != "" {
		t.Errorf("expected empty model, got %q", got)
	}
}

func TestProviderTUIView_OfficialTab_ShowsActiveModelSuffix(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		Model:    "claude-opus-4-8",
		Providers: map[string]ProviderEntry{
			"anthropic": {Model: "claude-opus-4-8"},
		},
	}
	m := newProviderTUI(cfg, "")
	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "(claude-opus-4-8)") {
		t.Errorf("view missing active model suffix; got:\n%s", got)
	}
}

func TestModelProviderName_OfficialTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	name := m.modelProviderName()
	if name == "" {
		t.Error("expected non-empty provider name")
	}
	providers := llm.ListProviders()
	if len(providers) == 0 {
		t.Skip("no providers registered")
	}
}

func TestModelProviderName_CustomTab(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "http://localhost", Model: "m"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	name := m.modelProviderName()
	if !strings.Contains(name, "(custom)") {
		t.Errorf("expected '(custom)' in name, got %q", name)
	}
}

func TestModelProviderName_CustomTab_NoSelection(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.customIdx = 999
	name := m.modelProviderName()
	if name != "" {
		t.Errorf("expected empty fallback for out-of-bounds custom, got %q", name)
	}
}

func TestModelCount(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	count := m.modelCount()
	models := m.models()
	if count != len(models)+1 {
		t.Errorf("modelCount() = %d, want %d", count, len(models)+1)
	}
}

func TestInit_ReturnsNil(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestListCursorPrefix_Active(t *testing.T) {
	got := listCursorPrefix(true)
	if !strings.Contains(got, tuiCursor) {
		t.Errorf("expected cursor in prefix, got %q", got)
	}
}

func TestListCursorPrefix_Inactive(t *testing.T) {
	got := listCursorPrefix(false)
	if strings.Contains(got, tuiCursor) {
		t.Errorf("expected no cursor in prefix, got %q", got)
	}
}

func TestRenderListName_Active(t *testing.T) {
	got := renderListName("my-provider", true)
	if !strings.Contains(got, "my-provider") {
		t.Errorf("expected name in output, got %q", got)
	}
}

func TestRenderListName_Inactive(t *testing.T) {
	got := renderListName("my-provider", false)
	if !strings.Contains(got, "my-provider") {
		t.Errorf("expected name in output, got %q", got)
	}
}

func TestCloneProviderEntry_WithExtraBody(t *testing.T) {
	orig := ProviderEntry{
		APIKey:     "key",
		URL:        "http://localhost",
		Protocol:   "openai",
		Model:      "gpt-4",
		Models:     []string{"gpt-4", "gpt-3.5"},
		AuthHeader: "Authorization",
		ExtraBody:  map[string]any{"temperature": 0.7, "stream": true},
	}
	clone := cloneProviderEntry(orig)

	if clone.APIKey != orig.APIKey || clone.URL != orig.URL || clone.Protocol != orig.Protocol {
		t.Error("basic fields not copied")
	}
	if len(clone.Models) != 2 || clone.Models[0] != "gpt-4" {
		t.Errorf("Models not cloned: %v", clone.Models)
	}
	if clone.ExtraBody == nil {
		t.Fatal("ExtraBody should not be nil")
	}
	if clone.ExtraBody["temperature"] != 0.7 {
		t.Errorf("ExtraBody[temperature] = %v", clone.ExtraBody["temperature"])
	}

	clone.ExtraBody["new_key"] = "value"
	if _, ok := orig.ExtraBody["new_key"]; ok {
		t.Error("modifying clone should not affect original ExtraBody")
	}

	clone.Models = append(clone.Models, "gpt-5")
	if len(orig.Models) != 2 {
		t.Error("modifying clone should not affect original Models")
	}
}

func TestCloneProviderEntry_NilExtraBody(t *testing.T) {
	orig := ProviderEntry{
		APIKey: "key",
		URL:    "http://localhost",
	}
	clone := cloneProviderEntry(orig)
	if clone.ExtraBody != nil {
		t.Error("ExtraBody should remain nil")
	}
}

func TestCustomListCount(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"a": {URL: "http://a"},
			"b": {URL: "http://b"},
		},
	}
	m := newProviderTUI(cfg, "")
	got := m.customListCount()
	if got != len(m.customProviders)+1 {
		t.Errorf("customListCount() = %d, want %d", got, len(m.customProviders)+1)
	}
}

func TestIsCustomModelItem(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	models := m.models()
	if m.isCustomModelItem(len(models)) != true {
		t.Error("expected true for custom model item index")
	}
	if len(models) > 0 && m.isCustomModelItem(0) {
		t.Error("expected false for non-custom model index")
	}
}

func upKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyUp}
}

func TestHandleUp_OfficialTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	if len(m.providers) < 2 {
		t.Skip("need at least 2 providers")
	}
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.officialIdx != 1 {
		t.Fatalf("after down, officialIdx = %d, want 1", m2.officialIdx)
	}
	result, _ = m2.Update(upKey())
	m3 := result.(providerTUIModel)
	if m3.officialIdx != 0 {
		t.Errorf("after up, officialIdx = %d, want 0", m3.officialIdx)
	}
}

func TestHandleUp_Wraps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	if len(m.providers) == 0 {
		t.Skip("no providers")
	}
	result, _ := m.Update(upKey())
	m2 := result.(providerTUIModel)
	if m2.officialIdx != len(m2.providers)-1 {
		t.Errorf("up from 0 should wrap to %d, got %d", len(m2.providers)-1, m2.officialIdx)
	}
}

func TestHandleUp_CustomTab(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"a": {URL: "http://a"},
			"b": {URL: "http://b"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 1
	result, _ := m.Update(upKey())
	m2 := result.(providerTUIModel)
	if m2.customIdx != 0 {
		t.Errorf("customIdx = %d, want 0", m2.customIdx)
	}
}

func TestHandleUp_ModelStep(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.modelIdx = 1
	result, _ := m.Update(upKey())
	m2 := result.(providerTUIModel)
	if m2.modelIdx != 0 {
		t.Errorf("modelIdx = %d, want 0", m2.modelIdx)
	}
}

func TestHandleUp_ModelStepWraps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.modelIdx = 0
	result, _ := m.Update(upKey())
	m2 := result.(providerTUIModel)
	expected := m2.modelCount() - 1
	if m2.modelIdx != expected {
		t.Errorf("modelIdx = %d, want %d", m2.modelIdx, expected)
	}
}

func TestBlurCPStep_AllSteps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	for _, step := range []customProviderStep{cpStepName, cpStepBaseURL, cpStepAPIKey, cpStepAuthHeader, cpStepProtocol} {
		m.cpStep = step
		m.blurCPStep()
	}
}

func TestFocusCPStep_AllSteps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	for _, step := range []customProviderStep{cpStepName, cpStepBaseURL, cpStepAPIKey, cpStepAuthHeader, cpStepProtocol} {
		m.cpStep = step
		m.focusCPStep()
	}
}

func TestCollectCustomProviders_Nil(t *testing.T) {
	got := collectCustomProviders(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCollectCustomProviders_NilMap(t *testing.T) {
	got := collectCustomProviders(&Config{})
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCollectCustomProviders_Sorted(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"zebra": {URL: "http://z"},
			"alpha": {URL: "http://a"},
			"mid":   {URL: "http://m"},
		},
	}
	got := collectCustomProviders(cfg)
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0].name != "alpha" || got[1].name != "mid" || got[2].name != "zebra" {
		t.Errorf("not sorted: %v, %v, %v", got[0].name, got[1].name, got[2].name)
	}
}

func TestCustomProviderNameTaken(t *testing.T) {
	m := providerTUIModel{existingCfg: nil}
	if m.customProviderNameTaken("test") {
		t.Error("nil cfg should return false")
	}

	m2 := newProviderTUI(&Config{}, "")
	if m2.customProviderNameTaken("test") {
		t.Error("nil CustomProviders should return false")
	}

	m3 := newProviderTUI(&Config{
		CustomProviders: map[string]ProviderEntry{"test": {URL: "http://test"}},
	}, "")
	if !m3.customProviderNameTaken("test") {
		t.Error("existing name should return true")
	}
	if m3.customProviderNameTaken("other") {
		t.Error("non-existing name should return false")
	}
}

func TestCurrentProvider_OutOfBounds(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.officialIdx = 9999
	p := m.currentProvider()
	if p.Name != "" {
		t.Errorf("expected empty provider, got %q", p.Name)
	}
}

func TestCurrentProvider_WrongTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	p := m.currentProvider()
	if p.Name != "" {
		t.Errorf("expected empty provider for non-official tab, got %q", p.Name)
	}
}

func TestSelectedCustomProvider_NotCustomTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	_, ok := m.selectedCustomProvider()
	if ok {
		t.Error("expected false for non-custom tab")
	}
}

func TestSelectedCustomProvider_OutOfBounds(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.customIdx = 9999
	_, ok := m.selectedCustomProvider()
	if ok {
		t.Error("expected false for out-of-bounds index")
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := result.(providerTUIModel)
	if m2.width != 120 || m2.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m2.width, m2.height)
	}
}

func TestBlurManualStep_AllSteps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	for _, step := range []manualStep{manualStepURL, manualStepProtocol, manualStepModel, manualStepAuthToken, manualStepAuthHeader} {
		m.manualStep = step
		m.blurManualStep()
	}
}

func TestFocusManualStep_AllSteps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	for _, step := range []manualStep{manualStepURL, manualStepProtocol, manualStepModel, manualStepAuthToken, manualStepAuthHeader} {
		m.manualStep = step
		m.focusManualStep()
	}
}

func TestHandleDown_OfficialTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	if len(m.providers) < 2 {
		t.Skip("need at least 2 providers")
	}
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.officialIdx != 1 {
		t.Errorf("officialIdx = %d, want 1", m2.officialIdx)
	}
}

func TestHandleDown_OfficialTab_Wraps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	if len(m.providers) == 0 {
		t.Skip("no providers")
	}
	m.officialIdx = len(m.providers) - 1
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.officialIdx != 0 {
		t.Errorf("down from last should wrap to 0, got %d", m2.officialIdx)
	}
}

func TestHandleDown_CustomTab(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"a": {URL: "http://a"},
			"b": {URL: "http://b"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.customIdx != 1 {
		t.Errorf("customIdx = %d, want 1", m2.customIdx)
	}
}

func TestHandleDown_CustomTab_Wraps(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"a": {URL: "http://a"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = m.customListCount() - 1
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.customIdx != 0 {
		t.Errorf("down from last custom should wrap to 0, got %d", m2.customIdx)
	}
}

func TestHandleDown_ModelStep(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.modelIdx = 0
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.modelIdx != 1 {
		t.Errorf("modelIdx = %d, want 1", m2.modelIdx)
	}
}

func TestHandleDown_ModelStep_Wraps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.modelIdx = m.modelCount() - 1
	result, _ := m.Update(downKey())
	m2 := result.(providerTUIModel)
	if m2.modelIdx != 0 {
		t.Errorf("modelIdx = %d, want 0", m2.modelIdx)
	}
}

func TestCloneCustomProvidersMap(t *testing.T) {
	src := map[string]ProviderEntry{
		"a": {URL: "http://a", ExtraBody: map[string]any{"k": "v"}},
		"b": {URL: "http://b"},
	}
	clone := cloneCustomProvidersMap(src)
	if len(clone) != 2 {
		t.Fatalf("expected 2, got %d", len(clone))
	}
	clone["a"] = ProviderEntry{URL: "http://changed"}
	if src["a"].URL != "http://a" {
		t.Error("modifying clone should not affect original")
	}
}

func TestCloneCustomProvidersMap_Nil(t *testing.T) {
	got := cloneCustomProvidersMap(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCloneCustomProviderList(t *testing.T) {
	src := []customProviderListItem{
		{name: "a", entry: ProviderEntry{URL: "http://a"}},
		{name: "b", entry: ProviderEntry{URL: "http://b"}},
	}
	clone := cloneCustomProviderList(src)
	if len(clone) != 2 {
		t.Fatalf("expected 2, got %d", len(clone))
	}
	clone[0].name = "changed"
	if src[0].name != "a" {
		t.Error("modifying clone should not affect original")
	}
}

func TestCustomProviderEntry_FromConfig(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"test": {URL: "http://real", Model: "m1"},
		},
	}
	m := newProviderTUI(cfg, "")
	fallback := ProviderEntry{URL: "http://fallback"}
	got := m.customProviderEntry("test", fallback)
	if got.URL != "http://real" {
		t.Errorf("expected config entry, got URL %q", got.URL)
	}
}

func TestCustomProviderEntry_Fallback(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	fallback := ProviderEntry{URL: "http://fallback"}
	got := m.customProviderEntry("nonexist", fallback)
	if got.URL != "http://fallback" {
		t.Errorf("expected fallback, got URL %q", got.URL)
	}
}

func TestNewModelTUI(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	if m.isCustomProvider {
		t.Error("preset provider test helper should set isCustomProvider=false")
	}
	if len(m.registryModels) == 0 {
		t.Error("registryModels should be populated for preset provider")
	}
	if m.modelIdx != 0 {
		t.Errorf("modelIdx = %d, want 0 for empty currentModel", m.modelIdx)
	}
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestNewModelTUI_WithCurrentModel(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 || len(p[0].Models) == 0 {
		t.Skip("need provider with models")
	}
	current := p[0].Models[0]
	m := newModelTUI(p[0], current)
	if m.modelIdx != 0 {
		t.Errorf("modelIdx = %d, want 0 for first model", m.modelIdx)
	}
	if m.activeModel != current {
		t.Errorf("activeModel = %q, want %q", m.activeModel, current)
	}
}

func TestNewModelTUI_CustomModel(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "custom-model-xyz")
	if m.modelIdx != len(p[0].Models) {
		t.Errorf("modelIdx = %d, want %d for custom model", m.modelIdx, len(p[0].Models))
	}
}

func TestModelTUI_IsCustomItem(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	if !m.isCustomItem(len(p[0].Models)) {
		t.Error("expected true for custom item index")
	}
	if m.isCustomItem(0) {
		t.Error("expected false for index 0")
	}
}

func TestModelTUI_ItemCount(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	if m.itemCount() != len(p[0].Models)+1 {
		t.Errorf("itemCount() = %d, want %d", m.itemCount(), len(p[0].Models)+1)
	}
}

func TestModelTUI_SelectedModel(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 || len(p[0].Models) == 0 {
		t.Skip("need provider with models")
	}
	m := newModelTUI(p[0], "")
	got := m.selectedModel()
	if got != p[0].Models[0] {
		t.Errorf("selectedModel() = %q, want %q", got, p[0].Models[0])
	}
}

func TestModelTUI_SelectedModel_OutOfBounds(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	m.modelIdx = 9999
	got := m.selectedModel()
	if got != "" {
		t.Errorf("expected empty for out-of-bounds, got %q", got)
	}
}

func TestModelTUI_Update_UpDown(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 || len(p[0].Models) < 2 {
		t.Skip("need provider with at least 2 models")
	}
	m := newModelTUI(p[0], "")
	result, _ := m.Update(downKey())
	m2 := result.(modelTUIModel)
	if m2.modelIdx != 1 {
		t.Errorf("after down, modelIdx = %d, want 1", m2.modelIdx)
	}
	result, _ = m2.Update(upKey())
	m3 := result.(modelTUIModel)
	if m3.modelIdx != 0 {
		t.Errorf("after up, modelIdx = %d, want 0", m3.modelIdx)
	}
}

func TestModelTUI_Update_WindowSize(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m2 := result.(modelTUIModel)
	if m2.width != 100 || m2.height != 50 {
		t.Errorf("size = %dx%d, want 100x50", m2.width, m2.height)
	}
}

func TestModelTUI_Update_EscCancels(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	result, _ := m.Update(escKey())
	m2 := result.(modelTUIModel)
	if !m2.cancelled {
		t.Error("expected cancelled after esc")
	}
}

// --- View rendering tests (smoke) ---

func TestProviderTUIView_StepProvider_OfficialTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	v := m.View()
	if v.Content == "" {
		t.Error("expected non-empty view")
	}
}

func TestProviderTUIView_StepProvider_CustomTab(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {URL: "http://localhost", Model: "m"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	v := m.View()
	if !strings.Contains(v.Content, "my-llm") {
		t.Errorf("expected custom provider name in view, got %q", v.Content)
	}
}

func TestProviderTUIView_StepProvider_CustomTab_CreatingCustom(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.creatingCustom = true
	v := m.View()
	if !strings.Contains(v.Content, "Add Custom Provider") {
		t.Errorf("expected 'Add Custom Provider' in view")
	}
}

func TestProviderTUIView_StepProvider_CustomTab_EditingCustom(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"ed": {URL: "http://ed"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.editingCustom = true
	m.editTargetName = "ed"
	v := m.View()
	if !strings.Contains(v.Content, "Edit Custom Provider") {
		t.Errorf("expected 'Edit Custom Provider' in view")
	}
}

func TestProviderTUIView_StepProvider_ManualTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabManual
	v := m.View()
	if !strings.Contains(v.Content, "Manual") {
		t.Errorf("expected 'Manual' in view")
	}
}

func TestProviderTUIView_StepProvider_ManualTab_InForm(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabManual
	m.inManualForm = true
	v := m.View()
	if !strings.Contains(v.Content, "Manual Configuration") {
		t.Errorf("expected 'Manual Configuration' in view")
	}
}

func TestProviderTUIView_StepProvider_ConfirmingDelete(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"del": {URL: "http://del"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.confirmingDelete = true
	m.deleteTargetName = "del"
	v := m.View()
	if !strings.Contains(v.Content, "Confirm") {
		t.Errorf("expected confirm help text in view")
	}
}

func TestProviderTUIView_StepModel(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	v := m.View()
	if !strings.Contains(v.Content, "Select a model") {
		t.Errorf("expected 'Select a model' in view, got %q", v.Content)
	}
}

func TestProviderTUIView_StepModel_CustomModel(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.customModel = true
	v := m.View()
	if v.Content == "" {
		t.Error("expected non-empty view")
	}
}

func TestProviderTUIView_StepModel_FormError(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.customModel = true
	m.formError = "model name required"
	v := m.View()
	if !strings.Contains(v.Content, "model name required") {
		t.Errorf("expected form error in view")
	}
}

func TestProviderTUIView_StepModel_ConfirmingDeleteModel(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepModel
	m.confirmingDeleteModel = true
	m.deleteModelName = "gpt-4"
	v := m.View()
	if !strings.Contains(v.Content, "gpt-4") {
		t.Errorf("expected delete model name in view")
	}
}

func TestProviderTUIView_StepAPIKey(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.step = stepAPIKey
	v := m.View()
	if !strings.Contains(v.Content, "API Key") {
		t.Errorf("expected 'API Key' in view, got %q", v.Content)
	}
}

func TestProviderTUIView_StepAPIKey_CustomProvider(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"cp": {URL: "http://cp"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.step = stepAPIKey
	m.activeTab = tabCustom
	m.customIdx = 0
	v := m.View()
	if !strings.Contains(v.Content, "cp") {
		t.Errorf("expected custom provider name in API Key view")
	}
}

func TestRenderTabBar_AllTabs(t *testing.T) {
	for _, tab := range []providerTab{tabOfficial, tabCustom, tabManual} {
		got := renderTabBar(tab)
		if got == "" {
			t.Errorf("renderTabBar(%d) returned empty", tab)
		}
	}
}

func TestModelTUI_View(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	v := m.View()
	if !strings.Contains(v.Content, "Select a model") {
		t.Errorf("expected 'Select a model' in view, got %q", v.Content)
	}
}

func TestModelTUI_View_CustomModel(t *testing.T) {
	p := llm.ListProviders()
	if len(p) == 0 {
		t.Skip("no providers")
	}
	m := newModelTUI(p[0], "")
	m.customModel = true
	v := m.View()
	if v.Content == "" {
		t.Error("expected non-empty view")
	}
}

func officialConfigModelTUI(t *testing.T, configPath string, extraModels []string) modelTUIModel {
	t.Helper()
	preset, ok := llm.LookupProvider("dashscope")
	if !ok {
		t.Skip("dashscope provider not in registry")
	}
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
	provider := preset
	provider.Models = mergeModelLists(preset.Models, cfg.Providers["dashscope"].Models)
	return newModelTUIConfig(modelTUIConfig{
		Provider:       provider,
		RegistryModels: preset.Models,
		ExistingCfg:    cfg,
		ConfigPath:     configPath,
		ProviderName:   "dashscope",
		IsCustom:       false,
	})
}

func customConfigModelTUI(t *testing.T, configPath string, models []string) modelTUIModel {
	t.Helper()
	cfg := &Config{
		Provider: "my-llm",
		Model:    "m1",
		CustomProviders: map[string]ProviderEntry{
			"my-llm": {
				URL:      "https://custom.api/v1",
				Protocol: "openai",
				Model:    "m1",
				Models:   append([]string(nil), models...),
			},
		},
	}
	provider := llm.Provider{
		Name:        "my-llm",
		DisplayName: "my-llm (custom)",
		Models:      mergeModelLists(models),
	}
	return newModelTUIConfig(modelTUIConfig{
		Provider:     provider,
		CurrentModel: "m1",
		ExistingCfg:  cfg,
		ConfigPath:   configPath,
		ProviderName: "my-llm",
		IsCustom:     true,
	})
}

func asModelTUIModel(t *testing.T, m tea.Model) modelTUIModel {
	t.Helper()
	switch v := m.(type) {
	case modelTUIModel:
		return v
	case *modelTUIModel:
		return *v
	default:
		t.Fatalf("expected modelTUIModel, got %T", m)
		return modelTUIModel{}
	}
}

func modelTUIEnterCustomModelName(t *testing.T, m modelTUIModel, name string) modelTUIModel {
	t.Helper()
	m.modelIdx = len(m.displayModels())
	result, _ := m.Update(enterKey())
	m2 := result.(modelTUIModel)
	if !m2.customModel {
		t.Fatal("expected customModel after enter on custom item")
	}
	m2.modelInput.SetValue(name)
	result, _ = m2.Update(enterKey())
	return result.(modelTUIModel)
}

func modelTUIIdxForName(t *testing.T, m modelTUIModel, name string) int {
	t.Helper()
	for i, model := range m.displayModels() {
		if model == name {
			return i
		}
	}
	t.Fatalf("model %q not found in %v", name, m.displayModels())
	return -1
}

func TestModelTUI_Official_AddCustomModelStaysOnList(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := officialConfigModelTUI(t, configPath, nil)
	m3 := modelTUIEnterCustomModelName(t, m, "new-model")

	if m3.confirmed {
		t.Error("adding a model should not confirm and quit")
	}
	if m3.customModel {
		t.Error("customModel should be cleared after add")
	}
	got := m3.existingCfg.Providers["dashscope"].Models
	if !llm.ModelListContains(got, "new-model") {
		t.Errorf("Models = %v, want new-model appended", got)
	}
	if m3.existingCfg.Model != "qwen3.7-max" {
		t.Errorf("cfg.Model = %q, want active model unchanged", m3.existingCfg.Model)
	}
	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load disk config: %v", err)
	}
	if !llm.ModelListContains(diskCfg.Providers["dashscope"].Models, "new-model") {
		t.Errorf("disk Models = %v, want new-model persisted", diskCfg.Providers["dashscope"].Models)
	}
}

func TestModelTUI_Official_ListEnterConfirmsSelection(t *testing.T) {
	m := officialConfigModelTUI(t, "", nil)
	m2 := modelTUIEnterCustomModelName(t, m, "picked-model")
	m2.modelIdx = modelTUIIdxForName(t, m2, "picked-model")
	result, _ := m2.Update(enterKey())
	m3 := result.(modelTUIModel)
	if !m3.confirmed {
		t.Error("enter on list item should confirm selection")
	}
	if m3.selectedModel() != "picked-model" {
		t.Errorf("selectedModel() = %q, want picked-model", m3.selectedModel())
	}
}

func TestModelTUI_CustomProvider_AddCustomModelStaysOnList(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := customConfigModelTUI(t, configPath, []string{"m1"})
	m3 := modelTUIEnterCustomModelName(t, m, "new-custom-model")

	if m3.confirmed {
		t.Error("adding a model should not confirm and quit")
	}
	if !llm.ModelListContains(m3.existingCfg.CustomProviders["my-llm"].Models, "new-custom-model") {
		t.Errorf("Models = %v, want new-custom-model appended", m3.existingCfg.CustomProviders["my-llm"].Models)
	}
	if m3.existingCfg.CustomProviders["my-llm"].Model != "m1" {
		t.Errorf("entry.Model = %q, want active model unchanged", m3.existingCfg.CustomProviders["my-llm"].Model)
	}
	if !m3.savedInSession {
		t.Error("savedInSession should be true after add")
	}
}

func TestModelTUI_EscCancelWithoutChangesNoSavedInSession(t *testing.T) {
	m := officialConfigModelTUI(t, "", nil)
	result, _ := m.Update(escKey())
	m2 := result.(modelTUIModel)
	if !m2.cancelled {
		t.Fatal("esc should cancel")
	}
	if m2.savedInSession {
		t.Error("savedInSession should be false when no add/delete occurred")
	}
}

func TestModelTUI_DeleteSetsSavedInSession(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := customConfigModelTUI(t, configPath, []string{"m1", "aaa"})
	m.modelIdx = modelTUIIdxForName(t, m, "aaa")
	m.deleteModelName = "aaa"
	m.confirmingDeleteModel = true

	result, _ := m.confirmDeleteCustomProviderModel()
	m2 := asModelTUIModel(t, result)
	if !m2.savedInSession {
		t.Error("savedInSession should be true after delete")
	}
}

func TestModelTUI_AddCustomModelRejectsDuplicate(t *testing.T) {
	m := officialConfigModelTUI(t, "", []string{"dup-model"})
	m.modelIdx = len(m.displayModels())
	result, _ := m.Update(enterKey())
	m2 := result.(modelTUIModel)
	m2.modelInput.SetValue("dup-model")
	result, _ = m2.Update(enterKey())
	m3 := result.(modelTUIModel)
	if m3.formError != "Already in list: dup-model" {
		t.Errorf("formError = %q, want duplicate message", m3.formError)
	}
	if !m3.customModel {
		t.Error("customModel should stay open after duplicate reject")
	}
}

func TestModelTUI_Official_DeleteUserAddedModel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := officialConfigModelTUI(t, configPath, []string{"my-custom-model"})
	m.modelIdx = modelTUIIdxForName(t, m, "my-custom-model")

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	if !m2.confirmingDeleteModel {
		t.Fatal("pressing d on user-added model should set confirmingDeleteModel = true")
	}

	result, _ = m2.Update(yKey())
	m3 := result.(modelTUIModel)
	got := m3.existingCfg.Providers["dashscope"].Models
	if len(got) != 1 || got[0] != "qwen3.7-max" {
		t.Errorf("Models = %v, want [qwen3.7-max]", got)
	}

	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load disk config: %v", err)
	}
	if len(diskCfg.Providers["dashscope"].Models) != 1 {
		t.Errorf("disk Models = %v, want [qwen3.7-max]", diskCfg.Providers["dashscope"].Models)
	}
}

func TestModelTUI_Official_DeleteBuiltInModelIgnored(t *testing.T) {
	m := officialConfigModelTUI(t, "", []string{"my-custom-model"})
	m.modelIdx = modelTUIIdxForName(t, m, "qwen3.7-max")

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on built-in model should not trigger delete confirmation")
	}
}

func TestIsUserAddedOfficialModelName_RegistryDuplicateNotUserAdded(t *testing.T) {
	preset, ok := llm.LookupProvider("dashscope")
	if !ok {
		t.Skip("dashscope provider not in registry")
	}
	cfg := &Config{
		Providers: map[string]ProviderEntry{
			"dashscope": {Models: []string{"qwen3.7-max", "my-custom-model"}},
		},
	}
	if isUserAddedOfficialModelName("qwen3.7-max", "dashscope", preset.Models, cfg) {
		t.Error("registry model should not be treated as user-added even when listed in config")
	}
	if !isUserAddedOfficialModelName("my-custom-model", "dashscope", preset.Models, cfg) {
		t.Error("config-only model should be user-added")
	}
}

func TestModelTUI_Official_RegistryModelNotDeletable(t *testing.T) {
	m := officialConfigModelTUI(t, "", []string{"my-custom-model"})
	m.modelIdx = modelTUIIdxForName(t, m, "qwen3.7-max")

	if m.isUserAddedModel("qwen3.7-max") {
		t.Error("qwen3.7-max should not be user-added when it is in the registry")
	}

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on registry model should not trigger delete confirmation")
	}
}

func TestModelTUI_Official_DeleteOnCustomModelInputIgnored(t *testing.T) {
	m := officialConfigModelTUI(t, "", []string{"my-custom-model"})
	m.modelIdx = len(m.displayModels())

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on Enter custom model name... should not trigger delete confirmation")
	}
}

func TestModelTUI_CustomProvider_DeleteOnCustomModelInputIgnored(t *testing.T) {
	m := customConfigModelTUI(t, "", []string{"m1", "aaa"})
	m.modelIdx = len(m.displayModels())

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	if m2.confirmingDeleteModel {
		t.Error("pressing d on Enter custom model name... should not trigger delete confirmation")
	}
}

func TestModelTUI_Official_UserAddedModelShowsDeleteHint(t *testing.T) {
	m := officialConfigModelTUI(t, "", []string{"my-custom-model"})

	m.modelIdx = modelTUIIdxForName(t, m, "qwen3.7-max")
	got := stripANSI(m.View().Content)
	if strings.Contains(got, "d Delete") {
		t.Errorf("built-in model should not show d Delete hint; got:\n%s", got)
	}

	m.modelIdx = modelTUIIdxForName(t, m, "my-custom-model")
	got = stripANSI(m.View().Content)
	if !strings.Contains(got, "d Delete") {
		t.Errorf("user-added model should show d Delete hint; got:\n%s", got)
	}
}

func TestModelTUI_CustomProvider_ShowsDeleteHint(t *testing.T) {
	m := customConfigModelTUI(t, "", []string{"m1", "aaa"})

	m.modelIdx = len(m.displayModels())
	got := stripANSI(m.View().Content)
	if strings.Contains(got, "d Delete") {
		t.Errorf("custom input row should not show d Delete hint; got:\n%s", got)
	}

	m.modelIdx = modelTUIIdxForName(t, m, "aaa")
	got = stripANSI(m.View().Content)
	if !strings.Contains(got, "d Delete") {
		t.Errorf("custom model row should show d Delete hint; got:\n%s", got)
	}
}

func TestModelTUI_CustomProvider_DeleteClearsCustomInput(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := customConfigModelTUI(t, configPath, []string{"m1", "aaa"})
	m.modelIdx = modelTUIIdxForName(t, m, "aaa")
	m.customModel = true
	m.modelInput.SetValue("aaa")
	m.deleteModelName = "aaa"
	m.confirmingDeleteModel = true

	result, _ := m.confirmDeleteCustomProviderModel()
	m2 := asModelTUIModel(t, result)

	if m2.customModel {
		t.Error("customModel should be false after delete")
	}
	if m2.modelInput.Value() != "" {
		t.Errorf("modelInput = %q, want empty after delete", m2.modelInput.Value())
	}
}

func TestModelTUI_CustomProvider_DeleteModelViaDKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := customConfigModelTUI(t, configPath, []string{"m1", "aaa"})
	m.modelIdx = modelTUIIdxForName(t, m, "aaa")

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	if !m2.confirmingDeleteModel || m2.deleteModelName != "aaa" {
		t.Fatalf("after d: confirming=%v deleteModelName=%q", m2.confirmingDeleteModel, m2.deleteModelName)
	}

	result, _ = m2.Update(yKey())
	m3 := result.(modelTUIModel)
	got := m3.existingCfg.CustomProviders["my-llm"].Models
	if len(got) != 1 || got[0] != "m1" {
		t.Errorf("Models = %v, want [m1]", got)
	}
}

func TestModelTUI_CustomProvider_DeleteCancel(t *testing.T) {
	m := customConfigModelTUI(t, "", []string{"m1", "aaa"})
	m.modelIdx = modelTUIIdxForName(t, m, "aaa")

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	result, _ = m2.Update(nKey())
	m3 := result.(modelTUIModel)
	if m3.confirmingDeleteModel {
		t.Error("confirmingDeleteModel should be false after n")
	}
	got := m3.existingCfg.CustomProviders["my-llm"].Models
	if len(got) != 2 {
		t.Errorf("Models = %v, want unchanged", got)
	}
}

func TestModelTUI_Official_DeleteCancel(t *testing.T) {
	cancelKeys := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"n", nKey()},
		{"esc", escKey()},
	}
	for _, tc := range cancelKeys {
		t.Run(tc.name, func(t *testing.T) {
			m := officialConfigModelTUI(t, "", []string{"my-custom-model"})
			m.modelIdx = modelTUIIdxForName(t, m, "my-custom-model")

			result, _ := m.Update(dKey())
			m2 := result.(modelTUIModel)
			if !m2.confirmingDeleteModel {
				t.Fatal("expected confirmingDeleteModel after d")
			}

			result, _ = m2.Update(tc.key)
			m3 := result.(modelTUIModel)
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

func TestModelTUI_Official_DeleteActiveUserModelClearsCfg(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	m := officialConfigModelTUI(t, configPath, []string{"my-custom-model"})
	m.existingCfg.Model = "my-custom-model"
	m.existingCfg.Providers["dashscope"] = ProviderEntry{
		Model:  "my-custom-model",
		Models: []string{"qwen3.7-max", "my-custom-model"},
	}
	m.modelIdx = modelTUIIdxForName(t, m, "my-custom-model")

	result, _ := m.Update(dKey())
	m2 := result.(modelTUIModel)
	result, _ = m2.Update(yKey())
	m3 := result.(modelTUIModel)

	if m3.existingCfg.Providers["dashscope"].Model != "" {
		t.Errorf("entry.Model = %q, want empty", m3.existingCfg.Providers["dashscope"].Model)
	}
	if m3.existingCfg.Model != "" {
		t.Errorf("cfg.Model = %q, want empty", m3.existingCfg.Model)
	}
}

// --- result() tests ---

func TestResult_OfficialTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	r := m.result()
	if r.provider == "" && len(m.providers) > 0 {
		t.Error("expected non-empty provider")
	}
}

func TestResult_OfficialTab_WithMaskedKey(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.apiKeyMasked = true
	m.apiKeyOriginal = "sk-secret"
	r := m.result()
	if r.apiKey != "sk-secret" {
		t.Errorf("expected masked key, got %q", r.apiKey)
	}
}

func TestResult_CustomTab_Creating(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.creatingCustom = true
	m.cpNameInput.SetValue("new-prov")
	m.cpURLInput.SetValue("http://url")
	r := m.result()
	if !r.isCustom {
		t.Error("expected isCustom=true")
	}
	if r.provider != "new-prov" {
		t.Errorf("provider = %q, want new-prov", r.provider)
	}
}

func TestResult_CustomTab_Editing(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"ed": {URL: "http://ed", Model: "m1", Models: []string{"m1", "m2"}},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.editingCustom = true
	m.editTargetName = "ed"
	m.cpNameInput.SetValue("ed")
	m.cpURLInput.SetValue("http://ed")
	r := m.result()
	if !r.isEdit {
		t.Error("expected isEdit=true")
	}
	if r.model != "m1" {
		t.Errorf("model = %q, want m1", r.model)
	}
}

func TestResult_CustomTab_Selected(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"sel": {URL: "http://sel", Model: "gpt", Models: []string{"gpt"}},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	r := m.result()
	if !r.isCustom {
		t.Error("expected isCustom=true")
	}
	if r.provider != "sel" {
		t.Errorf("provider = %q, want sel", r.provider)
	}
}

func TestResult_CustomTab_OutOfBounds(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.customIdx = 999
	r := m.result()
	if r.provider != "" {
		t.Errorf("expected empty provider, got %q", r.provider)
	}
}

func TestResult_ManualTab(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabManual
	m.manualURLInput.SetValue("http://manual")
	m.manualModelInput.SetValue("model-x")
	r := m.result()
	if !r.isManual {
		t.Error("expected isManual=true")
	}
	if r.url != "http://manual" {
		t.Errorf("url = %q, want http://manual", r.url)
	}
	if r.model != "model-x" {
		t.Errorf("model = %q, want model-x", r.model)
	}
}

func TestResult_ManualTab_MaskedToken(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabManual
	m.manualTokenMasked = true
	m.manualTokenOriginal = "tok-secret"
	r := m.result()
	if r.apiKey != "tok-secret" {
		t.Errorf("expected masked token, got %q", r.apiKey)
	}
}

// --- loadExistingAPIKey tests ---

func TestLoadExistingAPIKey_CustomTab_HasKey(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"cp": {URL: "http://cp", APIKey: "sk-key"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.loadExistingAPIKey()
	if !m.apiKeyMasked {
		t.Error("expected apiKeyMasked=true")
	}
	if m.apiKeyOriginal != "sk-key" {
		t.Errorf("apiKeyOriginal = %q, want sk-key", m.apiKeyOriginal)
	}
}

func TestLoadExistingAPIKey_CustomTab_NoKey(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"cp": {URL: "http://cp"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.loadExistingAPIKey()
	if m.apiKeyMasked {
		t.Error("expected apiKeyMasked=false")
	}
}

func TestLoadExistingAPIKey_OfficialTab_NilCfg(t *testing.T) {
	m := providerTUIModel{existingCfg: nil}
	m.loadExistingAPIKey()
	if m.apiKeyMasked {
		t.Error("expected apiKeyMasked=false for nil cfg")
	}
}

func TestLoadExistingAPIKey_OfficialTab_HasKey(t *testing.T) {
	cfg := &Config{}
	m := newProviderTUI(cfg, "")
	if len(m.providers) == 0 {
		t.Skip("no providers")
	}
	provName := m.providers[0].Name
	cfg.Providers = map[string]ProviderEntry{
		provName: {APIKey: "official-key"},
	}
	m.existingCfg = cfg
	m.officialIdx = 0
	m.loadExistingAPIKey()
	if !m.apiKeyMasked {
		t.Error("expected apiKeyMasked=true")
	}
	if m.apiKeyOriginal != "official-key" {
		t.Errorf("apiKeyOriginal = %q, want official-key", m.apiKeyOriginal)
	}
}

// --- selectedModelFromState tests ---

func TestSelectedModelFromState_CustomModelInput(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.customModel = true
	m.modelInput.SetValue("my-model")
	got := m.selectedModelFromState()
	if got != "my-model" {
		t.Errorf("expected my-model, got %q", got)
	}
}

func TestSelectedModelFromState_OutOfBounds(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.modelIdx = 9999
	got := m.selectedModelFromState()
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- syncSessionModelSelection tests ---

func TestSyncSessionModelSelection_NilCfg(t *testing.T) {
	m := providerTUIModel{existingCfg: nil}
	err := m.syncSessionModelSelection()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncSessionModelSelection_EmptyModel(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.modelIdx = 9999
	err := m.syncSessionModelSelection()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncSessionModelSelection_CrossOfficialProviderNoPersist(t *testing.T) {
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
	m.modelIdx = 0
	for i, name := range m.models() {
		if name == "glm-5" {
			m.modelIdx = i
			break
		}
	}

	if err := m.syncSessionModelSelection(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.savedInSession {
		t.Error("savedInSession should be false when browsing a non-active provider")
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("config file should not be written for cross-provider navigation")
	}
	if got := cfg.Providers["baidu-qianfan"].Model; got != "" {
		t.Errorf("in-memory baidu model = %q, want empty", got)
	}
}

func TestSyncSessionModelSelection_ActiveOfficialProviderDefersPersist(t *testing.T) {
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
	for i, name := range m.models() {
		if name == "deepseek-v4-pro" {
			m.modelIdx = i
			break
		}
	}

	if err := m.syncSessionModelSelection(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.savedInSession {
		t.Error("savedInSession should be false before wizard confirm")
	}
	if got := m.sessionModelPick["deepseek"]; got != "deepseek-v4-pro" {
		t.Errorf("sessionModelPick = %q, want deepseek-v4-pro", got)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("config file should not be written before wizard confirm")
	}
	if cfg.Model != "deepseek-v4-flash" {
		t.Errorf("cfg.Model = %q, want deepseek-v4-flash", cfg.Model)
	}
}

func TestSyncSessionModelSelection_CrossCustomProviderNoPersist(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "stepfun",
		Model:    "step-3.5-flash",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {
				URL:    "https://api.stepfun.com/v1",
				Model:  "step-3.5-flash",
				Models: []string{"step-3.5-flash", "step-3.7-flash"},
			},
			"other": {
				URL:    "https://example.com/v1",
				Model:  "step-3.7-flash",
				Models: []string{"step-3.7-flash"},
			},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	for i, cp := range m.customProviders {
		if cp.name == "other" {
			m.customIdx = i
			break
		}
	}
	m.modelIdx = 0

	if err := m.syncSessionModelSelection(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.savedInSession {
		t.Error("savedInSession should be false when browsing a non-active custom provider")
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("config file should not be written for cross-provider navigation")
	}
}

func TestSyncSessionModelSelection_RecordsSessionPickForInactiveOfficialProvider(t *testing.T) {
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
	for i, name := range m.models() {
		if name == "glm-5" {
			m.modelIdx = i
			break
		}
	}

	if err := m.syncSessionModelSelection(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.savedInSession {
		t.Error("savedInSession should be false for inactive provider")
	}
	if got := m.sessionModelPick["baidu-qianfan"]; got != "glm-5" {
		t.Errorf("sessionModelPick = %q, want glm-5", got)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("config file should not be written for inactive provider")
	}
}

func TestProviderTUI_ResultUsesSessionModelPickWhenSelectionEmpty(t *testing.T) {
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
	m.sessionModelPick = map[string]string{"baidu-qianfan": "glm-5"}
	m.modelIdx = 9999 // force selectedModelFromState() empty

	r := m.result()
	if r.model != "glm-5" {
		t.Errorf("result().model = %q, want glm-5", r.model)
	}
	if got := r.resolvedModel(); got != "glm-5" {
		t.Errorf("resolvedModel() = %q, want glm-5", got)
	}
}

func TestApiKeyStepCanConfirm_OfficialEmptyWithoutEnv(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	m.step = stepAPIKey

	ok, errMsg := m.apiKeyStepCanConfirm()
	if ok {
		t.Fatal("expected confirmation to be blocked")
	}
	if errMsg != "API key is required (or set $DEEPSEEK_API_KEY)" {
		t.Errorf("errMsg = %q", errMsg)
	}
}

func TestApiKeyStepCanConfirm_OfficialEmptyWithEnv(t *testing.T) {
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
	m.step = stepAPIKey

	ok, errMsg := m.apiKeyStepCanConfirm()
	if !ok {
		t.Fatalf("expected confirmation allowed, errMsg = %q", errMsg)
	}
}

func TestApiKeyStepCanConfirm_CustomEmpty(t *testing.T) {
	cfg := &Config{
		Provider: "stepfun",
		CustomProviders: map[string]ProviderEntry{
			"stepfun": {APIKey: ""},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepAPIKey

	ok, errMsg := m.apiKeyStepCanConfirm()
	if ok {
		t.Fatal("expected confirmation to be blocked")
	}
	if errMsg != "API key is required" {
		t.Errorf("errMsg = %q", errMsg)
	}
}

func TestApiKeyStepCanConfirm_MaskedSavedKey(t *testing.T) {
	cfg := &Config{
		Provider: "deepseek",
		Providers: map[string]ProviderEntry{
			"deepseek": {APIKey: "keep-me"},
		},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabOfficial
	m.step = stepAPIKey
	m.loadExistingAPIKey()

	ok, errMsg := m.apiKeyStepCanConfirm()
	if !ok {
		t.Fatalf("expected confirmation allowed, errMsg = %q", errMsg)
	}
}

// --- viewCustomProviderForm field steps ---

func TestProviderTUIView_CustomForm_AllSteps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.creatingCustom = true
	for _, step := range []customProviderStep{cpStepName, cpStepBaseURL, cpStepAPIKey, cpStepAuthHeader, cpStepProtocol} {
		m.cpStep = step
		v := m.View()
		if v.Content == "" {
			t.Errorf("empty view for step %d", step)
		}
	}
}

func TestProviderTUIView_CustomForm_WithError(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabCustom
	m.creatingCustom = true
	m.formError = "name is required"
	v := m.View()
	if !strings.Contains(v.Content, "name is required") {
		t.Error("expected form error in view")
	}
}

// --- viewManualTab field steps ---

func TestProviderTUIView_ManualForm_AllSteps(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabManual
	m.inManualForm = true
	for _, step := range []manualStep{manualStepURL, manualStepProtocol, manualStepModel, manualStepAuthToken, manualStepAuthHeader} {
		m.manualStep = step
		v := m.View()
		if v.Content == "" {
			t.Errorf("empty view for manual step %d", step)
		}
	}
}

func TestProviderTUIView_ManualForm_WithError(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.activeTab = tabManual
	m.inManualForm = true
	m.formError = "URL required"
	v := m.View()
	if !strings.Contains(v.Content, "URL required") {
		t.Error("expected form error in view")
	}
}

func TestProviderTUIView_ManualTab_WithExistingConfig(t *testing.T) {
	cfg := &Config{
		Llm: LlmConfig{URL: "http://existing", Model: "old-model"},
	}
	m := newProviderTUI(cfg, "")
	m.activeTab = tabManual
	v := m.View()
	if !strings.Contains(v.Content, "http://existing") {
		t.Errorf("expected existing URL in manual tab")
	}
}

// --- viewModel with custom tab and deleteModel ---

func TestProviderTUIView_StepModel_CustomTabDeleteHelp(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"cp": {URL: "http://cp", Models: []string{"m1", "m2"}},
		},
	}
	m := newProviderTUI(cfg, "")
	m.step = stepModel
	m.activeTab = tabCustom
	m.customIdx = 0

	m.modelIdx = len(m.models())
	got := stripANSI(m.View().Content)
	if strings.Contains(got, "d Delete") {
		t.Errorf("custom input row should not show d Delete hint; got:\n%s", got)
	}

	m.modelIdx = 0
	got = stripANSI(m.View().Content)
	if !strings.Contains(got, "d Delete") {
		t.Errorf("custom model row should show d Delete hint; got:\n%s", got)
	}
}
