// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import (
	"fmt"
	"strings"
)

// Effort selects a review effort preset that controls how much work the
// review agent performs (e.g., number of review rounds).
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"

	// EffortDefault is used when neither --effort nor config "effort" is set.
	EffortDefault = EffortMedium
)

// EffortNames returns the accepted values for shell completion and error messages.
func EffortNames() []string {
	return []string{string(EffortLow), string(EffortMedium), string(EffortHigh)}
}

// EffortPreset is the concrete knob set an Effort level expands to.
type EffortPreset struct {
	MaxReviewRounds int
}

var effortPresets = map[Effort]EffortPreset{
	EffortLow:    {MaxReviewRounds: 1},
	EffortMedium: {MaxReviewRounds: 2},
	EffortHigh:   {MaxReviewRounds: 3},
}

// ParseEffort validates a user-supplied effort value.
func ParseEffort(s string) (Effort, error) {
	e := Effort(strings.ToLower(s))
	if _, ok := effortPresets[e]; !ok {
		return "", fmt.Errorf("invalid effort %q: must be one of %s", s, strings.Join(EffortNames(), ", "))
	}
	return e, nil
}

// Preset returns the knob set for e.
func (e Effort) Preset() EffortPreset {
	if p, ok := effortPresets[e]; ok {
		return p
	}
	return effortPresets[EffortDefault]
}

// ApplyEffort overwrites the effort-controlled template scalars.
func (t *Template) ApplyEffort(e Effort) {
	p := e.Preset()
	t.MaxReviewRounds = p.MaxReviewRounds
}
