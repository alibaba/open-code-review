// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/llm"
)

type modelTUIConfig struct {
	Provider       llm.Provider
	CurrentModel   string
	RegistryModels []string
	ExistingCfg    *Config
	ConfigPath     string
	ProviderName   string
	IsCustom       bool
}
type modelTUIModel struct {
	width  int
	height int

	provider    llm.Provider
	models      []string
	modelIdx    int
	customModel bool
	modelInput  textinput.Model
	activeModel string

	registryModels   []string
	existingCfg      *Config
	configPath       string
	providerName     string
	isCustomProvider bool

	confirmingDeleteModel bool
	deleteModelName       string
	formError             string

	confirmed bool
	cancelled bool

	// savedInSession is true after a model add/delete was persisted during the session.
	savedInSession bool
}

// newModelTUI builds a model-only TUI for tests. It has no config path or existing
// config, so add/delete/persist operations are unavailable — use newModelTUIConfig
// in production (ocr config model). IsCustom follows whether the provider is a preset.
func newModelTUI(provider llm.Provider, currentModel string) modelTUIModel {
	registryModels := append([]string(nil), provider.Models...)
	isCustom := true
	if preset, ok := llm.LookupProvider(provider.Name); ok {
		isCustom = false
		registryModels = append([]string(nil), preset.Models...)
	}
	return newModelTUIConfig(modelTUIConfig{
		Provider:       provider,
		CurrentModel:   currentModel,
		ProviderName:   provider.Name,
		RegistryModels: registryModels,
		IsCustom:       isCustom,
	})
}
func newModelTUIConfig(cfg modelTUIConfig) modelTUIModel {
	mi := textinput.New()
	mi.Placeholder = "model name(s), comma-separated"
	mi.SetWidth(50)

	m := modelTUIModel{
		provider:         cfg.Provider,
		width:            80,
		height:           24,
		modelInput:       mi,
		activeModel:      cfg.CurrentModel,
		registryModels:   append([]string(nil), cfg.RegistryModels...),
		existingCfg:      cfg.ExistingCfg,
		configPath:       cfg.ConfigPath,
		providerName:     cfg.ProviderName,
		isCustomProvider: cfg.IsCustom,
	}

	if cfg.IsCustom {
		m.models = append([]string(nil), cfg.Provider.Models...)
	}

	if cfg.CurrentModel != "" {
		found := false
		models := m.displayModels()
		for i, model := range models {
			if model == cfg.CurrentModel {
				m.modelIdx = i
				found = true
				break
			}
		}
		if !found {
			m.modelIdx = len(models)
			m.modelInput.SetValue(cfg.CurrentModel)
		}
	}

	return m
}
func (m modelTUIModel) displayModels() []string {
	// Custom providers store the list in m.models (updated on add/delete).
	// Official providers derive the list from registry + config on each call.
	if m.isCustomProvider {
		return m.models
	}
	models := append([]string(nil), m.registryModels...)
	if m.existingCfg != nil && m.providerName != "" {
		if entry, ok := m.existingCfg.Providers[m.providerName]; ok {
			models = mergeModelLists(models, entry.Models)
		}
	}
	return models
}
func (m modelTUIModel) isUserAddedModel(name string) bool {
	if m.isCustomProvider {
		// Custom providers have no llm registry; every model comes from config and
		// is user-managed. List membership guards confirm against stale names.
		return llm.ModelListContains(m.displayModels(), name)
	}
	return isUserAddedOfficialModelName(name, m.providerName, m.registryModels, m.existingCfg)
}
func (m modelTUIModel) cursorOnUserAddedModel() bool {
	if m.confirmingDeleteModel {
		return false
	}
	models := m.displayModels()
	if m.modelIdx >= len(models) {
		return false
	}
	if m.isCustomProvider {
		return true
	}
	return m.isUserAddedModel(models[m.modelIdx])
}
func (m *modelTUIModel) adjustModelIdxAfterDelete() {
	m.syncModelsFromConfig()
	models := m.displayModels()
	if m.modelIdx >= len(models) {
		if len(models) > 0 {
			m.modelIdx = len(models) - 1
		} else {
			m.modelIdx = 0
		}
	}
}

// reloadConfigAfterSaveFailure reloads on-disk config so in-memory state matches
// persisted data after a failed save. Returns false when reload could not run.
func (m *modelTUIModel) reloadConfigAfterSaveFailure() bool {
	if m.configPath == "" {
		return false
	}
	reloaded, err := loadOrCreateConfig(m.configPath)
	if err != nil {
		return false
	}
	m.existingCfg = reloaded
	m.syncModelsFromConfig()
	return true
}
func (m *modelTUIModel) syncModelsFromConfig() {
	if !m.isCustomProvider || m.existingCfg == nil || m.providerName == "" {
		return
	}
	if entry, ok := m.existingCfg.CustomProviders[m.providerName]; ok {
		m.models = append([]string(nil), entry.Models...)
	}
}
func (m *modelTUIModel) refreshModelSelectionAfterAdd(name string) {
	models := m.displayModels()
	for i, model := range models {
		if model == name {
			m.modelIdx = i
			return
		}
	}
	if len(models) > 0 {
		m.modelIdx = len(models) - 1
	} else {
		m.modelIdx = 0
	}
}

