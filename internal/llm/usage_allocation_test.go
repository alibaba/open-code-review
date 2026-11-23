// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"strings"
	"testing"
)

func TestResolveUsageFlatAndFallbackFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want UsageInfo
	}{
		{
			name: "flat Chat Completions",
			raw:  `{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"cache_read_input_tokens":10,"cache_creation_input_tokens":5}`,
			want: UsageInfo{TotalTokens: 120, PromptTokens: 100, CompletionTokens: 20, CacheReadTokens: 10, CacheWriteTokens: 5},
		},
		{
			name: "flat Responses",
			raw:  `{"input_tokens":100,"output_tokens":20}`,
			want: UsageInfo{TotalTokens: 120, PromptTokens: 100, CompletionTokens: 20},
		},
		{
			name: "invalid nested values keep flat fallbacks",
			raw:  `{"usage":{"prompt_tokens":"unknown","completion_tokens":null},"prompt_tokens":100,"completion_tokens":20}`,
			want: UsageInfo{TotalTokens: 120, PromptTokens: 100, CompletionTokens: 20},
		},
		{
			name: "differently cased keys do not replace usage",
			raw:  `{"usage":{"total_tokens":120},"USAGE":{"total_tokens":900},"Usage":{"total_tokens":800}}`,
			want: UsageInfo{TotalTokens: 120},
		},
		{
			name: "escaped keys and duplicate values",
			raw:  `{"usage":{"total_tokens":900},"u\u0073age":{"total_tokens":120}}`,
			want: UsageInfo{TotalTokens: 120},
		},
		{
			name: "invalid duplicate replaces earlier object",
			raw:  `{"usage":{"total_tokens":900},"usage":"unknown","total_tokens":120}`,
			want: UsageInfo{TotalTokens: 120},
		},
		{
			name: "usage wins over flat and wrapped values",
			raw:  `{"usage":{"total_tokens":120,"prompt_tokens":100,"completion_tokens":20},"total_tokens":200,"data":{"usage":{"total_tokens":300}}}`,
			want: UsageInfo{TotalTokens: 120, PromptTokens: 100, CompletionTokens: 20},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUsage([]byte(tc.raw))
			if got == nil || *got != tc.want {
				t.Fatalf("resolveUsage() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func BenchmarkResolveUsageLargeCompletion(b *testing.B) {
	raw := []byte(`{"choices":[{"message":{"role":"assistant","content":"` + strings.Repeat("a", 1<<20) + `"}}],"usage":{"prompt_tokens":100,"completion_tokens":200,"total_tokens":300}}`)
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if usage := resolveUsage(raw); usage == nil || usage.TotalTokens != 300 {
			b.Fatal(usage)
		}
	}
}
