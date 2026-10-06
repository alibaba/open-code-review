// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"strings"
)

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
		if activeModel := m.officialProviderActiveModel(p); activeModel != "" {
			s.WriteString("  " + tuiDimStyle.Render("("+activeModel+")"))
		}
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
		// userAdded here means "green highlight when selected" for user-managed rows.
		highlight := isCursor

		s.WriteString(listCursorPrefixForModel(isCursor, highlight))
		s.WriteString(renderModelName(cp.name, isCursor, highlight))
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
	}
	if !m.cpAmbientProtocol() {
		fields = append(fields,
			field{"Base URL", m.cpURLInput.Value(), m.cpStep == cpStepBaseURL},
			field{"API Key", strings.Repeat("*", len(m.apiKeyInput.Value())), m.cpStep == cpStepAPIKey},
			field{"Auth Header", m.cpAuthInput.Value(), m.cpStep == cpStepAuthHeader},
		)
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
				if m.cpAmbientProtocol() {
					s.WriteString(tuiDimStyle.Render("    credentials come from the AWS chain; pin a region or profile with `ocr config set custom_providers."+m.cpNameInput.Value()+".aws_region <r>`") + "\n")
				}
			case cpStepBaseURL:
				s.WriteString("    " + m.cpURLInput.View() + "\n")
			case cpStepAPIKey:
				s.WriteString("    " + m.apiKeyInput.View() + "\n")
				if m.apiKeyMasked && m.apiKeyOriginal != "" {
					s.WriteString(tuiDimStyle.Render("    "+savedSecretReplaceHint(m.apiKeyOriginal)) + "\n")
				}
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
		{"Protocol", manualProtocols[m.manualProtocolIdx], m.manualStep == manualStepProtocol},
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
				for i, proto := range manualProtocols {
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
				if m.manualTokenMasked && m.manualTokenOriginal != "" {
					s.WriteString(tuiDimStyle.Render("    "+savedSecretReplaceHint(m.manualTokenOriginal)) + "\n")
				}
				if m.manualAuthTokenCmd() != "" {
					s.WriteString(tuiDimStyle.Render(keyCmdConfiguredHintLine("    ", "llm.auth_token_cmd")) + "\n")
				}
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
		if m.activeTab == tabOfficial {
			userAdded := m.isUserAddedOfficialModel(model)
			s.WriteString(listCursorPrefixForModel(isCursor, userAdded))
			s.WriteString(renderModelName(model, isCursor, userAdded))
		} else {
			// Custom tab: all models are user-managed; pass isCursor as userAdded
			// so green highlight applies only to the selected row (not registry semantics).
			s.WriteString(listCursorPrefixForModel(isCursor, isCursor))
			s.WriteString(renderModelName(model, isCursor, isCursor))
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
	} else if m.cursorOnDeletableModel() {
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

	// When an API key is already saved, the input starts masked. Surface a
	// hint so the user knows typing or pasting will replace the saved key,
	// and show a short prefix fingerprint so they can sanity-check which key
	// is currently saved without exposing it.
	if m.apiKeyMasked && m.apiKeyOriginal != "" {
		s.WriteString("\n")
		s.WriteString(tuiDimStyle.Render(savedSecretReplaceHintLine(m.apiKeyOriginal)))
		s.WriteString("\n")
	}

	// Mirrors the env-var hint below: the step is already satisfied, so say so
	// rather than leaving an empty field that looks unconfigured.
	if m.apiKeyCmdForStep() != "" {
		s.WriteString("\n")
		s.WriteString(tuiDimStyle.Render(keyCmdConfiguredHintLine("  ", "api_key_cmd")))
		s.WriteString("\n")
	}

	if m.activeTab == tabOfficial {
		provider := m.currentProvider()
		if envKey := os.Getenv(provider.EnvVar); envKey != "" {
			s.WriteString("\n")
			hasSavedKey := m.apiKeyMasked && m.apiKeyOriginal != ""
			s.WriteString(tuiDimStyle.Render(officialAPIKeyEnvSetHintLine(provider.EnvVar, hasSavedKey)))
			s.WriteString("\n")
		} else {
			s.WriteString("\n")
			s.WriteString(tuiDimStyle.Render(fmt.Sprintf("  Tip: You can also set via env var %s", provider.EnvVar)))
			s.WriteString("\n")
		}
	}

	if m.formError != "" {
		s.WriteString("\n")
		s.WriteString(tuiErrorStyle.Render("  " + m.formError))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(tuiHelpStyle.Render("  Enter Confirm  Esc Back"))
	s.WriteString("\n")
}

// savedSecretFingerprintMinHiddenLen is the minimum number of runes that must
// sit between the visible prefix and suffix so the fingerprint does not expose
// the entire key (e.g. a 10-rune key with prefix 6 + suffix 4).
const savedSecretFingerprintMinHiddenLen = 5

// savedSecretFingerprintMinLen is the minimum trimmed secret length required
// before a fingerprint is shown. Shorter keys hide the parenthetical hint.
const savedSecretFingerprintMinLen = savedSecretFingerprintPrefixLen + savedSecretFingerprintSuffixLen + savedSecretFingerprintMinHiddenLen

// savedSecretFingerprintPrefixLen is how many leading runes to show.
const savedSecretFingerprintPrefixLen = 6

// savedSecretFingerprintSuffixLen is how many trailing runes to show.
const savedSecretFingerprintSuffixLen = 4

// savedSecretFingerprint returns a short fingerprint for display, e.g.
// "sk-a1b2...wxyz" (first 6 + "..." + last 4). Returns "" when the trimmed
// secret is shorter than savedSecretFingerprintMinLen runes.
func savedSecretFingerprint(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) < savedSecretFingerprintMinLen {
		return ""
	}
	prefix := string(runes[:savedSecretFingerprintPrefixLen])
	suffix := string(runes[len(runes)-savedSecretFingerprintSuffixLen:])
	return prefix + "..." + suffix
}

// savedSecretReplaceHint builds the replace hint text without leading indent.
func savedSecretReplaceHint(original string) string {
	hint := "Type or paste to replace the saved key."
	if fp := savedSecretFingerprint(original); fp != "" {
		hint += fmt.Sprintf("  (saved: %s)", fp)
	}
	return hint
}
func savedSecretReplaceHintLine(original string) string {
	return "  " + savedSecretReplaceHint(original)
}
func officialAPIKeyEnvSetHint(envVar string, hasSavedKey bool) string {
	if hasSavedKey {
		return fmt.Sprintf("$%s is set; used only when no key is saved here.", envVar)
	}
	return fmt.Sprintf("$%s is set. Leave empty to use it; enter a key here to override.", envVar)
}
func officialAPIKeyEnvSetHintLine(envVar string, hasSavedKey bool) string {
	return "  " + officialAPIKeyEnvSetHint(envVar, hasSavedKey)
}

// keyCmdConfiguredHint explains why this step accepts an empty field. A
// provider configured only by command renders a blank input -- the command line
// is not the secret, but it is also not the value being edited here -- so
// without this the user has no way to tell a credential is already wired up,
// and no way to know that leaving the field empty is the correct action.
// keyLabel names the config key so the hint points at what to edit instead.
//
// The command itself is deliberately not echoed. It is usually a bare reference
// (`op read op://...`), but nothing stops a user from inlining a secret into it
// (`VAULT_TOKEN=hvs.xxx vault kv get ...`), and this wizard masks every other
// credential it displays -- printing one user-authored string verbatim into
// screenshots and terminal recordings is the one hole in that. Naming the config
// key is what the hint is for and is enough to identify the command: there is
// exactly one per provider, so the user knows which value to go read or edit.
func keyCmdConfiguredHint(keyLabel string) string {
	return fmt.Sprintf("%s is set; leave empty to keep using it.", keyLabel)
}
func keyCmdConfiguredHintLine(indent, keyLabel string) string {
	return indent + keyCmdConfiguredHint(keyLabel)
}

// --- Styles ---
