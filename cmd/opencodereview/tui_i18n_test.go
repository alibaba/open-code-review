// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

func TestTUILanguageCatalogs(t *testing.T) {
	if want := []string{"en", "zh-CN", "ja-JP", "ko-KR", "ru-RU"}; !reflect.DeepEqual(tuiLanguageIDs, want) {
		t.Fatalf("languages = %v, want %v", tuiLanguageIDs, want)
	}
	formats := regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)
	english := tuiLocales["en"]
	for _, id := range tuiLanguageIDs {
		t.Run(id, func(t *testing.T) {
			locale := tuiLocales[id]
			if locale.Name == "" || tuiLanguageName(id) != locale.Name {
				t.Fatal("language must have a native display name")
			}
			if len(locale.Messages) != len(english.Messages) {
				t.Fatalf("message count = %d, want %d", len(locale.Messages), len(english.Messages))
			}
			for key := range english.Messages {
				message, ok := locale.Messages[key]
				if !ok || strings.TrimSpace(message) == "" {
					t.Errorf("missing translation for %q", key)
					continue
				}
				if want := formats.FindAllString(key, -1); !reflect.DeepEqual(formats.FindAllString(message, -1), want) {
					t.Errorf("format placeholders changed for %q: %q", key, message)
				}
				if id == "en" && message != key {
					t.Errorf("English message differs from key %q", key)
				}
			}
			if id != "en" {
				data, err := tuiLocaleFiles.ReadFile("tui_locales/" + id + ".json")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.ContainsFunc(string(data), func(r rune) bool { return r > unicode.MaxASCII && unicode.IsLetter(r) }) {
					t.Fatal("localized resource does not contain native letters")
				}
				if locale.Messages["Settings"] == "Settings" {
					t.Fatal("settings must be translated")
				}
			}
		})
	}
}

func TestTUITextFallbackAndFormatting(t *testing.T) {
	if got := tuiText("unknown", "Settings"); got != "Settings" {
		t.Fatalf("unknown language fallback = %q", got)
	}
	if got := tuiText("ja-JP", "Uncatalogued message"); got != "Uncatalogued message" {
		t.Fatalf("unknown key fallback = %q", got)
	}
	if got := tuiText("en", "Could not save settings: %s", "disk full"); got != "Could not save settings: disk full" {
		t.Fatalf("formatting = %q", got)
	}
	if got := tuiText("unknown", "%d things", 4); got != "4 things" {
		t.Fatalf("formatted fallback = %q", got)
	}
	for _, id := range tuiLanguageIDs {
		got := tuiText(id, "Could not save settings: %s", "disk full")
		if !strings.HasSuffix(got, "disk full") || strings.Contains(got, "%!") {
			t.Errorf("%s: formatted translation = %q", id, got)
		}
	}
	if got := tuiLanguageName("unknown"); got != "unknown" {
		t.Fatalf("unknown language name = %q", got)
	}
}

func TestTUICommandDescriptionsAreLocalized(t *testing.T) {
	for _, id := range tuiLanguageIDs {
		for _, command := range tuiCommands {
			if tuiLocales[id].Messages[command.description] == "" {
				t.Errorf("%s: missing description for %s", id, command.name)
			}
		}
	}
}

func TestTUIPreferencesDefault(t *testing.T) {
	setTestHome(t, t.TempDir())
	prefs, err := loadTUIPreferences()
	if err != nil || prefs.Language != "en" {
		t.Fatalf("missing preferences = %+v, %v", prefs, err)
	}
	path, err := tuiPreferencesPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("loading defaults created preferences: %v", err)
	}
}