// persistAddedModelName appends a model to the provider's Models list in config
// and saves to disk. It does not change the active model.
func (m *modelTUIModel) persistAddedModelName(name string) error {
	if name == "" {
		return fmt.Errorf("model name must not be empty")
	}
	if m.existingCfg == nil || m.providerName == "" {
		return fmt.Errorf("config not available")
	}
	if m.isCustomProvider {
		if m.existingCfg.CustomProviders == nil {
			m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
		}
		entry := m.existingCfg.CustomProviders[m.providerName]
		prevEntry := cloneProviderEntry(entry)
		entry.Models = ensureModelInList(entry.Models, name)
		m.existingCfg.CustomProviders[m.providerName] = entry
		if m.configPath != "" {
			if err := saveConfig(m.configPath, m.existingCfg); err != nil {
				if !m.reloadConfigAfterSaveFailure() {
					m.existingCfg.CustomProviders[m.providerName] = prevEntry
					m.models = append([]string(nil), prevEntry.Models...)
				}
				return fmt.Errorf("failed to save models: %w", err)
			}
		}
		m.models = append([]string(nil), entry.Models...)
		m.savedInSession = true
		return nil
	}
	if m.existingCfg.Providers == nil {
		m.existingCfg.Providers = make(map[string]ProviderEntry)
	}
	entry := m.existingCfg.Providers[m.providerName]
	prevEntry := cloneProviderEntry(entry)
	entry.Models = ensureModelInList(entry.Models, name)
	m.existingCfg.Providers[m.providerName] = entry
	if m.configPath != "" {
		if err := saveConfig(m.configPath, m.existingCfg); err != nil {
			if !m.reloadConfigAfterSaveFailure() {
				m.existingCfg.Providers[m.providerName] = prevEntry
			}
			return fmt.Errorf("failed to save models: %w", err)
		}
	}
	m.savedInSession = true
	return nil
}
func (m modelTUIModel) Init() tea.Cmd {
	return nil
}
func (m modelTUIModel) isCustomItem(idx int) bool {
	return idx == len(m.displayModels())
}
func (m modelTUIModel) itemCount() int {
	return len(m.displayModels()) + 1
}
func (m modelTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()

		if m.confirmingDeleteModel {
			return m.updateDeleteModelConfirm(key)
		}

		if m.customModel {
			switch key {
			case "esc":
				m.customModel = false
				m.modelInput.Blur()
				m.modelInput.SetValue("")
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.modelInput.Value())
				if name == "" {
					return m, nil
				}
				if llm.ModelListContains(m.displayModels(), name) {
					m.formError = fmt.Sprintf("Already in list: %s", name)
					return m, nil
				}
				m.formError = ""
				if err := m.persistAddedModelName(name); err != nil {
					m.formError = err.Error()
					return m, nil
				}
				m.customModel = false
				m.modelInput.Blur()
				m.modelInput.SetValue("")
				m.refreshModelSelectionAfterAdd(name)
				return m, nil
			default:
				var cmd tea.Cmd
				m.modelInput, cmd = m.modelInput.Update(msg)
				m.formError = ""
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
		case "d":
			if m.cursorOnUserAddedModel() {
				models := m.displayModels()
				m.confirmingDeleteModel = true
				m.deleteModelName = models[m.modelIdx]
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
func (m *modelTUIModel) updateDeleteModelConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		return m.confirmDeleteModel()
	case "n", "N", "esc":
		m.confirmingDeleteModel = false
		return *m, nil
	case "ctrl+c":
		m.cancelled = true
		return *m, tea.Quit
	}
	return *m, nil
}
func (m *modelTUIModel) confirmDeleteModel() (tea.Model, tea.Cmd) {
	if m.isCustomProvider {
		return m.confirmDeleteCustomProviderModel()
	}
	return m.confirmDeleteOfficialModel()
}
func (m *modelTUIModel) confirmDeleteCustomProviderModel() (tea.Model, tea.Cmd) {
	if !m.isUserAddedModel(m.deleteModelName) {
		m.confirmingDeleteModel = false
		return *m, nil
	}
	if m.existingCfg == nil || m.providerName == "" {
		m.confirmingDeleteModel = false
		return *m, nil
	}
	if m.existingCfg.CustomProviders == nil {
		m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
	}
	prevEntry := cloneProviderEntry(m.existingCfg.CustomProviders[m.providerName])
	prevCfgModel := ""
	if m.existingCfg.Provider == m.providerName {
		prevCfgModel = m.existingCfg.Model
	}
	entry := applyModelDeleteToEntry(m.existingCfg.CustomProviders[m.providerName], m.deleteModelName)
	clearCfgActiveModelIfDeleted(m.existingCfg, m.providerName, m.deleteModelName)
	m.existingCfg.CustomProviders[m.providerName] = entry
	if m.configPath != "" {
		if err := saveConfig(m.configPath, m.existingCfg); err != nil {
			if !m.reloadConfigAfterSaveFailure() {
				m.rollbackModelDelete(prevEntry, prevCfgModel)
			}
			m.formError = fmt.Sprintf("failed to save: %v", err)
			m.adjustModelIdxAfterDelete()
			m.confirmingDeleteModel = false
			return *m, nil
		}
	}
	m.adjustModelIdxAfterDelete()
	m.resetCustomModelInput()
	m.savedInSession = true
	m.confirmingDeleteModel = false
	return *m, nil
}
func (m *modelTUIModel) confirmDeleteOfficialModel() (tea.Model, tea.Cmd) {
	if !m.isUserAddedModel(m.deleteModelName) {
		m.confirmingDeleteModel = false
		return *m, nil
	}
	if m.existingCfg == nil || m.providerName == "" {
		m.confirmingDeleteModel = false
		return *m, nil
	}
	if m.existingCfg.Providers == nil {
		m.existingCfg.Providers = make(map[string]ProviderEntry)
	}
	prevEntry := cloneProviderEntry(m.existingCfg.Providers[m.providerName])
	prevCfgModel := ""
	if m.existingCfg.Provider == m.providerName {
		prevCfgModel = m.existingCfg.Model
	}
	entry := applyModelDeleteToEntry(m.existingCfg.Providers[m.providerName], m.deleteModelName)
	clearCfgActiveModelIfDeleted(m.existingCfg, m.providerName, m.deleteModelName)
	m.existingCfg.Providers[m.providerName] = entry
	if m.configPath != "" {
		if err := saveConfig(m.configPath, m.existingCfg); err != nil {
			if !m.reloadConfigAfterSaveFailure() {
				m.rollbackModelDelete(prevEntry, prevCfgModel)
			}
			m.formError = fmt.Sprintf("failed to save: %v", err)
			m.adjustModelIdxAfterDelete()
			m.confirmingDeleteModel = false
			return *m, nil
		}
	}
	m.adjustModelIdxAfterDelete()
	m.resetCustomModelInput()
	m.savedInSession = true
	m.confirmingDeleteModel = false
	return *m, nil
}
func (m *modelTUIModel) resetCustomModelInput() {
	m.customModel = false
	m.modelInput.SetValue("")
	m.modelInput.Blur()
}
func (m modelTUIModel) selectedModel() string {
	if m.customModel || m.isCustomItem(m.modelIdx) {
		return m.modelInput.Value()
	}
	models := m.displayModels()
	if m.modelIdx < len(models) {
		return models[m.modelIdx]
	}
	return ""
}
func (m modelTUIModel) View() tea.View {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(tuiTitleStyle.Render(fmt.Sprintf("  Select a model (%s)", m.provider.DisplayName)))
	s.WriteString("\n")
	if m.provider.BaseURL != "" {
		s.WriteString(tuiDimStyle.Render(fmt.Sprintf("  Base URL: %s", m.provider.BaseURL)))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	models := m.displayModels()
	for i, model := range models {
		isCursor := i == m.modelIdx
		if m.isCustomProvider {
			// All models are user-managed; isCursor drives green highlight on selection.
			s.WriteString(listCursorPrefixForModel(isCursor, isCursor))
			s.WriteString(renderModelName(model, isCursor, isCursor))
		} else {
			userAdded := m.isUserAddedModel(model)
			s.WriteString(listCursorPrefixForModel(isCursor, userAdded))
			s.WriteString(renderModelName(model, isCursor, userAdded))
		}
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
		s.WriteString("\n")
	}

	if m.formError != "" {
		s.WriteString("\n")
		s.WriteString(tuiErrorStyle.Render("  " + m.formError))
		s.WriteString("\n")
	}

	s.WriteString("\n")

	if m.confirmingDeleteModel {
		s.WriteString("  " + tuiSelectedItemStyle.Render(fmt.Sprintf("Delete %q? (y/n)", m.deleteModelName)))
		s.WriteString("\n")
		s.WriteString(tuiHelpStyle.Render("  y Confirm · n/Esc Cancel"))
	} else if m.cursorOnUserAddedModel() {
		s.WriteString(tuiHelpStyle.Render("  ↑/↓ Select  Enter Confirm  d Delete  Esc Cancel"))
	} else {
		s.WriteString(tuiHelpStyle.Render("  ↑/↓ Select  Enter Confirm  Esc Cancel"))
	}
	s.WriteString("\n")

	v := tea.NewView(s.String())
	v.AltScreen = true
	return v
}
