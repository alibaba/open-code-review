// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alibaba/open-code-review/internal/llm"
)

type tuiStep int

const (
	stepProvider tuiStep = iota
	stepModel
	stepAPIKey
)

type providerTab int

const (
	tabOfficial providerTab = iota
	tabCustom
	tabManual
	tabCount // sentinel — must remain last
)

type customProviderStep int

const (
	cpStepName customProviderStep = iota
	cpStepProtocol
	cpStepBaseURL
	cpStepAPIKey
	cpStepAuthHeader
)

type manualStep int

const (
	manualStepURL manualStep = iota
	manualStepProtocol
	manualStepModel
	manualStepAuthToken
	manualStepAuthHeader
)

// cpProtocols lists the protocol options offered in the Custom provider form.
// Using the canonical names from protocol.go means whatever the user picks
// flows through resolver normalization unchanged and is written to config
// verbatim.
var cpProtocols = []string{
	llm.ProtocolAnthropic,
	llm.ProtocolOpenAIChatCompletions,
	llm.ProtocolOpenAIResponses,
	llm.ProtocolAnthropicBedrock,
}

// manualProtocols lists the protocol options offered in the Manual form, which
// writes llm.url and llm.auth_token. Bedrock is deliberately absent: that block
// holds no region or profile, and bedrock uses neither the url nor the token it
// does hold, so the resolver rejects the combination outright.
var manualProtocols = []string{
	llm.ProtocolAnthropic,
	llm.ProtocolOpenAIChatCompletions,
	llm.ProtocolOpenAIResponses,
}

type customProviderListItem struct {
	name  string
	entry ProviderEntry
}
type providerTUIResult struct {
	provider         string
	model            string
	models           []string
	apiKey           string
	isCustom         bool
	isEdit           bool
	editTargetName   string
	isManual         bool
	url              string
	protocol         string
	authHeader       string
	sessionModelPick map[string]string
}

// resolvedModel returns the model to persist, falling back to the in-session pick
// for the provider being finalized when result.model is empty.
func (r providerTUIResult) resolvedModel() string {
	if r.model != "" {
		return r.model
	}
	if r.sessionModelPick != nil {
		if pick, ok := r.sessionModelPick[r.provider]; ok && pick != "" {
			return pick
		}
	}
	return ""
}
func (m providerTUIModel) sessionModelPickFor(providerName string) string {
	if providerName == "" || m.sessionModelPick == nil {
		return ""
	}
	return m.sessionModelPick[providerName]
}
func (m providerTUIModel) sessionModelPickSnapshot() map[string]string {
	if len(m.sessionModelPick) == 0 {
		return nil
	}
	out := make(map[string]string, len(m.sessionModelPick))
	for k, v := range m.sessionModelPick {
		out[k] = v
	}
	return out
}

type providerTUIModel struct {
	step   tuiStep
	width  int
	height int

	activeTab providerTab

	// --- tab: official ---
	providers   []llm.Provider
	officialIdx int

	// --- tab: custom ---
	customProviders []customProviderListItem
	customIdx       int
	creatingCustom  bool
	editingCustom   bool
	editTargetName  string
	cpStep          customProviderStep
	cpProtocolIdx   int
	cpNameInput     textinput.Model
	cpURLInput      textinput.Model
	cpAuthInput     textinput.Model

	// --- tab: manual ---
	inManualForm          bool
	manualStep            manualStep
	manualProtocolIdx     int
	manualURLInput        textinput.Model
	manualModelInput      textinput.Model
	manualAuthHeaderInput textinput.Model
	manualTokenInput      textinput.Model
	manualTokenMasked     bool
	manualTokenOriginal   string

	// --- shared model/api-key steps (official + existing custom) ---
	modelIdx    int
	customModel bool
	modelInput  textinput.Model

	apiKeyInput    textinput.Model
	apiKeyMasked   bool
	apiKeyOriginal string

	existingCfg    *Config
	configPath     string
	confirmed      bool
	cancelled      bool
	formError      string
	savedInSession bool
	// sessionModelPick remembers model choices per provider during a wizard run
	// without persisting inactive-provider selections to disk.
	sessionModelPick map[string]string

	// --- delete confirmation ---
	confirmingDelete      bool
	deleteTargetIdx       int
	deleteTargetName      string
	deletedProviders      []string
	confirmingDeleteModel bool
	deleteModelName       string
}

