// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"reflect"
	"strings"
	"testing"
)

func TestBrowserCandidates(t *testing.T) {
	const url = "http://localhost:5483"
	tests := []struct {
		name       string
		goos       string
		browserEnv string
		want       [][]string
	}{
		{"darwin", "darwin", "", [][]string{{"open", url}}},
		{"windows", "windows", "", [][]string{{"rundll32", "url.dll,FileProtocolHandler", url}}},
		{"linux", "linux", "", [][]string{{"xdg-open", url}}},
		{"unknown falls back to xdg-open", "plan9", "", [][]string{{"xdg-open", url}}},
		{
			"browser env takes precedence",
			"linux", "firefox",
			[][]string{{"firefox", url}, {"xdg-open", url}},
		},
		{
			"browser env list is tried in order",
			"linux", "firefox:chromium",
			[][]string{{"firefox", url}, {"chromium", url}, {"xdg-open", url}},
		},
		{
			"browser env honors %s placeholder and extra args",
			"darwin", "open -a Safari %s",
			[][]string{{"open", "-a", "Safari", url}, {"open", url}},
		},
		{
			"blank browser env entries are skipped",
			"linux", ":: firefox :",
			[][]string{{"firefox", url}, {"xdg-open", url}},
		},
		{
			"browser env ignored on windows so drive letters survive",
			"windows", `C:\Program Files\Mozilla Firefox\firefox.exe`,
			[][]string{{"rundll32", "url.dll,FileProtocolHandler", url}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := browserCandidates(tt.goos, tt.browserEnv, url)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("browserCandidates(%q, %q, %q) = %v, want %v", tt.goos, tt.browserEnv, url, got, tt.want)
			}
		})
	}
}

func TestValidateOpenMode(t *testing.T) {
	for _, mode := range []string{OpenAuto, OpenAlways, OpenNever} {
		if err := ValidateOpenMode(mode); err != nil {
			t.Errorf("ValidateOpenMode(%q) = %v, want nil", mode, err)
		}
	}
	for _, mode := range []string{"", "yes", "true", "Auto", "no-open"} {
		err := ValidateOpenMode(mode)
		if err == nil {
			t.Errorf("ValidateOpenMode(%q) = nil, want error", mode)
			continue
		}
		if !strings.Contains(err.Error(), "auto, always, never") {
			t.Errorf("ValidateOpenMode(%q) error = %q, want it to list the valid values", mode, err)
		}
	}
}
