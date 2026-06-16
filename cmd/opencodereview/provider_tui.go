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
	cpStepModel
	cpStepModels
	cpStepAPIKey
	cpStepAuthHeader
)

type manualStep int

const (
	manualStepURL manualStep = iota
	manualStepModel
	manualStepAuthToken
)

var cpProtocols = []string{"anthropic", "openai"}

type customProviderListItem struct {
	name  string
	entry ProviderEntry
}

type providerTUIResult struct {
	provider   string
	model      string
	models     []string
	apiKey     string
	isCustom   bool
	isManual   bool
	url        string
	protocol   string
	authHeader string
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
	cpStep          customProviderStep
	cpProtocolIdx   int
	cpNameInput     textinput.Model
	cpURLInput      textinput.Model
	cpModelInput    textinput.Model
	cpModelsInput   textinput.Model
	cpAuthInput     textinput.Model

	// --- tab: manual ---
	inManualForm     bool
	manualStep       manualStep
	manualURLInput   textinput.Model
	manualModelInput textinput.Model
	manualTokenInput textinput.Model

	// --- shared model/api-key steps (official + existing custom) ---
	modelIdx    int
	customModel bool
	modelInput  textinput.Model

	apiKeyInput    textinput.Model
	apiKeyMasked   bool
	apiKeyOriginal string

	existingCfg *Config
	confirmed   bool
	cancelled   bool
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

func newProviderTUI(cfg *Config) providerTUIModel {
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
	mi.Placeholder = "model name"
	mi.SetWidth(40)

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

	cpModel := textinput.New()
	cpModel.Placeholder = "model name"
	cpModel.SetWidth(40)

	cpModels := textinput.New()
	cpModels.Placeholder = "optional comma-separated models"
	cpModels.SetWidth(50)

	cpAuth := textinput.New()
	cpAuth.Placeholder = "optional, leave empty for default (Authorization)"
	cpAuth.SetWidth(55)

	manualURL := textinput.New()
	manualURL.Placeholder = "enter your API base URL"
	manualURL.SetWidth(50)

	manualModel := textinput.New()
	manualModel.Placeholder = "enter model name"
	manualModel.SetWidth(40)

	manualToken := textinput.New()
	manualToken.Placeholder = "enter your auth token"
	manualToken.SetWidth(50)
	manualToken.EchoMode = textinput.EchoPassword
	manualToken.EchoCharacter = '*'

	m := providerTUIModel{
		providers:        providers,
		existingCfg:      cfg,
		modelInput:       mi,
		apiKeyInput:      ai,
		cpNameInput:      cpName,
		cpURLInput:       cpURL,
		cpModelInput:     cpModel,
		cpModelsInput:    cpModels,
		cpAuthInput:      cpAuth,
		manualURLInput:   manualURL,
		manualModelInput: manualModel,
		manualTokenInput: manualToken,
		width:            80,
		height:           24,
		activeTab:        tabOfficial,
		customProviders:  collectCustomProviders(cfg),
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

	if cfg.Llm.URL != "" {
		m.manualURLInput.SetValue(cfg.Llm.URL)
		m.manualModelInput.SetValue(cfg.Llm.Model)
		if cfg.Llm.AuthToken != "" {
			m.manualTokenInput.SetValue(cfg.Llm.AuthToken)
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

		if m.step == stepProvider && m.creatingCustom {
			return m.updateCustomProviderForm(key, msg)
		}

		if m.step == stepProvider && m.inManualForm {
			return m.updateManualForm(key, msg)
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
				}
			}
			return m, nil

		case "right", "l":
			if m.step == stepProvider {
				if m.activeTab < tabCount-1 {
					m.activeTab++
				}
			}
			return m, nil

		case "tab":
			if m.step == stepProvider {
				m.activeTab = (m.activeTab + 1) % tabCount
			}
			return m, nil
		}

	default:
		if m.step == stepProvider && m.creatingCustom {
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
		return m, nil
	case "enter":
		if m.modelInput.Value() != "" {
			m.customModel = false
			m.modelInput.Blur()
			m.step = stepAPIKey
			m.loadExistingAPIKey()
			return m, m.apiKeyInput.Focus()
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.modelInput, cmd = m.modelInput.Update(msg)
		return m, cmd
	}
}

func (m providerTUIModel) updateAPIKeyInput(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.apiKeyInput.Blur()
		m.step = stepModel
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
			m.cpNameInput.Blur()
			return m, nil
		}
		m.blurCPStep()
		m.cpStep--
		return m, m.focusCPStep()
	case "enter":
		return m.handleCustomFormEnter()
	case "up", "k":
		if m.cpStep == cpStepProtocol && m.cpProtocolIdx > 0 {
			m.cpProtocolIdx--
		}
		return m, nil
	case "down", "j":
		if m.cpStep == cpStepProtocol && m.cpProtocolIdx < len(cpProtocols)-1 {
			m.cpProtocolIdx++
		}
		return m, nil
	default:
		return m.passThroughCPInput(msg)
	}
}

func (m providerTUIModel) handleCustomFormEnter() (tea.Model, tea.Cmd) {
	switch m.cpStep {
	case cpStepName:
		if m.cpNameInput.Value() == "" {
			return m, nil
		}
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
		m.cpStep = cpStepModel
		return m, m.cpModelInput.Focus()
	case cpStepModel:
		if m.cpModelInput.Value() == "" {
			return m, nil
		}
		m.cpModelInput.Blur()
		m.cpStep = cpStepModels
		return m, m.cpModelsInput.Focus()
	case cpStepModels:
		m.cpModelsInput.Blur()
		m.cpStep = cpStepAPIKey
		m.apiKeyInput.SetValue("")
		m.apiKeyMasked = false
		return m, m.apiKeyInput.Focus()
	case cpStepAPIKey:
		m.apiKeyInput.Blur()
		m.cpStep = cpStepAuthHeader
		return m, m.cpAuthInput.Focus()
	case cpStepAuthHeader:
		m.cpAuthInput.Blur()
		m.confirmed = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *providerTUIModel) blurCPStep() {
	switch m.cpStep {
	case cpStepName:
		m.cpNameInput.Blur()
	case cpStepBaseURL:
		m.cpURLInput.Blur()
	case cpStepModel:
		m.cpModelInput.Blur()
	case cpStepModels:
		m.cpModelsInput.Blur()
	case cpStepAPIKey:
		m.apiKeyInput.Blur()
	case cpStepAuthHeader:
		m.cpAuthInput.Blur()
	}
}

func (m providerTUIModel) focusCPStep() tea.Cmd {
	switch m.cpStep {
	case cpStepName:
		return m.cpNameInput.Focus()
	case cpStepBaseURL:
		return m.cpURLInput.Focus()
	case cpStepModel:
		return m.cpModelInput.Focus()
	case cpStepModels:
		return m.cpModelsInput.Focus()
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
	case cpStepModel:
		m.cpModelInput, cmd = m.cpModelInput.Update(msg)
	case cpStepModels:
		m.cpModelsInput, cmd = m.cpModelsInput.Update(msg)
	case cpStepAPIKey:
		if m.apiKeyMasked {
			return m, nil
		}
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	case cpStepAuthHeader:
		m.cpAuthInput, cmd = m.cpAuthInput.Update(msg)
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
				m.manualTokenInput.SetValue(m.existingCfg.Llm.AuthToken)
			} else {
				m.manualURLInput.SetValue("")
				m.manualModelInput.SetValue("")
				m.manualTokenInput.SetValue("")
			}
			return m, nil
		}
		m.blurManualStep()
		m.manualStep--
		return m, m.focusManualStep()
	case "enter":
		return m.handleManualFormEnter()
	default:
		return m.passThroughManualInput(msg)
	}
}