// cpProtocolIndex maps a protocol string (canonical name or legacy alias) to
// its index in cpProtocols. Unknown / empty values default to the OpenAI Chat
// Completions entry (index 1) to preserve legacy behavior where any non-anthropic
// protocol was treated as OpenAI.
// cpAmbientProtocol reports whether the protocol selected in the Custom form
// authenticates from the environment rather than from a stored credential. Such
// a provider has no url, no api key and no auth header to collect, so the form
// ends at the protocol step instead of walking three fields that would be
// written as dead config.
func (m providerTUIModel) cpAmbientProtocol() bool {
	return cpProtocols[m.cpProtocolIdx] == llm.ProtocolAnthropicBedrock
}
func cpProtocolIndex(protocol string) int {
	return protocolIndexIn(cpProtocols, protocol)
}

// manualProtocolIndex is cpProtocolIndex for the Manual form's shorter list. A
// config that names bedrock in llm.protocol is unusable there and lands on the
// default rather than an out-of-range index; the resolver reports why.
func manualProtocolIndex(protocol string) int {
	return protocolIndexIn(manualProtocols, protocol)
}
func protocolIndexIn(list []string, protocol string) int {
	normalized := llm.NormalizeProtocol(protocol)
	for i, p := range list {
		if p == normalized {
			return i
		}
	}
	return 1
}
func (m providerTUIModel) customProviderNameTaken(name string) bool {
	if m.existingCfg == nil || m.existingCfg.CustomProviders == nil {
		return false
	}
	_, exists := m.existingCfg.CustomProviders[name]
	return exists
}
func (m providerTUIModel) customProviderActiveModel(cp customProviderListItem) string {
	if m.existingCfg == nil || m.existingCfg.Provider != cp.name {
		return ""
	}
	entry := m.customProviderEntry(cp.name, cp.entry)
	return activeModelForProvider(m.existingCfg, cp.name, entry)
}
func (m providerTUIModel) officialProviderActiveModel(p llm.Provider) string {
	if m.existingCfg == nil || m.existingCfg.Provider != p.Name {
		return ""
	}
	entry := ProviderEntry{}
	if m.existingCfg.Providers != nil {
		entry = m.existingCfg.Providers[p.Name]
	}
	return activeModelForProvider(m.existingCfg, p.Name, entry)
}
func collectCustomProviders(cfg *Config) []customProviderListItem {
	if cfg == nil || cfg.CustomProviders == nil {
		return nil
	}
	var out []customProviderListItem
	for name, entry := range cfg.CustomProviders {
		out = append(out, customProviderListItem{name: name, entry: entry})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}
func newProviderTUI(cfg *Config, configPath string) providerTUIModel {
	providers := llm.ListProviders()
	sort.SliceStable(providers, func(i, j int) bool {
		left := strings.ToLower(providers[i].DisplayName)
		right := strings.ToLower(providers[j].DisplayName)
		if left == right {
			return providers[i].Name < providers[j].Name
		}
		return left < right
	})

	mi := textinput.New()
	mi.Placeholder = "enter model name"
	mi.SetWidth(50)

	ai := textinput.New()
	ai.Placeholder = "paste your API key here"
	ai.SetWidth(50)
	ai.EchoMode = textinput.EchoPassword
	ai.EchoCharacter = '*'

	cpName := textinput.New()
	cpName.Placeholder = "provider name (e.g. my-llm)"
	cpName.SetWidth(40)

	cpURL := textinput.New()
	cpURL.Placeholder = "enter your API base URL"
	cpURL.SetWidth(50)

	cpAuth := textinput.New()
	cpAuth.Placeholder = "optional, leave empty for default (Authorization)"
	cpAuth.SetWidth(55)

	manualURL := textinput.New()
	manualURL.Placeholder = "enter your API base URL"
	manualURL.SetWidth(50)

	manualModel := textinput.New()
	manualModel.Placeholder = "enter model name"
	manualModel.SetWidth(40)

	manualAuthHeader := textinput.New()
	manualAuthHeader.Placeholder = "optional, leave empty for default (Authorization)"
	manualAuthHeader.SetWidth(55)

	manualToken := textinput.New()
	manualToken.Placeholder = "enter your auth token"
	manualToken.SetWidth(50)
	manualToken.EchoMode = textinput.EchoPassword
	manualToken.EchoCharacter = '*'

	m := providerTUIModel{
		providers:             providers,
		existingCfg:           cfg,
		modelInput:            mi,
		apiKeyInput:           ai,
		cpNameInput:           cpName,
		cpURLInput:            cpURL,
		cpAuthInput:           cpAuth,
		manualURLInput:        manualURL,
		manualModelInput:      manualModel,
		manualAuthHeaderInput: manualAuthHeader,
		manualTokenInput:      manualToken,
		width:                 80,
		height:                24,
		activeTab:             tabOfficial,
		customProviders:       collectCustomProviders(cfg),
		configPath:            configPath,
	}

	providerFound := false
	if cfg.Provider != "" {
		for i, p := range providers {
			if p.Name == cfg.Provider {
				m.officialIdx = i
				providerFound = true
				break
			}
		}

		if !providerFound {
			m.activeTab = tabCustom
			m.customIdx = len(m.customProviders) // default to "Add" option
			for i, cp := range m.customProviders {
				if cp.name == cfg.Provider {
					m.customIdx = i
					break
				}
			}
		}
	}

	if providerFound {
		if entry, ok := cfg.Providers[cfg.Provider]; ok && entry.Model != "" {
			selected := providers[m.officialIdx]
			found := false
			for i, model := range selected.Models {
				if model == entry.Model {
					m.modelIdx = i
					found = true
					break
				}
			}
			if !found {
				m.modelIdx = len(selected.Models)
				m.modelInput.SetValue(entry.Model)
			}
		}

		if entry, ok := cfg.Providers[cfg.Provider]; ok && entry.APIKey != "" {
			m.apiKeyOriginal = entry.APIKey
			m.apiKeyMasked = true
		}
	}

	if cfg.Provider == "" && cfg.Llm.URL != "" {
		m.activeTab = tabManual
	}
	// Intentionally do not auto-switch activeTab to tabCustom when only custom
	// providers exist — leave the cursor on Official so users navigate
	// explicitly via Tab/Right.

	if cfg.Llm.URL != "" {
		m.manualURLInput.SetValue(cfg.Llm.URL)
		m.manualModelInput.SetValue(cfg.Llm.Model)
		m.manualAuthHeaderInput.SetValue(cfg.Llm.AuthHeader)
		if cfg.Llm.AuthToken != "" {
			m.manualTokenOriginal = cfg.Llm.AuthToken
			m.manualTokenMasked = true
			m.manualTokenInput.SetValue(maskedSecretPlaceholder())
		}
		// Manual tab protocol: prefer cfg.Llm.Protocol (canonical, covers all three
		// protocols including openai-responses); fall back to use_anthropic for
		// configs written before llm.protocol existed.
		if cfg.Llm.Protocol != "" {
			m.manualProtocolIdx = manualProtocolIndex(cfg.Llm.Protocol)
		} else if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic {
			m.manualProtocolIdx = 0 // anthropic
		} else {
			m.manualProtocolIdx = 1 // openai
		}
	}

	return m
}
func (m providerTUIModel) Init() tea.Cmd {
	return nil
}
func (m providerTUIModel) currentProvider() llm.Provider {
	if m.activeTab != tabOfficial || m.officialIdx >= len(m.providers) {
		return llm.Provider{}
	}
	return m.providers[m.officialIdx]
}
func (m providerTUIModel) selectedCustomProvider() (customProviderListItem, bool) {
	if m.activeTab != tabCustom || m.customIdx >= len(m.customProviders) {
		return customProviderListItem{}, false
	}
	return m.customProviders[m.customIdx], true
}
func (m providerTUIModel) modelProviderName() string {
	if m.activeTab == tabCustom {
		if cp, ok := m.selectedCustomProvider(); ok {
			return cp.name + " (custom)"
		}
	}
	provider := m.currentProvider()
	if provider.DisplayName != "" {
		return provider.DisplayName
	}
	return provider.Name
}
func isUserAddedOfficialModelName(name, providerName string, registryModels []string, cfg *Config) bool {
	if providerName == "" || cfg == nil {
		return false
	}
	if llm.ModelListContains(registryModels, name) {
		return false
	}
	entry, ok := cfg.Providers[providerName]
	if !ok {
		return false
	}
	return llm.ModelListContains(entry.Models, name)
}
func registryModelsForProvider(name string, fallback []string) []string {
	if preset, ok := llm.LookupProvider(name); ok {
		return append([]string(nil), preset.Models...)
	}
	if len(fallback) == 0 {
		return nil
	}
	return append([]string(nil), fallback...)
}
func applyModelDeleteToEntry(entry ProviderEntry, name string) ProviderEntry {
	entry.Models = removeModels(entry.Models, []string{name})
	if entry.Model == name {
		entry.Model = ""
	}
	return entry
}
func clearCfgActiveModelIfDeleted(cfg *Config, providerName, name string) {
	if cfg != nil && cfg.Provider == providerName && cfg.Model == name {
		cfg.Model = ""
	}
}
func rollbackCfgActiveModel(cfg *Config, providerName, prevModel string) {
	if cfg != nil && cfg.Provider == providerName {
		cfg.Model = prevModel
	}
}
func (m *modelTUIModel) rollbackModelDelete(prevEntry ProviderEntry, prevCfgModel string) {
	if m.existingCfg == nil {
		return
	}
	if m.isCustomProvider {
		m.existingCfg.CustomProviders[m.providerName] = prevEntry
		m.syncModelsFromConfig()
	} else {
		m.existingCfg.Providers[m.providerName] = prevEntry
	}
	rollbackCfgActiveModel(m.existingCfg, m.providerName, prevCfgModel)
}
func (m providerTUIModel) isUserAddedOfficialModel(name string) bool {
	if m.activeTab != tabOfficial {
		return false
	}
	provider := m.currentProvider()
	return isUserAddedOfficialModelName(name, provider.Name, registryModelsForProvider(provider.Name, provider.Models), m.existingCfg)
}

// cursorOnDeletableModel reports whether the model-step cursor is on a row that
// can be deleted (not on "Enter custom model name...").
func (m providerTUIModel) cursorOnDeletableModel() bool {
	if m.step != stepModel || m.confirmingDeleteModel {
		return false
	}
	models := m.models()
	if m.modelIdx >= len(models) {
		return false
	}
	switch m.activeTab {
	case tabCustom:
		return m.customIdx < len(m.customProviders)
	case tabOfficial:
		return m.isUserAddedOfficialModel(models[m.modelIdx])
	default:
		return false
	}
}
func (m providerTUIModel) models() []string {
	switch m.activeTab {
	case tabOfficial:
		provider := m.currentProvider()
		models := registryModelsForProvider(provider.Name, provider.Models)
		if m.existingCfg != nil {
			if entry, ok := m.existingCfg.Providers[provider.Name]; ok {
				models = mergeModelLists(models, entry.Models)
			}
		}
		return models
	case tabCustom:
		if cp, ok := m.selectedCustomProvider(); ok {
			return cp.entry.Models
		}
	}
	return nil
}
func (m *providerTUIModel) prepareModelSelection(providerName, configModel string) {
	m.modelIdx = 0
	m.customModel = false
	m.modelInput.Blur()
	m.modelInput.SetValue("")

	currentModel := configModel
	if providerName != "" && m.sessionModelPick != nil {
		if pick, ok := m.sessionModelPick[providerName]; ok && pick != "" {
			currentModel = pick
		}
	}

	models := m.models()
	if currentModel == "" {
		return
	}

	for i, model := range models {
		if model == currentModel {
			m.modelIdx = i
			return
		}
	}
	m.modelIdx = len(models)
	m.modelInput.SetValue(currentModel)
}
func (m providerTUIModel) providerNameForModelStep() string {
	switch m.activeTab {
	case tabOfficial:
		return m.currentProvider().Name
	case tabCustom:
		if cp, ok := m.selectedCustomProvider(); ok {
			return cp.name
		}
	}
	return ""
}
func (m *providerTUIModel) recordSessionModelPick(model string) {
	if model == "" {
		return
	}
	name := m.providerNameForModelStep()
	if name == "" {
		return
	}
	if m.sessionModelPick == nil {
		m.sessionModelPick = make(map[string]string)
	}
	m.sessionModelPick[name] = model
}
func (m *providerTUIModel) customProviderEntry(name string, fallback ProviderEntry) ProviderEntry {
	if m.existingCfg != nil {
		if entry, ok := m.existingCfg.CustomProviders[name]; ok {
			return entry
		}
	}
	return fallback
}
func (m *providerTUIModel) syncSessionModelSelection() error {
	if m.existingCfg == nil {
		return nil
	}
	model := m.selectedModelFromState()
	if model == "" {
		return nil
	}
	// Remember the pick for in-wizard navigation only; persist provider/model on
	// final confirm (applyOfficialProviderConfig / applyCustomProviderConfig).
	m.recordSessionModelPick(model)
	return nil
}
func (m providerTUIModel) isCustomModelItem(idx int) bool {
	return idx == len(m.models())
}
func (m providerTUIModel) modelCount() int {
	return len(m.models()) + 1
}
func (m providerTUIModel) customListCount() int {
	return len(m.customProviders) + 1
}

// --- Update ---
func (m providerTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()

		if m.step == stepModel && m.customModel {
			return m.updateCustomModelInput(key, msg)
		}

		if m.step == stepAPIKey {
			return m.updateAPIKeyInput(key, msg)
		}

		if m.step == stepProvider && (m.creatingCustom || m.editingCustom) {
			return m.updateCustomProviderForm(key, msg)
		}

		if m.step == stepProvider && m.inManualForm {
			return m.updateManualForm(key, msg)
		}

		if m.step == stepProvider && m.confirmingDelete {
			return m.updateDeleteConfirm(key)
		}

		if m.step == stepModel && m.confirmingDeleteModel {
			return m.updateDeleteModelConfirm(key)
		}

		switch key {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit

		case "esc":
			if m.step == stepProvider {
				m.cancelled = true
				return m, tea.Quit
			}
			m.step--
			m.formError = ""
			return m, nil

		case "enter":
			return m.handleEnter()

		case "up", "k":
			return m.handleUp()

		case "down", "j":
			return m.handleDown()

		case "left", "h":
			if m.step == stepProvider {
				if m.activeTab > 0 {
					m.activeTab--
					m.formError = ""
				}
			}
			return m, nil

		case "right", "l":
			if m.step == stepProvider {
				if m.activeTab < tabCount-1 {
					m.activeTab++
					m.formError = ""
				}
			}
			return m, nil

		case "tab":
			if m.step == stepProvider {
				m.activeTab = (m.activeTab + 1) % tabCount
				m.formError = ""
			}
			return m, nil

		case "d":
			if m.step == stepProvider && m.activeTab == tabCustom && !m.creatingCustom && m.customIdx < len(m.customProviders) {
				m.confirmingDelete = true
				m.deleteTargetIdx = m.customIdx
				m.deleteTargetName = m.customProviders[m.customIdx].name
				return m, nil
			}
			if m.step == stepModel && m.cursorOnDeletableModel() {
				models := m.models()
				m.confirmingDeleteModel = true
				m.deleteModelName = models[m.modelIdx]
				return m, nil
			}
			return m, nil

		case "e":
			if m.step == stepProvider && m.activeTab == tabCustom && !m.creatingCustom && m.customIdx < len(m.customProviders) {
				m.enterEditCustomProvider()
				return m, m.cpNameInput.Focus()
			}
			return m, nil
		}

	default:
		if m.step == stepProvider && (m.creatingCustom || m.editingCustom) {
			return m.passThroughCPInput(msg)
		}
		if m.step == stepProvider && m.inManualForm {
			return m.passThroughManualInput(msg)
		}
		if m.step == stepAPIKey {
			if m.apiKeyMasked && isUserEditMsg(msg) {
				m.beginAPIKeyReplace()
			}
			var cmd tea.Cmd
			m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
			return m, cmd
		}
		if m.step == stepModel && m.customModel {
			var cmd tea.Cmd
			m.modelInput, cmd = m.modelInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
func (m providerTUIModel) updateCustomModelInput(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.customModel = false
		m.modelInput.Blur()
		m.modelInput.SetValue("")
		m.formError = ""
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.modelInput.Value())
		if name == "" {
			return m, nil
		}
		if llm.ModelListContains(m.models(), name) {
			m.formError = fmt.Sprintf("Already in list: %s", name)
			return m, nil
		}
		m.formError = ""
		persisted, err := m.persistCustomModelName(name)
		if err != nil {
			m.formError = err.Error()
			return m, nil
		}
		if !persisted {
			// No active provider context — refuse with an error message.
			m.formError = "no active provider to attach this model to"
			return m, nil
		}
		m.customModel = false
		m.modelInput.Blur()
		m.modelInput.SetValue("")
		// Reposition the cursor on the first newly-added model so the user
		// can see what just landed.
		m.refreshModelSelectionForCustom()
		return m, nil
	default:
		var cmd tea.Cmd
		m.modelInput, cmd = m.modelInput.Update(msg)
		m.formError = ""
		return m, cmd
	}
}

// persistCustomModelName appends a single model name to the active provider's
// Models list (official or custom) and saves the config. It does not change
// the active model — the user picks that explicitly from the list afterwards.
//
// Returns (persisted, error). When no provider is active (neither official
// nor custom), persisted is false and the caller decides how to handle it.
func (m *providerTUIModel) persistCustomModelName(name string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("model name must not be empty")
	}
	if m.existingCfg == nil {
		return false, nil
	}
	switch m.activeTab {
	case tabCustom:
		cp, ok := m.selectedCustomProvider()
		if !ok {
			return false, nil
		}
		entry := m.customProviderEntry(cp.name, cp.entry)
		prevEntry := cloneProviderEntry(entry)
		entry.Models = append(entry.Models, name)
		if m.existingCfg.CustomProviders == nil {
			m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
		}
		m.existingCfg.CustomProviders[cp.name] = entry
		cp.entry = entry
		m.customProviders[m.customIdx] = cp
		if m.configPath != "" {
			if err := saveConfig(m.configPath, m.existingCfg); err != nil {
				if !m.reloadConfigAfterSaveFailure() {
					m.existingCfg.CustomProviders[cp.name] = prevEntry
					cp.entry = prevEntry
					m.customProviders[m.customIdx] = cp
				}
				return false, fmt.Errorf("failed to save models: %w", err)
			}
		}
		m.savedInSession = true
		return true, nil
	case tabOfficial:
		provider := m.currentProvider()
		if provider.Name == "" {
			return false, nil
		}
		if m.existingCfg.Providers == nil {
			m.existingCfg.Providers = make(map[string]ProviderEntry)
		}
		entry := m.existingCfg.Providers[provider.Name]
		prevEntry := cloneProviderEntry(entry)
		entry.Models = append(entry.Models, name)
		m.existingCfg.Providers[provider.Name] = entry
		// Intentionally do not mutate m.providers[officialIdx].Models: that slice
		// is a read-only snapshot from the provider registry (llm.ListProviders).
		// User-added models live only in existingCfg.Providers; models() merges both
		// at display time, unlike custom tab where customProviders is the sole list.
		if m.configPath != "" {
			if err := saveConfig(m.configPath, m.existingCfg); err != nil {
				if !m.reloadConfigAfterSaveFailure() {
					m.existingCfg.Providers[provider.Name] = prevEntry
				}
				return false, fmt.Errorf("failed to save models: %w", err)
			}
		}
		m.savedInSession = true
		return true, nil
	default:
		return false, fmt.Errorf("unsupported tab for custom model: %v", m.activeTab)
	}
}

