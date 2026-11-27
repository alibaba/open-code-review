// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestCompareContainmentTiers(t *testing.T) {
	tests := []struct {
		name           string
		before         []model.LlmComment
		after          []model.LlmComment
		wantPersisting []string
		wantNew        []string
		wantResolved   []string
	}{
		{
			name:           "expanded selected range persists",
			before:         []model.LlmComment{cmt("db.go", 10, "bug", "rows, err := db.Query(query)", "handle the error")},
			after:          []model.LlmComment{cmt("db.go", 30, "bug", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "handle the error")},
			wantPersisting: []string{"db.go:30"},
			wantNew:        []string{},
			wantResolved:   []string{},
		},
		{
			name:           "contracted selected range persists",
			before:         []model.LlmComment{cmt("db.go", 10, "bug", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "handle the error")},
			after:          []model.LlmComment{cmt("db.go", 30, "bug", "rows, err := db.Query(query)", "handle the error")},
			wantPersisting: []string{"db.go:30"},
			wantNew:        []string{},
			wantResolved:   []string{},
		},
		{
			name:           "short generic line does not absorb a block",
			before:         []model.LlmComment{cmt("a.go", 10, "bug", "return nil", "error is swallowed")},
			after:          []model.LlmComment{cmt("a.go", 30, "bug", "if err != nil {\n    return nil\n}", "error is swallowed")},
			wantPersisting: []string{},
			wantNew:        []string{"a.go:30"},
			wantResolved:   []string{"a.go:10"},
		},
		{
			name:           "partial line overlap does not merge",
			before:         []model.LlmComment{cmt("a.go", 10, "bug", "err != nil { return err }", "handle the error")},
			after:          []model.LlmComment{cmt("a.go", 30, "bug", "if err != nil { return err }", "handle the error")},
			wantPersisting: []string{},
			wantNew:        []string{"a.go:30"},
			wantResolved:   []string{"a.go:10"},
		},
		{
			name:           "containment requires the same category",
			before:         []model.LlmComment{cmt("db.go", 10, "bug", "rows, err := db.Query(query)", "handle the error")},
			after:          []model.LlmComment{cmt("db.go", 30, "style", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "handle the error")},
			wantPersisting: []string{},
			wantNew:        []string{"db.go:30"},
			wantResolved:   []string{"db.go:10"},
		},
		{
			name:           "shared line with different finding text does not merge",
			before:         []model.LlmComment{cmt("a.go", 10, "style", "log.Printf(\"starting server\")", "use structured logging")},
			after:          []model.LlmComment{cmt("a.go", 30, "style", "log.Printf(\"starting server\")\ndef cancel()", "missing context cancellation")},
			wantPersisting: []string{},
			wantNew:        []string{"a.go:30"},
			wantResolved:   []string{"a.go:10"},
		},
		{
			name:           "missing finding text does not merge",
			before:         []model.LlmComment{cmt("db.go", 10, "bug", "rows, err := db.Query(query)", "")},
			after:          []model.LlmComment{cmt("db.go", 30, "bug", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "")},
			wantPersisting: []string{},
			wantNew:        []string{"db.go:30"},
			wantResolved:   []string{"db.go:10"},
		},
		{
			name: "containment still respects multiset matching",
			before: []model.LlmComment{
				cmt("db.go", 10, "bug", "rows, err := db.Query(query)", "handle the error"),
				cmt("db.go", 20, "bug", "rows, err := db.Query(query)", "handle the error"),
			},
			after:          []model.LlmComment{cmt("db.go", 30, "bug", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "handle the error")},
			wantPersisting: []string{"db.go:30"},
			wantNew:        []string{},
			wantResolved:   []string{"db.go:20"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.before, tt.after, nil)
			if have := ids(got.Persisting); !reflect.DeepEqual(have, tt.wantPersisting) {
				t.Errorf("persisting = %v, want %v", have, tt.wantPersisting)
			}
			if have := ids(got.New); !reflect.DeepEqual(have, tt.wantNew) {
				t.Errorf("new = %v, want %v", have, tt.wantNew)
			}
			if have := ids(got.Resolved); !reflect.DeepEqual(have, tt.wantResolved) {
				t.Errorf("resolved = %v, want %v", have, tt.wantResolved)
			}
		})
	}
}