func (m providerTUIModel) handleManualFormEnter() (tea.Model, tea.Cmd) {
	switch m.manualStep {
	case manualStepURL:
		if m.manualURLInput.Value() == "" {
			return m, nil
		}
		m.manualURLInput.Blur()
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
		m.manualTokenInput.Blur()
		m.confirmed = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *providerTUIModel) blurManualStep() {
	switch m.manualStep {
	case manualStepURL:
		m.manualURLInput.Blur()
	case manualStepModel:
		m.manualModelInput.Blur()
	case manualStepAuthToken:
		m.manualTokenInput.Blur()
	}
}

func (m providerTUIModel) focusManualStep() tea.Cmd {
	switch m.manualStep {
	case manualStepURL:
		return m.manualURLInput.Focus()
	case manualStepModel:
		return m.manualModelInput.Focus()
	case manualStepAuthToken:
		return m.manualTokenInput.Focus()
	}
	return nil
}

func (m providerTUIModel) passThroughManualInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.manualStep {
	case manualStepURL:
		m.manualURLInput, cmd = m.manualURLInput.Update(msg)
	case manualStepModel:
		m.manualModelInput, cmd = m.manualModelInput.Update(msg)
	case manualStepAuthToken:
		m.manualTokenInput, cmd = m.manualTokenInput.Update(msg)
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
				if entry, ok := m.existingCfg.Providers[m.currentProvider().Name]; ok && entry.Model != "" {
					currentModel = entry.Model
				}
			}
			m.prepareModelSelection(currentModel)
			return m, nil

		case tabCustom:
			addIdx := len(m.customProviders)
			if m.customIdx == addIdx {
				m.creatingCustom = true
				m.cpStep = cpStepName
				m.cpProtocolIdx = 1 // default openai
				m.cpNameInput.SetValue("")
				m.cpURLInput.SetValue("")
				m.cpModelInput.SetValue("")
				m.cpModelsInput.SetValue("")
				m.cpAuthInput.SetValue("")
				m.apiKeyInput.SetValue("")
				m.apiKeyMasked = false
				return m, m.cpNameInput.Focus()
			}
			cp := m.customProviders[m.customIdx]
			m.step = stepModel
			m.prepareModelSelection(cp.entry.Model)
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
		m.step = stepAPIKey
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
			}
		case tabCustom:
			if m.customIdx > 0 {
				m.customIdx--
			}
		}
	case stepModel:
		if m.modelIdx > 0 {
			m.modelIdx--
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
			}
		case tabCustom:
			if m.customIdx < m.customListCount()-1 {
				m.customIdx++
			}
		}
	case stepModel:
		if m.modelIdx < m.modelCount()-1 {
			m.modelIdx++
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
		if m.creatingCustom {
			protocol := cpProtocols[m.cpProtocolIdx]
			models := mergeModelLists(
				[]string{m.cpModelInput.Value()},
				strings.Split(m.cpModelsInput.Value(), ","),
			)
			return providerTUIResult{
				provider:   m.cpNameInput.Value(),
				model:      m.cpModelInput.Value(),
				models:     models,
				apiKey:     m.apiKeyInput.Value(),
				isCustom:   true,
				url:        m.cpURLInput.Value(),
				protocol:   protocol,
				authHeader: m.cpAuthInput.Value(),
			}
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
				models:     mergeModelLists([]string{model}, cp.entry.Models),
				apiKey:     apiKey,
				isCustom:   true,
				url:        cp.entry.URL,
				protocol:   cp.entry.Protocol,
				authHeader: cp.entry.AuthHeader,
			}
		}
		return providerTUIResult{}

	case tabManual:
		return providerTUIResult{
			isManual: true,
			url:      m.manualURLInput.Value(),
			model:    m.manualModelInput.Value(),
			apiKey:   m.manualTokenInput.Value(),
		}
	}

	return providerTUIResult{}
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
	if m.creatingCustom || m.inManualForm {
		s.WriteString(tuiHelpStyle.Render("  Enter Confirm · Esc Back"))
	} else {
		s.WriteString(tuiHelpStyle.Render("  Enter to select · Tab/Arrow keys to navigate · Esc to cancel"))
	}
	s.WriteString("\n")
}