const maskedSecretDisplayLen = 20

func maskedSecretPlaceholder() string {
	return strings.Repeat("*", maskedSecretDisplayLen)
}

// isUserEditMsg reports whether msg represents user text input (typing or paste).
func isUserEditMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
		return true
	default:
		return false
	}
}

// beginAPIKeyReplace switches from the fixed-length mask placeholder to edit
// mode so the next keystroke or paste fully replaces the saved key. While
// editing, EchoPassword shows one '*' per typed character.
func (m *providerTUIModel) beginAPIKeyReplace() {
	if !m.apiKeyMasked {
		return
	}
	m.apiKeyMasked = false
	m.apiKeyOriginal = ""
	m.apiKeyInput.SetValue("")
}

// customAPIKeyForSave reports the API key to persist and whether the user edited
// the field (vs left the masked placeholder untouched).
func (m providerTUIModel) customAPIKeyForSave() (key string, edited bool) {
	if m.apiKeyMasked {
		return m.apiKeyOriginal, false
	}
	return strings.TrimSpace(m.apiKeyInput.Value()), true
}

// beginManualTokenReplace is the manual-tab equivalent of beginAPIKeyReplace.
func (m *providerTUIModel) beginManualTokenReplace() {
	if !m.manualTokenMasked {
		return
	}
	m.manualTokenMasked = false
	m.manualTokenOriginal = ""
	m.manualTokenInput.SetValue("")
}

