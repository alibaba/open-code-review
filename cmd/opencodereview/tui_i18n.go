// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed tui_locales/*.json
var tuiLocaleFiles embed.FS

var tuiLanguageIDs = []string{"en", "zh-CN", "ja-JP", "ko-KR", "ru-RU"}

type tuiLocale struct {
	Name     string            `json:"name"`
	Messages map[string]string `json:"messages"`
}

var tuiLocales = loadTUILocales()

func loadTUILocales() map[string]tuiLocale {
	locales := make(map[string]tuiLocale, len(tuiLanguageIDs))
	for _, id := range tuiLanguageIDs {
		data, err := tuiLocaleFiles.ReadFile("tui_locales/" + id + ".json")
		if err != nil {
			panic(fmt.Sprintf("read embedded TUI language %s: %v", id, err))
		}
		var locale tuiLocale
		if err := json.Unmarshal(data, &locale); err != nil {
			panic(fmt.Sprintf("decode embedded TUI language %s: %v", id, err))
		}
		locales[id] = locale
	}
	return locales
}

func tuiText(language, key string, args ...any) string {
	message := tuiLocales[language].Messages[key]
	if message == "" {
		message = tuiLocales["en"].Messages[key]
	}
	if message == "" {
		message = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

func tuiLanguageName(id string) string {
	if locale, ok := tuiLocales[id]; ok {
		return locale.Name
	}
	return id
}

type tuiPreferences struct {
	Language string `json:"language"`
}

func tuiPreferencesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "tui.json"), nil
}

func loadTUIPreferences() (tuiPreferences, error) {
	defaults := tuiPreferences{Language: "en"}
	path, err := tuiPreferencesPath()
	if err != nil {
		return defaults, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("read TUI settings: %w", err)
	}
	prefs := new(tuiPreferences)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&prefs); err != nil {
		return defaults, fmt.Errorf("decode TUI settings: %w", err)
	}
	if prefs == nil {
		return defaults, fmt.Errorf("TUI settings must be a JSON object")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return defaults, fmt.Errorf("TUI settings must contain one JSON object")
	}
	if _, ok := tuiLocales[prefs.Language]; !ok {
		return defaults, fmt.Errorf("unsupported TUI language %q", prefs.Language)
	}
	return *prefs, nil
}

// saveTUIPreferences is called only after the user applies a language choice.
// The separate file leaves provider configuration unchanged.
func saveTUIPreferences(prefs tuiPreferences) error {
	if _, ok := tuiLocales[prefs.Language]; !ok {
		return fmt.Errorf("unsupported TUI language %q", prefs.Language)
	}
	path, err := tuiPreferencesPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create TUI settings directory: %w", err)
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode TUI settings: %w", err)
	}
	file, err := os.CreateTemp(dir, ".tui-*.json")
	if err != nil {
		return fmt.Errorf("create TUI settings file: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write TUI settings: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync TUI settings: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close TUI settings: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace TUI settings: %w", err)
	}
	return nil
}
