package main

import (
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
			"cp": {URL: "http://cp", Models: []string{"m1"}},
		},
	}
	m := newProviderTUI(cfg, "")
	m.step = stepModel
	m.activeTab = tabCustom
	m.customIdx = 0
	v := m.View()
	if !strings.Contains(v.Content, "Delete") {
		t.Errorf("expected delete help for custom tab model view")
	}
}
