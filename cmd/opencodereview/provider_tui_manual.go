// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/llm"
)

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
					m.manualTokenInput.SetValue(maskedSecretPlaceholder())
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
				if m.manualProtocolIdx < len(manualProtocols)-1 {
					m.manualProtocolIdx++
				}
				return m, nil
			}
		}
		if m.manualStep == manualStepAuthToken && m.manualTokenMasked {
			m.beginManualTokenReplace()
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
		switch m.activeTab {
		case tabCustom:
			return m.confirmDeleteCustomModel()
		case tabOfficial:
			return m.confirmDeleteOfficialModel()
		default:
			m.confirmingDeleteModel = false
		}
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
func (m providerTUIModel) confirmDeleteCustomModel() (tea.Model, tea.Cmd) {
	if m.customIdx >= len(m.customProviders) {
		m.confirmingDeleteModel = false
		return m, nil
	}
	models := m.models()
	if m.modelIdx < len(models) {
		cp := m.customProviders[m.customIdx]
		prevEntry := cloneProviderEntry(cp.entry)
		prevCfgModel := ""
		if m.existingCfg != nil && m.existingCfg.Provider == cp.name {
			prevCfgModel = m.existingCfg.Model
		}
		cp.entry = applyModelDeleteToEntry(cp.entry, m.deleteModelName)
		clearCfgActiveModelIfDeleted(m.existingCfg, cp.name, m.deleteModelName)
		m.customProviders[m.customIdx] = cp
		if m.existingCfg != nil {
			if m.existingCfg.CustomProviders == nil {
				m.existingCfg.CustomProviders = make(map[string]ProviderEntry)
			}
			m.existingCfg.CustomProviders[cp.name] = cp.entry
		}
		if m.configPath != "" {
			if err := saveConfig(m.configPath, m.existingCfg); err != nil {
				if !m.reloadConfigAfterSaveFailure() {
					cp.entry = prevEntry
					m.customProviders[m.customIdx] = cp
					if m.existingCfg != nil {
						m.existingCfg.CustomProviders[cp.name] = prevEntry
						rollbackCfgActiveModel(m.existingCfg, cp.name, prevCfgModel)
					}
				}
				m.formError = fmt.Sprintf("failed to save: %v", err)
				m.adjustModelIdxAfterDelete()
				m.confirmingDeleteModel = false
				return m, nil
			}
		}
		m.adjustModelIdxAfterDelete()
		m.savedInSession = true
	}
	m.confirmingDeleteModel = false
	return m, nil
}
func (m providerTUIModel) confirmDeleteOfficialModel() (tea.Model, tea.Cmd) {
	if !m.isUserAddedOfficialModel(m.deleteModelName) {
		m.confirmingDeleteModel = false
		return m, nil
	}
	provider := m.currentProvider()
	if m.existingCfg == nil || provider.Name == "" {
		m.confirmingDeleteModel = false
		return m, nil
	}
	if m.existingCfg.Providers == nil {
		m.existingCfg.Providers = make(map[string]ProviderEntry)
	}
	prevEntry := cloneProviderEntry(m.existingCfg.Providers[provider.Name])
	prevCfgModel := ""
	if m.existingCfg.Provider == provider.Name {
		prevCfgModel = m.existingCfg.Model
	}
	entry := applyModelDeleteToEntry(m.existingCfg.Providers[provider.Name], m.deleteModelName)
	clearCfgActiveModelIfDeleted(m.existingCfg, provider.Name, m.deleteModelName)
	m.existingCfg.Providers[provider.Name] = entry
	if m.configPath != "" {
		if err := saveConfig(m.configPath, m.existingCfg); err != nil {
			if !m.reloadConfigAfterSaveFailure() {
				m.existingCfg.Providers[provider.Name] = prevEntry
				rollbackCfgActiveModel(m.existingCfg, provider.Name, prevCfgModel)
			}
			m.formError = fmt.Sprintf("failed to save: %v", err)
			m.adjustModelIdxAfterDelete()
			m.confirmingDeleteModel = false
			return m, nil
		}
	}
	m.adjustModelIdxAfterDelete()
	// In-memory delete succeeded; configPath may be empty in tests (no disk write).
	m.savedInSession = true
	m.confirmingDeleteModel = false
	return m, nil
}
func (m *providerTUIModel) adjustModelIdxAfterDelete() {
	updated := m.models()
	if m.modelIdx >= len(updated) {
		if len(updated) > 0 {
			m.modelIdx = len(updated) - 1
		} else {
			m.modelIdx = 0
		}
	}
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
		// Same precedence as the provider tabs: an already configured
		// llm.auth_token_cmd stands in for a typed or saved token.
		if strings.TrimSpace(m.manualTokenInput.Value()) == "" && m.manualTokenOriginal == "" && m.manualAuthTokenCmd() == "" {
			m.formError = manualAuthTokenRequiredError
			return m, nil
		}
		m.formError = ""
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
func (m *providerTUIModel) focusManualStep() tea.Cmd {
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
		if m.manualTokenMasked && isUserEditMsg(msg) {
			m.beginManualTokenReplace()
		}
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
			m.prepareModelSelection(m.currentProvider().Name, currentModel)
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
			m.prepareModelSelection(cp.name, activeModelForProvider(m.existingCfg, cp.name, entry))
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
		if m.activeTab == tabOfficial && m.currentProvider().AmbientAuth {
			// An ambient-auth provider has no key to collect, so the model step
			// is the last one. Showing an API-key prompt that must be left blank
			// would read as a step the user failed to complete.
			m.formError = ""
			m.confirmed = true
			return m, tea.Quit
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
