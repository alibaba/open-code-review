package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/open-code-review/open-code-review/internal/llm"
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

var cpProtocols = []string{"anthropic", "openai"}

type customProviderListItem struct {
	name  string
	entry ProviderEntry
}

type providerTUIResult struct {
	provider       string
	model          string
	models         []string
	apiKey         string
	isCustom       bool
	isEdit         bool
	editTargetName string
	isManual       bool
	url            string
	protocol       string
	authHeader     string
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

	// --- delete confirmation ---
	confirmingDelete      bool
	deleteTargetIdx       int
	deleteTargetName      string
	deletedProviders      []string
	confirmingDeleteModel bool
	deleteModelName       string
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
			m.manualTokenInput.SetValue(strings.Repeat("*", 20))
		}
		if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic {
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

func (m providerTUIModel) models() []string {
	switch m.activeTab {
	case tabOfficial:
		models := m.currentProvider().Models
		if m.existingCfg != nil {
			provider := m.currentProvider()
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

func (m *providerTUIModel) prepareModelSelection(currentModel string) {
	m.modelIdx = 0
	m.customModel = false
	m.modelInput.Blur()
	m.modelInput.SetValue("")

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

	switch m.activeTab {
	case tabCustom:
		cp, ok := m.selectedCustomProvider()
		if !ok {
			return nil
		}
		entry := m.customProviderEntry(cp.name, cp.entry)
		entry.Model = model
		if m.existingCfg.CustomProviders == nil {
			m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
		}
		m.existingCfg.CustomProviders[cp.name] = entry
		cp.entry = entry
		m.customProviders[m.customIdx] = cp
		if m.existingCfg.Provider == cp.name {
			m.existingCfg.Model = model
		}
	case tabOfficial:
		provider := m.currentProvider()
		if m.existingCfg.Providers == nil {
			m.existingCfg.Providers = make(map[string]ProviderEntry)
		}
		entry := m.existingCfg.Providers[provider.Name]
		entry.Model = model
		m.existingCfg.Providers[provider.Name] = entry
		if m.existingCfg.Provider == provider.Name {
			m.existingCfg.Model = model
		}
	}

	if m.configPath != "" {
		if err := saveConfig(m.configPath, m.existingCfg); err != nil {
			return fmt.Errorf("failed to save: %w", err)
		}
	}
	m.savedInSession = true
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
			if m.step == stepModel && m.activeTab == tabCustom && m.customIdx < len(m.customProviders) {
				models := m.models()
				if m.modelIdx < len(models) {
					m.confirmingDeleteModel = true
					m.deleteModelName = models[m.modelIdx]
				}
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
		for _, existing := range m.models() {
			if existing == name {
				m.formError = fmt.Sprintf("Already in list: %s", name)
				return m, nil
			}
		}
		m.formError = ""
		if err := m.addCustomModelToSession(name); err != nil {
			m.formError = err.Error()
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

// addCustomModelToSession appends a single model name to the current custom
// provider's Models list and persists in-memory state to disk. It does not
// change the active model — the user picks that explicitly from the list
// afterwards.
func (m *providerTUIModel) addCustomModelToSession(name string) error {
	if m.existingCfg == nil {
		return nil
	}
	cp, ok := m.selectedCustomProvider()
	if !ok {
		return nil
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
			m.existingCfg.CustomProviders[cp.name] = prevEntry
			cp.entry = prevEntry
			m.customProviders[m.customIdx] = cp
			return fmt.Errorf("failed to save models: %w", err)
		}
	}
	m.savedInSession = true
	return nil
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

func (m providerTUIModel) updateAPIKeyInput(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.apiKeyInput.Blur()
		m.step = stepModel
		m.formError = ""
		return m, nil
	case "enter":
		m.confirmed = true
		return m, tea.Quit
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	default:
		if m.apiKeyMasked {
			if len(key) == 1 {
				m.apiKeyMasked = false
				m.apiKeyInput.SetValue("")
			} else {
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		m.formError = ""
		return m, cmd
	}
}

func (m providerTUIModel) updateCustomProviderForm(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "esc":
		if m.cpStep == cpStepName {
			m.creatingCustom = false
			m.editingCustom = false
			m.editTargetName = ""
			m.cpNameInput.Blur()
			m.cpNameInput.SetValue("")
			m.cpURLInput.SetValue("")
			m.cpAuthInput.SetValue("")
			m.apiKeyInput.SetValue("")
			m.apiKeyMasked = false
			m.apiKeyOriginal = ""
			m.formError = ""
			return m, nil
		}
		m.blurCPStep()
		if m.editingCustom && m.cpStep == cpStepAPIKey {
			m.cpStep = cpStepBaseURL
		} else {
			m.cpStep--
		}
		m.formError = ""
		return m, m.focusCPStep()
	case "enter":
		return m.handleCustomFormEnter()
	default:
		if m.cpStep == cpStepProtocol {
			switch key {
			case "up", "k":
				if m.cpProtocolIdx > 0 {
					m.cpProtocolIdx--
				}
				return m, nil
			case "down", "j":
				if m.cpProtocolIdx < len(cpProtocols)-1 {
					m.cpProtocolIdx++
				}
				return m, nil
			}
		}
		if m.cpStep == cpStepAPIKey {
			if m.apiKeyMasked {
				if len(key) == 1 {
					m.apiKeyMasked = false
					m.apiKeyInput.SetValue("")
				} else {
					return m, nil
				}
			}
			var cmd tea.Cmd
			m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
			return m, cmd
		}
		return m.passThroughCPInput(msg)
	}
}

func (m *providerTUIModel) enterEditCustomProvider() {
	cp := m.customProviders[m.customIdx]
	entry := m.customProviderEntry(cp.name, cp.entry)
	m.editingCustom = true
	m.editTargetName = cp.name
	m.cpStep = cpStepName
	m.formError = ""
	protoIdx := 1
	if entry.Protocol == "anthropic" {
		protoIdx = 0
	}
	m.cpProtocolIdx = protoIdx
	m.cpNameInput.SetValue(cp.name)
	m.cpURLInput.SetValue(entry.URL)
	m.cpAuthInput.SetValue(entry.AuthHeader)
	if entry.APIKey != "" {
		m.apiKeyOriginal = entry.APIKey
		m.apiKeyMasked = true
		m.apiKeyInput.SetValue(strings.Repeat("*", 20))
	} else {
		m.apiKeyInput.SetValue("")
		m.apiKeyMasked = false
		m.apiKeyOriginal = ""
	}
}

func authHeaderFormError(raw string) string {
	return fmt.Sprintf(
		"Unsupported Auth Header %q. Use 'authorization' (default), 'x-api-key', or leave empty.",
		strings.TrimSpace(raw),
	)
}

func (m providerTUIModel) handleCustomFormEnter() (tea.Model, tea.Cmd) {
	switch m.cpStep {
	case cpStepName:
		name := m.cpNameInput.Value()
		if name == "" {
			return m, nil
		}
		if m.creatingCustom && m.customProviderNameTaken(name) {
			m.formError = fmt.Sprintf(`Provider "%s" already exists`, name)
			return m, nil
		}
		if m.editingCustom && name != m.editTargetName && m.customProviderNameTaken(name) {
			m.formError = fmt.Sprintf(`Provider "%s" already exists`, name)
			return m, nil
		}
		m.formError = ""
		m.cpNameInput.Blur()
		m.cpStep = cpStepProtocol
		return m, nil
	case cpStepProtocol:
		m.cpStep = cpStepBaseURL
		return m, m.cpURLInput.Focus()
	case cpStepBaseURL:
		if m.cpURLInput.Value() == "" {
			return m, nil
		}
		m.cpURLInput.Blur()
		m.cpStep = cpStepAPIKey
		if m.creatingCustom {
			m.apiKeyInput.SetValue("")
			m.apiKeyMasked = false
		}
		return m, m.focusCPStep()
	case cpStepAPIKey:
		m.apiKeyInput.Blur()
		m.cpStep = cpStepAuthHeader
		return m, m.cpAuthInput.Focus()
	case cpStepAuthHeader:
		raw := m.cpAuthInput.Value()
		if _, err := llm.NormalizeAuthHeader(raw); err != nil {
			m.formError = authHeaderFormError(raw)
			return m, nil
		}
		m.cpAuthInput.Blur()
		if m.editingCustom {
			r := m.result()
			if err := m.applyEditCustomProviderSave(); err != nil {
				return m, nil
			}
			// Edit succeeded — drop the user into the model list for this provider.
			m.editingCustom = false
			m.editTargetName = ""
			if idx := m.findCustomIdx(r.provider); idx >= 0 {
				m.customIdx = idx
			}
			m.step = stepModel
			m.prepareModelSelection(m.customProviderEntry(r.provider, ProviderEntry{}).Model)
			return m, nil
		}
		if m.creatingCustom {
			return m.applyCreateCustomProvider()
		}
		m.confirmed = true
		return m, tea.Quit
	}
	return m, nil
}

func (m providerTUIModel) applyCreateCustomProvider() (tea.Model, tea.Cmd) {
	if m.existingCfg == nil {
		m.formError = "failed to save: config not loaded"
		return m, nil
	}
	if m.configPath == "" {
		m.formError = "failed to save: config path not available"
		return m, nil
	}
	r := m.result()
	if r.provider == "" {
		m.formError = "Provider name is required"
		m.cpStep = cpStepName
		return m, m.cpNameInput.Focus()
	}
	if m.customProviderNameTaken(r.provider) {
		m.formError = fmt.Sprintf(`Provider "%s" already exists`, r.provider)
		m.cpStep = cpStepName
		return m, m.cpNameInput.Focus()
	}

	if m.existingCfg.CustomProviders == nil {
		m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
	}

	entry := ProviderEntry{
		URL:        r.url,
		Protocol:   r.protocol,
		AuthHeader: r.authHeader,
	}
	if r.apiKey != "" {
		entry.APIKey = r.apiKey
	}
	m.existingCfg.CustomProviders[r.provider] = entry

	if err := saveConfig(m.configPath, m.existingCfg); err != nil {
		m.formError = fmt.Sprintf("failed to save: %v", err)
		return m, nil
	}

	m.customProviders = collectCustomProviders(m.existingCfg)
	if idx := m.findCustomIdx(r.provider); idx >= 0 {
		m.customIdx = idx
	}
	m.creatingCustom = false
	m.cpNameInput.SetValue("")
	m.cpURLInput.SetValue("")
	m.cpAuthInput.SetValue("")
	m.apiKeyInput.SetValue("")
	m.apiKeyMasked = false
	m.apiKeyOriginal = ""
	m.formError = ""
	m.cpStep = cpStepName
	m.savedInSession = true
	// Drop into the model selection step so the user picks/adds a model for
	// the newly created provider right away.
	m.step = stepModel
	m.prepareModelSelection("")
	return m, nil
}

// cloneProviderEntry deep-copies a ProviderEntry so callers (rollback paths,
// map cloning) can safely mutate the returned value without aliasing the
// original's slice or map fields.
func cloneProviderEntry(v ProviderEntry) ProviderEntry {
	out := ProviderEntry{
		APIKey:     v.APIKey,
		URL:        v.URL,
		Protocol:   v.Protocol,
		Model:      v.Model,
		Models:     append([]string(nil), v.Models...),
		AuthHeader: v.AuthHeader,
	}
	if v.ExtraBody != nil {
		out.ExtraBody = make(map[string]any, len(v.ExtraBody))
		for k, val := range v.ExtraBody {
			// Shallow copy only: nested maps/slices inside val are not cloned.
			out.ExtraBody[k] = val
		}
	}
	return out
}

func cloneCustomProvidersMap(src map[string]ProviderEntry) map[string]ProviderEntry {
	if src == nil {
		return nil
	}
	out := make(map[string]ProviderEntry, len(src))
	for k, v := range src {
		out[k] = cloneProviderEntry(v)
	}
	return out
}

func cloneCustomProviderList(src []customProviderListItem) []customProviderListItem {
	out := make([]customProviderListItem, len(src))
	for i, cp := range src {
		out[i] = customProviderListItem{name: cp.name, entry: cloneProviderEntry(cp.entry)}
	}
	return out
}

func (m *providerTUIModel) applyEditCustomProviderSave() error {
	if m.existingCfg == nil {
		m.formError = "failed to save: config not loaded"
		return fmt.Errorf("config not loaded")
	}
	if m.configPath == "" {
		m.formError = "failed to save: config path not available"
		return fmt.Errorf("config path not available")
	}
	r := m.result()
	backupProviders := cloneCustomProvidersMap(m.existingCfg.CustomProviders)
	backupActiveProvider := m.existingCfg.Provider
	backupActiveModel := m.existingCfg.Model
	backupCustomList := cloneCustomProviderList(m.customProviders)

	if m.existingCfg.CustomProviders == nil {
		m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
	}
	entry := m.existingCfg.CustomProviders[r.editTargetName]
	if r.model != "" {
		entry.Model = r.model
	}
	if len(r.models) > 0 {
		entry.Models = append([]string(nil), r.models...)
	}
	entry.Models = ensureModelInList(entry.Models, r.model)
	// Optional fields are always applied so users can intentionally clear them.
	// To detect "user cleared the API key" vs "user left it masked/untouched",
	// apiKey is only overwritten when the user actively typed something.
	entry.URL = r.url
	entry.Protocol = r.protocol
	entry.AuthHeader = r.authHeader
	if r.apiKey != "" {
		entry.APIKey = r.apiKey
	}
	// If name changed, delete old key
	if r.editTargetName != "" && r.editTargetName != r.provider {
		if _, exists := m.existingCfg.CustomProviders[r.provider]; exists {
			m.formError = fmt.Sprintf(`Provider "%s" already exists`, r.provider)
			return fmt.Errorf("provider %q already exists", r.provider)
		}
		delete(m.existingCfg.CustomProviders, r.editTargetName)
		if m.existingCfg.Provider == r.editTargetName {
			m.existingCfg.Provider = r.provider
			m.existingCfg.Model = ""
		}
	}
	m.existingCfg.CustomProviders[r.provider] = entry

	if err := saveConfig(m.configPath, m.existingCfg); err != nil {
		m.formError = fmt.Sprintf("failed to save: %v", err)
		if reloaded, reloadErr := loadOrCreateConfig(m.configPath); reloadErr == nil {
			m.existingCfg = reloaded
			m.customProviders = collectCustomProviders(reloaded)
		} else {
			m.existingCfg.CustomProviders = backupProviders
			m.existingCfg.Provider = backupActiveProvider
			m.existingCfg.Model = backupActiveModel
			m.customProviders = backupCustomList
		}
		return fmt.Errorf("save config: %w", err)
	}
	m.customProviders = collectCustomProviders(m.existingCfg)
	if idx := m.findCustomIdx(r.provider); idx >= 0 {
		m.customIdx = idx
	}
	m.savedInSession = true
	return nil
}

func (m providerTUIModel) findCustomIdx(name string) int {
	for i, cp := range m.customProviders {
		if cp.name == name {
			return i
		}
	}
	return -1
}

func (m *providerTUIModel) blurCPStep() {
	switch m.cpStep {
	case cpStepName:
		m.cpNameInput.Blur()
	case cpStepBaseURL:
		m.cpURLInput.Blur()
	case cpStepAPIKey:
		m.apiKeyInput.Blur()
	case cpStepAuthHeader:
		m.cpAuthInput.Blur()
	}
}

func (m *providerTUIModel) focusCPStep() tea.Cmd {
	switch m.cpStep {
	case cpStepName:
		return m.cpNameInput.Focus()
	case cpStepBaseURL:
		return m.cpURLInput.Focus()
	case cpStepAPIKey:
		return m.apiKeyInput.Focus()
	case cpStepAuthHeader:
		return m.cpAuthInput.Focus()
	}
	return nil
}

func (m providerTUIModel) passThroughCPInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.cpStep {
	case cpStepName:
		m.cpNameInput, cmd = m.cpNameInput.Update(msg)
	case cpStepBaseURL:
		m.cpURLInput, cmd = m.cpURLInput.Update(msg)
	case cpStepAPIKey:
		// masked unlock is handled in updateCustomProviderForm default branch
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	case cpStepAuthHeader:
		m.cpAuthInput, cmd = m.cpAuthInput.Update(msg)
	}
	if _, ok := msg.(tea.KeyPressMsg); ok {
		m.formError = ""
	}
	return m, cmd
}

func (m providerTUIModel) updateManualForm(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "esc":
		if m.manualStep == manualStepURL {
			m.inManualForm = false
			m.manualURLInput.Blur()
			if m.existingCfg != nil {
				m.manualURLInput.SetValue(m.existingCfg.Llm.URL)
				m.manualModelInput.SetValue(m.existingCfg.Llm.Model)
				m.manualAuthHeaderInput.SetValue(m.existingCfg.Llm.AuthHeader)
				if m.existingCfg.Llm.AuthToken != "" {
					m.manualTokenOriginal = m.existingCfg.Llm.AuthToken
					m.manualTokenMasked = true
					m.manualTokenInput.SetValue(strings.Repeat("*", 20))
				} else {
					m.manualTokenInput.SetValue("")
					m.manualTokenMasked = false
					m.manualTokenOriginal = ""
				}
			} else {
				m.manualURLInput.SetValue("")
				m.manualModelInput.SetValue("")
				m.manualAuthHeaderInput.SetValue("")
				m.manualTokenInput.SetValue("")
				m.manualTokenMasked = false
				m.manualTokenOriginal = ""
			}
			m.formError = ""
			return m, nil
		}
		m.blurManualStep()
		m.manualStep--
		m.formError = ""
		return m, m.focusManualStep()
	case "enter":
		return m.handleManualFormEnter()
	default:
		if m.manualStep == manualStepProtocol {
			switch key {
			case "up", "k":
				if m.manualProtocolIdx > 0 {
					m.manualProtocolIdx--
				}
				return m, nil
			case "down", "j":
				if m.manualProtocolIdx < len(cpProtocols)-1 {
					m.manualProtocolIdx++
				}
				return m, nil
			}
		}
		if m.manualStep == manualStepAuthToken && m.manualTokenMasked {
			if len(key) == 1 {
				m.manualTokenMasked = false
				m.manualTokenInput.SetValue("")
			} else {
				return m, nil
			}
		}
		return m.passThroughManualInput(msg)
	}
}

func (m providerTUIModel) updateDeleteConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		if m.deleteTargetIdx < 0 || m.deleteTargetIdx >= len(m.customProviders) {
			m.confirmingDelete = false
			return m, nil
		}
		m.deletedProviders = append(m.deletedProviders, m.deleteTargetName)
		newList := make([]customProviderListItem, 0, len(m.customProviders)-1)
		newList = append(newList, m.customProviders[:m.deleteTargetIdx]...)
		newList = append(newList, m.customProviders[m.deleteTargetIdx+1:]...)
		m.customProviders = newList
		if m.customIdx >= len(m.customProviders) && m.customIdx > 0 {
			m.customIdx = len(m.customProviders) - 1
		}
		if m.existingCfg != nil {
			if m.existingCfg.CustomProviders != nil {
				delete(m.existingCfg.CustomProviders, m.deleteTargetName)
			}
			if m.existingCfg.Provider == m.deleteTargetName {
				m.existingCfg.Provider = ""
				m.existingCfg.Model = ""
			}
			if m.configPath != "" {
				if err := saveConfig(m.configPath, m.existingCfg); err != nil {
					if reloaded, reloadErr := loadOrCreateConfig(m.configPath); reloadErr == nil {
						m.existingCfg = reloaded
						m.customProviders = collectCustomProviders(reloaded)
					}
					m.formError = fmt.Sprintf("failed to save: %v", err)
					m.confirmingDelete = false
					return m, nil
				}
			}
		}
		m.savedInSession = true
		m.confirmingDelete = false
		return m, nil
	case "n", "N", "esc":
		m.confirmingDelete = false
		return m, nil
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m providerTUIModel) updateDeleteModelConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		if m.customIdx >= len(m.customProviders) {
			m.confirmingDeleteModel = false
			return m, nil
		}
		models := m.models()
		if m.modelIdx < len(models) {
			cp := m.customProviders[m.customIdx]
			cp.entry.Models = removeModels(cp.entry.Models, []string{m.deleteModelName})
			if cp.entry.Model == m.deleteModelName {
				cp.entry.Model = ""
			}
			if m.existingCfg != nil && m.existingCfg.Provider == cp.name &&
				m.existingCfg.Model == m.deleteModelName {
				m.existingCfg.Model = ""
			}
			m.customProviders[m.customIdx] = cp
			if m.existingCfg != nil {
				if m.existingCfg.CustomProviders == nil {
					m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
				}
				m.existingCfg.CustomProviders[cp.name] = cp.entry
			}
			if m.configPath != "" {
				if err := saveConfig(m.configPath, m.existingCfg); err != nil {
					if reloaded, reloadErr := loadOrCreateConfig(m.configPath); reloadErr == nil {
						m.existingCfg = reloaded
						m.customProviders = collectCustomProviders(reloaded)
					}
					m.formError = fmt.Sprintf("failed to save: %v", err)
					m.confirmingDeleteModel = false
					return m, nil
				}
			}
			updated := m.models()
			if m.modelIdx >= len(updated) {
				if len(updated) > 0 {
					m.modelIdx = len(updated) - 1
				} else {
					m.modelIdx = 0
				}
			}
		}
		m.savedInSession = true
		m.confirmingDeleteModel = false
		return m, nil
	case "n", "N", "esc":
		m.confirmingDeleteModel = false
		return m, nil
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m providerTUIModel) handleManualFormEnter() (tea.Model, tea.Cmd) {
	switch m.manualStep {
	case manualStepURL:
		if m.manualURLInput.Value() == "" {
			return m, nil
		}
		m.manualURLInput.Blur()
		m.manualStep = manualStepProtocol
		return m, nil
	case manualStepProtocol:
		m.manualStep = manualStepModel
		return m, m.manualModelInput.Focus()
	case manualStepModel:
		if m.manualModelInput.Value() == "" {
			return m, nil
		}
		m.manualModelInput.Blur()
		m.manualStep = manualStepAuthToken
		return m, m.manualTokenInput.Focus()
	case manualStepAuthToken:
		if m.manualTokenInput.Value() == "" && m.manualTokenOriginal == "" {
			return m, nil
		}
		m.manualTokenInput.Blur()
		m.manualStep = manualStepAuthHeader
		return m, m.manualAuthHeaderInput.Focus()
	case manualStepAuthHeader:
		raw := m.manualAuthHeaderInput.Value()
		if _, err := llm.NormalizeAuthHeader(raw); err != nil {
			m.formError = authHeaderFormError(raw)
			return m, nil
		}
		m.manualAuthHeaderInput.Blur()
		m.confirmed = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *providerTUIModel) blurManualStep() {
	switch m.manualStep {
	case manualStepURL:
		m.manualURLInput.Blur()
	case manualStepProtocol:
		// no input to blur
	case manualStepModel:
		m.manualModelInput.Blur()
	case manualStepAuthToken:
		m.manualTokenInput.Blur()
	case manualStepAuthHeader:
		m.manualAuthHeaderInput.Blur()
	}
}

func (m providerTUIModel) focusManualStep() tea.Cmd {
	switch m.manualStep {
	case manualStepURL:
		return m.manualURLInput.Focus()
	case manualStepProtocol:
		return nil
	case manualStepModel:
		return m.manualModelInput.Focus()
	case manualStepAuthToken:
		return m.manualTokenInput.Focus()
	case manualStepAuthHeader:
		return m.manualAuthHeaderInput.Focus()
	}
	return nil
}

func (m providerTUIModel) passThroughManualInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.manualStep {
	case manualStepURL:
		m.manualURLInput, cmd = m.manualURLInput.Update(msg)
	case manualStepProtocol:
		return m, nil
	case manualStepModel:
		m.manualModelInput, cmd = m.manualModelInput.Update(msg)
	case manualStepAuthToken:
		m.manualTokenInput, cmd = m.manualTokenInput.Update(msg)
	case manualStepAuthHeader:
		m.manualAuthHeaderInput, cmd = m.manualAuthHeaderInput.Update(msg)
	}
	if _, ok := msg.(tea.KeyPressMsg); ok {
		m.formError = ""
	}
	return m, cmd
}

func (m providerTUIModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProvider:
		switch m.activeTab {
		case tabOfficial:
			m.step = stepModel
			currentModel := ""
			if m.existingCfg != nil {
				if entry, ok := m.existingCfg.Providers[m.currentProvider().Name]; ok {
					currentModel = activeModelForProvider(m.existingCfg, m.currentProvider().Name, entry)
				}
			}
			m.prepareModelSelection(currentModel)
			return m, nil

		case tabCustom:
			addIdx := len(m.customProviders)
			if m.customIdx == addIdx {
				m.creatingCustom = true
				m.cpStep = cpStepName
				m.cpProtocolIdx = 0 // default anthropic
				m.formError = ""
				m.cpNameInput.SetValue("")
				m.cpURLInput.SetValue("")
				m.cpAuthInput.SetValue("")
				m.apiKeyInput.SetValue("")
				m.apiKeyMasked = false
				return m, m.cpNameInput.Focus()
			}
			cp := m.customProviders[m.customIdx]
			m.step = stepModel
			entry := m.customProviderEntry(cp.name, cp.entry)
			m.prepareModelSelection(activeModelForProvider(m.existingCfg, cp.name, entry))
			return m, nil

		case tabManual:
			m.inManualForm = true
			m.manualStep = manualStepURL
			return m, m.manualURLInput.Focus()
		}

	case stepModel:
		if m.isCustomModelItem(m.modelIdx) {
			m.customModel = true
			return m, m.modelInput.Focus()
		}
		if err := m.syncSessionModelSelection(); err != nil {
			m.formError = err.Error()
			return m, nil
		}
		m.step = stepAPIKey
		m.formError = ""
		m.loadExistingAPIKey()
		return m, m.apiKeyInput.Focus()
	}
	return m, nil
}

func (m providerTUIModel) handleUp() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProvider:
		switch m.activeTab {
		case tabOfficial:
			if m.officialIdx > 0 {
				m.officialIdx--
			} else if len(m.providers) > 0 {
				m.officialIdx = len(m.providers) - 1
			}
		case tabCustom:
			if m.customIdx > 0 {
				m.customIdx--
			} else {
				m.customIdx = m.customListCount() - 1
			}
		}
	case stepModel:
		if m.modelIdx > 0 {
			m.modelIdx--
		} else {
			m.modelIdx = m.modelCount() - 1
		}
	}
	return m, nil
}