func TestTUIPreferencesPersistAndReplace(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	dir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(dir, "config.json")
	providerConfig := []byte(`{"provider":"existing"}`)
	if err := os.WriteFile(providerPath, providerConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range tuiLanguageIDs {
		if err := saveTUIPreferences(tuiPreferences{Language: id}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
		prefs, err := loadTUIPreferences()
		if err != nil || prefs.Language != id {
			t.Fatalf("load %s: %+v, %v", id, prefs, err)
		}
	}
	data, err := os.ReadFile(providerPath)
	if err != nil || string(data) != string(providerConfig) {
		t.Fatalf("provider configuration changed: %q, %v", data, err)
	}
	path, err := tuiPreferencesPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("preferences permissions = %o, want 600", info.Mode().Perm())
	}
	assertNoTUIPreferencesTemps(t, dir)
}

func TestTUIPreferencesRejectMalformedWithoutWriting(t *testing.T) {
	for _, data := range []string{
		`{`, `null`, `[]`, `{}`, `{"language":null}`, `{"language":""}`,
		`{"language":"unsupported"}`, `{"language":"en","extra":true}`,
		`{"language":"en"} {"language":"ja-JP"}`, `{"language":"en"} trailing`,
	} {
		t.Run(data, func(t *testing.T) {
			home := t.TempDir()
			setTestHome(t, home)
			dir := filepath.Join(home, ".opencodereview")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "tui.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			prefs, err := loadTUIPreferences()
			if err == nil || prefs.Language != "en" {
				t.Fatalf("invalid settings = %+v, %v", prefs, err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != data {
				t.Fatalf("invalid settings were overwritten: %q, %v", got, err)
			}
		})
	}
}

func TestTUIPreferencesExplicitSaveRepairsSettings(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	dir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tui.json"), []byte("malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveTUIPreferences(tuiPreferences{Language: "ja-JP"}); err != nil {
		t.Fatal(err)
	}
	prefs, err := loadTUIPreferences()
	if err != nil || prefs.Language != "ja-JP" {
		t.Fatalf("explicit save = %+v, %v", prefs, err)
	}
}

func TestTUIPreferencesRejectUnsupportedSave(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	if err := saveTUIPreferences(tuiPreferences{Language: "ko-KR"}); err != nil {
		t.Fatal(err)
	}
	if err := saveTUIPreferences(tuiPreferences{Language: "unsupported"}); err == nil {
		t.Fatal("unsupported language was saved")
	}
	prefs, err := loadTUIPreferences()
	if err != nil || prefs.Language != "ko-KR" {
		t.Fatalf("invalid save replaced settings: %+v, %v", prefs, err)
	}
}

func TestTUIPreferencesHomeFailure(t *testing.T) {
	setTestHome(t, "")
	if _, err := loadTUIPreferences(); err == nil {
		t.Fatal("load should report missing home directory")
	}
	if err := saveTUIPreferences(tuiPreferences{Language: "en"}); err == nil {
		t.Fatal("save should report missing home directory")
	}
}

func TestTUIPreferencesReadFailure(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".opencodereview", "tui.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadTUIPreferences(); err == nil || !strings.Contains(err.Error(), "read TUI settings") {
		t.Fatalf("read directory error = %v", err)
	}
}

func TestTUIPreferencesSaveDirectoryFailure(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	if err := os.WriteFile(filepath.Join(home, ".opencodereview"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveTUIPreferences(tuiPreferences{Language: "en"}); err == nil || !strings.Contains(err.Error(), "create TUI settings directory") {
		t.Fatalf("blocked directory error = %v", err)
	}
}

func TestTUIPreferencesSaveReplacementFailure(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	dir := filepath.Join(home, ".opencodereview")
	path := filepath.Join(dir, "tui.json")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := saveTUIPreferences(tuiPreferences{Language: "en"}); err == nil || !strings.Contains(err.Error(), "replace TUI settings") {
		t.Fatalf("replacement error = %v", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("existing directory changed: %v", err)
	}
	assertNoTUIPreferencesTemps(t, dir)
}

func assertNoTUIPreferencesTemps(t *testing.T, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, ".tui-*.json"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("settings temporary files remain: %v, %v", paths, err)
	}
}
