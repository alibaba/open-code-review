// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"
	"time"
)

func TestLLMTestTimeout(t *testing.T) {
	tests := []struct {
		name            string
		taskSeconds     int
		endpointTimeout time.Duration
		want            time.Duration
	}{
		{name: "keeps thirty second floor", taskSeconds: 5, endpointTimeout: 10 * time.Second, want: 30 * time.Second},
		{name: "uses task timeout", taskSeconds: 120, endpointTimeout: 0, want: 120 * time.Second},
		{name: "raises deadline to endpoint timeout", taskSeconds: 120, endpointTimeout: 5 * time.Minute, want: 5 * time.Minute},
		{name: "does not lower a longer task timeout", taskSeconds: 120, endpointTimeout: time.Minute, want: 120 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := llmTestTimeout(tt.taskSeconds, tt.endpointTimeout); got != tt.want {
				t.Errorf("llmTestTimeout(%d, %s) = %s, want %s", tt.taskSeconds, tt.endpointTimeout, got, tt.want)
			}
		})
	}
}