func (m providerTUIModel) handleDown() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProvider:
		switch m.activeTab {
		case tabOfficial:
			if m.officialIdx < len(m.providers)-1 {
				m.officialIdx++
			} else if len(m.providers) > 0 {
				m.officialIdx = 0
			}
		case tabCustom:
			if m.customIdx < m.customListCount()-1 {
				m.customIdx++
			} else {
				m.customIdx = 0
			}
		}
	case stepModel:
		if m.modelIdx < m.modelCount()-1 {
			m.modelIdx++
		} else {
			m.modelIdx = 0
		}
	}
	return m, nil
}

func (m *providerTUIModel) loadExistingAPIKey() {
	m.apiKeyMasked = false
	m.apiKeyOriginal = ""
	m.apiKeyInput.SetValue("")
	if m.activeTab == tabCustom {
		if cp, ok := m.selectedCustomProvider(); ok && cp.entry.APIKey != "" {
			m.apiKeyOriginal = cp.entry.APIKey
			m.apiKeyMasked = true
			m.apiKeyInput.SetValue(strings.Repeat("*", 20))
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
		m.apiKeyInput.SetValue(strings.Repeat("*", 20))
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

		apiKey := ""
		if m.apiKeyMasked {
			apiKey = m.apiKeyOriginal
		} else {
			apiKey = m.apiKeyInput.Value()
		}

		return providerTUIResult{
			provider: p.Name,
			model:    model,
			apiKey:   apiKey,
		}

	case tabCustom:
		if m.creatingCustom || m.editingCustom {
			protocol := cpProtocols[m.cpProtocolIdx]
			apiKey := m.apiKeyInput.Value()
			if m.apiKeyMasked {
				apiKey = m.apiKeyOriginal
			}
			authHeader, _ := llm.NormalizeAuthHeader(m.cpAuthInput.Value())
			r := providerTUIResult{
				provider:       m.cpNameInput.Value(),
				apiKey:         apiKey,
				isCustom:       true,
				isEdit:         m.editingCustom,
				editTargetName: m.editTargetName,
				url:            m.cpURLInput.Value(),
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
				model = cp.entry.Model
			}
			apiKey := ""
			if m.apiKeyMasked {
				apiKey = m.apiKeyOriginal
			} else {
				apiKey = m.apiKeyInput.Value()
			}
			return providerTUIResult{
				provider:   cp.name,
				model:      model,
				models:     append([]string(nil), cp.entry.Models...),
				apiKey:     apiKey,
				isCustom:   true,
				url:        cp.entry.URL,
				protocol:   cp.entry.Protocol,
				authHeader: cp.entry.AuthHeader,
			}
		}
		return providerTUIResult{}

	case tabManual:
		apiKey := m.manualTokenInput.Value()
		if m.manualTokenMasked || (apiKey == "" && m.manualTokenOriginal != "") {
			apiKey = m.manualTokenOriginal
		}
		authHeader, _ := llm.NormalizeAuthHeader(m.manualAuthHeaderInput.Value())
		return providerTUIResult{
			isManual:   true,
			url:        m.manualURLInput.Value(),
			model:      m.manualModelInput.Value(),
			apiKey:     apiKey,
			protocol:   cpProtocols[m.manualProtocolIdx],
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

func renderListName(name string, isCursor bool) string {
	if isCursor {
		return tuiSelectedItemStyle.Render(name)
	}
	return tuiItemStyle.Render(name)
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

func renderTabBar(active providerTab) string {
	tabs := []struct {
		label string
		tab   providerTab
	}{
		{"Official", tabOfficial},
		{"Custom", tabCustom},
		{"Manual", tabManual},
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == active {
			parts = append(parts, tuiActiveTabStyle.Render("◉ "+t.label))
		} else {
			parts = append(parts, tuiInactiveTabStyle.Render("○ "+t.label))
		}
	}
	return "  " + strings.Join(parts, "    ")
}

func (m providerTUIModel) viewProvider(s *strings.Builder) {
	s.WriteString(renderTabBar(m.activeTab))
	s.WriteString("\n\n")

	switch m.activeTab {
	case tabOfficial:
		m.viewOfficialTab(s)
	case tabCustom:
		m.viewCustomTab(s)
	case tabManual:
		m.viewManualTab(s)
	}

	s.WriteString("\n")
	if m.creatingCustom || m.editingCustom || m.inManualForm {
		s.WriteString(tuiHelpStyle.Render("  Enter Confirm · Esc Back"))
	} else if m.confirmingDelete {
		s.WriteString(tuiHelpStyle.Render("  y Confirm · n/Esc Cancel"))
	} else if m.activeTab == tabCustom && m.customIdx < len(m.customProviders) {
		s.WriteString(tuiHelpStyle.Render("  Enter Select · e Edit · d Delete · Tab/Arrow Navigate · Esc Cancel"))
	} else {
		s.WriteString(tuiHelpStyle.Render("  Enter to select · Tab/Arrow keys to navigate · Esc to cancel"))
	}
	s.WriteString("\n")
}

func (m providerTUIModel) viewOfficialTab(s *strings.Builder) {
	s.WriteString(tuiTitleStyle.Render("  Select a provider"))
	s.WriteString("\n\n")

	for i, p := range m.providers {
		isCursor := i == m.officialIdx
		s.WriteString(listCursorPrefix(isCursor) + renderListName(p.DisplayName, isCursor))
		s.WriteString("\n")
	}
}

func (m providerTUIModel) viewCustomTab(s *strings.Builder) {
	if m.creatingCustom || m.editingCustom {
		m.viewCustomProviderForm(s)
		return
	}

	s.WriteString(tuiTitleStyle.Render("  Select a provider"))
	s.WriteString("\n\n")

	for i, cp := range m.customProviders {
		isCursor := i == m.customIdx
		activeModel := m.customProviderActiveModel(cp)

		s.WriteString(listCursorPrefix(isCursor) + renderListName(cp.name, isCursor))
		if activeModel != "" {
			s.WriteString("  " + tuiDimStyle.Render("("+activeModel+")"))
		}
		s.WriteString("\n")
	}

	addIdx := len(m.customProviders)
	cursor := "    "
	if m.customIdx == addIdx {
		cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
	}
	addLabel := "+ Add custom provider"
	if m.customIdx == addIdx {
		s.WriteString(cursor + tuiSelectedItemStyle.Render(addLabel))
	} else {
		s.WriteString(cursor + tuiDimStyle.Render(addLabel))
	}
	s.WriteString("\n")

	if m.confirmingDelete {
		s.WriteString("\n")
		prompt := fmt.Sprintf("  Delete %q?", m.deleteTargetName)
		// existingCfg is the config snapshot from TUI startup; it reflects
		// the on-disk active provider, not any in-session selection changes.
		if m.existingCfg != nil && m.existingCfg.Provider == m.deleteTargetName {
			prompt += " This is the active provider."
		}
		prompt += " (y/n)"
		s.WriteString(tuiSelectedItemStyle.Render(prompt))
		s.WriteString("\n")
	}
}

func (m providerTUIModel) viewCustomProviderForm(s *strings.Builder) {
	title := "  Add Custom Provider"
	if m.editingCustom {
		title = fmt.Sprintf("  Edit Custom Provider (%s)", m.editTargetName)
	}
	s.WriteString(tuiTitleStyle.Render(title))
	s.WriteString("\n\n")

	type field struct {
		label  string
		value  string
		active bool
	}

	fields := []field{
		{"Provider name", m.cpNameInput.Value(), m.cpStep == cpStepName},
		{"Protocol", cpProtocols[m.cpProtocolIdx], m.cpStep == cpStepProtocol},
		{"Base URL", m.cpURLInput.Value(), m.cpStep == cpStepBaseURL},
		{"API Key", strings.Repeat("*", len(m.apiKeyInput.Value())), m.cpStep == cpStepAPIKey},
		{"Auth Header", m.cpAuthInput.Value(), m.cpStep == cpStepAuthHeader},
	}

	for _, f := range fields {
		if f.active {
			s.WriteString("  " + tuiSelectedItemStyle.Render(f.label+":") + "\n")
			switch m.cpStep {
			case cpStepName:
				s.WriteString("    " + m.cpNameInput.View() + "\n")
			case cpStepProtocol:
				for i, proto := range cpProtocols {
					if i == m.cpProtocolIdx {
						cur := "    " + tuiCursorStyle.Render(tuiCursor) + " "
						s.WriteString(cur + tuiSelectedItemStyle.Render(proto) + "\n")
					} else {
						cur := "      "
						s.WriteString(cur + tuiItemStyle.Render(proto) + "\n")
					}
				}
			case cpStepBaseURL:
				s.WriteString("    " + m.cpURLInput.View() + "\n")
			case cpStepAPIKey:
				s.WriteString("    " + m.apiKeyInput.View() + "\n")
			case cpStepAuthHeader:
				s.WriteString("    " + m.cpAuthInput.View() + "\n")
			}
		} else {
			display := f.value
			if display == "" && f.label == "Auth Header" {
				display = "(Authorization)"
			}
			if display == "" {
				s.WriteString("  " + tuiDimStyle.Render(f.label+":") + "\n")
			} else {
				s.WriteString("  " + tuiDimStyle.Render(f.label+": "+display) + "\n")
			}
		}
	}

	if m.formError != "" {
		s.WriteString("\n")
		s.WriteString(tuiErrorStyle.Render("  " + m.formError))
		s.WriteString("\n")
	}
}

func (m providerTUIModel) viewManualTab(s *strings.Builder) {
	if !m.inManualForm {
		s.WriteString(tuiTitleStyle.Render("  Manual Configuration"))
		s.WriteString("\n\n")
		s.WriteString(tuiItemStyle.Render("  Configure LLM endpoint manually."))
		s.WriteString("\n")
		if m.existingCfg != nil && m.existingCfg.Llm.URL != "" {
			s.WriteString("\n")
			s.WriteString(tuiDimStyle.Render(fmt.Sprintf("  Current: %s (%s)", m.existingCfg.Llm.URL, m.existingCfg.Llm.Model)))
			s.WriteString("\n")
		}
		s.WriteString("\n")
		s.WriteString(tuiItemStyle.Render("  Press Enter to configure."))
		s.WriteString("\n")
		return
	}

	s.WriteString(tuiTitleStyle.Render("  Manual Configuration"))
	s.WriteString("\n\n")

	type field struct {
		label  string
		value  string
		active bool
	}

	fields := []field{
		{"URL", m.manualURLInput.Value(), m.manualStep == manualStepURL},
		{"Protocol", cpProtocols[m.manualProtocolIdx], m.manualStep == manualStepProtocol},
		{"Model", m.manualModelInput.Value(), m.manualStep == manualStepModel},
		{"Auth Token", strings.Repeat("*", len(m.manualTokenInput.Value())), m.manualStep == manualStepAuthToken},
		{"Auth Header", m.manualAuthHeaderInput.Value(), m.manualStep == manualStepAuthHeader},
	}

	for _, f := range fields {
		if f.active {
			s.WriteString("  " + tuiSelectedItemStyle.Render(f.label+":") + "\n")
			switch m.manualStep {
			case manualStepURL:
				s.WriteString("    " + m.manualURLInput.View() + "\n")
			case manualStepProtocol:
				for i, proto := range cpProtocols {
					if i == m.manualProtocolIdx {
						cur := "    " + tuiCursorStyle.Render(tuiCursor) + " "
						s.WriteString(cur + tuiSelectedItemStyle.Render(proto) + "\n")
					} else {
						cur := "      "
						s.WriteString(cur + tuiItemStyle.Render(proto) + "\n")
					}
				}
			case manualStepModel:
				s.WriteString("    " + m.manualModelInput.View() + "\n")
			case manualStepAuthToken:
				s.WriteString("    " + m.manualTokenInput.View() + "\n")
			case manualStepAuthHeader:
				s.WriteString("    " + m.manualAuthHeaderInput.View() + "\n")
			}
		} else {
			display := f.value
			if display == "" && f.label == "Auth Header" {
				display = "(Authorization)"
			}
			if display == "" {
				s.WriteString("  " + tuiDimStyle.Render(f.label+":") + "\n")
			} else {
				s.WriteString("  " + tuiDimStyle.Render(f.label+": "+display) + "\n")
			}
		}
	}

	if m.formError != "" {
		s.WriteString("\n")
		s.WriteString(tuiErrorStyle.Render("  " + m.formError))
		s.WriteString("\n")
	}
}

func (m providerTUIModel) viewModel(s *strings.Builder) {
	s.WriteString(tuiTitleStyle.Render(fmt.Sprintf("  Select a model (%s)", m.modelProviderName())))
	s.WriteString("\n\n")

	models := m.models()

	for i, model := range models {
		isCursor := i == m.modelIdx
		s.WriteString(listCursorPrefix(isCursor) + renderListName(model, isCursor))
		s.WriteString("\n")
	}

	customIdx := len(models)
	isCursor := m.modelIdx == customIdx
	customLabel := "Enter custom model name..."
	if isCursor {
		s.WriteString(listCursorPrefix(isCursor) + tuiSelectedItemStyle.Render(customLabel))
	} else {
		s.WriteString(listCursorPrefix(isCursor) + tuiDimStyle.Render(customLabel))
	}
	s.WriteString("\n")

	if m.customModel {
		s.WriteString("\n")
		s.WriteString("  " + m.modelInput.View())
		if m.formError != "" {
			s.WriteString("\n")
			s.WriteString("  " + tuiErrorStyle.Render(m.formError))
		}
		s.WriteString("\n")
	}

	s.WriteString("\n")

	if m.confirmingDeleteModel {
		s.WriteString("  " + tuiSelectedItemStyle.Render(fmt.Sprintf("Delete %q? (y/n)", m.deleteModelName)))
		s.WriteString("\n")
		s.WriteString(tuiHelpStyle.Render("  y Confirm · n/Esc Cancel"))
	} else if m.activeTab == tabCustom && m.customIdx < len(m.customProviders) {
		s.WriteString(tuiHelpStyle.Render("  ↑/↓ Select  Enter Confirm  d Delete  Esc Back"))
	} else {
		s.WriteString(tuiHelpStyle.Render("  ↑/↓ Select  Enter Confirm  Esc Back"))
	}
	s.WriteString("\n")
}

func (m providerTUIModel) viewAPIKey(s *strings.Builder) {
	var title string
	if m.activeTab == tabCustom && m.customIdx < len(m.customProviders) {
		title = fmt.Sprintf("  Enter API Key (%s)", m.customProviders[m.customIdx].name)
	} else {
		provider := m.currentProvider()
		title = fmt.Sprintf("  Enter API Key (%s)", provider.DisplayName)
	}
	s.WriteString(tuiTitleStyle.Render(title))
	s.WriteString("\n\n")

	s.WriteString("  " + m.apiKeyInput.View())
	s.WriteString("\n")

	if m.activeTab == tabOfficial {
		provider := m.currentProvider()
		if envKey := os.Getenv(provider.EnvVar); envKey != "" {
			s.WriteString("\n")
			s.WriteString(tuiDimStyle.Render(fmt.Sprintf("  $%s is set", provider.EnvVar)))
			s.WriteString("\n")
		} else {
			s.WriteString("\n")
			s.WriteString(tuiDimStyle.Render(fmt.Sprintf("  Tip: You can also set via env var %s", provider.EnvVar)))
			s.WriteString("\n")
		}
	}

	s.WriteString("\n")
	s.WriteString(tuiHelpStyle.Render("  Enter Confirm  Esc Back"))
	s.WriteString("\n")
}

// --- Styles ---

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

type modelTUIModel struct {
	width  int
	height int

	provider    llm.Provider
	models      []string
	modelIdx    int
	customModel bool
	modelInput  textinput.Model
	activeModel string

	confirmed bool
	cancelled bool
}

func newModelTUI(provider llm.Provider, currentModel string) modelTUIModel {
	mi := textinput.New()
	mi.Placeholder = "model name(s), comma-separated"
	mi.SetWidth(50)

	m := modelTUIModel{
		provider:    provider,
		models:      provider.Models,
		width:       80,
		height:      24,
		modelInput:  mi,
		activeModel: currentModel,
	}

	if currentModel != "" {
		found := false
		for i, model := range m.models {
			if model == currentModel {
				m.modelIdx = i
				found = true
				break
			}
		}
		if !found {
			m.modelIdx = len(m.models)
			m.modelInput.SetValue(currentModel)
		}
	}

	return m
}

func (m modelTUIModel) Init() tea.Cmd {
	return nil
}

func (m modelTUIModel) isCustomItem(idx int) bool {
	return idx == len(m.models)
}

func (m modelTUIModel) itemCount() int {
	return len(m.models) + 1
}

func (m modelTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()

		if m.customModel {
			switch key {
			case "esc":
				m.customModel = false
				m.modelInput.Blur()
				m.modelInput.SetValue("")
				return m, nil
			case "enter":
				if m.modelInput.Value() != "" {
					m.confirmed = true
					return m, tea.Quit
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.modelInput, cmd = m.modelInput.Update(msg)
				return m, cmd
			}
		}

		switch key {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if m.isCustomItem(m.modelIdx) {
				m.customModel = true
				return m, m.modelInput.Focus()
			}
			m.confirmed = true
			return m, tea.Quit
		case "up", "k":
			if m.modelIdx > 0 {
				m.modelIdx--
			} else {
				m.modelIdx = m.itemCount() - 1
			}
			return m, nil
		case "down", "j":
			if m.modelIdx < m.itemCount()-1 {
				m.modelIdx++
			} else {
				m.modelIdx = 0
			}
			return m, nil
		}

	default:
		if m.customModel {
			var cmd tea.Cmd
			m.modelInput, cmd = m.modelInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m modelTUIModel) selectedModel() string {
	if m.customModel || m.isCustomItem(m.modelIdx) {
		return m.modelInput.Value()
	}
	if m.modelIdx < len(m.models) {
		return m.models[m.modelIdx]
	}
	return ""
}

func (m modelTUIModel) View() tea.View {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(tuiTitleStyle.Render(fmt.Sprintf("  Select a model (%s)", m.provider.DisplayName)))
	s.WriteString("\n\n")

	for i, model := range m.models {
		isCursor := i == m.modelIdx
		s.WriteString(listCursorPrefix(isCursor) + renderListName(model, isCursor))
		s.WriteString("\n")
	}

	customIdx := len(m.models)
	isCursor := m.modelIdx == customIdx
	customLabel := "Enter custom model name..."
	if isCursor {
		s.WriteString(listCursorPrefix(isCursor) + tuiSelectedItemStyle.Render(customLabel))
	} else {
		s.WriteString(listCursorPrefix(isCursor) + tuiDimStyle.Render(customLabel))
	}
	s.WriteString("\n")

	if m.customModel {
		s.WriteString("\n")
		s.WriteString("  " + m.modelInput.View())
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(tuiHelpStyle.Render("  ↑/↓ Select  Enter Confirm  Esc Cancel"))
	s.WriteString("\n")

	v := tea.NewView(s.String())
	v.AltScreen = true
	return v
}