// refreshModelSelectionForCustom moves the cursor to "Enter custom model name..."
// after the user adds models via the input field.
func (m *providerTUIModel) refreshModelSelectionForCustom() {
	models := m.models()
	m.modelIdx = 0
	if len(models) == 0 {
		return
	}
	m.modelIdx = len(models) // land on "Enter custom model name..."
}
func officialProviderEnvKeySet(p llm.Provider) bool {
	return p.EnvVar != "" && os.Getenv(p.EnvVar) != ""
}

// officialAPIKeyRequiredError mirrors the wording applyOfficialProviderConfig
// uses for the same failure, so the interactive and non-interactive paths name
// the same options in the same order (static key -> api_key_cmd -> env var).
func officialAPIKeyRequiredError(p llm.Provider) string {
	// Each alternative is independently gated: a provider with no Name still gets
	// the env-var hint, and vice versa. Naming api_key_cmd here is the point --
	// the step used to reject a provider that resolves fine through a command.
	var alternatives []string
	if p.Name != "" {
		alternatives = append(alternatives, fmt.Sprintf("set providers.%s.api_key_cmd", p.Name))
	}
	if p.EnvVar != "" {
		alternatives = append(alternatives, fmt.Sprintf("set $%s", p.EnvVar))
	}
	if len(alternatives) == 0 {
		return "API key is required"
	}
	return fmt.Sprintf("API key is required (configure it, %s)", strings.Join(alternatives, ", or "))
}

