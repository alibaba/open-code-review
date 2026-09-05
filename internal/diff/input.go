// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"

	"github.com/alibaba/open-code-review/internal/model"
)

// InputProvider is the common diff input contract used by the review agent.
type InputProvider interface {
	GetDiff(context.Context) ([]model.Diff, error)
	ResolveInput(context.Context) InputResolution
	RemoteIdentity(context.Context) string
}
