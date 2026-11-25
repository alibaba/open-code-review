// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"testing"
)

func TestBoolArg(t *testing.T) {
	if got, err := boolArg(map[string]any{}, "x"); got || err != nil {
		t.Errorf("missing key = %v, %v; want false, nil", got, err)
	}
	tests := []struct {
		in      any
		want    bool
		wantErr bool
	}{
		{true, true, false},
		{false, false, false},
		{"true", true, false},
		{"false", false, false},
		{nil, false, true},
		{"1", false, true},
		{"0", false, true},
		{"T", false, true},
		{"f", false, true},
		{"TRUE", false, true},
		{" true", false, true},
		{" False ", false, true},
		{"", false, true},
		{"junk", false, true},
		{1, false, true},
	}
	for _, tt := range tests {
		got, err := boolArg(map[string]any{"x": tt.in}, "x")
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("boolArg(%#v) = %v, %v", tt.in, got, err)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "x ") {
			t.Errorf("error should name the field: %v", err)
		}
	}
}

func TestNormalizeBoolArgs(t *testing.T) {
	args := map[string]any{"a": "true", "b": false, "text": "true"}
	if err := NormalizeBoolArgs(args, "a", "b", "missing"); err != nil {
		t.Fatal(err)
	}
	if args["a"] != true || args["b"] != false || args["text"] != "true" {
		t.Errorf("args = %#v", args)
	}
	if _, present := args["missing"]; present {
		t.Error("a missing option must stay missing")
	}
	err := NormalizeBoolArgs(map[string]any{"a": "1"}, "a")
	if err == nil || !strings.Contains(err.Error(), "a must be a boolean") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchAndFindInvalidBoolean(t *testing.T) {
	dir := setupTestRepo(t)
	r := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	for name, tc := range map[string]struct {
		run   func() (string, error)
		field string
	}{
		"search": {func() (string, error) {
			return NewCodeSearch(r).Execute(context.Background(), map[string]any{"search_text": "a", "case_sensitive": "junk"})
		}, "case_sensitive"},
		"perl": {func() (string, error) {
			return NewCodeSearch(r).Execute(context.Background(), map[string]any{"search_text": "a", "use_perl_regexp": 1})
		}, "use_perl_regexp"},
		"find": {func() (string, error) {
			return NewFileFind(r).Execute(context.Background(), map[string]any{"query_name": "a", "case_sensitive": "junk"})
		}, "case_sensitive"},
		"find blank query": {func() (string, error) {
			return NewFileFind(r).Execute(context.Background(), map[string]any{"query_name": " ", "case_sensitive": "1"})
		}, "case_sensitive"},
		"find null": {func() (string, error) {
			return NewFileFind(r).Execute(context.Background(), map[string]any{"query_name": "a", "case_sensitive": nil})
		}, "case_sensitive"},
	} {
		got, err := tc.run()
		if err == nil || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("%s: got %q, %v; want an error naming %s", name, got, err, tc.field)
		}
	}
}