func (m providerTUIModel) viewOfficialTab(s *strings.Builder) {
	s.WriteString(tuiTitleStyle.Render("  Select a provider"))
	s.WriteString("\n\n")

	for i, p := range m.providers {
		cursor := "    "
		if i == m.officialIdx {
			cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
		}
		name := p.DisplayName
		if i == m.officialIdx {
			s.WriteString(cursor + tuiSelectedItemStyle.Render(name))
		} else {
			s.WriteString(cursor + tuiItemStyle.Render(name))
		}
		s.WriteString("\n")
	}
}

func (m providerTUIModel) viewCustomTab(s *strings.Builder) {
	if m.creatingCustom {
		m.viewCustomProviderForm(s)
		return
	}

	s.WriteString(tuiTitleStyle.Render("  Select a provider"))
	s.WriteString("\n\n")

	for i, cp := range m.customProviders {
		cursor := "    "
		if i == m.customIdx {
			cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
		}
		label := cp.name
		if cp.entry.Model != "" {
			label += "  " + tuiDimStyle.Render("("+cp.entry.Model+")")
		}
		if i == m.customIdx {
			s.WriteString(cursor + tuiSelectedItemStyle.Render(cp.name))
			if cp.entry.Model != "" {
				s.WriteString("  " + tuiDimStyle.Render("("+cp.entry.Model+")"))
			}
		} else {
			s.WriteString(cursor + label)
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
}

func (m providerTUIModel) viewCustomProviderForm(s *strings.Builder) {
	s.WriteString(tuiTitleStyle.Render("  Add Custom Provider"))
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
		{"Model", m.cpModelInput.Value(), m.cpStep == cpStepModel},
		{"Models", m.cpModelsInput.Value(), m.cpStep == cpStepModels},
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
					cur := "      "
					if i == m.cpProtocolIdx {
						cur = "    " + tuiCursorStyle.Render(tuiCursor) + " "
					}
					if i == m.cpProtocolIdx {
						s.WriteString(cur + tuiSelectedItemStyle.Render(proto) + "\n")
					} else {
						s.WriteString(cur + tuiItemStyle.Render(proto) + "\n")
					}
				}
			case cpStepBaseURL:
				s.WriteString("    " + m.cpURLInput.View() + "\n")
			case cpStepModel:
				s.WriteString("    " + m.cpModelInput.View() + "\n")
			case cpStepModels:
				s.WriteString("    " + m.cpModelsInput.View() + "\n")
			case cpStepAPIKey:
				s.WriteString("    " + m.apiKeyInput.View() + "\n")
			case cpStepAuthHeader:
				s.WriteString("    " + m.cpAuthInput.View() + "\n")
			}
		} else if f.value != "" {
			s.WriteString("  " + tuiDimStyle.Render(f.label+": "+f.value) + "\n")
		}
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
		{"Model", m.manualModelInput.Value(), m.manualStep == manualStepModel},
		{"Auth Token", strings.Repeat("*", len(m.manualTokenInput.Value())), m.manualStep == manualStepAuthToken},
	}

	for _, f := range fields {
		if f.active {
			s.WriteString("  " + tuiSelectedItemStyle.Render(f.label+":") + "\n")
			switch m.manualStep {
			case manualStepURL:
				s.WriteString("    " + m.manualURLInput.View() + "\n")
			case manualStepModel:
				s.WriteString("    " + m.manualModelInput.View() + "\n")
			case manualStepAuthToken:
				s.WriteString("    " + m.manualTokenInput.View() + "\n")
			}
		} else if f.value != "" {
			s.WriteString("  " + tuiDimStyle.Render(f.label+": "+f.value) + "\n")
		}
	}
}

