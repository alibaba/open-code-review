// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"testing"
)

type extraRepo struct{}

func (extraRepo) ResolveCommit(_ context.Context, _ string) error { return nil }

func TestParserWithRepo(t *testing.T) {
	p := NewParser(nil, nil)
	if p.WithRepo(extraRepo{}).repo == nil {
		t.Fatal("repo was not assigned")
	}
}