// apiKeyCmdForStep returns the api_key_cmd already configured for the provider
// the API-key step is editing, reading the same config entry loadExistingAPIKey
// reads the static key from. The step serves the Official and Custom tabs; the
// Manual tab has its own form and uses llm.auth_token_cmd instead.
//
// Trimmed because the resolver treats a whitespace-only command as unset (see
// tryOCRConfig). Returning it verbatim would let this step accept an empty API
// key on the strength of an `api_key_cmd` of "   ", saving a config the resolver
// then rejects with "no api_key or api_key_cmd configured".
func (m providerTUIModel) apiKeyCmdForStep() string {
	switch m.activeTab {
	case tabOfficial:
		if m.existingCfg == nil {
			return ""
		}
		return strings.TrimSpace(m.existingCfg.Providers[m.currentProvider().Name].APIKeyCmd)
	case tabCustom:
		if cp, ok := m.selectedCustomProvider(); ok {
			return strings.TrimSpace(m.customProviderEntry(cp.name, cp.entry).APIKeyCmd)
		}
	}
	return ""
}
func (m providerTUIModel) apiKeyStepCanConfirm() (ok bool, errMsg string) {
	if m.apiKeyOriginal != "" {
		return true, ""
	}
	if !m.apiKeyMasked && strings.TrimSpace(m.apiKeyInput.Value()) != "" {
		return true, ""
	}
	// Resolver precedence is static key -> api_key_cmd -> env var, so an already
	// configured command satisfies the requirement: the field renders blank for
	// such a provider and must still be confirmable.
	if m.apiKeyCmdForStep() != "" {
		return true, ""
	}
	if m.activeTab == tabOfficial {
		p := m.currentProvider()
		if p.AmbientAuth {
			// Reachable when an existing config is edited: an empty key is the
			// correct state for a provider that signs from the AWS chain.
			return true, ""
		}
		if officialProviderEnvKeySet(p) {
			return true, ""
		}
		return false, officialAPIKeyRequiredError(p)
	}
	if cp, ok := m.selectedCustomProvider(); ok && cp.name != "" {
		return false, fmt.Sprintf("API key is required (configure it or set custom_providers.%s.api_key_cmd)", cp.name)
	}
	return false, "API key is required"
}
func (m providerTUIModel) updateAPIKeyInput(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.apiKeyInput.Blur()
		m.step = stepModel
		m.formError = ""
		return m, nil
	case "enter":
		if ok, errMsg := m.apiKeyStepCanConfirm(); !ok {
			m.formError = errMsg
			return m, nil
		}
		m.formError = ""
		m.confirmed = true
		return m, tea.Quit
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	default:
		if m.apiKeyMasked {
			m.beginAPIKeyReplace()
		}
		var cmd tea.Cmd
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		m.formError = ""
		return m, cmd
	}
}

