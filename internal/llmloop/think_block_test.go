// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import "testing"

func TestStripThinkBlocks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no think block, unchanged",
			in:   `{"comments":[]}`,
			want: `{"comments":[]}`,
		},
		{
			name: "balanced think block before JSON",
			in:   "<think>analyze braces { not json }</think>{\"comments\":[]}",
			want: `{"comments":[]}`,
		},
		{
			name: "unclosed think tag left in place",
			in:   "<think>truncated reasoning { open",
			want: "<think>truncated reasoning { open",
		},
		{
			name: "multiple blocks stripped, prose between kept",
			in:   "<think>a { 1 }</think>mid<think>b { 2 }</think>{\"ok\":true}",
			want: `mid{"ok":true}`,
		},
		{
			name: "think prose containing json-like text",
			in:   "<think>emit {\"comments\":[]} shaped output</think>{\"comments\":[{\"path\":\"a.c\"}]}",
			want: `{"comments":[{"path":"a.c"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripThinkBlocks(tt.in); got != tt.want {
				t.Errorf("stripThinkBlocks() = %q, want %q", got, tt.want)
			}
		})
	}
}
