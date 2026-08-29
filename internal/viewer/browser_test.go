// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"reflect"
	"testing"
)

func TestBrowserOpenCmd(t *testing.T) {
	const url = "http://localhost:5483"
	tests := []struct {
		name string
		goos string
		want []string
	}{
		{"darwin", "darwin", []string{"open", url}},
		{"windows", "windows", []string{"rundll32", "url.dll,FileProtocolHandler", url}},
		{"linux", "linux", []string{"xdg-open", url}},
		{"unknown falls back to xdg-open", "plan9", []string{"xdg-open", url}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := browserOpenCmd(tt.goos, url)
			if !reflect.DeepEqual(got.Args, tt.want) {
				t.Errorf("browserOpenCmd(%q, %q).Args = %v, want %v", tt.goos, url, got.Args, tt.want)
			}
		})
	}
}