func (m providerTUIModel) viewModel(s *strings.Builder) {
	s.WriteString(tuiTitleStyle.Render(fmt.Sprintf("  Select a model (%s)", m.modelProviderName())))
	s.WriteString("\n\n")

	models := m.models()
	for i, model := range models {
		cursor := "    "
		if i == m.modelIdx {
			cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
		}
		if i == m.modelIdx {
			s.WriteString(cursor + tuiSelectedItemStyle.Render(model))
		} else {
			s.WriteString(cursor + tuiItemStyle.Render(model))
		}
		s.WriteString("\n")
	}

	customIdx := len(models)
	cursor := "    "
	if m.modelIdx == customIdx {
		cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
	}
	customLabel := "Enter custom model name..."
	if m.modelIdx == customIdx {
		s.WriteString(cursor + tuiSelectedItemStyle.Render(customLabel))
	} else {
		s.WriteString(cursor + tuiDimStyle.Render(customLabel))
	}
	s.WriteString("\n")

	if m.customModel {
		s.WriteString("\n")
		s.WriteString("  " + m.modelInput.View())
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(tuiHelpStyle.Render("  ↑/↓ Select  Enter Confirm  Esc Back"))
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

	confirmed bool
	cancelled bool
}

func newModelTUI(provider llm.Provider, currentModel string) modelTUIModel {
	mi := textinput.New()
	mi.Placeholder = "model name"
	mi.SetWidth(40)

	m := modelTUIModel{
		provider:   provider,
		models:     provider.Models,
		width:      80,
		height:     24,
		modelInput: mi,
	}

	if currentModel != "" {
		found := false
		for i, model := range provider.Models {
			if model == currentModel {
				m.modelIdx = i
				found = true
				break
			}
		}
		if !found {
			m.modelIdx = len(provider.Models)
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
			}
			return m, nil
		case "down", "j":
			if m.modelIdx < m.itemCount()-1 {
				m.modelIdx++
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
		cursor := "    "
		if i == m.modelIdx {
			cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
		}
		if i == m.modelIdx {
			s.WriteString(cursor + tuiSelectedItemStyle.Render(model))
		} else {
			s.WriteString(cursor + tuiItemStyle.Render(model))
		}
		s.WriteString("\n")
	}

	customIdx := len(m.models)
	cursor := "    "
	if m.modelIdx == customIdx {
		cursor = "  " + tuiCursorStyle.Render(tuiCursor) + " "
	}
	customLabel := "Enter custom model name..."
	if m.modelIdx == customIdx {
		s.WriteString(cursor + tuiSelectedItemStyle.Render(customLabel))
	} else {
		s.WriteString(cursor + tuiDimStyle.Render(customLabel))
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
