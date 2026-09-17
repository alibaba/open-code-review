// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/llm"
)

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
				m.beginAPIKeyReplace()
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
	m.cpProtocolIdx = cpProtocolIndex(entry.Protocol)
	m.cpNameInput.SetValue(cp.name)
	m.cpURLInput.SetValue(entry.URL)
	m.cpAuthInput.SetValue(entry.AuthHeader)
	if entry.APIKey != "" {
		m.apiKeyOriginal = entry.APIKey
		m.apiKeyMasked = true
		m.apiKeyInput.SetValue(maskedSecretPlaceholder())
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

const manualAuthTokenRequiredError = "Auth token is required (configure it or set llm.auth_token_cmd; whitespace-only input is not accepted)"

// manualAuthTokenCmd returns the configured llm.auth_token_cmd, which the
// resolver runs when llm.auth_token is empty. Trimmed for the same reason as
// apiKeyCmdForStep: the resolver treats a whitespace-only command as unset.
func (m providerTUIModel) manualAuthTokenCmd() string {
	if m.existingCfg == nil {
		return ""
	}
	return strings.TrimSpace(m.existingCfg.Llm.AuthTokenCmd)
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
		if m.cpAmbientProtocol() {
			return m.finishCustomForm()
		}
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
		return m.finishCustomForm()
	}
	return m, nil
}

// finishCustomForm saves the Custom provider form. It runs from the auth-header
// step for a token-based protocol and from the protocol step for an ambient one,
// which has nothing further to collect.
func (m providerTUIModel) finishCustomForm() (tea.Model, tea.Cmd) {
	if m.editingCustom {
		r := m.result()
		if err := m.applyEditCustomProviderSave(); err != nil {
			return m, nil
		}
		// Edit succeeded — drop the user into the model list for this provider.
		m.editingCustom = false
		m.editTargetName = ""
		m.apiKeyInput.SetValue("")
		m.apiKeyMasked = false
		m.apiKeyOriginal = ""
		if idx := m.findCustomIdx(r.provider); idx >= 0 {
			m.customIdx = idx
		}
		m.step = stepModel
		m.prepareModelSelection(r.provider, m.customProviderEntry(r.provider, ProviderEntry{}).Model)
		return m, nil
	}
	if m.creatingCustom {
		return m.applyCreateCustomProvider()
	}
	m.confirmed = true
	return m, tea.Quit
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
		APIKey:     strings.TrimSpace(m.apiKeyInput.Value()),
	}
	if r.protocol == llm.ProtocolAnthropicBedrock {
		entry.APIKey = ""
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
	m.prepareModelSelection(r.provider, "")
	return m, nil
}

// cloneProviderEntry deep-copies a ProviderEntry so callers (rollback paths,
// map cloning) can safely mutate the returned value without aliasing the
// original's slice or map fields.
func cloneProviderEntry(v ProviderEntry) ProviderEntry {
	out := ProviderEntry{
		APIKey:     v.APIKey,
		APIKeyCmd:  v.APIKeyCmd,
		URL:        v.URL,
		Protocol:   v.Protocol,
		Model:      v.Model,
		Models:     append([]string(nil), v.Models...),
		AuthHeader: v.AuthHeader,
		TimeoutSec: v.TimeoutSec,
		RetryCodes: append([]int(nil), v.RetryCodes...),
		AWSProfile: v.AWSProfile,
		AWSRegion:  v.AWSRegion,
	}
	if v.ExtraBody != nil {
		out.ExtraBody = make(map[string]any, len(v.ExtraBody))
		for k, val := range v.ExtraBody {
			out.ExtraBody[k] = val
		}
	}
	if v.ExtraHeaders != nil {
		out.ExtraHeaders = make(map[string]string, len(v.ExtraHeaders))
		for k, val := range v.ExtraHeaders {
			out.ExtraHeaders[k] = val
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
	if key, edited := m.customAPIKeyForSave(); edited {
		entry.APIKey = key
	}
	// Switching an entry to an ambient protocol drops the key it no longer uses,
	// rather than leaving a live credential in a file nothing reads it from.
	if entry.Protocol == llm.ProtocolAnthropicBedrock {
		entry.APIKey = ""
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
		if m.apiKeyMasked && isUserEditMsg(msg) {
			m.beginAPIKeyReplace()
		}
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	case cpStepAuthHeader:
		m.cpAuthInput, cmd = m.cpAuthInput.Update(msg)
	}
	if _, ok := msg.(tea.KeyPressMsg); ok {
		m.formError = ""
	}
	return m, cmd
}
