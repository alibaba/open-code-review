// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package location defines source-side provenance for resolved review comments.
package location

// Side identifies which version of a changed file supplied a resolved location.
type Side string

const (
	SideUnknown Side = ""
	SideOld     Side = "OLD"
	SideNew     Side = "NEW"
)