// reloadConfigAfterSaveFailure reloads on-disk config and refreshes derived
// provider TUI state so the UI matches persisted data after a failed save.
// Returns false when reload could not run (e.g. missing/invalid config path).
func (m *providerTUIModel) reloadConfigAfterSaveFailure() bool {
	if m.configPath == "" {
		return false
	}
	reloaded, err := loadOrCreateConfig(m.configPath)
	if err != nil {
		return false
	}
	m.existingCfg = reloaded
	m.customProviders = collectCustomProviders(reloaded)
	return true
}
func (m *providerTUIModel) loadExistingAPIKey() {
	m.apiKeyMasked = false
	m.apiKeyOriginal = ""
	m.apiKeyInput.SetValue("")
	if m.activeTab == tabCustom {
		if cp, ok := m.selectedCustomProvider(); ok && cp.entry.APIKey != "" {
			m.apiKeyOriginal = cp.entry.APIKey
			m.apiKeyMasked = true
			m.apiKeyInput.SetValue(maskedSecretPlaceholder())
		}
		return
	}
	if m.existingCfg == nil {
		return
	}
	p := m.currentProvider()
	if entry, ok := m.existingCfg.Providers[p.Name]; ok && entry.APIKey != "" {
		m.apiKeyOriginal = entry.APIKey
		m.apiKeyMasked = true
		m.apiKeyInput.SetValue(maskedSecretPlaceholder())
	}
}
func (m providerTUIModel) selectedModelFromState() string {
	if m.modelInput.Value() != "" && (m.customModel || m.isCustomModelItem(m.modelIdx)) {
		return m.modelInput.Value()
	}
	models := m.models()
	if m.modelIdx < len(models) {
		return models[m.modelIdx]
	}
	return ""
}
func (m providerTUIModel) result() providerTUIResult {
	switch m.activeTab {
	case tabOfficial:
		p := m.currentProvider()
		model := m.selectedModelFromState()
		if model == "" {
			model = m.sessionModelPickFor(p.Name)
		}

		apiKey := ""
		if m.apiKeyMasked {
			apiKey = m.apiKeyOriginal
		} else {
			apiKey = strings.TrimSpace(m.apiKeyInput.Value())
		}

		return providerTUIResult{
			provider:         p.Name,
			model:            model,
			apiKey:           apiKey,
			sessionModelPick: m.sessionModelPickSnapshot(),
		}

	case tabCustom:
		if m.creatingCustom || m.editingCustom {
			protocol := cpProtocols[m.cpProtocolIdx]
			apiKey := strings.TrimSpace(m.apiKeyInput.Value())
			if m.apiKeyMasked {
				apiKey = m.apiKeyOriginal
			}
			authHeader, _ := llm.NormalizeAuthHeader(m.cpAuthInput.Value())
			url := m.cpURLInput.Value()
			// An ambient protocol collects none of these. Clearing them also
			// covers switching an existing entry over to one: the url the
			// previous protocol needed is dead config under bedrock, and leaving
			// it behind is how a stale host outlives the change that removed it.
			if m.cpAmbientProtocol() {
				url, apiKey, authHeader = "", "", ""
			}
			r := providerTUIResult{
				provider:       m.cpNameInput.Value(),
				apiKey:         apiKey,
				isCustom:       true,
				isEdit:         m.editingCustom,
				editTargetName: m.editTargetName,
				url:            url,
				protocol:       protocol,
				authHeader:     authHeader,
			}
			// Models are managed in the model selection step, not in the
			// create/edit form. Preserve existing model/models when editing.
			if m.editingCustom {
				if idx := m.findCustomIdx(m.editTargetName); idx >= 0 {
					r.model = m.customProviders[idx].entry.Model
					r.models = m.customProviders[idx].entry.Models
				}
			}
			return r
		}
		if m.customIdx < len(m.customProviders) {
			cp := m.customProviders[m.customIdx]
			model := m.selectedModelFromState()
			if model == "" {
				model = m.sessionModelPickFor(cp.name)
			}
			if model == "" {
				model = cp.entry.Model
			}
			apiKey := ""
			if m.apiKeyMasked {
				apiKey = m.apiKeyOriginal
			} else {
				apiKey = strings.TrimSpace(m.apiKeyInput.Value())
			}
			return providerTUIResult{
				provider:         cp.name,
				model:            model,
				models:           append([]string(nil), cp.entry.Models...),
				apiKey:           apiKey,
				isCustom:         true,
				url:              cp.entry.URL,
				protocol:         cp.entry.Protocol,
				authHeader:       cp.entry.AuthHeader,
				sessionModelPick: m.sessionModelPickSnapshot(),
			}
		}
		return providerTUIResult{}

	case tabManual:
		// Trim like the Official and Custom tabs: a whitespace-only token must
		// never persist, or it wins precedence over a working auth_token_cmd
		// and sends "Authorization: Bearer  ".
		apiKey := strings.TrimSpace(m.manualTokenInput.Value())
		if m.manualTokenMasked || (apiKey == "" && m.manualTokenOriginal != "") {
			apiKey = m.manualTokenOriginal
		}
		authHeader, _ := llm.NormalizeAuthHeader(m.manualAuthHeaderInput.Value())
		return providerTUIResult{
			isManual:   true,
			url:        m.manualURLInput.Value(),
			model:      m.manualModelInput.Value(),
			apiKey:     apiKey,
			protocol:   manualProtocols[m.manualProtocolIdx],
			authHeader: authHeader,
		}
	}

	return providerTUIResult{}
}
func listCursorPrefix(isCursor bool) string {
	if isCursor {
		return "  " + tuiCursorStyle.Render(tuiCursor) + " "
	}
	return "    "
}
func listCursorPrefixForModel(isCursor, userAdded bool) string {
	if !isCursor {
		return "    "
	}
	if userAdded {
		return "  " + tuiUserModelCursorStyle.Render(tuiCursor) + " "
	}
	return listCursorPrefix(true)
}
func renderListName(name string, isCursor bool) string {
	if isCursor {
		return tuiSelectedItemStyle.Render(name)
	}
	return tuiItemStyle.Render(name)
}
func renderModelName(name string, isCursor, userAdded bool) string {
	if isCursor && userAdded {
		return tuiUserModelSelectedStyle.Render(name)
	}
	return renderListName(name, isCursor)
}

// --- View ---
func (m providerTUIModel) View() tea.View {
	var s strings.Builder
	s.WriteString("\n")

	switch m.step {
	case stepProvider:
		m.viewProvider(&s)
	case stepModel:
		m.viewModel(&s)
	case stepAPIKey:
		m.viewAPIKey(&s)
	}

	v := tea.NewView(s.String())
	v.AltScreen = true
	return v
}

const tuiCursor = "▸"

var (
	tuiTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	tuiCursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))

	tuiSelectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12"))

	tuiUserModelSelectedStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("10"))

	tuiUserModelCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10"))

	tuiItemStyle = lipgloss.NewStyle()

	tuiDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	tuiHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	tuiActiveTabStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12"))

	tuiInactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8"))

	tuiErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))
)

// --- Model-only TUI (for `ocr config model`) ---
