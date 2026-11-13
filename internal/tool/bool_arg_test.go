// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"testing"
)

func TestBoolArg(t *testing.T) {
	tests := []struct {
		in      any
		want    bool
		wantErr bool
	}{
		{nil, false, false},
		{true, true, false},
		{false, false, false},
		{"true", true, false},
		{" False ", false, false},
		{"1", true, false},
		{"0", false, false},
		{"T", true, false},
		{"f", false, false},
		{"junk", false, true},
		{1, false, true},
	}
	for _, tt := range tests {
		got, err := boolArg(map[string]any{"x": tt.in}, "x")
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("boolArg(%#v) = %v, %v", tt.in, got, err)
		}
		if err != nil && !strings.Contains(err.Error(), "x") {
			t.Errorf("error should name field: %v", err)
		}
	}
}

func TestSearchAndFindInvalidBoolean(t *testing.T) {
	dir := setupTestRepo(t)
	r := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	for name, run := range map[string]func() (string, error){
		"search": func() (string, error) {
			return NewCodeSearch(r).Execute(context.Background(), map[string]any{"search_text": "a", "case_sensitive": "junk"})
		},
		"perl": func() (string, error) {
			return NewCodeSearch(r).Execute(context.Background(), map[string]any{"search_text": "a", "use_perl_regexp": 1})
		},
		"find": func() (string, error) {
			return NewFileFind(r).Execute(context.Background(), map[string]any{"query_name": "a", "case_sensitive": "junk"})
		},
	} {
		got, err := run()
		if err != nil || !strings.HasPrefix(got, "Error:") {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}
}
